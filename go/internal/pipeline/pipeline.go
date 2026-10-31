// Package pipeline runs engine jobs in parallel: a planner feeds a bounded
// jobs channel, N workers each drive one engine process, and a single writer
// goroutine saves results, so SQLite only ever has one writer.
package pipeline

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"

	"rearview/internal/engine"
	"rearview/internal/queue"
	"rearview/internal/store"
)

// Options for one run.
type Options struct {
	Workers  int
	Engine   engine.Config // copied per worker
	Library  string
	Kind     string // analyze | embed_frames
	FPS      float64
	Features []string
	// EmbedEveryS is the keyframe spacing for embed_frames jobs.
	EmbedEveryS float64
	// EmbedCrops is "center" or "three" (left, centre, right).
	EmbedCrops string
	// FPSFor, when set, picks the analysis rate per job (Phase 6 scheduling).
	FPSFor func(store.Job, store.Segment) float64
	// AnalyzeExtra adds fields to every analyze command (threaded, idle_fps, decoder).
	AnalyzeExtra map[string]any
	// Log receives one line per finished job (nil = silent).
	Log io.Writer
	// FlushEvery is how many observations the writer buffers before a commit.
	FlushEvery int
	// OnStats, when set, is called once at the start with a function that
	// returns a live snapshot (for progress views).
	OnStats func(func() Stats)
}

type msgKind int

const (
	msgStarted msgKind = iota
	msgReply
	msgReset
	msgDone
	msgFailed
	msgInterrupted
)

type result struct {
	kind   msgKind
	worker int
	job    store.Job
	seg    store.Segment
	reply  engine.Reply
	err    error
}

// Run processes every pending job of opt.Kind. Jobs left running by a crash
// are reset to pending first. Cancelling ctx stops the run; interrupted jobs
// go back to pending with their partial results deleted.
func Run(ctx context.Context, db *store.DB, opt Options) (Stats, error) {
	if opt.Workers < 1 {
		opt.Workers = 1
	}
	if opt.Kind == "" {
		opt.Kind = "analyze"
	}
	if opt.FlushEvery == 0 {
		opt.FlushEvery = 2000
	}
	if _, err := db.ResetRunning(ctx); err != nil {
		return Stats{}, fmt.Errorf("reset interrupted jobs: %w", err)
	}
	pending, err := db.Jobs(ctx, store.JobPending, opt.Kind)
	if err != nil {
		return Stats{}, err
	}
	st := newStats(opt.Workers, len(pending))
	q := queue.New()
	q.Push(pending...)
	q.Close()
	st.queue = q
	if opt.OnStats != nil {
		opt.OnStats(st.Snapshot)
	}

	g, gctx := errgroup.WithContext(ctx)
	jobs := make(chan store.Job, opt.Workers)
	results := make(chan result, 256)

	g.Go(func() error {
		defer close(jobs)
		for {
			j, ok := q.Pop(gctx)
			if !ok {
				return nil
			}
			select {
			case jobs <- j:
			case <-gctx.Done():
				return nil
			}
		}
	})

	var wg sync.WaitGroup
	for i := range opt.Workers {
		wg.Add(1)
		g.Go(func() error {
			defer wg.Done()
			return work(gctx, db, opt, i, jobs, results)
		})
	}
	go func() {
		wg.Wait()
		close(results)
	}()

	g.Go(func() error { return write(db, opt, st, results) })

	err = g.Wait()
	if errors.Is(err, context.Canceled) || ctx.Err() != nil {
		return st.Snapshot(), ctx.Err()
	}
	return st.Snapshot(), err
}

func work(ctx context.Context, db *store.DB, opt Options, id int, jobs <-chan store.Job, results chan<- result) error {
	ecfg := opt.Engine
	if ecfg.Stderr != nil {
		ecfg.Stderr = &prefixWriter{w: ecfg.Stderr, prefix: fmt.Sprintf("[worker %d] ", id)}
	}
	w := engine.NewWorker(ecfg)
	defer w.Close()
	for j := range jobs {
		seg, err := db.Segment(ctx, j.SegmentID)
		if err != nil {
			if ctx.Err() != nil {
				results <- result{kind: msgInterrupted, worker: id, job: j}
				continue
			}
			results <- result{kind: msgFailed, worker: id, job: j, err: err}
			continue
		}
		results <- result{kind: msgStarted, worker: id, job: j, seg: seg}
		ej := buildJob(opt, j, seg)
		done, err := w.Do(ctx, ej,
			func(r engine.Reply) { results <- result{kind: msgReply, worker: id, job: j, seg: seg, reply: r} },
			func() { results <- result{kind: msgReset, worker: id, job: j} })
		switch {
		case ctx.Err() != nil:
			results <- result{kind: msgInterrupted, worker: id, job: j}
		case err != nil:
			results <- result{kind: msgFailed, worker: id, job: j, err: err}
		default:
			results <- result{kind: msgDone, worker: id, job: j, reply: done}
		}
	}
	return nil
}

