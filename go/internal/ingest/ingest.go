// Package ingest copies new recordings from a source folder (an SD card or a
// backup) into the library and records them as segments.
package ingest

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"rearview/internal/config"
	"rearview/internal/store"
)

// hashChunk is how much of each end of a file goes into its content hash.
const hashChunk = 4 << 20

// Probe is what ffprobe tells us about a file.
type Probe struct {
	DurationS float64
	Width     int
	Height    int
	FPS       float64
}

// Prober reads a file's duration, resolution and frame rate.
type Prober func(ctx context.Context, path string) (Probe, error)

// Status of one file after an import.
type Status string

const (
	StatusNew     Status = "new"
	StatusSkipped Status = "skipped"
	StatusFailed  Status = "failed"
)

// Result is the outcome for one source file.
type Result struct {
	Source  string
	Status  Status
	Segment store.Segment
	Err     error
}

// Ingester imports files into the library.
type Ingester struct {
	DB        *store.DB
	Library   string
	Probe     Prober
	StableFor time.Duration
	PollEvery time.Duration
	Loc       *time.Location

	front, rear, interior, timeRe  *regexp.Regexp
	timeLayout, lockedDir, parking string

	now  func() time.Time
	open func(string) (io.ReadCloser, error)
}

// New builds an Ingester from the config.
func New(db *store.DB, cfg config.Config, probe Prober) (*Ingester, error) {
	loc, err := time.LoadLocation(cfg.TimeZone)
	if err != nil {
		return nil, fmt.Errorf("time zone %q: %w", cfg.TimeZone, err)
	}
	in := &Ingester{
		DB:         db,
		Library:    cfg.LibraryPath,
		Probe:      probe,
		StableFor:  time.Duration(cfg.Import.StableSeconds * float64(time.Second)),
		PollEvery:  time.Second,
		Loc:        loc,
		timeLayout: cfg.Import.TimeLayout,
		lockedDir:  cfg.Import.LockedDir,
		parking:    cfg.Import.ParkingDir,
		now:        time.Now,
		open:       func(p string) (io.ReadCloser, error) { return os.Open(p) },
	}
	for _, x := range []struct {
		dst **regexp.Regexp
		src string
	}{{&in.front, cfg.Import.FrontPattern}, {&in.rear, cfg.Import.RearPattern}, {&in.interior, cfg.Import.InteriorPattern}, {&in.timeRe, cfg.Import.TimePattern}} {
		if *x.dst, err = regexp.Compile(x.src); err != nil {
			return nil, err
		}
	}
	return in, nil
}

// FindVideos lists the MP4 files under root, sorted by path.
func FindVideos(root string) ([]string, error) {
	var out []string
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.EqualFold(filepath.Ext(p), ".mp4") {
			out = append(out, p)
		}
		return nil
	})
	slices.Sort(out)
	return out, err
}

// ImportDir imports every MP4 under root. One file failing does not stop the rest.
func (in *Ingester) ImportDir(ctx context.Context, root string) ([]Result, error) {
	files, err := FindVideos(root)
	if err != nil {
		return nil, fmt.Errorf("list %s: %w", root, err)
	}
	var out []Result
	for _, f := range files {
		if err := ctx.Err(); err != nil {
			return out, err
		}
		out = append(out, in.ImportFile(ctx, root, f))
	}
	return out, nil
}

// ImportFile imports one file found under root.
func (in *Ingester) ImportFile(ctx context.Context, root, src string) Result {
	r := Result{Source: src}
	seg, skipped, err := in.importFile(ctx, root, src)
	switch {
	case err != nil:
		r.Status, r.Err = StatusFailed, err
	case skipped:
		r.Status, r.Segment = StatusSkipped, seg
	default:
		r.Status, r.Segment = StatusNew, seg
	}
	return r
}

