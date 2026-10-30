package fusion

import (
	"encoding/json"
	"math"
	"testing"
	"time"

	"rearview/internal/config"
	"rearview/internal/store"
)

var t0 = time.Date(2026, 10, 23, 17, 0, 0, 0, time.UTC)

func cfg() Config {
	c := config.Default("/h")
	return Config{NearMiss: c.NearMiss, Following: c.Following}
}

func seg(id int64, cam string) store.Segment {
	return store.Segment{ID: id, TripID: 1, Camera: cam, StartedAt: t0, DurationS: 180, Width: 2560, Height: 1440}
}

// inPathBox is centred low in the frame, inside the default trapezoid.
func o(t float64, track int, ttc, dist *float64) store.Observation {
	return store.Observation{TOffsetS: t, TrackID: track, Cls: "car", X1: 1180, Y1: 1000, X2: 1380, Y2: 1200, Conf: 0.9, TTCS: ttc, DistM: dist}
}

func f(v float64) *float64 { return &v }

// seq makes observations at 10 fps from t with the given TTCs (nil = not approaching).
func seq(track int, from float64, ttcs ...float64) []store.Observation {
	var out []store.Observation
	for i, v := range ttcs {
		var p *float64
		if !math.IsNaN(v) {
			p = f(v)
		}
		out = append(out, o(from+0.1*float64(i), track, p, f(10)))
	}
	return out
}

func steady(kmh float64, seconds int) []store.GPSPoint {
	var out []store.GPSPoint
	for i := 0; i <= seconds; i++ {
		out = append(out, store.GPSPoint{TS: t0.Add(time.Duration(i) * time.Second), Lat: 45, Lon: -75, SpeedKmh: kmh})
	}
	return out
}

var nan = math.NaN()

func TestNearMissRules(t *testing.T) {
	tests := []struct {
		name   string
		obs    []store.Observation
		brakes []time.Time
		want   int
		score  float64
		source string
	}{
		{"three frames under 1.5 s", seq(1, 10, 2.0, 1.4, 1.2, 1.0, 2.0), nil, 1, 0.5 + 0.5*0.5/1.5, "vision"},
		{"only two frames", seq(1, 10, 2.0, 1.2, 1.0, 2.0), nil, 0, 0, ""},
		{"interrupted run", seq(1, 10, 1.2, 1.1, nan, 1.0, 1.0), nil, 0, 0, ""},
		{"never under threshold", seq(1, 10, 2.0, 1.6, 1.5, 1.7), nil, 0, 0, ""},
		{"hard brake within 2 s boosts", seq(1, 10, 1.2, 1.2, 1.2), []time.Time{t0.Add(12 * time.Second)}, 1, 0.5 + 0.5*0.3/1.5 + 0.25, "fused"},
		{"hard brake 5 s later does not", seq(1, 10, 1.2, 1.2, 1.2), []time.Time{t0.Add(15500 * time.Millisecond)}, 1, 0.5 + 0.5*0.3/1.5, "vision"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			in := Input{Segments: []store.Segment{seg(1, "front")}, Obs: map[int64][]store.Observation{1: tc.obs},
				Track: steady(30, 60), Brakes: tc.brakes}
			var got []store.Event
			for _, e := range Detect(in, cfg()) {
				if e.Type == "near_miss" {
					got = append(got, e)
				}
			}
			if len(got) != tc.want {
				t.Fatalf("got %d near-misses: %+v", len(got), got)
			}
			if tc.want == 1 {
				if math.Abs(got[0].Score-tc.score) > 1e-9 || got[0].Source != tc.source {
					t.Errorf("score %.3f source %s, want %.3f %s", got[0].Score, got[0].Source, tc.score, tc.source)
				}
				var d map[string]any
				json.Unmarshal([]byte(got[0].Details), &d)
				if d["min_ttc_s"] == nil || d["cls"] != "car" || d["track_id"] != float64(1) {
					t.Errorf("details %s", got[0].Details)
				}
			}
		})
	}
}

