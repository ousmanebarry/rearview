// Priority select: always drain the high-priority channel before the low one.
// This is the two-channel pattern task 4.4 can use for the job queue.
package main

import (
	"context"
	"fmt"
	"io"
	"os"
)

func main() {
	run(os.Stdout)
}

func run(w io.Writer) {
	high := make(chan string, 10)
	low := make(chan string, 10)
	low <- "normal segment 1"
	low <- "normal segment 2"
	high <- "locked RO clip"
	high <- "motion-event window"
	close(high)
	close(low)

	for {
		j, ok := next(context.Background(), high, low)
		if !ok {
			return
		}
		fmt.Fprintln(w, j)
	}
}

// next returns a high-priority job if one is waiting, otherwise whichever comes first.
// A plain select picks randomly among ready cases, so it alone cannot give priority.
func next(ctx context.Context, high, low <-chan string) (string, bool) {
	for high != nil || low != nil {
		select {
		case j, ok := <-high:
			if ok {
				return j, true
			}
			high = nil // a nil channel blocks forever, which removes it from the select
			continue
		default:
		}
		select {
		case <-ctx.Done():
			return "", false
		case j, ok := <-high:
			if !ok {
				high = nil
				continue
			}
			return j, true
		case j, ok := <-low:
			if !ok {
				low = nil
				continue
			}
			return j, true
		}
	}
	return "", false
}
