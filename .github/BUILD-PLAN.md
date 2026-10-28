# Rearview: Build Plan

Oct 25, 2026 · @Barry

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

| Tier | Phases | Time (part-time) | Contents |
| --- | --- | --- | --- |
| Core | 0–7 | 9–10 weeks | Import, GPS, trips, motion events, clips, detection and tracking, Go–C++ workers, near-miss and following distance, performance, natural-language search |
| Optional | 8–9 | 3–4 weeks | Privacy blur, driving score, incident PDF, Qt desktop viewer |
| Always | 10 | 1 week | README, results tables, demo video, write-up |

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
- Dev machine: Windows 11 (assumed; version 24H2 or later is needed for Windows ML), AMD Radeon RX 9070 XT with 16 GB, Ryzen 7 7800X3D (8 cores, 16 threads). No CUDA: GPU inference goes through Windows ML with AMD's MIGraphX execution provider, with DirectML as the fallback (Phase 6).

## Working with coding agents

Agents do every task in this plan; you review their work. Each checkbox in the phase sections is sized as one agent task (roughly one PR), with its own acceptance criteria.

**Where the plan lives: in the repo, split by topic**

Agents work best when the plan sits in the repo next to the code, split into small files, so each task loads only the context it needs. Export this doc as Markdown, and task 0.3 splits it like this:

| Repo file | Comes from this doc's section |
| --- | --- |
| `docs/plan/README.md` | Overview, plus links to every file below |
| `docs/plan/architecture.md` | Architecture |
| `docs/plan/tech-stack.md` | Tech stack and environment setup |
| `docs/plan/repo-structure.md` | Repository structure |
| `protocol/README.md` | Go ↔ engine protocol |
| `docs/plan/data-model.md` | Data model (becomes `0001_init.sql` in task 1.3) |
| `docs/plan/feature-specs.md` | Feature specs |
| `docs/plan/testing.md` | Testing and evaluation strategy |
| `docs/plan/decisions.md` | Risks, gotchas, decisions log and results |
| `docs/plan/phases/phase-0-setup.md` to `phase-10-polish.md` | One file per phase, with its checklist |

- Each phase file starts with a "Read first" list naming the spec files its tasks need (for example Phase 4: protocol, data model, architecture). Agents read those and nothing else from the plan.
- The checkboxes in the phase files are the progress tracker. The agent ticks a task's box in the same PR that completes it, and records any decision or measured result in `docs/plan/decisions.md`.
- Once exported, the repo is the source of truth. Plan changes go through PRs there, not edits to this doc, so the two never drift apart.
- Running several agents in parallel: turn each task into a GitHub issue (title = task number and name, body = its bullets) and have each PR close its issue, so two agents never pick the same task.

**How to run a task**

1. Start the agent in the repo. It reads `.github/AGENTS.md` first.
2. Give it one task by number ("Do task 1.2"), using the prompt template below or the `next-task` skill.
3. The agent reads `docs/plan/README.md`, the phase file, and only the spec files that phase's "Read first" list names.
4. It writes tests first, then the code, runs the full suite, ticks the task's box and opens a PR.
5. If the task has an "Input needed" line, the agent stops and asks for that input before going further.
6. You review the PR with the checklist below and merge.

**AGENTS.md template** (put it at `.github/AGENTS.md`, same as Fullcut)

```markdown
# AGENTS.md — rearview

## What this repo is
Rearview: dashcam footage analyzer. Go orchestrator (go/) + C++ vision engine (engine/).
Optional Qt viewer (viewer/). No Python or TypeScript anywhere in the repo.

## The plan
- docs/plan/README.md is the index. Read it first.
- Tasks live in docs/plan/phases/. Do ONE task per PR, in order, unless told
  otherwise.
- Before starting, read the phase file's "Read first" list. Don't load other
  plan files unless the task needs them.
- If a task has an "Input needed" line, stop and ask for that input first.
- When the task is done: tick its checkbox in the phase file, and record any
  decision or measured result in docs/plan/decisions.md, in the same PR.
- Tasks tagged [agent, you review] also need a plain-English walkthrough of
  the core logic in docs/notes/<task number>.md.
- If the plan is wrong or unclear, stop and say so in the PR description
  instead of guessing.

## Source of truth
- protocol/README.md defines every message between Go and the engine.
  Never change a message without updating the spec, the Go client and the
  C++ server in the same PR.
- go/internal/store/migrations/ defines the database. Schema changes = a new
  numbered migration, never an edit to an old one.

## Build and test
- Go:  cd go && go build ./... && go test ./... && go vet ./...
- C++: cmake --preset dev && cmake --build --preset dev && ctest --preset dev
- Lint: gofmt and staticcheck for Go, clang-format for C++.

## Rules
- Tests first. Every bug fix comes with a test that failed before the fix.
- Golden files live in testdata/. Update them only with --update-golden and
  explain why in the PR.
- Go: wrap errors with context (fmt.Errorf("...: %w", err)). Every goroutine
  must exit when its context is cancelled.
- C++: C++20, no raw new/delete, RAII for every resource. The engine writes
  protocol messages to stdout ONLY; logs go to stderr.
- Never commit footage, model files or the library folder. They are gitignored.
- Keep PRs to one task from the plan. Put the task number in the PR title.
```

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

Each task's tag sets the review depth. Interviewers will dig into the core parts, so for `[agent, you review]` tasks, read the agent's walkthrough in `docs/notes/` and ask it follow-up questions until the code makes sense without it open.

| Close review: `[agent, you review]` | Light review: `[agent]` |
| --- | --- |
| MP4 box walker and GPS decoder | CLI wiring, flags, help text |
| Worker pool, priority queue, cancellation, crash recovery | SQLite migrations and query boilerplate |
| Protocol spec changes | ffmpeg command wrappers |
| Tracker, time-to-collision and distance math | CMake, vcpkg and build presets |
| Event fusion and scoring rules | Test scaffolding, fixtures, golden-file helpers |
| Benchmark results and which optimizations to keep | Progress UI, PDF layout, Qt viewer screens |

**Review checklist for every agent PR**

- [ ] Tests exist, ran, and would fail if the feature broke.
- [ ] No goroutine can leak (each one exits on context cancel).
- [ ] Protocol or schema changes are in the spec / a new migration.
- [ ] No footage, models or secrets committed.
- [ ] You can explain the change in two sentences.

## Architecture

Three parts: a Go orchestrator that plans and records the work, a C++ engine that analyzes pixels, and an optional Qt viewer that reads the results. Rule of thumb: if it looks at pixels or runs a model, it's C++; if it coordinates, stores or decides, it's Go.

&#91;embedded content: system architecture · Go stages, C++ engine, protocol link\]

Footage flows through the Go stages, top row then bottom row. Each worker drives its own engine process over the protocol link (highlighted), so an engine crash costs one retry, not the whole run.

| Part | Language | Owns | Never does |
| --- | --- | --- | --- |
| Orchestrator (`rearview`) | Go | Import, GPS, trips, motion events, scheduling, engine processes, fusion, clips, database, search ranking, CLI | Decode video or run models |
| Engine (`engine`) | C++ | Decoding, preprocessing, YOLOX, CLIP, face and plate detection, tracking, distance and TTC, blur | Write to the database or decide what counts as an event |
| Viewer (optional) | C++ with Qt | Playback, event timeline, map, overlays | Write anything (read-only) |

## Tech stack and environment setup

Go handles everything that coordinates or stores; C++ handles everything that touches pixels or runs a model. Use the latest stable release of each tool unless a row says otherwise.

**Go (orchestrator)**

| Package | Used for |
| --- | --- |
| `github.com/spf13/cobra` | CLI commands and flags |
| `github.com/charmbracelet/bubbletea` + `lipgloss` | Live progress view during `process` |
| `github.com/fsnotify/fsnotify` | Watching the import folder |
| `golang.org/x/sync/errgroup` + `context` | Running pipeline stages and cancelling them together |
| `modernc.org/sqlite` | SQLite in pure Go (no C compiler needed on the Go side) |
| `github.com/go-pdf/fpdf` | Incident report PDFs (Phase 8) |
| `os/exec` | Running ffmpeg and the C++ engine |
| Standard `testing` + `go-cmp` | Unit and golden-file tests |

**C++ (engine)**

