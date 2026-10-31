package main

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"rearview/internal/engine"
	"rearview/internal/store"
)

func newExportCmd() *cobra.Command {
	var blurOn, sideBySide bool
	var camera string
	cmd := &cobra.Command{
		Use:   "export <event id> --blur [--camera rear]",
		Short: "Cut an event clip for sharing, with faces and number plates blurred",
		Long: "Cut the event's clip (as rearview clip does), then have the engine blur every face\n" +
			"(YuNet) and every vehicle's plate area and person's head (from YOLOX boxes), held\n" +
			"for a few frames so the blur doesn't flicker. The result is re-encoded.",
		Args: cobra.ExactArgs(1),
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
			src, err := clipEvent(ctx, db, cfg, id, camera, sideBySide, false)
			if err != nil {
				return err
			}
			if !blurOn {
				fmt.Fprintln(cmd.OutOrStdout(), src)
				return nil
			}
			out := strings.TrimSuffix(src, ".mp4") + "_blurred.mp4"
			eng, err := lazyEngine(ctx, cfg)
			if err != nil {
				return err
			}
			defer eng.Close()
			if _, err := eng.Do(ctx, engine.Job{Cmd: "blur", Fields: map[string]any{"path": src, "out_path": out}}, nil); err != nil {
				return fmt.Errorf("blur: %w", err)
			}
			layout := "single"
			if sideBySide {
				layout = "side_by_side"
			}
			if _, err := db.InsertClip(ctx, store.Clip{EventID: id, Path: out, Layout: layout, Blurred: true}); err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), out)
			return nil
		},
	}
	cmd.Flags().BoolVar(&blurOn, "blur", false, "blur faces and number plates")
	cmd.Flags().BoolVar(&sideBySide, "side-by-side", false, "front and rear next to each other")
	cmd.Flags().StringVar(&camera, "camera", "", "camera (default: the event's camera, else front)")
	return cmd
}
