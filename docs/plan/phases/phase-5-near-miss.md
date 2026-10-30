# Phase 5: Near-miss and following distance (weeks 7–8)

**Read first:** [../../../protocol/README.md](../../../protocol/README.md), [../feature-specs.md](../feature-specs.md) (features F, G, core math, starting thresholds), [../data-model.md](../data-model.md), [../testing.md](../testing.md)

Goal: `rearview events --type near_miss` finds your real close calls with a measured precision and recall, and following distance is accurate enough to trust.

- [ ] **5.1 Camera calibration [agent, you review]**
  - `engine calibrate <video> --board 9x6 --square-mm 25`: extract sharp frames, find checkerboard corners, run calibration, print the reprojection error, save `calibration/front.json` (focal length, centre, distortion).
  - The 140° front lens is very wide: try OpenCV's standard model and its fisheye model, keep whichever gives the lower reprojection error. Repeat for the rear camera.
  - Also write `docs/notes/calibration-howto.md`: how to print the board and what to record.
  - Input needed: a checkerboard (9×6 inner corners, squares about 25 mm) printed on stiff card, recorded with the front and rear cameras at 20+ angles and distances, including the corners of the frame.
  - Done when: reprojection error is under 1 pixel and undistorted frames show straight lines (lamp posts, building edges) as straight.
- [ ] **5.2 Distance estimation [agent, you review]**
  - `src/geometry`: undistort the bottom-centre point of each box, then distance = focal length × real height ÷ box height (formula in Feature specs). Per-class heights in config (car about 1.5 m, truck and bus larger, person about 1.7 m).
  - Input needed: a field-test recording. Park behind a car, tape-measure 5, 10 and 15 m, and record 10 seconds at each distance.
  - Done when: estimates on the field test land within 20%, and the clip is saved in `testdata/` as a regression test.
- [ ] **5.3 Time-to-collision [agent, you review]**
  - Use the tracker's smoothed height and height velocity: TTC = h ÷ (dh/dt), only when dh/dt > 0 (approaching) and the box is at least 40 px tall (smaller boxes are too noisy).
  - "In the path" = box bottom-centre inside a trapezoid in front of the car (configurable corners, drawn on the debug video). Ignore objects outside it.
  - Add `dist_m` and `ttc_s` to `frame` replies when requested.
  - Done when: the debug video shows TTC counting down when approaching a stopped car, and unit tests cover the approaching / receding / too-small cases.
- [x] **5.4 Event fusion [agent, you review]**
  - `internal/fusion`: scan `observations` per track. Near-miss = TTC under threshold for 3+ consecutive frames in the path region; score rises as minimum TTC falls, with a boost when a hard brake happens within 2 seconds.
  - Following close = following time (distance ÷ GPS speed) under threshold for 3+ seconds above 50 km/h. Tailgated = the same rule on the rear camera.
  - Merge events of the same type within 3 seconds; one moment seen by both cameras becomes one event. Save minimum TTC, distance, object class and track ID in `details_json`.
  - Done when: table-driven tests with synthetic observation sequences cover each rule, merging and the brake boost.
- [ ] **5.5 Evaluate and tune [agent, you review]**
  - Extend `rearview eval` to `near_miss`, `following_close` and `tailgated`. For every miss and false alarm, export the debug clip and a short note on the likely cause, propose threshold and path-trapezoid changes, re-run.
  - Expect false alarms from parked cars on curves, merging traffic and wipers at night; record each fix in `docs/plan/decisions.md`.
  - Input needed: a verdict on the clips the agent flags as borderline.
  - Done when: precision and recall for each event type are recorded in `docs/plan/decisions.md`. These are resume numbers.

**Phase 5 deliverable:** the smart part of the project, with honest accuracy numbers.
