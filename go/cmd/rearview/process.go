package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
	"golang.org/x/term"

	"rearview/internal/config"
	"rearview/internal/engine"
	"rearview/internal/fusion"
	"rearview/internal/pipeline"
	"rearview/internal/queue"
	"rearview/internal/store"
)

func newProcessCmd() *cobra.Command {
	var (
		workers     int
		onlyWindows bool
		device      string
		fps         float64
		retryFailed bool
		plain       bool
		embed       bool
	)
	cmd := &cobra.Command{
		Use:   "process [--workers 2] [--only-windows] [--device cpu|dml|migraphx] [--embed]",
		Short: "Run the engine over the library: plan jobs, analyse in parallel, save observations",
		Long: "Plan analyze jobs for segments that have none (locked files first, then motion-event\n" +
			"windows, then the rest), then run them on --workers engine processes. Interrupted runs\n" +
			"resume where they stopped. With --embed, compute CLIP keyframe vectors for search instead.\n" +
			"A live progress view is shown in a terminal; plain log lines otherwise (or with --plain).",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt)
			defer stop()
			cfg, db, err := openDB(ctx, cmd)
			if err != nil {
				return err
			}
			defer db.Close()
			if cmd.Flags().Changed("workers") {
				cfg.Engine.Workers = workers
			}
			if cmd.Flags().Changed("device") {
				cfg.Engine.Device = device
			}
			if cmd.Flags().Changed("fps") {
				cfg.Engine.FPS = fps
			}
			if err := cfg.Validate(); err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			kind := "analyze"
			var planned int
			if embed {
				kind = "embed_frames"
				planned, err = queue.PlanEmbed(ctx, db)
			} else {
				planned, err = queue.Plan(ctx, db, cfg, onlyWindows)
			}
			if err != nil {
				return fmt.Errorf("plan jobs: %w", err)
			}
			if retryFailed {
				if _, err := db.RetryFailed(ctx); err != nil {
					return err
				}
			}
			fmt.Fprintf(out, "planned %d new %s jobs\n", planned, kind)
			opt, closeLog, err := pipelineOptions(cfg, kind)
			if err != nil {
				return err
			}
			defer closeLog()
			tty := !plain && isTerminal(out)
			if !tty {
				opt.Log = out
			}
			st, err := runWithProgress(ctx, db, opt, out, tty)
			fmt.Fprintf(out, "done %d, failed %d, interrupted %d, %d frames in %s\n",
				st.Done, st.Failed, st.Interrupted, st.Frames, st.Elapsed.Round(time.Second))
			if err != nil {
				return err
			}
			if kind == "analyze" && st.Done > 0 {
				n, err := fusion.Rebuild(ctx, db, fusion.Config{NearMiss: cfg.NearMiss, Following: cfg.Following})
				if err != nil {
					return fmt.Errorf("fuse events: %w", err)
				}
				fmt.Fprintf(out, "%d vision events\n", n)
			}
			return nil
		},
	}
	cmd.Flags().IntVar(&workers, "workers", 2, "engine processes to run in parallel")
	cmd.Flags().BoolVar(&onlyWindows, "only-windows", false, "plan only locked files and motion-event windows")
	cmd.Flags().StringVar(&device, "device", "cpu", "inference device: cpu, dml or migraphx")
	cmd.Flags().Float64Var(&fps, "fps", 10, "frames analysed per second of video")
	cmd.Flags().BoolVar(&retryFailed, "retry-failed", false, "put failed jobs back in the queue")
	cmd.Flags().BoolVar(&plain, "plain", false, "plain log lines instead of the progress view")
	cmd.Flags().BoolVar(&embed, "embed", false, "compute CLIP keyframe vectors for search")
	return cmd
}

