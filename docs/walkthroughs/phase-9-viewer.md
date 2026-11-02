# Phase 9: desktop viewer

`viewer/` is a Qt 6 Widgets app (C++20, its own vcpkg manifest and CMake presets, separate from the engine so the engine build doesn't pull in Qt).

```powershell
cd viewer; cmake --preset dev; cmake --build --preset dev
rearview view    # or: build/dev/RelWithDebInfo/viewer.exe [--db ...] [--library ...] [--rearview path/to/rearview.exe]
```

`rearview view` ([view.go](../../go/cmd/rearview/view.go)) starts the viewer with `--db`, `--library` and `--rearview` taken from the config and its own path. It uses `viewer_path` from the config, else `viewer.exe` next to `rearview.exe`, else the repo build (from `go/bin/rearview.exe`), else `viewer` on `PATH`. The release bundles don't contain the viewer.

The first configure builds qtbase (about 13 minutes with 14 jobs) and qtmultimedia (2 minutes); later configures restore them from the vcpkg binary cache.

## Pieces

- **Database** ([db.cpp](../../viewer/src/db.cpp)): `QSqlDatabase` on the same SQLite file, opened with `QSQLITE_OPEN_READONLY`; the viewer never writes, so it can stay open while `rearview process` runs (WAL mode). Queries: trips, a trip's segments per camera, events (with their trip and camera), the front GPS track of a trip, and observations of one segment in a time window.
- **Video + timeline** ([main_window.cpp](../../viewer/src/main_window.cpp), [timeline.cpp](../../viewer/src/timeline.cpp)): the trip's segments of one camera play back to back (`QMediaPlayer`, next file on `EndOfMedia`). Any absolute time maps to (segment, offset) through the segments' start times, which is how timeline clicks, event clicks, map pins, search results and the front/rear switch all seek. Seeks wait for `LoadedMedia` when a new file is loading. Clicking an event opens its trip on its camera (rear for tailgating) 5 s before it. Audio is muted (cabin speech).
- **Map** ([map_widget.cpp](../../viewer/src/map_widget.cpp)): a small slippy map drawing OpenStreetMap tiles fetched with `QNetworkAccessManager` over HTTPS (Schannel), with a User-Agent as the OSM tile policy requires and the attribution in the corner; the GPS track, clickable event pins, and a marker that follows playback (nearest GPS point within 3 s). The wheel zooms. The plan said Qt Location; that needs QML, `qtdeclarative` and `qtlocation`, a much bigger build for one line and a few pins.
- **Overlays** ([video_view.cpp](../../viewer/src/video_view.cpp)): the video is a `QGraphicsVideoItem` in a `QGraphicsView`, and `drawForeground` paints the observations of the analysed frame nearest the playhead (within 0.25 s): box, track id, class, distance, TTC; red and thick when TTC < 1.5 s. Observations are loaded a few seconds at a time.
- **Search**: the search box runs `rearview search "<q>" --json --limit 10` with `QProcess` (asynchronously, so the UI doesn't block while CLIP loads) and lists the hits; clicking one opens the trip, switches camera if needed, and seeks 3 s before the hit.

## Build notes

- **Media backend**: Qt Multimedia uses Windows Media Foundation (`windowsmediaplugin`), not its FFmpeg backend. The FFmpeg feature pulls in `qtdeclarative` and a second FFmpeg build, and Windows decodes the camera's H.264 natively.
- **Plugins**: vcpkg copies the DLLs the exe links to, not the plugins Qt loads at run time. A post-build step copies `platforms`, `styles`, `imageformats`, `sqldrivers`, `multimedia`, `tls` and `networkinformation`, plus `sqlite3.dll` and `jpeg62.dll`, which only the plugins need. Without `sqlite3.dll` Qt reports "can not load requested driver 'QSQLITE', available drivers: QSQLITE".

## Checking it without clicking

`viewer --screenshot out.png --event <id>` (or `--search "<query>"`) opens the event (or runs the search and opens the top hit), plays for 8 s (30 s for search), saves the window to `out.png` and a status report to `out.png.txt` (trip, camera, file, player state and position, boxes drawn, map tiles loaded, search hits), then quits. Results on the real library:

| Run | Result |
| --- | --- |
| `--event 187` (front near-miss, trip 2) | trip 2 opened, `000621F` playing at 12.6 s, 5 boxes with distance/TTC drawn, 12 map tiles, track and pins on the map |
| `--event 244` (rear near-miss, trip 9) | switched to the rear camera, `000720R` playing, 12 boxes drawn |
| `--search "a construction crane"` | 10 hits; the top one (trip 8, `000689F` @ 75 s, same as the CLI) opened and playing |

The first screenshot showed lines from Ottawa to the Atlantic: the receiver also writes half-filled records (latitude set, longitude 0), which are now dropped by the GPS decoder like the all-zero ones.
