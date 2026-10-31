#!/usr/bin/env bash
# Runs once when the devcontainer / Codespace is created (see devcontainer.json):
# ffmpeg, the CLI built from this checkout, the prebuilt Linux engine from the latest
# release, and the models. REARVIEW_BUNDLE may name a local rearview-linux-x64.tar.gz.
set -euo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")/.."
repo="$PWD"

echo "== ffmpeg"
sudo apt-get update -qq
sudo DEBIAN_FRONTEND=noninteractive apt-get install -y -qq --no-install-recommends ffmpeg unzip >/dev/null

echo "== CLI (go/bin/rearview)"
(cd go && go build -o bin/rearview ./cmd/rearview)

echo "== engine (engine/build/release/engine)"
dest="$repo/engine/build/release"
bundle="${REARVIEW_BUNDLE:-https://github.com/ousmanebarry/rearview/releases/latest/download/rearview-linux-x64.tar.gz}"
engine_ok=1
if [ ! -x "$dest/engine" ]; then
  mkdir -p "$dest"
  extract() { tar -xz -C "$dest" --strip-components=1 --wildcards 'rearview/engine' 'rearview/libonnxruntime*'; }
  if [ -f "$bundle" ]; then
    extract <"$bundle" || engine_ok=0
  else
    curl -fsSL "$bundle" | extract || engine_ok=0
  fi
fi
if [ "$engine_ok" = 1 ] && "$dest/engine" info >/dev/null; then
  "$dest/engine" info | sed -n 1,5p
else
  rm -rf "$dest"
  cat <<'EOF'
Couldn't get the prebuilt engine (no release published yet?). Everything Go still works
(cd go && go test ./... runs the whole pipeline against a fake engine). To build the engine:
  git clone https://github.com/microsoft/vcpkg ~/vcpkg && ~/vcpkg/bootstrap-vcpkg.sh
  sudo apt-get install -y ninja-build nasm pkg-config autoconf autoconf-archive automake libtool
  cd engine && VCPKG_ROOT=~/vcpkg cmake --preset ci-linux && cmake --build --preset ci-linux   # ~30 min
EOF
fi

echo "== models ($REARVIEW_MODELS)"
go/bin/rearview models download

cat <<'EOF'

Ready. Try:
  bash scripts/quickstart.sh     sample clips -> detection, tracking, search, ~/rearview-demo/boxes.mp4
  cd go && go test ./...         the orchestrator against a fake engine (no models needed)
EOF
