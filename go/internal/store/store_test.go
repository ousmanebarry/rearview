package store

import (
	"context"
	"path/filepath"
	"slices"
	"testing"
)

// OpenTest opens a fresh database in a temp folder.
func OpenTest(t testing.TB) *DB {
	t.Helper()
	db, err := Open(context.Background(), filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func tables(t *testing.T, db *DB) []string {
	t.Helper()
	rows, err := db.Query(`SELECT name FROM sqlite_master WHERE type = 'table' ORDER BY name`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var n string
		rows.Scan(&n)
		out = append(out, n)
	}
	return out
}

func TestOpenCreatesEveryTable(t *testing.T) {
	db := OpenTest(t)
	got := tables(t, db)
	for _, want := range []string{"trips", "segments", "gps_points", "jobs", "observations", "events", "clips", "labels", "bench_runs", "schema_migrations"} {
		if !slices.Contains(got, want) {
			t.Errorf("missing table %s (have %v)", want, got)
		}
	}
}

func TestOpenTwiceChangesNothing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "r.db")
	ctx := context.Background()
	db, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	first := tables(t, db)
	v1, _ := db.Versions(ctx)
	db.Close()

	db, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	v2, _ := db.Versions(ctx)
	if !slices.Equal(first, tables(t, db)) || !slices.Equal(v1, v2) {
		t.Errorf("second open changed the schema: %v/%v vs %v/%v", first, v1, tables(t, db), v2)
	}
	if len(v1) == 0 || v1[0] != 1 {
		t.Errorf("versions = %v, want to start at 1", v1)
	}
}

func TestPragmas(t *testing.T) {
	db := OpenTest(t)
	var fk int
	var mode string
	db.QueryRow(`PRAGMA foreign_keys`).Scan(&fk)
	db.QueryRow(`PRAGMA journal_mode`).Scan(&mode)
	if fk != 1 || mode != "wal" {
		t.Errorf("foreign_keys=%d journal_mode=%s", fk, mode)
	}
	_, err := db.Exec(`INSERT INTO gps_points(segment_id, t_offset_s, ts_utc, lat, lon) VALUES (999, 0, 'x', 0, 0)`)
	if err == nil {
		t.Error("insert with a missing segment succeeded: foreign keys are off")
	}
}
