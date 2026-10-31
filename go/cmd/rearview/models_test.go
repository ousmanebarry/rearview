package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"testing"

	"rearview/internal/config"
	"rearview/internal/models"
)

// fakeModels points the model list at a local server and counts requests.
func fakeModels(t *testing.T) *atomic.Int32 {
	t.Helper()
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.Write([]byte("model " + r.URL.Path))
	}))
	t.Cleanup(srv.Close)
	var files []models.File
	for _, n := range []string{models.Marker, "clip_text.onnx"} {
		h := sha256.Sum256([]byte("model /" + n))
		files = append(files, models.File{Name: n, URL: srv.URL + "/" + n, SHA256: hex.EncodeToString(h[:]), SizeMB: 1})
	}
	oldFiles, oldClient := modelFiles, modelClient
	modelFiles, modelClient = files, srv.Client()
	t.Cleanup(func() { modelFiles, modelClient = oldFiles, oldClient })
	t.Setenv("REARVIEW_MODELS", "")
	return &hits
}

// fakeBundle puts a fake engine where a release download has it: next to the
// rearview binary.
func fakeBundle(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	b, err := os.ReadFile(fakeEngine(t))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, engineName()), b, 0o755); err != nil {
		t.Fatal(err)
	}
	old := exeDir
	exeDir = func() (string, error) { return dir, nil }
	t.Cleanup(func() { exeDir = old })
	return dir
}

func TestModelsCommand(t *testing.T) {
	hits := fakeModels(t)
	cfg, dir := testEnv(t)
	m := filepath.Join(dir, "m")
	f, _ := os.OpenFile(cfg, os.O_APPEND|os.O_WRONLY, 0)
	f.WriteString("engine:\n  models_path: " + filepath.ToSlash(m) + "\n")
	f.Close()

	out, _, err := execute(t, "models", "--config", cfg)
	if err != nil || !strings.Contains(out, "yolox_s.onnx") || !strings.Contains(out, "missing") {
		t.Fatalf("%v\n%s", err, out)
	}
	out, errOut, err := execute(t, "models", "download", "--config", cfg)
	if err != nil || !strings.Contains(errOut, "download  clip_text.onnx") {
		t.Fatalf("%v\n%s%s", err, out, errOut)
	}
	if hits.Load() != 2 {
		t.Errorf("%d requests, want 2", hits.Load())
	}
	out, _, _ = execute(t, "models", "--config", cfg)
	if strings.Contains(out, "missing") || !strings.Contains(out, filepath.Clean(m)) {
		t.Errorf("after download:\n%s", out)
	}
}

func TestBundledEngineDownloadsModelsOnFirstRun(t *testing.T) {
	hits := fakeModels(t)
	bundle := fakeBundle(t)
	cfg := config.Default(t.TempDir())
	cfg.Engine.Path = filepath.Join(t.TempDir(), "no-such-engine")

	ecfg, err := engineConfig(context.Background(), cfg, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(bundle, engineName()); ecfg.Path != want {
		t.Errorf("engine %s, want the one next to rearview (%s)", ecfg.Path, want)
	}
	m := filepath.Join(bundle, "models")
	i := slices.Index(ecfg.Args, "--models")
	if i < 0 || ecfg.Args[i+1] != m {
		t.Fatalf("args %v should pass --models %s", ecfg.Args, m)
	}
	if len(models.Missing(m, modelFiles)) != 0 || hits.Load() != 2 {
		t.Fatalf("models not downloaded (%d requests)", hits.Load())
	}
	if _, err := engineConfig(context.Background(), cfg, io.Discard); err != nil || hits.Load() != 2 {
		t.Errorf("second run downloaded again (%d requests, %v)", hits.Load(), err)
	}
}

func TestDevEngineNeverDownloads(t *testing.T) {
	hits := fakeModels(t)
	empty := t.TempDir()
	t.Setenv("REARVIEW_MODELS", empty)
	cfg := config.Default(t.TempDir())
	cfg.Engine.Path = fakeEngine(t)

	ecfg, err := engineConfig(context.Background(), cfg, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if hits.Load() != 0 || slices.Contains(ecfg.Args, "--models") {
		t.Errorf("%d requests, args %v: a dev engine without models is left to report it", hits.Load(), ecfg.Args)
	}
}
