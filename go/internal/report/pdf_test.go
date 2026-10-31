package report

import (
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"path/filepath"
	"testing"
	"time"

	"rearview/internal/store"
)

func TestGraphDerivesG(t *testing.T) {
	var track []store.GPSPoint
	for i := -20; i <= 20; i++ {
		v := 60.0
		if i >= 0 {
			v = 60 - 10*float64(min(i, 3)) // 10 km/h per second for 3 s
		}
		track = append(track, store.GPSPoint{TS: t0.Add(time.Duration(i) * time.Second), SpeedKmh: v})
	}
	g := Graph(track, t0, 15)
	if len(g) != 31 || g[0].T != -15 || g[30].T != 15 {
		t.Fatalf("got %d points from %v to %v", len(g), g[0].T, g[len(g)-1].T)
	}
	// 10 km/h in 1 s = 2.78 m/s² = 0.283 g of braking.
	if g[16].AccelG > -0.28 || g[16].AccelG < -0.29 {
		t.Errorf("g at +1 s = %v", g[16].AccelG)
	}
	if g[5].AccelG != 0 {
		t.Errorf("steady speed g = %v", g[5].AccelG)
	}
}

func TestWritePDF(t *testing.T) {
	dir := t.TempDir()
	img := image.NewRGBA(image.Rect(0, 0, 160, 90))
	for x := 0; x < 160; x++ {
		img.Set(x, 45, color.White)
	}
	jp := filepath.Join(dir, "f.jpg")
	f, _ := os.Create(jp)
	jpeg.Encode(f, img, nil)
	f.Close()
	var track []store.GPSPoint
	for i := -20; i <= 20; i++ {
		track = append(track, store.GPSPoint{TS: t0.Add(time.Duration(i) * time.Second), SpeedKmh: 50 + float64(i)})
	}
	in := Incident{
		Event:   store.Event{ID: 7, Type: "hard_brake", Source: "motion", TS: t0, Score: 0.8, Lat: 45.3, Lon: -75.9, HasPos: true, Details: `{"value":14}`},
		Segment: store.Segment{SourceName: "2026_1023_130515_000587F.MP4", StartedAt: t0.Add(-60 * time.Second)},
		Loc:     time.UTC,
		Frames:  []Frame{{jp, 58}, {jp, 60}, {jp, 62}},
		Track:   track,
		Version: "test",
	}
	out := filepath.Join(dir, "r.pdf")
	if err := WritePDF(out, in); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(out)
	if len(b) < 2000 || string(b[:5]) != "%PDF-" {
		t.Errorf("not a PDF (%d bytes)", len(b))
	}
}
