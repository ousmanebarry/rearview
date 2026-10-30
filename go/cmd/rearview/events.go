package main

import (
	"context"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"rearview/internal/clips"
	"rearview/internal/config"
	"rearview/internal/fusion"
	"rearview/internal/labels"
	"rearview/internal/prefilter"
	"rearview/internal/store"
)

func newEventsCmd() *cobra.Command {
	var typ, since string
	var tripID int64
	cmd := &cobra.Command{
		Use:   "events [--type hard_brake] [--since 7d] [--trip <id>]",
		Short: "List events, newest first, with time, type, score and location",
		Long: "List events, newest first.\n\nTypes: " + strings.Join(labels.Types, ", ") +
			"\n--since takes a duration with a d (days) or h (hours) suffix, or a date (2026-10-23).",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, db, err := openDB(cmd.Context(), cmd)
			if err != nil {
				return err
			}
			defer db.Close()
			loc, err := time.LoadLocation(cfg.TimeZone)
			if err != nil {
				return err
			}
			f := store.EventFilter{TripID: tripID}
			if typ != "" {
				f.Types = strings.Split(typ, ",")
			}
			if since != "" {
				if f.Since, err = parseSince(since, time.Now(), loc); err != nil {
					return err
				}
			}
			evs, err := db.Events(cmd.Context(), f)
			if err != nil {
				return err
			}
			w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "ID\tTIME\tTYPE\tSOURCE\tSCORE\tTRIP\tLOCATION")
			for _, e := range evs {
				pos := "-"
				if e.HasPos {
					pos = fmt.Sprintf("%.5f,%.5f", e.Lat, e.Lon)
				}
				fmt.Fprintf(w, "%d\t%s\t%s\t%s\t%.2f\t%d\t%s\n", e.ID, e.TS.In(loc).Format("2006-01-02 15:04:05"),
					e.Type, e.Source, e.Score, e.TripID, pos)
			}
			fmt.Fprintf(w, "%d events\n", len(evs))
			return w.Flush()
		},
	}
	cmd.Flags().StringVar(&typ, "type", "", "only these types (comma-separated)")
	cmd.Flags().StringVar(&since, "since", "", "only events after this (7d, 12h or 2026-10-23)")
	cmd.Flags().Int64Var(&tripID, "trip", 0, "only events of this trip")
	cmd.AddCommand(newEventsDetectCmd(), newEventsFuseCmd())
	return cmd
}

func newEventsFuseCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "fuse",
		Short: "Recompute near-miss, following-close and tailgated events from stored observations",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, db, err := openDB(cmd.Context(), cmd)
			if err != nil {
				return err
			}
			defer db.Close()
			n, err := fusion.Rebuild(cmd.Context(), db, fusion.Config{NearMiss: cfg.NearMiss, Following: cfg.Following})
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%d vision events\n", n)
			return nil
		},
	}
}

func parseSince(s string, now time.Time, loc *time.Location) (time.Time, error) {
	if n, ok := strings.CutSuffix(s, "d"); ok {
		if v, err := strconv.ParseFloat(n, 64); err == nil {
			return now.Add(-time.Duration(v * 24 * float64(time.Hour))), nil
		}
	}
	if d, err := time.ParseDuration(s); err == nil {
		return now.Add(-d), nil
	}
	if t, err := time.ParseInLocation("2006-01-02", s, loc); err == nil {
		return t, nil
	}
	return time.Time{}, fmt.Errorf("--since %q: want 7d, 12h or 2026-10-23", s)
}

func newEventsDetectCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "detect",
		Short: "Recompute motion events (hard brakes, swerves) from GPS with the current thresholds",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, db, err := openDB(cmd.Context(), cmd)
			if err != nil {
				return err
			}
			defer db.Close()
			n, err := prefilter.Rebuild(cmd.Context(), db, cfg.Motion)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%d motion events\n", n)
			return nil
		},
	}
}