| Library | Used for |
| --- | --- |
| C++20, CMake with presets, vcpkg (manifest mode) | Build and dependencies |
| OpenCV | Decoding (first version), resizing, drawing debug boxes, calibration, blur |
| ONNX Runtime | Running every model: CPU first, then the RX 9070 XT through Windows ML (MIGraphX) or DirectML in Phase 6 |
| FFmpeg libraries (libavcodec, libavformat) | Hardware-accelerated decoding on the RX 9070 XT (D3D11VA) in Phase 6 |
| nlohmann/json | Reading and writing protocol messages |
| Eigen | Kalman filter math in the tracker |
| GoogleTest | Unit tests |
| Qt 6 (Multimedia, Location) | Optional desktop viewer (Phase 9) |

**External tools**

| Tool | Used for |
| --- | --- |
| ffmpeg / ffprobe | Cutting clips, reading video duration and frame rate |
| exiftool | Test oracle only: its GPS output checks your parser |
| Git + GitHub | Repo, PRs, CI |

**Models (download pre-exported ONNX files; no training)**

| Model | Purpose | Licence note |
| --- | --- | --- |
| YOLOX (s or m size) | Cars, trucks, buses, people, bikes, traffic lights | Apache-2.0, no restrictions. Ultralytics YOLO is AGPL-3.0, fine only if the repo stays open source. |
| CLIP ViT-B/32 image + text encoders | Natural-language search | MIT (OpenAI CLIP) |
| OpenCV YuNet face detector | Privacy blur | Check the model zoo licence |
| A licence plate detector (ONNX) | Privacy blur | Check before choosing one |

Models go in `engine/models/` (gitignored) and are fetched by a download script with checksums, so every machine gets the same files.

**Setup checklist (Windows 11)**

- [ ] Install Go, Git, Visual Studio 2022 Build Tools ("Desktop development with C++"), CMake and vcpkg.
- [ ] Install ffmpeg and exiftool, and add both to PATH. Check with `ffmpeg -version` and `exiftool -ver`.
- [ ] AMD GPU: install the latest Adrenalin driver and check that Windows is version 24H2 or later (run winver), which Windows ML needs. No CUDA, cuDNN or ROCm install is needed on Windows.
- [ ] Clone the repo, run `go build ./...` in `go/`, and `cmake --preset dev` in `engine/` (vcpkg pulls the C++ libraries on first configure; expect 20–40 minutes for OpenCV).
- [ ] Run the model download script and check the checksums pass.

On Linux the same steps apply with gcc or clang instead of Visual Studio; everything in the stack is cross-platform.

## Repository structure

One monorepo with the Go orchestrator, the C++ engine and the protocol spec side by side, so a protocol change lands in one PR.

```
rearview/
├── .github/
│   ├── AGENTS.md                 # rules for coding agents
│   ├── skills/                   # agent skills (add-cli-command, ...)
│   └── workflows/ci.yml          # Go + C++ build and tests on every PR
├── protocol/
│   └── README.md                 # Go <-> engine message spec (source of truth)
├── go/
│   ├── go.mod
│   ├── cmd/
│   │   ├── rearview/              # the CLI binary
│   │   └── fakeengine/           # fake engine for tests (canned replies, crash on demand)
│   └── internal/
│       ├── config/               # config file loading, thresholds
│       ├── ingest/               # watch folder, stable-size check, copy, hash, tag
│       ├── mp4gps/               # MP4 box walker + GPS block decoder
│       ├── trips/                # grouping, front/rear pairing, GPS stitching
│       ├── prefilter/            # motion events from GPS / G-sensor
│       ├── queue/                # priority job queue
│       ├── engine/               # starts the C++ process, sends jobs, reads results
│       ├── pipeline/             # wires the stages together (errgroup + channels)
│       ├── fusion/               # combines signals into scored events
│       ├── clips/                # ffmpeg wrapper
│       ├── search/               # embedding storage + similarity search
│       ├── report/               # driving score, incident PDF
│       ├── bench/                # benchmark runner
│       └── store/                # SQLite access
│           └── migrations/       # 0001_init.sql, 0002_..., never edited after merge
├── engine/
│   ├── CMakeLists.txt
│   ├── CMakePresets.json
│   ├── vcpkg.json
│   ├── src/
│   │   ├── main.cpp              # stdin/stdout loop
│   │   ├── protocol/             # message parsing and writing
│   │   ├── decode/               # OpenCV first, FFmpeg + hardware decode later
│   │   ├── detect/               # preprocessing, ONNX Runtime session, NMS
│   │   ├── track/                # Kalman filter + matching
│   │   ├── geometry/             # calibration, undistortion, distance, TTC
│   │   ├── embed/                # CLIP image/text encoders, BPE tokenizer
│   │   ├── blur/                 # face/plate detection + blur
│   │   └── debug/                # writes videos with boxes drawn
│   ├── tests/                    # GoogleTest
│   ├── models/                   # ONNX files (gitignored)
│   └── scripts/download_models   # fetch + verify checksums
├── viewer/                       # optional Qt app (Phase 9)
├── testdata/
│   ├── clips/                    # a few short clips (small, committed via Git LFS)
│   ├── gps/                      # exiftool reference outputs for parser tests
│   └── labels/events.csv         # your hand-labelled events
├── bench/results/                # one JSON file per benchmark run
└── docs/                         # architecture notes, write-ups, demo assets
```

**Folders that are never committed:** your footage library, `engine/models/`, build outputs and the SQLite database. Add them to `.gitignore` on day one.

**Default runtime paths** (overridable in the config file): library in `~/rearview/library/`, database in `~/rearview/rearview.db`, clips in `~/rearview/clips/`.

## Go ↔ engine protocol

Go starts the engine as a long-running child process and they talk in newline-delimited JSON: one message per line on stdin (Go → engine) and stdout (engine → Go). This section is the starting version of `protocol/README.md`.

**Framing rules**

- UTF-8, one JSON object per line, `\n` terminated. No pretty-printing.
- stdout carries protocol messages only. All logging goes to stderr; Go forwards it to its own log with the worker id.
- Every command has an integer `id`, unique per engine process. Every reply carries the `id` it answers.
- One job at a time per engine process. Parallelism = several engine processes, one per worker.
- Unknown fields are ignored (forward compatibility). Unknown commands get an `error` reply with code `unknown_command`.

**Lifecycle**

1. Go starts `engine --models <dir> --device <cpu|dml|migraphx>`.
2. Engine loads its models, then sends `ready` with its protocol version and capabilities.
3. Go checks the version. A mismatch kills the process and fails the run with a clear message.
4. Go sends one command, reads replies until `done` or `error` for that `id`, then sends the next.
5. Go sends `cancel` to stop a job early; the engine replies `done` with `"cancelled": true`.
6. Go sends `shutdown` on exit; the engine flushes and exits with code 0 within 2 seconds, or Go kills it.
7. If the process dies or stays silent past the job timeout, Go restarts it and retries the job once.

**Commands (Go → engine)**

| Command | Phase | Purpose | Key fields |
| --- | --- | --- | --- |
| `ping` | 4 | Health check | none |
| `analyze` | 4 | Detect and track objects in time windows of one file | `path`, `camera`, `windows`, `fps`, `features` |
| `extract_frames` | 8 | Save specific frames as JPEGs | `path`, `times`, `out_dir` |
| `embed_frames` | 7 | CLIP vectors for keyframes | `path`, `every_sec` |
| `embed_text` | 7 | CLIP vector for a search query | `text` |
| `blur` | 8 | Write a copy of a clip with faces and plates blurred | `path`, `out_path` |
| `cancel` | 4 | Stop the job with this id | `target_id` |
| `shutdown` | 4 | Exit cleanly | none |

**Replies (engine → Go)**

| Type | When | Key fields |
| --- | --- | --- |
| `ready` | Once, after models load | `protocol_version`, `engine_version`, `device`, `capabilities` |
| `progress` | Every \~1 second during a job | `t` (seconds into the file), `frames_done` |
| `frame` | Per analyzed frame with objects | `t`, `objects[]` |
| `embedding` | Per keyframe, or once for text | `t` (frames only), `vector` (512 floats) |
| `done` | Job finished | `frames`, `fps`, `elapsed_ms`, `cancelled` |
| `error` | Job failed | `code`, `msg`, `t` if known |

**Examples**

