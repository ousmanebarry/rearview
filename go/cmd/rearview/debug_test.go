package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const gpsFixture = "../../../testdata/gps/2026_1023_151153_000593F.bin"

func TestDebugBoxes(t *testing.T) {
	out, _, err := execute(t, "debug", "boxes", gpsFixture)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "\"moov\" offset=16") || !strings.Contains(out, "  \"gps \" offset=24") {
		t.Errorf("unexpected tree:\n%.300s", out)
	}
}

func TestDebugGPSAndFixture(t *testing.T) {
	out, _, err := execute(t, "debug", "gps", gpsFixture)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if lines[0] != "t_offset_s,ts_utc,lat,lon,speed_kmh,heading_deg" || len(lines) != 174 {
		t.Fatalf("got %d lines starting %q", len(lines), lines[0])
	}

	// A fixture of a fixture is byte-identical.
	again := filepath.Join(t.TempDir(), "again.bin")
	if _, _, err := execute(t, "debug", "gps-fixture", gpsFixture, again); err != nil {
		t.Fatal(err)
	}
	a, _ := os.ReadFile(gpsFixture)
	b, _ := os.ReadFile(again)
	if string(a) != string(b) {
		t.Error("re-cut fixture differs")
	}

	if _, _, err := execute(t, "debug", "gps", "missing.mp4"); err == nil {
		t.Error("want error for a missing file")
	}
}
