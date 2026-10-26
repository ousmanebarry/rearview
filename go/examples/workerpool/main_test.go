package main

import (
	"bytes"
	"context"
	"errors"
	"testing"
)

func TestRunProcessesEveryJobOnce(t *testing.T) {
	var out bytes.Buffer
	if err := run(context.Background(), &out, 10, 3); err != nil {
		t.Fatal(err)
	}
	want := "saved 10 results: [1 2 3 4 5 6 7 8 9 10]\n"
	if out.String() != want {
		t.Fatalf("got %q, want %q", out.String(), want)
	}
}

func TestRunStopsWhenCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var out bytes.Buffer
	err := run(ctx, &out, 1000, 4)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
}
