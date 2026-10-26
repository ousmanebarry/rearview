package main

import (
	"bytes"
	"testing"
)

func TestRun(t *testing.T) {
	var out bytes.Buffer
	run(&out)
	want := "all ok: <nil>\nwith failure: import corrupt.mp4: bad moov box\n"
	if out.String() != want {
		t.Fatalf("got %q, want %q", out.String(), want)
	}
}
