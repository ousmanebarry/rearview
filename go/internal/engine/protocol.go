// Package engine runs the C++ engine as a child process and speaks the
// protocol in protocol/README.md over its stdin and stdout.
package engine

import (
	"encoding/json"
	"errors"
	"fmt"
)

// ProtocolVersion is the version this client speaks.
const ProtocolVersion = 1

// Object is one tracked object in a frame reply.
type Object struct {
	Track int        `json:"track"`
	Cls   string     `json:"cls"`
	Box   [4]float64 `json:"box"`
	Conf  float64    `json:"conf"`
	DistM *float64   `json:"dist_m,omitempty"`
	TTCS  *float64   `json:"ttc_s,omitempty"`
}

// Reply is any message from the engine; Type says which fields are set.
type Reply struct {
	ID   int64  `json:"id"`
	Type string `json:"type"`

	// ready
	ProtocolVersion int      `json:"protocol_version,omitempty"`
	EngineVersion   string   `json:"engine_version,omitempty"`
	Device          string   `json:"device,omitempty"`
	Capabilities    []string `json:"capabilities,omitempty"`

	// progress, frame, embedding, error
	T          *float64  `json:"t,omitempty"`
	FramesDone int       `json:"frames_done,omitempty"`
	Objects    []Object  `json:"objects,omitempty"`
	Vector     []float32 `json:"vector,omitempty"`
	Crop       string    `json:"crop,omitempty"`

	// done
	Frames    int                `json:"frames,omitempty"`
	FPS       float64            `json:"fps,omitempty"`
	ElapsedMs int64              `json:"elapsed_ms,omitempty"`
	Cancelled bool               `json:"cancelled,omitempty"`
	StagesMs  map[string]float64 `json:"stages_ms,omitempty"`
	PeakMemMB float64            `json:"peak_mem_mb,omitempty"`
	Files     []string           `json:"files,omitempty"`
	Path      string             `json:"path,omitempty"`

	// error
	Code string `json:"code,omitempty"`
	Msg  string `json:"msg,omitempty"`
}

// Final reports whether the reply ends a job.
func (r Reply) Final() bool { return r.Type == "done" || r.Type == "error" || r.Type == "pong" }

// Job is one command sent to the engine. Fields are flattened into the JSON
// command next to id and cmd.
type Job struct {
	Cmd    string
	Fields map[string]any
}

// Analyze builds an analyze job.
func Analyze(path, camera string, windows [][2]float64, fps float64, features []string) Job {
	if windows == nil {
		windows = [][2]float64{}
	}
	return Job{Cmd: "analyze", Fields: map[string]any{
		"path": path, "camera": camera, "windows": windows, "fps": fps, "features": features,
	}}
}

func (j Job) encode(id int64) ([]byte, error) {
	m := map[string]any{}
	for k, v := range j.Fields {
		m[k] = v
	}
	m["id"] = id
	m["cmd"] = j.Cmd
	b, err := json.Marshal(m)
	return append(b, '\n'), err
}

// EngineError is an `error` reply.
type EngineError struct {
	Code string
	Msg  string
	T    *float64
}

func (e *EngineError) Error() string {
	if e.T != nil {
		return fmt.Sprintf("engine %s at %.1fs: %s", e.Code, *e.T, e.Msg)
	}
	return fmt.Sprintf("engine %s: %s", e.Code, e.Msg)
}

// Retryable errors are worth one more attempt on a fresh process.
func (e *EngineError) Retryable() bool {
	return e.Code == "decode_failed" || e.Code == "out_of_memory"
}

var (
	// ErrDied means the engine process exited or closed stdout mid-job.
	ErrDied = errors.New("engine process died")
	// ErrHung means no message arrived within the job timeout.
	ErrHung = errors.New("engine stopped responding")
	// ErrVersion means the engine speaks another protocol version.
	ErrVersion = errors.New("engine protocol version mismatch")
)

// IsRetryable reports whether a job failing with err should be retried once
// on a restarted engine.
func IsRetryable(err error) bool {
	var ee *EngineError
	if errors.As(err, &ee) {
		return ee.Retryable()
	}
	return errors.Is(err, ErrDied) || errors.Is(err, ErrHung)
}
