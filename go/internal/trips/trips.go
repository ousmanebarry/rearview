// Package trips groups segments into drives: it corrects the camera clock with
// GPS time, pairs front and rear files, fills short GPS gaps, and computes
// each trip's distance and speeds.
package trips

import (
	"context"
	"fmt"
	"math"
	"slices"
	"sort"
	"time"

	"rearview/internal/config"
	"rearview/internal/store"
)

const earthRadiusM = 6371008.8

// Haversine returns the great-circle distance in metres.
func Haversine(lat1, lon1, lat2, lon2 float64) float64 {
	rad := math.Pi / 180
	dLat := (lat2 - lat1) * rad
	dLon := (lon2 - lon1) * rad
	a := math.Sin(dLat/2)*math.Sin(dLat/2) + math.Cos(lat1*rad)*math.Cos(lat2*rad)*math.Sin(dLon/2)*math.Sin(dLon/2)
	return 2 * earthRadiusM * math.Asin(math.Min(1, math.Sqrt(a)))
}

// Rebuild reassembles every trip from the segments and GPS in the database
// and returns the number of trips.
func Rebuild(ctx context.Context, db *store.DB, cfg config.Trips) (int, error) {
	segs, err := db.Segments(ctx, 0)
	if err != nil {
		return 0, err
	}
	gps, err := db.AllGPS(ctx)
	if err != nil {
		return 0, err
	}
	plan := Assemble(segs, gps, cfg)
	if err := db.ApplyTrips(ctx, plan); err != nil {
		return 0, fmt.Errorf("save trips: %w", err)
	}
	return len(plan.Trips), nil
}

// Assemble is the pure part of Rebuild. segs carry their current trip_id,
// which is used to keep trip ids stable across rebuilds.
func Assemble(segs []store.Segment, gps map[int64][]store.GPSPoint, cfg config.Trips) store.TripPlan {
	segs = slices.Clone(segs)
	fixClocks(segs, gps)
	sort.SliceStable(segs, func(i, j int) bool {
		if !segs[i].StartedAt.Equal(segs[j].StartedAt) {
			return segs[i].StartedAt.Before(segs[j].StartedAt)
		}
		return segs[i].Camera < segs[j].Camera
	})
	pair(segs, time.Duration(cfg.PairToleranceS*float64(time.Second)))

	var plan store.TripPlan
	for _, s := range segs {
		plan.Segments = append(plan.Segments, store.SegmentUpdate{
			ID: s.ID, StartedAt: s.StartedAt, ClockOffsetS: s.ClockOffsetS, PairKey: s.PairKey,
		})
		plan.Interpolated = append(plan.Interpolated, interpolate(s.ID, gps[s.ID], cfg.InterpolateMaxGapS)...)
	}

	gap := time.Duration(cfg.GapMinutes * float64(time.Minute))
	var groups [][]store.Segment
	var end time.Time
	for _, s := range segs {
		if s.Parking {
			continue
		}
		if len(groups) == 0 || s.StartedAt.Sub(end) > gap {
			groups = append(groups, nil)
			end = time.Time{}
		}
		groups[len(groups)-1] = append(groups[len(groups)-1], s)
		if s.End().After(end) {
			end = s.End()
		}
	}

	claimed := map[int64]bool{}
	for _, g := range groups {
		t := summarize(g, gps, cfg)
		t.ReuseID = reuseID(g, claimed)
		plan.Trips = append(plan.Trips, t)
	}
	return plan
}

// fixClocks sets each segment's clock offset to the median difference between
// its GPS times and its file-name times, and copies the nearest segment's
// offset to segments without a fix. StartedAt is moved by the change.
func fixClocks(segs []store.Segment, gps map[int64][]store.GPSPoint) {
	type known struct {
		base   time.Time
		offset float64
	}
	var ks []known
	has := make([]bool, len(segs))
	for i, s := range segs {
		base := s.StartedAt.Add(-secs(s.ClockOffsetS))
		var diffs []float64
		for _, p := range gps[s.ID] {
			if !p.Interpolated {
				diffs = append(diffs, p.TS.Sub(base).Seconds()-p.TOffsetS)
			}
		}
		if len(diffs) == 0 {
			continue
		}
		slices.Sort(diffs)
		off := diffs[len(diffs)/2]
		if len(diffs)%2 == 0 {
			off = (diffs[len(diffs)/2-1] + off) / 2
		}
		segs[i].ClockOffsetS = off
		segs[i].StartedAt = base.Add(secs(off))
		has[i] = true
		ks = append(ks, known{base, off})
	}
	for i, s := range segs {
		if has[i] {
			continue
		}
		base := s.StartedAt.Add(-secs(s.ClockOffsetS))
		off := 0.0
		best := time.Duration(math.MaxInt64)
		for _, k := range ks {
			d := k.base.Sub(base)
			if d < 0 {
				d = -d
			}
			if d < best {
				best, off = d, k.offset
			}
		}
		segs[i].ClockOffsetS = off
		segs[i].StartedAt = base.Add(secs(off))
	}
}

func secs(s float64) time.Duration { return time.Duration(s * float64(time.Second)) }

// PairKey is the pair_key value for a moment starting at t.
func PairKey(t time.Time) string { return t.UTC().Format("20060102T150405Z") }

