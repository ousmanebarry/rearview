package prefilter

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

// MotionTypes are the event types this package owns.
var MotionTypes = []string{"hard_brake", "swerve", "g_spike"}

// Rebuild recomputes every motion event from the trips' GPS tracks and
// returns how many it stored.
func Rebuild(ctx context.Context, db *store.DB, cfg config.Motion) (int, error) {
	ts, err := db.Trips(ctx)
	if err != nil {
		return 0, err
	}
	var evs []store.Event
	for _, t := range ts {
		track, segs, err := trips.Track(ctx, db, t.ID)
		if err != nil {
			return 0, fmt.Errorf("track of trip %d: %w", t.ID, err)
		}
		bySeg := map[int64]store.Segment{}
		for _, s := range segs {
			bySeg[s.ID] = s
		}
		for _, d := range Detect(track, cfg) {
			seg, ok := segmentAt(track, d.Peak, bySeg)
			if !ok {
				continue
			}
			details, _ := json.Marshal(map[string]float64{"value": round2(d.Value), "speed_kmh": round2(d.SpeedKmh)})
			evs = append(evs, store.Event{
				TripID:    t.ID,
				SegmentID: seg.ID,
				Type:      d.Type,
				Source:    "motion",
				TS:        d.Peak,
				TStartS:   d.Start.Sub(seg.StartedAt).Seconds(),
				TEndS:     d.End.Sub(seg.StartedAt).Seconds(),
				Score:     d.Score,
				Lat:       d.Lat,
				Lon:       d.Lon,
				HasPos:    true,
				Details:   string(details),
			})
		}
	}
	if _, err := db.ReplaceEvents(ctx, "motion", MotionTypes, evs); err != nil {
		return 0, err
	}
	return len(evs), nil
}

func round2(v float64) float64 { return math.Round(v*100) / 100 }

// segmentAt finds the segment whose point sits at time t.
func segmentAt(track []store.GPSPoint, t time.Time, segs map[int64]store.Segment) (store.Segment, bool) {
	i := sort.Search(len(track), func(i int) bool { return !track[i].TS.Before(t) })
	if i == len(track) {
		return store.Segment{}, false
	}
	s, ok := segs[track[i].SegmentID]
	return s, ok
}

// Window is a stretch of one file to analyse, in seconds from its start.
type Window struct {
	SegmentID int64
	Camera    string
	StartS    float64
	EndS      float64
}

// Windows maps the span [from, to] onto the segments that recorded it, one
// window per file and camera. A span crossing a file boundary yields one
// window in each file.
func Windows(from, to time.Time, segs []store.Segment) []Window {
	var out []Window
	for _, s := range segs {
		if s.Parking || s.DurationS <= 0 {
			continue
		}
		a := math.Max(0, from.Sub(s.StartedAt).Seconds())
		b := math.Min(s.DurationS, to.Sub(s.StartedAt).Seconds())
		if b-a > 1e-6 {
			out = append(out, Window{SegmentID: s.ID, Camera: s.Camera, StartS: a, EndS: b})
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Camera != out[j].Camera {
			return out[i].Camera < out[j].Camera
		}
		return out[i].SegmentID < out[j].SegmentID
	})
	return out
}

// EventWindows returns the analysis windows around an event's peak:
// before seconds earlier to after seconds later, on every camera.
func EventWindows(e store.Event, segs []store.Segment, before, after float64) []Window {
	return Windows(e.TS.Add(-secs(before)), e.TS.Add(secs(after)), segs)
}

func secs(s float64) time.Duration { return time.Duration(s * float64(time.Second)) }

// MergeWindows joins overlapping or touching windows of the same segment.
func MergeWindows(ws []Window) []Window {
	sort.SliceStable(ws, func(i, j int) bool {
		if ws[i].SegmentID != ws[j].SegmentID {
			return ws[i].SegmentID < ws[j].SegmentID
		}
		return ws[i].StartS < ws[j].StartS
	})
	var out []Window
	for _, w := range ws {
		if n := len(out); n > 0 && out[n-1].SegmentID == w.SegmentID && w.StartS <= out[n-1].EndS {
			out[n-1].EndS = math.Max(out[n-1].EndS, w.EndS)
			continue
		}
		out = append(out, w)
	}
	return out
}
