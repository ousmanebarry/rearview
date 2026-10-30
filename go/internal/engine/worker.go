package engine

import (
	"context"
	"errors"
	"fmt"
)

// Worker owns one engine process for its lifetime, starting it on first use
// and restarting it when it dies or hangs.
type Worker struct {
	cfg      Config
	c        *Client
	Restarts int
}

// NewWorker returns a worker; the engine starts on the first job.
func NewWorker(cfg Config) *Worker { return &Worker{cfg: cfg} }

func (w *Worker) client(ctx context.Context) (*Client, error) {
	if w.c != nil && w.c.Alive() {
		return w.c, nil
	}
	if w.c != nil {
		w.c.Kill()
		w.Restarts++
	}
	c, err := Start(ctx, w.cfg)
	if err != nil {
		w.c = nil
		return nil, err
	}
	w.c = c
	return c, nil
}

// Do runs a job. If the engine dies, hangs, or fails with a retryable error,
// the process is restarted, onReset is called (so partial results can be
// thrown away) and the job is retried once.
func (w *Worker) Do(ctx context.Context, j Job, onReply func(Reply), onReset func()) (Reply, error) {
	var lastErr error
	for attempt := 0; attempt < 2; attempt++ {
		if attempt > 0 {
			if w.c != nil {
				w.c.Kill()
			}
			if onReset != nil {
				onReset()
			}
		}
		c, err := w.client(ctx)
		if err != nil {
			if errors.Is(err, ErrVersion) || ctx.Err() != nil {
				return Reply{}, err
			}
			lastErr = err
			continue
		}
		r, err := c.Do(ctx, j, onReply)
		if err == nil || ctx.Err() != nil || !IsRetryable(err) {
			return r, err
		}
		lastErr = err
	}
	return Reply{}, fmt.Errorf("after retry: %w", lastErr)
}

// Close shuts the engine down.
func (w *Worker) Close() error {
	if w.c == nil {
		return nil
	}
	err := w.c.Close()
	w.c = nil
	return err
}

// Pid is the running engine's process id, or 0.
func (w *Worker) Pid() int {
	if w.c == nil || !w.c.Alive() {
		return 0
	}
	return w.c.Pid()
}
