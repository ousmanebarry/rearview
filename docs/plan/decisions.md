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
| Hard brake precision / recall              |       |         |      |
| Near-miss precision / recall               |       |         |      |
| Following close precision / recall         |       |         |      |
| Distance error at 5 / 10 / 15 m            |       |         |      |
| Search: correct moment in top 5            |       |         |      |
| Minutes per hour of footage (CPU baseline) |       |         |      |
| Minutes per hour of footage (final)        |       |         |      |
