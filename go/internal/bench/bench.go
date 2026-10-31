// Package bench runs a fixed set of footage through the full pipeline and
// records how fast it went, so every performance change has a number.
package bench

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"rearview/internal/pipeline"
	"rearview/internal/store"
)

// Result is one benchmark run.
type Result struct {
	Label           string             `json:"label"`
	RanAt           time.Time          `json:"ran_at"`
	GitSHA          string             `json:"git_sha"`
	Files           int                `json:"files"`
	FootageS        float64            `json:"footage_s"`
	ElapsedS        float64            `json:"elapsed_s"`
	MinutesPerHour  float64            `json:"minutes_per_hour"`
	Frames          int64              `json:"frames"`
	FPS             float64            `json:"fps"`
	StageMsPerFrame map[string]float64 `json:"stage_ms_per_frame"`
	StageFPS        map[string]float64 `json:"stage_fps"`
	WorkerUtil      float64            `json:"worker_util"`
	PeakMemMB       float64            `json:"peak_mem_mb"`
	GPUUtilPct      *float64           `json:"gpu_util_pct,omitempty"`
	Failed          int                `json:"failed"`
	Config          map[string]any     `json:"config"`
}

// ReadSet reads a set file: one library-relative path per line, # comments.
func ReadSet(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var out []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		l := strings.TrimSpace(sc.Text())
		if l == "" || strings.HasPrefix(l, "#") {
			continue
		}
		out = append(out, l)
	}
	return out, sc.Err()
}

// Run copies the set's segments into a scratch database, plans one whole-file
// analyze job per file, runs the pipeline with opt and measures it.
func Run(ctx context.Context, main *store.DB, set []string, opt pipeline.Options, scratch string) (Result, error) {
	db, err := store.Open(ctx, filepath.Join(scratch, "bench.db"))
	if err != nil {
		return Result{}, err
	}
	defer db.Close()
	all, err := main.Segments(ctx, 0)
	if err != nil {
		return Result{}, err
	}
	byPath := map[string]store.Segment{}
	for _, s := range all {
		byPath[s.Path] = s
	}
	var r Result
	for _, p := range set {
		s, ok := byPath[p]
		if !ok {
			return r, fmt.Errorf("bench set file %s is not in the library (import it first)", p)
		}
		s.TripID, s.ID = 0, 0
		id, err := db.InsertSegment(ctx, s)
		if err != nil {
			return r, err
		}
		if _, err := db.InsertJob(ctx, store.Job{SegmentID: id, Kind: "analyze", Priority: 2}); err != nil {
			return r, err
		}
		r.Files++
		r.FootageS += s.DurationS
	}

	gpu := startGPUSampler(ctx)
	start := time.Now()
	st, err := pipeline.Run(ctx, db, opt)
	r.ElapsedS = time.Since(start).Seconds()
	if g := gpu.stop(); g != nil {
		r.GPUUtilPct = g
	}
	if err != nil {
		return r, err
	}
	r.RanAt = start.UTC()
	r.Frames = st.Frames
	r.Failed = st.Failed
	r.FPS = float64(st.Frames) / r.ElapsedS
	r.MinutesPerHour = r.ElapsedS / 60 / (r.FootageS / 3600)
	r.StageMsPerFrame, r.StageFPS = map[string]float64{}, map[string]float64{}
	if st.Frames > 0 {
		for k, v := range st.StagesMs {
			r.StageMsPerFrame[k] = round(v/float64(st.Frames), 3)
			if v > 0 {
				r.StageFPS[k] = round(float64(st.Frames)*1000/v, 1)
			}
		}
	}
	var busy time.Duration
	for _, w := range st.Workers {
		busy += w.BusyTime
	}
	if n := len(st.Workers); n > 0 {
		r.WorkerUtil = round(busy.Seconds()/(float64(n)*r.ElapsedS), 3)
	}
	r.PeakMemMB = st.PeakMemMB
	r.MinutesPerHour = round(r.MinutesPerHour, 2)
	r.FPS = round(r.FPS, 1)
	r.ElapsedS = round(r.ElapsedS, 1)
	return r, nil
}

func round(v float64, d int) float64 {
	k := math.Pow(10, float64(d))
	return math.Round(v*k) / k
}

// Save writes the result to bench_runs and bench/results/<date>_<sha>.json.
func Save(ctx context.Context, db *store.DB, r Result, resultsDir string) (string, error) {
	cfg, _ := json.Marshal(r.Config)
	res, _ := json.MarshalIndent(r, "", "  ")
	if _, err := db.ExecContext(ctx, `INSERT INTO bench_runs (ran_at, git_sha, config_json, results_json) VALUES (?,?,?,?)`,
		store.FormatTime(r.RanAt), r.GitSHA, string(cfg), string(res)); err != nil {
		return "", err
	}
	if err := os.MkdirAll(resultsDir, 0o755); err != nil {
		return "", err
	}
	name := fmt.Sprintf("%s_%s.json", r.RanAt.Format("20060102T150405Z"), shortSHA(r.GitSHA))
	path := filepath.Join(resultsDir, name)
	return path, os.WriteFile(path, append(res, '\n'), 0o644)
}

