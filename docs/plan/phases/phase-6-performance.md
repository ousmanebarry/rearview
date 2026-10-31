# Phase 6: Performance (weeks 8–9)

**Read first:** [design: architecture](../../design.md#architecture), [design: stack](../../design.md#stack), [data-model.md](../../data-model.md) (`bench_runs`), [design: testing](../../design.md#testing-and-evaluation), [../decisions.md](../decisions.md)

**Walkthrough:** [phase-6-performance.md](../../walkthroughs/phase-6-performance.md)

Goal: a headline number ("1 hour of 1440p footage in X minutes") and a record of how each change moved it. Measure first, change one thing, measure again.

- [x] **6.1 Benchmark command [agent, you review]**
  - `rearview bench [--set testdata/bench]`: runs a fixed set of footage (1 hour total, mixed city/highway/night) through the full pipeline and records: minutes per hour of footage, frames per second per stage (decode, preprocess, inference, post-process, tracking), worker utilization, peak memory, and GPU utilization (read from Windows' "GPU Engine" performance counters, which work on AMD).
  - Save each run to `bench_runs` and `bench/results/<date>_<sha>.json`. `rearview bench compare` prints the last two runs side by side.
  - Benchmarks run on the dev PC (7800X3D + RX 9070 XT), so the agent runs them locally, not in CI.
  - Done when: two runs on the same commit agree within 5%.
- [x] **6.2 Baseline on the 7800X3D [agent]**
  - Run `bench` on the CPU-only Phase 4 build. Before recording the baseline, compare 2 workers × 4 ONNX Runtime threads against 1 worker × 8 threads (8 cores, 16 threads) and keep the faster one.
  - Record the numbers and which stage dominates in `docs/plan/decisions.md` before changing anything.
- [ ] **6.3 GPU inference on the RX 9070 XT [agent, you review]**
  - No CUDA or TensorRT on AMD. Build two GPU routes behind `--device` and keep whichever `bench` says is faster.
  - Route 1, DirectML (`--device dml`): ONNX Runtime's DirectML build runs on any DirectX 12 GPU and is the quickest way to get GPU numbers. Microsoft has put DirectML in maintenance mode, so treat it as the fallback ([Windows ML docs](https://learn.microsoft.com/en-us/windows/ai/new-windows-ml/webgpu-ep)).
  - Route 2, MIGraphX (`--device migraphx`): Microsoft's recommended AMD GPU path is Windows ML with AMD's MIGraphX execution provider, downloaded on demand on Windows 11 24H2 or later. It means linking against Windows ML's shared ONNX Runtime instead of a bundled copy, so keep it behind a CMake option (`REARVIEW_WINML=ON`).
  - MIGraphX compiles each model for the GPU on first run, which can be slow. Cache the compiled model next to the ONNX file so later runs start fast, and exclude the first run from `bench`.
  - Check the session logs for operators that fall back to the CPU; a model split between CPU and GPU can be slower than CPU alone.
  - Done when: `bench` shows the inference speedup for each route, and detections match the CPU run on the test clips (same boxes within a small tolerance).
- [ ] **6.4 Hardware video decoding [agent, you review]**
  - Replace the OpenCV frame source with one using the FFmpeg libraries and D3D11VA hardware decoding, which runs on the RX 9070 XT's video engine, behind the same `FrameSource` interface. (On Linux, the same code path would use VAAPI.)
  - Decoding 2560×1440 on the CPU is often the real bottleneck once inference is on the GPU.
  - Done when: same frames and timestamps as the OpenCV source on the test clips, and `bench` shows the decode stage speedup.
- [x] **6.5 Threaded pipeline inside the engine [agent, you review]**
  - Decode, preprocess, inference and tracking on separate threads connected by bounded queues, so the GPU is never waiting for the decoder.
  - Done when: GPU utilization rises in `bench` and results are identical to the single-threaded run.
- [x] **6.6 Batched inference [agent]**
  - Run 4–8 frames per inference call. Measure; keep it only if it helps on the RX 9070 XT.
- [ ] **6.7 Smarter scheduling in Go [agent, you review]**
  - Lower the analysis fps on empty highway stretches (no objects in the last N frames), full fps around motion events, skip parking footage entirely.
  - Worker count: the 9070 XT has 16 GB of video memory, so start with 2 GPU workers and add more only if `bench` improves. Leave CPU cores free for decoding and the Go side.
  - Done when: `bench` shows the gain and Phase 5 accuracy numbers did not drop (re-run `eval`).
- [x] **6.8 Results table [agent]**
  - Generate a markdown table from `bench_runs` (change, minutes per hour, speedup vs baseline) for the README.

**Phase 6 deliverable:** the performance story with numbers for every step.