// pair gives a front segment and the nearest rear segment starting within tol
// the same pair key. Unpaired segments get a key of their own.
func pair(segs []store.Segment, tol time.Duration) {
	used := make([]bool, len(segs))
	for i := range segs {
		segs[i].PairKey = ""
	}
	for i, f := range segs {
		if f.Camera != "front" {
			continue
		}
		best, bestD := -1, tol+1
		for j, r := range segs {
			if r.Camera != "rear" || used[j] {
				continue
			}
			d := r.StartedAt.Sub(f.StartedAt)
			if d < 0 {
				d = -d
			}
			if d <= tol && d < bestD {
				best, bestD = j, d
			}
		}
		if best >= 0 {
			used[best] = true
			first := f.StartedAt
			if segs[best].StartedAt.Before(first) {
				first = segs[best].StartedAt
			}
			segs[i].PairKey = PairKey(first)
			segs[best].PairKey = segs[i].PairKey
		}
	}
	for i := range segs {
		if segs[i].PairKey == "" {
			segs[i].PairKey = PairKey(segs[i].StartedAt) + "-" + segs[i].Camera
		}
	}
}

// interpolate fills gaps shorter than maxGap seconds between real points with
// one point per second on a straight line.
func interpolate(segID int64, pts []store.GPSPoint, maxGap float64) []store.GPSPoint {
	var out []store.GPSPoint
	var prev *store.GPSPoint
	for i := range pts {
		p := &pts[i]
		if p.Interpolated {
			continue
		}
		if prev != nil {
			dt := p.TOffsetS - prev.TOffsetS
			if dt > 1 && dt < maxGap {
				for k := 1.0; k < dt; k++ {
					f := k / dt
					out = append(out, store.GPSPoint{
						SegmentID:    segID,
						TOffsetS:     prev.TOffsetS + k,
						TS:           prev.TS.Add(secs(k)),
						Lat:          prev.Lat + f*(p.Lat-prev.Lat),
						Lon:          prev.Lon + f*(p.Lon-prev.Lon),
						SpeedKmh:     prev.SpeedKmh + f*(p.SpeedKmh-prev.SpeedKmh),
						HeadingDeg:   lerpAngle(prev.HeadingDeg, p.HeadingDeg, f),
						Interpolated: true,
					})
				}
			}
		}
		prev = p
	}
	return out
}

// HeadingDelta is the signed smallest change from a to b in degrees (-180, 180].
func HeadingDelta(a, b float64) float64 {
	d := math.Mod(b-a, 360)
	if d > 180 {
		d -= 360
	} else if d <= -180 {
		d += 360
	}
	return d
}

func lerpAngle(a, b, f float64) float64 {
	v := math.Mod(a+f*HeadingDelta(a, b)+360, 360)
	return v
}

// summarize computes a trip's times, distance and speeds from its segments,
// using one camera's GPS per moment.
func summarize(g []store.Segment, gps map[int64][]store.GPSPoint, cfg config.Trips) store.PlannedTrip {
	t := store.PlannedTrip{}
	t.StartedAt = g[0].StartedAt
	for _, s := range g {
		t.SegmentIDs = append(t.SegmentIDs, s.ID)
		if s.End().After(t.EndedAt) {
			t.EndedAt = s.End()
		}
	}
	t.DurationS = t.EndedAt.Sub(t.StartedAt).Seconds()
	track := stitch(g, gps)
	for i, p := range track {
		if p.SpeedKmh > t.MaxSpeedKmh && p.SpeedKmh <= cfg.MaxPlausibleSpeedKm {
			t.MaxSpeedKmh = p.SpeedKmh
		}
		if i == 0 {
			continue
		}
		q := track[i-1]
		d := Haversine(q.Lat, q.Lon, p.Lat, p.Lon)
		dt := p.TS.Sub(q.TS).Seconds()
		if dt <= 0 || d/dt*3.6 > cfg.MaxPlausibleSpeedKm {
			continue // a glitch, not a 300 km/h jump
		}
		t.DistanceKm += d / 1000
	}
	if t.DurationS > 0 {
		t.AvgSpeedKmh = t.DistanceKm / (t.DurationS / 3600)
	}
	return t
}

// stitch merges the GPS of a trip's segments into one track ordered by time,
// taking each moment from the front camera when it has points, else the
// other camera of the pair.
func stitch(g []store.Segment, gps map[int64][]store.GPSPoint) []store.GPSPoint {
	byPair := map[string][]store.Segment{}
	var keys []string
	for _, s := range g {
		if _, ok := byPair[s.PairKey]; !ok {
			keys = append(keys, s.PairKey)
		}
		byPair[s.PairKey] = append(byPair[s.PairKey], s)
	}
	rank := map[string]int{"front": 0, "rear": 1, "interior": 2}
	var out []store.GPSPoint
	for _, k := range keys {
		ss := byPair[k]
		sort.Slice(ss, func(i, j int) bool { return rank[ss[i].Camera] < rank[ss[j].Camera] })
		for _, s := range ss {
			if pts := gps[s.ID]; len(pts) > 0 {
				out = append(out, pts...)
				break
			}
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].TS.Before(out[j].TS) })
	return out
}

// reuseID picks the existing trip id most of the group's segments had.
func reuseID(g []store.Segment, claimed map[int64]bool) int64 {
	votes := map[int64]int{}
	for _, s := range g {
		if s.TripID != 0 && !claimed[s.TripID] {
			votes[s.TripID]++
		}
	}
	var best int64
	for id, n := range votes {
		if best == 0 || n > votes[best] || (n == votes[best] && id < best) {
			best = id
		}
	}
	if best != 0 {
		claimed[best] = true
	}
	return best
}

// Track returns a trip's stitched GPS track (real and interpolated points).
func Track(ctx context.Context, db *store.DB, tripID int64) ([]store.GPSPoint, []store.Segment, error) {
	segs, err := db.Segments(ctx, tripID)
	if err != nil {
		return nil, nil, err
	}
	gps := map[int64][]store.GPSPoint{}
	for _, s := range segs {
		if gps[s.ID], err = db.GPS(ctx, s.ID); err != nil {
			return nil, nil, err
		}
	}
	return stitch(segs, gps), segs, nil
}
