package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConfigShow(t *testing.T) {
	p := filepath.Join(t.TempDir(), "c.yaml")
	os.WriteFile(p, []byte("motion:\n  swerve_deg: 25\n"), 0o644)
	out, _, err := execute(t, "config", "show", "--config", p)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "swerve_deg: 25") || !strings.Contains(out, "hard_brake_drop_kmh: 14") {
		t.Errorf("merged config:\n%s", out)
	}

	os.WriteFile(p, []byte("nope: 1\n"), 0o644)
	if _, _, err := execute(t, "config", "show", "--config", p); err == nil {
		t.Error("want error for an unknown key")
	}
}
