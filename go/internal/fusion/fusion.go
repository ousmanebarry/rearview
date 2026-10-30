// Package fusion turns per-frame observations from the engine, the GPS speed
// and motion events into scored vision events: near-misses, following too
// close, and being tailgated.
package fusion

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"time"

	"rearview/internal/config"
	"rearview/internal/store"
	"rearview/internal/trips"
)

// Types are the event types fusion owns.
var Types = []string{"near_miss", "following_close", "tailgated"}

// maxGapS is the longest gap between two observations of a track that still
// counts as consecutive (analysis runs at 5-10 fps).
const maxGapS = 0.35

// Config is what the rules need from the config file.
type Config struct {
	NearMiss  config.NearMiss
	Following config.Following
}

// Input is one trip's data.
type Input struct {
	Segments []store.Segment
	Obs      map[int64][]store.Observation // by segment id
	Track    []store.GPSPoint              // stitched trip GPS, sorted by time
	Brakes   []time.Time                   // hard-brake event times
}

type obs struct {
	store.Observation
	at  time.Time
	seg store.Segment
}

// Detect applies the near-miss, following-close and tailgated rules and
// merges events of one type closer than the merge window.
func Detect(in Input, cfg Config) []store.Event {
	tracks := map[string][]obs{} // key: segment/track
	var keys []string
	for _, s := range in.Segments {
		for _, o := range in.Obs[s.ID] {
			k := fmt.Sprintf("%d/%d", s.ID, o.TrackID)
			if _, ok := tracks[k]; !ok {
				keys = append(keys, k)
			}
			tracks[k] = append(tracks[k], obs{o, s.StartedAt.Add(secs(o.TOffsetS)), s})
		}
	}
	sort.Strings(keys)
	var cs []cand
	for _, k := range keys {
		os := tracks[k]
		sort.Slice(os, func(i, j int) bool { return os[i].TOffsetS < os[j].TOffsetS })
		cs = append(cs, nearMisses(os, in, cfg)...)
		cs = append(cs, following(os, in, cfg)...)
	}
	return merge(cs, cfg.NearMiss.MergeS)
}

func secs(s float64) time.Duration { return time.Duration(s * float64(time.Second)) }

// InPath reports whether a box's bottom-centre lies in the path trapezoid
// (corners as fractions of the frame: bottom-left, top-left, top-right, bottom-right).
func InPath(o store.Observation, trap [][2]float64, w, h int) bool {
	if len(trap) != 4 || w == 0 || h == 0 {
		return true
	}
	x := (o.X1 + o.X2) / 2 / float64(w)
	y := o.Y2 / float64(h)
	inside := false
	for i, j := 0, 3; i < 4; j, i = i, i+1 {
		xi, yi, xj, yj := trap[i][0], trap[i][1], trap[j][0], trap[j][1]
		if (yi > y) != (yj > y) && x < (xj-xi)*(y-yi)/(yj-yi)+xi {
			inside = !inside
		}
	}
	return inside
}

// runs splits a track into runs of consecutive observations satisfying ok.
func runs(os []obs, ok func(obs) bool) [][]obs {
	var out [][]obs
	var cur []obs
	for _, o := range os {
		if !ok(o) {
			if len(cur) > 0 {
				out = append(out, cur)
				cur = nil
			}
			continue
		}
		if len(cur) > 0 && o.TOffsetS-cur[len(cur)-1].TOffsetS > maxGapS {
			out = append(out, cur)
			cur = nil
		}
		cur = append(cur, o)
	}
	if len(cur) > 0 {
		out = append(out, cur)
	}
	return out
}

func nearMisses(os []obs, in Input, cfg Config) []cand {
	c := cfg.NearMiss
	ok := func(o obs) bool {
		return o.TTCS != nil && *o.TTCS < c.TTCS && InPath(o.Observation, c.PathTrapezoid, o.seg.Width, o.seg.Height)
	}
	var out []cand
	for _, r := range runs(os, ok) {
		if len(r) < c.MinFrames {
			continue
		}
		worst := r[0]
		for _, o := range r {
			if *o.TTCS < *worst.TTCS {
				worst = o
			}
		}
		score := 0.5 + 0.5*(c.TTCS-*worst.TTCS)/c.TTCS
		source := "vision"
		brake := false
		for _, b := range in.Brakes {
			if !b.Before(r[0].at.Add(-secs(c.BrakeBoostS))) && !b.After(r[len(r)-1].at.Add(secs(c.BrakeBoostS))) {
				brake = true
				break
			}
		}
		if brake {
			score += 0.25
			source = "fused"
		}
		details := map[string]any{"min_ttc_s": round(*worst.TTCS, 2), "cls": worst.Cls, "track_id": worst.TrackID,
			"frames": len(r), "camera": worst.seg.Camera, "hard_brake": brake}
		if worst.DistM != nil {
			details["dist_m"] = round(*worst.DistM, 1)
		}
		out = append(out, event("near_miss", source, r, worst, math.Min(1, score), details, in.Track))
	}
	return out
}

