package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

// Job states.
const (
	JobPending = "pending"
	JobRunning = "running"
	JobDone    = "done"
	JobFailed  = "failed"
)

// Job is one unit of engine work: a kind of command over windows of a segment.
type Job struct {
	ID        int64
	SegmentID int64
	Kind      string       // analyze | embed_frames | blur
	Windows   [][2]float64 // empty = whole file
	Priority  int          // 0 = highest
	State     string
	Attempts  int
	LastError string
	EngineFPS float64
}

func windowsJSON(w [][2]float64) string {
	if len(w) == 0 {
		return "[]"
	}
	b, _ := json.Marshal(w)
	return string(b)
}

// InsertJob adds a pending job unless an identical one exists. It reports
// whether a row was added.
func (db *DB) InsertJob(ctx context.Context, j Job) (bool, error) {
	res, err := db.ExecContext(ctx, `INSERT OR IGNORE INTO jobs (segment_id, kind, windows_json, priority, state, created_at)
		VALUES (?,?,?,?, 'pending', ?)`, j.SegmentID, j.Kind, windowsJSON(j.Windows), j.Priority, FormatTime(time.Now()))
	if err != nil {
		return false, fmt.Errorf("insert job: %w", err)
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

func scanJob(r scanner) (Job, error) {
	var j Job
	var w string
	var lastErr sql.NullString
	var fps sql.NullFloat64
	if err := r.Scan(&j.ID, &j.SegmentID, &j.Kind, &w, &j.Priority, &j.State, &j.Attempts, &lastErr, &fps); err != nil {
		return j, err
	}
	j.LastError, j.EngineFPS = lastErr.String, fps.Float64
	if err := json.Unmarshal([]byte(w), &j.Windows); err != nil {
		return j, fmt.Errorf("job %d windows: %w", j.ID, err)
	}
	if len(j.Windows) == 0 {
		j.Windows = nil
	}
	return j, nil
}

const jobCols = `id, segment_id, kind, windows_json, priority, state, attempts, last_error, engine_fps`

// Jobs returns jobs in a state (all when empty) of a kind (all when empty),
// highest priority first.
func (db *DB) Jobs(ctx context.Context, state, kind string) ([]Job, error) {
	q := `SELECT ` + jobCols + ` FROM jobs WHERE (? = '' OR state = ?) AND (? = '' OR kind = ?) ORDER BY priority, id`
	rows, err := db.QueryContext(ctx, q, state, state, kind, kind)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Job
	for rows.Next() {
		j, err := scanJob(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, j)
	}
	return out, rows.Err()
}

// SegmentsWithJobs returns the ids of segments that already have jobs of kind.
func (db *DB) SegmentsWithJobs(ctx context.Context, kind string) (map[int64]bool, error) {
	rows, err := db.QueryContext(ctx, `SELECT DISTINCT segment_id FROM jobs WHERE kind = ?`, kind)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int64]bool{}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out[id] = true
	}
	return out, rows.Err()
}

// StartJob marks a job running and counts the attempt.
func (db *DB) StartJob(ctx context.Context, id int64) error {
	_, err := db.ExecContext(ctx, `UPDATE jobs SET state = 'running', attempts = attempts + 1, started_at = ?, last_error = NULL WHERE id = ?`,
		FormatTime(time.Now()), id)
	return err
}

// FinishJob marks a job done (errMsg empty) or failed.
func (db *DB) FinishJob(ctx context.Context, id int64, engineFPS float64, errMsg string) error {
	state := JobDone
	var le any
	if errMsg != "" {
		state, le = JobFailed, errMsg
	}
	_, err := db.ExecContext(ctx, `UPDATE jobs SET state = ?, engine_fps = ?, last_error = ?, finished_at = ? WHERE id = ?`,
		state, engineFPS, le, FormatTime(time.Now()), id)
	return err
}

