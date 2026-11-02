package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/spf13/cobra"
)

func newViewCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "view",
		Short: "Open the desktop viewer on the configured database and library",
		Long: "Start the Qt viewer with --db, --library and --rearview filled in from the config.\n" +
			"The viewer is viewer_path from the config, else viewer.exe next to rearview,\n" +
			"else the repo's viewer/build/dev/RelWithDebInfo/viewer.exe, else viewer on PATH.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := loadConfig(cmd)
			if err != nil {
				return err
			}
			self, err := os.Executable()
			if err != nil {
				return err
			}
			viewer, err := findViewer(cfg.ViewerPath, self)
			if err != nil {
				return err
			}
			c := exec.Command(viewer, "--db", cfg.DatabasePath, "--library", cfg.LibraryPath, "--rearview", self)
			if err := c.Start(); err != nil {
				return fmt.Errorf("start viewer: %w", err)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "started %s (pid %d)\n", viewer, c.Process.Pid)
			return c.Process.Release()
		},
	}
}

// findViewer resolves the viewer executable; self is the running rearview binary.
func findViewer(configured, self string) (string, error) {
	if configured != "" {
		return exec.LookPath(configured)
	}
	dir := filepath.Dir(self)
	for _, p := range []string{
		filepath.Join(dir, "viewer.exe"),
		// go/bin/rearview.exe in the repo
		filepath.Join(dir, "..", "..", "viewer", "build", "dev", "RelWithDebInfo", "viewer.exe"),
	} {
		if _, err := os.Stat(p); err == nil {
			return filepath.Clean(p), nil
		}
	}
	if p, err := exec.LookPath("viewer"); err == nil {
		return p, nil
	}
	return "", errors.New("viewer not found: build it (see README) or set viewer_path in the config")
}
