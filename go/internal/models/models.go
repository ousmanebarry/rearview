// Package models downloads the engine's ONNX models and CLIP tokenizer files
// and checks their SHA-256 checksums. Files mirrors
// engine/scripts/download_models.cmake (a test keeps the two in step).
package models

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// File is one file of the models folder.
type File struct {
	Name   string
	URL    string
	SHA256 string
	SizeMB int // approximate, for messages
}

// Files is everything the engine loads from its models folder.
var Files = []File{
	{"yolox_s.onnx", "https://github.com/Megvii-BaseDetection/YOLOX/releases/download/0.1.1rc0/yolox_s.onnx", "c5c2d13e59ae883e6af3b45daea64af4833a4951c92d116ec270d9ddbe998063", 34},
	{"face_detection_yunet_2023mar.onnx", "https://github.com/opencv/opencv_zoo/raw/main/models/face_detection_yunet/face_detection_yunet_2023mar.onnx", "8f2383e4dd3cfbb4553ea8718107fc0423210dc964f9f4280604804ed2552fa4", 1},
	{"clip_vision.onnx", "https://huggingface.co/Xenova/clip-vit-base-patch32/resolve/main/onnx/vision_model.onnx", "fd6e1402a588279d1723c7534d4bcba5bc0b14b47dfab0e46f8c47b8270d7d40", 335},
	{"clip_text.onnx", "https://huggingface.co/Xenova/clip-vit-base-patch32/resolve/main/onnx/text_model.onnx", "3f6571f5bad13a97c469c1622e1cfc4d9aef78b79fdbfcff804ca357bfada8cc", 242},
	{"clip_vocab.json", "https://huggingface.co/openai/clip-vit-base-patch32/resolve/main/vocab.json", "5047b556ce86ccaf6aa22b3ffccfc52d391ea4accdab9c2f2407da5b742d4363", 1},
	{"clip_merges.txt", "https://huggingface.co/openai/clip-vit-base-patch32/resolve/main/merges.txt", "f526393189112391ce6f9795d4695f704121ce452c3aad1f5335cc41337eba85", 1},
}

// Marker is the file the engine looks for to recognise a models folder.
const Marker = "yolox_s.onnx"

// Missing returns the files absent from dir. It doesn't hash anything.
func Missing(dir string, files []File) []File {
	var out []File
	for _, f := range files {
		if _, err := os.Stat(filepath.Join(dir, f.Name)); err != nil {
			out = append(out, f)
		}
	}
	return out
}

// TotalMB is the approximate download size of files.
func TotalMB(files []File) int {
	n := 0
	for _, f := range files {
		n += f.SizeMB
	}
	return n
}

// FindNear looks for a "models" folder holding Marker in dir and up to four of
// its parents, the same search the engine does from its own folder.
func FindNear(dir string) (string, bool) {
	for range 5 {
		m := filepath.Join(dir, "models")
		if _, err := os.Stat(filepath.Join(m, Marker)); err == nil {
			return m, true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "", false
}

// Download makes dir hold every file in files with the right checksum: files
// already there are hashed and kept if they match; the rest are downloaded to
// a .part file, checked, then renamed into place. Progress goes to log.
func Download(ctx context.Context, client *http.Client, dir string, files []File, log io.Writer) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("models folder: %w", err)
	}
	var errs []error
	for _, f := range files {
		dest := filepath.Join(dir, f.Name)
		if have, err := hashFile(dest); err == nil {
			if have == f.SHA256 {
				fmt.Fprintf(log, "ok        %s\n", f.Name)
				continue
			}
			fmt.Fprintf(log, "mismatch  %s: downloading again\n", f.Name)
		}
		fmt.Fprintf(log, "download  %s (%d MB)\n", f.Name, f.SizeMB)
		start := time.Now()
		if err := fetch(ctx, client, f, dest); err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			errs = append(errs, fmt.Errorf("%s: %w", f.Name, err))
			continue
		}
		fmt.Fprintf(log, "ok        %s (%v)\n", f.Name, time.Since(start).Round(100*time.Millisecond))
	}
	return errors.Join(errs...)
}

func fetch(ctx context.Context, client *http.Client, f File, dest string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, f.URL, nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GET %s: %s", f.URL, resp.Status)
	}
	part := dest + ".part"
	out, err := os.Create(part)
	if err != nil {
		return err
	}
	h := sha256.New()
	_, err = io.Copy(io.MultiWriter(out, h), resp.Body)
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		if got := hex.EncodeToString(h.Sum(nil)); got != f.SHA256 {
			err = fmt.Errorf("checksum %s, expected %s", got, f.SHA256)
		}
	}
	if err != nil {
		os.Remove(part)
		return err
	}
	return os.Rename(part, dest)
}

func hashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