```json
{"type":"ready","protocol_version":1,"engine_version":"0.3.0","device":"migraphx","capabilities":["analyze","embed_frames","embed_text"]}
{"id":41,"cmd":"analyze","path":"C:/rearview/library/2026/10/24/front_0042.mp4","camera":"front","windows":[[12.0,30.0]],"fps":10,"features":["detect","track","distance","ttc"]}
{"id":41,"type":"frame","t":12.4,"objects":[{"track":7,"cls":"car","box":[812,402,1105,640],"conf":0.91,"dist_m":9.8,"ttc_s":1.3}]}
{"id":41,"type":"progress","t":20.1,"frames_done":77}
{"id":41,"type":"done","frames":180,"fps":143.2,"elapsed_ms":1257,"cancelled":false}
{"id":42,"type":"error","code":"decode_failed","msg":"corrupt packet","t":14.2}
```

**Field details**

- `windows`: list of `[start, end]` in seconds from the start of the file. Empty list = whole file.
- `box`: `[x1, y1, x2, y2]` in pixels of the original 2560×1440 frame, not the model's input size.
- `cls`: one of `car`, `truck`, `bus`, `motorcycle`, `bicycle`, `person`, `traffic_light`. Everything else is dropped in the engine.
- `dist_m` and `ttc_s`: present only when those features were requested and the value is valid; `ttc_s` is omitted when the object is not approaching.

**Error codes:** `unknown_command`, `bad_request`, `file_not_found`, `decode_failed`, `model_error`, `out_of_memory`, `internal`. Go retries only `decode_failed`, `out_of_memory` and process crashes; the rest fail the job immediately.

**Versioning:** bump `protocol_version` for any breaking change. Go refuses to run against a different version.

## Data model

One SQLite database, written only by Go (the viewer reads it). This is the starting `0001_init.sql`; later phases add tables through new migrations.

**Conventions**

- Absolute times are UTC, stored as ISO 8601 text (`2026-10-24T18:42:07Z`). Times inside a file are seconds from its start (`t_offset_s`).
- Speeds in km/h, distances in metres or kilometres, angles in degrees. Units are in column names.
- Turn on `PRAGMA foreign_keys = ON` and `PRAGMA journal_mode = WAL` at every connection, so readers and the writer don't block each other.

```sql
-- 0001_init.sql
CREATE TABLE trips (
  id              INTEGER PRIMARY KEY,
  started_at      TEXT NOT NULL,
  ended_at        TEXT NOT NULL,
  distance_km     REAL,
  duration_s      REAL,
  max_speed_kmh   REAL,
  avg_speed_kmh   REAL,
  score           REAL                       -- Phase 8
);

CREATE TABLE segments (
  id              INTEGER PRIMARY KEY,
  path            TEXT NOT NULL UNIQUE,      -- path inside the library
  source_name     TEXT NOT NULL,             -- original file name on the SD card
  content_hash    TEXT NOT NULL UNIQUE,      -- dedup key
  size_bytes      INTEGER NOT NULL,
  camera          TEXT NOT NULL CHECK (camera IN ('front','rear','interior')),
  started_at      TEXT NOT NULL,             -- UTC, corrected with GPS time when available
  duration_s      REAL,
  width           INTEGER,
  height          INTEGER,
  fps             REAL,
  locked          INTEGER NOT NULL DEFAULT 0, -- came from the RO folder
  parking         INTEGER NOT NULL DEFAULT 0, -- parking-mode footage
  pair_key        TEXT,                      -- same value for front + rear of one moment
  trip_id         INTEGER REFERENCES trips(id),
  imported_at     TEXT NOT NULL
);
CREATE INDEX segments_started ON segments(started_at);
CREATE INDEX segments_trip ON segments(trip_id);

CREATE TABLE gps_points (
  segment_id      INTEGER NOT NULL REFERENCES segments(id) ON DELETE CASCADE,
  t_offset_s      REAL NOT NULL,
  ts_utc          TEXT NOT NULL,
  lat             REAL NOT NULL,
  lon             REAL NOT NULL,
  speed_kmh       REAL,
  heading_deg     REAL,
  accel_x         REAL, accel_y REAL, accel_z REAL, -- only if the camera stores them
  interpolated    INTEGER NOT NULL DEFAULT 0,
  PRIMARY KEY (segment_id, t_offset_s)
);

CREATE TABLE jobs (
  id              INTEGER PRIMARY KEY,
  segment_id      INTEGER NOT NULL REFERENCES segments(id) ON DELETE CASCADE,
  kind            TEXT NOT NULL,             -- analyze | embed_frames | blur
  windows_json    TEXT NOT NULL DEFAULT '[]',
  priority        INTEGER NOT NULL,          -- 0 = highest
  state           TEXT NOT NULL CHECK (state IN ('pending','running','done','failed')),
  attempts        INTEGER NOT NULL DEFAULT 0,
  last_error      TEXT,
  engine_fps      REAL,
  created_at      TEXT NOT NULL,
  started_at      TEXT,
  finished_at     TEXT,
  UNIQUE (segment_id, kind, windows_json)
);
CREATE INDEX jobs_state ON jobs(state, priority);

CREATE TABLE observations (                  -- one row per tracked object per analyzed frame
  job_id          INTEGER NOT NULL REFERENCES jobs(id) ON DELETE CASCADE,
  segment_id      INTEGER NOT NULL REFERENCES segments(id) ON DELETE CASCADE,
  t_offset_s      REAL NOT NULL,
  track_id        INTEGER NOT NULL,
  cls             TEXT NOT NULL,
  x1 REAL, y1 REAL, x2 REAL, y2 REAL,
  conf            REAL NOT NULL,
  dist_m          REAL,
  ttc_s           REAL
);
CREATE INDEX observations_seg_t ON observations(segment_id, t_offset_s);

CREATE TABLE events (
  id              INTEGER PRIMARY KEY,
  trip_id         INTEGER REFERENCES trips(id),
  segment_id      INTEGER NOT NULL REFERENCES segments(id),
  camera          TEXT,
  type            TEXT NOT NULL,             -- hard_brake | swerve | g_spike | near_miss | following_close | tailgated
  source          TEXT NOT NULL,             -- motion | vision | fused
  ts_utc          TEXT NOT NULL,
  t_start_s       REAL NOT NULL,
  t_end_s         REAL NOT NULL,
  score           REAL NOT NULL,             -- 0..1, higher = more severe
  lat REAL, lon REAL,
  details_json    TEXT NOT NULL DEFAULT '{}',
  created_at      TEXT NOT NULL
);
CREATE INDEX events_type_ts ON events(type, ts_utc);

CREATE TABLE clips (
  id              INTEGER PRIMARY KEY,
  event_id        INTEGER REFERENCES events(id) ON DELETE SET NULL,
  path            TEXT NOT NULL,
  layout          TEXT NOT NULL DEFAULT 'single', -- single | side_by_side
  blurred         INTEGER NOT NULL DEFAULT 0,
  created_at      TEXT NOT NULL
);

CREATE TABLE labels (                        -- your ground truth for accuracy numbers
  id              INTEGER PRIMARY KEY,
  ts_utc          TEXT NOT NULL,             -- keyed by time, so labels survive re-imports
  camera          TEXT,
  type            TEXT NOT NULL,
  note            TEXT
);

CREATE TABLE bench_runs (
  id              INTEGER PRIMARY KEY,
  ran_at          TEXT NOT NULL,
  git_sha         TEXT NOT NULL,
  config_json     TEXT NOT NULL,
  results_json    TEXT NOT NULL
);
```

**Added later**

- Phase 7 (`0002_search.sql`): `keyframes(id, segment_id, t_offset_s, model, vector BLOB)` with the vector stored as 512 little-endian float32 values (2,048 bytes per keyframe).
- Phase 8 (`0003_reports.sql`): anything the reports need beyond `trips.score`.

**Size check:** at 10 analyzed frames per second and about 5 objects per frame, `observations` grows by roughly 180,000 rows per hour of footage. SQLite handles that easily; if it ever becomes a problem, store only frames inside event windows.

## Feature specs

Fifteen features, each with a clear owner: Go coordinates, stores and decides; C++ looks at pixels and runs models. "Done when" is the acceptance test for the feature as a whole.