func buildJob(opt Options, j store.Job, seg store.Segment) engine.Job {
	path := filepath.Join(opt.Library, filepath.FromSlash(seg.Path))
	switch j.Kind {
	case "embed_frames":
		every := opt.EmbedEveryS
		if every <= 0 {
			every = 1
		}
		return engine.Job{Cmd: "embed_frames", Fields: map[string]any{"path": path, "every_sec": every, "crops": crops(opt.EmbedCrops)}}
	default:
		fps := opt.FPS
		if opt.FPSFor != nil {
			fps = opt.FPSFor(j, seg)
		}
		j := engine.Analyze(path, seg.Camera, j.Windows, fps, opt.Features)
		for k, v := range opt.AnalyzeExtra {
			j.Fields[k] = v
		}
		return j
	}
}

// write is the only goroutine that writes to the database during a run. It
// uses a context that is never cancelled so an interrupted run still leaves
// the job table consistent.
func write(db *store.DB, opt Options, st *Stats, results <-chan result) error {
	ctx := context.Background()
	buf := map[int64][]store.Observation{}
	kfs := map[int64][]store.Keyframe{}
	flush := func(jobID int64) error {
		if err := db.InsertObservations(ctx, buf[jobID]); err != nil {
			return err
		}
		delete(buf, jobID)
		if err := db.InsertKeyframes(ctx, kfs[jobID]); err != nil {
			return err
		}
		delete(kfs, jobID)
		return nil
	}
	var firstErr error
	keep := func(err error) {
		if err != nil && firstErr == nil {
			firstErr = err
		}
	}
	for r := range results {
		if firstErr != nil {
			continue // drain so workers never block
		}
		switch r.kind {
		case msgStarted:
			keep(db.StartJob(ctx, r.job.ID))
			st.started(r.worker, r.job)
		case msgReply:
			switch r.reply.Type {
			case "frame":
				for _, o := range r.reply.Objects {
					buf[r.job.ID] = append(buf[r.job.ID], store.Observation{
						JobID: r.job.ID, SegmentID: r.job.SegmentID, TOffsetS: deref(r.reply.T),
						TrackID: o.Track, Cls: o.Cls, X1: o.Box[0], Y1: o.Box[1], X2: o.Box[2], Y2: o.Box[3],
						Conf: o.Conf, DistM: o.DistM, TTCS: o.TTCS,
					})
				}
				st.frame(r.worker, len(r.reply.Objects))
				if len(buf[r.job.ID]) >= opt.FlushEvery {
					keep(flush(r.job.ID))
				}
			case "embedding":
				model := ""
				if r.reply.Crop != "" && r.reply.Crop != "center" {
					model = store.KeyframeModel + "/" + r.reply.Crop
				}
				kfs[r.job.ID] = append(kfs[r.job.ID], store.Keyframe{
					JobID: r.job.ID, SegmentID: r.job.SegmentID, TOffsetS: deref(r.reply.T), Model: model, Vector: r.reply.Vector,
				})
				if model == "" {
					st.frame(r.worker, 0)
				}
			case "progress":
				st.progress(r.worker, r.reply.FramesDone)
			}
		case msgReset:
			delete(buf, r.job.ID)
			delete(kfs, r.job.ID)
			keep(db.DeleteObservations(ctx, r.job.ID))
			keep(db.DeleteKeyframes(ctx, r.job.ID))
			st.restarted(r.worker)
		case msgDone:
			keep(flush(r.job.ID))
			keep(db.FinishJob(ctx, r.job.ID, r.reply.FPS, ""))
			st.finished(r.worker, r.reply, nil)
			logf(opt.Log, "job %d segment %d done: %d frames at %.1f fps\n", r.job.ID, r.job.SegmentID, r.reply.Frames, r.reply.FPS)
		case msgFailed:
			delete(buf, r.job.ID)
			delete(kfs, r.job.ID)
			keep(db.DeleteObservations(ctx, r.job.ID))
			keep(db.DeleteKeyframes(ctx, r.job.ID))
			keep(db.FinishJob(ctx, r.job.ID, 0, r.err.Error()))
			st.finished(r.worker, engine.Reply{}, r.err)
			logf(opt.Log, "job %d segment %d failed: %v\n", r.job.ID, r.job.SegmentID, r.err)
		case msgInterrupted:
			delete(buf, r.job.ID)
			delete(kfs, r.job.ID)
			keep(db.RequeueJob(ctx, r.job.ID))
			st.interrupted(r.worker)
		}
	}
	return firstErr
}

func crops(s string) string {
	if s == "" {
		return "center"
	}
	return s
}

func deref(p *float64) float64 {
	if p == nil {
		return 0
	}
	return *p
}

func logf(w io.Writer, format string, args ...any) {
	if w != nil {
		fmt.Fprintf(w, time.Now().Format("15:04:05 ")+format, args...)
	}
}

// prefixWriter prefixes each line of engine stderr with the worker id.
type prefixWriter struct {
	mu     sync.Mutex
	w      io.Writer
	prefix string
	mid    bool
}

func (p *prefixWriter) Write(b []byte) (int, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, c := range b {
		if !p.mid {
			if _, err := io.WriteString(p.w, p.prefix); err != nil {
				return 0, err
			}
			p.mid = true
		}
		if _, err := p.w.Write([]byte{c}); err != nil {
			return 0, err
		}
		if c == '\n' {
			p.mid = false
		}
	}
	return len(b), nil
}
