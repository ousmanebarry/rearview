package main

import (
	"bytes"
	"testing"
)

func TestRun(t *testing.T) {
	var out bytes.Buffer
	run(&out)
	want := "timeout: stopped=true deadline=true frames>0=true\ncancel: canceled=true\n"
	if out.String() != want {
		t.Fatalf("got %q, want %q", out.String(), want)
	}
}
