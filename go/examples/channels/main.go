// Channels: unbuffered vs buffered, closing, ranging and select.
package main

import (
	"fmt"
	"io"
	"os"
	"time"
)

func main() {
	run(os.Stdout)
}

func run(w io.Writer) {
	// Unbuffered: a send blocks until a receiver takes the value (a hand-off).
	handoff := make(chan string)
	go func() { handoff <- "frame 1" }()
	fmt.Fprintln(w, "unbuffered got:", <-handoff)

	// Buffered: sends succeed without a receiver until the buffer is full.
	// This is how a bounded queue applies back-pressure to a fast producer.
	queue := make(chan int, 3)
	queue <- 1
	queue <- 2
	queue <- 3
	fmt.Fprintf(w, "buffered len=%d cap=%d\n", len(queue), cap(queue))

	// Only the sender closes; range stops once the channel is closed and drained.
	close(queue)
	for job := range queue {
		fmt.Fprintln(w, "drained job", job)
	}

	// Receiving from a closed channel returns the zero value and ok=false.
	_, ok := <-queue
	fmt.Fprintln(w, "receive after close ok =", ok)

	// select waits on several channels; time.After gives it a timeout.
	silent := make(chan string)
	select {
	case msg := <-silent:
		fmt.Fprintln(w, "got", msg)
	case <-time.After(50 * time.Millisecond):
		fmt.Fprintln(w, "timed out waiting for a reply")
	}
}