// RequeueJob puts a job back to pending (used when a run is interrupted).
func (db *DB) RequeueJob(ctx context.Context, id int64) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM observations WHERE job_id = ?`, id); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM keyframes WHERE job_id = ?`, id); err != nil && !noSuchTable(err) {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE jobs SET state = 'pending', started_at = NULL WHERE id = ?`, id); err != nil {
		return err
	}
	return tx.Commit()
}

// ResetRunning recovers from a crash: every job left running loses its
// partial observations and goes back to pending. It returns how many.
func (db *DB) ResetRunning(ctx context.Context) (int, error) {
	jobs, err := db.Jobs(ctx, JobRunning, "")
	if err != nil {
		return 0, err
	}
	for _, j := range jobs {
		if err := db.RequeueJob(ctx, j.ID); err != nil {
			return 0, err
		}
	}
	return len(jobs), nil
}

// RetryFailed puts failed jobs back to pending.
func (db *DB) RetryFailed(ctx context.Context) (int64, error) {
	res, err := db.ExecContext(ctx, `UPDATE jobs SET state = 'pending' WHERE state = 'failed'`)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// JobCounts returns the number of jobs per state.
func (db *DB) JobCounts(ctx context.Context, kind string) (map[string]int, error) {
	rows, err := db.QueryContext(ctx, `SELECT state, COUNT(*) FROM jobs WHERE (? = '' OR kind = ?) GROUP BY state`, kind, kind)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var s string
		var n int
		if err := rows.Scan(&s, &n); err != nil {
			return nil, err
		}
		out[s] = n
	}
	return out, rows.Err()
}

// Observation is one tracked object in one analysed frame.
type Observation struct {
	JobID     int64
	SegmentID int64
	TOffsetS  float64
	TrackID   int
	Cls       string
	X1, Y1    float64
	X2, Y2    float64
	Conf      float64
	DistM     *float64
	TTCS      *float64
}

// InsertObservations writes a batch in one transaction.
func (db *DB) InsertObservations(ctx context.Context, obs []Observation) error {
	if len(obs) == 0 {
		return nil
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	stmt, err := tx.PrepareContext(ctx, `INSERT INTO observations (job_id, segment_id, t_offset_s, track_id, cls, x1, y1, x2, y2, conf, dist_m, ttc_s)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, o := range obs {
		if _, err := stmt.ExecContext(ctx, o.JobID, o.SegmentID, o.TOffsetS, o.TrackID, o.Cls, o.X1, o.Y1, o.X2, o.Y2, o.Conf, o.DistM, o.TTCS); err != nil {
			return fmt.Errorf("insert observation: %w", err)
		}
	}
	return tx.Commit()
}

// DeleteObservations removes a job's observations (before a retry).
func (db *DB) DeleteObservations(ctx context.Context, jobID int64) error {
	_, err := db.ExecContext(ctx, `DELETE FROM observations WHERE job_id = ?`, jobID)
	return err
}

// Observations returns a segment's observations ordered by time and track.
func (db *DB) Observations(ctx context.Context, segmentID int64) ([]Observation, error) {
	rows, err := db.QueryContext(ctx, `SELECT job_id, segment_id, t_offset_s, track_id, cls, x1, y1, x2, y2, conf, dist_m, ttc_s
		FROM observations WHERE segment_id = ? ORDER BY t_offset_s, track_id`, segmentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Observation
	for rows.Next() {
		var o Observation
		var d, ttc sql.NullFloat64
		if err := rows.Scan(&o.JobID, &o.SegmentID, &o.TOffsetS, &o.TrackID, &o.Cls, &o.X1, &o.Y1, &o.X2, &o.Y2, &o.Conf, &d, &ttc); err != nil {
			return nil, err
		}
		if d.Valid {
			o.DistM = &d.Float64
		}
		if ttc.Valid {
			o.TTCS = &ttc.Float64
		}
		out = append(out, o)
	}
	return out, rows.Err()
}
