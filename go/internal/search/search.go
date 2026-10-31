// Package search ranks stored CLIP keyframe vectors against a text query and
// groups the hits into moments.
package search

import (
	"context"
	"sort"
	"strings"
	"time"

	"rearview/internal/store"
)

// Index holds every keyframe vector in memory (a week of footage at one
// keyframe per second is well under 100 MB).
type Index struct {
	keys []key
	vecs [][]float32
	segs map[int64]store.Segment
}

type key struct {
	seg int64
	t   float64
}

// Load reads the keyframes. With allCrops, the left/centre/right crops of a
// keyframe are kept and the best of them counts.
func Load(ctx context.Context, db *store.DB, allCrops bool) (*Index, error) {
	kfs, err := db.Keyframes(ctx, allCrops)
	if err != nil {
		return nil, err
	}
	segs, err := db.Segments(ctx, 0)
	if err != nil {
		return nil, err
	}
	ix := &Index{segs: map[int64]store.Segment{}}
	for _, s := range segs {
		ix.segs[s.ID] = s
	}
	for _, k := range kfs {
		ix.keys = append(ix.keys, key{k.SegmentID, k.TOffsetS})
		ix.vecs = append(ix.vecs, k.Vector)
	}
	return ix, nil
}

// Len is the number of stored vectors.
func (ix *Index) Len() int { return len(ix.vecs) }

// Moment is a stretch of one file matching the query.
type Moment struct {
	Segment  store.Segment
	TOffsetS float64 // best hit
	FromS    float64
	ToS      float64
	Score    float64
	Hits     int
}

// At is the moment's absolute time.
func (m Moment) At() time.Time {
	return m.Segment.StartedAt.Add(time.Duration(m.TOffsetS * float64(time.Second)))
}

func dot(a, b []float32) float64 {
	var s float32
	for i := range a {
		if i < len(b) {
			s += a[i] * b[i]
		}
	}
	return float64(s)
}

// Search scores every keyframe by dot product with q (both unit vectors, so
// this is cosine similarity), keeps the topK best keyframes, groups hits in
// the same file within groupS seconds into one moment, and returns the
// moments best first.
func (ix *Index) Search(q []float32, topK int, groupS float64) []Moment {
	best := map[key]float64{}
	for i, v := range ix.vecs {
		s := dot(q, v)
		if old, ok := best[ix.keys[i]]; !ok || s > old {
			best[ix.keys[i]] = s
		}
	}
	type hit struct {
		k key
		s float64
	}
	hits := make([]hit, 0, len(best))
	for k, s := range best {
		hits = append(hits, hit{k, s})
	}
	sort.Slice(hits, func(i, j int) bool {
		if hits[i].s != hits[j].s {
			return hits[i].s > hits[j].s
		}
		if hits[i].k.seg != hits[j].k.seg {
			return hits[i].k.seg < hits[j].k.seg
		}
		return hits[i].k.t < hits[j].k.t
	})
	if len(hits) > topK {
		hits = hits[:topK]
	}
	// Group by segment in time order; a gap over groupS starts a new moment.
	sort.Slice(hits, func(i, j int) bool {
		if hits[i].k.seg != hits[j].k.seg {
			return hits[i].k.seg < hits[j].k.seg
		}
		return hits[i].k.t < hits[j].k.t
	})
	var out []Moment
	for _, h := range hits {
		n := len(out)
		if n > 0 && out[n-1].Segment.ID == h.k.seg && h.k.t-out[n-1].ToS <= groupS {
			m := &out[n-1]
			m.ToS = h.k.t
			m.Hits++
			if h.s > m.Score {
				m.Score, m.TOffsetS = h.s, h.k.t
			}
			continue
		}
		out = append(out, Moment{Segment: ix.segs[h.k.seg], TOffsetS: h.k.t, FromS: h.k.t, ToS: h.k.t, Score: h.s, Hits: 1})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Score > out[j].Score })
	return out
}

// Prompt fills the template's {query} placeholder.
func Prompt(template, query string) string {
	if template == "" || !strings.Contains(template, "{query}") {
		return query
	}
	return strings.ReplaceAll(template, "{query}", query)
}
