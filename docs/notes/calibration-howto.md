# Camera calibration how-to (task 5.1)

Distance from box height needs each camera's focal length and lens distortion. Until a camera is calibrated, the engine assumes a 100° horizontal field of view with no distortion (`geometry.hfov_deg`), which can be off by 20% or more on the 140° VIOFO lens.

## 1. Print the board

- Use OpenCV's 10×7-square chessboard, which has **9×6 inner corners** (the corners where four squares meet; the outer edge doesn't count). Generate it with any chessboard generator, or print `pattern.png` from the OpenCV docs.
- Print at 100% scale on A4 or Letter and **glue it to stiff card or foam board**: a curled sheet ruins the calibration.
- Measure one square with a ruler. The plan assumes 25 mm; if yours is 24.6 mm, pass `--square-mm 24.6`.
- Leave a white margin around the board; the corner detector needs it.

## 2. Record each camera

Record directly with the dashcam, at its normal resolution (2560×1440), one file per camera, about 60–90 seconds:

- Hold the board 0.5–2 m from the lens and move it slowly (motion blur makes frames unusable; the tool skips blurry frames).
- Cover the whole frame, **including the four corners and the edges**: that is where the 140° lens distorts most and where the model learns the distortion.
- Tilt the board up to about 45° in each direction, and vary the distance.
- Aim for 20+ distinct positions. Keep the whole board in view each time.
- Daylight, no glare on the card.

For the rear camera, do the same through the rear window, as it is mounted.

## 3. Run the calibration

```
engine calibrate front_board.mp4 --board 9x6 --square-mm 25 --out ~/rearview/calibration/front.json --undistort-preview front_undistorted.jpg
engine calibrate rear_board.mp4  --board 9x6 --square-mm 25 --out ~/rearview/calibration/rear.json  --undistort-preview rear_undistorted.jpg
```

- It samples every 10th frame, keeps sharp frames where the whole board is found (up to 40), and solves with both OpenCV models: the standard (Brown–Conrady) model and the fisheye model. It keeps the one with the lower reprojection error and prints both.
- **Goal: reprojection error under 1 px.** The command exits with code 3 above that. If it fails, record again with more corner positions and less blur.
- Open the `--undistort-preview` image: lamp posts and building edges near the frame edges must look straight.
- `rearview process` passes `geometry.calibration_dir` (default `~/rearview/calibration/`) to the engine, which loads `<camera>.json` and scales it if the frame size differs.

## 4. Field test for distance (task 5.2)

1. Park behind a car on a flat road. Mark 5 m, 10 m and 15 m from the front of your windshield to the rear bumper of the parked car with a tape measure.
2. At each distance, record 10 seconds with the front camera (park, wait, record, move).
3. Note the start time of each stretch.
4. Hand over the file and the three time ranges. The agent runs `engine detect <file> --features detect,track,distance --json` on each stretch, compares the median `dist_m` with the tape, records the error in `docs/plan/decisions.md`, and keeps a trimmed copy in `testdata/` as a regression test.
5. If the error is consistently off by a factor, the class height (`geometry.class_heights.car`, 1.5 m) or the focal length is wrong; fix the calibration first, then the height.
