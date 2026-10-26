# Testing and evaluation strategy

Two kinds of checking run side by side: tests that prove the code is correct, and evaluations that prove the results are good. Tests run on every PR; evaluations run when you change a model, threshold or the scheduler.

| Layer               | What it checks                                                          | Tools                                   | Where                                      |
| ------------------- | ----------------------------------------------------------------------- | --------------------------------------- | ------------------------------------------ |
| Go unit tests       | Parsers, detectors, queue order, fusion rules, scoring                  | `testing`, table-driven tests, `go-cmp` | Next to each package                       |
| Go race tests       | No data races in the pipeline                                           | `go test -race ./...`                   | CI, every PR                               |
| Golden files        | GPS parser output vs exiftool, tokenizer IDs, protocol messages         | `testdata/` + an `--update-golden` flag | `testdata/gps/`, `testdata/tokens/`        |
| Pipeline tests      | Worker pool, crash and hang recovery, cancel, resume                    | Fake engine (`go/cmd/fakeengine`)       | `internal/pipeline`                        |
| C++ unit tests      | Letterbox math, IoU, NMS, Kalman filter, tracker IDs, TTC, distance     | GoogleTest                              | `engine/tests/`                            |
| Protocol round-trip | Every command and reply survives Go → engine → Go                       | Real engine on a 10-second test clip    | Integration test, CI once C++ builds there |
| End-to-end smoke    | import → process → events on the 3 test clips finishes with sane output | CLI run in CI                           | `.github/workflows/ci.yml`                 |
| Accuracy evaluation | Precision and recall per event type against your labels                 | `rearview eval`                         | Run by hand; results in the decisions log  |
| Search evaluation   | Correct moment in top 5                                                 | `rearview eval --search`                | Run by hand                                |
| Benchmarks          | Minutes per hour of footage, per-stage fps                              | `rearview bench`                        | Run by hand; results in `bench/results/`   |

## Rules that keep the numbers honest

- Keep a held-out set: label at least one full day of driving that you never look at while tuning, and report final numbers on it.
- A match counts only within 3 seconds and with the same event type.
- Re-run `eval` after every performance change; a faster pipeline that misses more events is not an improvement.
- Report dataset size next to every number (hours of footage, number of labelled events).
