# Phase 3: the C++ engine

Decoding, YOLOX preprocessing and inference, and the tracker.

## 3.2 Frame source

### What it does

[`FrameSource`](../../engine/src/decode/frame_source.h) is the decoder interface: `seek(seconds)`, `next() -> optional<Frame{mat, t, index}>`, plus fps, size and duration. The first implementation, `OpenCvSource`, wraps `cv::VideoCapture` (FFmpeg backend). `WindowedReader` walks a list of `[start, end)` windows on top of any source.

### Core logic ([frame_source.cpp](../../engine/src/decode/frame_source.cpp))

- **Timestamps** are `index / fps`, not `CAP_PROP_POS_MSEC`, which is unreliable after a seek. The VIOFO files are constant 30 fps, so the frame index is exact.
- **Sampling.** `step = file fps / sample fps` (3 for 10 fps). `next()` calls `grab()` for every frame but `retrieve()` (the YUV → BGR conversion and copy, about half the per-frame cost at 1440p) only for the first frame at or past the next sample position, with half a frame of slack so 29.97 fps files don't drift. Asking for more than the file rate returns every frame.
- **Seeking.** A target less than 2 s ahead is reached by grabbing (cheaper than a keyframe seek plus decode); anything else uses `CAP_PROP_POS_FRAMES`, which FFmpeg turns into a keyframe seek and decodes forward to the exact frame.
- **Windows.** `WindowedReader` sorts the windows, seeks to each start, returns frames while `t < end`, then moves on. `window_changed()` is true on the first frame of each window so the caller can reset the tracker (a jump in time breaks the motion model).

### Tests ([frame_source_test.cpp](../../engine/tests/frame_source_test.cpp))

On `testdata/clips/city.mp4`: 10 fps over `[0, 20)` returns 200 frames at exactly `0.0, 0.1, …`; two out-of-order windows return 5 + 10 frames at the right times with `window_changed` on the first of each; a sample rate above 30 returns all 30 frames of one second; a missing file returns no source.

## 3.3 Preprocessing

### YOLOX conventions (checked against the exported model)

`engine info` shows the release model takes `images[1,3,640,640]` and returns `output[1,8400,85]`. The Megvii export (0.1.1rc0 and later) expects:

- **BGR** channel order, as OpenCV decodes it: no swap.
- **Raw 0–255 floats**: no division by 255 and no mean/std. (Older "legacy" YOLOX checkpoints normalised; doing that here gives near-zero scores and no error, the silent-garbage case the plan warns about.)
- **Letterbox at the top-left**, not centred: the resized image is pasted at (0, 0) and the rest is grey 114, as in YOLOX's `preproc`.

### Core logic ([letterbox.cpp](../../engine/src/detect/letterbox.cpp))

- `scale = min(640 / w, 640 / h)`: 0.25 for 2560×1440, giving a 640×360 image above 280 rows of grey.
- `Letterbox{scale, pad_x, pad_y}` is kept per frame; `to_original` maps a model box back with `(v − pad) / scale`, `to_model` is the inverse. Padding is always 0 for YOLOX, but the struct supports centred letterboxes (CLIP, YuNet).
- `to_nchw` splits the interleaved HWC bytes into three planes of float, appending to a reused buffer (so a batch is several images back to back).

### Tests ([letterbox_test.cpp](../../engine/tests/letterbox_test.cpp))

Scale and placement for a 2560×1440 frame (pixel colours at the corners of the image and the first padded row); a box mapped to the model and back lands within 1 px for both top-left and centred letterboxes; a model-space box maps to 4× in the original; NCHW keeps B, G, R planes in order.

## 3.4 Inference and post-processing

### What it does

[`Detector`](../../engine/src/detect/yolox.h) owns one ONNX Runtime session, created at startup and reused for every frame. `detect(frame)` letterboxes, runs the model and returns boxes in original 2560×1440 pixels. Times for each step are kept for 3.8.

### Core logic ([yolox.cpp](../../engine/src/detect/yolox.cpp), [nms.cpp](../../engine/src/detect/nms.cpp))

