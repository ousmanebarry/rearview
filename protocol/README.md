# Go ↔ engine protocol

Go starts the engine as a long-running child process and they talk in newline-delimited JSON: one message per line on stdin (Go → engine) and stdout (engine → Go). This file is the source of truth: never change a message without updating this spec, the Go client and the C++ server in the same PR.

## Framing rules

- UTF-8, one JSON object per line, `\n` terminated. No pretty-printing.
- stdout carries protocol messages only. All logging goes to stderr; Go forwards it to its own log with the worker id.
- Every command has an integer `id`, unique per engine process. Every reply carries the `id` it answers.
- One job at a time per engine process. Parallelism = several engine processes, one per worker.
- Unknown fields are ignored (forward compatibility). Unknown commands get an `error` reply with code `unknown_command`.

## Lifecycle

1. Go starts `engine --models <dir> --device <cpu|dml|migraphx>`.
2. Engine loads its models, then sends `ready` with its protocol version and capabilities.
3. Go checks the version. A mismatch kills the process and fails the run with a clear message.
4. Go sends one command, reads replies until `done` or `error` for that `id`, then sends the next.
5. Go sends `cancel` to stop a job early; the engine replies `done` with `"cancelled": true`.
6. Go sends `shutdown` on exit; the engine flushes and exits with code 0 within 2 seconds, or Go kills it.
7. If the process dies or stays silent past the job timeout, Go restarts it and retries the job once.

## Commands (Go → engine)

| Command          | Phase | Purpose                                              | Key fields                                     |
| ---------------- | ----- | ---------------------------------------------------- | ---------------------------------------------- |
| `ping`           | 4     | Health check                                         | none                                           |
| `analyze`        | 4     | Detect and track objects in time windows of one file | `path`, `camera`, `windows`, `fps`, `features` |
| `extract_frames` | 8     | Save specific frames as JPEGs                        | `path`, `times`, `out_dir`                     |
| `embed_frames`   | 7     | CLIP vectors for keyframes                           | `path`, `every_sec`                            |
| `embed_text`     | 7     | CLIP vector for a search query                       | `text`                                         |
| `blur`           | 8     | Write a copy of a clip with faces and plates blurred | `path`, `out_path`                             |
| `cancel`         | 4     | Stop the job with this id                            | `target_id`                                    |
| `shutdown`       | 4     | Exit cleanly                                         | none                                           |

## Replies (engine → Go)

| Type        | When                            | Key fields                                                     |
| ----------- | ------------------------------- | -------------------------------------------------------------- |
| `ready`     | Once, after models load         | `protocol_version`, `engine_version`, `device`, `capabilities` |
| `pong`      | Answer to `ping`                | none                                                           |
| `progress`  | Every ~1 second during a job    | `t` (seconds into the file), `frames_done`                     |
| `frame`     | Per analyzed frame with objects | `t`, `objects[]`                                               |
| `embedding` | Per keyframe, or once for text  | `t` (frames only), `vector` (512 floats)                       |
| `done`      | Job finished                    | `frames`, `fps`, `elapsed_ms`, `cancelled`, `stages_ms`        |
| `error`     | Job failed                      | `code`, `msg`, `t` if known                                    |

## Examples

```json
{"type":"ready","protocol_version":1,"engine_version":"0.3.0","device":"migraphx","capabilities":["analyze","embed_frames","embed_text"]}
{"id":41,"cmd":"analyze","path":"C:/rearview/library/2026/10/24/front_0042.mp4","camera":"front","windows":[[12.0,30.0]],"fps":10,"features":["detect","track","distance","ttc"]}
{"id":41,"type":"frame","t":12.4,"objects":[{"track":7,"cls":"car","box":[812,402,1105,640],"conf":0.91,"dist_m":9.8,"ttc_s":1.3}]}
{"id":41,"type":"progress","t":20.1,"frames_done":77}
{"id":41,"type":"done","frames":180,"fps":143.2,"elapsed_ms":1257,"cancelled":false,"stages_ms":{"decode":310.2,"preprocess":250.1,"inference":610.4,"postprocess":6.0,"tracking":4.1,"geometry":1.0}}
{"id":42,"type":"error","code":"decode_failed","msg":"corrupt packet","t":14.2}
{"id":43,"cmd":"ping"}
{"id":43,"type":"pong"}
```

## Field details

- `windows`: list of `[start, end]` in seconds from the start of the file. Empty list = whole file.
- `analyze` also accepts `debug_video` (write an MP4 with the boxes drawn) and `decoder` (`opencv` or `ffmpeg`); both optional.
- `cancel` gets no reply of its own: the cancelled job ends with `done` and `"cancelled": true`. A `cancel` for a job that hasn't started yet makes it end at once the same way.
- `stages_ms`: milliseconds spent per engine stage over the whole job (`decode`, `preprocess`, `inference`, `postprocess`, `tracking`, `geometry`); `rearview bench` sums them.
- Commands that aren't valid JSON or lack an integer `id` and a `cmd` get an `error` with code `bad_request` (and the `id` when there is one).
- `box`: `[x1, y1, x2, y2]` in pixels of the original 2560×1440 frame, not the model's input size.
- `cls`: one of `car`, `truck`, `bus`, `motorcycle`, `bicycle`, `person`, `traffic_light`. Everything else is dropped in the engine.
- `dist_m` and `ttc_s`: present only when those features were requested and the value is valid; `ttc_s` is omitted when the object is not approaching.

## Error codes

`unknown_command`, `bad_request`, `file_not_found`, `decode_failed`, `model_error`, `out_of_memory`, `internal`. Go retries only `decode_failed`, `out_of_memory` and process crashes; the rest fail the job immediately.

## Versioning

Bump `protocol_version` for any breaking change. Go refuses to run against a different version. Adding a command is not a breaking change.
