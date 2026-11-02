package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFindViewer(t *testing.T) {
	dir := t.TempDir()
	self := filepath.Join(dir, "go", "bin", "rearview.exe")
	repoViewer := filepath.Join(dir, "viewer", "build", "dev", "RelWithDebInfo", "viewer.exe")
	if err := os.MkdirAll(filepath.Dir(repoViewer), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(repoViewer, nil, 0o755); err != nil {
		t.Fatal(err)
	}
	got, err := findViewer("", self)
	if err != nil || got != repoViewer {
		t.Fatalf("repo build: got %q, %v; want %q", got, err, repoViewer)
	}

	next := filepath.Join(filepath.Dir(self), "viewer.exe")
	if err := os.MkdirAll(filepath.Dir(next), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(next, nil, 0o755); err != nil {
		t.Fatal(err)
	}
	if got, err := findViewer("", self); err != nil || got != next {
		t.Fatalf("next to rearview: got %q, %v; want %q", got, err, next)
	}

	if _, err := findViewer(filepath.Join(dir, "missing.exe"), self); err == nil {
		t.Fatal("configured missing viewer: want error")
	}
}
