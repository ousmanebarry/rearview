# Phase 5: distance, TTC and near-misses

Camera calibration, distance and time-to-collision in the engine, event fusion in Go, and how to calibrate your own camera.

## 5.1–5.3 Calibration, distance and TTC

Status: code and unit tests done. The real calibration (5.1) and the tape-measure field test (5.2) need recordings only the owner can make; see [calibration-howto.md](#camera-calibration-how-to-task-51).

### 5.1 Calibration ([calibration.cpp](../../engine/src/geometry/calibration.cpp))

- `engine calibrate <video>` samples every 10th frame, drops blurry ones (variance of the Laplacian under 50), and finds the 9×6 inner corners. The corner search runs on a 1280-wide copy for speed; `cornerSubPix` then refines the corners on the full frame.
- Both `cv::calibrateCamera` (standard radial-tangential model, k1 k2 p1 p2 k3) and `cv::fisheye::calibrate` (equidistant model, k1..k4) are solved; the lower RMS reprojection error wins. A 140° lens often fits the fisheye model better near the edges.
- Saved as JSON: model, frame size, fx fy cx cy, distortion, rms. `Calibration::scaled` adapts the intrinsics to another frame size.
- No calibration file: `from_fov` gives a pinhole camera with `fx = (w/2) / tan(hfov/2)` (100° → 1,074 px at 2560 wide).
- Test: 12 synthetic boards rendered with a known pinhole camera (f = 1000 px) at different poses; calibration recovers fx within 20 px and RMS under 1 px.

### 5.2 Distance ([distance.cpp](../../engine/src/geometry/distance.cpp))

- The pinhole formula is `d = f·H / h`. With distortion, pixel heights aren't proportional to angles, so the box's **top-centre and bottom-centre** points are undistorted into normalised coordinates (`(x − cx)/fx` after removing distortion). Then `d = H / (y_bottom − y_top)`: the same formula with the distortion removed.
- Heights per class (config `geometry.class_heights`): car 1.5 m, truck 2.0 m (YOLOX's "truck" here is mostly pickups and vans; a semi is 4 m), bus 3.2 m, person 1.7 m, bicycle 1.6 m, motorcycle 1.4 m. Traffic lights get no distance.
- Tests: 120 px car at f = 1280 → 16 m; twice as tall → 8 m; barrel distortion makes an off-centre box read closer after undistortion than with the plain formula; class without a height.

### 5.3 TTC and the path region ([analyzer.cpp](../../engine/src/pipeline/analyzer.cpp))

- `TTC = h / (dh/dt)` from the tracker's Kalman-smoothed height and height velocity (px per analysed frame × analysis fps = px/s). Only when `dh/dt > 0` (approaching) and `h ≥ 40 px`: below that a one-pixel jitter is a large relative change and TTC is noise.
- Why box growth works without distance: for an object at distance d closing at speed v, `h ∝ 1/d`, so `(dh/dt) / h = v / d = 1 / TTC`. No calibration or class height is needed, which is why TTC is the primary near-miss signal.
- **In path**: the box's bottom-centre (where it touches the road) must lie inside a trapezoid ahead of the car, given as frame fractions (`near_miss.path_trapezoid`, default bottom 30–70% of the width at 95% height narrowing to 46–54% at 55% height). TTC is only reported for objects inside it; distance is reported for all.
- The debug video draws the trapezoid, thicker boxes for in-path objects, and `12.3m ttc 1.4s` labels.
- `dist_m` and `ttc_s` are added to `frame` replies when the `distance` / `ttc` features are requested (`rearview process` asks for both).
- Tests: approaching (100 px growing 50 px/s → 2 s), receding, steady and too-small cases; trapezoid membership by bottom-centre; parsing of `--path` and `--class-heights`.

### Seen on real footage

On the hard-brake window of `2026_1023_151753_000597F` (stop behind a car), the car ahead reads 15–17 m with TTC 7–28 s as the gap closes slowly: no false near-miss. A class flip from car to truck in the same track doubled the distance (31 m at the old 3 m truck height), which is why the truck height is now 2 m.

TTC countdown check (Phase 10, same file, 140–160 s, `engine detect --features detect,track,distance,ttc --debug-video`): track 4 closes from 19.7 m to 10.8 m between 149 and 155 s and its TTC falls from about 40 s to 9–10 s; the distance change over that stretch (about 1.5 m/s at 14 m) gives a true TTC of about 9.5 s, matching. As we brake to a stop behind it, TTC rises again (15–45 s). Frame-to-frame TTC still jitters by ±30% at these long values, which is why a near-miss needs 3 consecutive frames under 1.5 s.

## 5.4 Event fusion

### What it does

[`fusion.Rebuild`](../../go/internal/fusion/fusion.go) reads, per trip, every observation the engine stored, the stitched GPS track and the trip's hard-brake events, applies three rules, merges, and replaces all `vision` / `fused` events of the types `near_miss`, `following_close` and `tailgated`. It runs at the end of `rearview process` and on `rearview events fuse`.

### Core logic

- **Per-track sequences.** Observations are grouped by (segment, track id) and sorted by time. A **run** is a stretch of consecutive observations satisfying a rule, where consecutive means less than 0.35 s apart (the analysis runs at 5–10 fps, so one missed frame is tolerated, two are not).
- **Near-miss.** `ttc_s < near_miss.ttc_s` (1.5 s) and in the path trapezoid, for at least `min_frames` (3) frames in a run. The event is placed at the run's lowest TTC. Score = 0.5 at the threshold rising linearly to 1.0 at TTC 0. If a hard brake (from Phase 2) falls between 2 s before the run and 2 s after it, score + 0.25 (capped at 1) and `source = fused`: the driver reacted, which makes it much more likely to be real.
- **Following close.** Following time = `dist_m ÷ GPS speed (m/s)`, speed interpolated from the stitched track at the observation's absolute time (not across GPS gaps over 10 s). An in-path object with following time under `following.time_s` (1.0 s) above `following.min_speed_kmh` (50) for at least `following.min_duration_s` (3 s) is an event. Score uses the same 0.5→1.0 ramp on the minimum following time.
- **Tailgated.** The same rule on a rear-camera segment.
- **Merging.** All candidates of one type, from any camera or track, are sorted by start; one starting less than `merge_s` (3 s) after the previous one's end is folded into it, keeping the highest-scoring candidate's details and the union of the spans. So one moment seen by front and rear, or a car whose track id changed mid-event, gives one event.
- **Details** (`details_json`): minimum TTC or following time, distance, class, track id, camera, frames or duration, and whether a hard brake boosted it. Position from the GPS at the peak.
- The in-path test is repeated in Go (`InPath`, ray casting on the same trapezoid) so following-distance rules only use objects in the lane ahead (the engine reports distance for everything).

### Tests ([fusion_test.go](../../go/internal/fusion/fusion_test.go))

Synthetic observation sequences: three frames under 1.5 s → one event with the expected score; two frames → none; a run broken by a frame without TTC → none; never under the threshold → none; hard brake 2 s after → fused with +0.25; hard brake 5 s later → not boosted; out-of-path object ignored; front tracks, a second track and the rear camera within 3 s merge into one event that keeps the 0.8 s worst TTC, while one 20 s later stays separate; following at 0.75 s for 3.9 s at 72 km/h → one event, a 1.9 s stretch → none, at 40 km/h → none; the same on the rear camera is `tailgated`; speed interpolation and gap handling; trapezoid membership.

## Camera calibration how-to (task 5.1)

Distance from box height needs each camera's focal length and lens distortion. Until a camera is calibrated, the engine assumes a 100° horizontal field of view with no distortion (`geometry.hfov_deg`), which can be off by 20% or more on the 140° VIOFO lens.

### 1. Print the board

- Use OpenCV's 10×7-square chessboard, which has **9×6 inner corners** (the corners where four squares meet; the outer edge doesn't count). Generate it with any chessboard generator, or print `pattern.png` from the OpenCV docs.
- Print at 100% scale on A4 or Letter and **glue it to stiff card or foam board**: a curled sheet ruins the calibration.
- Measure one square with a ruler. The plan assumes 25 mm; if yours is 24.6 mm, pass `--square-mm 24.6`.
- Leave a white margin around the board; the corner detector needs it.

### 2. Record each camera

Record directly with the dashcam, at its normal resolution (2560×1440), one file per camera, about 60–90 seconds:

- Hold the board 0.5–2 m from the lens and move it slowly (motion blur makes frames unusable; the tool skips blurry frames).
- Cover the whole frame, **including the four corners and the edges**: that is where the 140° lens distorts most and where the model learns the distortion.
- Tilt the board up to about 45° in each direction, and vary the distance.
- Aim for 20+ distinct positions. Keep the whole board in view each time.
- Daylight, no glare on the card.

For the rear camera, do the same through the rear window, as it is mounted.

### 3. Run the calibration

```
engine calibrate front_board.mp4 --board 9x6 --square-mm 25 --out ~/rearview/calibration/front.json --undistort-preview front_undistorted.jpg
engine calibrate rear_board.mp4  --board 9x6 --square-mm 25 --out ~/rearview/calibration/rear.json  --undistort-preview rear_undistorted.jpg
```

- It samples every 10th frame, keeps sharp frames where the whole board is found (up to 40), and solves with both OpenCV models: the standard (Brown–Conrady) model and the fisheye model. It keeps the one with the lower reprojection error and prints both.
- **Goal: reprojection error under 1 px.** The command exits with code 3 above that. If it fails, record again with more corner positions and less blur.
- Open the `--undistort-preview` image: lamp posts and building edges near the frame edges must look straight.
- `rearview process` passes `geometry.calibration_dir` (default `~/rearview/calibration/`) to the engine, which loads `<camera>.json` and scales it if the frame size differs.

### 4. Field test for distance (task 5.2)

1. Park behind a car on a flat road. Mark 5 m, 10 m and 15 m from the front of your windshield to the rear bumper of the parked car with a tape measure.
2. At each distance, record 10 seconds with the front camera (park, wait, record, move).
3. Note the start time of each stretch.
4. Run `engine detect <file> --from <start> --to <start+10> --features detect,track,distance --json` on each stretch and compare the median `dist_m` of the parked car with the tape. Record the error in `docs/plan/decisions.md` and keep a trimmed copy in `testdata/` as a regression test (task 5.2).
5. If the error is consistently off by a factor, the class height (`geometry.class_heights.car`, 1.5 m) or the focal length is wrong; fix the calibration first, then the height.
