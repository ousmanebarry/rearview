package prefilter

import (
	"testing"
	"time"

	"rearview/internal/config"
	"rearview/internal/store"
)

var t0 = time.Date(2026, 10, 23, 17, 0, 0, 0, time.UTC)

// cfg pins the thresholds the tables below were written for.
func cfg() config.Motion {
	c := config.Default("/h").Motion
	c.HardBrakeDropKmh = 14
	return c
}

// trace builds 1 Hz points from speeds and headings.
func trace(speeds, headings []float64) []store.GPSPoint {
	var out []store.GPSPoint
	for i, v := range speeds {
		h := 0.0
		if headings != nil {
			h = headings[i]
		}
		out = append(out, store.GPSPoint{TS: t0.Add(time.Duration(i) * time.Second), TOffsetS: float64(i), SpeedKmh: v, HeadingDeg: h})
	}
	return out
}

func constant(n int, v float64) []float64 {
	out := make([]float64, n)
	for i := range out {
		out[i] = v
	}
	return out
}

func TestDetectors(t *testing.T) {
	tests := []struct {
		name      string
		speeds    []float64
		headings  []float64
		wantTypes []string
		wantScore []float64
	}{
		{"steady", constant(10, 60), nil, nil, nil},
		{"gentle stop", []float64{60, 52, 44, 36, 28, 20, 12, 4, 0}, nil, nil, nil},
		{"just below threshold", []float64{60, 46.1, 40}, nil, nil, nil},
		{"at threshold", []float64{60, 46, 40}, nil, []string{"hard_brake"}, []float64{0.5}},
		{"double threshold", []float64{60, 32, 30}, nil, []string{"hard_brake"}, []float64{1}},
		{"swerve", constant(5, 50), []float64{90, 90, 125, 125, 125}, []string{"swerve"}, []float64{35.0 / 60}},
		{"swerve too slow", constant(5, 20), []float64{90, 90, 150, 150, 150}, nil, nil},
		{"wraparound is 20 not 340", constant(4, 50), []float64{350, 350, 10, 10}, nil, nil},
		{"wraparound swerve", constant(4, 50), []float64{345, 345, 25, 25}, []string{"swerve"}, []float64{40.0 / 60}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := Detect(trace(tc.speeds, tc.headings), cfg())
			if len(got) != len(tc.wantTypes) {
				t.Fatalf("got %+v, want types %v", got, tc.wantTypes)
			}
			for i, d := range got {
				if d.Type != tc.wantTypes[i] || abs(d.Score-tc.wantScore[i]) > 1e-9 {
					t.Errorf("detection %d: %s %.3f, want %s %.3f", i, d.Type, d.Score, tc.wantTypes[i], tc.wantScore[i])
				}
			}
		})
	}
}

func abs(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}

func TestMergeCloseDetections(t *testing.T) {
	// Two drops 2 s apart merge; a third 6 s later stays separate.
	speeds := []float64{80, 60, 60, 40, 40, 40, 40, 40, 40, 20, 20}
	got := Detect(trace(speeds, nil), cfg())
	if len(got) != 2 {
		t.Fatalf("got %d events: %+v", len(got), got)
	}
	m := got[0]
	if !m.Start.Equal(t0) || !m.End.Equal(t0.Add(3*time.Second)) {
		t.Errorf("merged span %v-%v", m.Start, m.End)
	}
	if !got[1].Start.Equal(t0.Add(8 * time.Second)) {
		t.Errorf("second event starts %v", got[1].Start)
	}
}

func TestMergeKeepsWorstPeak(t *testing.T) {
	speeds := []float64{80, 66, 66, 36}
	got := Detect(trace(speeds, nil), cfg())
	if len(got) != 1 || got[0].Value != 30 || !got[0].Peak.Equal(t0.Add(3*time.Second)) {
		t.Errorf("got %+v", got)
	}
}

func TestIgnoresInterpolatedPoints(t *testing.T) {
	pts := trace([]float64{60, 30, 60}, nil)
	pts[1].Interpolated = true
	if got := Detect(pts, cfg()); len(got) != 0 {
		t.Errorf("detected on an interpolated point: %+v", got)
	}
}

func TestGapLongerThanWindowIsNotABrake(t *testing.T) {
	pts := trace([]float64{60, 30}, nil)
	pts[1].TS = t0.Add(3 * time.Second) // 30 km/h lost over 3 s is ordinary braking
	if got := Detect(pts, cfg()); len(got) != 0 {
		t.Errorf("got %+v", got)
	}
}

func TestReceiverDropoutIsNotABrake(t *testing.T) {
	// 130 km/h north, then two records reading 0 km/h / 0° while still moving.
	speeds := []float64{130, 130, 0, 0, 130, 130}
	heads := []float64{62, 62, 0, 0, 62, 62}
	pts := trace(speeds, heads)
	for i := range pts {
		pts[i].Lat = 45 + float64(i)*130/3.6/111195
	}
	if got := Detect(pts, cfg()); len(got) != 0 {
		t.Errorf("dropout detected as %+v", got)
	}
	// Heading-only dropout: speed nearly fine, heading 0 for one record.
	pts = trace([]float64{60, 60, 5.5, 60, 60}, []float64{59, 59, 0, 59, 59})
	if got := Detect(pts, cfg()); len(got) != 0 {
		t.Errorf("heading dropout detected as %+v", got)
	}
	// A real stop reading 0/0 that stays at 0 still counts.
	stop := trace([]float64{20, 0, 0}, []float64{0, 0, 0})
	if got := Detect(stop, cfg()); len(got) != 1 {
		t.Errorf("real stop: %+v", got)
	}
}
