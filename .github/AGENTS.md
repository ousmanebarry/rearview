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

- Go: cd go && go build ./... && go test ./... && go vet ./...
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
