package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// Trip is one drive: segments with no gap longer than the trip gap.
type Trip struct {
	ID          int64
	StartedAt   time.Time
	EndedAt     time.Time
	DistanceKm  float64
	DurationS   float64
	MaxSpeedKmh float64
	AvgSpeedKmh float64
	Score       *float64
}

const tripCols = `id, started_at, ended_at, COALESCE(distance_km,0), COALESCE(duration_s,0),
	COALESCE(max_speed_kmh,0), COALESCE(avg_speed_kmh,0), score`

func scanTrip(r scanner) (Trip, error) {
	var t Trip
	var s, e string
	var score sql.NullFloat64
	if err := r.Scan(&t.ID, &s, &e, &t.DistanceKm, &t.DurationS, &t.MaxSpeedKmh, &t.AvgSpeedKmh, &score); err != nil {
		return t, err
	}
	if score.Valid {
		t.Score = &score.Float64
	}
	var err error
	if t.StartedAt, err = ParseTime(s); err != nil {
		return t, err
	}
	t.EndedAt, err = ParseTime(e)
	return t, err
}

// Trips returns every trip, oldest first.
func (db *DB) Trips(ctx context.Context) ([]Trip, error) {
	rows, err := db.QueryContext(ctx, `SELECT `+tripCols+` FROM trips ORDER BY started_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Trip
	for rows.Next() {
		t, err := scanTrip(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// Trip returns one trip.
func (db *DB) Trip(ctx context.Context, id int64) (Trip, error) {
	t, err := scanTrip(db.QueryRowContext(ctx, `SELECT `+tripCols+` FROM trips WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return t, fmt.Errorf("trip %d not found", id)
	}
	return t, err
}

// SetTripScore stores a trip's driving score.
func (db *DB) SetTripScore(ctx context.Context, id int64, score float64) error {
	_, err := db.ExecContext(ctx, `UPDATE trips SET score = ? WHERE id = ?`, score, id)
	return err
}

// TripPlan is the result of trip assembly, written by ApplyTrips.
type TripPlan struct {
	Trips    []PlannedTrip
	Segments []SegmentUpdate
	// Interpolated replaces every segment's interpolated gps_points rows.
	Interpolated []GPSPoint
}

// PlannedTrip is a trip and the segments it holds. ReuseID, when not 0, is
// the id of an existing trip covering the same footage, so ids stay stable.
type PlannedTrip struct {
	Trip
	ReuseID    int64
	SegmentIDs []int64
}

// SegmentUpdate is what assembly changes on a segment.
type SegmentUpdate struct {
	ID           int64
	StartedAt    time.Time
	ClockOffsetS float64
	PairKey      string
}

// ApplyTrips replaces the trips table with the plan in one transaction. Trip
// ids are reused where the plan says so; events follow their segment's trip.
func (db *DB) ApplyTrips(ctx context.Context, p TripPlan) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	exec := func(q string, args ...any) error {
		_, err := tx.ExecContext(ctx, q, args...)
		return err
	}
	if err := exec(`UPDATE events SET trip_id = NULL`); err != nil {
		return err
	}
	if err := exec(`UPDATE segments SET trip_id = NULL`); err != nil {
		return err
	}
	keep := map[int64]bool{}
	for _, t := range p.Trips {
		if t.ReuseID != 0 {
			keep[t.ReuseID] = true
		}
	}
	rows, err := tx.QueryContext(ctx, `SELECT id FROM trips`)
	if err != nil {
		return err
	}
	var drop []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		if !keep[id] {
			drop = append(drop, id)
		}
	}
	rows.Close()
	for _, id := range drop {
		if err := exec(`DELETE FROM trips WHERE id = ?`, id); err != nil {
			return err
		}
	}
	for _, t := range p.Trips {
		id := t.ReuseID
		args := []any{FormatTime(t.StartedAt), FormatTime(t.EndedAt), t.DistanceKm, t.DurationS, t.MaxSpeedKmh, t.AvgSpeedKmh}
		if id != 0 {
			if err := exec(`UPDATE trips SET started_at=?, ended_at=?, distance_km=?, duration_s=?, max_speed_kmh=?, avg_speed_kmh=? WHERE id=?`,
				append(args, id)...); err != nil {
				return err
			}
		} else {
			res, err := tx.ExecContext(ctx, `INSERT INTO trips (started_at, ended_at, distance_km, duration_s, max_speed_kmh, avg_speed_kmh)
				VALUES (?,?,?,?,?,?)`, args...)
			if err != nil {
				return err
			}
			if id, err = res.LastInsertId(); err != nil {
				return err
			}
		}
		for _, sid := range t.SegmentIDs {
			if err := exec(`UPDATE segments SET trip_id = ? WHERE id = ?`, id, sid); err != nil {
				return err
			}
		}
	}
	for _, s := range p.Segments {
		if err := exec(`UPDATE segments SET started_at = ?, clock_offset_s = ?, pair_key = ? WHERE id = ?`,
			FormatTime(s.StartedAt), s.ClockOffsetS, nullStr(s.PairKey), s.ID); err != nil {
			return err
		}
	}
	if err := exec(`DELETE FROM gps_points WHERE interpolated = 1`); err != nil {
		return err
	}
	for _, g := range p.Interpolated {
		if err := exec(`INSERT INTO gps_points (segment_id, t_offset_s, ts_utc, lat, lon, speed_kmh, heading_deg, interpolated)
			VALUES (?,?,?,?,?,?,?,1)`, g.SegmentID, g.TOffsetS, FormatTime(g.TS), g.Lat, g.Lon, g.SpeedKmh, g.HeadingDeg); err != nil {
			return fmt.Errorf("insert interpolated point: %w", err)
		}
	}
	if err := exec(`UPDATE events SET trip_id = (SELECT trip_id FROM segments WHERE segments.id = events.segment_id)`); err != nil {
		return err
	}
	return tx.Commit()
}

// AllGPS returns every segment's points keyed by segment id.
func (db *DB) AllGPS(ctx context.Context) (map[int64][]GPSPoint, error) {
	rows, err := db.QueryContext(ctx, `SELECT segment_id, t_offset_s, ts_utc, lat, lon, COALESCE(speed_kmh,0),
		COALESCE(heading_deg,0), interpolated FROM gps_points ORDER BY segment_id, t_offset_s`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int64][]GPSPoint{}
	for rows.Next() {
		var p GPSPoint
		var ts string
		if err := rows.Scan(&p.SegmentID, &p.TOffsetS, &ts, &p.Lat, &p.Lon, &p.SpeedKmh, &p.HeadingDeg, &p.Interpolated); err != nil {
			return nil, err
		}
		if p.TS, err = ParseTime(ts); err != nil {
			return nil, err
		}
		out[p.SegmentID] = append(out[p.SegmentID], p)
	}
	return out, rows.Err()
}
