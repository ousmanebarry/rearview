// Package queue holds the job queue: a priority queue workers pull from, and
// the planner that turns segments and motion events into jobs.
package queue

import (
	"container/heap"
	"context"
	"sync"

	"rearview/internal/store"
)

// Queue is a priority queue of jobs: lowest Priority first, then lowest id.
// Pop blocks until a job is available, the queue is closed, or ctx ends.
type Queue struct {
	mu     sync.Mutex
	h      jobHeap
	closed bool
	notify chan struct{} // closed and replaced whenever the queue changes
}

// New returns an empty queue.
func New() *Queue { return &Queue{notify: make(chan struct{})} }

func (q *Queue) changed() {
	close(q.notify)
	q.notify = make(chan struct{})
}

// Push adds jobs.
func (q *Queue) Push(jobs ...store.Job) {
	q.mu.Lock()
	defer q.mu.Unlock()
	for _, j := range jobs {
		heap.Push(&q.h, j)
	}
	q.changed()
}

// Close wakes every Pop; once empty, Pop returns ok=false.
func (q *Queue) Close() {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.closed = true
	q.changed()
}

// Len is the number of queued jobs.
func (q *Queue) Len() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.h.Len()
}

// Pop returns the highest-priority job.
func (q *Queue) Pop(ctx context.Context) (store.Job, bool) {
	for {
		q.mu.Lock()
		if q.h.Len() > 0 {
			j := heap.Pop(&q.h).(store.Job)
			q.mu.Unlock()
			return j, true
		}
		if q.closed {
			q.mu.Unlock()
			return store.Job{}, false
		}
		wait := q.notify
		q.mu.Unlock()
		select {
		case <-ctx.Done():
			return store.Job{}, false
		case <-wait:
		}
	}
}

type jobHeap []store.Job

func (h jobHeap) Len() int { return len(h) }
func (h jobHeap) Less(i, j int) bool {
	if h[i].Priority != h[j].Priority {
		return h[i].Priority < h[j].Priority
	}
	return h[i].ID < h[j].ID
}
func (h jobHeap) Swap(i, j int) { h[i], h[j] = h[j], h[i] }
func (h *jobHeap) Push(x any)   { *h = append(*h, x.(store.Job)) }
func (h *jobHeap) Pop() any {
	old := *h
	j := old[len(old)-1]
	*h = old[:len(old)-1]
	return j
}
