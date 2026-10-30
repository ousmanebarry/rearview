package trips

import (
	"context"
	"math"
	"path/filepath"
	"testing"
	"time"

	"rearview/internal/config"
	"rearview/internal/store"
)

var t0 = time.Date(2026, 10, 23, 17, 0, 0, 0, time.UTC)

func seg(id int64, cam string, start time.Duration, durS float64) store.Segment {
	return store.Segment{ID: id, Camera: cam, StartedAt: t0.Add(start), DurationS: durS}
}

// line makes n real points one second apart heading north at speedKmh,
// with GPS time = file time + offsetS.
func line(segID int64, s store.Segment, n int, offsetS float64, lat0, speedKmh float64) []store.GPSPoint {
	var out []store.GPSPoint
	degPerS := speedKmh / 3.6 / 111195.0
	for i := range n {
		out = append(out, store.GPSPoint{
			SegmentID: segID, TOffsetS: float64(i),
			TS:  s.StartedAt.Add(time.Duration((float64(i) + offsetS) * float64(time.Second))),
			Lat: lat0 + degPerS*float64(i), Lon: -75.9, SpeedKmh: speedKmh,
		})
	}
	return out
}

func cfg() config.Trips { return config.Default("/h").Trips }

func TestGapsSplitTrips(t *testing.T) {
	segs := []store.Segment{
		seg(1, "front", 0, 180),
		seg(2, "front", 3*time.Minute, 180),
		seg(3, "front", 6*time.Minute+4*time.Minute, 180),  // 4 min after the last end: same trip
		seg(4, "front", 13*time.Minute+6*time.Minute, 180), // 6 min gap: new trip
		{ID: 5, Camera: "front", StartedAt: t0.Add(14 * time.Minute), DurationS: 60, Parking: true},
	}
	p := Assemble(segs, nil, cfg())
	if len(p.Trips) != 2 {
		t.Fatalf("%d trips, want 2", len(p.Trips))
	}
	if got := p.Trips[0].SegmentIDs; len(got) != 3 {
		t.Errorf("trip 1 segments %v", got)
	}
	if got := p.Trips[1].SegmentIDs; len(got) != 1 || got[0] != 4 {
		t.Errorf("trip 2 segments %v (parking must stay out)", got)
	}
	if want := 13 * time.Minute; p.Trips[0].EndedAt.Sub(p.Trips[0].StartedAt) != want {
		t.Errorf("trip 1 length %v, want %v", p.Trips[0].EndedAt.Sub(p.Trips[0].StartedAt), want)
	}
}

func TestPairing(t *testing.T) {
	segs := []store.Segment{
		seg(1, "front", 0, 180),
		seg(2, "rear", 1*time.Second, 180),
		seg(3, "front", 3*time.Minute, 180),
		seg(4, "rear", 3*time.Minute+3*time.Second, 180), // 3 s apart: not a pair
	}
	p := Assemble(segs, nil, cfg())
	key := map[int64]string{}
	for _, s := range p.Segments {
		key[s.ID] = s.PairKey
	}
	if key[1] != key[2] || key[1] != "20261023T170000Z" {
		t.Errorf("pair 1/2 keys %q %q", key[1], key[2])
	}
	if key[3] == key[4] || key[3] == "" || key[4] == "" {
		t.Errorf("3 and 4 should have distinct keys: %q %q", key[3], key[4])
	}
}

func TestClockCorrection(t *testing.T) {
	a := seg(1, "front", 0, 180)
	b := seg(2, "rear", 0, 180)          // no fix: borrows a's offset
	c := seg(3, "front", time.Hour, 180) // far away, own fix
	gps := map[int64][]store.GPSPoint{
		1: line(1, a, 100, 3, 45, 50), // GPS says the camera is 3 s slow
		3: line(3, c, 10, -2, 45, 0),
	}
	p := Assemble([]store.Segment{a, b, c}, gps, cfg())
	got := map[int64]store.SegmentUpdate{}
	for _, s := range p.Segments {
		got[s.ID] = s
	}
	if got[1].ClockOffsetS != 3 || !got[1].StartedAt.Equal(t0.Add(3*time.Second)) {
		t.Errorf("segment 1: %+v", got[1])
	}
	if got[2].ClockOffsetS != 3 {
		t.Errorf("segment 2 without fix: offset %v, want 3", got[2].ClockOffsetS)
	}
	if got[3].ClockOffsetS != -2 {
		t.Errorf("segment 3: offset %v, want -2", got[3].ClockOffsetS)
	}

	// Rebuilding with the corrected values must not shift again.
	a.StartedAt, a.ClockOffsetS = got[1].StartedAt, 3
	b.StartedAt, b.ClockOffsetS = got[2].StartedAt, 3
	p2 := Assemble([]store.Segment{a, b, c}, gps, cfg())
	for _, s := range p2.Segments {
		if s.ID != 3 && !s.StartedAt.Equal(t0.Add(3*time.Second)) {
			t.Errorf("second pass moved segment %d to %v", s.ID, s.StartedAt)
		}
	}
}

