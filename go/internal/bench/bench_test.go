package bench

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"rearview/internal/store"
)

func TestReadSet(t *testing.T) {
	p := filepath.Join(t.TempDir(), "set.txt")
	os.WriteFile(p, []byte("# comment\n2026/10/23/a.MP4\n\n  2026/10/23/b.MP4  \n"), 0o644)
	got, err := ReadSet(p)
	if err != nil || len(got) != 2 || got[1] != "2026/10/23/b.MP4" {
		t.Errorf("%v %v", got, err)
	}
}

func TestSaveLoadCompareTable(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(ctx, filepath.Join(t.TempDir(), "r.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	dir := t.TempDir()
	base := Result{Label: "cpu", RanAt: time.Date(2026, 10, 30, 1, 0, 0, 0, time.UTC), GitSHA: "abcdef123",
		FootageS: 3600, MinutesPerHour: 28, FPS: 20, StageMsPerFrame: map[string]float64{"inference": 50}}
	gpu := base
	gpu.Label, gpu.RanAt, gpu.MinutesPerHour = "dml", base.RanAt.Add(time.Hour), 2.5
	for _, r := range []Result{base, gpu} {
		p, err := Save(ctx, db, r, dir)
		if err != nil || !strings.HasSuffix(p, "_abcdef1.json") {
			t.Fatalf("save %s %v", p, err)
		}
	}
	rs, err := Load(ctx, db)
	if err != nil || len(rs) != 2 || rs[1].Label != "dml" {
		t.Fatalf("%+v %v", rs, err)
	}
	var b bytes.Buffer
	Compare(&b, rs[0], rs[1])
	if !strings.Contains(b.String(), "-91.1%") {
		t.Errorf("compare:\n%s", b.String())
	}
	b.Reset()
	Table(&b, rs)
	if !strings.Contains(b.String(), "| 2 | dml | 60 min | 2.5 | 11.2× |") {
		t.Errorf("table:\n%s", b.String())
	}
}
