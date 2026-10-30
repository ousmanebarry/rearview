package engine

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"sync"
	"time"
)

// Config describes how to start an engine process.
type Config struct {
	Path string   // engine binary
	Args []string // e.g. serve --models dir --device cpu
	// JobTimeout is the longest silence allowed while a job runs; the engine
	// sends progress about once a second, so this can be short.
	JobTimeout   time.Duration
	ReadyTimeout time.Duration
	// Stderr receives the engine's log lines, prefixed by the caller.
	Stderr io.Writer
}

// Client is one running engine process. It runs one job at a time.
type Client struct {
	cfg    Config
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	lines  chan []byte // stdout lines; closed when stdout closes
	exited chan struct{}
	stop   chan struct{} // closed by Kill so the reader never blocks
	stopMu sync.Once
	ready  Reply

	mu     sync.Mutex // guards writes to stdin and nextID
	nextID int64
}

// Start launches the engine and waits for `ready` with the right protocol version.
func Start(ctx context.Context, cfg Config) (*Client, error) {
	if cfg.JobTimeout == 0 {
		cfg.JobTimeout = 60 * time.Second
	}
	if cfg.ReadyTimeout == 0 {
		cfg.ReadyTimeout = 2 * time.Minute // model loading; MIGraphX compiles on first run
	}
	// Not CommandContext: Close decides how the process ends.
	cmd := exec.Command(cfg.Path, cfg.Args...)
	cmd.Stderr = cfg.Stderr
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start engine %s: %w", cfg.Path, err)
	}
	c := &Client{cfg: cfg, cmd: cmd, stdin: stdin, lines: make(chan []byte, 256),
		exited: make(chan struct{}), stop: make(chan struct{})}
	readDone := make(chan struct{})
	go func() {
		c.readLoop(stdout)
		close(readDone)
	}()
	go func() {
		// Wait closes the stdout pipe, so it must only run once reading is finished.
		<-readDone
		_ = cmd.Wait()
		close(c.exited)
	}()

	timer := time.NewTimer(cfg.ReadyTimeout)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			c.Kill()
			return nil, ctx.Err()
		case <-timer.C:
			c.Kill()
			return nil, fmt.Errorf("engine not ready after %v: %w", cfg.ReadyTimeout, ErrHung)
		case line, ok := <-c.lines:
			if !ok {
				c.Kill()
				return nil, fmt.Errorf("engine exited before ready: %w", ErrDied)
			}
			var r Reply
			if err := json.Unmarshal(line, &r); err != nil || r.Type != "ready" {
				continue // not ours; the spec says ready comes first, so tolerate noise only
			}
			if r.ProtocolVersion != ProtocolVersion {
				c.Kill()
				return nil, fmt.Errorf("%w: engine speaks %d, rearview %d", ErrVersion, r.ProtocolVersion, ProtocolVersion)
			}
			c.ready = r
			return c, nil
		}
	}
}

// Ready is the engine's ready message.
func (c *Client) Ready() Reply { return c.ready }

// Pid is the engine's process id.
func (c *Client) Pid() int { return c.cmd.Process.Pid }

func (c *Client) readLoop(stdout io.Reader) {
	defer close(c.lines)
	sc := bufio.NewScanner(stdout)
	sc.Buffer(make([]byte, 64*1024), 16<<20)
	for sc.Scan() {
		b := append([]byte(nil), sc.Bytes()...)
		select {
		case c.lines <- b:
		case <-c.stop: // nobody will read it; keep draining until the pipe closes
		}
	}
}

func (c *Client) send(j Job) (int64, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.nextID++
	b, err := j.encode(c.nextID)
	if err != nil {
		return 0, err
	}
	if _, err := c.stdin.Write(b); err != nil {
		return 0, fmt.Errorf("write to engine: %w", ErrDied)
	}
	return c.nextID, nil
}

