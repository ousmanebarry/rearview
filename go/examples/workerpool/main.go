// Worker pool: the shape of the Phase 4 pipeline.
// producer -> bounded jobs channel -> N workers -> results channel -> one writer.
package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"sort"
	"sync"

	"golang.org/x/sync/errgroup"
)

type job struct{ id int }

type result struct {
	jobID  int
	worker int
}

func main() {
	if err := run(context.Background(), os.Stdout, 10, 3); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context, w io.Writer, numJobs, numWorkers int) error {
	g, ctx := errgroup.WithContext(ctx)
	jobs := make(chan job, numWorkers)
	results := make(chan result, numWorkers)

	// Producer: the only sender on jobs, so it is the one that closes it.
	g.Go(func() error {
		defer close(jobs)
		for i := 1; i <= numJobs; i++ {
			select {
			case jobs <- job{id: i}:
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		return nil
	})

	// Workers: many senders on results, so nobody closes it until all are done.
	var workers sync.WaitGroup
	for n := 1; n <= numWorkers; n++ {
		workers.Go(func() {
			for j := range jobs {
				select {
				case results <- result{jobID: j.id, worker: n}:
				case <-ctx.Done():
					return
				}
			}
		})
	}
	g.Go(func() error {
		workers.Wait()
		close(results)
		return nil
	})

	// Single writer: the only goroutine touching storage (SQLite in the real app).
	var saved []int
	g.Go(func() error {
		for r := range results {
			saved = append(saved, r.jobID)
		}
		return nil
	})

	if err := g.Wait(); err != nil {
		return fmt.Errorf("pipeline: %w", err)
	}
	sort.Ints(saved)
	fmt.Fprintf(w, "saved %d results: %v\n", len(saved), saved)
	return nil
}
