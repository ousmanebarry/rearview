package store

import (
	"context"
	"testing"
	"time"
)

func TestReplaceEventsKeepsIDs(t *testing.T) {
	db := OpenTest(t)
	ctx := context.Background()
	t0 := time.Date(2026, 10, 23, 17, 0, 0, 0, time.UTC)
	seg, err := db.InsertSegment(ctx, Segment{Path: "a.mp4", SourceName: "a.mp4", Hash: "a", Camera: "front", StartedAt: t0, DurationS: 180, ImportedAt: t0})
	if err != nil {
		t.Fatal(err)
	}
	ev := func(typ, source string, s int, score float64) Event {
		return Event{SegmentID: seg, Camera: "front", Type: typ, Source: source, TS: t0.Add(time.Duration(s) * time.Second), Score: score}
	}
	types := []string{"near_miss", "following_close"}
	sources := []string{"vision", "fused"}
	first, err := db.ReplaceEvents(ctx, sources, types, []Event{ev("near_miss", "vision", 10, 0.6), ev("following_close", "vision", 50, 0.7)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO clips (event_id, path, created_at) VALUES (?, 'c.mp4', ?)`, first[0], FormatTime(t0)); err != nil {
		t.Fatal(err)
	}
	// Re-run: the near-miss is now fused and rescored, the following event is gone, a new one appears.
	second, err := db.ReplaceEvents(ctx, sources, types, []Event{ev("near_miss", "fused", 10, 0.85), ev("following_close", "vision", 90, 0.7)})
	if err != nil {
		t.Fatal(err)
	}
	if second[0] != first[0] || second[1] == first[1] {
		t.Errorf("ids %v then %v: want the near-miss id kept and a new id for the new event", first, second)
	}
	var source string
	var score float64
	db.QueryRowContext(ctx, `SELECT source, score FROM events WHERE id = ?`, first[0]).Scan(&source, &score)
	if source != "fused" || score != 0.85 {
		t.Errorf("kept row not updated: %s %.2f", source, score)
	}
	var n, linked int
	db.QueryRowContext(ctx, `SELECT COUNT(*) FROM events`).Scan(&n)
	db.QueryRowContext(ctx, `SELECT COUNT(*) FROM clips WHERE event_id = ?`, first[0]).Scan(&linked)
	if n != 2 || linked != 1 {
		t.Errorf("%d events (want 2), clip linked %d (want 1)", n, linked)
	}
}
