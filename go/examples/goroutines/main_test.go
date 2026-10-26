package main

import (
	"bytes"
	"testing"
)

func TestRun(t *testing.T) {
	var out bytes.Buffer
	run(&out)
	want := "front_0001.mp4: 14000 bytes\nfront_0002.mp4: 14000 bytes\nrear_0001.mp4: 13000 bytes\n"
	if out.String() != want {
		t.Fatalf("got %q, want %q", out.String(), want)
	}
}
