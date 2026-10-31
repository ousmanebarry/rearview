# Phase 1: import, GPS and trips

How footage gets into the library: the MP4 box walker, the GPS decoder, the importer and trip assembly.

## 1.1 MP4 box walker

### What it does

`mp4gps.Walk` reads the box tree of an MP4 through an `io.ReaderAt` and returns `[]Box` (type, offset, size, header size, children). `rearview debug boxes <file>` prints it. Only box headers are read, so a 650 MB file costs a few hundred small reads.

### Core logic ([go/internal/mp4gps/box.go](../../go/internal/mp4gps/box.go))

- **Header (`readHeader`).** 4-byte big-endian size, 4-byte type. Size 1: a 64-bit size follows and the header is 16 bytes. Size 0: the box runs to the end of its parent (or the file).
- **Sanity checks.** A size smaller than its own header is an error (it would loop forever). A box that runs past its parent's end, or a header cut short, is `ErrTruncated`. 64-bit sizes above 2^62 are rejected before they can overflow `int64` arithmetic.
- **Recursion (`walk`).** Only known containers (`moov`, `trak`, `mdia`, `minf`, `stbl`, `udta`, `edts`, `dinf`) are descended into, bounded to their parent's range and to 16 levels. Everything else (`mdat`, `free`, `gps `) is a leaf.
- **Partial results.** On truncation `Walk` returns the boxes read so far together with the error, so a recording cut by a power loss still yields its `moov` (VIOFO writes `moov` near the start of the file).
- **`Find(boxes, "moov", "gps ")`** follows a path of types and returns the first match.

### Real file layout (VIOFO A229 Plus)

`ftyp`, `skip`, `moov` (`mvhd`, video `trak`, audio `trak`, `udta`, `gps `), then `free` boxes holding the GPS blocks interleaved with the media data.

### Tests ([box_test.go](../../go/internal/mp4gps/box_test.go))

Synthetic trees built in memory: nesting and offsets, 64-bit sizes, size 0, a file cut inside a box, a child running past its parent, a header cut inside its 64-bit size, and a size smaller than the header.

## 1.2 GPS block decoder

### Where the GPS lives

`moov/gps ` is an index: version/flags (`0x00000101`), a count, then one big-endian (offset, size) pair per block. Each pair points to a 16 KB `free` box written once per second (180 per 3-minute file). Front and rear files each carry their own copy.

### Block layout ([go/internal/mp4gps/gps.go](../../go/internal/mp4gps/gps.go))

After the 8-byte box header: `"GPS "`, a little-endian length (56), then:

| Offset (after length) | Field                                               | Type          |
| --------------------- | --------------------------------------------------- | ------------- |
| 0–23                  | hour, minute, second, year − 2000, month, day (UTC) | 6 × uint32 LE |
| 24                    | status `A` (fix) / `V` (void)                       | byte          |
| 25, 26                | `N`/`S`, `E`/`W`                                    | byte          |
| 28                    | latitude, NMEA `ddmm.mmmm`                          | float32 LE    |
| 32                    | longitude, NMEA `dddmm.mmmm`                        | float32 LE    |
| 36                    | speed, knots                                        | float32 LE    |
| 40                    | heading, degrees                                    | float32 LE    |

- **Coordinates.** `4517.7832` means 45° 17.7832′, so degrees = `floor(v/100) + (v mod 100)/60`; south and west are negated. Found by decoding the first fix of `2026_1023_130515_000587F` (parked, 45.29639 N 75.90025 W) and matching `exiftool -ee` to the last digit.
- **Speed.** Knots × 1.852 = km/h.
- **Void records** (`V`) are skipped. They occur at the start of a drive before the receiver gets a fix (7–35 per affected file).
- **All-zero records.** 72 records in the 235 files are flagged `A` but have position, speed and heading all 0, and 60 more have a latitude but a longitude of 0 (or the reverse). Any record with a zero latitude or longitude is skipped like a void record (found in Phase 10 when a hard brake landed at 0,0, and in Phase 9 when the viewer's map drew lines to the Atlantic). exiftool keeps them; the golden test skips them on its side.
- **Accelerometer.** The 16 bytes after the heading are zero in all 235 backed-up files, and exiftool reports no accelerometer tags, so this camera stores none. The G-spike detector (2.1) stays disabled.
- **Corrupt dates.** The firmware occasionally writes a garbage date with a good position (seen: year 41 month 2 day 0, and year 2 month 0 day 0). `DecodeBlock` leaves `Time` zero for those, and `ReadGPS` repairs it from the nearest good block shifted by the index difference (`TimeRepaired = true`). exiftool keeps the first kind with the bogus date and drops the second kind entirely.
- **`Index`** is the block's position in the index. Blocks are one second apart, so it doubles as `t_offset_s`.

### Golden tests ([gps_test.go](../../go/internal/mp4gps/gps_test.go))

Stream-copied clips lose the GPS blocks, so `rearview debug gps-fixture <in> <out>` writes a ~13 KB file holding only `ftyp`, `moov/gps ` and each block cut to 64 bytes. `testdata/gps/` holds three of these plus `exiftool -ee -n -p '$gpsdatetime,$gpslatitude,$gpslongitude,$gpsspeed,$gpstrack'` output from the full files:

- `2026_1023_151153_000593F`: front, 7 void records at the start.
- `2026_1024_122252_000669F`: front, 29 void records and both kinds of corrupt date.
- `2026_1024_131801_000700R`: rear, highway, full fix.

The test aligns our points with exiftool's (latitude/longitude within 1e-5°, speed within 0.1 km/h, heading within 0.01°, same time wherever exiftool's date is plausible). The only extras allowed are records with a repaired time.

