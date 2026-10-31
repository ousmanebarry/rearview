package main

import (
	"fmt"
	"os"
	"os/signal"
	"path/filepath"

	"github.com/spf13/cobra"

	"rearview/internal/bench"
)

func newBenchCmd() *cobra.Command {
	var (
		set, label, enginePathFlag, device, decoder string
		limit, workers, threads                     int
		fps, idleFPS                                float64
		serial                                      bool
	)
	cmd := &cobra.Command{
		Use:   "bench [--set bench/set.txt] [--label text] [--workers n] [--device cpu|dml] [--threads n]",
		Short: "Run a fixed set of footage through the pipeline and record minutes per hour of footage",
		Long: "Run every file in the set (library-relative paths, about 1 h of footage) through the full\n" +
			"pipeline in a scratch database, then record minutes of processing per hour of footage,\n" +
			"per-stage frames per second, worker utilization, peak engine memory and GPU utilization\n" +
			"(Windows GPU Engine counters). Results go to the bench_runs table and bench/results/.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt)
			defer stop()
			cfg, db, err := openDB(ctx, cmd)
			if err != nil {
				return err
			}
			defer db.Close()
			root, err := findRepoRoot()
			if err != nil {
				return err
			}
			if set == "" {
				set = filepath.Join(root, "bench", "set.txt")
			}
			files, err := bench.ReadSet(set)
			if err != nil {
				return err
			}
			if limit > 0 && limit < len(files) {
				files = files[:limit]
			}
			if cmd.Flags().Changed("workers") {
				cfg.Engine.Workers = workers
			}
			if cmd.Flags().Changed("device") {
				cfg.Engine.Device = device
			}
			if cmd.Flags().Changed("threads") {
				cfg.Engine.Threads = threads
			}
			if cmd.Flags().Changed("fps") {
				cfg.Engine.FPS = fps
			}
			if enginePathFlag != "" {
				cfg.Engine.Path = enginePathFlag
			}
			if err := cfg.Validate(); err != nil {
				return err
			}
			opt, closeLog, err := pipelineOptions(cfg, "analyze")
			if err != nil {
				return err
			}
			defer closeLog()
			opt.AnalyzeExtra = map[string]any{"threaded": !serial, "decoder": decoder}
			if idleFPS > 0 {
				opt.AnalyzeExtra["idle_fps"] = idleFPS
			}
			scratch, err := os.MkdirTemp("", "rearview-bench")
			if err != nil {
				return err
			}
			defer os.RemoveAll(scratch)
			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "bench: %d files, %d workers, device %s, threads %d, decoder %s, %g fps\n",
				len(files), cfg.Engine.Workers, cfg.Engine.Device, cfg.Engine.Threads, decoder, cfg.Engine.FPS)
			r, err := bench.Run(ctx, db, files, opt, scratch)
			if err != nil {
				return err
			}
			r.Label = label
			r.GitSHA = bench.GitSHA()
			r.Config = map[string]any{"workers": cfg.Engine.Workers, "device": cfg.Engine.Device, "threads": cfg.Engine.Threads,
				"decoder": decoder, "fps": cfg.Engine.FPS, "threaded": !serial, "idle_fps": idleFPS, "engine": opt.Engine.Path, "files": len(files)}
			path, err := bench.Save(ctx, db, r, filepath.Join(root, "bench", "results"))
			if err != nil {
				return err
			}
			fmt.Fprintf(out, "%.1f min of footage in %.1f min: %.2f minutes per hour, %.1f frames/s, utilization %.0f%%, peak %0.f MB",
				r.FootageS/60, r.ElapsedS/60, r.MinutesPerHour, r.FPS, r.WorkerUtil*100, r.PeakMemMB)
			if r.GPUUtilPct != nil {
				fmt.Fprintf(out, ", GPU %.0f%%", *r.GPUUtilPct)
			}
			fmt.Fprintf(out, "\nwrote %s\n", path)
			return nil
		},
	}
	cmd.Flags().StringVar(&set, "set", "", "file listing the footage (default <repo>/bench/set.txt)")
	cmd.Flags().StringVar(&label, "label", "", "short description of what changed (shown in compare and table)")
	cmd.Flags().IntVar(&limit, "limit", 0, "use only the first n files of the set")
	cmd.Flags().IntVar(&workers, "workers", 2, "engine processes")
	cmd.Flags().StringVar(&device, "device", "cpu", "cpu, dml or migraphx")
	cmd.Flags().IntVar(&threads, "threads", 0, "ONNX Runtime threads per engine (0 = default)")
	cmd.Flags().Float64Var(&fps, "fps", 10, "analysis frames per second")
	cmd.Flags().Float64Var(&idleFPS, "idle-fps", 0, "drop to this rate after 20 empty frames (0 = off)")
	cmd.Flags().StringVar(&decoder, "decoder", "opencv", "engine frame source: opencv, ffmpeg (hardware) or ffmpeg-sw")
	cmd.Flags().BoolVar(&serial, "serial", false, "run decode, preprocess and inference on one thread")
	cmd.Flags().StringVar(&enginePathFlag, "engine", "", "engine binary (default from config)")
	cmd.AddCommand(&cobra.Command{
		Use:   "compare",
		Short: "Print the last two benchmark runs side by side",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			_, db, err := openDB(cmd.Context(), cmd)
			if err != nil {
				return err
			}
			defer db.Close()
			rs, err := bench.Load(cmd.Context(), db)
			if err != nil {
				return err
			}
			if len(rs) < 2 {
				return fmt.Errorf("need two runs, have %d", len(rs))
			}
			bench.Compare(cmd.OutOrStdout(), rs[len(rs)-2], rs[len(rs)-1])
			return nil
		},
	}, &cobra.Command{
		Use:   "table",
		Short: "Print the benchmark history as a markdown table",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			_, db, err := openDB(cmd.Context(), cmd)
			if err != nil {
				return err
			}
			defer db.Close()
			rs, err := bench.Load(cmd.Context(), db)
			if err != nil {
				return err
			}
			bench.Table(cmd.OutOrStdout(), rs)
			return nil
		},
	})
	return cmd
}