func (in *Ingester) importFile(ctx context.Context, root, src string) (store.Segment, bool, error) {
	size, err := in.waitStable(ctx, src)
	if err != nil {
		return store.Segment{}, false, err
	}
	hash, err := in.hash(src, size)
	if err != nil {
		return store.Segment{}, false, err
	}
	if seg, ok, err := in.DB.SegmentByHash(ctx, hash); err != nil || ok {
		return seg, ok, err
	}
	name := filepath.Base(src)
	camera, err := in.camera(name)
	if err != nil {
		return store.Segment{}, false, err
	}
	pr, err := in.Probe(ctx, src)
	if err != nil {
		return store.Segment{}, false, fmt.Errorf("probe %s: %w", name, err)
	}
	started, local, err := in.startTime(name, src, pr.DurationS)
	if err != nil {
		return store.Segment{}, false, err
	}
	rel, err := in.copyIn(src, local, name, size)
	if err != nil {
		return store.Segment{}, false, err
	}
	seg := store.Segment{
		Path:       rel,
		SourceName: name,
		Hash:       hash,
		SizeBytes:  size,
		Camera:     camera,
		StartedAt:  started,
		DurationS:  pr.DurationS,
		Width:      pr.Width,
		Height:     pr.Height,
		FPS:        pr.FPS,
		Locked:     underDir(root, src, in.lockedDir),
		Parking:    underDir(root, src, in.parking),
		ImportedAt: in.now().UTC(),
	}
	if seg.ID, err = in.DB.InsertSegment(ctx, seg); err != nil {
		os.Remove(filepath.Join(in.Library, filepath.FromSlash(rel)))
		return store.Segment{}, false, err
	}
	return seg, false, nil
}

// waitStable returns the file's size once it has not changed for StableFor.
// A file last modified longer ago than that is stable straight away.
func (in *Ingester) waitStable(ctx context.Context, path string) (int64, error) {
	st, err := os.Stat(path)
	if err != nil {
		return 0, err
	}
	size := st.Size()
	since := st.ModTime()
	for {
		if in.now().Sub(since) >= in.StableFor {
			return size, nil
		}
		select {
		case <-ctx.Done():
			return 0, ctx.Err()
		case <-time.After(in.PollEvery):
		}
		st, err := os.Stat(path)
		if err != nil {
			return 0, err
		}
		if st.Size() != size {
			size, since = st.Size(), in.now()
		} else if st.ModTime().After(since) {
			since = st.ModTime()
		}
	}
}

// Hash is SHA-256 over the first and last 4 MB plus the size, so it is quick
// on 650 MB files and still tells loop recordings apart.
func Hash(r io.ReaderAt, size int64) (string, error) {
	h := sha256.New()
	head := min(size, hashChunk)
	if _, err := io.Copy(h, io.NewSectionReader(r, 0, head)); err != nil {
		return "", err
	}
	if tail := min(hashChunk, size-head); tail > 0 {
		if _, err := io.Copy(h, io.NewSectionReader(r, size-tail, tail)); err != nil {
			return "", err
		}
	}
	h.Write(binary.LittleEndian.AppendUint64(nil, uint64(size)))
	return hex.EncodeToString(h.Sum(nil)), nil
}

func (in *Ingester) hash(path string, size int64) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	s, err := Hash(f, size)
	if err != nil {
		return "", fmt.Errorf("hash %s: %w", path, err)
	}
	return s, nil
}

func (in *Ingester) camera(name string) (string, error) {
	switch {
	case in.front.MatchString(name):
		return "front", nil
	case in.rear.MatchString(name):
		return "rear", nil
	case in.interior.MatchString(name):
		return "interior", nil
	}
	return "", fmt.Errorf("%s: no camera pattern matches the file name", name)
}

// startTime reads the camera-local start time from the file name; without
// one it falls back to the modification time minus the duration.
func (in *Ingester) startTime(name, path string, durationS float64) (utc, local time.Time, err error) {
	if m := in.timeRe.FindStringSubmatch(name); len(m) > 1 {
		t, err := time.ParseInLocation(in.timeLayout, m[1], in.Loc)
		if err == nil {
			return t.UTC(), t, nil
		}
	}
	st, err := os.Stat(path)
	if err != nil {
		return utc, local, err
	}
	t := st.ModTime().Add(-time.Duration(durationS * float64(time.Second))).Truncate(time.Second)
	return t.UTC(), t.In(in.Loc), nil
}

