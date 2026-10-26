// errgroup: run goroutines together; the first error cancels the rest.
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"golang.org/x/sync/errgroup"
)

func main() {
	run(os.Stdout)
}

func run(w io.Writer) {
	err := importAll(context.Background(), []string{"a.mp4", "b.mp4", "c.mp4"})
	fmt.Fprintln(w, "all ok:", err)

	err = importAll(context.Background(), []string{"a.mp4", "corrupt.mp4", "c.mp4", "d.mp4"})
	fmt.Fprintln(w, "with failure:", err)
}

// importAll processes at most 2 files at a time and stops at the first failure.
func importAll(ctx context.Context, files []string) error {
	g, ctx := errgroup.WithContext(ctx)
	g.SetLimit(2)
	for _, f := range files {
		g.Go(func() error {
			if err := importOne(ctx, f); err != nil {
				return fmt.Errorf("import %s: %w", f, err)
			}
			return nil
		})
	}
	// Wait returns the first non-nil error, after every goroutine has returned.
	return g.Wait()
}

func importOne(ctx context.Context, f string) error {
	if f == "corrupt.mp4" {
		return errors.New("bad moov box")
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(20 * time.Millisecond):
		return nil
	}
}
