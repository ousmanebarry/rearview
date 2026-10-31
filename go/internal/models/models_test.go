package models

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync/atomic"
	"testing"
)

func sum(b string) string {
	h := sha256.Sum256([]byte(b))
	return hex.EncodeToString(h[:])
}

func TestDownload(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		switch r.URL.Path {
		case "/a":
			w.Write([]byte("model a"))
		case "/b":
			w.Write([]byte("tampered"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	files := []File{
		{Name: "a.onnx", URL: srv.URL + "/a", SHA256: sum("model a")},
		{Name: "b.onnx", URL: srv.URL + "/b", SHA256: sum("model b")},
		{Name: "c.onnx", URL: srv.URL + "/c", SHA256: sum("model c")},
	}
	dir := filepath.Join(t.TempDir(), "models")
	var log bytes.Buffer
	err := Download(context.Background(), srv.Client(), dir, files, &log)
	if err == nil || !strings.Contains(err.Error(), "b.onnx: checksum") || !strings.Contains(err.Error(), "c.onnx: GET") {
		t.Fatalf("err = %v", err)
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "a.onnx")); string(b) != "model a" {
		t.Errorf("a.onnx = %q", b)
	}
	for _, n := range []string{"b.onnx", "b.onnx.part", "c.onnx", "c.onnx.part"} {
		if _, err := os.Stat(filepath.Join(dir, n)); err == nil {
			t.Errorf("%s should not exist after a failed download", n)
		}
	}
	if got := Missing(dir, files); len(got) != 2 || got[0].Name != "b.onnx" {
		t.Errorf("missing = %v", got)
	}

	// A file already there with the right checksum isn't downloaded again.
	hits.Store(0)
	if err := Download(context.Background(), srv.Client(), dir, files[:1], &log); err != nil {
		t.Fatal(err)
	}
	if hits.Load() != 0 {
		t.Errorf("%d requests for a file already present", hits.Load())
	}
	// A corrupted one is.
	os.WriteFile(filepath.Join(dir, "a.onnx"), []byte("corrupt"), 0o644)
	if err := Download(context.Background(), srv.Client(), dir, files[:1], &log); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "a.onnx")); string(b) != "model a" || !strings.Contains(log.String(), "mismatch  a.onnx") {
		t.Errorf("a.onnx = %q\n%s", b, log.String())
	}
}

func TestDownloadCancelled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer srv.Close()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := Download(ctx, srv.Client(), t.TempDir(), []File{{Name: "a", URL: srv.URL, SHA256: "x"}}, &bytes.Buffer{})
	if err != context.Canceled {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
}

func TestFindNear(t *testing.T) {
	root := t.TempDir()
	exeDir := filepath.Join(root, "engine", "build", "dev", "RelWithDebInfo")
	os.MkdirAll(exeDir, 0o755)
	if _, ok := FindNear(exeDir); ok {
		t.Fatal("found models in an empty tree")
	}
	m := filepath.Join(root, "engine", "models")
	os.MkdirAll(m, 0o755)
	os.WriteFile(filepath.Join(m, Marker), nil, 0o644)
	if got, ok := FindNear(exeDir); !ok || got != m {
		t.Fatalf("FindNear = %q, %v; want %q", got, ok, m)
	}
	// Five levels at most, like the engine.
	deep := filepath.Join(exeDir, "a", "b")
	os.MkdirAll(deep, 0o755)
	if _, ok := FindNear(deep); ok {
		t.Fatal("searched further up than the engine does")
	}
}

// TestMatchesCMakeScript keeps Files in step with engine/scripts/download_models.cmake.
func TestMatchesCMakeScript(t *testing.T) {
	b, err := os.ReadFile("../../../engine/scripts/download_models.cmake")
	if err != nil {
		t.Fatal(err)
	}
	re := regexp.MustCompile(`"([^"|]+)\|([^"|]+)\|([0-9a-f]{64})"`)
	rows := re.FindAllStringSubmatch(string(b), -1)
	if len(rows) != len(Files) {
		t.Fatalf("cmake script lists %d files, Files has %d", len(rows), len(Files))
	}
	for i, r := range rows {
		f := Files[i]
		if r[1] != f.Name || r[2] != f.URL || r[3] != f.SHA256 {
			t.Errorf("entry %d: cmake %v, Go %+v", i, r[1:], f)
		}
	}
}
