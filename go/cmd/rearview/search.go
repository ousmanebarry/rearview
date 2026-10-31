package main

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"rearview/internal/clips"
	"rearview/internal/config"
	"rearview/internal/engine"
	"rearview/internal/search"
	"rearview/internal/store"
)

// textEmbedder turns queries into CLIP vectors with one engine process.
type textEmbedder struct {
	c *engine.Client
}

func newTextEmbedder(ctx context.Context, cfg config.Config) (*textEmbedder, error) {
	ecfg, err := engineConfig(ctx, cfg, io.Discard)
	if err != nil {
		return nil, err
	}
	ecfg.Args = append(ecfg.Args, "--lazy")
	c, err := engine.Start(ctx, ecfg)
	if err != nil {
		return nil, err
	}
	return &textEmbedder{c}, nil
}

func (e *textEmbedder) embed(ctx context.Context, text string) ([]float32, error) {
	var vec []float32
	_, err := e.c.Do(ctx, engine.Job{Cmd: "embed_text", Fields: map[string]any{"text": text}}, func(r engine.Reply) {
		if r.Type == "embedding" {
			vec = r.Vector
		}
	})
	if err == nil && len(vec) == 0 {
		err = errors.New("engine returned no embedding")
	}
	return vec, err
}

func (e *textEmbedder) Close() { e.c.Close() }

type searchHit struct {
	Rank      int     `json:"rank"`
	Time      string  `json:"time_utc"`
	Local     string  `json:"time_local"`
	TripID    int64   `json:"trip_id"`
	SegmentID int64   `json:"segment_id"`
	Path      string  `json:"path"`
	Camera    string  `json:"camera"`
	TOffsetS  float64 `json:"t_offset_s"`
	Score     float64 `json:"score"`
	Lat       float64 `json:"lat,omitempty"`
	Lon       float64 `json:"lon,omitempty"`
	Clip      string  `json:"clip,omitempty"`
}

func newSearchCmd() *cobra.Command {
	var (
		limit       int
		cut, asJSON bool
		crops       string
		raw         bool
	)
	cmd := &cobra.Command{
		Use:   `search "<query>" [--limit 10] [--clip] [--json]`,
		Short: "Find moments in your footage that match a plain-English description",
		Long: "Embed the query with CLIP's text model (wrapped in search.prompt_template), score every\n" +
			"stored keyframe (rearview process --embed), group hits within search.group_s seconds of\n" +
			"each other into moments, and print the best ones. --clip cuts a 10 s clip around each.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			cfg, db, err := openDB(ctx, cmd)
			if err != nil {
				return err
			}
			defer db.Close()
			start := time.Now()
			ix, err := search.Load(ctx, db, crops == "three")
			if err != nil {
				return err
			}
			if ix.Len() == 0 {
				return errors.New("no keyframes yet: run rearview process --embed first")
			}
			emb, err := newTextEmbedder(ctx, cfg)
			if err != nil {
				return err
			}
			defer emb.Close()
			loaded := time.Since(start)
			start = time.Now()
			text := args[0]
			if !raw {
				text = search.Prompt(cfg.Search.PromptTemplate, args[0])
			}
			q, err := emb.embed(ctx, text)
			if err != nil {
				return err
			}
			moments := ix.Search(q, cfg.Search.TopK, cfg.Search.GroupS)
			took := time.Since(start)
			if len(moments) > limit {
				moments = moments[:limit]
			}
			loc, err := time.LoadLocation(cfg.TimeZone)
			if err != nil {
				return err
			}
			var hits []searchHit
			for i, m := range moments {
				h := searchHit{Rank: i + 1, Time: store.FormatTime(m.At()), Local: m.At().In(loc).Format("2006-01-02 15:04:05"),
					TripID: m.Segment.TripID, SegmentID: m.Segment.ID, Path: m.Segment.Path, Camera: m.Segment.Camera,
					TOffsetS: m.TOffsetS, Score: math.Round(m.Score*1000) / 1000}
				if pts, err := db.GPS(ctx, m.Segment.ID); err == nil && len(pts) > 0 {
					best := pts[0]
					for _, p := range pts {
						if math.Abs(p.TOffsetS-m.TOffsetS) < math.Abs(best.TOffsetS-m.TOffsetS) {
							best = p
						}
					}
					h.Lat, h.Lon = best.Lat, best.Lon
				}
				if cut {
					part := clips.Part{Path: filepath.Join(cfg.LibraryPath, filepath.FromSlash(m.Segment.Path)),
						StartS: math.Max(0, m.TOffsetS-5), EndS: math.Min(m.Segment.DurationS, m.TOffsetS+5)}
					out := filepath.Join(cfg.ClipsPath, fmt.Sprintf("%s_search_%s_%d.mp4", m.At().In(loc).Format("20060102_150405"), slug(args[0]), i+1))
					if err := clips.Cut(ctx, []clips.Part{part}, out, clips.Options{}); err != nil {
						return err
					}
					h.Clip = out
				}
				hits = append(hits, h)
			}
			out := cmd.OutOrStdout()
			if asJSON {
				enc := json.NewEncoder(out)
				enc.SetIndent("", "  ")
				return enc.Encode(hits)
			}
			w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "#\tTIME\tTRIP\tSCORE\tCAMERA\tLOCATION\tFILE")
			for _, h := range hits {
				pos := "-"
				if h.Lat != 0 {
					pos = fmt.Sprintf("%.5f,%.5f", h.Lat, h.Lon)
				}
				file := fmt.Sprintf("%s@%.0fs", filepath.Base(h.Path), h.TOffsetS)
				if h.Clip != "" {
					file = h.Clip
				}
				fmt.Fprintf(w, "%d\t%s\t%d\t%.3f\t%s\t%s\t%s\n", h.Rank, h.Local, h.TripID, h.Score, h.Camera, pos, file)
			}
			w.Flush()
			fmt.Fprintf(out, "%d keyframes searched in %v (engine and index load %v)\n", ix.Len(), took.Round(time.Millisecond), loaded.Round(time.Millisecond))
			return nil
		},
	}
	cmd.Flags().IntVar(&limit, "limit", 10, "moments to show")
	cmd.Flags().BoolVar(&cut, "clip", false, "cut a 10 s clip around each result")
	cmd.Flags().BoolVar(&asJSON, "json", false, "print results as JSON")
	cmd.Flags().StringVar(&crops, "crops", "three", "keyframe crops to use: center or three (best of left, centre, right)")
	cmd.Flags().BoolVar(&raw, "raw", false, "use the query as is, without search.prompt_template")
	return cmd
}

