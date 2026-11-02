// Package config holds every path, file-name pattern and threshold. Defaults
// live in code; a YAML file overrides any subset of them.
package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"

	"gopkg.in/yaml.v3"
)

type Config struct {
	LibraryPath  string `yaml:"library_path"`
	DatabasePath string `yaml:"database_path"`
	ClipsPath    string `yaml:"clips_path"`
	// TimeZone is the camera clock's zone; file names are local wall-clock times.
	TimeZone string `yaml:"time_zone"`
	// ViewerPath is the Qt viewer for `rearview view`; empty = look next to rearview and in the repo.
	ViewerPath string `yaml:"viewer_path"`

	Import    Import    `yaml:"import"`
	Trips     Trips     `yaml:"trips"`
	Motion    Motion    `yaml:"motion"`
	Engine    Engine    `yaml:"engine"`
	Detect    Detect    `yaml:"detect"`
	Geometry  Geometry  `yaml:"geometry"`
	NearMiss  NearMiss  `yaml:"near_miss"`
	Following Following `yaml:"following"`
	Search    Search    `yaml:"search"`
	Score     Score     `yaml:"score"`
}

type Import struct {
	// Patterns are matched against the file name; group 1 of TimePattern is the
	// wall-clock start time in TimeLayout.
	FrontPattern    string  `yaml:"front_pattern"`
	RearPattern     string  `yaml:"rear_pattern"`
	InteriorPattern string  `yaml:"interior_pattern"`
	TimePattern     string  `yaml:"time_pattern"`
	TimeLayout      string  `yaml:"time_layout"`
	LockedDir       string  `yaml:"locked_dir"`
	ParkingDir      string  `yaml:"parking_dir"`
	StableSeconds   float64 `yaml:"stable_seconds"`
}

type Trips struct {
	GapMinutes          float64 `yaml:"gap_minutes"`
	PairToleranceS      float64 `yaml:"pair_tolerance_s"`
	InterpolateMaxGapS  float64 `yaml:"interpolate_max_gap_s"`
	MinMovingSpeedKmh   float64 `yaml:"min_moving_speed_kmh"`
	MaxPlausibleSpeedKm float64 `yaml:"max_plausible_speed_kmh"`
}

type Motion struct {
	HardBrakeDropKmh float64 `yaml:"hard_brake_drop_kmh"`
	HardBrakeWindowS float64 `yaml:"hard_brake_window_s"`
	SwerveDeg        float64 `yaml:"swerve_deg"`
	SwerveWindowS    float64 `yaml:"swerve_window_s"`
	SwerveMinKmh     float64 `yaml:"swerve_min_kmh"`
	GSpikeG          float64 `yaml:"g_spike_g"` // 0 = off (no accelerometer in this camera)
	MergeS           float64 `yaml:"merge_s"`
	WindowBeforeS    float64 `yaml:"window_before_s"`
	WindowAfterS     float64 `yaml:"window_after_s"`
}

type Engine struct {
	Path        string  `yaml:"path"`
	ModelsPath  string  `yaml:"models_path"`
	Device      string  `yaml:"device"`
	Workers     int     `yaml:"workers"`
	JobTimeoutS float64 `yaml:"job_timeout_s"`
	FPS         float64 `yaml:"fps"`
	// Threads is ONNX Runtime's intra-op thread count per engine (0 = its default).
	Threads int `yaml:"threads"`
	// Decoder is the engine's frame source: opencv, or ffmpeg (hardware decoding).
	Decoder string `yaml:"decoder"`
	// EmptyFPS is used on stretches far from any motion event when the last
	// analysed frames had no objects (Phase 6 scheduling).
	EmptyFPS float64 `yaml:"empty_fps"`
}

type Detect struct {
	ConfThreshold float64 `yaml:"conf_threshold"`
	NMSIoU        float64 `yaml:"nms_iou"`
}

// Geometry is what the engine needs for distance (Phase 5).
type Geometry struct {
	// CalibrationDir holds front.json / rear.json from `engine calibrate`.
	CalibrationDir string `yaml:"calibration_dir"`
	// HFOVDeg is assumed for a camera without a calibration file.
	HFOVDeg float64 `yaml:"hfov_deg"`
	// ClassHeights are real object heights in metres.
	ClassHeights map[string]float64 `yaml:"class_heights"`
}

type NearMiss struct {
	TTCS        float64 `yaml:"ttc_s"`
	MinFrames   int     `yaml:"min_frames"`
	BrakeBoostS float64 `yaml:"brake_boost_s"`
	MinBoxPx    float64 `yaml:"min_box_px"`
	// PathTrapezoid is the in-path region as fractions of the frame:
	// bottom-left, top-left, top-right, bottom-right.
	PathTrapezoid [][2]float64 `yaml:"path_trapezoid"`
	MergeS        float64      `yaml:"merge_s"`
}

type Following struct {
	TimeS        float64 `yaml:"time_s"`
	MinDurationS float64 `yaml:"min_duration_s"`
	MinSpeedKmh  float64 `yaml:"min_speed_kmh"`
}

type Search struct {
	PromptTemplate string  `yaml:"prompt_template"`
	TopK           int     `yaml:"top_k"`
	GroupS         float64 `yaml:"group_s"`
	KeyframeEveryS float64 `yaml:"keyframe_every_s"`
	// Crops is "three" (left, centre and right squares of each keyframe) or "center".
	Crops string `yaml:"crops"`
}

type Score struct {
	HardBrake     float64 `yaml:"hard_brake"`
	Swerve        float64 `yaml:"swerve"`
	NearMiss      float64 `yaml:"near_miss"`
	FollowingMin  float64 `yaml:"following_close_per_min"`
	Tailgated     float64 `yaml:"tailgated"`
	PerKm         float64 `yaml:"per_km"`
	MinDistanceKm float64 `yaml:"min_distance_km"`
}

