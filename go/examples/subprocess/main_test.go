package main

import (
	"bytes"
	"context"
	"os"
	"testing"
)

// TestMain lets the test binary act as the child process when re-executed.
func TestMain(m *testing.M) {
	if os.Getenv(childEnv) == "1" {
		child(os.Stdin, os.Stdout)
		return
	}
	os.Exit(m.Run())
}

func TestRunKillsHungChild(t *testing.T) {
	var out bytes.Buffer
	if err := run(context.Background(), &out); err != nil {
		t.Fatal(err)
	}
	want := "child is ready\nping -> pong\nhang -> no reply within 200ms\nchild killed and reaped\n"
	if out.String() != want {
		t.Fatalf("got %q, want %q", out.String(), want)
	}
}
