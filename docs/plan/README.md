# Rearview: Build Plan

This folder is the plan, split by topic so each task loads only the context it needs. The repo is the source of truth: plan changes go through PRs here. Agents read [.github/AGENTS.md](../../.github/AGENTS.md) first, then this file.

## Plan files

| File                                                 | Contents                                             |
| ---------------------------------------------------- | ---------------------------------------------------- |
| [README.md](README.md)                               | Overview, how to work with coding agents (this file) |
| [architecture.md](architecture.md)                   | The three parts and who owns what                    |
| [tech-stack.md](tech-stack.md)                       | Libraries, tools, models, environment setup          |
| [repo-structure.md](repo-structure.md)               | Folder layout and runtime paths                      |
| [../../protocol/README.md](../../protocol/README.md) | Go ↔ engine protocol (source of truth)               |
| [data-model.md](data-model.md)                       | SQLite schema (becomes `0001_init.sql` in task 1.3)  |
| [feature-specs.md](feature-specs.md)                 | Features A–O, core math, starting thresholds         |
| [testing.md](testing.md)                             | Testing and evaluation strategy                      |
| [decisions.md](decisions.md)                         | Risks, gotchas, decisions log and results            |

**Phases** (the checkboxes in these files are the progress tracker)

| Phase        | File                                                               | Time (part-time) |
| ------------ | ------------------------------------------------------------------ | ---------------- |
| 0            | [phases/phase-0-setup.md](phases/phase-0-setup.md)                 | week 1           |
| 1            | [phases/phase-1-go-foundation.md](phases/phase-1-go-foundation.md) | weeks 2–3        |
| 2            | [phases/phase-2-motion-events.md](phases/phase-2-motion-events.md) | weeks 3–4        |
| 3            | [phases/phase-3-engine.md](phases/phase-3-engine.md)               | weeks 4–6        |
| 4            | [phases/phase-4-connect.md](phases/phase-4-connect.md)             | weeks 6–7        |
| 5            | [phases/phase-5-near-miss.md](phases/phase-5-near-miss.md)         | weeks 7–8        |
| 6            | [phases/phase-6-performance.md](phases/phase-6-performance.md)     | weeks 8–9        |
| 7            | [phases/phase-7-search.md](phases/phase-7-search.md)               | weeks 9–10       |
| 8 (optional) | [phases/phase-8-extras.md](phases/phase-8-extras.md)               | 1–2 weeks        |
| 9 (optional) | [phases/phase-9-viewer.md](phases/phase-9-viewer.md)               | about 2 weeks    |
| 10           | [phases/phase-10-polish.md](phases/phase-10-polish.md)             | week 10–11       |

## Overview

The goal: a Go + C++ program that turns hours of VIOFO A229 Plus footage into a searchable list of real driving events (near-misses, hard braking, tailgating), built in about 9–10 weeks part-time.

**What it does, end to end**

1. Imports footage from the SD card into a local library and skips anything already imported.
2. Extracts the GPS track hidden inside each MP4 and groups segments into trips.
3. Finds motion events (hard braking, swerves) from GPS and G-sensor data alone.
4. Runs object detection and tracking on the video, prioritizing the moments that look interesting.
5. Detects near-misses (time-to-collision) and following distance, and scores each event.
6. Cuts shareable clips, and lets you search footage in plain English ("cyclist at night").

**Why it is a strong portfolio piece**

- Go shows concurrency, pipelines, process management and data handling.
- C++ shows native performance, computer vision and GPU inference.
- Every claim has a number behind it: detection precision and recall, search accuracy, minutes per hour of footage.

**Scope**

| Tier     | Phases | Time (part-time) | Contents                                                                                                                                                 |
| -------- | ------ | ---------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Core     | 0–7    | 9–10 weeks       | Import, GPS, trips, motion events, clips, detection and tracking, Go–C++ workers, near-miss and following distance, performance, natural-language search |
| Optional | 8–9    | 3–4 weeks        | Privacy blur, driving score, incident PDF, Qt desktop viewer                                                                                             |
| Always   | 10     | 1 week           | README, results tables, demo video, write-up                                                                                                             |

**Definition of done for the whole project**

- `rearview import` → `rearview process` → `rearview events` works on a full week of your own drives.
- Near-miss detection has a measured precision and recall against your labelled events.
- `rearview search` finds the right moment in the top 5 for most test queries.
- `rearview bench` reports minutes of processing per hour of footage, with a history of improvements.
- The repo has a README with architecture, protocol, results tables and a 60–90 second demo video.

