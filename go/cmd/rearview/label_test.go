package main

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestLabelAddAndList(t *testing.T) {
	file := filepath.Join(t.TempDir(), "events.csv")

	out, _, err := execute(t, "label", "add", "--file", file,
		"--at", "2026-10-23T13:07:12", "--camera", "front", "--type", "hard_brake", "--note", "src=watched")
	if err != nil {
		t.Fatal(err)
	}
	if want := "added 2026-10-23T17:07:12Z front hard_brake (local 2026-10-23 13:07:12 EDT)\n"; out != want {
		t.Fatalf("add output = %q, want %q", out, want)
	}

	if _, _, err := execute(t, "label", "add", "--file", file,
		"--at", "2026-10-22T09:00:00Z", "--camera", "rear", "--type", "tailgated"); err != nil {
		t.Fatal(err)
	}

	out, _, err = execute(t, "label", "list", "--file", file)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 4 {
		t.Fatalf("list output:\n%s", out)
	}
	if !strings.HasPrefix(lines[1], "2026-10-22T09:00:00Z  2026-10-22 05:00:00 EDT") {
		t.Errorf("rows not sorted by time or wrong local time:\n%s", out)
	}
	if lines[3] != "2 labels" {
		t.Errorf("last line = %q", lines[3])
	}

	out, _, err = execute(t, "label", "list", "--file", file, "--type", "tailgated")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "1 labels") || strings.Contains(out, "hard_brake") {
		t.Errorf("type filter output:\n%s", out)
	}
}

func TestLabelAddRejectsBadInput(t *testing.T) {
	file := filepath.Join(t.TempDir(), "events.csv")
	tests := [][]string{
		{"--at", "yesterday", "--camera", "front", "--type", "near_miss"},
		{"--at", "2026-10-23T13:07:12", "--camera", "side", "--type", "near_miss"},
		{"--at", "2026-10-23T13:07:12", "--camera", "front", "--type", "crash"},
		{"--camera", "front", "--type", "near_miss"},
		{"--at", "2026-10-23T13:07:12", "--camera", "front", "--type", "near_miss", "--tz", "Mars/Olympus"},
	}
	for _, args := range tests {
		full := append([]string{"label", "add", "--file", file}, args...)
		if _, _, err := execute(t, full...); err == nil {
			t.Errorf("%v: want error", args)
		}
	}

	add := []string{"label", "add", "--file", file, "--at", "2026-10-23T13:07:12", "--camera", "front", "--type", "near_miss"}
	if _, _, err := execute(t, add...); err != nil {
		t.Fatal(err)
	}
	if _, _, err := execute(t, add...); err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Errorf("duplicate add: err = %v", err)
	}
}
