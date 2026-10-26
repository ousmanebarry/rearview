# Feature specs

Fifteen features, each with a clear owner: Go coordinates, stores and decides; C++ looks at pixels and runs models. "Done when" is the acceptance test for the feature as a whole.

| #   | Feature                    | Go does                                                                              | C++ does                                                        | Phase | Done when                                                                           |
| --- | -------------------------- | ------------------------------------------------------------------------------------ | --------------------------------------------------------------- | ----- | ----------------------------------------------------------------------------------- |
| A   | Footage library and import | Watch folder, stable-size check, copy, hash, tag camera and locked, insert rows      | —                                                               | 1     | Re-running import on the same card adds 0 rows                                      |
| B   | GPS extraction             | Walk MP4 boxes, decode freeGPS blocks, convert knots to km/h                         | —                                                               | 1     | Matches exiftool on every test file within 1e-5° and 0.1 km/h                       |
| C   | Trips                      | Split on gaps, pair front/rear, stitch GPS, fix clock, compute summaries, export GPX | —                                                               | 1     | A week of drives lists as the trips you actually took; GPX lines up with real roads |
| D   | Motion events              | Hard braking, swerves, G-spikes from GPS; turn them into priority windows            | —                                                               | 2     | Finds at least 80% of your labelled hard brakes                                     |
| E   | Detection and tracking     | Choose files, windows, frame rate, features                                          | Decode, preprocess, YOLOX via ONNX Runtime, NMS, Kalman tracker | 3     | Debug video shows stable IDs on cars and people through a 30-second clip            |
| F   | Near-miss (TTC)            | Fuse TTC with braking and speed, score, merge                                        | Box-growth TTC per tracked object in your path, smoothed        | 5     | Measured precision and recall on labelled near-misses                               |
| G   | Following distance         | Following time = distance ÷ GPS speed; flag sustained close following                | Distance from box height and camera calibration                 | 5     | Distance within 20% of a measured car-length test                                   |
| H   | Clip extraction            | ffmpeg stream copy, join across segments, side-by-side layout                        | —                                                               | 2     | Any event becomes a playable 10–20 s clip in under 2 s (stream copy)                |
| I   | Natural-language search    | Store vectors, rank by cosine similarity, group hits into moments                    | CLIP image and text encoders, BPE tokenizer                     | 7     | Right moment in top 5 for at least 70% of test queries                              |
| J   | Privacy blur               | `export --blur` command                                                              | Face and plate detection, tracked blur, write video             | 8     | No readable plate or face in 5 exported test clips                                  |
| K   | Driving score              | Per-trip score from events, following time, smoothness                               | —                                                               | 8     | Score breakdown explains every point lost                                           |
| L   | Incident report PDF        | Layout, speed and G-force graph, map location, timeline                              | Exact frame extraction                                          | 8     | One command produces a readable PDF for any event                                   |
| M   | CLI and progress view      | cobra commands, bubbletea progress screen                                            | —                                                               | 1–4   | Every command has `--help` and a test                                               |
| N   | Performance and benchmarks | Scheduling (skip parking, lower fps on empty roads), `bench` command                 | GPU inference, hardware decode, threaded pipeline, batching     | 6     | `bench` reports minutes per hour of footage, with history                           |
| O   | Desktop viewer (optional)  | —                                                                                    | Qt app: video, event timeline, map, search                      | 9     | Click an event, the video jumps there, the map shows where                          |

## The core math (Phase 5)

Time-to-collision from how fast a tracked car's box grows. With box height $h$ in pixels and its growth rate $dh/dt$ (from the tracker's smoothed estimate):

$$
\mathrm{TTC} \approx \frac{h}{\,dh/dt\,}
$$

Distance from box height, using the calibrated focal length $f$ in pixels and an average real vehicle height $H$ (about 1.5 m for cars; use a per-class value):

$$
d \approx \frac{f \cdot H}{h}
$$

Following time: C++ supplies $d$, Go supplies your speed $v$ from GPS (in m/s):

$$
t_{\text{follow}} = \frac{d}{v}
$$

## Starting thresholds

All in the config file; tune them against your labels in Phase 5.

| Event           | Starting rule                                                                               |
| --------------- | ------------------------------------------------------------------------------------------- |
| Hard brake      | Speed drops by 14 km/h or more within 1 second (about 0.4 g)                                |
| Swerve          | Heading changes 30° or more within 1 second, above 30 km/h                                  |
| Near-miss       | TTC under 1.5 s for 3 or more consecutive analyzed frames, object in the centre path region |
| Following close | Following time under 1.0 s for 3 s or more, above 50 km/h                                   |
| Tailgated       | Same rule on the rear camera                                                                |
