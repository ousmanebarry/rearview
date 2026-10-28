package labels

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	_ "time/tzdata"

	"github.com/google/go-cmp/cmp"
)

func newYork(t *testing.T) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	return loc
}

func utc(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic(err)
	}
	return t.UTC()
}

func TestParseTime(t *testing.T) {
	loc := newYork(t)
	tests := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{in: "2026-10-23T17:07:12Z", want: "2026-10-23T17:07:12Z"},
		{in: "2026-10-23T13:07:12-04:00", want: "2026-10-23T17:07:12Z"},
		{in: "2026-10-23T17:07:12.900Z", want: "2026-10-23T17:07:12Z"},
		{in: "2026-10-23T13:07:12", want: "2026-10-23T17:07:12Z"}, // EDT, UTC-4
		{in: "2026-10-23 13:07:12", want: "2026-10-23T17:07:12Z"},
		{in: "2026-11-02T13:00:00", want: "2026-11-02T18:00:00Z"}, // EST after DST ends Nov 1
		{in: "2026-11-01T01:30:00", want: "2026-11-01T05:30:00Z"}, // repeated hour: first (EDT) occurrence
		{in: "  2026-10-23T13:07:12  ", want: "2026-10-23T17:07:12Z"},
		{in: "2026-10-23", wantErr: true},
		{in: "13:07:12", wantErr: true},
		{in: "", wantErr: true},
	}
	for _, tt := range tests {
		got, err := ParseTime(tt.in, loc)
		if tt.wantErr {
			if err == nil {
				t.Errorf("ParseTime(%q) = %v, want error", tt.in, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("ParseTime(%q): %v", tt.in, err)
			continue
		}
		if !got.Equal(utc(tt.want)) || got.Location() != time.UTC {
			t.Errorf("ParseTime(%q) = %v, want %s", tt.in, got, tt.want)
		}
	}
}

func TestValidate(t *testing.T) {
	ok := Label{TS: utc("2026-10-23T17:07:12Z"), Camera: "front", Type: "near_miss"}
	tests := []struct {
		name    string
		mutate  func(*Label)
		wantErr string
	}{
		{name: "valid", mutate: func(*Label) {}},
		{name: "valid with note", mutate: func(l *Label) { l.Note = `cut off, "white van"` }},
		{name: "zero time", mutate: func(l *Label) { l.TS = time.Time{} }, wantErr: "missing time"},
		{name: "bad camera", mutate: func(l *Label) { l.Camera = "side" }, wantErr: "camera"},
		{name: "empty camera", mutate: func(l *Label) { l.Camera = "" }, wantErr: "camera"},
		{name: "bad type", mutate: func(l *Label) { l.Type = "crash" }, wantErr: "type"},
		{name: "multiline note", mutate: func(l *Label) { l.Note = "a\nb" }, wantErr: "single line"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := ok
			tt.mutate(&l)
			err := l.Validate()
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("err = %v, want it to mention %q", err, tt.wantErr)
			}
		})
	}
}

func TestAppendAndLoad(t *testing.T) {
	path := filepath.Join(t.TempDir(), "events.csv")

	got, err := Load(path)
	if err != nil || got != nil {
		t.Fatalf("Load(missing) = %v, %v; want nil, nil", got, err)
	}

	want := []Label{
		{TS: utc("2026-10-23T17:07:12Z"), Camera: "front", Type: "hard_brake", Note: "src=suggested"},
		{TS: utc("2026-10-24T01:15:00Z"), Camera: "rear", Type: "tailgated", Note: `src=watched, "black SUV", very close`},
	}
	for _, l := range want {
		if err := Append(path, l); err != nil {
			t.Fatalf("Append: %v", err)
		}
	}

	got, err = Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Fatalf("round trip mismatch (-want +got):\n%s", diff)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(string(raw), "ts_utc,camera,type,note"); n != 1 {
		t.Fatalf("header written %d times:\n%s", n, raw)
	}
	if !strings.HasPrefix(string(raw), "ts_utc,camera,type,note\n2026-10-23T17:07:12Z,front,hard_brake,src=suggested\n") {
		t.Fatalf("unexpected file contents:\n%s", raw)
	}
}

func TestAppendRejectsDuplicateAndInvalid(t *testing.T) {
	path := filepath.Join(t.TempDir(), "events.csv")
	l := Label{TS: utc("2026-10-23T17:07:12Z"), Camera: "front", Type: "near_miss"}
	if err := Append(path, l); err != nil {
		t.Fatal(err)
	}

	dup := l
	dup.Note = "different note, same event"
	if err := Append(path, dup); !errors.Is(err, ErrDuplicate) {
		t.Fatalf("err = %v, want ErrDuplicate", err)
	}

	otherCam := l
	otherCam.Camera = "rear"
	if err := Append(path, otherCam); err != nil {
		t.Fatalf("same time on another camera should be allowed: %v", err)
	}

	bad := l
	bad.Type = "crash"
	if err := Append(path, bad); err == nil {
		t.Fatal("want error for invalid type")
	}

	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d labels, want 2", len(got))
	}
}

func TestLoadRejectsBadFiles(t *testing.T) {
	tests := map[string]string{
		"wrong header": "time,camera,type,note\n",
		"bad time":     "ts_utc,camera,type,note\n2026-10-23 17:07:12,front,near_miss,\n",
		"bad type":     "ts_utc,camera,type,note\n2026-10-23T17:07:12Z,front,crash,\n",
		"short row":    "ts_utc,camera,type,note\n2026-10-23T17:07:12Z,front\n",
	}
	for name, content := range tests {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "events.csv")
			if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
				t.Fatal(err)
			}
			if _, err := Load(path); err == nil {
				t.Fatal("want error")
			}
		})
	}
}
