package clips

import (
	"context"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

const city = "../../../testdata/clips/city.mp4"
const highway = "../../../testdata/clips/highway.mp4"

func needFFmpeg(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not on PATH")
	}
}

func probe(t *testing.T, path string) (dur float64, frames int, width int) {
	t.Helper()
	out, err := exec.Command("ffprobe", "-v", "error", "-select_streams", "v:0", "-count_packets",
		"-show_entries", "stream=width,nb_read_packets:format=duration", "-of", "default=nw=1", path).Output()
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		k, v, _ := strings.Cut(strings.TrimSpace(line), "=")
		switch k {
		case "duration":
			dur, _ = strconv.ParseFloat(v, 64)
		case "nb_read_packets":
			frames, _ = strconv.Atoi(v)
		case "width":
			width, _ = strconv.Atoi(v)
		}
	}
	return
}

func TestCutStreamCopyIsFast(t *testing.T) {
	needFFmpeg(t)
	out := filepath.Join(t.TempDir(), "a.mp4")
	start := time.Now()
	if err := Cut(context.Background(), []Part{{city, 5, 15}}, out, Options{}); err != nil {
		t.Fatal(err)
	}
	if el := time.Since(start); el > 2*time.Second {
		t.Errorf("stream-copy cut took %v, want under 2 s", el)
	}
	dur, frames, _ := probe(t, out)
	if dur < 9.9 || dur > 10.6 || frames < 297 || frames > 316 {
		t.Errorf("duration %.2f s, %d frames; want about 10 s / 300", dur, frames)
	}
}

func TestCutAcrossFiles(t *testing.T) {
	needFFmpeg(t)
	out := filepath.Join(t.TempDir(), "b.mp4")
	// The last 4 s of one file and the first 6 s of the next.
	if err := Cut(context.Background(), []Part{{city, 16, 20}, {highway, 0, 6}}, out, Options{}); err != nil {
		t.Fatal(err)
	}
	dur, frames, _ := probe(t, out)
	if dur < 9.9 || dur > 10.6 || frames < 297 || frames > 316 {
		t.Errorf("joined clip %.2f s, %d frames; want about 10 s with no gap", dur, frames)
	}
}

func TestCutExact(t *testing.T) {
	needFFmpeg(t)
	out := filepath.Join(t.TempDir(), "c.mp4")
	if err := Cut(context.Background(), []Part{{city, 3.2, 4.2}}, out, Options{Exact: true}); err != nil {
		t.Fatal(err)
	}
	if _, frames, _ := probe(t, out); frames != 30 {
		t.Errorf("exact 1 s cut has %d frames, want 30", frames)
	}
}

func TestSideBySide(t *testing.T) {
	needFFmpeg(t)
	out := filepath.Join(t.TempDir(), "d.mp4")
	if err := SideBySide(context.Background(), []Part{{city, 0, 1}}, []Part{{highway, 0, 1}}, out, Options{}); err != nil {
		t.Fatal(err)
	}
	if _, _, w := probe(t, out); w != 2560 {
		t.Errorf("width %d, want 2560", w)
	}
}

func TestErrors(t *testing.T) {
	needFFmpeg(t)
	dir := t.TempDir()
	if err := Cut(context.Background(), nil, filepath.Join(dir, "x.mp4"), Options{}); err == nil {
		t.Error("want error for no parts")
	}
	if err := Cut(context.Background(), []Part{{"missing.mp4", 0, 1}}, filepath.Join(dir, "y.mp4"), Options{}); err == nil {
		t.Error("want error for a missing file")
	}
	if err := Cut(context.Background(), []Part{{city, 5, 5}}, filepath.Join(dir, "z.mp4"), Options{}); err == nil {
		t.Error("want error for an empty part")
	}
}

func TestName(t *testing.T) {
	loc, _ := time.LoadLocation("America/New_York")
	got := Name(time.Date(2026, 10, 23, 17, 7, 12, 0, time.UTC), "hard_brake", 42, loc)
	if got != "20261023_130712_hard_brake_42.mp4" {
		t.Errorf("name %q", got)
	}
}
