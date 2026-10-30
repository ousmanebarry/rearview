package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// Segment is one recorded file in the library.
type Segment struct {
	ID         int64
	Path       string // relative to the library root, forward slashes
	SourceName string
	Hash       string
	SizeBytes  int64
	Camera     string
	StartedAt  time.Time
	DurationS  float64
	Width      int
	Height     int
	FPS        float64
	Locked     bool
	Parking    bool
	PairKey    string
	TripID     int64 // 0 = none
	ImportedAt time.Time
}

// End is when the segment's recording stops.
func (s Segment) End() time.Time {
	return s.StartedAt.Add(time.Duration(s.DurationS * float64(time.Second)))
}

func nullStr(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func nullID(id int64) any {
	if id == 0 {
		return nil
	}
	return id
}

// InsertSegment adds a segment and returns its id.
func (db *DB) InsertSegment(ctx context.Context, s Segment) (int64, error) {
	res, err := db.ExecContext(ctx, `INSERT INTO segments
		(path, source_name, content_hash, size_bytes, camera, started_at, duration_s, width, height, fps,
		 locked, parking, pair_key, trip_id, imported_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		s.Path, s.SourceName, s.Hash, s.SizeBytes, s.Camera, FormatTime(s.StartedAt), s.DurationS, s.Width, s.Height, s.FPS,
		s.Locked, s.Parking, nullStr(s.PairKey), nullID(s.TripID), FormatTime(s.ImportedAt))
	if err != nil {
		return 0, fmt.Errorf("insert segment %s: %w", s.SourceName, err)
	}
	return res.LastInsertId()
}

const segmentCols = `id, path, source_name, content_hash, size_bytes, camera, started_at, COALESCE(duration_s,0),
	COALESCE(width,0), COALESCE(height,0), COALESCE(fps,0), locked, parking, COALESCE(pair_key,''), COALESCE(trip_id,0), imported_at`

type scanner interface{ Scan(...any) error }

func scanSegment(r scanner) (Segment, error) {
	var s Segment
	var started, imported string
	err := r.Scan(&s.ID, &s.Path, &s.SourceName, &s.Hash, &s.SizeBytes, &s.Camera, &started, &s.DurationS,
		&s.Width, &s.Height, &s.FPS, &s.Locked, &s.Parking, &s.PairKey, &s.TripID, &imported)
	if err != nil {
		return s, err
	}
	if s.StartedAt, err = ParseTime(started); err != nil {
		return s, err
	}
	s.ImportedAt, err = ParseTime(imported)
	return s, err
}

// SegmentByHash returns the segment with this content hash, or ok=false.
func (db *DB) SegmentByHash(ctx context.Context, hash string) (Segment, bool, error) {
	s, err := scanSegment(db.QueryRowContext(ctx, `SELECT `+segmentCols+` FROM segments WHERE content_hash = ?`, hash))
	if errors.Is(err, sql.ErrNoRows) {
		return s, false, nil
	}
	return s, err == nil, err
}

// Segment returns one segment by id.
func (db *DB) Segment(ctx context.Context, id int64) (Segment, error) {
	s, err := scanSegment(db.QueryRowContext(ctx, `SELECT `+segmentCols+` FROM segments WHERE id = ?`, id))
	if err != nil {
		return s, fmt.Errorf("segment %d: %w", id, err)
	}
	return s, nil
}

// Segments returns segments ordered by start time. tripID 0 means all.
func (db *DB) Segments(ctx context.Context, tripID int64) ([]Segment, error) {
	q := `SELECT ` + segmentCols + ` FROM segments`
	var args []any
	if tripID != 0 {
		q += ` WHERE trip_id = ?`
		args = append(args, tripID)
	}
	rows, err := db.QueryContext(ctx, q+` ORDER BY started_at, camera, id`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Segment
	for rows.Next() {
		s, err := scanSegment(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// GPSPoint is one row of gps_points.
type GPSPoint struct {
	SegmentID    int64
	TOffsetS     float64
	TS           time.Time
	Lat, Lon     float64
	SpeedKmh     float64
	HeadingDeg   float64
	Interpolated bool
}

// ReplaceGPS deletes a segment's points and inserts pts in one transaction.
func (db *DB) ReplaceGPS(ctx context.Context, segmentID int64, pts []GPSPoint) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM gps_points WHERE segment_id = ?`, segmentID); err != nil {
		return err
	}
	stmt, err := tx.PrepareContext(ctx, `INSERT INTO gps_points
		(segment_id, t_offset_s, ts_utc, lat, lon, speed_kmh, heading_deg, interpolated) VALUES (?,?,?,?,?,?,?,?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, p := range pts {
		if _, err := stmt.ExecContext(ctx, segmentID, p.TOffsetS, FormatTime(p.TS), p.Lat, p.Lon, p.SpeedKmh, p.HeadingDeg, p.Interpolated); err != nil {
			return fmt.Errorf("insert gps point %v: %w", p.TOffsetS, err)
		}
	}
	return tx.Commit()
}

// GPS returns a segment's points ordered by offset.
func (db *DB) GPS(ctx context.Context, segmentID int64) ([]GPSPoint, error) {
	rows, err := db.QueryContext(ctx, `SELECT segment_id, t_offset_s, ts_utc, lat, lon, COALESCE(speed_kmh,0),
		COALESCE(heading_deg,0), interpolated FROM gps_points WHERE segment_id = ? ORDER BY t_offset_s`, segmentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []GPSPoint
	for rows.Next() {
		var p GPSPoint
		var ts string
		if err := rows.Scan(&p.SegmentID, &p.TOffsetS, &ts, &p.Lat, &p.Lon, &p.SpeedKmh, &p.HeadingDeg, &p.Interpolated); err != nil {
			return nil, err
		}
		if p.TS, err = ParseTime(ts); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}