func TestInterpolation(t *testing.T) {
	a := seg(1, "front", 0, 180)
	pts := line(1, a, 30, 0, 45, 36)
	// Drop offsets 5..8 (4-s hole -> filled) and 15..26 (12-s hole -> left).
	var kept []store.GPSPoint
	for _, p := range pts {
		if (p.TOffsetS >= 5 && p.TOffsetS <= 8) || (p.TOffsetS >= 15 && p.TOffsetS <= 26) {
			continue
		}
		kept = append(kept, p)
	}
	kept[0].HeadingDeg, kept[1].HeadingDeg = 0, 0
	p := Assemble([]store.Segment{a}, map[int64][]store.GPSPoint{1: kept}, cfg())
	if len(p.Interpolated) != 4 {
		t.Fatalf("%d interpolated points, want 4", len(p.Interpolated))
	}
	mid := p.Interpolated[1] // offset 6
	if mid.TOffsetS != 6 || !mid.Interpolated || math.Abs(mid.Lat-pts[6].Lat) > 1e-9 {
		t.Errorf("interpolated %+v, want lat %v", mid, pts[6].Lat)
	}
}

func TestLerpAngleWraps(t *testing.T) {
	if got := lerpAngle(350, 10, 0.5); math.Abs(got) > 1e-9 && math.Abs(got-360) > 1e-9 {
		t.Errorf("lerp 350->10 at 0.5 = %v, want 0", got)
	}
	if d := HeadingDelta(359, 1); d != 2 {
		t.Errorf("delta 359->1 = %v", d)
	}
	if d := HeadingDelta(1, 359); d != -2 {
		t.Errorf("delta 1->359 = %v", d)
	}
}

func TestSummary(t *testing.T) {
	a := seg(1, "front", 0, 100)
	r := seg(2, "rear", 0, 100)
	front := line(1, a, 101, 0, 45, 72) // 72 km/h = 20 m/s for 100 s = 2 km
	rear := line(2, r, 101, 0, 46, 72)  // a different place: must be ignored
	front[50].Lat += 1                  // a one-point glitch, 111 km away
	front[50].SpeedKmh = 900
	p := Assemble([]store.Segment{a, r}, map[int64][]store.GPSPoint{1: front, 2: rear}, cfg())
	tr := p.Trips[0]
	if math.Abs(tr.DistanceKm-1.96) > 0.05 { // the two legs touching the glitch are dropped
		t.Errorf("distance %v km, want about 1.96", tr.DistanceKm)
	}
	if tr.MaxSpeedKmh != 72 || tr.DurationS != 100 {
		t.Errorf("max %v duration %v", tr.MaxSpeedKmh, tr.DurationS)
	}
	if math.Abs(tr.AvgSpeedKmh-tr.DistanceKm/(100.0/3600)) > 1e-9 {
		t.Errorf("avg %v", tr.AvgSpeedKmh)
	}
}

func TestHaversine(t *testing.T) {
	// One degree of latitude is about 111.2 km.
	if d := Haversine(45, -75, 46, -75); math.Abs(d-111195) > 50 {
		t.Errorf("1 degree = %v m", d)
	}
}

func TestRebuildKeepsIDsAndIsIdempotent(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(ctx, filepath.Join(t.TempDir(), "r.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	add := func(name, cam string, start time.Duration) int64 {
		s := seg(0, cam, start, 180)
		s.Path, s.SourceName, s.Hash, s.ImportedAt = name, name, name, t0
		id, err := db.InsertSegment(ctx, s)
		if err != nil {
			t.Fatal(err)
		}
		s.ID = id
		if err := db.ReplaceGPS(ctx, id, line(id, s, 180, 1, 45, 50)); err != nil {
			t.Fatal(err)
		}
		return id
	}
	add("a", "front", 0)
	add("b", "rear", 0)
	add("c", "front", time.Hour)
	c := cfg()
	if n, err := Rebuild(ctx, db, c); err != nil || n != 2 {
		t.Fatalf("n=%d err=%v", n, err)
	}
	first, _ := db.Trips(ctx)

	add("d", "front", 2*time.Hour)
	if n, err := Rebuild(ctx, db, c); err != nil || n != 3 {
		t.Fatalf("n=%d err=%v", n, err)
	}
	second, _ := db.Trips(ctx)
	if second[0].ID != first[0].ID || second[1].ID != first[1].ID {
		t.Errorf("trip ids changed: %v -> %v", first, second)
	}
	if !second[0].StartedAt.Equal(t0.Add(time.Second)) {
		t.Errorf("trip start %v, want GPS-corrected %v", second[0].StartedAt, t0.Add(time.Second))
	}
	segs, _ := db.Segments(ctx, second[0].ID)
	if len(segs) != 2 || segs[0].PairKey != segs[1].PairKey {
		t.Errorf("trip 1 segments %+v", segs)
	}
	track, _, err := Track(ctx, db, second[0].ID)
	if err != nil || len(track) != 180 {
		t.Errorf("track has %d points (%v), want 180 from one camera", len(track), err)
	}
}
