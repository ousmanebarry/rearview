# Phase 1: Go foundation (weeks 2–3)

**Read first:** [../repo-structure.md](../repo-structure.md), [../data-model.md](../data-model.md), [../feature-specs.md](../feature-specs.md) (features A, B, C, M), [../testing.md](../testing.md), [../decisions.md](../decisions.md) (risks and gotchas)

Goal: `rearview import` then `rearview trips` lists your real drives with correct distances and times, and the GPX export lines up with real roads on a map.

- [x] **1.1 MP4 box walker [agent, you review]**
  - `internal/mp4gps`: read the box tree (4-byte big-endian size + 4-byte type; size 1 means a 64-bit size follows; size 0 means "to end of file"). Recurse into container boxes (`moov`, `trak`, `mdia`, `minf`, `stbl`, `udta`).
  - Use `io.ReaderAt` and read headers only; never load a whole file into memory (files are hundreds of MB).
  - Done when: a `rearview debug boxes <file>` command prints the box tree of a real file, and a unit test covers 64-bit sizes and a truncated file.
- [x] **1.2 GPS block decoder [agent, you review]**
  - Find the GPS blocks (marked `freeGPS`), decode time, active flag, latitude, longitude, speed and course. Values are little-endian floats; speed is in knots (× 1.852 = km/h).
  - Work out the coordinate format by decoding a point at a known location, then compare against `exiftool -ee`.
  - Skip records where the active flag says there is no fix. Look for accelerometer values in the same blocks and decode them if present.
  - Input needed: the coordinates of one known spot (for example the driveway) and a file recorded there.
  - Done when: golden test compares the output with saved exiftool output for 3+ files: latitude/longitude within 1e-5°, speed within 0.1 km/h, same number of points.
- [x] **1.3 Store and migrations [agent]**
  - `internal/store`: open the database with WAL and foreign keys, apply numbered migrations from an embedded folder (`embed.FS`), record applied versions.
  - Add `0001_init.sql` from `docs/plan/data-model.md`.
  - Done when: opening a fresh database creates every table; opening it again changes nothing; tests run against a temp file.
- [x] **1.4 Config file [agent]**
  - `internal/config`: YAML or TOML file with library path, database path, clip folder, file-name patterns and every threshold from Feature specs. Defaults in code; the file overrides them.
  - Done when: `rearview config show` prints the merged config.
- [x] **1.5 Ingest [agent, you review]**
  - `internal/ingest`: given a source folder, find MP4s, wait until each file's size is unchanged for 5 seconds, compute the hash (SHA-256 of the first and last 4 MB plus the size), skip known hashes, copy into `library/YYYY/MM/DD/`, then insert the `segments` row.
  - Tag camera from the file name using patterns from config (front/rear suffix); mark `locked` for files under an `RO` folder.
  - Read duration, resolution and fps with `ffprobe -print_format json`.
  - Copy to a temp name and rename at the end, so a crash never leaves half a file in the library.
  - Done when: importing the same folder twice adds 0 rows; pulling the card mid-import leaves no partial files; tests cover each case using small fake files.
- [x] **1.6 Import command [agent]**
  - `rearview import <path>` runs ingest + GPS extraction for each new segment and prints a summary (new, skipped, failed). `--watch` keeps watching the folder with fsnotify.
  - Done when: running it on the backed-up footage imports everything with sensible counts.
- [x] **1.7 Trip assembly [agent, you review]**
  - `internal/trips`: sort segments by time; a gap over 5 minutes (configurable) starts a new trip. Give front and rear files that start within 2 seconds of each other the same `pair_key`.
  - Correct `started_at` using GPS UTC time when a fix exists (store the offset so files without a fix get the same correction).
  - Fill GPS gaps under 10 seconds by linear interpolation (`interpolated = 1`); leave longer gaps empty.
  - Compute distance (haversine sum), duration, average and maximum speed.
  - Done when: a test with hand-built segments covers gaps, pairing and clock correction, and the review confirms the real week of footage splits into the drives actually taken.
- [x] **1.8 Trips command and GPX export [agent]**
  - `rearview trips` lists trips (date, start time, duration, distance, max speed). `rearview trips export <id> --gpx out.gpx` writes a GPX track.
  - Done when: the GPX opens in a map viewer and follows the roads driven.

**Phase 1 deliverable:** a working importer and trip log built on your own MP4 parser.