// copyIn copies src to library/YYYY/MM/DD/name through a temp file renamed at
// the end, so a failure never leaves a partial file under the real name.
func (in *Ingester) copyIn(src string, local time.Time, name string, size int64) (string, error) {
	relDir := local.Format("2006/01/02")
	dir := filepath.Join(in.Library, filepath.FromSlash(relDir))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	dst := filepath.Join(dir, name)
	for i := 1; ; i++ {
		if _, err := os.Stat(dst); errors.Is(err, fs.ErrNotExist) {
			break
		}
		// Same name, different content: a crash after the rename but before
		// the row was inserted, or a name reused by the camera.
		if same, _ := sameContent(dst, src, size); same {
			return relDir + "/" + filepath.Base(dst), nil
		}
		ext := filepath.Ext(name)
		dst = filepath.Join(dir, strings.TrimSuffix(name, ext)+"_"+strconv.Itoa(i)+ext)
	}
	tmp := dst + ".part"
	if err := in.copyFile(src, tmp, size); err != nil {
		os.Remove(tmp)
		return "", err
	}
	if err := os.Rename(tmp, dst); err != nil {
		os.Remove(tmp)
		return "", fmt.Errorf("rename into library: %w", err)
	}
	return relDir + "/" + filepath.Base(dst), nil
}

func (in *Ingester) copyFile(src, dst string, size int64) error {
	r, err := in.open(src)
	if err != nil {
		return err
	}
	defer r.Close()
	w, err := os.Create(dst)
	if err != nil {
		return err
	}
	n, err := io.Copy(w, r)
	if err == nil && n != size {
		err = fmt.Errorf("copied %d of %d bytes", n, size)
	}
	if err == nil {
		err = w.Sync()
	}
	if cerr := w.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return fmt.Errorf("copy %s: %w", filepath.Base(src), err)
	}
	return nil
}

func sameContent(a, b string, size int64) (bool, error) {
	st, err := os.Stat(a)
	if err != nil || st.Size() != size {
		return false, err
	}
	fa, err := os.Open(a)
	if err != nil {
		return false, err
	}
	defer fa.Close()
	fb, err := os.Open(b)
	if err != nil {
		return false, err
	}
	defer fb.Close()
	ha, err := Hash(fa, size)
	if err != nil {
		return false, err
	}
	hb, err := Hash(fb, size)
	return ha == hb, err
}

// underDir reports whether path sits inside a folder called dir below root.
func underDir(root, path, dir string) bool {
	if dir == "" {
		return false
	}
	rel, err := filepath.Rel(root, filepath.Dir(path))
	if err != nil {
		return false
	}
	for _, part := range strings.Split(filepath.ToSlash(rel), "/") {
		if strings.EqualFold(part, dir) {
			return true
		}
	}
	return false
}

// CleanPartials removes .part files left in the library by a crash.
func CleanPartials(library string) error {
	return filepath.WalkDir(library, func(p string, d fs.DirEntry, err error) error {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.HasSuffix(p, ".part") {
			return os.Remove(p)
		}
		return nil
	})
}

// FFprobe reads duration, resolution and frame rate with ffprobe.
func FFprobe(ctx context.Context, path string) (Probe, error) {
	out, err := exec.CommandContext(ctx, "ffprobe", "-v", "error", "-print_format", "json",
		"-show_format", "-show_streams", "-select_streams", "v:0", path).Output()
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return Probe{}, fmt.Errorf("ffprobe: %w: %s", err, strings.TrimSpace(string(ee.Stderr)))
		}
		return Probe{}, fmt.Errorf("ffprobe: %w", err)
	}
	return parseFFprobe(out)
}

func parseFFprobe(b []byte) (Probe, error) {
	var v struct {
		Streams []struct {
			Width        int    `json:"width"`
			Height       int    `json:"height"`
			AvgFrameRate string `json:"avg_frame_rate"`
			Duration     string `json:"duration"`
		} `json:"streams"`
		Format struct {
			Duration string `json:"duration"`
		} `json:"format"`
	}
	if err := json.Unmarshal(b, &v); err != nil {
		return Probe{}, fmt.Errorf("parse ffprobe output: %w", err)
	}
	if len(v.Streams) == 0 {
		return Probe{}, errors.New("no video stream")
	}
	s := v.Streams[0]
	p := Probe{Width: s.Width, Height: s.Height}
	if num, den, ok := strings.Cut(s.AvgFrameRate, "/"); ok {
		n, _ := strconv.ParseFloat(num, 64)
		d, _ := strconv.ParseFloat(den, 64)
		if d > 0 {
			p.FPS = n / d
		}
	}
	d := v.Format.Duration
	if d == "" {
		d = s.Duration
	}
	p.DurationS, _ = strconv.ParseFloat(d, 64)
	return p, nil
}
