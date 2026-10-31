Rearview: dashcam footage analyser
https://github.com/ousmanebarry/rearview

Try it on three 20-second sample clips (about 5 minutes on a laptop CPU; the first run
downloads the models, about 615 MB, and the clips, about 200 MB):

  Windows   powershell -ExecutionPolicy Bypass -File quickstart.ps1
  Linux     ./quickstart.sh

It imports the clips into ~/rearview-demo, runs detection, tracking, distance and
time-to-collision, indexes them for plain-English search, runs a search, and writes
~/rearview-demo/boxes.mp4 with what the engine sees drawn on the video.

Needs ffmpeg and ffprobe on PATH (Windows: winget install Gyan.FFmpeg; Ubuntu: sudo apt install ffmpeg).

On your own footage (VIOFO A229-style file names):

  rearview import E:\          copy recordings off the SD card, read GPS, build trips
  rearview process             analyse them (the models download on first run)
  rearview process --embed     index keyframes for search
  rearview events              what it found
  rearview search "a cyclist at night"

The Windows engine is the DirectML build: it runs on the CPU by default; add --device dml
(or engine.device: dml in ~/rearview/config.yaml) to use a DirectX 12 GPU.
Full documentation: the README at the link above.