// engineGeometryArgs passes the distance and TTC settings to the engine.
func engineGeometryArgs(cfg config.Config) []string {
	var corners []string
	for _, c := range cfg.NearMiss.PathTrapezoid {
		corners = append(corners, fmt.Sprintf("%g,%g", c[0], c[1]))
	}
	names := make([]string, 0, len(cfg.Geometry.ClassHeights))
	for k := range cfg.Geometry.ClassHeights {
		names = append(names, k)
	}
	sort.Strings(names)
	var heights []string
	for _, k := range names {
		heights = append(heights, fmt.Sprintf("%s=%g", k, cfg.Geometry.ClassHeights[k]))
	}
	args := []string{"--path", strings.Join(corners, ";"), "--min-box-px", fmt.Sprint(cfg.NearMiss.MinBoxPx),
		"--hfov", fmt.Sprint(cfg.Geometry.HFOVDeg)}
	if len(heights) > 0 {
		args = append(args, "--class-heights", strings.Join(heights, ","))
	}
	if cfg.Geometry.CalibrationDir != "" {
		args = append(args, "--calibration", cfg.Geometry.CalibrationDir)
	}
	return args
}

func isTerminal(w io.Writer) bool {
	f, ok := w.(*os.File)
	return ok && term.IsTerminal(int(f.Fd()))
}

// enginePath finds the engine binary: the configured path if it exists or is
// on PATH, else the dev build inside the repo.
func enginePath(cfg config.Config) (string, error) {
	if p, err := exec.LookPath(cfg.Engine.Path); err == nil {
		return p, nil
	}
	if root, err := findRepoRoot(); err == nil {
		name := "engine"
		if runtime.GOOS == "windows" {
			name += ".exe"
		}
		for _, p := range []string{
			filepath.Join(root, "engine", "build", "dev", "RelWithDebInfo", name),
			filepath.Join(root, "engine", "build", "dev", "Release", name),
			filepath.Join(root, "engine", "build", "ci-linux", name),
		} {
			if _, err := os.Stat(p); err == nil {
				return p, nil
			}
		}
	}
	return "", fmt.Errorf("engine binary %q not found: build it (engine/) or set engine.path in the config", cfg.Engine.Path)
}

func engineConfig(cfg config.Config, stderr io.Writer) (engine.Config, error) {
	path, err := enginePath(cfg)
	if err != nil {
		return engine.Config{}, err
	}
	args := []string{"serve", "--device", cfg.Engine.Device,
		"--conf", fmt.Sprint(cfg.Detect.ConfThreshold), "--nms", fmt.Sprint(cfg.Detect.NMSIoU)}
	if cfg.Engine.ModelsPath != "" {
		args = append(args, "--models", cfg.Engine.ModelsPath)
	}
	if cfg.Engine.Threads > 0 {
		args = append(args, "--threads", fmt.Sprint(cfg.Engine.Threads))
	}
	if cfg.Engine.Decoder != "" {
		args = append(args, "--decoder", cfg.Engine.Decoder)
	}
	args = append(args, engineGeometryArgs(cfg)...)
	return engine.Config{
		Path:       path,
		Args:       args,
		JobTimeout: time.Duration(cfg.Engine.JobTimeoutS * float64(time.Second)),
		Stderr:     stderr,
	}, nil
}

// pipelineOptions builds the run options; the returned func closes the engine log.
func pipelineOptions(cfg config.Config, kind string) (pipeline.Options, func(), error) {
	logPath := filepath.Join(filepath.Dir(cfg.DatabasePath), "engine.log")
	logf, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return pipeline.Options{}, nil, err
	}
	close := func() { logf.Close() }
	ecfg, err := engineConfig(cfg, logf)
	if err != nil {
		close()
		return pipeline.Options{}, nil, err
	}
	features := []string{"detect", "track", "distance", "ttc"}
	return pipeline.Options{
		Workers:     cfg.Engine.Workers,
		Engine:      ecfg,
		Library:     cfg.LibraryPath,
		Kind:        kind,
		FPS:         cfg.Engine.FPS,
		Features:    features,
		EmbedEveryS: cfg.Search.KeyframeEveryS,
		EmbedCrops:  cfg.Search.Crops,
	}, close, nil
}

type statsMsg pipeline.Stats
type doneMsg struct {
	st  pipeline.Stats
	err error
}

type progressModel struct {
	cancel context.CancelFunc
	poll   func() pipeline.Stats
	st     pipeline.Stats
	done   bool
}

