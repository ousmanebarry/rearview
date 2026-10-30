package ingest

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"rearview/internal/config"
	"rearview/internal/store"
)

func fakeProbe(context.Context, string) (Probe, error) {
	return Probe{DurationS: 180, Width: 2560, Height: 1440, FPS: 30}, nil
}

type env struct {
	in   *Ingester
	db   *store.DB
	card string
}

func newEnv(t *testing.T) env {
	t.Helper()
	dir := t.TempDir()
	db, err := store.Open(context.Background(), filepath.Join(dir, "r.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	cfg := config.Default(dir)
	in, err := New(db, cfg, fakeProbe)
	if err != nil {
		t.Fatal(err)
	}
	in.StableFor = 0
	in.PollEvery = 10 * time.Millisecond
	card := filepath.Join(dir, "card")
	return env{in, db, card}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	os.MkdirAll(filepath.Dir(path), 0o755)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func count(t *testing.T, db *store.DB) int {
	var n int
	db.QueryRow(`SELECT COUNT(*) FROM segments`).Scan(&n)
	return n
}

func statuses(rs []Result) map[Status]int {
	m := map[Status]int{}
	for _, r := range rs {
		m[r.Status]++
	}
	return m
}

func TestImportTwiceAddsNothing(t *testing.T) {
	e := newEnv(t)
	writeFile(t, filepath.Join(e.card, "DCIM/Movie/2026_1023_130515_000587F.MP4"), "front video")
	writeFile(t, filepath.Join(e.card, "DCIM/Movie/2026_1023_130515_000588R.MP4"), "rear video")
	writeFile(t, filepath.Join(e.card, "DCIM/Movie/RO/2026_1023_131115_000591F.MP4"), "locked")
	writeFile(t, filepath.Join(e.card, "DCIM/Movie/notes.txt"), "ignored")
	ctx := context.Background()

	rs, err := e.in.ImportDir(ctx, e.card)
	if err != nil {
		t.Fatal(err)
	}
	if s := statuses(rs); s[StatusNew] != 3 || s[StatusFailed] != 0 {
		t.Fatalf("first import: %v %+v", s, rs)
	}
	rs, _ = e.in.ImportDir(ctx, e.card)
	if s := statuses(rs); s[StatusSkipped] != 3 || s[StatusNew] != 0 {
		t.Fatalf("second import: %v", s)
	}
	if n := count(t, e.db); n != 3 {
		t.Fatalf("%d rows, want 3", n)
	}

	segs, _ := e.db.Segments(ctx, 0)
	byName := map[string]store.Segment{}
	for _, s := range segs {
		byName[s.SourceName] = s
	}
	f := byName["2026_1023_130515_000587F.MP4"]
	if f.Camera != "front" || f.Locked || f.Path != "2026/10/23/2026_1023_130515_000587F.MP4" {
		t.Errorf("front segment: %+v", f)
	}
	// 13:05:15 EDT is 17:05:15 UTC.
	if want := time.Date(2026, 10, 23, 17, 5, 15, 0, time.UTC); !f.StartedAt.Equal(want) {
		t.Errorf("started_at %v, want %v", f.StartedAt, want)
	}
	if f.Width != 2560 || f.FPS != 30 || f.DurationS != 180 {
		t.Errorf("probe values not stored: %+v", f)
	}
	if byName["2026_1023_130515_000588R.MP4"].Camera != "rear" {
		t.Error("rear not tagged")
	}
	if !byName["2026_1023_131115_000591F.MP4"].Locked {
		t.Error("RO file not locked")
	}
	b, err := os.ReadFile(filepath.Join(e.in.Library, "2026", "10", "23", "2026_1023_130515_000587F.MP4"))
	if err != nil || string(b) != "front video" {
		t.Errorf("library copy: %q %v", b, err)
	}
}

type failingReader struct{ n int }

func (f *failingReader) Read(p []byte) (int, error) {
	if f.n <= 0 {
		return 0, errors.New("device removed")
	}
	k := min(len(p), f.n)
	f.n -= k
	return k, nil
}
func (f *failingReader) Close() error { return nil }

func TestCardPulledMidCopyLeavesNothing(t *testing.T) {
	e := newEnv(t)
	src := filepath.Join(e.card, "2026_1023_130515_000587F.MP4")
	writeFile(t, src, strings.Repeat("x", 1000))
	e.in.open = func(string) (io.ReadCloser, error) { return &failingReader{n: 300}, nil }

	rs, _ := e.in.ImportDir(context.Background(), e.card)
	if len(rs) != 1 || rs[0].Status != StatusFailed || !strings.Contains(rs[0].Err.Error(), "device removed") {
		t.Fatalf("results: %+v", rs)
	}
	if n := count(t, e.db); n != 0 {
		t.Errorf("%d rows after a failed copy", n)
	}
	var files []string
	filepath.WalkDir(e.in.Library, func(p string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			files = append(files, p)
		}
		return nil
	})
	if len(files) != 0 {
		t.Errorf("files left in library: %v", files)
	}
}

func TestUnknownCameraFails(t *testing.T) {
	e := newEnv(t)
	writeFile(t, filepath.Join(e.card, "clip.mp4"), "x")
	rs, _ := e.in.ImportDir(context.Background(), e.card)
	if rs[0].Status != StatusFailed {
		t.Fatalf("want failure, got %+v", rs[0])
	}
}

func TestWaitsForGrowingFile(t *testing.T) {
	e := newEnv(t)
	e.in.StableFor = 300 * time.Millisecond
	src := filepath.Join(e.card, "2026_1023_130515_000587F.MP4")
	writeFile(t, src, "part one")
	go func() {
		time.Sleep(100 * time.Millisecond)
		f, _ := os.OpenFile(src, os.O_APPEND|os.O_WRONLY, 0)
		f.WriteString(" part two")
		f.Close()
	}()
	start := time.Now()
	r := e.in.ImportFile(context.Background(), e.card, src)
	if r.Status != StatusNew {
		t.Fatalf("%+v", r)
	}
	if time.Since(start) < 300*time.Millisecond {
		t.Error("did not wait for the file to settle")
	}
	if r.Segment.SizeBytes != int64(len("part one part two")) {
		t.Errorf("imported %d bytes, want the full file", r.Segment.SizeBytes)
	}
}

func TestWaitStableCancel(t *testing.T) {
	e := newEnv(t)
	e.in.StableFor = time.Hour
	src := filepath.Join(e.card, "2026_1023_130515_000587F.MP4")
	writeFile(t, src, "x")
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if r := e.in.ImportFile(ctx, e.card, src); !errors.Is(r.Err, context.DeadlineExceeded) {
		t.Errorf("err = %v", r.Err)
	}
}

func TestHash(t *testing.T) {
	big := bytes.Repeat([]byte{1}, 9<<20)
	h1, _ := Hash(bytes.NewReader(big), int64(len(big)))
	big[len(big)-1] = 2 // tail differs
	h2, _ := Hash(bytes.NewReader(big), int64(len(big)))
	big[len(big)-1] = 1
	big[4<<20+1000] = 9 // middle differs: not covered, by design
	h3, _ := Hash(bytes.NewReader(big), int64(len(big)))
	if h1 == h2 || h1 != h3 {
		t.Errorf("h1=%s h2=%s h3=%s", h1, h2, h3)
	}
	small, _ := Hash(strings.NewReader("abc"), 3)
	if len(small) != 64 {
		t.Errorf("hash %q", small)
	}
}

func TestParseFFprobe(t *testing.T) {
	p, err := parseFFprobe([]byte(`{"streams":[{"width":2560,"height":1440,"avg_frame_rate":"30000/1001"}],"format":{"duration":"180.013"}}`))
	if err != nil {
		t.Fatal(err)
	}
	if p.Width != 2560 || p.Height != 1440 || p.DurationS != 180.013 || p.FPS < 29.96 || p.FPS > 29.98 {
		t.Errorf("%+v", p)
	}
	if _, err := parseFFprobe([]byte(`{"streams":[]}`)); err == nil {
		t.Error("want error with no video stream")
	}
}

func TestCleanPartials(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "a/b.mp4.part"), "x")
	writeFile(t, filepath.Join(dir, "a/c.mp4"), "x")
	if err := CleanPartials(dir); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "a/b.mp4.part")); err == nil {
		t.Error(".part not removed")
	}
	if _, err := os.Stat(filepath.Join(dir, "a/c.mp4")); err != nil {
		t.Error("real file removed")
	}
	if err := CleanPartials(filepath.Join(dir, "missing")); err != nil {
		t.Error(err)
	}
}
