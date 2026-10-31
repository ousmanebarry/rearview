# Interview prep

Draft answers with pointers into the code. Review them and say them in your own words; the numbers are from [plan/decisions.md](../plan/decisions.md).

## Resume line

> Built Rearview, a dashcam analysis system in Go and C++: a concurrent Go pipeline with crash-isolated C++ inference workers, GPU-accelerated object detection and tracking, near-miss detection via time-to-collision estimation, natural-language video search with CLIP embeddings, and processing of 1 hour of 1440p footage in 2.4 minutes (11.7× faster than the CPU baseline).

The plan's line has "X% near-miss precision"; there are no near-miss labels yet, so that number is left out. Once labelled, use: "...near-miss detection via time-to-collision estimation (X% precision on N labelled events)...". A hard-brake number (P/R 1.00 on 9 labels) exists but is weak evidence: the labels came from GPS candidates.

## GPS decoding

**Q: How did you find the GPS data, and how do you know you decode it correctly?**
`moov/gps ` is an index of (offset, size) pairs to 16 KB `free` boxes, one per second ([box.go](../../go/internal/mp4gps/box.go), [gps.go](../../go/internal/mp4gps/gps.go) `ReadGPS`, `DecodeBlock`). I matched the first fix of a parked recording against `exiftool -ee` to the last digit. Golden tests compare three real files with exiftool output (1e-5°, 0.1 km/h) using 13 KB fixtures holding only the GPS boxes, because full files are 650 MB ([gps_test.go](../../go/internal/mp4gps/gps_test.go) `TestGolden`).

**Q: What went wrong with real data?**
Corrupt dates (repaired from neighbours, `repairTimes`), void records, all-zero records, and dropouts of 0 km/h / 0° at full speed ([prefilter.go](../../go/internal/prefilter/prefilter.go) `dropDropouts`). The dropouts produced 30 of 34 false hard brakes at first, and three of my own labels were drafted from them. Lesson: plot the raw signal before tuning; don't trust labels derived from the signal under test.

**Q: Why parse MP4 boxes yourself instead of calling exiftool?**
One dependency fewer at run time, and with `io.ReaderAt` only the index and the blocks it points to are read (about 3 MB of a 650 MB file). exiftool is still the oracle in tests.

## Crash recovery and the Go/C++ split

**Q: What happens when the C++ engine crashes mid-job?**
Each worker goroutine owns one engine process ([worker.go](../../go/internal/engine/worker.go) `Worker.Do`). The client's `pump` ([client.go](../../go/internal/engine/client.go)) turns a closed stdout into a `died` reply and a silent engine (no message for `job_timeout_s`) into `hung`; both kill the process. The worker restarts it, calls `onReset` so the partial observations of that job are deleted, and retries once; a second failure marks the job failed with the error ([jobs.go](../../go/internal/store/jobs.go) `FinishJob`). Tests use a fake engine that crashes, hangs or ignores cancel on demand ([engine tests](../../go/internal/engine/) `TestCrashAndRetry`, `TestHangAndRetry`, `TestCancelIgnoredKills`; [pipeline tests](../../go/internal/pipeline/) `TestCrashRetryKeepsNoPartialRows`).

**Q: And if the Go process is killed?**
Jobs are rows in SQLite. On start, `ResetRunning` puts `running` jobs back to `pending`, and their partial observations are deleted before the rerun (`TestResumeAfterHardKill`). Import copies to a `.part` file, checks the byte count, fsyncs and renames into the library ([ingest.go](../../go/internal/ingest/ingest.go) `copyIn`); leftovers are removed by `CleanPartials`, and duplicates are caught by a content hash, so a pulled SD card leaves nothing behind (`TestCardPulledMidCopyLeavesNothing`).

**Q: Why not cgo?**
Independent builds (no C compiler for Go on Windows), a native crash costs one retry instead of the run, and the Go side is testable with a fake engine. The cost is serialising detections as JSON lines, small next to decoding and resizing a 1440p frame (8 + 6 ms).

