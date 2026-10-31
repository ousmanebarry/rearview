package queue

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"rearview/internal/config"
	"rearview/internal/store"
)

func TestPopsInPriorityOrder(t *testing.T) {
	q := New()
	q.Push(
		store.Job{ID: 1, Priority: 2}, store.Job{ID: 2, Priority: 1}, store.Job{ID: 3, Priority: 0},
		store.Job{ID: 4, Priority: 2}, store.Job{ID: 5, Priority: 0}, store.Job{ID: 6, Priority: 1},
	)
	q.Close()
	var got []int64
	for {
		j, ok := q.Pop(context.Background())
		if !ok {
			break
		}
		got = append(got, j.ID)
	}
	if d := cmp.Diff([]int64{3, 5, 2, 6, 1, 4}, got); d != "" {
		t.Error(d)
	}
}

func TestLateHighPriorityJumpsTheLine(t *testing.T) {
	q := New()
	q.Push(store.Job{ID: 1, Priority: 2}, store.Job{ID: 2, Priority: 2})
	j, _ := q.Pop(context.Background())
	q.Push(store.Job{ID: 3, Priority: 0})
	j2, _ := q.Pop(context.Background())
	if j.ID != 1 || j2.ID != 3 {
		t.Errorf("got %d then %d", j.ID, j2.ID)
	}
}

func TestPopBlocksUntilPushOrCancel(t *testing.T) {
	q := New()
	var wg sync.WaitGroup
	wg.Add(1)
	var got store.Job
	go func() {
		defer wg.Done()
		got, _ = q.Pop(context.Background())
	}()
	time.Sleep(20 * time.Millisecond)
	q.Push(store.Job{ID: 7})
	wg.Wait()
	if got.ID != 7 {
		t.Errorf("got %d", got.ID)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if _, ok := q.Pop(ctx); ok {
		t.Error("Pop on an empty queue returned a job")
	}
}

func TestComplement(t *testing.T) {
	got := Complement([][2]float64{{50, 65}, {10, 20.5}, {60, 70}}, 180)
	want := [][2]float64{{0, 10}, {20.5, 50}, {70, 180}}
	if d := cmp.Diff(want, got); d != "" {
		t.Error(d)
	}
	if got := Complement([][2]float64{{0, 179.5}}, 180); len(got) != 0 {
		t.Errorf("sliver kept: %v", got)
	}
}

func TestPlan(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(ctx, filepath.Join(t.TempDir(), "r.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	t0 := time.Date(2026, 10, 23, 17, 0, 0, 0, time.UTC)
	add := func(name string, start time.Duration, locked, parking bool) store.Segment {
		s := store.Segment{Path: name, SourceName: name, Hash: name, Camera: "front", StartedAt: t0.Add(start),
			DurationS: 180, Locked: locked, Parking: parking, ImportedAt: t0}
		s.ID, err = db.InsertSegment(ctx, s)
		if err != nil {
			t.Fatal(err)
		}
		return s
	}
	a := add("a", 0, false, false)
	b := add("b", 180*time.Second, true, false)
	add("c", 360*time.Second, false, true)
	d := add("d", 540*time.Second, false, false)
	if _, err := db.ReplaceEvents(ctx, []string{"motion"}, nil, []store.Event{
		{SegmentID: a.ID, Type: "hard_brake", Source: "motion", TS: t0.Add(100 * time.Second)},
	}); err != nil {
		t.Fatal(err)
	}
	cfg := config.Default("/h")
	n, err := Plan(ctx, db, cfg, false)
	if err != nil {
		t.Fatal(err)
	}
	jobs, _ := db.Jobs(ctx, "", "")
	type row struct {
		Seg int64
		Pri int
		W   [][2]float64
	}
	var got []row
	for _, j := range jobs {
		got = append(got, row{j.SegmentID, j.Priority, j.Windows})
	}
	want := []row{
		{b.ID, 0, nil},
		{a.ID, 1, [][2]float64{{90, 105}}},
		{a.ID, 2, [][2]float64{{0, 90}, {105, 180}}},
		{d.ID, 2, nil},
	}
	if d := cmp.Diff(want, got); d != "" || n != 4 {
		t.Errorf("n=%d (-want +got):\n%s", n, d)
	}
	// Planning again adds nothing.
	if n, _ := Plan(ctx, db, cfg, false); n != 0 {
		t.Errorf("second plan added %d", n)
	}
}

func TestPlanOnlyWindows(t *testing.T) {
	ctx := context.Background()
	db, _ := store.Open(ctx, filepath.Join(t.TempDir(), "r.db"))
	defer db.Close()
	s := store.Segment{Path: "a", SourceName: "a", Hash: "a", Camera: "front", StartedAt: time.Unix(0, 0), DurationS: 180, ImportedAt: time.Unix(0, 0)}
	s.ID, _ = db.InsertSegment(ctx, s)
	if n, _ := Plan(ctx, db, config.Default("/h"), true); n != 0 {
		t.Errorf("planned %d jobs for a segment without events", n)
	}
}
