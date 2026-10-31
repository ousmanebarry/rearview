# Risks, gotchas, decisions log and results

The biggest risks are a GPS format that differs from what's documented, a wide-angle lens that makes distance inaccurate, and night or snow footage that hurts detection. Each has a fallback below.

## Risks

| Risk                                                                                                                     | Impact                        | Mitigation                                                                                                                   |
| ------------------------------------------------------------------------------------------------------------------------ | ----------------------------- | ---------------------------------------------------------------------------------------------------------------------------- |
| GPS block layout differs on your firmware                                                                                | Phase 1 stalls                | Hex-dump a file, compare with exiftool output, decode field by field; exiftool can serve as a temporary fallback             |
| Firmware update changes file names                                                                                       | Import mis-tags cameras       | Name patterns live in config; fall back to resolution and embedded metadata                                                  |
| 140° lens distortion skews distance                                                                                      | Following distance unreliable | Calibrate with both lens models (5.1); report distance with its measured error                                               |
| Night, rain, snow and glare                                                                                              | More misses and false alarms  | Report accuracy per condition; keep night clips in the held-out set                                                          |
| TTC noise from small or jittery boxes                                                                                    | False near-misses             | Minimum box height, smoothing, consecutive-frame rule, path trapezoid                                                        |
| AMD GPU inference on Windows is less mature than CUDA (slow first-run model compile, some operators falling back to CPU) | Phase 6 delays                | Everything works on CPU first (the 7800X3D is a strong baseline); DirectML as the fallback; Linux with ROCm as a last resort |
| vcpkg builds are slow                                                                                                    | Slow CI and setup             | Cache vcpkg in CI; build OpenCV with only the modules you use                                                                |
| Scope creep                                                                                                              | Project never finishes        | Phases 8–9 are optional; Phase 10 happens regardless                                                                         |
| Agent-written core code that can't be explained in an interview                                                          | Weak interviews               | Close review on [agent, you review] tasks, walkthroughs in docs/notes/, interview prep in task 10.6                          |

## Gotchas to remember

- The engine must never print anything except protocol messages to stdout. One stray debug print breaks the Go parser.
- Loop recording overwrites footage: import before analysing, never analyse straight from the card.
- `-ss` before `-i` with stream copy cuts on keyframes; use `--exact` when frame accuracy matters.
- Heading wraps at 360°; speed in the GPS blocks is knots; times in the database are UTC.
- SQLite: one writer goroutine, WAL mode, foreign keys turned on per connection.
- Don't publish clips with other people's plates or faces visible; use the blur export.

## Decisions log

Newest first; add a row whenever you make a call you'd want to explain later.

