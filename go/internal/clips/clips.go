// Package clips cuts event clips out of library files with ffmpeg.
package clips

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Part is a stretch of one file, in seconds from its start.
type Part struct {
	Path   string
	StartS float64
	EndS   float64
}

// Options controls how a clip is cut.
type Options struct {
	// Exact re-encodes so the cut lands on the exact frame; otherwise the
	// stream is copied and the cut starts at the keyframe before StartS.
	Exact bool
	// FFmpeg is the ffmpeg binary; "ffmpeg" when empty.
	FFmpeg string
}

func (o Options) bin() string {
	if o.FFmpeg == "" {
		return "ffmpeg"
	}
	return o.FFmpeg
}

func secs(v float64) string { return strconv.FormatFloat(v, 'f', 3, 64) }

func run(ctx context.Context, bin string, args ...string) error {
	cmd := exec.CommandContext(ctx, bin, append([]string{"-hide_banner", "-loglevel", "error", "-y"}, args...)...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("ffmpeg %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return nil
}

// Cut writes parts, in order, as one clip at out. Parts from consecutive
// files are cut separately and joined with the concat demuxer.
func Cut(ctx context.Context, parts []Part, out string, o Options) error {
	if len(parts) == 0 {
		return errors.New("no parts to cut")
	}
	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		return err
	}
	tmp := out + ".part.mp4"
	defer os.Remove(tmp)
	if len(parts) == 1 {
		if err := cutOne(ctx, parts[0], tmp, o); err != nil {
			return err
		}
		return os.Rename(tmp, out)
	}
	dir, err := os.MkdirTemp(filepath.Dir(out), ".cut-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	var list strings.Builder
	for i, p := range parts {
		name := filepath.Join(dir, fmt.Sprintf("%02d.mp4", i))
		if err := cutOne(ctx, p, name, o); err != nil {
			return err
		}
		fmt.Fprintf(&list, "file '%s'\n", strings.ReplaceAll(filepath.ToSlash(name), "'", `'\''`))
	}
	listPath := filepath.Join(dir, "list.txt")
	if err := os.WriteFile(listPath, []byte(list.String()), 0o644); err != nil {
		return err
	}
	if err := run(ctx, o.bin(), "-f", "concat", "-safe", "0", "-i", listPath, "-c", "copy", "-movflags", "+faststart", tmp); err != nil {
		return err
	}
	return os.Rename(tmp, out)
}

func cutOne(ctx context.Context, p Part, out string, o Options) error {
	if p.EndS <= p.StartS {
		return fmt.Errorf("empty part %s %.2f-%.2f", p.Path, p.StartS, p.EndS)
	}
	args := []string{"-ss", secs(p.StartS), "-i", p.Path, "-t", secs(p.EndS - p.StartS), "-map", "0:v:0", "-map", "0:a?"}
	if o.Exact {
		args = append(args, "-c:v", "libx264", "-preset", "veryfast", "-crf", "20", "-c:a", "aac")
	} else {
		args = append(args, "-c", "copy", "-avoid_negative_ts", "make_zero")
	}
	return run(ctx, o.bin(), append(args, "-movflags", "+faststart", out)...)
}

// SideBySide writes front and rear next to each other (each scaled to
// 1280x720, rear mirrored back to a driver's view is not applied), re-encoded.
func SideBySide(ctx context.Context, front, rear []Part, out string, o Options) error {
	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		return err
	}
	dir, err := os.MkdirTemp(filepath.Dir(out), ".sbs-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	f, r := filepath.Join(dir, "front.mp4"), filepath.Join(dir, "rear.mp4")
	exact := Options{Exact: true, FFmpeg: o.FFmpeg}
	if err := Cut(ctx, front, f, exact); err != nil {
		return fmt.Errorf("front: %w", err)
	}
	if err := Cut(ctx, rear, r, exact); err != nil {
		return fmt.Errorf("rear: %w", err)
	}
	tmp := out + ".part.mp4"
	defer os.Remove(tmp)
	err = run(ctx, o.bin(), "-i", f, "-i", r, "-filter_complex",
		"[0:v]scale=1280:720,setsar=1[a];[1:v]scale=1280:720,setsar=1[b];[a][b]hstack=inputs=2:shortest=1[v]",
		"-map", "[v]", "-c:v", "libx264", "-preset", "veryfast", "-crf", "21", "-movflags", "+faststart", tmp)
	if err != nil {
		return err
	}
	return os.Rename(tmp, out)
}

// Name is the clip file name: YYYYMMDD_HHMMSS_<type>_<event id>.mp4 in local time.
func Name(t time.Time, typ string, eventID int64, loc *time.Location) string {
	return fmt.Sprintf("%s_%s_%d.mp4", t.In(loc).Format("20060102_150405"), typ, eventID)
}