- **Session.** [`ort_env.cpp`](../../engine/src/detect/ort_env.cpp): one `Ort::Env` per process, graph optimisation on, intra-op threads configurable (`--threads`, 0 = ONNX Runtime decides). The device switch (`cpu`, `dml`, `migraphx`) is there for Phase 6.
- **Grid decoding (`decode_yolox`).** The release model does not include the decode step. Its 8400 rows are the cells of three grids, in order: 80×80 at stride 8, 40×40 at stride 16, 20×20 at stride 32. For a row at cell (gx, gy): `cx = (x + gx)·s`, `cy = (y + gy)·s`, `w = exp(w)·s`, `h = exp(h)·s`. Objectness and the 80 class scores already went through a sigmoid in the exported graph.
- **Score and class.** Confidence = objectness × best class score. Only the best class counts: if it isn't one of person, bicycle, car, motorcycle, bus, truck, traffic light, the box is dropped rather than relabelled. Threshold 0.3 (`--conf`).
- **NMS.** Sorted by confidence; a box is dropped when a kept box of the same class overlaps it by IoU > 0.45 (`--nms`). One addition: YOLOX often reports the same vehicle as both car and truck (seen on the night clip), which would give two tracks and two TTCs for one vehicle. A car/truck/bus box overlapping a more confident box of another of those classes by IoU > 0.7 is dropped too.
- **Own vehicle.** The camera's own hood is detected as a car at the bottom of every front frame. [`drop_own_vehicle`](../../engine/src/pipeline/analyzer.cpp) removes boxes wider than half the frame whose centre is in the bottom fifth.

### Tests

[nms_test.cpp](../../engine/tests/nms_test.cpp): IoU of identical, disjoint, half-overlapping, touching and empty boxes; per-class suppression; strict threshold; car/truck merge. [yolox_decode_test.cpp](../../engine/tests/yolox_decode_test.cpp): a hand-made output tensor decodes to the expected box at stride 8 and 16, mapped back through the letterbox; unwanted classes and low scores dropped. The review of `/tmp/city_dbg.mp4`, `highway_dbg.mp4` and `night_dbg.mp4` (frames in the 3.x PR) shows boxes on the right vehicles.

## 3.6 Tracker

### What it does

[`Tracker::update(detections)`](../../engine/src/track/tracker.h) advances one analysed frame and returns the confirmed tracks seen in it, each with a stable id, a smoothed box, the smoothed box height and its rate of change. Phase 5 computes TTC from those last two.

### Kalman filter ([kalman.cpp](../../engine/src/track/kalman.cpp))

- State: box centre `cx, cy`, aspect ratio `a = w/h`, height `h`, and the four velocities (per frame). Constant-velocity model: `x' = x + v`.
- Measurement: `(cx, cy, a, h)` from the detection box.
- Noise is proportional to the box height (1/20 for position, 1/160 for velocity, as in ByteTrack/DeepSORT), so a 300-px truck and a 30-px car are trusted the same relative amount. Aspect ratio gets small fixed noise: it barely changes.
- `height_velocity()` is `vh` in px per frame; multiply by the analysis fps for px per second.

### Matching ([tracker.cpp](../../engine/src/track/tracker.cpp))

Every track is predicted forward one frame, then three greedy IoU passes (highest IoU first, one to one):

1. High-confidence detections (≥ 0.5) against confirmed and lost tracks, IoU > 0.2.
2. Low-confidence detections (0.3–0.5) against confirmed tracks still unmatched, IoU > 0.5. This is ByteTrack's key idea: a partly hidden car's weak detection keeps its track alive instead of being thrown away; it never starts a track.
3. Leftover high detections against tentative tracks, IoU > 0.3.

Greedy matching instead of the Hungarian algorithm: with a few dozen boxes per frame and IoU costs, the results are the same in practice and the code is easier to follow.

### Track life cycle

- New high-confidence detection → **tentative** (1 hit). Needs 3 hits to become **confirmed**; a miss before that deletes it (kills one-frame false positives).
- Confirmed and unmatched → **lost**: still predicted, not reported. Matched again within 30 frames → confirmed with the same id. Otherwise removed.
- Class is a vote over all matched detections, so a car occasionally called a truck stays a car.
- `WindowedReader::window_changed()` resets the tracker between windows.

### Tests

[kalman_test.cpp](../../engine/tests/kalman_test.cpp): learns 4 px/frame, learns +1 px/frame height growth. [tracker_test.cpp](../../engine/tests/tracker_test.cpp): confirmation after 3 hits; same id after a 5-frame occlusion; two crossing objects keep their ids; low-confidence boxes keep a track but start none; removal after `max_lost`; tentative tracks dropped on a miss; one-to-one matching. [analyzer_test.cpp](../../engine/tests/analyzer_test.cpp): on the city clip the vehicle ahead keeps one id for the whole first 9 seconds (91 frames).
