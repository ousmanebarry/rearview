// Package report computes per-trip driving scores and writes incident PDFs.
package report

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"time"

	"rearview/internal/config"
	"rearview/internal/store"
)

// Line is one entry of a score breakdown.
type Line struct {
	EventID int64
	Type    string
	At      time.Time
	Points  float64 // points lost, already scaled per 100 km
	Note    string
}

// Score is a trip's score and every point lost.
type Score struct {
	TripID     int64
	DistanceKm float64
	Value      float64
	Lines      []Line
}

// Lost is the total of the breakdown.
func (s Score) Lost() float64 {
	var t float64
	for _, l := range s.Lines {
		t += l.Points
	}
	return t
}

// Compute scores one trip: 100 minus points per event, scaled to 100 km so
// long and short drives compare (distances under min_distance_km count as
// that minimum, so a 1 km trip isn't punished 100-fold). The score never goes
// below 0; when it would, a final "floor" line gives points back so the
// breakdown still adds up exactly.
func Compute(t store.Trip, evs []store.Event, cfg config.Score) Score {
	s := Score{TripID: t.ID, DistanceKm: t.DistanceKm}
	scale := cfg.PerKm / math.Max(t.DistanceKm, cfg.MinDistanceKm)
	sort.Slice(evs, func(i, j int) bool { return evs[i].TS.Before(evs[j].TS) })
	for _, e := range evs {
		var base float64
		note := ""
		switch e.Type {
		case "hard_brake":
			base = cfg.HardBrake
		case "swerve":
			base = cfg.Swerve
		case "near_miss":
			base = cfg.NearMiss
		case "following_close":
			mins := (e.TEndS - e.TStartS) / 60
			var d struct {
				DurationS float64 `json:"duration_s"`
			}
			if json.Unmarshal([]byte(e.Details), &d) == nil && d.DurationS > 0 {
				mins = d.DurationS / 60
			}
			base = cfg.FollowingMin * mins
			note = fmt.Sprintf("%.1f min", mins)
		case "tailgated":
			base = cfg.Tailgated
		}
		if base == 0 {
			continue
		}
		s.Lines = append(s.Lines, Line{EventID: e.ID, Type: e.Type, At: e.TS, Points: round2(base * scale), Note: note})
	}
	s.Value = round2(100 - s.Lost())
	if s.Value < 0 {
		s.Lines = append(s.Lines, Line{Type: "floor", Points: s.Value, Note: "points given back: the score cannot go below 0"})
		s.Value = 0
	}
	return s
}

func round2(v float64) float64 { return math.Round(v*100) / 100 }

// ScoreAll computes and stores every trip's score.
func ScoreAll(ctx context.Context, db *store.DB, cfg config.Score) ([]Score, error) {
	ts, err := db.Trips(ctx)
	if err != nil {
		return nil, err
	}
	var out []Score
	for _, t := range ts {
		evs, err := db.Events(ctx, store.EventFilter{TripID: t.ID})
		if err != nil {
			return nil, err
		}
		s := Compute(t, evs, cfg)
		if err := db.SetTripScore(ctx, t.ID, s.Value); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, nil
}

// Week is one ISO week's average score.
type Week struct {
	Year, Week int
	Avg        float64
	Trips      int
}

// Weekly averages trip scores by ISO week (weighted by distance).
func Weekly(ts []store.Trip, scores map[int64]float64, loc *time.Location) []Week {
	type acc struct {
		sum, w float64
		n      int
	}
	m := map[[2]int]*acc{}
	for _, t := range ts {
		s, ok := scores[t.ID]
		if !ok {
			continue
		}
		y, wk := t.StartedAt.In(loc).ISOWeek()
		k := [2]int{y, wk}
		if m[k] == nil {
			m[k] = &acc{}
		}
		w := math.Max(t.DistanceKm, 0.1)
		m[k].sum += s * w
		m[k].w += w
		m[k].n++
	}
	var out []Week
	for k, a := range m {
		out = append(out, Week{k[0], k[1], round2(a.sum / a.w), a.n})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Year != out[j].Year {
			return out[i].Year < out[j].Year
		}
		return out[i].Week < out[j].Week
	})
	return out
}

// Spark renders values in 0..100 as a one-line bar chart.
func Spark(vals []float64) string {
	bars := []rune("▁▂▃▄▅▆▇█")
	out := make([]rune, len(vals))
	for i, v := range vals {
		k := int(math.Round(math.Max(0, math.Min(100, v)) / 100 * float64(len(bars)-1)))
		out[i] = bars[k]
	}
	return string(out)
}
