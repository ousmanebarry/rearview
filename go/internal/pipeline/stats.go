package pipeline

import (
	"sync"
	"time"

	"rearview/internal/engine"
	"rearview/internal/queue"
	"rearview/internal/store"
)

// WorkerStats is one worker's live state.
type WorkerStats struct {
	JobID    int64
	Busy     bool
	Frames   int64
	FPS      float64 // analysed frames per second over its current job
	Restarts int
	started  time.Time
	jobFrame int64
}

// Stats is a snapshot of a run, safe to read from another goroutine.
type Stats struct {
	Total       int
	Done        int
	Failed      int
	Interrupted int
	QueueDepth  int
	Frames      int64
	Objects     int64
	Workers     []WorkerStats
	Started     time.Time
	Elapsed     time.Duration
	// StagesMs sums the engine's per-stage times over finished jobs.
	StagesMs map[string]float64
	Errors   []string

	mu    *sync.Mutex
	queue *queue.Queue
}

func newStats(workers, total int) *Stats {
	return &Stats{Total: total, Workers: make([]WorkerStats, workers), Started: time.Now(),
		StagesMs: map[string]float64{}, mu: &sync.Mutex{}}
}

// Snapshot copies the stats.
func (s *Stats) Snapshot() Stats {
	s.mu.Lock()
	defer s.mu.Unlock()
	c := *s
	c.Workers = append([]WorkerStats(nil), s.Workers...)
	c.StagesMs = map[string]float64{}
	for k, v := range s.StagesMs {
		c.StagesMs[k] = v
	}
	c.Errors = append([]string(nil), s.Errors...)
	c.Elapsed = time.Since(s.Started)
	if s.queue != nil {
		c.QueueDepth = s.queue.Len()
	}
	return c
}

// ETA estimates the remaining time from the average job time so far.
func (s Stats) ETA() time.Duration {
	finished := s.Done + s.Failed
	if finished == 0 {
		return 0
	}
	left := s.Total - finished
	return time.Duration(float64(s.Elapsed) / float64(finished) * float64(left))
}

func (s *Stats) started(w int, j store.Job) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Workers[w].JobID, s.Workers[w].Busy = j.ID, true
	s.Workers[w].started, s.Workers[w].jobFrame = time.Now(), 0
}

func (s *Stats) frame(w int, objects int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Frames++
	s.Objects += int64(objects)
	ws := &s.Workers[w]
	ws.Frames++
	ws.jobFrame++
	if el := time.Since(ws.started).Seconds(); el > 0 {
		ws.FPS = float64(ws.jobFrame) / el
	}
}

func (s *Stats) progress(w int, framesDone int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ws := &s.Workers[w]
	if el := time.Since(ws.started).Seconds(); el > 0 && framesDone > 0 {
		ws.FPS = float64(framesDone) / el
	}
}

func (s *Stats) restarted(w int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Workers[w].Restarts++
}

func (s *Stats) finished(w int, r engine.Reply, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Workers[w].Busy = false
	if err != nil {
		s.Failed++
		if len(s.Errors) < 20 {
			s.Errors = append(s.Errors, err.Error())
		}
		return
	}
	s.Done++
	for k, v := range r.StagesMs {
		s.StagesMs[k] += v
	}
}

func (s *Stats) interrupted(w int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Workers[w].Busy = false
	s.Interrupted++
}
