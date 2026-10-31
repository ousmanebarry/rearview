# AGENTS.md — rearview

## What this repo is

Rearview: dashcam footage analyser. Go orchestrator (`go/`) + C++ vision engine (`engine/`) + optional Qt viewer (`viewer/`). No Python or TypeScript anywhere in the repo. How to build and run it: the root README.md.

## The plan

- docs/plan/README.md is the index. Read it first; docs/design.md is the design it implements.
- Tasks live in docs/plan/phases/. Do ONE task per PR, in order, unless told otherwise.
- Before starting, read the phase file's "Read first" list. Don't load other docs unless the task needs them.
- If a task has an "Input needed" line, stop and ask for that input first.
- When the task is done: tick its checkbox in the phase file, and record any decision or measured result in docs/plan/decisions.md, in the same PR.
- Tasks tagged [agent, you review] also need a plain-English walkthrough of the core logic, added to the phase's file in docs/walkthroughs/.
- If the plan is wrong or unclear, stop and say so in the PR description instead of guessing.

## Source of truth

- protocol/README.md defines every message between Go and the engine. Never change a message without updating the spec, the Go client and the C++ server in the same PR.
- go/internal/store/migrations/ defines the database (summarised in docs/data-model.md). Schema changes = a new numbered migration, never an edit to an old one.

## Build and test

- Go: `cd go && go build ./... && go vet ./... && go test ./...` (`-race` only in CI: it needs cgo).
- C++: `cd engine && cmake --preset dev && cmake --build --preset dev && ctest --preset dev` (`dev-dml` for the DirectML build; its tests run with `ctest --test-dir build/dev-dml -C RelWithDebInfo`).
- Viewer: `cd viewer && cmake --preset dev && cmake --build --preset dev`.
- Lint: gofmt and staticcheck (`go run honnef.co/go/tools/cmd/staticcheck@v0.8.2 ./...`) for Go, clang-format for C++.

## Rules

- Tests first. Every bug fix comes with a test that failed before the fix.
- Golden files live in testdata/. Regenerate them only with the documented command and explain why in the PR.
- Go: wrap errors with context (`fmt.Errorf("...: %w", err)`). Every goroutine must exit when its context is cancelled.
- C++: C++20, no raw new/delete, RAII for every resource. The engine writes protocol messages to stdout ONLY; logs go to stderr.
- Never commit footage, model files, the library folder or the database. They are gitignored.
- Keep PRs to one task from the plan. Put the task number in the PR title.