var nonWord = regexp.MustCompile(`[^a-z0-9]+`)

func slug(s string) string {
	s = strings.Trim(nonWord.ReplaceAllString(strings.ToLower(s), "-"), "-")
	if len(s) > 40 {
		s = s[:40]
	}
	return s
}

// searchQuery is one row of testdata/search_queries.csv.
type searchQuery struct {
	Query    string
	From, To time.Time
}

func loadSearchQueries(path string, loc *time.Location) ([]searchQuery, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	r := csv.NewReader(f)
	r.Comment = '#'
	rows, err := r.ReadAll()
	if err != nil {
		return nil, err
	}
	var out []searchQuery
	for i, row := range rows {
		if i == 0 || len(row) < 3 {
			continue
		}
		from, err1 := time.ParseInLocation("2006-01-02 15:04:05", strings.TrimSpace(row[1]), loc)
		to, err2 := time.ParseInLocation("2006-01-02 15:04:05", strings.TrimSpace(row[2]), loc)
		if err := errors.Join(err1, err2); err != nil {
			return nil, fmt.Errorf("%s line %d: %w", path, i+1, err)
		}
		out = append(out, searchQuery{Query: strings.TrimSpace(row[0]), From: from, To: to})
	}
	return out, nil
}

// runSearchEval scores search against the labelled queries: the share whose
// expected time range contains one of the top 5 moments, for centre-crop vs
// three-crop keyframes and prompt template vs plain query.
func runSearchEval(cmd *cobra.Command, file string) error {
	ctx := cmd.Context()
	cfg, db, err := openDB(ctx, cmd)
	if err != nil {
		return err
	}
	defer db.Close()
	loc, err := time.LoadLocation(cfg.TimeZone)
	if err != nil {
		return err
	}
	if file == "" {
		root, err := findRepoRoot()
		if err != nil {
			return err
		}
		file = filepath.Join(root, "testdata", "search_queries.csv")
	}
	qs, err := loadSearchQueries(file, loc)
	if err != nil {
		return err
	}
	centre, err := search.Load(ctx, db, false)
	if err != nil {
		return err
	}
	three, err := search.Load(ctx, db, true)
	if err != nil {
		return err
	}
	emb, err := newTextEmbedder(ctx, cfg)
	if err != nil {
		return err
	}
	defer emb.Close()
	type variant struct {
		name     string
		ix       *search.Index
		template bool
		hits     int
	}
	vs := []*variant{{"centre crop, template", centre, true, 0}, {"centre crop, plain", centre, false, 0},
		{"three crops, template", three, true, 0}, {"three crops, plain", three, false, 0}}
	out := cmd.OutOrStdout()
	cache := map[string][]float32{}
	for _, q := range qs {
		line := fmt.Sprintf("%-32s", q.Query)
		for _, v := range vs {
			text := q.Query
			if v.template {
				text = search.Prompt(cfg.Search.PromptTemplate, q.Query)
			}
			vec, ok := cache[text]
			if !ok {
				if vec, err = emb.embed(ctx, text); err != nil {
					return err
				}
				cache[text] = vec
			}
			ms := v.ix.Search(vec, cfg.Search.TopK, cfg.Search.GroupS)
			found := "-"
			for i, m := range ms {
				if i >= 5 {
					break
				}
				from := m.Segment.StartedAt.Add(time.Duration(m.FromS * float64(time.Second)))
				to := m.Segment.StartedAt.Add(time.Duration(m.ToS * float64(time.Second)))
				if !to.Before(q.From) && !from.After(q.To) {
					found = fmt.Sprint(i + 1)
					v.hits++
					break
				}
			}
			line += fmt.Sprintf("  %3s", found)
		}
		fmt.Fprintln(out, line)
	}
	fmt.Fprintf(out, "\n%d queries; correct moment in the top 5:\n", len(qs))
	for _, v := range vs {
		fmt.Fprintf(out, "  %-24s %d/%d = %.0f%%\n", v.name, v.hits, len(qs), 100*float64(v.hits)/float64(max(1, len(qs))))
	}
	return nil
}