func newClipCmd() *cobra.Command {
	var sideBySide, exact bool
	var camera string
	cmd := &cobra.Command{
		Use:   "clip <event id> [--side-by-side] [--exact]",
		Short: "Cut a clip of an event and print its path",
		Long: "Cut a clip from 10 s before to 5 s after the event (motion.window_before_s/after_s).\n" +
			"By default the stream is copied (fast, starts at the keyframe before the cut).\n" +
			"--exact re-encodes for a frame-accurate cut; --side-by-side stacks front and rear (re-encodes).",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := strconv.ParseInt(args[0], 10, 64)
			if err != nil {
				return fmt.Errorf("event id %q: %w", args[0], err)
			}
			cfg, db, err := openDB(cmd.Context(), cmd)
			if err != nil {
				return err
			}
			defer db.Close()
			path, err := clipEvent(cmd.Context(), db, cfg, id, camera, sideBySide, exact)
			if err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), path)
			return nil
		},
	}
	cmd.Flags().BoolVar(&sideBySide, "side-by-side", false, "front and rear next to each other (re-encodes)")
	cmd.Flags().BoolVar(&exact, "exact", false, "re-encode for a frame-accurate cut")
	cmd.Flags().StringVar(&camera, "camera", "", "camera to cut (default: the event's camera, else front)")
	return cmd
}

// eventParts finds the library parts covering an event's clip window per camera.
func eventParts(ctx context.Context, db *store.DB, cfg config.Config, e store.Event) (map[string][]clips.Part, error) {
	seg, err := db.Segment(ctx, e.SegmentID)
	if err != nil {
		return nil, err
	}
	segs, err := db.Segments(ctx, seg.TripID)
	if err != nil {
		return nil, err
	}
	if seg.TripID == 0 {
		segs = []store.Segment{seg}
	}
	byID := map[int64]store.Segment{}
	for _, s := range segs {
		byID[s.ID] = s
	}
	parts := map[string][]clips.Part{}
	for _, w := range prefilter.EventWindows(e, segs, cfg.Motion.WindowBeforeS, cfg.Motion.WindowAfterS) {
		s := byID[w.SegmentID]
		parts[w.Camera] = append(parts[w.Camera], clips.Part{
			Path: filepath.Join(cfg.LibraryPath, filepath.FromSlash(s.Path)), StartS: w.StartS, EndS: w.EndS,
		})
	}
	return parts, nil
}

func clipEvent(ctx context.Context, db *store.DB, cfg config.Config, id int64, camera string, sideBySide, exact bool) (string, error) {
	e, err := db.Event(ctx, id)
	if err != nil {
		return "", err
	}
	loc, err := time.LoadLocation(cfg.TimeZone)
	if err != nil {
		return "", err
	}
	parts, err := eventParts(ctx, db, cfg, e)
	if err != nil {
		return "", err
	}
	out := filepath.Join(cfg.ClipsPath, clips.Name(e.TS, e.Type, e.ID, loc))
	layout := "single"
	opts := clips.Options{Exact: exact}
	if sideBySide {
		if len(parts["front"]) == 0 || len(parts["rear"]) == 0 {
			return "", fmt.Errorf("event %d: need both front and rear footage for --side-by-side", id)
		}
		out = strings.TrimSuffix(out, ".mp4") + "_sbs.mp4"
		layout = "side_by_side"
		err = clips.SideBySide(ctx, parts["front"], parts["rear"], out, opts)
	} else {
		if camera == "" {
			camera = e.Camera
		}
		if camera == "" {
			camera = "front"
		}
		if len(parts[camera]) == 0 {
			return "", fmt.Errorf("event %d: no %s footage around %s", id, camera, e.TS.Format(time.RFC3339))
		}
		if camera != "front" {
			out = strings.TrimSuffix(out, ".mp4") + "_" + camera + ".mp4"
		}
		err = clips.Cut(ctx, parts[camera], out, opts)
	}
	if err != nil {
		return "", err
	}
	if _, err := db.InsertClip(ctx, store.Clip{EventID: id, Path: out, Layout: layout}); err != nil {
		return "", err
	}
	return out, nil
}