| # | Feature | Go does | C++ does | Phase | Done when |
| --- | --- | --- | --- | --- | --- |
| A | Footage library and import | Watch folder, stable-size check, copy, hash, tag camera and locked, insert rows | — | 1 | Re-running import on the same card adds 0 rows |
| B | GPS extraction | Walk MP4 boxes, decode freeGPS blocks, convert knots to km/h | — | 1 | Matches exiftool on every test file within 1e-5° and 0.1 km/h |
| C | Trips | Split on gaps, pair front/rear, stitch GPS, fix clock, compute summaries, export GPX | — | 1 | A week of drives lists as the trips you actually took; GPX lines up with real roads |
| D | Motion events | Hard braking, swerves, G-spikes from GPS; turn them into priority windows | — | 2 | Finds at least 80% of your labelled hard brakes |
| E | Detection and tracking | Choose files, windows, frame rate, features | Decode, preprocess, YOLOX via ONNX Runtime, NMS, Kalman tracker | 3 | Debug video shows stable IDs on cars and people through a 30-second clip |
| F | Near-miss (TTC) | Fuse TTC with braking and speed, score, merge | Box-growth TTC per tracked object in your path, smoothed | 5 | Measured precision and recall on labelled near-misses |
| G | Following distance | Following time = distance ÷ GPS speed; flag sustained close following | Distance from box height and camera calibration | 5 | Distance within 20% of a measured car-length test |
| H | Clip extraction | ffmpeg stream copy, join across segments, side-by-side layout | — | 2 | Any event becomes a playable 10–20 s clip in under 2 s (stream copy) |
| I | Natural-language search | Store vectors, rank by cosine similarity, group hits into moments | CLIP image and text encoders, BPE tokenizer | 7 | Right moment in top 5 for at least 70% of test queries |
| J | Privacy blur | `export --blur` command | Face and plate detection, tracked blur, write video | 8 | No readable plate or face in 5 exported test clips |
| K | Driving score | Per-trip score from events, following time, smoothness | — | 8 | Score breakdown explains every point lost |
| L | Incident report PDF | Layout, speed and G-force graph, map location, timeline | Exact frame extraction | 8 | One command produces a readable PDF for any event |
| M | CLI and progress view | cobra commands, bubbletea progress screen | — | 1–4 | Every command has `--help` and a test |
| N | Performance and benchmarks | Scheduling (skip parking, lower fps on empty roads), `bench` command | GPU inference, hardware decode, threaded pipeline, batching | 6 | `bench` reports minutes per hour of footage, with history |
| O | Desktop viewer (optional) | — | Qt app: video, event timeline, map, search | 9 | Click an event, the video jumps there, the map shows where |

**The core math (Phase 5)**

Time-to-collision from how fast a tracked car's box grows. With box height h in pixels and its growth rate dh/dt (from the tracker's smoothed estimate):

```latex
\mathrm{TTC} \approx \frac{h}{\,dh/dt\,}
```

Distance from box height, using the calibrated focal length f in pixels and an average real vehicle height H (about 1.5 m for cars; use a per-class value):

```latex
d \approx \frac{f \cdot H}{h}
```

Following time: C++ supplies d, Go supplies your speed v from GPS (in m/s):

```latex
t_{\text{follow}} = \frac{d}{v}
```

**Starting thresholds** (all in the config file; tune them against your labels in Phase 5)

| Event | Starting rule |
| --- | --- |
| Hard brake | Speed drops by 14 km/h or more within 1 second (about 0.4 g) |
| Swerve | Heading changes 30° or more within 1 second, above 30 km/h |
| Near-miss | TTC under 1.5 s for 3 or more consecutive analyzed frames, object in the centre path region |
| Following close | Following time under 1.0 s for 3 s or more, above 50 km/h |
| Tailgated | Same rule on the rear camera |

## Phase 0: Setup and dataset (week 1)

Goal: both toolchains build, the repo skeleton and plan files exist, and real footage plus a first set of labelled events are in place. Tags: **\[agent\]** = an agent does it, light review; **\[agent, you review\]** = an agent does it, close review. **Input needed** marks something an agent can't produce on its own (footage off the SD card, a real-world measurement); the agent stops and asks for it.

- [x] **0.1 Go primer \[agent, you review\]**
  - Write `docs/notes/go-primer.md`: the Go patterns this repo relies on (goroutines, buffered vs unbuffered channels, `context` cancellation, `errgroup`, worker pools), each with a short runnable example in `go/examples/`.
  - Done when: every example runs with `go run`, and the primer covers what's needed to review Phase 4 PRs.
- [x] **0.2 Toolchain setup script \[agent\]**
  - Write `scripts/setup-windows.ps1`: install or check Go, Git, Visual Studio 2022 Build Tools, CMake, vcpkg, ffmpeg and exiftool (using winget where possible), then print every version.
  - Input needed: run the script once with admin rights, and install the latest AMD Adrenalin driver.
  - Done when: the script reports every tool present.
- [x] **0.3 Repo skeleton and plan files \[agent\]**
  - Folders from Repository structure, `.gitignore` (library, models, build, `*.db`), `.github/AGENTS.md` from the template, and `docs/plan/` split from the Markdown export of this doc as listed in Working with coding agents, with a "Read first" list at the top of each phase file. `protocol/README.md` gets the protocol section.
  - Go module in `go/` with an empty `rearview` command that prints its version.
  - Input needed: the Markdown export of this doc.
  - Done when: `go build ./...` passes, the tree matches this doc, and `docs/plan/README.md` links to every plan file.
- [ ] **0.4 C++ hello world \[agent\]**
  - `engine/` with CMake presets, `vcpkg.json` listing OpenCV and nlohmann-json, and a `main.cpp` that opens an MP4 given as an argument and prints its frame count, fps and resolution.
  - Done when: it prints `2560x1440` and a sensible fps for a front-camera file.