func shortSHA(s string) string {
	if len(s) > 7 {
		return s[:7]
	}
	if s == "" {
		return "nogit"
	}
	return s
}

// Load returns stored runs, oldest first.
func Load(ctx context.Context, db *store.DB) ([]Result, error) {
	rows, err := db.QueryContext(ctx, `SELECT results_json FROM bench_runs ORDER BY ran_at, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Result
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return nil, err
		}
		var r Result
		if err := json.Unmarshal([]byte(s), &r); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// Compare prints two runs side by side.
func Compare(w io.Writer, a, b Result) {
	row := func(name string, x, y float64, unit string) {
		change := ""
		if x != 0 {
			change = fmt.Sprintf("%+.1f%%", (y-x)/x*100)
		}
		fmt.Fprintf(w, "%-22s %12.2f %12.2f %9s %s\n", name, x, y, change, unit)
	}
	fmt.Fprintf(w, "%-22s %12s %12s\n", "", label(a), label(b))
	row("minutes per hour", a.MinutesPerHour, b.MinutesPerHour, "")
	row("frames per second", a.FPS, b.FPS, "")
	row("worker utilization", a.WorkerUtil, b.WorkerUtil, "")
	row("peak memory", a.PeakMemMB, b.PeakMemMB, "MB")
	ga, gb := 0.0, 0.0
	if a.GPUUtilPct != nil {
		ga = *a.GPUUtilPct
	}
	if b.GPUUtilPct != nil {
		gb = *b.GPUUtilPct
	}
	row("GPU utilization", ga, gb, "%")
	keys := map[string]bool{}
	for k := range a.StageMsPerFrame {
		keys[k] = true
	}
	for k := range b.StageMsPerFrame {
		keys[k] = true
	}
	var ks []string
	for k := range keys {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	for _, k := range ks {
		row(k+" ms/frame", a.StageMsPerFrame[k], b.StageMsPerFrame[k], "")
	}
}

func label(r Result) string {
	if r.Label != "" {
		return r.Label
	}
	return shortSHA(r.GitSHA)
}

// Table renders the history as markdown: change, minutes per hour, speedup
// against the first run.
func Table(w io.Writer, rs []Result) {
	fmt.Fprintln(w, "| Run | Change | Footage | Minutes per hour of footage | Speedup vs baseline | Frames/s | Inference ms/frame |")
	fmt.Fprintln(w, "| --- | --- | --- | --- | --- | --- | --- |")
	if len(rs) == 0 {
		return
	}
	base := rs[0].MinutesPerHour
	for i, r := range rs {
		speed := "1.0×"
		if r.MinutesPerHour > 0 {
			speed = fmt.Sprintf("%.1f×", base/r.MinutesPerHour)
		}
		fmt.Fprintf(w, "| %d | %s | %.0f min | %.1f | %s | %.1f | %.1f |\n", i+1, label(r), r.FootageS/60, r.MinutesPerHour, speed, r.FPS, r.StageMsPerFrame["inference"])
	}
}

// GitSHA returns the current commit, or "" outside a repo.
func GitSHA() string {
	out, err := exec.Command("git", "rev-parse", "HEAD").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

type gpuSampler struct {
	cmd     *exec.Cmd
	mu      sync.Mutex
	samples []float64
	done    chan struct{}
}

// startGPUSampler reads Windows' "GPU Engine" counters (3D and compute
// engines, all processes) about every second. AMD drivers expose them too.
// Elsewhere it does nothing.
func startGPUSampler(ctx context.Context) *gpuSampler {
	g := &gpuSampler{done: make(chan struct{})}
	if runtime.GOOS != "windows" {
		close(g.done)
		return g
	}
	script := `while ($true) { $s = (Get-Counter '\GPU Engine(*engtype_3D)\Utilization Percentage','\GPU Engine(*engtype_Compute*)\Utilization Percentage' -ErrorAction SilentlyContinue).CounterSamples; ` +
		`$t = @{}; foreach ($c in $s) { $k = ($c.InstanceName -split 'engtype_')[1]; $t[$k] += $c.CookedValue }; ` +
		`if ($t.Count) { [Console]::Out.WriteLine((($t.Values | Measure-Object -Maximum).Maximum)) } }`
	g.cmd = exec.CommandContext(ctx, "powershell", "-NoProfile", "-Command", script)
	out, err := g.cmd.StdoutPipe()
	if err != nil || g.cmd.Start() != nil {
		close(g.done)
		g.cmd = nil
		return g
	}
	go func() {
		defer close(g.done)
		sc := bufio.NewScanner(out)
		for sc.Scan() {
			if v, err := strconv.ParseFloat(strings.TrimSpace(sc.Text()), 64); err == nil {
				g.mu.Lock()
				g.samples = append(g.samples, math.Min(100, v))
				g.mu.Unlock()
			}
		}
	}()
	return g
}

func (g *gpuSampler) stop() *float64 {
	if g.cmd != nil {
		_ = g.cmd.Process.Kill()
		<-g.done
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if len(g.samples) == 0 {
		return nil
	}
	var sum float64
	for _, v := range g.samples {
		sum += v
	}
	avg := round(sum/float64(len(g.samples)), 1)
	return &avg
}
