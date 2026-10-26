# Tech stack and environment setup

Go handles everything that coordinates or stores; C++ handles everything that touches pixels or runs a model. Use the latest stable release of each tool unless a row says otherwise.

## Go (orchestrator)

| Package                                           | Used for                                                |
| ------------------------------------------------- | ------------------------------------------------------- |
| `github.com/spf13/cobra`                          | CLI commands and flags                                  |
| `github.com/charmbracelet/bubbletea` + `lipgloss` | Live progress view during `process`                     |
| `github.com/fsnotify/fsnotify`                    | Watching the import folder                              |
| `golang.org/x/sync/errgroup` + `context`          | Running pipeline stages and cancelling them together    |
| `modernc.org/sqlite`                              | SQLite in pure Go (no C compiler needed on the Go side) |
| `github.com/go-pdf/fpdf`                          | Incident report PDFs (Phase 8)                          |
| `os/exec`                                         | Running ffmpeg and the C++ engine                       |
| Standard `testing` + `go-cmp`                     | Unit and golden-file tests                              |

## C++ (engine)

| Library                                          | Used for                                                                                                 |
| ------------------------------------------------ | -------------------------------------------------------------------------------------------------------- |
| C++20, CMake with presets, vcpkg (manifest mode) | Build and dependencies                                                                                   |
| OpenCV                                           | Decoding (first version), resizing, drawing debug boxes, calibration, blur                               |
| ONNX Runtime                                     | Running every model: CPU first, then the RX 9070 XT through Windows ML (MIGraphX) or DirectML in Phase 6 |
| FFmpeg libraries (libavcodec, libavformat)       | Hardware-accelerated decoding on the RX 9070 XT (D3D11VA) in Phase 6                                     |
| nlohmann/json                                    | Reading and writing protocol messages                                                                    |
| Eigen                                            | Kalman filter math in the tracker                                                                        |
| GoogleTest                                       | Unit tests                                                                                               |
| Qt 6 (Multimedia, Location)                      | Optional desktop viewer (Phase 9)                                                                        |

## External tools

| Tool             | Used for                                             |
| ---------------- | ---------------------------------------------------- |
| ffmpeg / ffprobe | Cutting clips, reading video duration and frame rate |
| exiftool         | Test oracle only: its GPS output checks your parser  |
| Git + GitHub     | Repo, PRs, CI                                        |

## Models (download pre-exported ONNX files; no training)

| Model                               | Purpose                                            | Licence note                                                                                        |
| ----------------------------------- | -------------------------------------------------- | --------------------------------------------------------------------------------------------------- |
| YOLOX (s or m size)                 | Cars, trucks, buses, people, bikes, traffic lights | Apache-2.0, no restrictions. Ultralytics YOLO is AGPL-3.0, fine only if the repo stays open source. |
| CLIP ViT-B/32 image + text encoders | Natural-language search                            | MIT (OpenAI CLIP)                                                                                   |
| OpenCV YuNet face detector          | Privacy blur                                       | Check the model zoo licence                                                                         |
| A licence plate detector (ONNX)     | Privacy blur                                       | Check before choosing one                                                                           |

Models go in `engine/models/` (gitignored) and are fetched by a download script with checksums, so every machine gets the same files.

## Setup checklist (Windows 11)

- [ ] Install Go, Git, Visual Studio 2022 Build Tools ("Desktop development with C++"), CMake and vcpkg.
- [ ] Install ffmpeg and exiftool, and add both to PATH. Check with `ffmpeg -version` and `exiftool -ver`.
- [ ] AMD GPU: install the latest Adrenalin driver and check that Windows is version 24H2 or later (run winver), which Windows ML needs. No CUDA, cuDNN or ROCm install is needed on Windows.
- [ ] Clone the repo, run `go build ./...` in `go/`, and `cmake --preset dev` in `engine/` (vcpkg pulls the C++ libraries on first configure; expect 20–40 minutes for OpenCV).
- [ ] Run the model download script and check the checksums pass.

On Linux the same steps apply with gcc or clang instead of Visual Studio; everything in the stack is cross-platform.
