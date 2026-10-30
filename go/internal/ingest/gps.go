package ingest

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"rearview/internal/mp4gps"
	"rearview/internal/store"
)

// ExtractGPS decodes a library segment's GPS blocks into gps_points and
// returns how many points it stored. A file without a gps index stores none.
func ExtractGPS(ctx context.Context, db *store.DB, library string, seg store.Segment) (int, error) {
	f, err := os.Open(filepath.Join(library, filepath.FromSlash(seg.Path)))
	if err != nil {
		return 0, err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return 0, err
	}
	pts, err := mp4gps.ReadGPS(f, st.Size())
	if errors.Is(err, mp4gps.ErrNoGPS) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("gps of %s: %w", seg.SourceName, err)
	}
	rows := make([]store.GPSPoint, len(pts))
	for i, p := range pts {
		rows[i] = store.GPSPoint{
			SegmentID:  seg.ID,
			TOffsetS:   float64(p.Index),
			TS:         p.Time,
			Lat:        p.Lat,
			Lon:        p.Lon,
			SpeedKmh:   p.SpeedKmh,
			HeadingDeg: p.HeadingDeg,
		}
	}
	return len(rows), db.ReplaceGPS(ctx, seg.ID, rows)
}
