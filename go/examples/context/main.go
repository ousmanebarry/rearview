// Context: cancel long-running work from outside, by hand or by timeout.
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"time"
)

func main() {
	run(os.Stdout)
}

func run(w io.Writer) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Millisecond)
	defer cancel()
	n, err := analyze(ctx)
	fmt.Fprintf(w, "timeout: stopped=%v deadline=%v frames>0=%v\n",
		err != nil, errors.Is(err, context.DeadlineExceeded), n > 0)

	ctx2, cancel2 := context.WithCancel(context.Background())
	cancel2()
	_, err = analyze(ctx2)
	fmt.Fprintf(w, "cancel: canceled=%v\n", errors.Is(err, context.Canceled))
}

// analyze pretends to process frames forever; it returns only when ctx ends.
func analyze(ctx context.Context) (int, error) {
	frames := 0
	tick := time.NewTicker(10 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return frames, fmt.Errorf("analyze stopped after %d frames: %w", frames, ctx.Err())
		case <-tick.C:
			frames++
		}
	}
}
