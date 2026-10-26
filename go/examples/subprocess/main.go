// Subprocess: drive a long-running child over stdin/stdout lines, and kill it when it hangs.
// This is the shape of the engine client in task 4.3. The child is this same binary,
// re-started with REARVIEW_EXAMPLE_CHILD=1, so the example needs no C++ build.
package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"time"
)

const childEnv = "REARVIEW_EXAMPLE_CHILD"

func main() {
	if os.Getenv(childEnv) == "1" {
		child(os.Stdin, os.Stdout)
		return
	}
	if err := run(context.Background(), os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// child is a toy engine: "ready" on start, "pong" for "ping", silence for "hang".
func child(in io.Reader, out io.Writer) {
	fmt.Fprintln(out, "ready")
	sc := bufio.NewScanner(in)
	for sc.Scan() {
		switch sc.Text() {
		case "ping":
			fmt.Fprintln(out, "pong")
		case "hang":
			fmt.Fprintln(os.Stderr, "child: pretending to hang")
			time.Sleep(time.Hour) // select{} would trip Go's deadlock detector and exit instead
		}
	}
}

func run(ctx context.Context, w io.Writer) error {
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("find own executable: %w", err)
	}
	cmd := exec.CommandContext(ctx, exe)
	cmd.Env = append(os.Environ(), childEnv+"=1")
	cmd.Stderr = os.Stderr
	// If ctx is cancelled, CommandContext kills the child; WaitDelay bounds how long Wait then blocks on pipes.
	cmd.WaitDelay = 2 * time.Second
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("stdout pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start child: %w", err)
	}

	// One reader goroutine owns stdout; it exits when the child's stdout closes.
	lines := make(chan string)
	go func() {
		defer close(lines)
		sc := bufio.NewScanner(stdout)
		for sc.Scan() {
			lines <- sc.Text()
		}
	}()

	if msg, err := await(ctx, lines, time.Second); err != nil || msg != "ready" {
		return fmt.Errorf("waiting for ready: got %q: %v", msg, err)
	}
	fmt.Fprintln(w, "child is ready")

	fmt.Fprintln(stdin, "ping")
	msg, err := await(ctx, lines, time.Second)
	if err != nil {
		return fmt.Errorf("ping: %w", err)
	}
	fmt.Fprintln(w, "ping ->", msg)

	fmt.Fprintln(stdin, "hang")
	if _, err := await(ctx, lines, 200*time.Millisecond); err != nil {
		fmt.Fprintln(w, "hang ->", err)
		if err := cmd.Process.Kill(); err != nil {
			return fmt.Errorf("kill child: %w", err)
		}
	}
	_ = cmd.Wait() // reports "killed"; expected here
	for range lines {
	}
	fmt.Fprintln(w, "child killed and reaped")
	return nil
}

// await returns the next line, or an error if none arrives within timeout.
func await(ctx context.Context, lines <-chan string, timeout time.Duration) (string, error) {
	t := time.NewTimer(timeout)
	defer t.Stop()
	select {
	case msg, ok := <-lines:
		if !ok {
			return "", fmt.Errorf("child exited")
		}
		return msg, nil
	case <-t.C:
		return "", fmt.Errorf("no reply within %v", timeout)
	case <-ctx.Done():
		return "", ctx.Err()
	}
}
