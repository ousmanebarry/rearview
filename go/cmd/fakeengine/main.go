// fakeengine speaks the engine protocol (protocol/README.md) with canned
// replies so Go tests can run the whole pipeline without the C++ engine.
// Flags make it slow, return errors, go silent or crash.
package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sync"
	"time"
)

func main() {
	var (
		frames       = flag.Int("frames", 5, "frame replies per analyze job")
		delay        = flag.Duration("delay", 0, "pause before each frame")
		fail         = flag.String("fail", "", "answer every analyze with this error code")
		silentAfter  = flag.Int("silent-after", -1, "stop answering (but stay alive) after this many jobs")
		crashAfter   = flag.Int("crash-after", -1, "exit with code 3 in the middle of the job after this many jobs")
		version      = flag.Int("version", 1, "protocol version to announce")
		ignoreCancel = flag.Bool("ignore-cancel", false, "keep working after a cancel")
	)
	// The pipeline passes engine-style arguments (serve --models ...); ignore what we don't know.
	args := []string{}
	for i := 1; i < len(os.Args); i++ {
		a := os.Args[i]
		if flag.CommandLine.Lookup(trim(a)) != nil {
			args = append(args, a)
			if i+1 < len(os.Args) && !isBoolFlag(trim(a)) && !hasValue(a) {
				args = append(args, os.Args[i+1])
				i++
			}
		}
	}
	_ = flag.CommandLine.Parse(args)

	out := bufio.NewWriter(os.Stdout)
	var mu sync.Mutex
	send := func(m map[string]any) {
		b, _ := json.Marshal(m)
		mu.Lock()
		out.Write(append(b, '\n'))
		out.Flush()
		mu.Unlock()
	}
	send(map[string]any{"type": "ready", "protocol_version": *version, "engine_version": "fake",
		"device": "cpu", "capabilities": []string{"analyze", "embed_frames", "embed_text"}})

	jobs := make(chan map[string]any, 16)
	var cmu sync.Mutex
	cancelled := map[int64]bool{}
	go func() {
		sc := bufio.NewScanner(os.Stdin)
		sc.Buffer(make([]byte, 1<<20), 1<<24)
		for sc.Scan() {
			var m map[string]any
			if json.Unmarshal(sc.Bytes(), &m) != nil {
				continue
			}
			switch m["cmd"] {
			case "cancel":
				cmu.Lock()
				cancelled[int64(m["target_id"].(float64))] = true
				cmu.Unlock()
			case "shutdown":
				close(jobs)
				return
			default:
				jobs <- m
			}
		}
		close(jobs)
	}()

	n := 0
	for m := range jobs {
		id := int64(m["id"].(float64))
		if *silentAfter >= 0 && n >= *silentAfter {
			continue
		}
		switch m["cmd"] {
		case "ping":
			send(map[string]any{"id": id, "type": "pong"})
			continue
		case "analyze":
		case "embed_text":
			send(map[string]any{"id": id, "type": "embedding", "vector": vector(fmt.Sprint(m["text"]))})
			send(map[string]any{"id": id, "type": "done", "frames": 0, "fps": 0, "elapsed_ms": 1, "cancelled": false})
			continue
		case "embed_frames":
			for i := 0; i < *frames; i++ {
				send(map[string]any{"id": id, "type": "embedding", "t": float64(i), "vector": vector(fmt.Sprint(i))})
			}
			send(map[string]any{"id": id, "type": "done", "frames": *frames, "fps": 1, "elapsed_ms": 1, "cancelled": false})
			continue
		default:
			send(map[string]any{"id": id, "type": "error", "code": "unknown_command", "msg": fmt.Sprint("unknown command: ", m["cmd"])})
			continue
		}
		n++
		if *fail != "" {
			send(map[string]any{"id": id, "type": "error", "code": *fail, "msg": "fake failure", "t": 1.0})
			continue
		}
		start := 0.0
		if ws, ok := m["windows"].([]any); ok && len(ws) > 0 {
			start = ws[0].([]any)[0].(float64)
		}
		done := 0
		wasCancelled := false
		for i := 0; i < *frames; i++ {
			cmu.Lock()
			c := cancelled[id]
			cmu.Unlock()
			if c && !*ignoreCancel {
				wasCancelled = true
				break
			}
			time.Sleep(*delay)
			t := start + float64(i)*0.1
			send(map[string]any{"id": id, "type": "frame", "t": t, "objects": []map[string]any{
				{"track": 1, "cls": "car", "box": []float64{100, 200, 300, 400}, "conf": 0.9, "dist_m": 12.5, "ttc_s": 3.2},
				{"track": 2, "cls": "person", "box": []float64{500, 200, 540, 300}, "conf": 0.6},
			}})
			done++
			if *crashAfter >= 0 && n > *crashAfter && i == 0 {
				os.Exit(3)
			}
		}
		send(map[string]any{"id": id, "type": "progress", "t": start, "frames_done": done})
		send(map[string]any{"id": id, "type": "done", "frames": done, "fps": 100.0, "elapsed_ms": 10,
			"cancelled": wasCancelled, "stages_ms": map[string]float64{"inference": 5}})
	}
}

// vector is a deterministic unit vector derived from a string.
func vector(s string) []float32 {
	v := make([]float32, 512)
	h := uint32(2166136261)
	for _, c := range []byte(s) {
		h = (h ^ uint32(c)) * 16777619
	}
	v[h%512] = 1
	return v
}

func trim(a string) string {
	for len(a) > 0 && a[0] == '-' {
		a = a[1:]
	}
	for i := range a {
		if a[i] == '=' {
			return a[:i]
		}
	}
	return a
}

func hasValue(a string) bool {
	for i := range a {
		if a[i] == '=' {
			return true
		}
	}
	return false
}

func isBoolFlag(name string) bool { return name == "ignore-cancel" }