// Run sends a job and streams its replies. The channel is closed after the
// final reply (`done` or `error`); a process death, a hang or a cancel ends it
// with a synthetic `error` reply whose Code is "died", "hung" or "cancelled".
// Cancelling ctx sends `cancel` and kills the process if the job hasn't
// finished within 2 seconds.
func (c *Client) Run(ctx context.Context, j Job) (<-chan Reply, error) {
	id, err := c.send(j)
	if err != nil {
		return nil, err
	}
	out := make(chan Reply, 64)
	go c.pump(ctx, id, out)
	return out, nil
}

func (c *Client) pump(ctx context.Context, id int64, out chan<- Reply) {
	defer close(out)
	timer := time.NewTimer(c.cfg.JobTimeout)
	defer timer.Stop()
	done := ctx.Done()
	var killAt <-chan time.Time
	for {
		select {
		case <-done:
			done = nil
			_, _ = c.send(Job{Cmd: "cancel", Fields: map[string]any{"target_id": id}})
			killAt = time.After(2 * time.Second)
		case <-killAt:
			c.Kill()
			out <- Reply{ID: id, Type: "error", Code: "cancelled", Msg: "engine did not stop within 2 s; killed"}
			return
		case <-timer.C:
			c.Kill()
			out <- Reply{ID: id, Type: "error", Code: "hung", Msg: fmt.Sprintf("no message for %v", c.cfg.JobTimeout)}
			return
		case line, ok := <-c.lines:
			if !ok {
				out <- Reply{ID: id, Type: "error", Code: "died", Msg: "engine closed stdout"}
				return
			}
			timer.Reset(c.cfg.JobTimeout)
			var r Reply
			if err := json.Unmarshal(line, &r); err != nil || r.ID != id {
				continue // stray line or a reply to an older job
			}
			out <- r
			if r.Final() {
				return
			}
		}
	}
}

// Do runs a job to completion, calling onReply for every intermediate reply,
// and returns the `done` reply or an error (*EngineError, ErrDied, ErrHung,
// or ctx.Err()).
func (c *Client) Do(ctx context.Context, j Job, onReply func(Reply)) (Reply, error) {
	ch, err := c.Run(ctx, j)
	if err != nil {
		return Reply{}, err
	}
	var last Reply
	for r := range ch {
		if !r.Final() {
			if onReply != nil {
				onReply(r)
			}
			continue
		}
		last = r
	}
	switch {
	case last.Type == "done" || last.Type == "pong":
		if last.Cancelled && ctx.Err() != nil {
			return last, ctx.Err()
		}
		return last, nil
	case last.Code == "died":
		return last, ErrDied
	case last.Code == "hung":
		return last, ErrHung
	case last.Code == "cancelled":
		return last, ctx.Err()
	case last.Type == "error":
		return last, &EngineError{Code: last.Code, Msg: last.Msg, T: last.T}
	}
	return last, ErrDied
}

// Ping checks the engine answers.
func (c *Client) Ping(ctx context.Context) error {
	_, err := c.Do(ctx, Job{Cmd: "ping"}, nil)
	return err
}

// Alive reports whether the process is still running.
func (c *Client) Alive() bool {
	select {
	case <-c.exited:
		return false
	default:
		return true
	}
}

// Close sends shutdown and waits up to 2 seconds before killing the process.
func (c *Client) Close() error {
	if !c.Alive() {
		return nil
	}
	_, _ = c.send(Job{Cmd: "shutdown"})
	_ = c.stdin.Close()
	c.stopMu.Do(func() { close(c.stop) })
	select {
	case <-c.exited:
		return nil
	case <-time.After(2 * time.Second):
		c.Kill()
		return errors.New("engine did not exit within 2 s; killed")
	}
}

// Kill ends the process immediately.
func (c *Client) Kill() {
	c.stopMu.Do(func() { close(c.stop) })
	if c.cmd.Process != nil {
		_ = c.cmd.Process.Kill()
	}
	<-c.exited
}
