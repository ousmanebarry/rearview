package store

import (
	"context"
	"encoding/binary"
	"fmt"
	"math"
)

// KeyframeModel names the embedding model stored with each vector.
const KeyframeModel = "clip-vit-b32"

// Keyframe is one embedded frame: 512 floats, stored little-endian.
type Keyframe struct {
	ID        int64
	JobID     int64
	SegmentID int64
	TOffsetS  float64
	Vector    []float32
}

// EncodeVector packs floats as little-endian float32 bytes.
func EncodeVector(v []float32) []byte {
	b := make([]byte, 4*len(v))
	for i, f := range v {
		binary.LittleEndian.PutUint32(b[4*i:], math.Float32bits(f))
	}
	return b
}

// DecodeVector unpacks little-endian float32 bytes.
func DecodeVector(b []byte) ([]float32, error) {
	if len(b)%4 != 0 {
		return nil, fmt.Errorf("vector blob of %d bytes", len(b))
	}
	v := make([]float32, len(b)/4)
	for i := range v {
		v[i] = math.Float32frombits(binary.LittleEndian.Uint32(b[4*i:]))
	}
	return v, nil
}

// InsertKeyframes writes a batch in one transaction.
func (db *DB) InsertKeyframes(ctx context.Context, kfs []Keyframe) error {
	if len(kfs) == 0 {
		return nil
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, k := range kfs {
		if _, err := tx.ExecContext(ctx, `INSERT INTO keyframes (job_id, segment_id, t_offset_s, model, vector) VALUES (?,?,?,?,?)`,
			nullID(k.JobID), k.SegmentID, k.TOffsetS, KeyframeModel, EncodeVector(k.Vector)); err != nil {
			return fmt.Errorf("insert keyframe: %w", err)
		}
	}
	return tx.Commit()
}

// DeleteKeyframes removes a job's keyframes.
func (db *DB) DeleteKeyframes(ctx context.Context, jobID int64) error {
	_, err := db.ExecContext(ctx, `DELETE FROM keyframes WHERE job_id = ?`, jobID)
	return err
}

// Keyframes loads every stored vector.
func (db *DB) Keyframes(ctx context.Context) ([]Keyframe, error) {
	rows, err := db.QueryContext(ctx, `SELECT id, COALESCE(job_id,0), segment_id, t_offset_s, vector FROM keyframes WHERE model = ? ORDER BY segment_id, t_offset_s`, KeyframeModel)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Keyframe
	for rows.Next() {
		var k Keyframe
		var b []byte
		if err := rows.Scan(&k.ID, &k.JobID, &k.SegmentID, &k.TOffsetS, &b); err != nil {
			return nil, err
		}
		if k.Vector, err = DecodeVector(b); err != nil {
			return nil, err
		}
		out = append(out, k)
	}
	return out, rows.Err()
}