- [ ] **0.5 CI \[agent\]**
  - GitHub Actions workflow that builds and tests Go on every PR. Add the C++ build once 0.4 is stable (cache vcpkg so it doesn't take 40 minutes each time).
  - Done when: a PR shows green checks.
- [ ] **0.6 Footage backup \[agent\]**
  - Write `scripts/backup-footage.ps1`: copy everything from the SD card into a raw folder outside the repo and verify checksums.
  - Input needed: the SD card plugged in, holding 1–2 weeks of drives (ideally some night, rain and highway driving, plus a few locked RO clips).
  - Done when: at least 5 hours of footage are backed up outside the card with verified checksums.
- [ ] **0.7 Labelling tool and first labels \[agent, you review\]**
  - Build `rearview label add --at <UTC time> --camera front --type near_miss [--note ...]`, which validates and appends to `testdata/labels/events.csv` (columns `ts_utc,camera,type,note`), plus `rearview label list`.
  - Input needed: event timestamps from watching the footage at 2× (hard brakes, close calls, cut-offs, tailgaters). Aim for 50–100 by the end of Phase 4 and keep adding.
  - Done when: the file has at least 20 validated rows.
- [ ] **0.8 Test clips \[agent\]**
  - Trim three 10–30 second clips (busy city, highway, night) with ffmpeg and commit them to `testdata/clips/` via Git LFS. These feed every automated test later.
  - Input needed: the file and start time of one good stretch of each kind.

**Phase 0 deliverable:** a repo with green CI, an engine binary that reads your footage, and a labelled dataset.

## Phase 1: Go foundation (weeks 2–3)

Goal: `rearview import` then `rearview trips` lists your real drives with correct distances and times, and the GPX export lines up with real roads on a map.

- [ ] **1.1 MP4 box walker \[agent, you review\]**
  - `internal/mp4gps`: read the box tree (4-byte big-endian size + 4-byte type; size 1 means a 64-bit size follows; size 0 means "to end of file"). Recurse into container boxes (`moov`, `trak`, `mdia`, `minf`, `stbl`, `udta`).
  - Use `io.ReaderAt` and read headers only; never load a whole file into memory (files are hundreds of MB).
  - Done when: a `rearview debug boxes <file>` command prints the box tree of a real file, and a unit test covers 64-bit sizes and a truncated file.
- [ ] **1.2 GPS block decoder \[agent, you review\]**
  - Find the GPS blocks (marked `freeGPS`), decode time, active flag, latitude, longitude, speed and course. Values are little-endian floats; speed is in knots (× 1.852 = km/h).
  - Work out the coordinate format by decoding a point at a known location, then compare against `exiftool -ee`.
  - Skip records where the active flag says there is no fix. Look for accelerometer values in the same blocks and decode them if present.
  - Input needed: the coordinates of one known spot (for example the driveway) and a file recorded there.
  - Done when: golden test compares the output with saved exiftool output for 3+ files: latitude/longitude within 1e-5°, speed within 0.1 km/h, same number of points.
- [ ] **1.3 Store and migrations \[agent\]**
  - `internal/store`: open the database with WAL and foreign keys, apply numbered migrations from an embedded folder (`embed.FS`), record applied versions.
  - Add `0001_init.sql` from `docs/plan/data-model.md`.
  - Done when: opening a fresh database creates every table; opening it again changes nothing; tests run against a temp file.
- [ ] **1.4 Config file \[agent\]**
  - `internal/config`: YAML or TOML file with library path, database path, clip folder, file-name patterns and every threshold from Feature specs. Defaults in code; the file overrides them.
  - Done when: `rearview config show` prints the merged config.
- [ ] **1.5 Ingest \[agent, you review\]**
  - `internal/ingest`: given a source folder, find MP4s, wait until each file's size is unchanged for 5 seconds, compute the hash (SHA-256 of the first and last 4 MB plus the size), skip known hashes, copy into `library/YYYY/MM/DD/`, then insert the `segments` row.
  - Tag camera from the file name using patterns from config (front/rear suffix); mark `locked` for files under an `RO` folder.
  - Read duration, resolution and fps with `ffprobe -print_format json`.
  - Copy to a temp name and rename at the end, so a crash never leaves half a file in the library.
  - Done when: importing the same folder twice adds 0 rows; pulling the card mid-import leaves no partial files; tests cover each case using small fake files.
- [ ] **1.6 Import command \[agent\]**
  - `rearview import <path>` runs ingest + GPS extraction for each new segment and prints a summary (new, skipped, failed). `--watch` keeps watching the folder with fsnotify.
  - Done when: running it on the backed-up footage imports everything with sensible counts.
- [ ] **1.7 Trip assembly \[agent, you review\]**
  - `internal/trips`: sort segments by time; a gap over 5 minutes (configurable) starts a new trip. Give front and rear files that start within 2 seconds of each other the same `pair_key`.
  - Correct `started_at` using GPS UTC time when a fix exists (store the offset so files without a fix get the same correction).
  - Fill GPS gaps under 10 seconds by linear interpolation (`interpolated = 1`); leave longer gaps empty.
  - Compute distance (haversine sum), duration, average and maximum speed.
  - Done when: a test with hand-built segments covers gaps, pairing and clock correction, and the review confirms the real week of footage splits into the drives actually taken.
- [ ] **1.8 Trips command and GPX export \[agent\]**
  - `rearview trips` lists trips (date, start time, duration, distance, max speed). `rearview trips export <id> --gpx out.gpx` writes a GPX track.
  - Done when: the GPX opens in a map viewer and follows the roads driven.

**Phase 1 deliverable:** a working importer and trip log built on your own MP4 parser.

## Phase 2: Motion events and clips (weeks 3–4)

Goal: the program finds your real hard brakes and swerves from GPS alone and clips them, with no AI involved yet. This is already a useful tool and your first accuracy number.

- [ ] **2.1 Motion event detectors \[agent, you review\]**
  - `internal/prefilter`: run over each trip's stitched GPS track. Hard brake = speed drop past the threshold within 1 second; swerve = heading change past the threshold at speed; G-spike = accelerometer magnitude past the threshold (only if 1.2 found accelerometer data).
  - Handle heading wraparound (359° → 1° is a 2° change, not 358°). Ignore interpolated points.
  - Merge detections closer than 3 seconds into one event; score 0–1 by how far past the threshold they went.
  - Done when: table-driven tests with synthetic speed traces cover each detector, wraparound, and merging.
- [ ] **2.2 Priority windows \[agent, you review\]**
  - Turn each motion event into an analysis window (10 seconds before to 5 seconds after), mapped back to segment + `t_offset_s` for both cameras of the pair.
  - Windows crossing a file boundary split into two windows, one per file.
  - Done when: a test with an event 2 seconds before a file boundary produces two correct windows.
- [ ] **2.3 Clip extraction \[agent\]**
  - `internal/clips`: ffmpeg with `-ss` before `-i` and `-c copy` for speed. Events crossing a file boundary: cut both parts, then join them with the concat demuxer.
  - `--exact` flag re-encodes for frame-accurate cuts; `--side-by-side` stacks front and rear with `hstack` (re-encodes).
  - Clip names: `YYYYMMDD_HHMMSS_<type>_<event id>.mp4`. Insert a `clips` row.
  - Done when: clips play in VLC, cross-boundary clips have no gap or freeze, and a stream-copy clip takes under 2 seconds.
- [ ] **2.4 Events and clip commands \[agent\]**
  - `rearview events [--type hard_brake] [--since 7d] [--trip <id>]` lists events newest first with time, type, score and location.
  - `rearview clip <event id> [--side-by-side] [--exact]` writes the clip and prints its path.
  - Done when: both commands have tests and `--help` text.
- [ ] **2.5 Evaluation command \[agent, you review\]**
  - `rearview eval --type hard_brake` compares detected events with `labels` (a match = within 3 seconds and same type) and prints precision, recall and the list of misses and false alarms with timestamps.
  - Add an `import-labels` command that loads `testdata/labels/events.csv` into the `labels` table.
  - Done when: it prints numbers for the real data, recorded in `docs/plan/decisions.md`.
- [ ] **2.6 Tune thresholds \[agent, you review\]**
  - Pull the speed trace and a clip for every miss and false alarm, propose threshold changes in config with the reasoning, re-run `eval`. Stop when recall is at least 80% without drowning in false alarms.
  - Input needed: a verdict on borderline clips the agent flags (real event or not).

**Phase 2 deliverable:** real events with clips and a first precision/recall number.

## Phase 3: C++ engine v1 (weeks 4–6)

Goal: `engine detect clip.mp4 --debug-video out.mp4` produces a video with stable, labelled boxes on cars and people. Everything runs on the CPU for now; speed comes in Phase 6.

- [ ] **3.1 Model download script \[agent\]**
  - `engine/scripts/download_models`: fetch YOLOX-s (and later CLIP) ONNX files, verify SHA-256 checksums, place them in `engine/models/`.
  - Done when: running it on a clean machine produces the same files every time.
- [ ] **3.2 Frame source \[agent, you review\]**
  - `src/decode`: an interface `FrameSource` with `seek(seconds)` and `next() -> optional<Frame{mat, t}>`. First implementation wraps `cv::VideoCapture`.
  - Sample at a requested fps by skipping frames (grab without decoding where possible). Respect time windows.
  - Done when: asking for 10 fps over a 20-second window returns 200 frames (±1) with correct timestamps.
- [ ] **3.3 Preprocessing \[agent, you review\]**
  - `src/detect`: letterbox resize to the model input (640×640 for YOLOX-s), pad with grey (114), convert HWC BGR to NCHW float.
  - Check the YOLOX preprocessing conventions for the exported model (channel order, whether to normalize). Getting this wrong gives silent garbage, not an error.
  - Keep the scale and padding so boxes map back to 2560×1440 coordinates.
  - Done when: a unit test maps a box through letterbox and back and gets the same box within 1 pixel.
- [ ] **3.4 Inference + post-processing \[agent, you review\]**
  - Create one ONNX Runtime session at startup and reuse it. Run the model, then decode outputs into boxes. If the export doesn't include the decoding step, apply YOLOX grid decoding (strides 8, 16, 32).
  - Keep only COCO classes person (0), bicycle (1), car (2), motorcycle (3), bus (5), truck (7), traffic light (9). Confidence threshold 0.3, per-class NMS at IoU 0.45 (both configurable).
  - Done when: unit tests cover IoU and NMS on hand-made boxes, and the review confirms detections in the debug output look right.
- [ ] **3.5 Debug video writer \[agent\]**
  - `src/debug`: draw boxes, class, confidence and track ID onto frames and write an MP4 (OpenCV `VideoWriter`). Colour by class.
  - Done when: the output plays in VLC.
- [ ] **3.6 Tracker \[agent, you review\]**
  - `src/track`: ByteTrack-style. Kalman filter with a constant-velocity state for box centre, aspect ratio and height (Eigen). Match high-confidence detections to predicted tracks by IoU first, then low-confidence ones to the leftovers.
  - Track states: tentative (needs 3 hits) → confirmed → lost (kept 30 frames) → removed.
  - Expose each track's smoothed height and height velocity: Phase 5 needs them for TTC.
  - Done when: unit tests with synthetic moving boxes keep IDs through a 5-frame occlusion, and on the city test clip a car crossing the frame keeps one ID.
- [ ] **3.7 Engine CLI \[agent\]**
  - `engine info` prints versions, available execution providers and model input shapes.
  - `engine detect <file> [--fps 10] [--from 12 --to 30] [--debug-video out.mp4] [--json]` runs the full chain and prints JSON lines in the protocol's `frame` format.
  - Done when: both commands work on the three test clips.
- [ ] **3.8 Stage timing \[agent\]**
  - Time decode, preprocess, inference, post-process and tracking separately; print a summary at the end. This is the Phase 6 baseline.

**Phase 3 deliverable:** a debug video of your own footage with stable tracked boxes, plus per-stage CPU timings.

## Phase 4: Connect Go and C++ (weeks 6–7)

Goal: `rearview process` runs the real engine over your library in parallel, survives the engine being killed mid-run, and resumes where it stopped after a restart.

- [ ] **4.1 Engine server loop \[agent, you review\]**
  - `engine serve`: load models, send `ready`, then read one JSON command per line from stdin and stream replies on stdout. Implement `ping`, `analyze`, `cancel`, `shutdown` from the protocol spec.
  - Flush stdout after every message. Read `cancel` on a separate thread so a long job can be stopped.
  - Done when: a GoogleTest feeds commands through a fake stdin and checks every reply type, including `error` for a missing file and `unknown_command`.
- [ ] **4.2 Fake engine \[agent\]**
  - `go/cmd/fakeengine`: speaks the same protocol with canned replies. Flags make it slow, return errors, go silent, or crash after N jobs.
  - Done when: Go tests can run the whole pipeline without the C++ engine built.
- [ ] **4.3 Engine client \[agent, you review\]**
  - `internal/engine`: start the process with `os/exec`, wait for `ready`, check `protocol_version`, forward stderr to the log.
  - `Run(ctx, job) (<-chan Reply, error)`: send the command, stream replies until `done` or `error`. Context cancel → send `cancel`, then kill the process if it doesn't finish within 2 seconds.
  - Detect a dead process (stdout closed, exit) and a hung one (no message within the job timeout). Restart, then retry the job once.
  - Done when: tests with the fake engine cover normal runs, errors, crash-and-retry, hang-and-retry, and cancel.
- [ ] **4.4 Job planner and priority queue \[agent, you review\]**
  - `internal/queue`: jobs come from the `jobs` table. Priority 0 = locked RO files, 1 = motion-event windows, 2 = everything else (skip parking footage).
  - Use the two-channel select pattern (high, low) or a heap guarded by a mutex; workers always drain priority 0 first.
  - Done when: a test enqueues mixed jobs and checks they come out in priority order.
- [ ] **4.5 Pipeline wiring \[agent, you review\]**
  - `internal/pipeline`: errgroup with planner → bounded jobs channel → N workers (each owns one engine process) → bounded results channel → a single writer goroutine that saves `observations` and job state.
  - Close the results channel only after all workers finish (WaitGroup). One writer goroutine avoids SQLite write contention.
  - Done when: a test with the fake engine processes 100 jobs with 4 workers, and `go test -race` passes.
- [ ] **4.6 Resume after crash \[agent, you review\]**
  - Job states are saved as they change. At startup, any job left `running` goes back to `pending` (its partial observations are deleted first).
  - Done when: killing `rearview process` mid-run and starting it again finishes the remaining jobs without duplicating rows.
- [ ] **4.7 Process command + progress view \[agent\]**
  - `rearview process [--workers 2] [--only-windows] [--device cpu|dml|migraphx]` with a bubbletea screen: jobs done/total, frames per second per worker, queue depth, failures, ETA.
  - Plain log output when not running in a terminal (for CI).
  - Done when: it runs on a full day of footage and shows sensible numbers.

**Phase 4 deliverable:** a robust parallel pipeline. Demo: kill an engine process with Task Manager mid-run and watch it recover.

## Phase 5: Near-miss and following distance (weeks 7–8)

Goal: `rearview events --type near_miss` finds your real close calls with a measured precision and recall, and following distance is accurate enough to trust.

- [ ] **5.1 Camera calibration \[agent, you review\]**
  - `engine calibrate <video> --board 9x6 --square-mm 25`: extract sharp frames, find checkerboard corners, run calibration, print the reprojection error, save `calibration/front.json` (focal length, centre, distortion).
  - The 140° front lens is very wide: try OpenCV's standard model and its fisheye model, keep whichever gives the lower reprojection error. Repeat for the rear camera.
  - Also write `docs/notes/calibration-howto.md`: how to print the board and what to record.
  - Input needed: a checkerboard (9×6 inner corners, squares about 25 mm) printed on stiff card, recorded with the front and rear cameras at 20+ angles and distances, including the corners of the frame.
  - Done when: reprojection error is under 1 pixel and undistorted frames show straight lines (lamp posts, building edges) as straight.
- [ ] **5.2 Distance estimation \[agent, you review\]**
  - `src/geometry`: undistort the bottom-centre point of each box, then distance = focal length × real height ÷ box height (formula in Feature specs). Per-class heights in config (car about 1.5 m, truck and bus larger, person about 1.7 m).
  - Input needed: a field-test recording. Park behind a car, tape-measure 5, 10 and 15 m, and record 10 seconds at each distance.
  - Done when: estimates on the field test land within 20%, and the clip is saved in `testdata/` as a regression test.
- [ ] **5.3 Time-to-collision \[agent, you review\]**
  - Use the tracker's smoothed height and height velocity: TTC = h ÷ (dh/dt), only when dh/dt > 0 (approaching) and the box is at least 40 px tall (smaller boxes are too noisy).
  - "In the path" = box bottom-centre inside a trapezoid in front of the car (configurable corners, drawn on the debug video). Ignore objects outside it.
  - Add `dist_m` and `ttc_s` to `frame` replies when requested.
  - Done when: the debug video shows TTC counting down when approaching a stopped car, and unit tests cover the approaching / receding / too-small cases.
- [ ] **5.4 Event fusion \[agent, you review\]**
  - `internal/fusion`: scan `observations` per track. Near-miss = TTC under threshold for 3+ consecutive frames in the path region; score rises as minimum TTC falls, with a boost when a hard brake happens within 2 seconds.
  - Following close = following time (distance ÷ GPS speed) under threshold for 3+ seconds above 50 km/h. Tailgated = the same rule on the rear camera.
  - Merge events of the same type within 3 seconds; one moment seen by both cameras becomes one event. Save minimum TTC, distance, object class and track ID in `details_json`.
  - Done when: table-driven tests with synthetic observation sequences cover each rule, merging and the brake boost.
- [ ] **5.5 Evaluate and tune \[agent, you review\]**
  - Extend `rearview eval` to `near_miss`, `following_close` and `tailgated`. For every miss and false alarm, export the debug clip and a short note on the likely cause, propose threshold and path-trapezoid changes, re-run.
  - Expect false alarms from parked cars on curves, merging traffic and wipers at night; record each fix in `docs/plan/decisions.md`.
  - Input needed: a verdict on the clips the agent flags as borderline.
  - Done when: precision and recall for each event type are recorded in `docs/plan/decisions.md`. These are resume numbers.

**Phase 5 deliverable:** the smart part of the project, with honest accuracy numbers.

## Phase 6: Performance (weeks 8–9)

Goal: a headline number ("1 hour of 1440p footage in X minutes") and a record of how each change moved it. Measure first, change one thing, measure again.

- [ ] **6.1 Benchmark command \[agent, you review\]**
  - `rearview bench [--set testdata/bench]`: runs a fixed set of footage (1 hour total, mixed city/highway/night) through the full pipeline and records: minutes per hour of footage, frames per second per stage (decode, preprocess, inference, post-process, tracking), worker utilization, peak memory, and GPU utilization (read from Windows' "GPU Engine" performance counters, which work on AMD).
  - Save each run to `bench_runs` and `bench/results/<date>_<sha>.json`. `rearview bench compare` prints the last two runs side by side.
  - Benchmarks run on the dev PC (7800X3D + RX 9070 XT), so the agent runs them locally, not in CI.
  - Done when: two runs on the same commit agree within 5%.
- [ ] **6.2 Baseline on the 7800X3D \[agent\]**
  - Run `bench` on the CPU-only Phase 4 build. Before recording the baseline, compare 2 workers × 4 ONNX Runtime threads against 1 worker × 8 threads (8 cores, 16 threads) and keep the faster one.
  - Record the numbers and which stage dominates in `docs/plan/decisions.md` before changing anything.
- [ ] **6.3 GPU inference on the RX 9070 XT \[agent, you review\]**
  - No CUDA or TensorRT on AMD. Build two GPU routes behind `--device` and keep whichever `bench` says is faster.
  - Route 1, DirectML (`--device dml`): ONNX Runtime's DirectML build runs on any DirectX 12 GPU and is the quickest way to get GPU numbers. Microsoft has put DirectML in maintenance mode, so treat it as the fallback ([Windows ML docs](https://learn.microsoft.com/en-us/windows/ai/new-windows-ml/webgpu-ep)).
  - Route 2, MIGraphX (`--device migraphx`): Microsoft's recommended AMD GPU path is Windows ML with AMD's MIGraphX execution provider, downloaded on demand on Windows 11 24H2 or later. It means linking against Windows ML's shared ONNX Runtime instead of a bundled copy, so keep it behind a CMake option (`REARVIEW_WINML=ON`).
  - MIGraphX compiles each model for the GPU on first run, which can be slow. Cache the compiled model next to the ONNX file so later runs start fast, and exclude the first run from `bench`.
  - Check the session logs for operators that fall back to the CPU; a model split between CPU and GPU can be slower than CPU alone.
  - Done when: `bench` shows the inference speedup for each route, and detections match the CPU run on the test clips (same boxes within a small tolerance).
- [ ] **6.4 Hardware video decoding \[agent, you review\]**
  - Replace the OpenCV frame source with one using the FFmpeg libraries and D3D11VA hardware decoding, which runs on the RX 9070 XT's video engine, behind the same `FrameSource` interface. (On Linux, the same code path would use VAAPI.)
  - Decoding 2560×1440 on the CPU is often the real bottleneck once inference is on the GPU.
  - Done when: same frames and timestamps as the OpenCV source on the test clips, and `bench` shows the decode stage speedup.
- [ ] **6.5 Threaded pipeline inside the engine \[agent, you review\]**
  - Decode, preprocess, inference and tracking on separate threads connected by bounded queues, so the GPU is never waiting for the decoder.
  - Done when: GPU utilization rises in `bench` and results are identical to the single-threaded run.
- [ ] **6.6 Batched inference \[agent\]**
  - Run 4–8 frames per inference call. Measure; keep it only if it helps on the RX 9070 XT.
- [ ] **6.7 Smarter scheduling in Go \[agent, you review\]**
  - Lower the analysis fps on empty highway stretches (no objects in the last N frames), full fps around motion events, skip parking footage entirely.
  - Worker count: the 9070 XT has 16 GB of video memory, so start with 2 GPU workers and add more only if `bench` improves. Leave CPU cores free for decoding and the Go side.
  - Done when: `bench` shows the gain and Phase 5 accuracy numbers did not drop (re-run `eval`).
- [ ] **6.8 Results table \[agent\]**
  - Generate a markdown table from `bench_runs` (change, minutes per hour, speedup vs baseline) for the README.

**Phase 6 deliverable:** the performance story with numbers for every step.

## Phase 7: Natural-language search (weeks 9–10)

Goal: `rearview search "red truck"` returns the right moments from your own drives, with a measured hit rate.

- [ ] **7.1 CLIP image encoder \[agent, you review\]**
  - `src/embed`: CLIP ViT-B/32 image model via ONNX Runtime. Preprocess: resize the short side to 224, centre-crop 224×224, RGB, scale to 0–1, normalize with CLIP's mean (0.48145466, 0.4578275, 0.40821073) and std (0.26862954, 0.26130258, 0.27577711). L2-normalize the 512-value output.
  - Dashcam frames are very wide, so a centre crop loses the sides. Also try three crops (left, centre, right) per keyframe and keep whichever scores better in 7.6.
  - Done when: embedding the same frame twice gives identical vectors, and a sanity test ranks a photo of a car above a photo of a tree for the text "a car".
- [ ] **7.2 BPE tokenizer + text encoder \[agent, you review\]**
  - Implement CLIP's byte-pair-encoding tokenizer in C++ from its vocabulary and merges files: lowercase, clean whitespace, split into words, apply merges, add start token 49406 and end token 49407, pad to 77 tokens.
  - Run the CLIP text model, L2-normalize the output.
  - Done when: golden tests match reference token IDs for 20 sample strings (taken once from a trusted implementation and committed).
- [ ] **7.3 Protocol commands \[agent\]**
  - Add `embed_frames` and `embed_text` to both sides and the spec, with round-trip tests. Adding commands is not a breaking change, so the protocol version stays the same.
- [ ] **7.4 Keyframe indexing \[agent, you review\]**
  - Migration `0002_search.sql` adds `keyframes`. Planner creates `embed_frames` jobs at one keyframe per second (front camera first). Store vectors as 512 little-endian float32 values.
  - Done when: `rearview process --embed` fills `keyframes` for a day of footage, and re-running skips finished segments.
- [ ] **7.5 Search \[agent, you review\]**
  - `internal/search`: load all vectors into memory (a week of footage is well under 100 MB), get the query vector from `embed_text` with a prompt template such as "a dashcam photo of {query}", score by dot product, keep the top 50.
  - Group hits within 5 seconds of each other in the same segment into one moment; rank moments by their best score.
  - `rearview search "<query>" [--limit 10] [--clip]` prints time, trip, score and location; `--clip` cuts a clip for each result.
  - Done when: a query over a week of footage answers in under 1 second.
- [ ] **7.6 Evaluate \[agent, you review\]**
  - Draft 20–30 candidate test queries ("snowy road", "bus", "cyclist", "night rain", "traffic light", "bridge") by sampling keyframes, each with the time range where it appears, in `testdata/search_queries.csv`.
  - `rearview eval --search` reports the share of queries with a correct moment in the top 5. Compare centre-crop vs three-crop, and prompt template vs plain query.
  - Input needed: confirm or correct each query's expected time range, since only someone who knows the footage can vouch for them.
  - Done when: the number is recorded in `docs/plan/decisions.md`.

**Phase 7 deliverable:** search that works on your own drives, with a hit rate. With this, the core project is complete.

## Optional phases 8–9: Extras and desktop viewer

Only start these after Phase 7 is done and Phase 10 polish is at least drafted; each item stands alone, so pick the ones you like.

**Phase 8: Extras (about 1–2 weeks)**

- [ ] **8.1 Privacy blur \[agent, you review\]**
  - Engine `blur` command: run the OpenCV YuNet face detector and a plate detector on every frame, track the boxes so the blur doesn't flicker, expand each box by 15%, apply a strong Gaussian blur, write the video (re-encoded).
  - `rearview export <event id> --blur` cuts the clip, sends it to the engine, saves the result.
  - Input needed: a frame-by-frame check of 5 exported test clips with plates and pedestrians.
  - Done when: nothing readable remains in those 5 clips.
- [ ] **8.2 Driving score \[agent, you review\]**
  - Per-trip score out of 100: points lost per hard brake, swerve, near-miss and minute of close following, scaled per 100 km. Save to `trips.score`.
  - `rearview trips --score` shows the score and a breakdown of every point lost; a weekly trend line in the terminal.
  - Done when: the breakdown always adds up to the score.
- [ ] **8.3 Incident report PDF \[agent\]**
  - `rearview report <event id>`: one-page PDF with 3 key frames (engine `extract_frames`), a speed and G-force graph for 30 seconds around the event, coordinates and nearest timestamp, event details and the clip file name.
  - Done when: the PDF is readable printed in black and white.

**Phase 9: Desktop viewer (about 2 weeks)**

- [ ] **9.1 Qt app skeleton \[agent\]** — Qt 6 app in `viewer/` that opens the SQLite database read-only and lists trips.
- [ ] **9.2 Video + timeline \[agent, you review\]** — play a trip's segments back to back (Qt Multimedia), switch front and rear, event markers on a timeline that jump the video when clicked.
- [ ] **9.3 Map \[agent\]** — GPS track on a map (Qt Location with OpenStreetMap tiles), a marker that follows playback, event pins.
- [ ] **9.4 Overlays \[agent, you review\]** — draw tracked boxes and TTC from `observations` on top of the video at the current time.
- [ ] **9.5 Search bar \[agent\]** — run `rearview search --json` as a subprocess and show results; clicking one jumps to it.

* Done when: the full demo (event list → click → video jumps → map follows → search finds a cyclist) runs without touching the terminal.

## Phase 10: Portfolio polish (week 10–11)

Goal: someone who spends 2 minutes on the repo understands what it does, sees it work, and sees the numbers. Do this phase even if Phases 8–9 never happen.

- [ ] **10.1 README \[agent, you review\]**
  - Top: one sentence on what it does, the demo GIF or video, and the 3 headline numbers (near-miss precision/recall, search top-5 rate, minutes per hour of footage).
  - Then: architecture diagram, why Go and C++ (one paragraph), the protocol in brief, how to build and run, results tables, limitations.
- [ ] **10.2 Demo video, 60–90 seconds \[agent, you review\]**
  - Write `docs/notes/demo-script.md`: the exact commands and shot list (import → process with the progress view → near-miss found → clip plays with boxes and TTC → search "cyclist" → result plays). Then cut the raw recording into the final video with ffmpeg, adding captions and blurring other people's plates.
  - Input needed: a screen recording of the demo script being run.
- [ ] **10.3 Results tables \[agent\]**
  - Accuracy per event type (precision, recall, number of labelled events), search top-5 rate, benchmark history from 6.8. State the dataset size honestly (hours of footage, number of labels).
- [ ] **10.4 Write-up \[agent, you review\]**
  - Draft `docs/writeup.md` from `docs/plan/decisions.md` and `docs/notes/`: the 2–3 hardest problems (decoding the GPS blocks, getting TTC to stop crying wolf, the performance journey), including what didn't work. The review should rewrite it in your own voice before publishing.
- [ ] **10.5 Clean-up \[agent\]**
  - Remove dead code, make sure `go vet`, `staticcheck`, `clang-format` and all tests pass, pin dependency versions, add a licence file.
- [ ] **10.6 Resume line and interview prep \[agent, you review\]**
  - Fill in the resume line with the real numbers: "Built a Rearview, a dashcam analysis system in Go and C++: a concurrent Go pipeline with crash-isolated C++ inference workers, GPU-accelerated object detection and tracking, near-miss detection via time-to-collision estimation (X% precision), natural-language video search with CLIP embeddings, and processing of 1 hour of 1440p footage in Y minutes."
  - Write `docs/notes/interview-prep.md`: the questions an interviewer is likely to ask about each core module (GPS decoding, crash recovery, TTC and its false alarms, the biggest performance win), each answered with pointers into the code.

## Testing and evaluation strategy

Two kinds of checking run side by side: tests that prove the code is correct, and evaluations that prove the results are good. Tests run on every PR; evaluations run when you change a model, threshold or the scheduler.

| Layer | What it checks | Tools | Where |
| --- | --- | --- | --- |
| Go unit tests | Parsers, detectors, queue order, fusion rules, scoring | `testing`, table-driven tests, `go-cmp` | Next to each package |
| Go race tests | No data races in the pipeline | `go test -race ./...` | CI, every PR |
| Golden files | GPS parser output vs exiftool, tokenizer IDs, protocol messages | `testdata/` + an `--update-golden` flag | `testdata/gps/`, `testdata/tokens/` |
| Pipeline tests | Worker pool, crash and hang recovery, cancel, resume | Fake engine (`go/cmd/fakeengine`) | `internal/pipeline` |
| C++ unit tests | Letterbox math, IoU, NMS, Kalman filter, tracker IDs, TTC, distance | GoogleTest | `engine/tests/` |
| Protocol round-trip | Every command and reply survives Go → engine → Go | Real engine on a 10-second test clip | Integration test, CI once C++ builds there |
| End-to-end smoke | import → process → events on the 3 test clips finishes with sane output | CLI run in CI | `.github/workflows/ci.yml` |
| Accuracy evaluation | Precision and recall per event type against your labels | `rearview eval` | Run by hand; results in Decisions log |
| Search evaluation | Correct moment in top 5 | `rearview eval --search` | Run by hand |
| Benchmarks | Minutes per hour of footage, per-stage fps | `rearview bench` | Run by hand; results in `bench/results/` |

**Rules that keep the numbers honest**

- Keep a held-out set: label at least one full day of driving that you never look at while tuning, and report final numbers on it.
- A match counts only within 3 seconds and with the same event type.
- Re-run `eval` after every performance change; a faster pipeline that misses more events is not an improvement.
- Report dataset size next to every number (hours of footage, number of labelled events).

## Risks, gotchas and decisions log

The biggest risks are a GPS format that differs from what's documented, a wide-angle lens that makes distance inaccurate, and night or snow footage that hurts detection. Each has a fallback below.

**Risks**

| Risk | Impact | Mitigation |
| --- | --- | --- |
| GPS block layout differs on your firmware | Phase 1 stalls | Hex-dump a file, compare with exiftool output, decode field by field; exiftool can serve as a temporary fallback |
| Firmware update changes file names | Import mis-tags cameras | Name patterns live in config; fall back to resolution and embedded metadata |
| 140° lens distortion skews distance | Following distance unreliable | Calibrate with both lens models (5.1); report distance with its measured error |
| Night, rain, snow and glare | More misses and false alarms | Report accuracy per condition; keep night clips in the held-out set |
| TTC noise from small or jittery boxes | False near-misses | Minimum box height, smoothing, consecutive-frame rule, path trapezoid |
| AMD GPU inference on Windows is less mature than CUDA (slow first-run model compile, some operators falling back to CPU) | Phase 6 delays | Everything works on CPU first (the 7800X3D is a strong baseline); DirectML as the fallback; Linux with ROCm as a last resort |
| vcpkg builds are slow | Slow CI and setup | Cache vcpkg in CI; build OpenCV with only the modules you use |
| Scope creep | Project never finishes | Phases 8–9 are optional; Phase 10 happens regardless |
| Agent-written core code that can't be explained in an interview | Weak interviews | Close review on \[agent, you review\] tasks, walkthroughs in docs/notes/, interview prep in task 10.6 |

**Gotchas to remember**

- The engine must never print anything except protocol messages to stdout. One stray debug print breaks the Go parser.
- Loop recording overwrites footage: import before analysing, never analyse straight from the card.
- `-ss` before `-i` with stream copy cuts on keyframes; use `--exact` when frame accuracy matters.
- Heading wraps at 360°; speed in the GPS blocks is knots; times in the database are UTC.
- SQLite: one writer goroutine, WAL mode, foreign keys turned on per connection.
- Don't publish clips with other people's plates or faces visible; use the blur export.

**Decisions log** (newest first; add a row whenever you make a call you'd want to explain later)

| Date | Decision | Why |
| --- | --- | --- |
| 2026-10-25 | GPU inference via Windows ML + MIGraphX, DirectML as fallback | Dev GPU is an AMD RX 9070 XT, so CUDA and TensorRT are out; MIGraphX is Microsoft's recommended AMD path |
| 2026-10-25 | Go ↔ engine over stdin/stdout JSON lines, not cgo | Simpler builds, crash isolation, easy to test with a fake engine |
| 2026-10-25 | YOLOX instead of Ultralytics YOLO | Apache-2.0 licence, ready ONNX exports |
| 2026-10-25 | Pretrained models only, no training | No Python in the repo; the value is in the systems around the models |
| 2026-10-25 | SQLite via pure-Go driver | No C compiler on the Go side; one file to back up |
| 2026-10-25 | Go orchestrator + C++ engine for the Rearview project | Shows concurrency and native performance; differs from Fullcut's web stack |

**Results** (fill in as you go; these become the README numbers)

| Metric | Value | Dataset | Date |
| --- | --- | --- | --- |
| Hard brake precision / recall |  |  |  |
| Near-miss precision / recall |  |  |  |
| Following close precision / recall |  |  |  |
| Distance error at 5 / 10 / 15 m |  |  |  |
| Search: correct moment in top 5 |  |  |  |
| Minutes per hour of footage (CPU baseline) |  |  |  |
| Minutes per hour of footage (final) |  |  |  |
