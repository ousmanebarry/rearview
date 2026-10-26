package main

import (
	"bytes"
	"testing"
)

func TestHighPriorityDrainsFirst(t *testing.T) {
	var out bytes.Buffer
	run(&out)
	want := "locked RO clip\nmotion-event window\nnormal segment 1\nnormal segment 2\n"
	if out.String() != want {
		t.Fatalf("got %q, want %q", out.String(), want)
	}
}
