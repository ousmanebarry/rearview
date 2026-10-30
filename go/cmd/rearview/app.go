package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"rearview/internal/config"
	"rearview/internal/store"
)

// loadConfig reads the file named by --config (default ~/rearview/config.yaml).
func loadConfig(cmd *cobra.Command) (config.Config, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return config.Config{}, fmt.Errorf("find home folder: %w", err)
	}
	path, _ := cmd.Flags().GetString("config")
	if path == "" {
		path = config.DefaultPath(home)
	}
	return config.Load(path, home)
}

// openDB loads the config and opens its database, creating the folder if needed.
func openDB(ctx context.Context, cmd *cobra.Command) (config.Config, *store.DB, error) {
	cfg, err := loadConfig(cmd)
	if err != nil {
		return cfg, nil, err
	}
	if err := os.MkdirAll(filepath.Dir(cfg.DatabasePath), 0o755); err != nil {
		return cfg, nil, err
	}
	db, err := store.Open(ctx, cfg.DatabasePath)
	return cfg, db, err
}

func newConfigCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Inspect the configuration",
	}
	cmd.AddCommand(&cobra.Command{
		Use:   "show",
		Short: "Print the defaults merged with the config file, as YAML",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := loadConfig(cmd)
			if err != nil {
				return err
			}
			b, err := cfg.YAML()
			if err != nil {
				return err
			}
			_, err = cmd.OutOrStdout().Write(b)
			return err
		},
	})
	return cmd
}
