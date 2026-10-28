// Package labels reads and writes the hand-labelled ground-truth events in
// testdata/labels/events.csv (columns ts_utc,camera,type,note).
package labels

import (
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"
	"time"
)

// Types are the event types from the events table in docs/plan/data-model.md.
var Types = []string{"hard_brake", "swerve", "g_spike", "near_miss", "following_close", "tailgated"}

// Cameras are the camera values allowed by the segments table.
var Cameras = []string{"front", "rear", "interior"}

var header = []string{"ts_utc", "camera", "type", "note"}

const tsLayout = "2006-01-02T15:04:05Z"

// ErrDuplicate is returned by Append when a label with the same time, camera and type exists.
var ErrDuplicate = errors.New("duplicate label")

type Label struct {
	TS     time.Time // always UTC, whole seconds
	Camera string
	Type   string
	Note   string
}

// ParseTime accepts RFC 3339 with Z or an offset, or a wall-clock time
// ("2006-01-02T15:04:05" or "2006-01-02 15:04:05") read in loc. The result is UTC.
// A wall-clock time inside the repeated hour when DST ends resolves to the first occurrence.
func ParseTime(s string, loc *time.Location) (time.Time, error) {
	s = strings.TrimSpace(s)
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t.UTC().Truncate(time.Second), nil
	}
	for _, layout := range []string{"2006-01-02T15:04:05", "2006-01-02 15:04:05"} {
		if t, err := time.ParseInLocation(layout, s, loc); err == nil {
			return t.UTC(), nil
		}
	}
	return time.Time{}, fmt.Errorf("parse time %q: want RFC 3339 (2026-10-23T17:07:12Z) or local (2026-10-23T13:07:12)", s)
}

func (l Label) Validate() error {
	if l.TS.IsZero() {
		return errors.New("missing time")
	}
	if !slices.Contains(Cameras, l.Camera) {
		return fmt.Errorf("camera %q: must be one of %s", l.Camera, strings.Join(Cameras, ", "))
	}
	if !slices.Contains(Types, l.Type) {
		return fmt.Errorf("type %q: must be one of %s", l.Type, strings.Join(Types, ", "))
	}
	if strings.ContainsAny(l.Note, "\r\n") {
		return errors.New("note must be a single line")
	}
	return nil
}

func (l Label) sameEvent(o Label) bool {
	return l.TS.Equal(o.TS) && l.Camera == o.Camera && l.Type == o.Type
}

// Load reads every label in path. A missing file means no labels yet.
func Load(path string) ([]Label, error) {
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("open labels: %w", err)
	}
	defer f.Close()

	r := csv.NewReader(f)
	r.FieldsPerRecord = len(header)
	first, err := r.Read()
	if errors.Is(err, io.EOF) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read labels header: %w", err)
	}
	if !slices.Equal(first, header) {
		return nil, fmt.Errorf("labels header is %v, want %v", first, header)
	}

	var out []Label
	for {
		rec, err := r.Read()
		if errors.Is(err, io.EOF) {
			return out, nil
		}
		if err != nil {
			return nil, fmt.Errorf("read labels: %w", err)
		}
		line, _ := r.FieldPos(0)
		ts, err := time.Parse(tsLayout, rec[0])
		if err != nil {
			return nil, fmt.Errorf("labels line %d: bad ts_utc %q", line, rec[0])
		}
		l := Label{TS: ts, Camera: rec[1], Type: rec[2], Note: rec[3]}
		if err := l.Validate(); err != nil {
			return nil, fmt.Errorf("labels line %d: %w", line, err)
		}
		out = append(out, l)
	}
}

// Append validates l and adds it to path, creating the file with a header if needed.
func Append(path string, l Label) error {
	l.TS = l.TS.UTC().Truncate(time.Second)
	if err := l.Validate(); err != nil {
		return err
	}
	existing, err := Load(path)
	if err != nil {
		return err
	}
	for _, e := range existing {
		if e.sameEvent(l) {
			return fmt.Errorf("%w: %s %s %s", ErrDuplicate, l.TS.Format(tsLayout), l.Camera, l.Type)
		}
	}

	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return fmt.Errorf("open labels for append: %w", err)
	}
	st, err := f.Stat()
	if err != nil {
		f.Close()
		return fmt.Errorf("stat labels: %w", err)
	}
	w := csv.NewWriter(f)
	if st.Size() == 0 {
		w.Write(header)
	}
	w.Write([]string{l.TS.Format(tsLayout), l.Camera, l.Type, l.Note})
	w.Flush()
	if err := w.Error(); err != nil {
		f.Close()
		return fmt.Errorf("write label: %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("close labels: %w", err)
	}
	return nil
}

// FormatTS renders a label time the way it is stored in the CSV.
func FormatTS(t time.Time) string { return t.UTC().Format(tsLayout) }
