# Go primer for reviewing Rearview

The Go patterns this repo relies on, each with a runnable example in [go/examples/](../../go/examples/). Run any of them from `go/`:

```
go run ./examples/<name>
go test ./examples/...
```

| Example                                            | Pattern                                                          | Used in                 |
| -------------------------------------------------- | ---------------------------------------------------------------- | ----------------------- |
| [goroutines](../../go/examples/goroutines/main.go) | Goroutines + `sync.WaitGroup`                                    | Everywhere              |
| [channels](../../go/examples/channels/main.go)     | Unbuffered vs buffered, `close`, `range`, `select` with timeout  | Pipeline, engine client |
| [context](../../go/examples/context/main.go)       | Cancellation and timeouts with `context`                         | Every long-running call |
| [errgroup](../../go/examples/errgroup/main.go)     | `errgroup.WithContext`, first error cancels the rest, `SetLimit` | Import, pipeline stages |
| [workerpool](../../go/examples/workerpool/main.go) | Producer → bounded jobs → N workers → results → single writer    | Task 4.5                |
| [priority](../../go/examples/priority/main.go)     | Two-channel priority select                                      | Task 4.4                |
| [subprocess](../../go/examples/subprocess/main.go) | Child process over stdin/stdout lines, timeout, kill, reap       | Task 4.3                |

## 1. Goroutines

`go f()` runs `f` concurrently and returns immediately. Nothing waits for it unless you make something wait: when `main` returns, every goroutine dies mid-flight. `sync.WaitGroup` is the usual way to wait; `wg.Go(f)` (Go 1.25+) does the `Add(1)` / `defer Done()` bookkeeping for you.

Two goroutines writing the same variable is a data race. The example avoids a mutex by giving each goroutine its own slice slot. `go test -race` detects races at runtime; it needs cgo, so it runs in CI (Linux) rather than on a Windows machine without a C compiler for Go.

## 2. Channels

- **Unbuffered** (`make(chan T)`): a send waits until someone receives. It's a hand-off, and it synchronizes the two goroutines.
- **Buffered** (`make(chan T, n)`): sends succeed until `n` values are queued, then block. A small buffer between stages is a **bounded queue**: a fast producer gets slowed down (back-pressure) instead of filling memory.
- **Closing:** only the sender closes, and only once. `for v := range ch` ends when the channel is closed and drained. Receiving from a closed channel returns the zero value with `ok == false`. Sending on a closed channel panics.
- **`select`:** waits on several channel operations and runs whichever is ready; if several are ready, it picks **randomly**. Add `case <-time.After(d)` for a timeout, or `default` to not block at all.
- **nil channels** block forever. Setting a channel variable to `nil` inside a loop removes that case from a `select` (used in the priority example).

## 3. Context

A `context.Context` carries a cancellation signal down the call chain. `WithCancel` gives you a `cancel()` function; `WithTimeout` cancels on its own after a duration. Always `defer cancel()` to release its resources.

A function that loops or waits must watch `ctx.Done()` in its `select` and return `ctx.Err()` (`context.Canceled` or `context.DeadlineExceeded`). Check which with `errors.Is`, because errors get wrapped with `%w`. Convention: `ctx` is the first parameter, never stored in a struct.

## 4. errgroup

`g, ctx := errgroup.WithContext(parent)` runs a set of goroutines as one unit:

- `g.Go(func() error {...})` starts one.
- The first non-nil error cancels `ctx`, so the other goroutines (which watch `ctx`) stop early.
- `g.Wait()` waits for **all** of them, then returns that first error.
- `g.SetLimit(n)` caps how many run at once; `g.Go` blocks while `n` are running.

The returned `ctx` shadows the parent on purpose: goroutines in the group must use the group's context, or they won't see the cancel.

## 5. Worker pool (the Phase 4 pipeline)

```
producer --jobs (bounded)--> N workers --results (bounded)--> single writer --> SQLite
```

The rules that keep it correct:

1. **The producer closes `jobs`** when it's done, since it is the only sender. Workers `range` over `jobs` and exit when it's closed.
2. **Nobody closes `results` until every worker has finished.** There are N senders, so a separate goroutine does `workers.Wait(); close(results)`. Closing earlier panics a worker that is still sending.
3. **One writer goroutine** owns the database. SQLite allows one writer at a time; a single goroutine avoids lock contention and makes ordering easy to reason about.
4. **Every blocking send also watches `ctx.Done()`.** Otherwise a worker blocked on a full `results` channel never notices cancellation and leaks.

`TestRunStopsWhenCancelled` shows the whole pipeline returning `context.Canceled` without hanging.

## 6. Priority select (task 4.4)

A single `select` over a high and a low channel picks randomly when both are ready, so it gives no priority. The fix: first try the high channel with a non-blocking `select` (`default` case), and only if it is empty fall through to a blocking `select` over both. When a channel closes, set it to `nil` so it drops out. The alternative is a heap guarded by a mutex; both are acceptable for 4.4.

## 7. Subprocess (task 4.3)

The engine client starts the C++ engine with `os/exec` and talks newline-delimited messages over its stdin/stdout. The example re-runs its own binary as a toy child so it needs no C++ build.

- `exec.CommandContext(ctx, ...)` kills the child when `ctx` is cancelled. `cmd.WaitDelay` bounds how long `Wait` then waits for the pipes to close.
- **One goroutine owns stdout**, scanning lines into a channel and closing it on EOF. The caller `select`s on that channel, a timer, and `ctx.Done()`, so it can notice three things: a reply, a hang (timer fires), and a crash (channel closed).
- After `Kill`, always call `cmd.Wait()` to reap the process and release its handles, and drain the lines channel so the reader goroutine can exit.
- In the real engine, stderr carries logs and stdout carries only protocol messages.

## Checklist for reviewing Phase 4 PRs

- [ ] Every goroutine has a clear exit: a closed channel it ranges over, or `ctx.Done()`.
- [ ] Every blocking send or receive is inside a `select` that also watches `ctx.Done()` (or the channel is guaranteed to be drained).
- [ ] Each channel has exactly one closer, and it's the sender side. Multi-sender channels are closed after a `WaitGroup`.
- [ ] Buffered channels are bounded with a small, deliberate size, never "big enough".
- [ ] Errors are wrapped with `%w` and checked with `errors.Is` / `errors.As`.
- [ ] Every started process is waited on (`cmd.Wait`), even after `Kill`.
- [ ] Only one goroutine writes to SQLite.
- [ ] Tests cover cancellation, not only the happy path, and pass under `go test -race` in CI.
