package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"rearview/internal/labels"
)

const defaultTZ = "America/New_York"

func newLabelCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "label",
		Short: "Manage hand-labelled ground-truth events",
	}
	cmd.PersistentFlags().String("file", "", "labels CSV (default: <repo>/testdata/labels/events.csv)")
	cmd.PersistentFlags().String("tz", defaultTZ, "time zone for local times without an offset")
	cmd.AddCommand(newLabelAddCmd(), newLabelListCmd())
	return cmd
}

func newLabelAddCmd() *cobra.Command {
	var at, camera, typ, note string
	cmd := &cobra.Command{
		Use:   "add --at <time> --camera <front|rear|interior> --type <event type> [--note ...]",
		Short: "Validate a label and append it to the labels CSV",
		Long: "Validate a label and append it to the labels CSV.\n\n" +
			"--at takes RFC 3339 (2026-10-23T17:07:12Z, 2026-10-23T13:07:12-04:00) or a camera-local\n" +
			"time without an offset (2026-10-23T13:07:12), read in --tz. Labels are stored in UTC.\n\n" +
			"Types: " + strings.Join(labels.Types, ", "),
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			path, loc, err := labelFileAndZone(cmd)
			if err != nil {
				return err
			}
			ts, err := labels.ParseTime(at, loc)
			if err != nil {
				return err
			}
			l := labels.Label{TS: ts, Camera: camera, Type: typ, Note: note}
			if err := labels.Append(path, l); err != nil {
				return fmt.Errorf("add label: %w", err)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "added %s %s %s (local %s)\n",
				labels.FormatTS(ts), camera, typ, ts.In(loc).Format("2006-01-02 15:04:05 MST"))
			return nil
		},
	}
	cmd.Flags().StringVar(&at, "at", "", "event time (required)")
	cmd.Flags().StringVar(&camera, "camera", "", "front, rear or interior (required)")
	cmd.Flags().StringVar(&typ, "type", "", "event type (required)")
	cmd.Flags().StringVar(&note, "note", "", "free-text note, single line")
	for _, f := range []string{"at", "camera", "type"} {
		_ = cmd.MarkFlagRequired(f)
	}
	return cmd
}

func newLabelListCmd() *cobra.Command {
	var typ string
	cmd := &cobra.Command{
		Use:   "list [--type <event type>]",
		Short: "List labels sorted by time, in UTC and local time",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			path, loc, err := labelFileAndZone(cmd)
			if err != nil {
				return err
			}
			all, err := labels.Load(path)
			if err != nil {
				return err
			}
			var shown []labels.Label
			for _, l := range all {
				if typ == "" || l.Type == typ {
					shown = append(shown, l)
				}
			}
			slices.SortStableFunc(shown, func(a, b labels.Label) int { return a.TS.Compare(b.TS) })

			w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "TS_UTC\tLOCAL\tCAMERA\tTYPE\tNOTE")
			for _, l := range shown {
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", labels.FormatTS(l.TS),
					l.TS.In(loc).Format("2006-01-02 15:04:05 MST"), l.Camera, l.Type, l.Note)
			}
			if err := w.Flush(); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%d labels\n", len(shown))
			return nil
		},
	}
	cmd.Flags().StringVar(&typ, "type", "", "only show this event type")
	return cmd
}

func labelFileAndZone(cmd *cobra.Command) (string, *time.Location, error) {
	tz, _ := cmd.Flags().GetString("tz")
	loc, err := time.LoadLocation(tz)
	if err != nil {
		return "", nil, fmt.Errorf("time zone %q: %w", tz, err)
	}
	path, _ := cmd.Flags().GetString("file")
	if path == "" {
		root, err := findRepoRoot()
		if err != nil {
			return "", nil, err
		}
		path = filepath.Join(root, "testdata", "labels", "events.csv")
	}
	return path, loc, nil
}

// findRepoRoot walks up from the working directory to the folder holding .github/AGENTS.md.
func findRepoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("working directory: %w", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, ".github", "AGENTS.md")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", errors.New("not inside the rearview repo; pass --file")
		}
		dir = parent
	}
}
