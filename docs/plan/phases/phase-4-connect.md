# Phase 4: Connect Go and C++ (weeks 6–7)

**Read first:** [../../../protocol/README.md](../../../protocol/README.md), [data-model.md](../../data-model.md), [design: architecture](../../design.md#architecture), [design: testing](../../design.md#testing-and-evaluation)

**Walkthrough:** [phase-4-pipeline.md](../../walkthroughs/phase-4-pipeline.md)

Goal: `rearview process` runs the real engine over your library in parallel, survives the engine being killed mid-run, and resumes where it stopped after a restart.

- [x] **4.1 Engine server loop [agent, you review]**
  - `engine serve`: load models, send `ready`, then read one JSON command per line from stdin and stream replies on stdout. Implement `ping`, `analyze`, `cancel`, `shutdown` from the protocol spec.
  - Flush stdout after every message. Read `cancel` on a separate thread so a long job can be stopped.
  - Done when: a GoogleTest feeds commands through a fake stdin and checks every reply type, including `error` for a missing file and `unknown_command`.
- [x] **4.2 Fake engine [agent]**
  - `go/cmd/fakeengine`: speaks the same protocol with canned replies. Flags make it slow, return errors, go silent, or crash after N jobs.
  - Done when: Go tests can run the whole pipeline without the C++ engine built.
- [x] **4.3 Engine client [agent, you review]**
  - `internal/engine`: start the process with `os/exec`, wait for `ready`, check `protocol_version`, forward stderr to the log.
  - `Run(ctx, job) (<-chan Reply, error)`: send the command, stream replies until `done` or `error`. Context cancel → send `cancel`, then kill the process if it doesn't finish within 2 seconds.
  - Detect a dead process (stdout closed, exit) and a hung one (no message within the job timeout). Restart, then retry the job once.
  - Done when: tests with the fake engine cover normal runs, errors, crash-and-retry, hang-and-retry, and cancel.
- [x] **4.4 Job planner and priority queue [agent, you review]**
  - `internal/queue`: jobs come from the `jobs` table. Priority 0 = locked RO files, 1 = motion-event windows, 2 = everything else (skip parking footage).
  - Use the two-channel select pattern (high, low) or a heap guarded by a mutex; workers always drain priority 0 first.
  - Done when: a test enqueues mixed jobs and checks they come out in priority order.
- [x] **4.5 Pipeline wiring [agent, you review]**
  - `internal/pipeline`: errgroup with planner → bounded jobs channel → N workers (each owns one engine process) → bounded results channel → a single writer goroutine that saves `observations` and job state.
  - Close the results channel only after all workers finish (WaitGroup). One writer goroutine avoids SQLite write contention.
  - Done when: a test with the fake engine processes 100 jobs with 4 workers, and `go test -race` passes.
- [x] **4.6 Resume after crash [agent, you review]**
  - Job states are saved as they change. At startup, any job left `running` goes back to `pending` (its partial observations are deleted first).
  - Done when: killing `rearview process` mid-run and starting it again finishes the remaining jobs without duplicating rows.
- [x] **4.7 Process command + progress view [agent]**
  - `rearview process [--workers 2] [--only-windows] [--device cpu|dml|migraphx]` with a bubbletea screen: jobs done/total, frames per second per worker, queue depth, failures, ETA.
  - Plain log output when not running in a terminal (for CI).
  - Done when: it runs on a full day of footage and shows sensible numbers.

**Phase 4 deliverable:** a robust parallel pipeline. Demo: kill an engine process with Task Manager mid-run and watch it recover.
