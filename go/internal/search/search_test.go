package search

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"rearview/internal/store"
)

func unit(i int) []float32 {
	v := make([]float32, 8)
	v[i] = 1
	return v
}

func mix(a, b int, wa float32) []float32 {
	v := make([]float32, 8)
	v[a], v[b] = wa, 1-wa
	return v
}

func TestSearchGroupsAndRanks(t *testing.T) {
	t0 := time.Date(2026, 10, 23, 17, 0, 0, 0, time.UTC)
	ix := &Index{segs: map[int64]store.Segment{1: {ID: 1, StartedAt: t0}, 2: {ID: 2, StartedAt: t0.Add(time.Hour)}}}
	add := func(seg int64, ts float64, v []float32) {
		ix.keys = append(ix.keys, key{seg, ts})
		ix.vecs = append(ix.vecs, v)
	}
	// Segment 1: a run of matches at 10-13 s (best at 12) and one at 40 s.
	add(1, 10, mix(0, 1, 0.7))
	add(1, 11, mix(0, 1, 0.8))
	add(1, 12, mix(0, 1, 0.95))
	add(1, 13, mix(0, 1, 0.6))
	add(1, 40, mix(0, 1, 0.75))
	// Segment 2: one strong match.
	add(2, 5, mix(0, 1, 0.9))
	// Noise.
	for i := 0; i < 20; i++ {
		add(2, float64(100+i), unit(3))
	}
	got := ix.Search(unit(0), 50, 5)
	if len(got) < 3 {
		t.Fatalf("got %d moments", len(got))
	}
	if got[0].Segment.ID != 1 || got[0].TOffsetS != 12 || got[0].Hits != 4 || got[0].FromS != 10 || got[0].ToS != 13 {
		t.Errorf("first moment %+v", got[0])
	}
	if got[1].Segment.ID != 2 || got[1].TOffsetS != 5 {
		t.Errorf("second moment %+v", got[1])
	}
	if got[2].TOffsetS != 40 {
		t.Errorf("third moment %+v", got[2])
	}
	if !got[0].At().Equal(t0.Add(12 * time.Second)) {
		t.Errorf("at %v", got[0].At())
	}
	if top := ix.Search(unit(0), 2, 5); len(top) != 2 || top[0].Hits != 1 {
		t.Errorf("topK 2 keeps the 2 best keyframes, one per moment: %+v", top)
	}
}

func TestBestCropCounts(t *testing.T) {
	ix := &Index{segs: map[int64]store.Segment{1: {ID: 1}}}
	ix.keys = []key{{1, 5}, {1, 5}}
	ix.vecs = [][]float32{unit(1), unit(0)} // left crop misses, right crop matches
	got := ix.Search(unit(0), 10, 5)
	if len(got) != 1 || got[0].Score != 1 {
		t.Errorf("got %+v", got)
	}
}

func TestLoadAndPrompt(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(ctx, filepath.Join(t.TempDir(), "r.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	id, _ := db.InsertSegment(ctx, store.Segment{Path: "a", SourceName: "a", Hash: "a", Camera: "front", StartedAt: time.Unix(0, 0), ImportedAt: time.Unix(0, 0)})
	db.InsertKeyframes(ctx, []store.Keyframe{
		{SegmentID: id, TOffsetS: 1, Vector: unit(0)},
		{SegmentID: id, TOffsetS: 1, Model: store.KeyframeModel + "/left", Vector: unit(1)},
	})
	c, _ := Load(ctx, db, false)
	a, _ := Load(ctx, db, true)
	if c.Len() != 1 || a.Len() != 2 {
		t.Errorf("centre %d, all %d", c.Len(), a.Len())
	}
	if p := Prompt("a dashcam photo of {query}", "a bus"); p != "a dashcam photo of a bus" {
		t.Errorf("prompt %q", p)
	}
	if p := Prompt("", "bus"); p != "bus" {
		t.Errorf("prompt %q", p)
	}
}
