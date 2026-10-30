package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"rearview/internal/ingest"
)

// testEnv writes a config pointing at a temp library and database, and stubs
// ffprobe. It returns the --config path and the temp root.
func testEnv(t *testing.T) (string, string) {
	t.Helper()
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config.yaml")
	body := "library_path: " + filepath.ToSlash(filepath.Join(dir, "library")) + "\n" +
		"database_path: " + filepath.ToSlash(filepath.Join(dir, "r.db")) + "\n" +
		"clips_path: " + filepath.ToSlash(filepath.Join(dir, "clips")) + "\n" +
		"import:\n  stable_seconds: 0\n"
	if err := os.WriteFile(cfg, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	old := probeFunc
	probeFunc = func(context.Context, string) (ingest.Probe, error) {
		return ingest.Probe{DurationS: 180, Width: 2560, Height: 1440, FPS: 30}, nil
	}
	t.Cleanup(func() { probeFunc = old })
	return cfg, dir
}

// card copies the GPS fixtures under their camera file names.
func card(t *testing.T, dir string) string {
	t.Helper()
	root := filepath.Join(dir, "card", "DCIM", "Movie")
	os.MkdirAll(root, 0o755)
	for _, n := range []string{"2026_1023_151153_000593F", "2026_1024_122252_000669F", "2026_1024_131801_000700R"} {
		b, err := os.ReadFile("../../../testdata/gps/" + n + ".bin")
		if err != nil {
			t.Fatal(err)
		}
		os.WriteFile(filepath.Join(root, n+".MP4"), b, 0o644)
	}
	return filepath.Join(dir, "card")
}

func TestImportTripsExport(t *testing.T) {
	cfg, dir := testEnv(t)
	src := card(t, dir)
	out, _, err := execute(t, "import", src, "--config", cfg)
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if !strings.Contains(out, "new 3, skipped 0, failed 0") || !strings.Contains(out, "3 trips") {
		t.Errorf("first import:\n%s", out)
	}
	out, _, _ = execute(t, "import", src, "--config", cfg)
	if !strings.Contains(out, "new 0, skipped 3, failed 0") {
		t.Errorf("second import:\n%s", out)
	}

	out, _, err = execute(t, "trips", "--config", cfg)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 4 || !strings.Contains(lines[1], "2026-10-23  15:11") {
		t.Errorf("trips:\n%s", out)
	}

	gpx := filepath.Join(dir, "t.gpx")
	out, _, err = execute(t, "trips", "export", "1", "--gpx", gpx, "--config", cfg)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(gpx)
	if !strings.Contains(out, "(173 points)") || strings.Count(string(b), "<trkpt") != 173 {
		t.Errorf("export: %s, %d trkpt", out, strings.Count(string(b), "<trkpt"))
	}
	if _, _, err := execute(t, "trips", "export", "99", "--gpx", gpx, "--config", cfg); err == nil {
		t.Error("want error for a missing trip")
	}
}

func TestImportMissingFolder(t *testing.T) {
	cfg, dir := testEnv(t)
	if _, _, err := execute(t, "import", filepath.Join(dir, "nope"), "--config", cfg); err == nil {
		t.Error("want error")
	}
}