**Q: How do results get into SQLite without lock contention?**
Workers send results on a channel to a single writer goroutine ([pipeline.go](../../go/internal/pipeline/pipeline.go) `write`), which batches observations per job in one transaction. `errgroup` ties the workers and the writer to one context, so one fatal error cancels everything.

**Q: How are jobs prioritised?**
A heap ordered by priority then age ([queue.go](../../go/internal/queue/queue.go)); `Plan` ([plan.go](../../go/internal/queue/plan.go)) puts motion-event windows and locked files first, then the rest of each file (`Complement`), so useful results arrive in the first minutes.

## TTC and false alarms

**Q: How do you estimate time to collision without knowing distance?**
For an object at distance d closing at v, image height h ∝ 1/d, so (dh/dt)/h = v/d = 1/TTC ([distance.cpp](../../engine/src/geometry/distance.cpp) `ttc_s`). h and dh/dt come from the Kalman filter state ([kalman.cpp](../../engine/src/track/kalman.cpp), [tracker.cpp](../../engine/src/track/tracker.cpp)), so it's smoothed. No calibration needed; distance (for following time) does need it.

**Q: What caused false alarms and how did you suppress them?**
The hood detected as a car (dropped by shape and position, `drop_own_vehicle` in [analyzer.cpp](../../engine/src/pipeline/analyzer.cpp)); car/truck class flips creating twin tracks (cross-class NMS at IoU 0.7, [nms.cpp](../../engine/src/detect/nms.cpp)); small boxes where 1 px is 5% (`min_box_px` 40); cars in other lanes (path trapezoid); single-frame spikes (3 consecutive frames, [fusion.go](../../go/internal/fusion/fusion.go)). A hard brake within 2 s boosts the score.

**Q: How accurate is it?**
Honest answer: no near-miss labels yet, so no precision/recall. On every analysed event window it fired zero times, and stopping behind a car reads TTC 7–28 s. I'd label near-misses from a longer recording period and report precision on the held-out day.

**Q: Why greedy matching in the tracker rather than Hungarian?**
With fewer than ~15 boxes per frame the assignments are the same in practice, and it's easier to read and test (`Tracker.MatchIouIsOneToOne`, `KeepsIdThroughFiveFrameOcclusion`).

## Performance

**Q: What was the biggest win?**
GPU inference with ONNX Runtime + DirectML on an AMD card: 53 → 3 ms per frame, 28 → 2.9 minutes per hour, identical detections ([ort_env.cpp](../../engine/src/detect/ort_env.cpp), [onnxruntime.cmake](../../engine/cmake/onnxruntime.cmake)). Then two workers and a threaded decode → preprocess → infer pipeline inside the engine (−19%, byte-identical output, `Analyzer.ThreadedMatchesSerial`).

**Q: What didn't work?**
Hardware decoding was 2.6× slower (single-frame D3D11VA decode plus a GPU→CPU copy of every sampled 1440p frame, against a frame-threaded CPU decoder), and had a colour-range bug first. Idle-rate sampling saved nothing because decoding, not inference, is the bottleneck. Batching needed a re-exported model.

**Q: How do you know a speedup is real?**
`rearview bench` runs the real pipeline on a fixed set and records git SHA, config, per-stage times, utilisation and memory ([bench.go](../../go/internal/bench/bench.go)); repeat runs agree within 1.7%.

**Q: Where would you go next?**
Resize on the GPU or decode at reduced resolution (8 ms decode + 6 ms resize per frame vs 3 ms inference).

## Search

**Q: How does search work and how good is it?**
CLIP ViT-B/32 image embeddings for one keyframe per second (three crops, hood removed), a from-scratch BPE tokenizer checked against reference ids ([bpe.cpp](../../engine/src/embed/bpe.cpp)), brute-force dot product in Go over 58k vectors in under a second ([search.go](../../go/internal/search/search.go)). The correct moment is in the top 5 for 21% of 24 drafted queries; the eval undercounts because common objects appear many times but only one occurrence is labelled.
