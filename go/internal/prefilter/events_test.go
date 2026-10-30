package prefilter

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"rearview/internal/config"
	"rearview/internal/store"
	"rearview/internal/trips"
)

func s(id int64, cam string, start time.Duration, dur float64) store.Segment {
	return store.Segment{ID: id, Camera: cam, StartedAt: t0.Add(start), DurationS: dur}
}

func TestWindowNearFileBoundary(t *testing.T) {
	segs := []store.Segment{
		s(1, "front", 0, 180), s(2, "rear", 0, 180),
		s(3, "front", 180*time.Second, 180), s(4, "rear", 180*time.Second, 180),
	}
	// Event 2 s before the boundary: 10 s before to 5 s after spans both files.
	e := store.Event{TS: t0.Add(178 * time.Second)}
	got := EventWindows(e, segs, 10, 5)
	want := []Window{
		{1, "front", 168, 180}, {3, "front", 0, 3},
		{2, "rear", 168, 180}, {4, "rear", 0, 3},
	}
	if d := cmp.Diff(want, got); d != "" {
		t.Errorf("windows (-want +got):\n%s", d)
	}
}

func TestWindowClampsToFileStart(t *testing.T) {
	got := EventWindows(store.Event{TS: t0.Add(4 * time.Second)}, []store.Segment{s(1, "front", 0, 180)}, 10, 5)
	if d := cmp.Diff([]Window{{1, "front", 0, 9}}, got); d != "" {
		t.Error(d)
	}
}

func TestWindowsSkipParking(t *testing.T) {
	p := s(1, "front", 0, 180)
	p.Parking = true
	if got := Windows(t0, t0.Add(time.Minute), []store.Segment{p}); len(got) != 0 {
		t.Errorf("got %+v", got)
	}
}

func TestMergeWindows(t *testing.T) {
	got := MergeWindows([]Window{{1, "front", 20, 30}, {1, "front", 0, 10}, {1, "front", 8, 15}, {2, "front", 0, 5}})
	want := []Window{{1, "front", 0, 15}, {1, "front", 20, 30}, {2, "front", 0, 5}}
	if d := cmp.Diff(want, got); d != "" {
		t.Error(d)
	}
}

func TestRebuildStoresEvents(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(ctx, filepath.Join(t.TempDir(), "r.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	seg := s(0, "front", 0, 20)
	seg.Path, seg.SourceName, seg.Hash, seg.ImportedAt = "a", "a", "a", t0
	id, _ := db.InsertSegment(ctx, seg)
	speeds := []float64{60, 60, 60, 60, 60, 30, 30, 30, 30, 30}
	pts := trace(speeds, nil)
	for i := range pts {
		pts[i].SegmentID, pts[i].Lat, pts[i].Lon = id, 45, -75
	}
	db.ReplaceGPS(ctx, id, pts)
	cfg := config.Default("/h")
	if _, err := trips.Rebuild(ctx, db, cfg.Trips); err != nil {
		t.Fatal(err)
	}
	for range 2 { // twice: no duplicates
		if n, err := Rebuild(ctx, db, cfg.Motion); err != nil || n != 1 {
			t.Fatalf("n=%d err=%v", n, err)
		}
	}
	evs, _ := db.Events(ctx, store.EventFilter{})
	if len(evs) != 1 {
		t.Fatalf("%d events", len(evs))
	}
	e := evs[0]
	if e.Type != "hard_brake" || e.Source != "motion" || e.TStartS != 4 || e.TEndS != 5 || e.TripID == 0 || e.Score != 1 {
		t.Errorf("event %+v", e)
	}
}
