package main

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"rearview/internal/eval"
	"rearview/internal/labels"
	"rearview/internal/store"
)

func newImportLabelsCmd() *cobra.Command {
	var file string
	cmd := &cobra.Command{
		Use:   "import-labels [--file events.csv]",
		Short: "Load the labels CSV into the labels table (replacing what is there)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if file == "" {
				var err error
				if file, err = defaultLabelFile(); err != nil {
					return err
				}
			}
			ls, err := labels.Load(file)
			if err != nil {
				return err
			}
			_, db, err := openDB(cmd.Context(), cmd)
			if err != nil {
				return err
			}
			defer db.Close()
			rows := make([]store.Label, len(ls))
			for i, l := range ls {
				rows[i] = store.Label{TS: l.TS, Camera: l.Camera, Type: l.Type, Note: l.Note}
			}
			if err := db.ReplaceLabels(cmd.Context(), rows); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "imported %d labels from %s\n", len(rows), file)
			return nil
		},
	}
	cmd.Flags().StringVar(&file, "file", "", "labels CSV (default: <repo>/testdata/labels/events.csv)")
	return cmd
}

func newEvalCmd() *cobra.Command {
	var typ, onlyDays, skipDays string
	var tolS float64
	cmd := &cobra.Command{
		Use:   "eval --type <event type>",
		Short: "Compare detected events with the labels: precision, recall, misses and false alarms",
		Long: "Compare detected events with the labels table (load it with import-labels first).\n" +
			"A match is a detection within --tolerance seconds (3) of a label of the same type, one to one.\n" +
			"--only-days / --skip-days take local dates (2026-10-25,2026-10-26) to report on or leave out the held-out set.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if typ == "" {
				return fmt.Errorf("--type is required")
			}
			cfg, db, err := openDB(cmd.Context(), cmd)
			if err != nil {
				return err
			}
			defer db.Close()
			loc, err := time.LoadLocation(cfg.TimeZone)
			if err != nil {
				return err
			}
			keep := dayFilter(onlyDays, skipDays, loc)
			types := strings.Split(typ, ",")
			evs, err := db.Events(cmd.Context(), store.EventFilter{Types: types})
			if err != nil {
				return err
			}
			ls, err := db.Labels(cmd.Context(), types...)
			if err != nil {
				return err
			}
			var dets, labs []eval.Item
			for _, e := range evs {
				if keep(e.TS) {
					dets = append(dets, eval.Item{ID: e.ID, TS: e.TS, Type: e.Type, Note: fmt.Sprintf("score %.2f %s", e.Score, e.Details)})
				}
			}
			for _, l := range ls {
				if keep(l.TS) {
					labs = append(labs, eval.Item{ID: l.ID, TS: l.TS, Type: l.Type, Note: l.Note})
				}
			}
			r := eval.Match(dets, labs, time.Duration(tolS*float64(time.Second)))
			w := cmd.OutOrStdout()
			fmt.Fprintf(w, "%s: %d labels, %d detections, %d matched\n", typ, len(labs), len(dets), len(r.Matched))
			fmt.Fprintf(w, "precision %.3f  recall %.3f\n", r.Precision(), r.Recall())
			for _, m := range r.Misses {
				fmt.Fprintf(w, "miss         %s  label %d  %s\n", m.TS.In(loc).Format("2006-01-02 15:04:05"), m.ID, m.Note)
			}
			for _, f := range r.FalseAlarms {
				fmt.Fprintf(w, "false alarm  %s  event %d  %s\n", f.TS.In(loc).Format("2006-01-02 15:04:05"), f.ID, f.Note)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&typ, "type", "", "event type(s) to score, comma-separated")
	cmd.Flags().StringVar(&onlyDays, "only-days", "", "only these local dates")
	cmd.Flags().StringVar(&skipDays, "skip-days", "", "leave out these local dates")
	cmd.Flags().Float64Var(&tolS, "tolerance", 3, "match tolerance in seconds")
	return cmd
}

func defaultLabelFile() (string, error) {
	root, err := findRepoRoot()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "testdata", "labels", "events.csv"), nil
}

func dayFilter(only, skip string, loc *time.Location) func(time.Time) bool {
	split := func(s string) []string {
		if s == "" {
			return nil
		}
		return strings.Split(s, ",")
	}
	o, s := split(only), split(skip)
	return func(t time.Time) bool {
		d := t.In(loc).Format("2006-01-02")
		if len(o) > 0 && !slices.Contains(o, d) {
			return false
		}
		return !slices.Contains(s, d)
	}
}