| Date       | Decision                                                                                                                                              | Why                                                                                                            |
| ---------- | ----------------------------------------------------------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------- |
| 2026-10-30 | Plates are blurred as a region of each YOLOX vehicle box (no plate detector); faces with YuNet, heads from person boxes; 8-frame hold against flicker | No plate model with a clear licence; region blur can't miss a plate on a detected vehicle. 15-s clip blurred in 43 s |
| 2026-10-30 | Driving score: 100 − points per event scaled per 100 km (5 km floor); tailgated costs 0; a "floor" line keeps the breakdown summing to the score at 0 | Short trips with one brake would otherwise lose hundreds of points |
| 2026-10-30 | Incident PDF G-force is Δspeed/Δt from 1 Hz GPS | The camera has no accelerometer |
| 2026-10-30 | Search eval: 5/24 (21%) correct moment in the top 5 (best variants: centre crop + plain query, or three crops + plain query), on agent-drafted queries with single ±5 s ranges | Far below 70%; the eval counts only one labelled occurrence of common objects. Queries need owner review and all occurrences labelled (docs/notes/7.md) |
| 2026-10-30 | CLIP keyframes skip the bottom 22% of the frame (hood + camera text overlay); three crops (left/centre/right) stored, search defaults to best-of-three | Three-crop + plain query went 8% → 21% with the crop |
| 2026-10-30 | CLIP tokenizer golden ids generated once with the Hugging Face `tokenizers` library in a throwaway venv outside the repo | "Taken once from a trusted implementation"; no Python is committed |
| 2026-10-30 | GPU route: ONNX Runtime DirectML 1.24.4 (preset `dev-dml`, `--device dml`); MIGraphX via Windows ML not built | DirectML: 53 → 3 ms inference per frame, identical detections; inference is no longer the bottleneck. Windows ML needs the Windows App SDK runtime and EP catalogue (future work) |
| 2026-10-30 | Default decoder stays OpenCV (software, frame-threaded); D3D11VA source kept behind `--decoder ffmpeg` | Hardware decode was 2.6× slower end to end: per-sample GPU→CPU copies and single-frame decoding vs 16 CPU threads |
| 2026-10-30 | Engine pipeline threaded (decode / preprocess / inference+tracking) by default | −19% minutes per hour, results byte-identical |
| 2026-10-30 | Adaptive idle fps implemented but off by default; batching not possible (static batch-1 YOLOX export) | Both measured: no gain while decoding dominates |
| 2026-10-30 | Benchmark: CPU baseline 28.0 min per hour of footage (2 workers × 4 threads, 15-min subset); final 2.4 min per hour (DirectML, 2 workers, full 61.9-min set; repeat within 1.7%) | docs/notes/6.md, bench/results/ |
| 2026-10-30 | Until `engine calibrate` has run on real board recordings, distance assumes a 100° horizontal FOV pinhole (fx 1,074 px at 2560 wide); truck height 2.0 m | 5.1/5.2 need owner recordings (docs/notes/calibration-howto.md); YOLOX "truck" is mostly pickups here |
| 2026-10-30 | TTC is reported only for in-path objects; distance for all; Go re-checks the trapezoid for following distance | The plan says ignore objects outside the path for TTC; following/tailgating needs distance in the lane only |
| 2026-10-30 | First fused run over the 30 motion-event windows: 0 near-misses, 1 following_close (0.73 s at 53 km/h), 1 tailgated (0.51 s at 77 km/h, rear) | No near-miss/following/tailgated labels exist yet, so no precision/recall (5.5 input needed) |
| 2026-10-30 | Protocol additions (version stays 1): `pong` reply to `ping`, `stages_ms` in `done`, optional `debug_video` / `decoder` on `analyze`; `cancel` has no reply of its own | Needed by the client and `bench`; additive, so not breaking |
| 2026-10-30 | Priority-2 jobs cover the complement of a segment's event windows instead of the whole file; segments are planned once | No second of video is analysed twice, so observations never overlap |
| 2026-10-30 | Priority queue is a mutex-guarded heap with a close-and-replace notify channel, not two channels | Strict priority order even for late pushes, and `Pop` can select on ctx |
| 2026-10-30 | Real run: `process --only-windows` on the library, 30 jobs / 3,031 frames in 3.5 min on 2 CPU workers (~12 fps each); two `taskkill /F` mid-run left 0 duplicate observations | Task 4.6/4.7 acceptance |
| 2026-10-30 | Engine CPU stage timings, 10 fps over a 20 s clip (200 frames, YOLOX-s, ORT default threads): decode 1.9–2.7, preprocess 1.4, inference 47–50, post-process 0.04, tracking 0.04 ms/frame; 15–17 fps end to end | Phase 6 baseline: inference is 95% of the time |
| 2026-10-30 | ONNX Runtime comes from the official prebuilt release (1.31.1, SHA-256 pinned in `engine/cmake/onnxruntime.cmake`), not vcpkg | The vcpkg port builds from source (hours) and has no DirectML feature |
| 2026-10-30 | Models: YOLOX-s (Megvii 0.1.1rc0), CLIP ViT-B/32 ONNX from `Xenova/clip-vit-base-patch32` (image and text towers with projection, 512-d), YuNet 2023mar; fetched by `cmake -P engine/scripts/download_models.cmake` with SHA-256 checks | Pretrained exports only; no Python |
| 2026-10-30 | Car/truck/bus boxes overlapping by IoU > 0.7 are merged across classes; boxes wider than half the frame centred in its bottom fifth (own hood) are dropped | One vehicle reported as car and truck gave two tracks; the hood was a "car" in every front frame |
| 2026-10-30 | Tracker uses greedy IoU matching instead of the Hungarian algorithm | Same assignments at dashcam box counts, simpler to explain |
| 2026-10-30 | Hard-brake threshold 10 km/h in 1 s (was 14); heading-0° receiver dropouts are removed before detection | 2.6 sweep on 10-23/10-24: P 1.00 / R 0.92 at 10, R 0.17 at 14 after the dropout fix. See docs/notes/2.6.md |
| 2026-10-30 | Five of six swerve labels coincide with heading-0° dropouts; flagged for the owner to re-watch | They were GPS-suggested; the glitch, not the car, may have triggered them |
| 2026-10-30 | `rearview import` on the full backup: 235 new, 0 failed, 15 min (copy-bound); second run 0 new / 235 skipped; 19 trips; camera clock offset 0 s | Task 1.6/1.7 acceptance on real footage |
| 2026-10-30 | Migration `0002_clock_offset.sql` adds `segments.clock_offset_s`; Phase 7's search migration becomes `0003_search.sql` | Rebuilding trips must not apply a borrowed clock offset twice |
| 2026-10-30 | Stream-copy clips: 0.2 s warm, ~6 s when the 650 MB source is not in the OS file cache | Under the 2 s target once read; the cold case is disk-bound |
| 2026-10-30 | GPS records with a corrupt date (firmware glitch, e.g. year 41 month 2 day 0) are kept with a time repaired from the nearest good block | Position and speed in those blocks are good; exiftool keeps some with a bogus date and drops others, so the golden test allows only these extras |
| 2026-10-30 | No accelerometer data in the freeGPS blocks (16 trailing bytes are zero in all 235 files, exiftool shows none); G-spike detection stays off | Checked every backed-up file |
| 2026-10-30 | GPS golden inputs are 13 KB fixtures (`rearview debug gps-fixture`: ftyp + moov/gps index + each block cut to 64 bytes) from 3 files: 593F (void fixes), 669F (corrupt dates), 700R (rear) | Full files are 650 MB and stream-copied clips lose the blocks |
| 2026-10-27 | Test clips (20 s, video only, `ffmpeg -ss <s> -i <file> -t 20 -map 0:v:0 -c copy`, Git LFS): `city.mp4` = `2026_1024_124513_000679F.MP4` @ 80 s; `highway.mp4` = `2026_1024_131801_000699F.MP4` @ 60 s; `night.mp4` = `2026_1023_210924_000653F.MP4` @ 60 s. Each 2560x1440 H.264, 600 frames, ~71 MB | Picked from GPS speed stats + previews, confirmed by the owner. None from the held-out day (2026-10-25). Audio dropped (cabin speech). Overlay shows own plate and GPS position: fine while the repo is private, blur before it goes public |
| 2026-10-27 | Held-out day for final accuracy numbers: 2026-10-25 (1.11 h, day + night) | Never used for test clips, suggested labels or tuning |
| 2026-10-27 | Stream-copied clips lose the camera's freeGPS data | Task 1.2 golden tests need another input: small files built from real GPS boxes, with full files kept outside the repo |
| 2026-10-27 | Raw footage lives in `~/rearview-raw/` (card layout kept: `DCIM/Movie/`), with `SHA256SUMS.txt` maintained by `scripts/backup-footage.ps1` | SD card was already reused, so the copy can't be compared with the card; the manifest guards it from now on. 237 files, 117.9 GB, 5.35 h front footage (10-23: 1.94 h, 10-24: 2.29 h, 10-25: 1.11 h, 10-26: 0.01 h); `-VerifyOnly` passes |
| 2026-10-27 | Footage facts for Phase 1: names `YYYY_MMDD_HHMMSS_NNNNNN{F\|R}.MP4`, 3-min loops, H.264 2560x1440 30 fps (~650 MB/file); no `RO` or parking folder in this batch; `000814R` has no front partner; camera clock is America/New_York local time | Confirmed with ffprobe and against GPS UTC (file 13:05:15 local = first fix 17:05:15Z) |
| 2026-10-27 | Engine build: vcpkg manifest pinned to baseline `817892b0`, `opencv4` with default features off plus `ffmpeg`; presets `dev` (VS 2022, Windows) and `ci-linux` (Ninja) | First configure took ~22 min (FFmpeg + OpenCV). `engine <front file>` prints `2560x1440 30.00 fps 5400 frames`; rear files are the same |
| 2026-10-25 | `scripts/setup-windows.ps1` installs with winget (vcpkg cloned to `C:\vcpkg`, `VCPKG_ROOT` set for the user); `-CheckOnly` reports without installing | `-CheckOnly` on the dev PC: every tool present, Windows build 26300 (24H2+), RX 9070 XT driver 32.0.31041.1004 |
| 2026-10-25 | Go primer examples live in `go/examples/<name>/`, each with a test; `golang.org/x/sync` added for errgroup                                            | They build and test with the rest of the module, so they can't rot. The primer doubles as the 0.1 walkthrough  |
| 2026-10-25 | `go test -race` runs in CI only                                                                                                                       | The race detector needs cgo, and the Windows dev box has no C compiler on PATH for Go                          |
| 2026-10-25 | Go module path is `rearview` for now; `rearview` command uses only the standard library until the first cobra task                                    | No GitHub remote exists yet; rename the module path once it does. Keeps 0.3 buildable with no dependencies     |
| 2026-10-25 | Plan split into `docs/plan/` and `protocol/README.md` (task 0.3); the repo is now the source of truth                                                 | Agents load only the files a task needs                                                                        |
| 2026-10-25 | GPU inference via Windows ML + MIGraphX, DirectML as fallback                                                                                         | Dev GPU is an AMD RX 9070 XT, so CUDA and TensorRT are out; MIGraphX is Microsoft's recommended AMD path       |
| 2026-10-25 | Go ↔ engine over stdin/stdout JSON lines, not cgo                                                                                                     | Simpler builds, crash isolation, easy to test with a fake engine                                               |
| 2026-10-25 | YOLOX instead of Ultralytics YOLO                                                                                                                     | Apache-2.0 licence, ready ONNX exports                                                                         |
| 2026-10-25 | Pretrained models only, no training                                                                                                                   | No Python in the repo; the value is in the systems around the models                                           |
| 2026-10-25 | SQLite via pure-Go driver                                                                                                                             | No C compiler on the Go side; one file to back up                                                              |
| 2026-10-25 | Go orchestrator + C++ engine for the Rearview project                                                                                                 | Shows concurrency and native performance; differs from Fullcut's web stack                                     |

## Results

Fill in as you go; these become the README numbers.

| Metric                                     | Value | Dataset | Date |
| ------------------------------------------ | ----- | ------- | ---- |
| Hard brake precision / recall              | 1.00 / 0.92 (11 of 12) | 10-23 + 10-24, 4.2 h, 12 labels; held-out day has no hard-brake labels yet | 2026-10-30 |
| Near-miss precision / recall               |       |         |      |
| Following close precision / recall         |       |         |      |
| Distance error at 5 / 10 / 15 m            |       |         |      |
| Search: correct moment in top 5            | 21% (5/24) | 24 draft queries, 5.35 h front footage, 19,266 keyframes | 2026-10-30 |
| Minutes per hour of footage (CPU baseline) | 28.0 (2 workers × 4 threads) | 15 min of front footage (bench set, first 5 files), 10 fps analysis | 2026-10-30 |
| Minutes per hour of footage (final)        | 2.4 (DirectML, 2 workers) | 61.9 min front footage (bench/set.txt), 10 fps analysis | 2026-10-30 |
