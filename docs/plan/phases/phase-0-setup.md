# Phase 0: Setup and dataset (week 1)

**Read first:** [../architecture.md](../architecture.md), [../tech-stack.md](../tech-stack.md), [../repo-structure.md](../repo-structure.md)

Goal: both toolchains build, the repo skeleton and plan files exist, and real footage plus a first set of labelled events are in place. Tags: **[agent]** = an agent does it, light review; **[agent, you review]** = an agent does it, close review. **Input needed** marks something an agent can't produce on its own (footage off the SD card, a real-world measurement); the agent stops and asks for it.

- [x] **0.1 Go primer [agent, you review]**
  - Write `docs/notes/go-primer.md`: the Go patterns this repo relies on (goroutines, buffered vs unbuffered channels, `context` cancellation, `errgroup`, worker pools), each with a short runnable example in `go/examples/`.
  - Done when: every example runs with `go run`, and the primer covers what's needed to review Phase 4 PRs.
- [x] **0.2 Toolchain setup script [agent]**
  - Write `scripts/setup-windows.ps1`: install or check Go, Git, Visual Studio 2022 Build Tools, CMake, vcpkg, ffmpeg and exiftool (using winget where possible), then print every version.
  - Input needed: run the script once with admin rights, and install the latest AMD Adrenalin driver.
  - Done when: the script reports every tool present.
- [x] **0.3 Repo skeleton and plan files [agent]**
  - Folders from Repository structure, `.gitignore` (library, models, build, `*.db`), `.github/AGENTS.md` from the template, and `docs/plan/` split from the Markdown export of the plan, with a "Read first" list at the top of each phase file. `protocol/README.md` gets the protocol section.
  - Go module in `go/` with an empty `rearview` command that prints its version.
  - Input needed: the Markdown export of the plan.
  - Done when: `go build ./...` passes, the tree matches the plan, and `docs/plan/README.md` links to every plan file.
- [x] **0.4 C++ hello world [agent]**
  - `engine/` with CMake presets, `vcpkg.json` listing OpenCV and nlohmann-json, and a `main.cpp` that opens an MP4 given as an argument and prints its frame count, fps and resolution.
  - Done when: it prints `2560x1440` and a sensible fps for a front-camera file.
- [ ] **0.5 CI [agent]**
  - GitHub Actions workflow that builds and tests Go on every PR. Add the C++ build once 0.4 is stable (cache vcpkg so it doesn't take 40 minutes each time).
  - Done when: a PR shows green checks.
- [ ] **0.6 Footage backup [agent]**
  - Write `scripts/backup-footage.ps1`: copy everything from the SD card into a raw folder outside the repo and verify checksums.
  - Input needed: the SD card plugged in, holding 1–2 weeks of drives (ideally some night, rain and highway driving, plus a few locked RO clips).
  - Done when: at least 5 hours of footage are backed up outside the card with verified checksums.
- [ ] **0.7 Labelling tool and first labels [agent, you review]**
  - Build `rearview label add --at <UTC time> --camera front --type near_miss [--note ...]`, which validates and appends to `testdata/labels/events.csv` (columns `ts_utc,camera,type,note`), plus `rearview label list`.
  - Input needed: event timestamps from watching the footage at 2× (hard brakes, close calls, cut-offs, tailgaters). Aim for 50–100 by the end of Phase 4 and keep adding.
  - Done when: the file has at least 20 validated rows.
- [ ] **0.8 Test clips [agent]**
  - Trim three 10–30 second clips (busy city, highway, night) with ffmpeg and commit them to `testdata/clips/` via Git LFS. These feed every automated test later.
  - Input needed: the file and start time of one good stretch of each kind.

**Phase 0 deliverable:** a repo with green CI, an engine binary that reads your footage, and a labelled dataset.
