# Repository structure

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
│   │   ├── rearview/             # the CLI binary
│   │   └── fakeengine/           # fake engine for tests (canned replies, crash on demand)
│   ├── examples/                 # runnable Go primer examples (docs/notes/go-primer.md)
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
├── scripts/                      # setup-windows.ps1, backup-footage.ps1
└── docs/                         # architecture notes, write-ups, demo assets
```

Empty folders hold a `.gitkeep` until their task adds real files. `ci.yml` arrives in task 0.5, the engine's CMake files in 0.4, `events.csv` in 0.7.

**Folders that are never committed:** your footage library, `engine/models/`, build outputs and the SQLite database. They are in `.gitignore`.

**Default runtime paths** (overridable in the config file): library in `~/rearview/library/`, database in `~/rearview/rearview.db`, clips in `~/rearview/clips/`.
