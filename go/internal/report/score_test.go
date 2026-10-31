package report

import (
	"math"
	"testing"
	"time"

	"rearview/internal/config"
	"rearview/internal/store"
)

var t0 = time.Date(2026, 10, 23, 17, 0, 0, 0, time.UTC)

func ev(id int64, typ string, dur float64) store.Event {
	return store.Event{ID: id, Type: typ, TS: t0.Add(time.Duration(id) * time.Minute), TStartS: 10, TEndS: 10 + dur, Details: "{}"}
}

func TestBreakdownAddsUp(t *testing.T) {
	cfg := config.Default("/h").Score
	tests := []struct {
		name string
		km   float64
		evs  []store.Event
		want float64
	}{
		{"clean 50 km", 50, nil, 100},
		// 100 km: no scaling. 4 + 3 + 10 + 2*1.5 min = 20.
		{"mixed 100 km", 100, []store.Event{ev(1, "hard_brake", 1), ev(2, "swerve", 1), ev(3, "near_miss", 1), ev(4, "following_close", 90)}, 80},
		// 50 km: points doubled.
		{"one brake 50 km", 50, []store.Event{ev(1, "hard_brake", 1)}, 92},
		// 1 km counts as 5 km: 4 * 20 = 80 per brake; two brakes floor at 0.
		{"short trip floor", 1, []store.Event{ev(1, "hard_brake", 1), ev(2, "hard_brake", 1)}, 0},
		// Tailgated costs nothing by default (not the driver's doing).
		{"tailgated free", 100, []store.Event{ev(1, "tailgated", 5)}, 100},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := Compute(store.Trip{ID: 1, DistanceKm: tc.km}, tc.evs, cfg)
			if math.Abs(s.Value-tc.want) > 1e-9 {
				t.Errorf("score %v, want %v (%+v)", s.Value, tc.want, s.Lines)
			}
			if math.Abs(100-s.Lost()-s.Value) > 1e-9 {
				t.Errorf("breakdown %v does not add up to %v", s.Lost(), s.Value)
			}
		})
	}
}

func TestFollowingUsesDetailsDuration(t *testing.T) {
	e := ev(1, "following_close", 1)
	e.Details = `{"duration_s": 120}`
	s := Compute(store.Trip{DistanceKm: 100}, []store.Event{e}, config.Default("/h").Score)
	if s.Value != 96 {
		t.Errorf("score %v, want 96 (2 min x 2 points)", s.Value)
	}
}

func TestWeeklyAndSpark(t *testing.T) {
	loc := time.UTC
	ts := []store.Trip{
		{ID: 1, StartedAt: t0, DistanceKm: 10},
		{ID: 2, StartedAt: t0.Add(24 * time.Hour), DistanceKm: 30},
		{ID: 3, StartedAt: t0.Add(8 * 24 * time.Hour), DistanceKm: 5},
	}
	w := Weekly(ts, map[int64]float64{1: 100, 2: 60, 3: 90}, loc)
	if len(w) != 2 || w[0].Avg != 70 || w[0].Trips != 2 || w[1].Avg != 90 {
		t.Errorf("weeks %+v", w)
	}
	if s := Spark([]float64{0, 50, 100}); s != "▁▅█" {
		t.Errorf("spark %q", s)
	}
}
