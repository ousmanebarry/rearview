# Phase 2: labels and motion events

Ground-truth labels, the GPS-only hard-brake and swerve detectors, priority windows, the `eval` command and how the thresholds were tuned.

## 0.7 Labelling tool

### What it does

`rearview label add` checks one ground-truth event and appends it to `testdata/labels/events.csv`. `rearview label list` prints the labels sorted by time. Phase 2's `import-labels` and `eval` read this file, so it has to stay clean.

### Core logic ([go/internal/labels/labels.go](../../go/internal/labels/labels.go))

- **Time parsing (`ParseTime`).** RFC 3339 input (`...Z` or `...-04:00`) is taken as-is. Input with no offset is a camera-local wall-clock time, read in `--tz` (default `America/New_York`, the camera's clock). Everything is converted to UTC and cut to whole seconds, because the data model stores UTC.
  - DST: on 2026-11-01 the hour 01:00–02:00 happens twice. Go picks the first (EDT) occurrence. Use an explicit offset if an event falls in that hour.
  - `time/tzdata` is embedded in the binary because Windows has no zoneinfo database for Go.
- **Validation (`Validate`).** Type must be one of the event types in the `events` table, camera one of `front`/`rear`/`interior`, and the note a single line.
- **Writing (`Append`).** Loads the existing file first and rejects a duplicate (same time, camera and type; the note doesn't matter). Then appends one row with `encoding/csv`, which quotes notes containing commas or quotes. The header is written only when the file is empty.
- **Reading (`Load`).** A missing file means "no labels yet". A wrong header, bad time, unknown type or short row is an error with the line number, so a hand-edit mistake is caught before it skews `eval`.

### CLI ([go/cmd/rearview/label.go](../../go/cmd/rearview/label.go))

- The default `--file` is found by walking up from the working directory to the folder that holds `.github/AGENTS.md`, so the command works from `go/` or the repo root.
- Note convention: `src=suggested` for events first flagged by the GPS speed-drop pass and then confirmed by watching; `src=watched` for events found only by watching. This lets later accuracy numbers be split by how a label was found.

### Tests

- [labels_test.go](../../go/internal/labels/labels_test.go): time formats including EDT/EST and the repeated DST hour; every validation rule; CSV round trip with awkward notes; header written once; duplicate rejection; corrupt files.
- [label_test.go](../../go/cmd/rearview/label_test.go): add then list end to end, local-time display, the `--type` filter, and bad flags.

## 2.1 Motion event detectors

### What it does

`prefilter.Detect(track, cfg)` scans one trip's stitched GPS track (1 Hz, from [trips.Track](../../go/internal/trips/trips.go)) and returns hard brakes and swerves. `prefilter.Rebuild` runs it over every trip and replaces all `source = 'motion'` events in one transaction, so re-running with new thresholds never duplicates rows. It runs after every import and on `rearview events detect`.

### Core logic ([go/internal/prefilter/prefilter.go](../../go/internal/prefilter/prefilter.go))

- **Inputs.** Interpolated points are dropped first: a straight line between two fixes can't show a brake.
- **Receiver dropouts (`dropDropouts`).** The VIOFO's GPS sometimes writes a record with heading exactly 0.00° (and speed 0 or a few km/h) for one or two seconds, then carries on at the old speed with the position moving smoothly the whole time. Every one of those looked like a 130 km/h hard brake or a 60° swerve. A run of exact-0° records is removed when the speed before it is above 10 km/h and the record after it (within 5 s) is back to at least half that speed. A run that stays slow is a real stop and is kept.
- **Pairs within a window (`pairsWithin`).** For each point, look back at every earlier point no more than the window (1 s) away. At 1 Hz that is just the previous point; the code is written for any rate. A gap in the data longer than the window is never bridged, so 30 km/h lost over 3 s of missing data is not a brake.
- **Hard brake.** `speed(a) − speed(b) ≥ hard_brake_drop_kmh`.
- **Swerve.** Both points above `swerve_min_kmh` (30) and `|HeadingDelta(a, b)| ≥ swerve_deg` (30°). `HeadingDelta` is the signed shortest turn in (−180°, 180°], so 359° → 1° is +2°, not 358°.
- **G-spike.** Not implemented: this camera stores no accelerometer data (see [1.2](phase-1-import-gps-trips.md#12-gps-block-decoder)).
- **Score.** `min(1, value / (2 × threshold))`: 0.5 exactly at the threshold, 1.0 at twice the threshold or more.
- **Merging (`merge`).** Detections of one type are sorted by start; one starting less than `merge_s` (3 s) after the previous one's end extends it. The merged event keeps the highest-scoring peak (time, value, position).
- **Storage.** The event's segment is the one whose GPS point sits at the peak (the front file when both cameras have GPS); `t_start_s` / `t_end_s` are seconds from that segment's corrected start. `details_json` holds the drop or turn (`value`) and the speed before it.

### Tests ([prefilter_test.go](../../go/internal/prefilter/prefilter_test.go))

Table-driven synthetic traces: steady speed, gentle stop, just below / at / twice the threshold, swerve, swerve too slow, 350° → 10° (20°, no swerve), 345° → 25° (40°, swerve); merging of drops 2 s apart and separation at 6 s; worst peak kept; interpolated points ignored; no bridging of data gaps; 130 km/h and heading-only dropouts ignored while a real stop still counts. [events_test.go](../../go/internal/prefilter/events_test.go) checks the database round trip and that a second run doesn't duplicate.

## 2.2 Priority windows

### What it does

`prefilter.EventWindows(event, segments, before, after)` turns one event into the stretches of video to analyse: from `window_before_s` (10 s) before the event's peak to `window_after_s` (5 s) after, on every camera, as `(segment, start_s, end_s)` triples. The job planner (4.4) and clip extraction (2.3) both use it.

### Core logic ([go/internal/prefilter/events.go](../../go/internal/prefilter/events.go))

- **`Windows(from, to, segments)`** intersects the absolute span with each segment's `[started_at, started_at + duration]` (clock-corrected in 1.7) and converts it to seconds from that segment's start. A span crossing a file boundary intersects two consecutive files and so yields two windows, one per file, per camera. Parking segments are skipped.
- Windows are clamped to the file: an event 4 s into the first file of a trip gives `[0, 9]`, not a negative start.
- **`MergeWindows`** joins overlapping windows of the same segment, so two events 5 s apart become one analysis job instead of decoding the same seconds twice.

### Tests ([events_test.go](../../go/internal/prefilter/events_test.go))

An event 2 s before the boundary of two 180-s pairs gives `front: [168,180] + [0,3]` and the same for the rear; clamping at the file start; parking skipped; merging of overlapping and separate windows.

## 2.5 Evaluation command

### What it does

`rearview import-labels` loads `testdata/labels/events.csv` into the `labels` table (replacing it). `rearview eval --type hard_brake` compares stored events with those labels and prints precision, recall and every miss and false alarm with its local time, so each one can be looked up and clipped.

### Core logic ([go/internal/eval/eval.go](../../go/internal/eval/eval.go))

- **Matching (`Match`).** Every (detection, label) pair of the same type within the tolerance (3 s, from [design.md](../design.md#testing-and-evaluation)) is a candidate. Candidates are taken closest first, and each detection and label can be used once. Greedy closest-first is optimal here because events of one type are seconds apart and labels minutes apart; it also makes the pairing explainable.
- **Precision** = matched detections ÷ detections; **recall** = matched labels ÷ labels. With nothing to count, both are 1 (no false alarms, nothing missed).
- **Camera** is ignored: motion events belong to the car, not to a camera.
- **Held-out data.** `--only-days` / `--skip-days` take local dates. Tuning runs use `--skip-days 2026-10-25`; final numbers use `--only-days 2026-10-25`.
- Notes from the labels (`src=suggested gps=45->35 km/h file=...@71s`) and event details are printed next to each miss and false alarm.

### Tests

[eval_test.go](../../go/internal/eval/eval_test.go): closest pair wins, 3 s matches and 4 s doesn't, type must agree, one-to-one, empty sets. [events_test.go](../../go/cmd/rearview/events_test.go): import-labels then eval end to end.

## 2.6 Threshold tuning

Data: 2026-10-23 and 2026-10-24 (the held-out 2026-10-25 was not looked at), 12 hard-brake and 6 swerve labels, all `src=suggested` (GPS candidates confirmed by watching).

### Steps

1. **Starting point (14 km/h in 1 s):** 34 hard-brake detections, 2 matched: precision 0.06, recall 0.17.
2. **False alarms.** Almost all had `value == speed_kmh`: the speed fell to 0 in one second from 50–130 km/h. The raw trace shows receiver dropouts: one or two records with heading exactly 0.00° and speed 0 (or a few km/h) while the position keeps moving at full speed, then back to normal. Fix: drop such runs when the speed before is above 10 km/h and comes back to at least half within 5 s ([2.1](#21-motion-event-detectors)). Real stops, which stay at 0, are kept.
3. **Misses.** The labels are drops of 10–13 km/h in one second (`45->35`, `33->20`). At 14 km/h none of them can be found. Threshold sweep after the dropout fix:

   | Drop in 1 s | Detections | Matched | Precision | Recall   |
   | ----------- | ---------- | ------- | --------- | -------- |
   | 12 km/h     | 7          | 6       | 0.86      | 0.50     |
   | **10 km/h** | **11**     | **11**  | **1.00**  | **0.92** |
   | 9 km/h      | 17         | 10      | 0.59      | 0.83     |

   At 9 km/h recall drops because nearby 9-km/h drops merge with the real event and move its peak more than 3 s away from the label.

4. **Decision:** `motion.hard_brake_drop_kmh: 10` (about 0.28 g). The one miss (`13:39:10`, 12 → 0 km/h) is a stop whose last record is a dropout-shaped 0°/0 km/h reading followed by a stationary car: correctly treated as a stop, but the 12 → 0 step lands on the dropped record.
5. **Later (Phase 10 clean-up):** one hard brake showed up at position 0,0. The receiver also writes records flagged valid with position, speed and heading all zero (72 in the 235 files), and at low speed the dropout filter above kept 0 km/h / 0° records. A stopped car keeps its last heading (seen: 0.06 km/h at 260°), so 0/0 is never a real reading: both kinds are now dropped. Three labels (`12:31:24`, `13:39:10`, `17:30:28` local) had been drafted from such records (e.g. 14.7 → "0" → 6.4 km/h while slowing for a light at about 4 km/h per second) and were removed. Result: 9 labels, 9 detections, P 1.00 / R 1.00.

### Swerves: labels need a second look (input needed)

With the dropout filter, swerve recall falls from 1.0 to 0.17 (1 of 6). Five of the six swerve labels sit on the exact second of a heading-0.00° dropout (`19:15:51Z`, `19:16:29Z`, `01:08:34Z`, `21:56:16Z`, `21:56:25Z`), with the heading on both sides within 2° of each other. The GPS suggestion pass flagged them because of the glitch. Please re-watch those five clips (`rearview clip <id>` around those times) and keep each label only if the car really changed lanes sharply; otherwise remove it. The swerve threshold stays at 30°.
