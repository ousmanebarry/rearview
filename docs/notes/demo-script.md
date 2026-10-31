# Demo script (10.2)

Target: 60–90 s, 1080p screen recording of a terminal (and the viewer, if built), then cut with ffmpeg. Event ids below are from the current database (`rearview events`); check them before recording.

## Before recording

- Import: the full card takes 15 min (copy-bound), so record the import of a small card copy (2–3 recordings) with `--config demo.yaml` pointing at a scratch library, and use the real library for everything else.
- Terminal: 120×32, large font, dark theme.
- Close anything showing other people's data. Our plate and GPS are burned into the camera overlay: blur it in the final cut (step 4 below) or keep the video private.

## Shot list

| # | Time | Shot | Command / action | Caption |
| --- | --- | --- | --- | --- |
| 1 | 0–8 s | Title card over a frame of the highway clip | — | Rearview: what happened on my drives? |
| 2 | 8–18 s | Import | `rearview import E:\` | Copies new recordings and reads the GPS hidden in each MP4 |
| 3 | 18–30 s | Process with the progress view | `rearview process --only-windows` | Go schedules C++ engine workers on the GPU |
| 4 | 30–40 s | Events found | `rearview events` | Hard brakes from GPS, tailgating and following distance from vision |
| 5 | 40–52 s | Clip with boxes and TTC | `rearview clip 19` then play the `--debug-video` render of the same window (or the viewer: trip 12, event 14:07:04) | Tracked cars, distance and time-to-collision |
| 6 | 52–60 s | Rear camera tailgater | `rearview export 18 --blur --camera rear`, play the result | Plates and faces blurred on export |
| 7 | 60–72 s | Search | `rearview search "a construction crane"` then play the top hit (`--clip`) | Plain-English search over 19,266 keyframes with CLIP |
| 8 | 72–80 s | Report + score | `rearview report 19`, open the PDF; `rearview trips --score` | One-page incident report, driving score |
| 9 | 80–88 s | Numbers card | — | 1 h of 1440p footage in 2.4 min · hard brakes P/R 1.00 on 9 labels · search 21% top-5 |

Debug render for shot 5: the engine draws boxes, track ids, distance and TTC with
`engine detect <file> --from <s> --to <s+15> --device dml --features detect,track,distance,ttc --debug-video out.mp4` (without `--features` only boxes and track ids are drawn). Checked on event 19 (`--from 148 --to 160`): the car ahead reads `#4 car 12.6m ttc 7.4s` at 59 km/h, with the path trapezoid drawn; 120 frames in about 4 s. The file and offset of event 19:
`sqlite3 ~/rearview/rearview.db "SELECT s.path, e.t_start_s FROM events e JOIN segments s ON s.id = e.segment_id WHERE e.id = 19"` (path is inside the library).

## Cutting (ffmpeg)

1. Trim each shot from the raw recording (`raw.mkv`, timestamps from the recording):

   ```sh
   ffmpeg -ss 00:00:12 -to 00:00:22 -i raw.mkv -c:v libx264 -crf 20 -an s2.mp4
   ```

2. Captions, bottom-centre, one per shot:

   ```sh
   ffmpeg -i s2.mp4 -vf "drawtext=text='Copies new recordings and reads the GPS hidden in each MP4':fontcolor=white:fontsize=36:box=1:boxcolor=black@0.6:boxborderw=12:x=(w-text_w)/2:y=h-90" -c:v libx264 -crf 20 s2c.mp4
   ```

3. Join: list the files in `shots.txt` (`file 's1c.mp4'` …) and `ffmpeg -f concat -safe 0 -i shots.txt -c copy demo.mp4`.
4. Blur other people's plates in footage shots: use the `export --blur` output for every dashcam clip shown, never the raw file. For the camera's own overlay strip (our plate and position), add `-vf "boxblur=20:enable='between(t,0,999)'"` on a crop of the bottom 6% (`[0:v]crop=iw:ih*0.06:0:ih*0.94,boxblur=20[b];[0:v][b]overlay=0:H*0.94`).
5. GIF for the README (first 10 s of shot 5):

   ```sh
   ffmpeg -i s5c.mp4 -t 10 -vf "fps=12,scale=800:-1:flags=lanczos,split[a][b];[a]palettegen[p];[b][p]paletteuse" demo.gif
   ```

Input needed: the screen recording itself.
