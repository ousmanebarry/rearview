# Phase 3: C++ engine v1 (weeks 4–6)

**Read first:** [../../../protocol/README.md](../../../protocol/README.md), [../tech-stack.md](../tech-stack.md), [../repo-structure.md](../repo-structure.md), [../feature-specs.md](../feature-specs.md) (feature E), [../testing.md](../testing.md)

Goal: `engine detect clip.mp4 --debug-video out.mp4` produces a video with stable, labelled boxes on cars and people. Everything runs on the CPU for now; speed comes in Phase 6.

- [x] **3.1 Model download script [agent]**
  - `engine/scripts/download_models`: fetch YOLOX-s (and later CLIP) ONNX files, verify SHA-256 checksums, place them in `engine/models/`.
  - Done when: running it on a clean machine produces the same files every time.
- [x] **3.2 Frame source [agent, you review]**
  - `src/decode`: an interface `FrameSource` with `seek(seconds)` and `next() -> optional<Frame{mat, t}>`. First implementation wraps `cv::VideoCapture`.
  - Sample at a requested fps by skipping frames (grab without decoding where possible). Respect time windows.
  - Done when: asking for 10 fps over a 20-second window returns 200 frames (±1) with correct timestamps.
- [x] **3.3 Preprocessing [agent, you review]**
  - `src/detect`: letterbox resize to the model input (640×640 for YOLOX-s), pad with grey (114), convert HWC BGR to NCHW float.
  - Check the YOLOX preprocessing conventions for the exported model (channel order, whether to normalize). Getting this wrong gives silent garbage, not an error.
  - Keep the scale and padding so boxes map back to 2560×1440 coordinates.
  - Done when: a unit test maps a box through letterbox and back and gets the same box within 1 pixel.
- [x] **3.4 Inference + post-processing [agent, you review]**
  - Create one ONNX Runtime session at startup and reuse it. Run the model, then decode outputs into boxes. If the export doesn't include the decoding step, apply YOLOX grid decoding (strides 8, 16, 32).
  - Keep only COCO classes person (0), bicycle (1), car (2), motorcycle (3), bus (5), truck (7), traffic light (9). Confidence threshold 0.3, per-class NMS at IoU 0.45 (both configurable).
  - Done when: unit tests cover IoU and NMS on hand-made boxes, and the review confirms detections in the debug output look right.
- [x] **3.5 Debug video writer [agent]**
  - `src/debug`: draw boxes, class, confidence and track ID onto frames and write an MP4 (OpenCV `VideoWriter`). Colour by class.
  - Done when: the output plays in VLC.
- [x] **3.6 Tracker [agent, you review]**
  - `src/track`: ByteTrack-style. Kalman filter with a constant-velocity state for box centre, aspect ratio and height (Eigen). Match high-confidence detections to predicted tracks by IoU first, then low-confidence ones to the leftovers.
  - Track states: tentative (needs 3 hits) → confirmed → lost (kept 30 frames) → removed.
  - Expose each track's smoothed height and height velocity: Phase 5 needs them for TTC.
  - Done when: unit tests with synthetic moving boxes keep IDs through a 5-frame occlusion, and on the city test clip a car crossing the frame keeps one ID.
- [x] **3.7 Engine CLI [agent]**
  - `engine info` prints versions, available execution providers and model input shapes.
  - `engine detect <file> [--fps 10] [--from 12 --to 30] [--debug-video out.mp4] [--json]` runs the full chain and prints JSON lines in the protocol's `frame` format.
  - Done when: both commands work on the three test clips.
- [x] **3.8 Stage timing [agent]**
  - Time decode, preprocess, inference, post-process and tracking separately; print a summary at the end. This is the Phase 6 baseline.

**Phase 3 deliverable:** a debug video of your own footage with stable tracked boxes, plus per-stage CPU timings.
