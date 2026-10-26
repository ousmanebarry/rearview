package main

import (
	"bytes"
	"testing"
)

func TestRun(t *testing.T) {
	var out bytes.Buffer
	run(&out)
	want := "unbuffered got: frame 1\n" +
		"buffered len=3 cap=3\n" +
		"drained job 1\ndrained job 2\ndrained job 3\n" +
		"receive after close ok = false\n" +
		"timed out waiting for a reply\n"
	if out.String() != want {
		t.Fatalf("got %q, want %q", out.String(), want)
	}
}