func following(os []obs, in Input, cfg Config) []cand {
	c := cfg.Following
	typ := "following_close"
	if len(os) > 0 && os[0].seg.Camera == "rear" {
		typ = "tailgated"
	}
	ft := func(o obs) (float64, bool) {
		if o.DistM == nil || !InPath(o.Observation, cfg.NearMiss.PathTrapezoid, o.seg.Width, o.seg.Height) {
			return 0, false
		}
		v, ok := SpeedAt(in.Track, o.at)
		if !ok || v < c.MinSpeedKmh {
			return 0, false
		}
		return *o.DistM / (v / 3.6), true
	}
	ok := func(o obs) bool {
		t, ok := ft(o)
		return ok && t < c.TimeS
	}
	var out []cand
	for _, r := range runs(os, ok) {
		if r[len(r)-1].TOffsetS-r[0].TOffsetS < c.MinDurationS {
			continue
		}
		worst, worstT := r[0], math.Inf(1)
		for _, o := range r {
			if t, _ := ft(o); t < worstT {
				worst, worstT = o, t
			}
		}
		v, _ := SpeedAt(in.Track, worst.at)
		details := map[string]any{"min_follow_s": round(worstT, 2), "dist_m": round(*worst.DistM, 1),
			"speed_kmh": round(v, 1), "cls": worst.Cls, "track_id": worst.TrackID, "camera": worst.seg.Camera,
			"duration_s": round(r[len(r)-1].TOffsetS-r[0].TOffsetS, 1)}
		score := 0.5 + 0.5*(c.TimeS-worstT)/c.TimeS
		out = append(out, event(typ, "vision", r, worst, math.Min(1, score), details, in.Track))
	}
	return out
}

// cand is an event with its absolute span, for merging.
type cand struct {
	ev       store.Event
	from, to time.Time
}

func event(typ, source string, r []obs, peak obs, score float64, details map[string]any, track []store.GPSPoint) cand {
	b, _ := json.Marshal(details)
	e := store.Event{
		TripID: peak.seg.TripID, SegmentID: peak.seg.ID, Camera: peak.seg.Camera, Type: typ, Source: source,
		TS: peak.at, TStartS: r[0].at.Sub(peak.seg.StartedAt).Seconds(), TEndS: r[len(r)-1].at.Sub(peak.seg.StartedAt).Seconds(),
		Score: score, Details: string(b),
	}
	if lat, lon, ok := PositionAt(track, peak.at); ok {
		e.Lat, e.Lon, e.HasPos = lat, lon, true
	}
	return cand{e, r[0].at, r[len(r)-1].at}
}

func round(v float64, d int) float64 {
	k := math.Pow(10, float64(d))
	return math.Round(v*k) / k
}

// merge joins events of the same type whose spans are closer than gapS
// (whichever camera or track saw them), keeping the highest score.
func merge(cs []cand, gapS float64) []store.Event {
	sort.SliceStable(cs, func(i, j int) bool {
		if cs[i].ev.Type != cs[j].ev.Type {
			return cs[i].ev.Type < cs[j].ev.Type
		}
		return cs[i].from.Before(cs[j].from)
	})
	var out []cand
	for _, c := range cs {
		n := len(out)
		if n > 0 && out[n-1].ev.Type == c.ev.Type && c.from.Sub(out[n-1].to).Seconds() < gapS {
			m := &out[n-1]
			to := m.to
			if c.to.After(to) {
				to = c.to
			}
			from := m.from
			if c.ev.Score > m.ev.Score {
				*m = c
			}
			m.from, m.to = from, to
			continue
		}
		out = append(out, c)
	}
	evs := make([]store.Event, len(out))
	for i, c := range out {
		evs[i] = c.ev
	}
	return evs
}

// SpeedAt interpolates the GPS speed (km/h) at t; false outside the track or
// across a gap longer than 10 s.
func SpeedAt(track []store.GPSPoint, t time.Time) (float64, bool) {
	i, f, ok := locate(track, t)
	if !ok {
		return 0, false
	}
	return track[i].SpeedKmh + f*(track[i+1].SpeedKmh-track[i].SpeedKmh), true
}

// PositionAt interpolates the GPS position at t.
func PositionAt(track []store.GPSPoint, t time.Time) (float64, float64, bool) {
	i, f, ok := locate(track, t)
	if !ok {
		return 0, 0, false
	}
	a, b := track[i], track[i+1]
	return a.Lat + f*(b.Lat-a.Lat), a.Lon + f*(b.Lon-a.Lon), true
}

func locate(track []store.GPSPoint, t time.Time) (int, float64, bool) {
	n := len(track)
	if n == 0 {
		return 0, 0, false
	}
	j := sort.Search(n, func(i int) bool { return track[i].TS.After(t) })
	if j == 0 || j == n {
		if j == n && n > 0 && t.Sub(track[n-1].TS) <= time.Second {
			if n == 1 {
				return 0, 0, false
			}
			return n - 2, 1, true
		}
		return 0, 0, false
	}
	a, b := track[j-1], track[j]
	span := b.TS.Sub(a.TS).Seconds()
	if span > 10 {
		return 0, 0, false
	}
	return j - 1, t.Sub(a.TS).Seconds() / span, true
}

// Rebuild recomputes every fused event from the stored observations.
func Rebuild(ctx context.Context, db *store.DB, cfg Config) (int, error) {
	ts, err := db.Trips(ctx)
	if err != nil {
		return 0, err
	}
	brakes, err := db.Events(ctx, store.EventFilter{Types: []string{"hard_brake"}})
	if err != nil {
		return 0, err
	}
	var all []store.Event
	for _, t := range ts {
		track, segs, err := trips.Track(ctx, db, t.ID)
		if err != nil {
			return 0, err
		}
		in := Input{Segments: segs, Obs: map[int64][]store.Observation{}, Track: track}
		for _, s := range segs {
			if in.Obs[s.ID], err = db.Observations(ctx, s.ID); err != nil {
				return 0, err
			}
		}
		for _, b := range brakes {
			if b.TripID == t.ID {
				in.Brakes = append(in.Brakes, b.TS)
			}
		}
		all = append(all, Detect(in, cfg)...)
	}
	if _, err := db.ReplaceEvents(ctx, "fused", Types, nil); err != nil {
		return 0, err
	}
	if _, err := db.ReplaceEvents(ctx, "vision", Types, all); err != nil {
		return 0, err
	}
	return len(all), nil
}
