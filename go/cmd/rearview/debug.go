package main

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"rearview/internal/mp4gps"
)

func newDebugCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "debug",
		Short: "Low-level tools for inspecting footage files",
	}
	cmd.AddCommand(newDebugBoxesCmd(), newDebugGPSCmd(), newDebugGPSFixtureCmd())
	return cmd
}

func newDebugBoxesCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "boxes <file>",
		Short: "Print the MP4 box tree of a file",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			f, size, err := openSized(args[0])
			if err != nil {
				return err
			}
			defer f.Close()
			boxes, werr := mp4gps.Walk(f, size)
			printBoxes(cmd.OutOrStdout(), boxes, 0)
			return werr
		},
	}
}

func printBoxes(w io.Writer, boxes []mp4gps.Box, depth int) {
	for _, b := range boxes {
		fmt.Fprintf(w, "%s%q offset=%d size=%d\n", strings.Repeat("  ", depth), b.Type, b.Offset, b.Size)
		printBoxes(w, b.Children, depth+1)
	}
}

func newDebugGPSCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "gps <file>",
		Short: "Print the decoded GPS points of a file as CSV",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			f, size, err := openSized(args[0])
			if err != nil {
				return err
			}
			defer f.Close()
			pts, err := mp4gps.ReadGPS(f, size)
			if err != nil {
				return err
			}
			w := cmd.OutOrStdout()
			fmt.Fprintln(w, "t_offset_s,ts_utc,lat,lon,speed_kmh,heading_deg")
			for _, p := range pts {
				fmt.Fprintf(w, "%d,%s,%.7f,%.7f,%.2f,%.2f\n", p.Index, p.Time.Format("2006-01-02T15:04:05Z"),
					p.Lat, p.Lon, p.SpeedKmh, p.HeadingDeg)
			}
			return nil
		},
	}
}

func newDebugGPSFixtureCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "gps-fixture <in.mp4> <out.bin>",
		Short: "Write a small file holding only the GPS boxes of a recording (for golden tests)",
		Args:  cobra.ExactArgs(2),
		RunE: func(_ *cobra.Command, args []string) error {
			f, size, err := openSized(args[0])
			if err != nil {
				return err
			}
			defer f.Close()
			out, err := os.Create(args[1])
			if err != nil {
				return err
			}
			if err := mp4gps.WriteFixture(out, f, size); err != nil {
				out.Close()
				return err
			}
			return out.Close()
		},
	}
}

func openSized(path string) (*os.File, int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, 0, err
	}
	st, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, 0, err
	}
	return f, st.Size(), nil
}
