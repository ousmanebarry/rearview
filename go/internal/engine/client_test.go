package engine

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"
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

func cfg(args ...string) Config {
	return Config{Path: fakePath, Args: args, JobTimeout: time.Second, ReadyTimeout: 5 * time.Second, Stderr: io.Discard}
}

func job() Job {
	return Analyze("x.mp4", "front", [][2]float64{{12, 30}}, 10, []string{"detect", "track"})
}

func TestNormalRun(t *testing.T) {
	ctx := context.Background()
	c, err := Start(ctx, cfg())
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if c.Ready().EngineVersion != "fake" {
		t.Errorf("ready %+v", c.Ready())
	}
	if err := c.Ping(ctx); err != nil {
		t.Fatal(err)
	}
	var frames []Reply
	done, err := c.Do(ctx, job(), func(r Reply) {
		if r.Type == "frame" {
			frames = append(frames, r)
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(frames) != 5 || done.Frames != 5 || *frames[0].T != 12 || frames[0].Objects[0].Cls != "car" {
		t.Errorf("frames %d done %+v first %+v", len(frames), done, frames[0])
	}
	if *frames[0].Objects[0].TTCS != 3.2 || frames[0].Objects[1].TTCS != nil {
		t.Error("optional fields not decoded")
	}
	// The process stays up for the next job.
	if _, err := c.Do(ctx, job(), nil); err != nil {
		t.Fatal(err)
	}
}

func TestEngineErrorNotRetried(t *testing.T) {
	w := NewWorker(cfg("-fail", "file_not_found"))
	defer w.Close()
	resets := 0
	_, err := w.Do(context.Background(), job(), nil, func() { resets++ })
	var ee *EngineError
	if !errors.As(err, &ee) || ee.Code != "file_not_found" || resets != 0 {
		t.Errorf("err=%v resets=%d", err, resets)
	}
}

func TestRetryableErrorRetriedOnce(t *testing.T) {
	w := NewWorker(cfg("-fail", "decode_failed"))
	defer w.Close()
	resets := 0
	_, err := w.Do(context.Background(), job(), nil, func() { resets++ })
	var ee *EngineError
	if !errors.As(err, &ee) || ee.Code != "decode_failed" || resets != 1 {
		t.Errorf("err=%v resets=%d", err, resets)
	}
}

func TestCrashAndRetry(t *testing.T) {
	w := NewWorker(cfg("-crash-after", "1"))
	defer w.Close()
	ctx := context.Background()
	if _, err := w.Do(ctx, job(), nil, nil); err != nil {
		t.Fatal(err)
	}
	pid := w.Pid()
	frames, resets := 0, 0
	done, err := w.Do(ctx, job(), func(r Reply) { frames++ }, func() { resets++; frames = 0 })
	if err != nil {
		t.Fatal(err)
	}
	if resets != 1 || w.Restarts != 1 || w.Pid() == pid || done.Frames != 5 || frames != 6 {
		t.Errorf("resets=%d restarts=%d frames=%d done=%+v", resets, w.Restarts, frames, done)
	}
}

func TestCrashTwiceFails(t *testing.T) {
	w := NewWorker(cfg("-crash-after", "0"))
	defer w.Close()
	_, err := w.Do(context.Background(), job(), nil, nil)
	if !errors.Is(err, ErrDied) {
		t.Errorf("err = %v, want ErrDied", err)
	}
}

func TestHangAndRetry(t *testing.T) {
	w := NewWorker(cfg("-silent-after", "1"))
	defer w.Close()
	ctx := context.Background()
	if _, err := w.Do(ctx, job(), nil, nil); err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	_, err := w.Do(ctx, job(), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if el := time.Since(start); el < time.Second || el > 5*time.Second {
		t.Errorf("hang detected after %v, want about the 1 s job timeout", el)
	}
	if w.Restarts != 1 {
		t.Errorf("restarts %d", w.Restarts)
	}
}

func TestCancel(t *testing.T) {
	c, err := Start(context.Background(), cfg("-frames", "100", "-delay", "50ms"))
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	start := time.Now()
	r, err := c.Do(ctx, job(), nil)
	if !errors.Is(err, context.DeadlineExceeded) || !r.Cancelled {
		t.Errorf("err=%v reply=%+v", err, r)
	}
	if time.Since(start) > time.Second {
		t.Error("cancel took too long")
	}
	if !c.Alive() {
		t.Error("a cooperative cancel should leave the engine running")
	}
}

func TestCancelIgnoredKills(t *testing.T) {
	c, err := Start(context.Background(), cfg("-frames", "1000", "-delay", "10ms", "-ignore-cancel"))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err = c.Do(ctx, job(), nil)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("err = %v", err)
	}
	if el := time.Since(start); el < 2*time.Second || el > 4*time.Second {
		t.Errorf("killed after %v, want about 2 s", el)
	}
	if c.Alive() {
		t.Error("engine still alive")
	}
}

func TestVersionMismatch(t *testing.T) {
	if _, err := Start(context.Background(), cfg("-version", "2")); !errors.Is(err, ErrVersion) {
		t.Errorf("err = %v", err)
	}
}

func TestStartMissingBinary(t *testing.T) {
	c := cfg()
	c.Path = filepath.Join(t.TempDir(), "nope.exe")
	if _, err := Start(context.Background(), c); err == nil {
		t.Error("want error")
	}
}
