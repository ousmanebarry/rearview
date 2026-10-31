package main

import (
	"fmt"
	"os"
	"strconv"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"rearview/internal/trips"
)

func newTripsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "trips",
		Short: "List trips (date, start, duration, distance, max speed)",
		Args:  cobra.NoArgs,
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
			ts, err := db.Trips(cmd.Context())
			if err != nil {
				return err
			}
			if withScore, _ := cmd.Flags().GetBool("score"); withScore {
				return printScoreDetails(cmd, db, cfg, loc)
			}
			w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "ID\tDATE\tSTART\tDURATION\tDISTANCE\tMAX SPEED")
			for _, t := range ts {
				st := t.StartedAt.In(loc)
				fmt.Fprintf(w, "%d\t%s\t%s\t%s\t%.1f km\t%.0f km/h\n", t.ID, st.Format("2006-01-02"), st.Format("15:04"),
					fmtDuration(t.DurationS), t.DistanceKm, t.MaxSpeedKmh)
			}
			return w.Flush()
		},
	}
	cmd.Flags().Bool("score", false, "score every trip and show what each lost points for, plus a weekly trend")
	cmd.AddCommand(newTripsExportCmd())
	return cmd
}

func fmtDuration(s float64) string {
	m := int(s/60 + 0.5)
	if m < 60 {
		return fmt.Sprintf("%d min", m)
	}
	return fmt.Sprintf("%dh%02d", m/60, m%60)
}

func newTripsExportCmd() *cobra.Command {
	var out string
	cmd := &cobra.Command{
		Use:   "export <trip id> --gpx <out.gpx>",
		Short: "Write a trip's GPS track as GPX",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := strconv.ParseInt(args[0], 10, 64)
			if err != nil {
				return fmt.Errorf("trip id %q: %w", args[0], err)
			}
			_, db, err := openDB(cmd.Context(), cmd)
			if err != nil {
				return err
			}
			defer db.Close()
			t, err := db.Trip(cmd.Context(), id)
			if err != nil {
				return err
			}
			track, _, err := trips.Track(cmd.Context(), db, id)
			if err != nil {
				return err
			}
			f, err := os.Create(out)
			if err != nil {
				return err
			}
			name := fmt.Sprintf("Trip %d, %s", id, t.StartedAt.Format(time.RFC3339))
			if err := trips.WriteGPX(f, name, track, 30*time.Second); err != nil {
				f.Close()
				return err
			}
			if err := f.Close(); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "wrote %s (%d points)\n", out, len(track))
			return nil
		},
	}
	cmd.Flags().StringVar(&out, "gpx", "", "output GPX file (required)")
	_ = cmd.MarkFlagRequired("gpx")
	return cmd
}