// Default returns the built-in configuration with paths under home.
func Default(home string) Config {
	root := filepath.Join(home, "rearview")
	return Config{
		LibraryPath:  filepath.Join(root, "library"),
		DatabasePath: filepath.Join(root, "rearview.db"),
		ClipsPath:    filepath.Join(root, "clips"),
		TimeZone:     "America/New_York",
		Import: Import{
			FrontPattern:    `(?i)_\d+F\.mp4$`,
			RearPattern:     `(?i)_\d+R\.mp4$`,
			InteriorPattern: `(?i)_\d+I\.mp4$`,
			TimePattern:     `^(\d{4}_\d{4}_\d{6})_`,
			TimeLayout:      "2006_0102_150405",
			LockedDir:       "RO",
			ParkingDir:      "Parking",
			StableSeconds:   5,
		},
		Trips: Trips{
			GapMinutes:          5,
			PairToleranceS:      2,
			InterpolateMaxGapS:  10,
			MinMovingSpeedKmh:   3,
			MaxPlausibleSpeedKm: 250,
		},
		Motion: Motion{
			HardBrakeDropKmh: 10, // tuned in 2.6; the plan started at 14
			HardBrakeWindowS: 1,
			SwerveDeg:        30,
			SwerveWindowS:    1,
			SwerveMinKmh:     30,
			GSpikeG:          0,
			MergeS:           3,
			WindowBeforeS:    10,
			WindowAfterS:     5,
		},
		Engine: Engine{
			Path:        "engine",
			ModelsPath:  "",
			Device:      "cpu",
			Workers:     2,
			JobTimeoutS: 120,
			FPS:         10,
			EmptyFPS:    10,
		},
		Detect: Detect{ConfThreshold: 0.3, NMSIoU: 0.45},
		Geometry: Geometry{
			CalibrationDir: filepath.Join(root, "calibration"),
			HFOVDeg:        100,
			ClassHeights: map[string]float64{
				"person": 1.7, "bicycle": 1.6, "car": 1.5, "motorcycle": 1.4, "bus": 3.2, "truck": 2.0,
			},
		},
		NearMiss: NearMiss{
			TTCS:          1.5,
			MinFrames:     3,
			BrakeBoostS:   2,
			MinBoxPx:      40,
			PathTrapezoid: [][2]float64{{0.30, 0.95}, {0.46, 0.55}, {0.54, 0.55}, {0.70, 0.95}},
			MergeS:        3,
		},
		Following: Following{TimeS: 1.0, MinDurationS: 3, MinSpeedKmh: 50},
		Search: Search{
			// Plain query: with the hood cropped, the "a dashcam photo of {query}"
			// template scored lower (docs/walkthroughs/phase-7-search.md).
			PromptTemplate: "{query}",
			TopK:           50,
			GroupS:         5,
			KeyframeEveryS: 1,
			Crops:          "three",
		},
		Score: Score{
			HardBrake:     4,
			Swerve:        3,
			NearMiss:      10,
			FollowingMin:  2,
			Tailgated:     0, // being tailgated isn't the driver's doing
			PerKm:         100,
			MinDistanceKm: 5,
		},
	}
}

// DefaultPath is where the config file lives unless --config says otherwise.
func DefaultPath(home string) string { return filepath.Join(home, "rearview", "config.yaml") }

// Load returns the defaults overridden by the file at path. A missing file is
// not an error. Unknown keys are, so a typo can't silently do nothing.
func Load(path, home string) (Config, error) {
	c := Default(home)
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return c, c.Validate()
	}
	if err != nil {
		return c, fmt.Errorf("read config: %w", err)
	}
	dec := yaml.NewDecoder(bytes.NewReader(b))
	dec.KnownFields(true)
	if err := dec.Decode(&c); err != nil && !errors.Is(err, io.EOF) {
		return c, fmt.Errorf("parse config %s: %w", path, err)
	}
	if err := c.Validate(); err != nil {
		return c, fmt.Errorf("config %s: %w", path, err)
	}
	return c, nil
}

// Validate checks values that would make later stages misbehave.
func (c Config) Validate() error {
	for name, p := range map[string]string{
		"import.front_pattern": c.Import.FrontPattern, "import.rear_pattern": c.Import.RearPattern,
		"import.interior_pattern": c.Import.InteriorPattern, "import.time_pattern": c.Import.TimePattern,
	} {
		if _, err := regexp.Compile(p); err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
	}
	if c.Trips.GapMinutes <= 0 || c.Motion.HardBrakeWindowS <= 0 || c.Motion.SwerveWindowS <= 0 {
		return errors.New("trips.gap_minutes and motion windows must be positive")
	}
	if c.Engine.Workers < 1 {
		return errors.New("engine.workers must be at least 1")
	}
	switch c.Engine.Device {
	case "cpu", "dml", "migraphx":
	default:
		return fmt.Errorf("engine.device %q: must be cpu, dml or migraphx", c.Engine.Device)
	}
	if len(c.NearMiss.PathTrapezoid) != 4 {
		return errors.New("near_miss.path_trapezoid needs 4 corners")
	}
	if c.Engine.FPS <= 0 || c.Engine.EmptyFPS <= 0 {
		return errors.New("engine.fps and engine.empty_fps must be positive")
	}
	return nil
}

// YAML renders the config as YAML.
func (c Config) YAML() ([]byte, error) {
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(c); err != nil {
		return nil, err
	}
	return buf.Bytes(), enc.Close()
}
