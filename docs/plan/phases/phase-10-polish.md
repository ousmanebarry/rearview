# Phase 10: Portfolio polish (week 10–11)

**Read first:** [../README.md](../README.md), [../architecture.md](../architecture.md), [../testing.md](../testing.md), [../decisions.md](../decisions.md), [../../../protocol/README.md](../../../protocol/README.md)

Goal: someone who spends 2 minutes on the repo understands what it does, sees it work, and sees the numbers. Do this phase even if Phases 8–9 never happen.

- [x] **10.1 README [agent, you review]**
  - Top: one sentence on what it does, the demo GIF or video, and the 3 headline numbers (near-miss precision/recall, search top-5 rate, minutes per hour of footage).
  - Then: architecture diagram, why Go and C++ (one paragraph), the protocol in brief, how to build and run, results tables, limitations.
- [ ] **10.2 Demo video, 60–90 seconds [agent, you review]**
  - Write `docs/notes/demo-script.md`: the exact commands and shot list (import → process with the progress view → near-miss found → clip plays with boxes and TTC → search "cyclist" → result plays). Then cut the raw recording into the final video with ffmpeg, adding captions and blurring other people's plates.
  - Input needed: a screen recording of the demo script being run.
- [x] **10.3 Results tables [agent]**
  - Accuracy per event type (precision, recall, number of labelled events), search top-5 rate, benchmark history from 6.8. State the dataset size honestly (hours of footage, number of labels).
- [x] **10.4 Write-up [agent, you review]**
  - Draft `docs/writeup.md` from `docs/plan/decisions.md` and `docs/notes/`: the 2–3 hardest problems (decoding the GPS blocks, getting TTC to stop crying wolf, the performance journey), including what didn't work. The review should rewrite it in your own voice before publishing.
- [x] **10.5 Clean-up [agent]**
  - Remove dead code, make sure `go vet`, `staticcheck`, `clang-format` and all tests pass, pin dependency versions, add a licence file.
- [x] **10.6 Resume line and interview prep [agent, you review]**
  - Fill in the resume line with the real numbers: "Built Rearview, a dashcam analysis system in Go and C++: a concurrent Go pipeline with crash-isolated C++ inference workers, GPU-accelerated object detection and tracking, near-miss detection via time-to-collision estimation (X% precision), natural-language video search with CLIP embeddings, and processing of 1 hour of 1440p footage in Y minutes."
  - Write `docs/notes/interview-prep.md`: the questions an interviewer is likely to ask about each core module (GPS decoding, crash recovery, TTC and its false alarms, the biggest performance win), each answered with pointers into the code.
