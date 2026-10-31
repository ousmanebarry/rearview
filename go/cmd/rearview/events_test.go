package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"rearview/internal/config"
	"rearview/internal/ingest"
	"rearview/internal/store"
)

func TestEventsDetectListAndEval(t *testing.T) {
	cfg, dir := testEnv(t)
	if _, _, err := execute(t, "import", card(t, dir), "--config", cfg); err != nil {
		t.Fatal(err)
	}
	out, _, err := execute(t, "events", "detect", "--config", cfg)
	if err != nil || !strings.Contains(out, "motion events") {
		t.Fatalf("detect: %v %s", err, out)
	}
	out, _, err = execute(t, "events", "--type", "hard_brake,swerve", "--since", "2026-10-01", "--config", cfg)
	if err != nil || !strings.HasPrefix(out, "ID") || !strings.Contains(out, " events\n") {
		t.Fatalf("events: %v\n%s", err, out)
	}
	if _, _, err := execute(t, "events", "--since", "yesterday", "--config", cfg); err == nil {
		t.Error("want error for a bad --since")
	}

	csv := filepath.Join(dir, "labels.csv")
	os.WriteFile(csv, []byte("ts_utc,camera,type,note\n2026-10-24T17:20:00Z,front,hard_brake,x\n"), 0o644)
	out, _, err = execute(t, "import-labels", "--file", csv, "--config", cfg)
	if err != nil || !strings.Contains(out, "imported 1 labels") {
		t.Fatalf("import-labels: %v %s", err, out)
	}
	out, _, err = execute(t, "eval", "--type", "hard_brake", "--config", cfg)
	if err != nil || !strings.Contains(out, "1 labels") || !strings.Contains(out, "miss ") {
		t.Fatalf("eval: %v\n%s", err, out)
	}
	if _, _, err := execute(t, "eval", "--config", cfg); err == nil {
		t.Error("want error without --type")
	}
}

func TestParseSince(t *testing.T) {
	now := time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)
	loc, _ := time.LoadLocation("America/New_York")
	for in, want := range map[string]time.Time{
		"7d":         now.Add(-7 * 24 * time.Hour),
		"12h":        now.Add(-12 * time.Hour),
		"2026-10-23": time.Date(2026, 10, 23, 4, 0, 0, 0, time.UTC),
	} {
		got, err := parseSince(in, now, loc)
		if err != nil || !got.Equal(want) {
			t.Errorf("%s: %v %v, want %v", in, got, err, want)
		}
	}
}

func TestClipCommand(t *testing.T) {
	if _, err := os.Stat("../../../testdata/clips/city.mp4"); err != nil {
		t.Skip("test clips not present")
	}
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not on PATH")
	}
	cfgPath, dir := testEnv(t)
	probeFunc = func(context.Context, string) (ingest.Probe, error) {
		return ingest.Probe{DurationS: 20, Width: 2560, Height: 1440, FPS: 30}, nil
	}
	src := filepath.Join(dir, "card")
	os.MkdirAll(src, 0o755)
	b, _ := os.ReadFile("../../../testdata/clips/city.mp4")
	os.WriteFile(filepath.Join(src, "2026_1024_124513_000679F.MP4"), b, 0o644)
	os.WriteFile(filepath.Join(src, "2026_1024_124513_000680R.MP4"), append(b, 0), 0o644) // same video, distinct hash
	if out, _, err := execute(t, "import", src, "--config", cfgPath); err != nil {
		t.Fatalf("%v %s", err, out)
	}

	cfg, _ := config.Load(cfgPath, dir)
	ctx := context.Background()
	db, err := store.Open(ctx, cfg.DatabasePath)
	if err != nil {
		t.Fatal(err)
	}
	segs, _ := db.Segments(ctx, 0)
	ids, err := db.ReplaceEvents(ctx, []string{"motion"}, nil, []store.Event{{
		SegmentID: segs[0].ID, TripID: segs[0].TripID, Type: "hard_brake", Source: "motion",
		TS: segs[0].StartedAt.Add(12 * time.Second), TStartS: 11, TEndS: 12, Score: 0.7,
	}})
	db.Close()
	if err != nil {
		t.Fatal(err)
	}
	id := strconv.FormatInt(ids[0], 10)

	out, _, err := execute(t, "clip", id, "--config", cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	path := strings.TrimSpace(out)
	if !strings.HasSuffix(path, "_hard_brake_"+id+".mp4") {
		t.Errorf("clip path %q", path)
	}
	if st, err := os.Stat(path); err != nil || st.Size() < 1000 {
		t.Errorf("clip file: %v", err)
	}
	out, _, err = execute(t, "clip", id, "--side-by-side", "--config", cfgPath)
	if err != nil || !strings.HasSuffix(strings.TrimSpace(out), "_sbs.mp4") {
		t.Errorf("side by side: %v %s", err, out)
	}
	if _, _, err := execute(t, "clip", "999", "--config", cfgPath); err == nil {
		t.Error("want error for a missing event")
	}
}
