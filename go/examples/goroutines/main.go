// Goroutines: start concurrent work and wait for all of it with a WaitGroup.
package main

import (
	"fmt"
	"io"
	"os"
	"sync"
)

func main() {
	run(os.Stdout)
}

func run(w io.Writer) {
	files := []string{"front_0001.mp4", "front_0002.mp4", "rear_0001.mp4"}
	sizes := make([]int, len(files))

	var wg sync.WaitGroup
	for i, name := range files {
		// Each goroutine writes only its own slot, so no mutex is needed.
		wg.Go(func() {
			sizes[i] = len(name) * 1000
		})
	}
	wg.Wait()

	for i, name := range files {
		fmt.Fprintf(w, "%s: %d bytes\n", name, sizes[i])
	}
}
