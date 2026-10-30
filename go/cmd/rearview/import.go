package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/spf13/cobra"

	"rearview/internal/config"
	"rearview/internal/ingest"
	"rearview/internal/prefilter"
	"rearview/internal/store"
	"rearview/internal/trips"
)

// probeFunc is replaced in tests, which have no real video files.
var probeFunc ingest.Prober = ingest.FFprobe

func newImportCmd() *cobra.Command {
	var watch bool
	cmd := &cobra.Command{
		Use:   "import <path> [--watch]",
		Short: "Copy new recordings into the library, extract their GPS and rebuild trips",
		Long: "Import every MP4 under <path> (an SD card or a backup folder) into the library.\n" +
			"Files already imported (same content hash) are skipped. With --watch, keep running\n" +
			"and import files as they appear.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt)
			defer stop()
			cfg, db, err := openDB(ctx, cmd)
			if err != nil {
				return err
			}
			defer db.Close()
			in, err := ingest.New(db, cfg, probeFunc)
			if err != nil {
				return err
			}
			if err := ingest.CleanPartials(cfg.LibraryPath); err != nil {
				return fmt.Errorf("clean partial copies: %w", err)
			}
			imp := importer{in: in, db: db, cfg: cfg, out: cmd.OutOrStdout()}
			if err := imp.dir(ctx, args[0]); err != nil {
				return err
			}
			if watch {
				return imp.watch(ctx, args[0])
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&watch, "watch", false, "keep watching the folder for new files")
	return cmd
}

type importer struct {
	in  *ingest.Ingester
	db  *store.DB
	cfg config.Config
	out io.Writer
}

func (imp importer) dir(ctx context.Context, root string) error {
	if _, err := os.Stat(root); err != nil {
		return err
	}
	rs, err := imp.in.ImportDir(ctx, root)
	if err != nil {
		return err
	}
	return imp.finish(ctx, rs)
}

// finish extracts GPS for new segments, rebuilds trips and prints a summary.
func (imp importer) finish(ctx context.Context, rs []ingest.Result) error {
	var nNew, nSkip, nFail int
	for i, r := range rs {
		switch r.Status {
		case ingest.StatusNew:
			if _, err := ingest.ExtractGPS(ctx, imp.db, imp.cfg.LibraryPath, r.Segment); err != nil {
				rs[i].Err = err
				fmt.Fprintf(imp.out, "gps failed: %s: %v\n", filepath.Base(r.Source), err)
			}
			nNew++
		case ingest.StatusSkipped:
			nSkip++
		case ingest.StatusFailed:
			nFail++
			fmt.Fprintf(imp.out, "failed: %s: %v\n", filepath.Base(r.Source), r.Err)
		}
	}
	if nNew > 0 {
		n, err := trips.Rebuild(ctx, imp.db, imp.cfg.Trips)
		if err != nil {
			return fmt.Errorf("rebuild trips: %w", err)
		}
		fmt.Fprintf(imp.out, "%d trips\n", n)
		m, err := prefilter.Rebuild(ctx, imp.db, imp.cfg.Motion)
		if err != nil {
			return fmt.Errorf("motion events: %w", err)
		}
		fmt.Fprintf(imp.out, "%d motion events\n", m)
	}
	fmt.Fprintf(imp.out, "new %d, skipped %d, failed %d\n", nNew, nSkip, nFail)
	if nFail > 0 {
		return fmt.Errorf("%d files failed", nFail)
	}
	return nil
}

func (imp importer) watch(ctx context.Context, root string) error {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	defer w.Close()
	// fsnotify is not recursive: watch every folder, and new ones as they appear.
	err = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err == nil && d.IsDir() {
			return w.Add(p)
		}
		return err
	})
	if err != nil {
		return err
	}
	fmt.Fprintf(imp.out, "watching %s (Ctrl+C to stop)\n", root)
	pending := map[string]bool{}
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case err := <-w.Errors:
			fmt.Fprintf(imp.out, "watch error: %v\n", err)
		case ev := <-w.Events:
			if ev.Has(fsnotify.Create) {
				if st, err := os.Stat(ev.Name); err == nil && st.IsDir() {
					_ = w.Add(ev.Name)
					continue
				}
			}
			if (ev.Has(fsnotify.Create) || ev.Has(fsnotify.Write)) && strings.EqualFold(filepath.Ext(ev.Name), ".mp4") {
				pending[ev.Name] = true
			}
		case <-tick.C:
			if len(pending) == 0 {
				continue
			}
			var rs []ingest.Result
			for p := range pending {
				delete(pending, p)
				r := imp.in.ImportFile(ctx, root, p)
				if errors.Is(r.Err, context.Canceled) {
					return nil
				}
				rs = append(rs, r)
			}
			if err := imp.finish(ctx, rs); err != nil {
				fmt.Fprintln(imp.out, err)
			}
		}
	}
}
