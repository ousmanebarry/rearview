# Rearview: notes from building a dashcam analyser in Go and C++

_Draft written from [plan/decisions.md](plan/decisions.md) and [notes/](notes/). Rewrite it in your own voice before publishing._

I have a two-channel dashcam (VIOFO A229 Plus, 2560×1440 front and rear) and 5.35 hours of footage from four days of ordinary driving. Rearview imports it, finds the moments worth looking at (hard brakes, following too close, being tailgated, near-misses), cuts clips and one-page reports, and lets me search the video in plain English. A Go program does the coordination; a C++ engine does the pixels; they talk over JSON lines on stdin/stdout.

Three problems took most of the time.

## 1. The GPS hidden in the MP4, and what it gets wrong

The camera doesn't write a GPX file. Its GPS lives inside each MP4: `moov/gps ` holds an index of (offset, size) pairs pointing to 16 KB `free` boxes, one per second, each with a 56-byte record. I worked out the layout by decoding the first fix of a file recorded while parked and comparing against `exiftool -ee` until the last digit matched: six little-endian uint32 for the UTC time, a status byte, hemisphere bytes, then four float32s. Latitude is NMEA `ddmm.mmmm`, so `4517.7832` is 45° 17.7832′; speed is in knots.

Getting the decoder right was a day. Learning what the receiver does wrong took much longer, and every glitch looked like a driving event:

- **Corrupt dates.** Occasionally a block says year 41, month 2, day 0, with a perfectly good position. Dropping those leaves holes; trusting them breaks trip assembly. The decoder now repairs the time from the nearest good block (blocks are exactly one second apart).
- **Dropouts.** At 130 km/h on the highway the receiver sometimes reports one or two records of 0 km/h and heading exactly 0.00°, then carries on at 130. To a "speed fell by 14 km/h in one second" rule that's a 130 km/h emergency stop: the first sweep found 34 hard brakes, of which 2 were real. Removing runs of 0° records that come back to speed within 5 s cut 30 false alarms, and only then could the threshold be tuned honestly (14 → 10 km/h, which caught real 10–13 km/h drops).
- **The ones I missed at first.** Late in the project a hard brake showed up at latitude 0, longitude 0. There were 72 records flagged valid with every field zero. Looking closer, a few more 0 km/h / 0° records at low speed had survived the dropout filter, because "a slow run that stays slow" looked like a real stop. The tell was that a stopped car keeps its last heading (I found 0.06 km/h at 260°), so 0/0 is never a real reading. Three of my GPS-suggested labels had been drafted from exactly these records, which means I'd been scoring the detector against its own bug. They're gone, and the labelled set shrank from 12 brakes to 9.

What I'd tell someone else: **plot the raw signal before you tune anything**, and be suspicious of labels that came from the same signal you're evaluating.

## 2. Getting time-to-collision to stop crying wolf

The near-miss signal is time-to-collision from box growth: if an object at distance _d_ closes at speed _v_, its image height _h_ is proportional to 1/_d_, so (dh/dt)/h = v/d = 1/TTC. That needs no camera calibration and no assumed car height, which matters because my calibration is still a 100° pinhole guess.

The formula is easy. Most of the work was removing the reasons it fires on nothing:

- **Our own hood was a car.** YOLOX reported the bonnet as a car in every front frame, centred, enormous and "approaching" whenever the exposure changed. Boxes wider than half the frame centred in the bottom fifth are now dropped.
- **One car, two tracks.** A pickup flickering between `car` and `truck` gave two tracks, each with a fresh, jittery velocity, and a class-dependent distance that doubled when the class flipped (31 m with a 3 m truck height). Car/truck/bus boxes overlapping by IoU > 0.7 are merged across classes, and a "truck" is 2 m tall (here it's mostly pickups and vans).
- **Small boxes are noise.** At 20 px tall, one pixel of jitter is a 5% height change, which reads as TTC around 2 s. TTC is only computed from the Kalman-smoothed height and height velocity, and only for boxes at least 40 px tall.
- **Things that aren't in the way.** A car in the next lane can close fast without being a threat. TTC is only reported when the box's bottom-centre is inside a trapezoid ahead of the car.
- **One bad frame.** A near-miss needs TTC under 1.5 s for 3 consecutive analysed frames. If a hard brake happens within 2 s, the score goes up: the driver's reaction is the best evidence I have.

Result: over every analysed event window, no near-miss fired; stopping behind a car reads 15–17 m with TTC 7–28 s as the gap closes. But I have **no near-miss labels**, so I can't claim precision or recall, and "it didn't fire" is not the same as "it would have fired". This is the biggest gap in the project.

## 3. The performance journey: 28 → 2.4 minutes per hour

The target was to analyse an hour of 1440p footage in under 10 minutes. `rearview bench` runs the real pipeline on a fixed 62-minute set and records minutes per hour of footage, per-stage times from the engine, worker utilisation, GPU utilisation and peak memory, so every change was measured the same way. Two runs on the same commit agree within 1.7%.

| Step                                    | Minutes per hour |
| --------------------------------------- | ---------------- |
| CPU, 2 workers × 4 ONNX Runtime threads | 28.0             |
| DirectML on the RX 9070 XT              | 2.9              |
| 2 engine workers instead of 1           | 2.5              |
| Final, full set                         | **2.4**          |

DirectML took inference from 53 ms to 3 ms per frame, with bit-identical detections on the test clip. After that, inference wasn't the problem, and the next three ideas all failed for the same reason:

- **Hardware decoding (D3D11VA)** made it 2.6× _slower_. OpenCV's software decoder spreads frame threads over 16 hardware threads and skips colour conversion for frames we don't sample; the GPU path decodes one frame at a time and copies every sampled 1440p frame back to the CPU. It also produced washed-out colours until I noticed the camera writes full-range YUV and the GPU frames lose that flag. Kept behind a flag, off by default.
- **Dropping to 5 fps on empty road** saved nothing (2.47 vs 2.46): skipping inference doesn't skip decoding, and H.264 must decode every frame.
- **Batched inference** wasn't possible: the released YOLOX-s export has a fixed batch of 1, and re-exporting needs Python.

What did help was a three-stage thread pipeline inside the engine (decode → letterbox → inference, with bounded queues), which cut 19% and is tested to produce byte-identical output to the serial loop. Today a frame costs 8 ms of decode, 6 ms of resizing 2560×1440 to 640×640 and 3–6 ms of inference. The next win is resizing on the GPU or decoding at reduced resolution, not a faster model.

## Things that worked better than expected

- **A child process instead of cgo.** The engine can crash, hang or be killed, and the Go side just restarts it and retries the job once. Tests run against a fake engine written in Go, so the whole pipeline is tested on machines without a GPU.
- **One SQLite writer.** Workers send results to a single goroutine that owns the write connection; no lock contention, and a killed run resumes from the job table.

## What's still missing

- Near-miss, following-distance and tailgating labels, and a held-out-day evaluation for hard brakes.
- A real lens calibration and a tape-measure test, so distances are more than indicative.
- Search is weak (correct moment in the top 5 for 21% of 24 drafted queries), partly because the test counts one labelled occurrence of things like "white pickup truck" that appear dozens of times.
