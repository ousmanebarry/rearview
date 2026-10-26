package main

import (
	"bytes"
	"testing"
)

func TestRunPrintsVersion(t *testing.T) {
	var out bytes.Buffer
	if code := run(nil, &out); code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if got, want := out.String(), "rearview "+version+"\n"; got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}
