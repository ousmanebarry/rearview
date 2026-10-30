// Package store owns the SQLite database: opening it with the right pragmas,
// applying the numbered migrations, and the queries the rest of the program uses.
package store

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"net/url"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"

	_ "modernc.org/sqlite" // registers the "sqlite" driver
)

//go:embed migrations/*.sql
var migrations embed.FS

// TimeLayout is how absolute times are stored (UTC, ISO 8601).
const TimeLayout = "2006-01-02T15:04:05Z"

// FormatTime formats t for a TEXT time column.
func FormatTime(t time.Time) string { return t.UTC().Format(TimeLayout) }

// ParseTime parses a TEXT time column.
func ParseTime(s string) (time.Time, error) { return time.Parse(TimeLayout, s) }

// DB wraps the database handle.
type DB struct {
	*sql.DB
}

// Open opens (creating if needed) the database at path with WAL and foreign
// keys turned on for every connection, then applies pending migrations.
func Open(ctx context.Context, dbPath string) (*DB, error) {
	q := url.Values{}
	for _, p := range []string{"foreign_keys(1)", "journal_mode(WAL)", "busy_timeout(10000)", "synchronous(NORMAL)"} {
		q.Add("_pragma", p)
	}
	dsn := "file:" + filepathToURI(dbPath) + "?" + q.Encode()
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", dbPath, err)
	}
	s := &DB{db}
	if err := s.migrate(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate %s: %w", dbPath, err)
	}
	return s, nil
}

func filepathToURI(p string) string {
	p = strings.ReplaceAll(p, `\`, "/")
	return strings.ReplaceAll(p, "?", "%3F")
}

type migration struct {
	version int
	name    string
	sql     string
}

func loadMigrations() ([]migration, error) {
	entries, err := fs.ReadDir(migrations, "migrations")
	if err != nil {
		return nil, err
	}
	var out []migration
	for _, e := range entries {
		name := e.Name()
		num, _, ok := strings.Cut(name, "_")
		v, err := strconv.Atoi(num)
		if !ok || err != nil {
			return nil, fmt.Errorf("migration %q: name must start with a number and _", name)
		}
		b, err := migrations.ReadFile(path.Join("migrations", name))
		if err != nil {
			return nil, err
		}
		out = append(out, migration{v, name, string(b)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].version < out[j].version })
	for i := 1; i < len(out); i++ {
		if out[i].version == out[i-1].version {
			return nil, fmt.Errorf("two migrations numbered %d", out[i].version)
		}
	}
	return out, nil
}

func (s *DB) migrate(ctx context.Context) error {
	if _, err := s.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		version    INTEGER PRIMARY KEY,
		name       TEXT NOT NULL,
		applied_at TEXT NOT NULL)`); err != nil {
		return err
	}
	all, err := loadMigrations()
	if err != nil {
		return err
	}
	for _, m := range all {
		if err := s.apply(ctx, m); err != nil {
			return fmt.Errorf("%s: %w", m.name, err)
		}
	}
	return nil
}

func (s *DB) apply(ctx context.Context, m migration) error {
	tx, err := s.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var n int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM schema_migrations WHERE version = ?`, m.version).Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return nil
	}
	if _, err := tx.ExecContext(ctx, m.sql); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO schema_migrations(version, name, applied_at) VALUES (?, ?, ?)`,
		m.version, m.name, FormatTime(time.Now())); err != nil {
		return err
	}
	return tx.Commit()
}

// Versions returns the applied migration versions in order.
func (s *DB) Versions(ctx context.Context) ([]int, error) {
	rows, err := s.QueryContext(ctx, `SELECT version FROM schema_migrations ORDER BY version`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []int
	for rows.Next() {
		var v int
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
