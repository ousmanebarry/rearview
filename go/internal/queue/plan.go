package queue

import (
	"context"
	"math"
	"sort"

	"rearview/internal/config"
	"rearview/internal/prefilter"
	"rearview/internal/store"
)

// Priorities from the plan (task 4.4).
const (
	PriorityLocked = 0 // files the driver locked (RO folder)
	PriorityEvent  = 1 // windows around motion events
	PriorityRest   = 2 // everything else
)

// minWindowS is the shortest leftover stretch worth its own window.
const minWindowS = 1.0

// Plan creates analyze jobs for every segment that has none yet: the whole
// file at priority 0 for locked files; otherwise the motion-event windows at
// priority 1 and the rest of the file at priority 2 (skipped with
// onlyWindows). Parking footage is skipped. It returns the jobs added.
func Plan(ctx context.Context, db *store.DB, cfg config.Config, onlyWindows bool) (int, error) {
	segs, err := db.Segments(ctx, 0)
	if err != nil {
		return 0, err
	}
	planned, err := db.SegmentsWithJobs(ctx, "analyze")
	if err != nil {
		return 0, err
	}
	evs, err := db.Events(ctx, store.EventFilter{Source: "motion"})
	if err != nil {
		return 0, err
	}
	byTrip := map[int64][]store.Segment{}
	for _, s := range segs {
		byTrip[s.TripID] = append(byTrip[s.TripID], s)
	}
	windows := map[int64][]prefilter.Window{}
	for _, e := range evs {
		for _, w := range prefilter.EventWindows(e, byTrip[e.TripID], cfg.Motion.WindowBeforeS, cfg.Motion.WindowAfterS) {
			windows[w.SegmentID] = append(windows[w.SegmentID], w)
		}
	}
	added := 0
	for _, s := range segs {
		if planned[s.ID] || s.Parking || s.DurationS <= 0 {
			continue
		}
		var jobs []store.Job
		switch {
		case s.Locked:
			jobs = append(jobs, store.Job{SegmentID: s.ID, Kind: "analyze", Priority: PriorityLocked})
		default:
			ev := prefilter.MergeWindows(windows[s.ID])
			if len(ev) > 0 {
				jobs = append(jobs, store.Job{SegmentID: s.ID, Kind: "analyze", Priority: PriorityEvent, Windows: spans(ev)})
			}
			if !onlyWindows {
				rest := Complement(spans(ev), s.DurationS)
				if len(rest) > 0 {
					if len(ev) == 0 {
						rest = nil // the whole file
					}
					jobs = append(jobs, store.Job{SegmentID: s.ID, Kind: "analyze", Priority: PriorityRest, Windows: rest})
				}
			}
		}
		for _, j := range jobs {
			ok, err := db.InsertJob(ctx, j)
			if err != nil {
				return added, err
			}
			if ok {
				added++
			}
		}
	}
	return added, nil
}

// PlanEmbed creates one embed_frames job per segment that has none: front
// files at priority 0, the other cameras after. Parking footage is skipped.
// camera, when set, limits planning to that camera.
func PlanEmbed(ctx context.Context, db *store.DB, camera string) (int, error) {
	segs, err := db.Segments(ctx, 0)
	if err != nil {
		return 0, err
	}
	planned, err := db.SegmentsWithJobs(ctx, "embed_frames")
	if err != nil {
		return 0, err
	}
	added := 0
	for _, s := range segs {
		if planned[s.ID] || s.Parking || s.DurationS <= 0 || (camera != "" && s.Camera != camera) {
			continue
		}
		pri := 0
		if s.Camera != "front" {
			pri = 1
		}
		ok, err := db.InsertJob(ctx, store.Job{SegmentID: s.ID, Kind: "embed_frames", Priority: pri})
		if err != nil {
			return added, err
		}
		if ok {
			added++
		}
	}
	return added, nil
}

func spans(ws []prefilter.Window) [][2]float64 {
	out := make([][2]float64, len(ws))
	for i, w := range ws {
		out[i] = [2]float64{round3(w.StartS), round3(w.EndS)}
	}
	return out
}

func round3(v float64) float64 { return math.Round(v*1000) / 1000 }

// Complement returns the parts of [0, duration] not covered by ws, dropping
// slivers shorter than a second.
func Complement(ws [][2]float64, duration float64) [][2]float64 {
	sorted := append([][2]float64(nil), ws...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i][0] < sorted[j][0] })
	var out [][2]float64
	at := 0.0
	for _, w := range sorted {
		if w[0]-at >= minWindowS {
			out = append(out, [2]float64{at, w[0]})
		}
		at = math.Max(at, w[1])
	}
	if duration-at >= minWindowS {
		out = append(out, [2]float64{at, round3(duration)})
	}
	return out
}
