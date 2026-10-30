package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
)

var (
	fakeOnce sync.Once
	fakeBin  string
	fakeErr  error
)

// fakeEngine builds go/cmd/fakeengine once per test binary.
func fakeEngine(t *testing.T) string {
	t.Helper()
	fakeOnce.Do(func() {
		dir, err := os.MkdirTemp("", "fake")
		if err != nil {
			fakeErr = err
			return
		}
		fakeBin = filepath.Join(dir, "fakeengine")
		if runtime.GOOS == "windows" {
			fakeBin += ".exe"
		}
		if out, err := exec.Command("go", "build", "-o", fakeBin, "rearview/cmd/fakeengine").CombinedOutput(); err != nil {
			fakeErr = err
			fakeBin = string(out)
		}
	})
	if fakeErr != nil {
		t.Fatalf("build fakeengine: %v %s", fakeErr, fakeBin)
	}
	return fakeBin
}

// withEngine appends an engine section pointing at the fake engine.
func withEngine(t *testing.T, cfg string, extra string) {
	t.Helper()
	f, err := os.OpenFile(cfg, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	f.WriteString("engine:\n  path: " + filepath.ToSlash(fakeEngine(t)) + "\n  workers: 2\n  job_timeout_s: 5\n" + extra)
}

func TestProcessWithFakeEngine(t *testing.T) {
	cfg, dir := testEnv(t)
	withEngine(t, cfg, "")
	if _, _, err := execute(t, "import", card(t, dir), "--config", cfg); err != nil {
		t.Fatal(err)
	}
	out, _, err := execute(t, "process", "--plain", "--config", cfg)
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if !strings.Contains(out, "planned 4 new analyze jobs") || !strings.Contains(out, "done 4, failed 0") {
		t.Errorf("output:\n%s", out)
	}
	out, _, _ = execute(t, "process", "--plain", "--config", cfg)
	if !strings.Contains(out, "planned 0 new") || !strings.Contains(out, "done 0, failed 0") {
		t.Errorf("second run should have nothing to do:\n%s", out)
	}
}
