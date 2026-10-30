package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Event is one row of the events table.
type Event struct {
	ID        int64
	TripID    int64
	SegmentID int64
	Camera    string
	Type      string
	Source    string // motion | vision | fused
	TS        time.Time
	TStartS   float64
	TEndS     float64
	Score     float64
	Lat, Lon  float64
	HasPos    bool
	Details   string // JSON
	CreatedAt time.Time
}

const eventCols = `id, COALESCE(trip_id,0), segment_id, COALESCE(camera,''), type, source, ts_utc, t_start_s, t_end_s,
	score, lat, lon, details_json, created_at`

func scanEvent(r scanner) (Event, error) {
	var e Event
	var ts, created string
	var lat, lon sql.NullFloat64
	if err := r.Scan(&e.ID, &e.TripID, &e.SegmentID, &e.Camera, &e.Type, &e.Source, &ts, &e.TStartS, &e.TEndS,
		&e.Score, &lat, &lon, &e.Details, &created); err != nil {
		return e, err
	}
	e.Lat, e.Lon, e.HasPos = lat.Float64, lon.Float64, lat.Valid && lon.Valid
	var err error
	if e.TS, err = ParseTime(ts); err != nil {
		return e, err
	}
	e.CreatedAt, err = ParseTime(created)
	return e, err
}

// ReplaceEvents deletes every event from source whose type is in types,
// then inserts evs, in one transaction. It returns the new ids.
func (db *DB) ReplaceEvents(ctx context.Context, source string, types []string, evs []Event) ([]int64, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if len(types) > 0 {
		q := `DELETE FROM events WHERE source = ? AND type IN (?` + strings.Repeat(",?", len(types)-1) + `)`
		args := []any{source}
		for _, t := range types {
			args = append(args, t)
		}
		if _, err := tx.ExecContext(ctx, q, args...); err != nil {
			return nil, err
		}
	}
	ids := make([]int64, 0, len(evs))
	for _, e := range evs {
		id, err := insertEvent(ctx, tx, e)
		if err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, tx.Commit()
}

type execer interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}

func insertEvent(ctx context.Context, x execer, e Event) (int64, error) {
	var lat, lon any
	if e.HasPos {
		lat, lon = e.Lat, e.Lon
	}
	if e.Details == "" {
		e.Details = "{}"
	}
	if e.CreatedAt.IsZero() {
		e.CreatedAt = time.Now()
	}
	res, err := x.ExecContext(ctx, `INSERT INTO events (trip_id, segment_id, camera, type, source, ts_utc, t_start_s, t_end_s,
		score, lat, lon, details_json, created_at) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		nullID(e.TripID), e.SegmentID, nullStr(e.Camera), e.Type, e.Source, FormatTime(e.TS), e.TStartS, e.TEndS,
		e.Score, lat, lon, e.Details, FormatTime(e.CreatedAt))
	if err != nil {
		return 0, fmt.Errorf("insert %s event: %w", e.Type, err)
	}
	return res.LastInsertId()
}

// EventFilter narrows Events. Zero values mean "any".
type EventFilter struct {
	Types  []string
	Source string
	Since  time.Time
	Until  time.Time
	TripID int64
}

// Events returns matching events, newest first.
func (db *DB) Events(ctx context.Context, f EventFilter) ([]Event, error) {
	var where []string
	var args []any
	if len(f.Types) > 0 {
		where = append(where, `type IN (?`+strings.Repeat(",?", len(f.Types)-1)+`)`)
		for _, t := range f.Types {
			args = append(args, t)
		}
	}
	if f.Source != "" {
		where = append(where, `source = ?`)
		args = append(args, f.Source)
	}
	if !f.Since.IsZero() {
		where = append(where, `ts_utc >= ?`)
		args = append(args, FormatTime(f.Since))
	}
	if !f.Until.IsZero() {
		where = append(where, `ts_utc < ?`)
		args = append(args, FormatTime(f.Until))
	}
	if f.TripID != 0 {
		where = append(where, `trip_id = ?`)
		args = append(args, f.TripID)
	}
	q := `SELECT ` + eventCols + ` FROM events`
	if len(where) > 0 {
		q += ` WHERE ` + strings.Join(where, " AND ")
	}
	rows, err := db.QueryContext(ctx, q+` ORDER BY ts_utc DESC, id DESC`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Event
	for rows.Next() {
		e, err := scanEvent(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// Event returns one event.
func (db *DB) Event(ctx context.Context, id int64) (Event, error) {
	e, err := scanEvent(db.QueryRowContext(ctx, `SELECT `+eventCols+` FROM events WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return e, fmt.Errorf("event %d not found", id)
	}
	return e, err
}

// Clip is one row of the clips table.
type Clip struct {
	ID        int64
	EventID   int64
	Path      string
	Layout    string // single | side_by_side
	Blurred   bool
	CreatedAt time.Time
}

// InsertClip records a written clip.
func (db *DB) InsertClip(ctx context.Context, c Clip) (int64, error) {
	if c.Layout == "" {
		c.Layout = "single"
	}
	res, err := db.ExecContext(ctx, `INSERT INTO clips (event_id, path, layout, blurred, created_at) VALUES (?,?,?,?,?)`,
		nullID(c.EventID), c.Path, c.Layout, c.Blurred, FormatTime(time.Now()))
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// Label is one row of the labels table.
type Label struct {
	ID     int64
	TS     time.Time
	Camera string
	Type   string
	Note   string
}

// ReplaceLabels swaps the labels table for ls.
func (db *DB) ReplaceLabels(ctx context.Context, ls []Label) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM labels`); err != nil {
		return err
	}
	for _, l := range ls {
		if _, err := tx.ExecContext(ctx, `INSERT INTO labels (ts_utc, camera, type, note) VALUES (?,?,?,?)`,
			FormatTime(l.TS), nullStr(l.Camera), l.Type, nullStr(l.Note)); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// Labels returns labels of the given types (all when empty), oldest first.
func (db *DB) Labels(ctx context.Context, types ...string) ([]Label, error) {
	q := `SELECT id, ts_utc, COALESCE(camera,''), type, COALESCE(note,'') FROM labels`
	var args []any
	if len(types) > 0 {
		q += ` WHERE type IN (?` + strings.Repeat(",?", len(types)-1) + `)`
		for _, t := range types {
			args = append(args, t)
		}
	}
	rows, err := db.QueryContext(ctx, q+` ORDER BY ts_utc`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Label
	for rows.Next() {
		var l Label
		var ts string
		if err := rows.Scan(&l.ID, &ts, &l.Camera, &l.Type, &l.Note); err != nil {
			return nil, err
		}
		if l.TS, err = ParseTime(ts); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}
