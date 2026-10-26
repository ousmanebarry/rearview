# Phase 2: Motion events and clips (weeks 3–4)

**Read first:** [../data-model.md](../data-model.md), [../feature-specs.md](../feature-specs.md) (features D, H, starting thresholds), [../testing.md](../testing.md), [../decisions.md](../decisions.md) (gotchas)

Goal: the program finds your real hard brakes and swerves from GPS alone and clips them, with no AI involved yet. This is already a useful tool and your first accuracy number.

- [ ] **2.1 Motion event detectors [agent, you review]**
  - `internal/prefilter`: run over each trip's stitched GPS track. Hard brake = speed drop past the threshold within 1 second; swerve = heading change past the threshold at speed; G-spike = accelerometer magnitude past the threshold (only if 1.2 found accelerometer data).
  - Handle heading wraparound (359° → 1° is a 2° change, not 358°). Ignore interpolated points.
  - Merge detections closer than 3 seconds into one event; score 0–1 by how far past the threshold they went.
  - Done when: table-driven tests with synthetic speed traces cover each detector, wraparound, and merging.
- [ ] **2.2 Priority windows [agent, you review]**
  - Turn each motion event into an analysis window (10 seconds before to 5 seconds after), mapped back to segment + `t_offset_s` for both cameras of the pair.
  - Windows crossing a file boundary split into two windows, one per file.
  - Done when: a test with an event 2 seconds before a file boundary produces two correct windows.
- [ ] **2.3 Clip extraction [agent]**
  - `internal/clips`: ffmpeg with `-ss` before `-i` and `-c copy` for speed. Events crossing a file boundary: cut both parts, then join them with the concat demuxer.
  - `--exact` flag re-encodes for frame-accurate cuts; `--side-by-side` stacks front and rear with `hstack` (re-encodes).
  - Clip names: `YYYYMMDD_HHMMSS_<type>_<event id>.mp4`. Insert a `clips` row.
  - Done when: clips play in VLC, cross-boundary clips have no gap or freeze, and a stream-copy clip takes under 2 seconds.
- [ ] **2.4 Events and clip commands [agent]**
  - `rearview events [--type hard_brake] [--since 7d] [--trip <id>]` lists events newest first with time, type, score and location.
  - `rearview clip <event id> [--side-by-side] [--exact]` writes the clip and prints its path.
  - Done when: both commands have tests and `--help` text.
- [ ] **2.5 Evaluation command [agent, you review]**
  - `rearview eval --type hard_brake` compares detected events with `labels` (a match = within 3 seconds and same type) and prints precision, recall and the list of misses and false alarms with timestamps.
  - Add an `import-labels` command that loads `testdata/labels/events.csv` into the `labels` table.
  - Done when: it prints numbers for the real data, recorded in `docs/plan/decisions.md`.
- [ ] **2.6 Tune thresholds [agent, you review]**
  - Pull the speed trace and a clip for every miss and false alarm, propose threshold changes in config with the reasoning, re-run `eval`. Stop when recall is at least 80% without drowning in false alarms.
  - Input needed: a verdict on borderline clips the agent flags (real event or not).

**Phase 2 deliverable:** real events with clips and a first precision/recall number.
