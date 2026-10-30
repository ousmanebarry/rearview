// Package eval scores detected events against hand labels.
package eval

import (
	"math"
	"sort"
	"time"
)

// Item is one detected event or label.
type Item struct {
	ID   int64
	TS   time.Time
	Type string
	Note string
}

// Result of matching detections with labels of one type.
type Result struct {
	Matched     [][2]Item // detection, label
	FalseAlarms []Item
	Misses      []Item
}

// Precision is matched detections over all detections (1 when there are none).
func (r Result) Precision() float64 {
	n := len(r.Matched) + len(r.FalseAlarms)
	if n == 0 {
		return 1
	}
	return float64(len(r.Matched)) / float64(n)
}

// Recall is matched labels over all labels (1 when there are none).
func (r Result) Recall() float64 {
	n := len(r.Matched) + len(r.Misses)
	if n == 0 {
		return 1
	}
	return float64(len(r.Matched)) / float64(n)
}

// Match pairs detections and labels of the same type within tol, one to one,
// closest pairs first.
func Match(dets, labels []Item, tol time.Duration) Result {
	type cand struct {
		d, l int
		gap  time.Duration
	}
	var cs []cand
	for i, d := range dets {
		for j, l := range labels {
			if d.Type != l.Type {
				continue
			}
			g := d.TS.Sub(l.TS)
			if g < 0 {
				g = -g
			}
			if g <= tol {
				cs = append(cs, cand{i, j, g})
			}
		}
	}
	sort.SliceStable(cs, func(a, b int) bool { return cs[a].gap < cs[b].gap })
	usedD := make([]bool, len(dets))
	usedL := make([]bool, len(labels))
	var r Result
	for _, c := range cs {
		if usedD[c.d] || usedL[c.l] {
			continue
		}
		usedD[c.d], usedL[c.l] = true, true
		r.Matched = append(r.Matched, [2]Item{dets[c.d], labels[c.l]})
	}
	for i, d := range dets {
		if !usedD[i] {
			r.FalseAlarms = append(r.FalseAlarms, d)
		}
	}
	for j, l := range labels {
		if !usedL[j] {
			r.Misses = append(r.Misses, l)
		}
	}
	sort.Slice(r.Matched, func(a, b int) bool { return r.Matched[a][1].TS.Before(r.Matched[b][1].TS) })
	return r
}

// Round3 rounds a ratio for printing.
func Round3(v float64) float64 { return math.Round(v*1000) / 1000 }
