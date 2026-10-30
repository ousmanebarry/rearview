package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func write(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestMissingFileGivesDefaults(t *testing.T) {
	c, err := Load(filepath.Join(t.TempDir(), "none.yaml"), "/home/u")
	if err != nil {
		t.Fatal(err)
	}
	if c.Motion.HardBrakeDropKmh != 14 || c.LibraryPath != filepath.Join("/home/u", "rearview", "library") {
		t.Errorf("defaults not applied: %+v", c)
	}
}

func TestFileOverridesOnlyWhatItSets(t *testing.T) {
	p := write(t, "library_path: D:/lib\nmotion:\n  hard_brake_drop_kmh: 12\nengine:\n  workers: 4\n")
	c, err := Load(p, "/home/u")
	if err != nil {
		t.Fatal(err)
	}
	if c.LibraryPath != "D:/lib" || c.Motion.HardBrakeDropKmh != 12 || c.Engine.Workers != 4 {
		t.Errorf("overrides not applied: %+v", c)
	}
	if c.Motion.SwerveDeg != 30 || c.Engine.Device != "cpu" {
		t.Errorf("untouched values changed: swerve=%v device=%v", c.Motion.SwerveDeg, c.Engine.Device)
	}
}

func TestEmptyFile(t *testing.T) {
	if _, err := Load(write(t, ""), "/h"); err != nil {
		t.Fatal(err)
	}
}

func TestRejects(t *testing.T) {
	for name, body := range map[string]string{
		"unknown key":   "motion:\n  hard_brak_drop_kmh: 12\n",
		"bad regex":     "import:\n  front_pattern: \"(\"\n",
		"bad device":    "engine:\n  device: cuda\n",
		"zero workers":  "engine:\n  workers: 0\n",
		"not yaml":      "::: [",
		"bad trapezoid": "near_miss:\n  path_trapezoid: [[0, 1]]\n",
	} {
		if _, err := Load(write(t, body), "/h"); err == nil {
			t.Errorf("%s: want error", name)
		}
	}
}

func TestYAMLRoundTrip(t *testing.T) {
	c := Default("/h")
	b, err := c.YAML()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "hard_brake_drop_kmh: 14") {
		t.Errorf("yaml:\n%s", b)
	}
	back, err := Load(write(t, string(b)), "/other")
	if err != nil {
		t.Fatal(err)
	}
	if back.LibraryPath != c.LibraryPath || back.NearMiss.PathTrapezoid[3] != c.NearMiss.PathTrapezoid[3] {
		t.Error("round trip lost values")
	}
}
