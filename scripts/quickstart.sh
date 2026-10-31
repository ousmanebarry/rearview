#!/usr/bin/env bash
# Try Rearview in a few minutes on three 20-second sample clips: no dashcam, GPU or build needed.
#
# Downloads the models (once, about 615 MB) and the sample clips, imports the clips into a
# scratch library, runs detection, tracking, distance and time-to-collision on them, indexes
# them for plain-English search, runs a search, and writes a video with what the engine sees
# drawn on it. Everything goes into $DEMO (default ~/rearview-demo); delete it to start over.
#
# Works from the release folder (rearview and engine next to this script), from a repo
# checkout where both are built, or with them on PATH (the devcontainer). Needs ffmpeg,
# ffprobe, curl and unzip.
#
#   ./quickstart.sh                       # or: QUERY="a white truck" ./quickstart.sh
#
# Overrides: DEMO, QUERY, REARVIEW (CLI path), ENGINE (engine path), SAMPLES_URL.
set -euo pipefail

here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DEMO="${DEMO:-$HOME/rearview-demo}"
QUERY="${QUERY:-a traffic light}"
SAMPLES_URL="${SAMPLES_URL:-https://github.com/ousmanebarry/rearview/releases/latest/download/rearview-samples.zip}"

find_first() { # find_first <command> <candidate paths...>
  local cmd="$1"; shift
  for p in "$@"; do
    if [ -n "$p" ] && [ -f "$p" ] && [ -x "$p" ]; then echo "$p"; return; fi
  done
  command -v "$cmd" || true
}
step() { printf '\n\033[36m== %s\033[0m\n' "$1"; }
die() { echo "quickstart: $*" >&2; exit 1; }

REARVIEW="$(find_first rearview "${REARVIEW:-}" "$here/rearview" "$here/../go/bin/rearview")"
ENGINE="$(find_first engine "${ENGINE:-}" "$here/engine" "$here/../engine/build/ci-linux/engine")"
[ -n "$REARVIEW" ] || die "rearview not found: run this from the release folder, build go/ (see README) or set REARVIEW"
[ -n "$ENGINE" ] || die "engine not found: run this from the release folder, build engine/ (see README) or set ENGINE"
for tool in ffmpeg ffprobe curl unzip; do
  command -v "$tool" >/dev/null || die "$tool is not installed (Debian/Ubuntu: sudo apt-get install -y ffmpeg curl unzip)"
done
REARVIEW="$(realpath "$REARVIEW")"
ENGINE="$(realpath "$ENGINE")"
echo "rearview  $REARVIEW"
echo "engine    $ENGINE"
echo "demo      $DEMO"

mkdir -p "$DEMO"
cfg="$DEMO/config.yaml"
cat >"$cfg" <<EOF
# Written by quickstart.sh: a scratch library for the sample clips.
library_path: '$DEMO/library'
database_path: '$DEMO/rearview.db'
clips_path: '$DEMO/clips'
engine:
  path: '$ENGINE'
EOF

step "Models (downloaded once, checksums verified)"
"$REARVIEW" models download --config "$cfg"
models_dir="$("$REARVIEW" models --config "$cfg" | sed -n '1s/^models //p')"

step "Sample clips"
shopt -s nullglob
samples=("$here"/samples/*.mp4)
if [ ${#samples[@]} -eq 0 ]; then
  # A repo checkout has them in testdata/clips (Git LFS; small files are LFS pointers).
  for f in "$here"/../testdata/clips/*.mp4; do
    if [ "$(stat -c %s "$f")" -gt 1048576 ]; then samples+=("$f"); fi
  done
fi
[ ${#samples[@]} -gt 0 ] || samples=("$DEMO"/samples/*.mp4)
if [ ${#samples[@]} -eq 0 ]; then
  echo "downloading $SAMPLES_URL (about 200 MB)"
  curl -fL --progress-bar -o "$DEMO/samples.zip" "$SAMPLES_URL"
  unzip -oq "$DEMO/samples.zip" -d "$DEMO/samples"
  rm "$DEMO/samples.zip"
  samples=("$DEMO"/samples/*.mp4)
fi
# Give the clips the camera's file names (consecutive 20-second loops of one trip) so
# import treats them like recordings off the SD card. They carry no GPS, so speed-based
# events (hard brakes, following distance) stay empty; vision and search work.
card="$DEMO/card/DCIM/Movie"
mkdir -p "$card"
start=$(date -u -d '2026-10-23 15:11:53' +%s)
first=""
for i in "${!samples[@]}"; do
  name="$(date -u -d "@$((start + 20 * i))" +%Y_%m%d_%H%M%S)_$(printf '%06d' $((i + 1)))F.MP4"
  [ -e "$card/$name" ] || cp "${samples[$i]}" "$card/$name"
  [ -n "$first" ] || first="$card/$name"
  echo "$(basename "${samples[$i]}") -> $name"
done

step "Import (file names, GPS, trips)"
"$REARVIEW" import "$DEMO/card" --config "$cfg"

step "Detect, track, distance and time-to-collision (CPU)"
"$REARVIEW" process --plain --config "$cfg"

step "Index keyframes for search (CLIP)"
"$REARVIEW" process --embed --plain --config "$cfg"

step "Search: \"$QUERY\""
"$REARVIEW" search "$QUERY" --limit 3 --clip --config "$cfg"

step "What the engine sees (boxes, track ids, distance, TTC drawn on the video)"
"$ENGINE" detect "$first" --features detect,track,distance,ttc --models "$models_dir" --debug-video "$DEMO/boxes_raw.mp4"
# Re-encode to H.264 at 720p so any player (and a browser) can show it.
ffmpeg -y -loglevel error -i "$DEMO/boxes_raw.mp4" -vf scale=1280:-2 -c:v libx264 -pix_fmt yuv420p -crf 26 "$DEMO/boxes.mp4"
rm "$DEMO/boxes_raw.mp4"

printf '\n\033[32mDone. In %s:\033[0m\n' "$DEMO"
echo "  boxes.mp4   the first clip with every tracked object, its distance and time-to-collision"
echo "  clips/      15-second clips around the search results"
echo
echo "Try more:"
echo "  $REARVIEW search \"a white truck\" --config $cfg"
echo "  $REARVIEW trips --config $cfg"
echo "  $REARVIEW --help"