To regenerate: `rearview debug gps-fixture <file> testdata/gps/<name>.bin` and the exiftool command above into `<name>.exiftool.csv`.

## 1.5 Ingest

### What it does

`ingest.Ingester.ImportDir(root)` finds every `.mp4` under `root`, and for each one: waits until it stops changing, hashes it, skips it if the hash is known, tags camera / locked / parking, probes it with ffprobe, copies it into `library/YYYY/MM/DD/` and inserts the `segments` row. One failing file does not stop the rest; each gets a `Result` (`new`, `skipped`, `failed` + error).

### Core logic ([go/internal/ingest/ingest.go](../../go/internal/ingest/ingest.go))

- **Stable-size wait (`waitStable`).** A file is stable once its size has not changed for `import.stable_seconds` (5 s). The clock starts at the file's modification time, so a backup folder written hours ago imports without waiting, while a file still being written by the camera (or copied in by Explorer) is polled once a second until it settles. Cancelling the context stops the wait.
- **Content hash (`Hash`).** SHA-256 over the first 4 MB, the last 4 MB and the size as a little-endian uint64. Reading 8 MB instead of 650 MB keeps a re-import of a full card fast. Two different loop recordings share neither their start (different timestamps in the first frames) nor their end, so this is a safe dedup key; the test documents that a change in the middle is not detected, by design.
- **Camera and flags.** Regexes from config (`_\d+F.mp4` front, `R` rear, `I` interior). No match is a failure, not a guess. `locked` = the file sits in a folder named `RO` below the source root; `parking` = a folder named `Parking`.
- **Start time.** The VIOFO file name is the camera-local start time (`2026_1023_130515` → 13:05:15 in `time_zone`, America/New_York), converted to UTC. Without a parsable name it falls back to modification time minus duration. Phase 1.7 corrects it with GPS time.
- **Crash-safe copy (`copyIn`).** Copy to `<name>.part`, `fsync`, then rename. A read error (card pulled) deletes the `.part`, so the library never holds a partial file under a real name. If the row insert fails after the rename, the copy is removed. If a file with the same name already exists (a crash between rename and insert), it is reused when its hash matches, otherwise the new copy gets a `_1` suffix. `CleanPartials` removes `.part` files left by a hard crash; the import command calls it at start.
- **Probe (`FFprobe`).** `ffprobe -print_format json -show_format -show_streams -select_streams v:0`: width, height, `avg_frame_rate` as a fraction, duration from the container.

### Tests ([ingest_test.go](../../go/internal/ingest/ingest_test.go))

Small fake files and a stub prober: importing twice adds 0 rows; camera, locked, start time (EDT → UTC) and library path; a reader that fails after 300 bytes leaves no file and no row; a file that grows during the wait is imported in full and only after it settles; cancel during the wait; hash coverage; ffprobe JSON parsing; `.part` clean-up.

## 1.7 Trip assembly

### What it does

`trips.Rebuild` reads every segment and GPS point, computes a `TripPlan` with the pure function `Assemble`, and writes it with `store.ApplyTrips` in one transaction. The import command runs it after any import that added files, so trips always reflect the whole library.

### Core logic ([go/internal/trips/trips.go](../../go/internal/trips/trips.go))

1. **Clock correction (`fixClocks`).** For each segment, compare every real GPS time with the file-name time plus the point's offset; the median difference is the camera clock error. `started_at` becomes file-name time + offset. Segments without a fix (rear files with void records, a short file before the receiver locks) borrow the offset of the nearest segment that has one.
   - The offset is stored in `segments.clock_offset_s` (migration `0002_clock_offset.sql`), and the file-name time is recovered as `started_at − clock_offset_s`. Without it, a second rebuild would apply a borrowed offset twice.
   - On the real footage the offset is 0 s: the VIOFO syncs its clock from GPS.
2. **Pairing (`pair`).** Each front file is matched with the nearest unmatched rear file starting within `trips.pair_tolerance_s` (2 s). Both get `pair_key` = the earlier start as `20261023T170515Z`. Unpaired files get `<start>-<camera>`, so the key is never empty and never shared by accident.
3. **Trips.** Segments sorted by corrected start; a gap of more than `trips.gap_minutes` (5) between the latest end so far and the next start opens a new trip. Parking footage is left out of trips.
4. **Gap filling (`interpolate`).** Inside each segment, two real points 2–9 s apart get one point per missing second on the straight line between them (`interpolated = 1`). Heading is interpolated the short way round (350° → 10° passes through 0°). Gaps of 10 s or more stay empty. Interpolated rows are deleted and recomputed on every rebuild, so they never pile up.
5. **Summary (`summarize`, `stitch`).** One camera's GPS per moment (front preferred, else rear), merged in time order. Distance = sum of haversine legs; a leg implying more than `max_plausible_speed_kmh` (250) is a glitch and is skipped. Max speed ignores implausible values; average = distance ÷ duration (stops included).
6. **Stable ids (`reuseID`).** A new trip takes the id most of its segments already had, so `rearview trips export 3` keeps meaning the same drive after the next import. Events are re-pointed to their segment's trip at the end of the transaction.

### Tests ([trips_test.go](../../go/internal/trips/trips_test.go))

Hand-built segments: 4-minute gap joins, 6-minute gap splits, parking excluded; pairing at 1 s and not at 3 s; clock correction (+3 s with fix, borrowed for the rear without fix, −2 s for a distant segment) and no drift on a second pass; 4-s hole filled, 12-s hole left; heading wraparound; distance with a one-point glitch and the rear track ignored; a database round trip that keeps trip ids after a new trip is added.
