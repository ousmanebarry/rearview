package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"rearview/internal/config"
	"rearview/internal/models"
)

// Overridden in tests.
var (
	modelFiles  = models.Files
	modelClient = http.DefaultClient
	exeDir      = func() (string, error) {
		exe, err := os.Executable()
		if err != nil {
			return "", err
		}
		if p, err := filepath.EvalSymlinks(exe); err == nil {
			exe = p
		}
		return filepath.Dir(exe), nil
	}
)

func engineName() string {
	if runtime.GOOS == "windows" {
		return "engine.exe"
	}
	return "engine"
}

// bundled reports whether the engine sits next to this rearview binary, as in
// the release download.
func bundled(enginePath string) bool {
	dir, err := exeDir()
	if err != nil || enginePath == "" {
		return false
	}
	a, err1 := os.Stat(dir)
	b, err2 := os.Stat(filepath.Dir(enginePath))
	return err1 == nil && err2 == nil && os.SameFile(a, b)
}

// modelsDir picks the engine's models folder: engine.models_path, then
// $REARVIEW_MODELS, then a "models" folder at or above the engine binary (what
// the engine itself does; always the engine's own folder for a release
// download), the repo's engine/models, and ~/rearview/models.
// found is false when none holds the models yet; dir is then where to put them.
// enginePath may be empty when no engine was found.
func modelsDir(cfg config.Config, enginePath string) (dir string, found bool) {
	has := func(d string) bool {
		_, err := os.Stat(filepath.Join(d, models.Marker))
		return err == nil
	}
	if d := cfg.Engine.ModelsPath; d != "" {
		return d, has(d)
	}
	if d := os.Getenv("REARVIEW_MODELS"); d != "" {
		return d, has(d)
	}
	if enginePath != "" {
		if d, ok := models.FindNear(filepath.Dir(enginePath)); ok {
			return d, true
		}
		if bundled(enginePath) {
			return filepath.Join(filepath.Dir(enginePath), "models"), false
		}
	}
	root, rootErr := findRepoRoot()
	if rootErr == nil && has(filepath.Join(root, "engine", "models")) {
		return filepath.Join(root, "engine", "models"), true
	}
	data := filepath.Join(filepath.Dir(cfg.DatabasePath), "models")
	if has(data) {
		return data, true
	}
	if rootErr == nil {
		return filepath.Join(root, "engine", "models"), false
	}
	return data, false
}

// engineModels returns the models folder to pass to the engine ("" to let the
// engine look for itself). A bundled engine with missing models gets them
// downloaded first, so the release works on first run.
func engineModels(ctx context.Context, cfg config.Config, enginePath string) (string, error) {
	dir, found := modelsDir(cfg, enginePath)
	if !bundled(enginePath) {
		if found {
			return dir, nil
		}
		return "", nil
	}
	missing := models.Missing(dir, modelFiles)
	if len(missing) == 0 {
		return dir, nil
	}
	fmt.Fprintf(os.Stderr, "first run: downloading the models (about %d MB) into %s\n", models.TotalMB(missing), dir)
	if err := models.Download(ctx, modelClient, dir, missing, os.Stderr); err != nil {
		return "", fmt.Errorf("download models (retry with `rearview models download`): %w", err)
	}
	return dir, nil
}

func newModelsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "models",
		Short: "Show where the engine's models are and which are missing",
		Long: "List the files of the engine's models folder (YOLOX, CLIP, YuNet; about 615 MB).\n" +
			"`rearview models download` fetches the missing ones and checks every checksum.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := loadConfig(cmd)
			if err != nil {
				return err
			}
			eng, _ := enginePath(cfg)
			dir, _ := modelsDir(cfg, eng)
			dir = filepath.Clean(dir)
			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "models %s\n", dir)
			missing := map[string]bool{}
			for _, f := range models.Missing(dir, modelFiles) {
				missing[f.Name] = true
			}
			tw := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
			for _, f := range modelFiles {
				state := "ok"
				if missing[f.Name] {
					state = "missing"
				}
				fmt.Fprintf(tw, "  %s\t%d MB\t%s\n", f.Name, f.SizeMB, state)
			}
			tw.Flush()
			if len(missing) > 0 {
				fmt.Fprintln(out, "run `rearview models download` to fetch the missing files")
			}
			return nil
		},
	}
	var dirFlag string
	download := &cobra.Command{
		Use:   "download [--dir folder]",
		Short: "Download the missing models and verify every checksum",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt)
			defer stop()
			cfg, err := loadConfig(cmd)
			if err != nil {
				return err
			}
			dir := dirFlag
			if dir == "" {
				eng, _ := enginePath(cfg)
				dir, _ = modelsDir(cfg, eng)
			}
			dir = filepath.Clean(dir)
			fmt.Fprintf(cmd.ErrOrStderr(), "models %s (about %d MB to download)\n", dir, models.TotalMB(models.Missing(dir, modelFiles)))
			if err := models.Download(ctx, modelClient, dir, modelFiles, cmd.ErrOrStderr()); err != nil {
				return err
			}
			if dirFlag != "" {
				fmt.Fprintf(cmd.OutOrStdout(), "done; set engine.models_path to %s if the engine can't find them\n", dir)
			} else {
				fmt.Fprintln(cmd.OutOrStdout(), "done")
			}
			return nil
		},
	}
	download.Flags().StringVar(&dirFlag, "dir", "", "folder to download into (default: where the engine looks)")
	cmd.AddCommand(download)
	return cmd
}
