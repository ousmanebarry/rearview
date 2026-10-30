package pipeline

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"rearview/internal/engine"
	"rearview/internal/store"
)

var fakePath string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "fakeengine")
	if err != nil {
		panic(err)
	}
	fakePath = filepath.Join(dir, "fakeengine")
	if runtime.GOOS == "windows" {
		fakePath += ".exe"
	}
	if out, err := exec.Command("go", "build", "-o", fakePath, "rearview/cmd/fakeengine").CombinedOutput(); err != nil {
		panic(string(out))
	}
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

// setup creates n segments with one pending analyze job each.
func setup(t *testing.T, n int) *store.DB {
	t.Helper()
	ctx := context.Background()
	db, err := store.Open(ctx, filepath.Join(t.TempDir(), "r.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	t0 := time.Date(2026, 10, 23, 17, 0, 0, 0, time.UTC)
	for i := range n {
		name := fmt.Sprintf("s%03d.mp4", i)
		id, err := db.InsertSegment(ctx, store.Segment{Path: name, SourceName: name, Hash: name, Camera: "front",
			StartedAt: t0.Add(time.Duration(i) * 3 * time.Minute), DurationS: 180, ImportedAt: t0})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.InsertJob(ctx, store.Job{SegmentID: id, Kind: "analyze", Priority: i % 3}); err != nil {
			t.Fatal(err)
		}
	}
	return db
}

func opts(workers int, args ...string) Options {
	return Options{
		Workers:  workers,
		Engine:   engine.Config{Path: fakePath, Args: args, JobTimeout: time.Second, Stderr: io.Discard},
		Library:  "lib",
		FPS:      10,
		Features: []string{"detect", "track"},
	}
}

func count(t *testing.T, db *store.DB, q string) int {
	t.Helper()
	var n int
	if err := db.QueryRow(q).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestHundredJobsFourWorkers(t *testing.T) {
	db := setup(t, 100)
	st, err := Run(context.Background(), db, opts(4))
	if err != nil {
		t.Fatal(err)
	}
	if st.Done != 100 || st.Failed != 0 || st.Frames != 500 {
		t.Errorf("stats %+v", st)
	}
	if n := count(t, db, `SELECT COUNT(*) FROM jobs WHERE state = 'done'`); n != 100 {
		t.Errorf("%d jobs done", n)
	}
	// 5 frames x 2 objects per job.
	if n := count(t, db, `SELECT COUNT(*) FROM observations`); n != 1000 {
		t.Errorf("%d observations, want 1000", n)
	}
	busy := 0
	for _, w := range st.Workers {
		if w.Frames > 0 {
			busy++
		}
	}
	if busy < 2 {
		t.Errorf("only %d workers did any work", busy)
	}
}

func TestCrashRetryKeepsNoPartialRows(t *testing.T) {
	db := setup(t, 6)
	st, err := Run(context.Background(), db, opts(2, "-crash-after", "1"))
	if err != nil {
		t.Fatal(err)
	}
	if st.Done != 6 {
		t.Errorf("stats %+v", st)
	}
	if n := count(t, db, `SELECT COUNT(*) FROM observations`); n != 60 {
		t.Errorf("%d observations, want 60 (partial rows from crashed attempts must be gone)", n)
	}
}

func TestFailuresAreRecorded(t *testing.T) {
	db := setup(t, 3)
	st, err := Run(context.Background(), db, opts(2, "-fail", "file_not_found"))
	if err != nil {
		t.Fatal(err)
	}
	if st.Failed != 3 || len(st.Errors) != 3 {
		t.Errorf("stats %+v", st)
	}
	var le string
	db.QueryRow(`SELECT last_error FROM jobs LIMIT 1`).Scan(&le)
	if le == "" {
		t.Error("last_error not saved")
	}
}

func TestCancelThenResume(t *testing.T) {
	db := setup(t, 20)
	ctx, cancel := context.WithTimeout(context.Background(), 700*time.Millisecond)
	defer cancel()
	st, err := Run(ctx, db, opts(2, "-delay", "50ms"))
	if err == nil {
		t.Fatal("want a cancellation error")
	}
	if st.Done == 0 || st.Done == 20 {
		t.Fatalf("done %d: the test needs a partial run", st.Done)
	}
	if n := count(t, db, `SELECT COUNT(*) FROM jobs WHERE state = 'running'`); n != 0 {
		t.Errorf("%d jobs left running after a clean cancel", n)
	}

	st2, err := Run(context.Background(), db, opts(2))
	if err != nil {
		t.Fatal(err)
	}
	if st.Done+st2.Done != 20 {
		t.Errorf("first run %d + second run %d != 20", st.Done, st2.Done)
	}
	if n := count(t, db, `SELECT COUNT(*) FROM observations`); n != 200 {
		t.Errorf("%d observations, want 200 with no duplicates", n)
	}
}

func TestResumeAfterHardKill(t *testing.T) {
	db := setup(t, 4)
	ctx := context.Background()
	// Simulate a process killed mid-job: job 1 running with partial rows.
	db.StartJob(ctx, 1)
	db.InsertObservations(ctx, []store.Observation{{JobID: 1, SegmentID: 1, TrackID: 9, Cls: "car", Conf: 1}})
	st, err := Run(ctx, db, opts(2))
	if err != nil {
		t.Fatal(err)
	}
	if st.Done != 4 {
		t.Errorf("done %d", st.Done)
	}
	if n := count(t, db, `SELECT COUNT(*) FROM observations WHERE track_id = 9`); n != 0 {
		t.Error("partial observations from the killed run survived")
	}
	if n := count(t, db, `SELECT COUNT(*) FROM observations`); n != 40 {
		t.Errorf("%d observations, want 40", n)
	}
	var attempts int
	db.QueryRow(`SELECT attempts FROM jobs WHERE id = 1`).Scan(&attempts)
	if attempts != 2 {
		t.Errorf("attempts %d, want 2", attempts)
	}
}

func TestPrefixWriter(t *testing.T) {
	var b stringsBuilder
	p := &prefixWriter{w: &b, prefix: "[w1] "}
	p.Write([]byte("one\ntw"))
	p.Write([]byte("o\n"))
	if b.s != "[w1] one\n[w1] two\n" {
		t.Errorf("%q", b.s)
	}
}

type stringsBuilder struct{ s string }

func (b *stringsBuilder) Write(p []byte) (int, error) { b.s += string(p); return len(p), nil }
