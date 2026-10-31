package main

import (
	"context"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"rearview/internal/config"
	"rearview/internal/engine"
	"rearview/internal/report"
	"rearview/internal/store"
	"rearview/internal/trips"
)

// lazyEngine starts an engine that loads its models only when needed.
func lazyEngine(ctx context.Context, cfg config.Config) (*engine.Client, error) {
	ecfg, err := engineConfig(cfg, os.Stderr)
	if err != nil {
		return nil, err
	}
	ecfg.Args = append(ecfg.Args, "--lazy")
	return engine.Start(ctx, ecfg)
}

func newReportCmd() *cobra.Command {
	var out string
	cmd := &cobra.Command{
		Use:   "report <event id> [--out report.pdf]",
		Short: "Write a one-page incident PDF: key frames, speed and G graph, location, details",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			id, err := strconv.ParseInt(args[0], 10, 64)
			if err != nil {
				return fmt.Errorf("event id %q: %w", args[0], err)
			}
			cfg, db, err := openDB(ctx, cmd)
			if err != nil {
				return err
			}
			defer db.Close()
			e, err := db.Event(ctx, id)
			if err != nil {
				return err
			}
			seg, err := db.Segment(ctx, e.SegmentID)
			if err != nil {
				return err
			}
			loc, err := time.LoadLocation(cfg.TimeZone)
			if err != nil {
				return err
			}
			clipPath, err := clipEvent(ctx, db, cfg, id, "", false, false)
			if err != nil {
				return fmt.Errorf("clip: %w", err)
			}
			tmp, err := os.MkdirTemp("", "rearview-report")
			if err != nil {
				return err
			}
			defer os.RemoveAll(tmp)
			peak := e.TS.Sub(seg.StartedAt).Seconds()
			times := []float64{math.Max(0, peak-2), peak, math.Min(seg.DurationS-0.1, peak+2)}
			eng, err := lazyEngine(ctx, cfg)
			if err != nil {
				return err
			}
			done, err := eng.Do(ctx, engine.Job{Cmd: "extract_frames", Fields: map[string]any{
				"path": filepath.Join(cfg.LibraryPath, filepath.FromSlash(seg.Path)), "times": times, "out_dir": tmp,
			}}, nil)
			eng.Close()
			if err != nil {
				return fmt.Errorf("extract frames: %w", err)
			}
			var frames []report.Frame
			for i, f := range done.Files {
				frames = append(frames, report.Frame{Path: f, TOffsetS: times[i]})
			}
			track, _, err := trips.Track(ctx, db, e.TripID)
			if err != nil {
				return err
			}
			if out == "" {
				out = filepath.Join(cfg.ClipsPath, strings.TrimSuffix(filepath.Base(clipPath), ".mp4")+".pdf")
			}
			if err := report.WritePDF(out, report.Incident{Event: e, Segment: seg, Loc: loc, Frames: frames,
				Track: track, ClipPath: clipPath, Version: version}); err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), out)
			return nil
		},
	}
	cmd.Flags().StringVar(&out, "out", "", "output PDF (default next to the clip)")
	return cmd
}

// printScoreDetails scores every trip, prints each breakdown and a weekly trend.
func printScoreDetails(cmd *cobra.Command, db *store.DB, cfg config.Config, loc *time.Location) error {
	ctx := cmd.Context()
	scores, err := report.ScoreAll(ctx, db, cfg.Score)
	if err != nil {
		return err
	}
	ts, err := db.Trips(ctx)
	if err != nil {
		return err
	}
	out := cmd.OutOrStdout()
	byID := map[int64]float64{}
	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "TRIP\tDATE\tKM\tSCORE\tPOINTS LOST")
	for i, s := range scores {
		byID[s.TripID] = s.Value
		var parts []string
		for _, l := range s.Lines {
			label := l.Type
			if l.EventID != 0 {
				label = fmt.Sprintf("%s #%d", l.Type, l.EventID)
			}
			if l.Note != "" {
				label += " (" + l.Note + ")"
			}
			parts = append(parts, fmt.Sprintf("%s -%.1f", label, l.Points))
		}
		if len(parts) == 0 {
			parts = []string{"none"}
		}
		fmt.Fprintf(w, "%d\t%s\t%.1f\t%.0f\t%s\n", s.TripID, ts[i].StartedAt.In(loc).Format("2006-01-02 15:04"), s.DistanceKm, s.Value, strings.Join(parts, ", "))
	}
	if err := w.Flush(); err != nil {
		return err
	}
	weeks := report.Weekly(ts, byID, loc)
	var vals []float64
	fmt.Fprintln(out, "\nweekly average (by distance):")
	for _, wk := range weeks {
		vals = append(vals, wk.Avg)
		fmt.Fprintf(out, "  %d-W%02d  %5.1f  (%d trips)\n", wk.Year, wk.Week, wk.Avg, wk.Trips)
	}
	fmt.Fprintf(out, "  trend  %s\n", report.Spark(vals))
	return nil
}
