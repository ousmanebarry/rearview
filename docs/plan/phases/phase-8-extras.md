# Phase 8 (optional): Extras (about 1–2 weeks)

**Read first:** [../../../protocol/README.md](../../../protocol/README.md), [../data-model.md](../data-model.md), [../feature-specs.md](../feature-specs.md) (features J, K, L)

Only start this after Phase 7 is done and Phase 10 polish is at least drafted; each item stands alone, so pick the ones you like.

- [ ] **8.1 Privacy blur [agent, you review]**
  - Engine `blur` command: run the OpenCV YuNet face detector and a plate detector on every frame, track the boxes so the blur doesn't flicker, expand each box by 15%, apply a strong Gaussian blur, write the video (re-encoded).
  - `rearview export <event id> --blur` cuts the clip, sends it to the engine, saves the result.
  - Input needed: a frame-by-frame check of 5 exported test clips with plates and pedestrians.
  - Done when: nothing readable remains in those 5 clips.
- [x] **8.2 Driving score [agent, you review]**
  - Per-trip score out of 100: points lost per hard brake, swerve, near-miss and minute of close following, scaled per 100 km. Save to `trips.score`.
  - `rearview trips --score` shows the score and a breakdown of every point lost; a weekly trend line in the terminal.
  - Done when: the breakdown always adds up to the score.
- [x] **8.3 Incident report PDF [agent]**
  - `rearview report <event id>`: one-page PDF with 3 key frames (engine `extract_frames`), a speed and G-force graph for 30 seconds around the event, coordinates and nearest timestamp, event details and the clip file name.
  - Done when: the PDF is readable printed in black and white.