func (m progressModel) Init() tea.Cmd { return tick() }

func tick() tea.Cmd {
	return tea.Tick(250*time.Millisecond, func(time.Time) tea.Msg { return statsMsg{} })
}

func (m progressModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.String() == "ctrl+c" || msg.String() == "q" {
			m.cancel()
		}
	case statsMsg:
		m.st = m.poll()
		return m, tick()
	case doneMsg:
		m.st, m.done = msg.st, true
		return m, tea.Quit
	}
	return m, nil
}

var (
	titleStyle = lipgloss.NewStyle().Bold(true)
	dimStyle   = lipgloss.NewStyle().Faint(true)
	barStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
	errStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
)

func (m progressModel) View() string {
	s := m.st
	var b strings.Builder
	finished := s.Done + s.Failed
	frac := 0.0
	if s.Total > 0 {
		frac = float64(finished) / float64(s.Total)
	}
	const width = 40
	fill := int(frac * width)
	b.WriteString(titleStyle.Render("rearview process") + "\n\n")
	b.WriteString(barStyle.Render(strings.Repeat("█", fill)) + dimStyle.Render(strings.Repeat("░", width-fill)))
	fmt.Fprintf(&b, "  %d/%d jobs  %.0f%%\n", finished, s.Total, frac*100)
	fmt.Fprintf(&b, "queue %d   frames %d   objects %d   elapsed %s   ETA %s\n",
		s.QueueDepth, s.Frames, s.Objects, s.Elapsed.Round(time.Second), s.ETA().Round(time.Second))
	if s.Failed > 0 {
		b.WriteString(errStyle.Render(fmt.Sprintf("failed %d", s.Failed)) + "  " + dimStyle.Render(last(s.Errors)) + "\n")
	}
	b.WriteString("\n")
	for i, w := range s.Workers {
		state := dimStyle.Render("idle")
		if w.Busy {
			state = fmt.Sprintf("job %-6d %6.1f fps", w.JobID, w.FPS)
		}
		fmt.Fprintf(&b, "worker %d  %s  frames %d  restarts %d\n", i, state, w.Frames, w.Restarts)
	}
	if !m.done {
		b.WriteString(dimStyle.Render("\nq or Ctrl+C to stop (resumes next time)") + "\n")
	}
	return b.String()
}

func last(s []string) string {
	if len(s) == 0 {
		return ""
	}
	return s[len(s)-1]
}

// runWithProgress runs the pipeline with the live view (tty) or periodic log lines.
func runWithProgress(ctx context.Context, db *store.DB, opt pipeline.Options, out io.Writer, tty bool) (pipeline.Stats, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	var live func() pipeline.Stats
	ready := make(chan struct{})
	opt.OnStats = func(f func() pipeline.Stats) {
		live = f
		close(ready)
	}
	result := make(chan doneMsg, 1)
	go func() {
		st, err := pipeline.Run(ctx, db, opt)
		result <- doneMsg{st, err}
	}()
	poll := func() pipeline.Stats {
		select {
		case <-ready:
			return live()
		default:
			return pipeline.Stats{}
		}
	}
	if !tty {
		t := time.NewTicker(10 * time.Second)
		defer t.Stop()
		for {
			select {
			case r := <-result:
				return r.st, r.err
			case <-t.C:
				s := poll()
				fmt.Fprintf(out, "%s progress: %d/%d jobs, %d failed, queue %d, %d frames, ETA %s\n",
					time.Now().Format("15:04:05"), s.Done+s.Failed, s.Total, s.Failed, s.QueueDepth, s.Frames, s.ETA().Round(time.Second))
			}
		}
	}
	p := tea.NewProgram(progressModel{cancel: cancel, poll: poll}, tea.WithOutput(out))
	go func() {
		r := <-result
		result <- r
		p.Send(r)
	}()
	if _, err := p.Run(); err != nil {
		cancel()
	}
	r := <-result
	if errors.Is(r.err, context.Canceled) {
		return r.st, errors.New("stopped; run process again to resume")
	}
	return r.st, r.err
}