**Inputs and assumptions**

- Camera: VIOFO A229 Plus, MP4 files, 2560×1440 front and rear, one file per camera per loop segment.
- GPS is embedded in each MP4 in binary "freeGPS" blocks; speed is stored in knots.
- Locked event clips sit in a separate RO folder; normal clips are overwritten by loop recording.
- File naming is not guaranteed stable across firmware versions, so names are parsed with fallbacks.
- Dev machine: Windows 11 (version 24H2 or later is needed for Windows ML), AMD Radeon RX 9070 XT with 16 GB, Ryzen 7 7800X3D (8 cores, 16 threads). No CUDA: GPU inference goes through Windows ML with AMD's MIGraphX execution provider, with DirectML as the fallback (Phase 6).

## Working with coding agents

Agents do every task in this plan; you review their work. Each checkbox in the phase files is sized as one agent task (roughly one PR), with its own acceptance criteria.

- Each phase file starts with a "Read first" list naming the spec files its tasks need. Agents read those and nothing else from the plan.
- The agent ticks a task's box in the same PR that completes it, and records any decision or measured result in [decisions.md](decisions.md).
- Running several agents in parallel: turn each task into a GitHub issue (title = task number and name, body = its bullets) and have each PR close its issue, so two agents never pick the same task.

**Tags**

- **[agent]**: an agent does it, light review.
- **[agent, you review]**: an agent does it, close review, plus a walkthrough in `docs/notes/<task number>.md`.
- **Input needed**: something an agent can't produce on its own (footage, a real-world measurement); the agent stops and asks for it.

**How to run a task**

1. Start the agent in the repo. It reads `.github/AGENTS.md` first.
2. Give it one task by number ("Do task 1.2"), using the prompt template below or the `next-task` skill.
3. The agent reads this file, the phase file, and only the spec files that phase's "Read first" list names.
4. It writes tests first, then the code, runs the full suite, ticks the task's box and opens a PR.
5. If the task has an "Input needed" line, the agent stops and asks for that input before going further.
6. You review the PR with the checklist below and merge.

**Task prompt template**

```markdown
Task: <number and name, e.g. 1.2 GPS block decoder>
Read: .github/AGENTS.md, docs/plan/README.md, docs/plan/phases/<phase file>,
and the files in that phase's "Read first" list.
Goal: do exactly this task. Its "Done when" line is the acceptance test.
Constraints: only touch <directories>. Tests first. Run the full suite.
Input provided: <footage paths, measurements, labels — only if the task has
an "Input needed" line>
When done: tick the task's box, open a PR summarizing what changed, what was
tested and anything uncertain. For [agent, you review] tasks, add the
walkthrough in docs/notes/.
```

**Skills worth adding** (in `.github/skills/`)

- `next-task`: find the first unticked task in the current phase, read its context, do it, tick it, open the PR.
- `add-cli-command`: how a new cobra command is wired, tested and documented.
- `add-engine-command`: the steps to add a protocol message on both sides, with a round-trip test.
- `run-benchmarks`: how to run `rearview bench`, where results go, how to compare against the last run.

**How closely to review**

Interviewers will dig into the core parts, so for `[agent, you review]` tasks, read the agent's walkthrough in `docs/notes/` and ask it follow-up questions until the code makes sense without it open.

| Close review: `[agent, you review]`                       | Light review: `[agent]`                         |
| --------------------------------------------------------- | ----------------------------------------------- |
| MP4 box walker and GPS decoder                            | CLI wiring, flags, help text                    |
| Worker pool, priority queue, cancellation, crash recovery | SQLite migrations and query boilerplate         |
| Protocol spec changes                                     | ffmpeg command wrappers                         |
| Tracker, time-to-collision and distance math              | CMake, vcpkg and build presets                  |
| Event fusion and scoring rules                            | Test scaffolding, fixtures, golden-file helpers |
| Benchmark results and which optimizations to keep         | Progress UI, PDF layout, Qt viewer screens      |

**Review checklist for every agent PR**

- [ ] Tests exist, ran, and would fail if the feature broke.
- [ ] No goroutine can leak (each one exits on context cancel).
- [ ] Protocol or schema changes are in the spec / a new migration.
- [ ] No footage, models or secrets committed.
- [ ] You can explain the change in two sentences.