func TestOutsidePathIgnored(t *testing.T) {
	obs := seq(1, 10, 1.0, 1.0, 1.0, 1.0)
	for i := range obs {
		obs[i].X1, obs[i].X2 = 100, 300 // far left
	}
	in := Input{Segments: []store.Segment{seg(1, "front")}, Obs: map[int64][]store.Observation{1: obs}, Track: steady(30, 60)}
	if got := Detect(in, cfg()); len(got) != 0 {
		t.Errorf("got %+v", got)
	}
}

func TestMergesAcrossTracksAndCameras(t *testing.T) {
	front := append(seq(1, 10, 1.0, 1.0, 1.0), seq(2, 11, 0.8, 0.8, 0.8)...)
	rear := seq(5, 12, 1.1, 1.1, 1.1)
	far := seq(3, 30, 1.0, 1.0, 1.0)
	in := Input{
		Segments: []store.Segment{seg(1, "front"), seg(2, "rear")},
		Obs:      map[int64][]store.Observation{1: append(front, far...), 2: rear},
		Track:    steady(20, 60),
	}
	got := Detect(in, cfg())
	if len(got) != 2 {
		t.Fatalf("got %d events, want 2: %+v", len(got), got)
	}
	var d map[string]any
	json.Unmarshal([]byte(got[0].Details), &d)
	if d["min_ttc_s"] != 0.8 || d["track_id"] != float64(2) {
		t.Errorf("merged event should keep the worst moment: %s", got[0].Details)
	}
}

func TestFollowingClose(t *testing.T) {
	// 72 km/h = 20 m/s; 15 m ahead = 0.75 s following time.
	var obs []store.Observation
	for i := 0; i < 40; i++ { // 3.9 s
		obs = append(obs, o(10+0.1*float64(i), 1, nil, f(15)))
	}
	short := []store.Observation{}
	for i := 0; i < 20; i++ { // 1.9 s only
		short = append(short, o(40+0.1*float64(i), 2, nil, f(15)))
	}
	in := Input{Segments: []store.Segment{seg(1, "front")}, Obs: map[int64][]store.Observation{1: append(obs, short...)}, Track: steady(72, 60)}
	got := Detect(in, cfg())
	if len(got) != 1 || got[0].Type != "following_close" {
		t.Fatalf("got %+v", got)
	}
	if math.Abs(got[0].Score-(0.5+0.5*0.25)) > 1e-9 || got[0].TStartS != 10 {
		t.Errorf("event %+v", got[0])
	}

	// Below 50 km/h the same gap is fine.
	in.Track = steady(40, 60)
	if got := Detect(in, cfg()); len(got) != 0 {
		t.Errorf("slow: %+v", got)
	}

	// The same rule on the rear camera is "tailgated".
	in = Input{Segments: []store.Segment{seg(2, "rear")}, Obs: map[int64][]store.Observation{2: obs}, Track: steady(72, 60)}
	if got := Detect(in, cfg()); len(got) != 1 || got[0].Type != "tailgated" || got[0].Camera != "rear" {
		t.Errorf("rear: %+v", got)
	}
}

func TestSpeedAt(t *testing.T) {
	tr := []store.GPSPoint{{TS: t0, SpeedKmh: 10}, {TS: t0.Add(time.Second), SpeedKmh: 20}, {TS: t0.Add(30 * time.Second), SpeedKmh: 50}}
	if v, ok := SpeedAt(tr, t0.Add(500*time.Millisecond)); !ok || v != 15 {
		t.Errorf("v=%v ok=%v", v, ok)
	}
	if _, ok := SpeedAt(tr, t0.Add(10*time.Second)); ok {
		t.Error("interpolated across a 29 s gap")
	}
	if _, ok := SpeedAt(tr, t0.Add(-time.Second)); ok {
		t.Error("before the track")
	}
}

func TestInPath(t *testing.T) {
	trap := config.Default("/h").NearMiss.PathTrapezoid
	if !InPath(o(0, 1, nil, nil), trap, 2560, 1440) {
		t.Error("centre box not in path")
	}
	if InPath(store.Observation{X1: 0, X2: 100, Y2: 1300}, trap, 2560, 1440) {
		t.Error("left box in path")
	}
}
