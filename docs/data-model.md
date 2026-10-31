# Data model

One SQLite database (`~/rearview/rearview.db`), written only by Go; the viewer opens it read-only. The schema is the migrations in [go/internal/store/migrations/](../go/internal/store/migrations/), applied in order on open: `0001_init.sql` below, then the two additions at the end.

## Conventions

- Absolute times are UTC, stored as ISO 8601 text (`2026-10-24T18:42:07Z`). Times inside a file are seconds from its start (`t_offset_s`).
- Speeds in km/h, distances in metres or kilometres, angles in degrees. Units are in column names.
- Turn on `PRAGMA foreign_keys = ON` and `PRAGMA journal_mode = WAL` at every connection, so readers and the writer don't block each other.

## 0001_init.sql

```sql
-- 0001_init.sql
CREATE TABLE trips (
  id              INTEGER PRIMARY KEY,
  started_at      TEXT NOT NULL,
  ended_at        TEXT NOT NULL,
  distance_km     REAL,
  duration_s      REAL,
  max_speed_kmh   REAL,
  avg_speed_kmh   REAL,
  score           REAL                       -- Phase 8
);

CREATE TABLE segments (
  id              INTEGER PRIMARY KEY,
  path            TEXT NOT NULL UNIQUE,      -- path inside the library
  source_name     TEXT NOT NULL,             -- original file name on the SD card
  content_hash    TEXT NOT NULL UNIQUE,      -- dedup key
  size_bytes      INTEGER NOT NULL,
  camera          TEXT NOT NULL CHECK (camera IN ('front','rear','interior')),
  started_at      TEXT NOT NULL,             -- UTC, corrected with GPS time when available
  duration_s      REAL,
  width           INTEGER,
  height          INTEGER,
  fps             REAL,
  locked          INTEGER NOT NULL DEFAULT 0, -- came from the RO folder
  parking         INTEGER NOT NULL DEFAULT 0, -- parking-mode footage
  pair_key        TEXT,                      -- same value for front + rear of one moment
  trip_id         INTEGER REFERENCES trips(id),
  imported_at     TEXT NOT NULL
);
CREATE INDEX segments_started ON segments(started_at);
CREATE INDEX segments_trip ON segments(trip_id);

CREATE TABLE gps_points (
  segment_id      INTEGER NOT NULL REFERENCES segments(id) ON DELETE CASCADE,
  t_offset_s      REAL NOT NULL,
  ts_utc          TEXT NOT NULL,
  lat             REAL NOT NULL,
  lon             REAL NOT NULL,
  speed_kmh       REAL,
  heading_deg     REAL,
  accel_x         REAL, accel_y REAL, accel_z REAL, -- only if the camera stores them
  interpolated    INTEGER NOT NULL DEFAULT 0,
  PRIMARY KEY (segment_id, t_offset_s)
);

CREATE TABLE jobs (
  id              INTEGER PRIMARY KEY,
  segment_id      INTEGER NOT NULL REFERENCES segments(id) ON DELETE CASCADE,
  kind            TEXT NOT NULL,             -- analyze | embed_frames | blur
  windows_json    TEXT NOT NULL DEFAULT '[]',
  priority        INTEGER NOT NULL,          -- 0 = highest
  state           TEXT NOT NULL CHECK (state IN ('pending','running','done','failed')),
  attempts        INTEGER NOT NULL DEFAULT 0,
  last_error      TEXT,
  engine_fps      REAL,
  created_at      TEXT NOT NULL,
  started_at      TEXT,
  finished_at     TEXT,
  UNIQUE (segment_id, kind, windows_json)
);
CREATE INDEX jobs_state ON jobs(state, priority);

CREATE TABLE observations (                  -- one row per tracked object per analyzed frame
  job_id          INTEGER NOT NULL REFERENCES jobs(id) ON DELETE CASCADE,
  segment_id      INTEGER NOT NULL REFERENCES segments(id) ON DELETE CASCADE,
  t_offset_s      REAL NOT NULL,
  track_id        INTEGER NOT NULL,
  cls             TEXT NOT NULL,
  x1 REAL, y1 REAL, x2 REAL, y2 REAL,
  conf            REAL NOT NULL,
  dist_m          REAL,
  ttc_s           REAL
);
CREATE INDEX observations_seg_t ON observations(segment_id, t_offset_s);

CREATE TABLE events (
  id              INTEGER PRIMARY KEY,
  trip_id         INTEGER REFERENCES trips(id),
  segment_id      INTEGER NOT NULL REFERENCES segments(id),
  camera          TEXT,
  type            TEXT NOT NULL,             -- hard_brake | swerve | g_spike | near_miss | following_close | tailgated
  source          TEXT NOT NULL,             -- motion | vision | fused
  ts_utc          TEXT NOT NULL,
  t_start_s       REAL NOT NULL,
  t_end_s         REAL NOT NULL,
  score           REAL NOT NULL,             -- 0..1, higher = more severe
  lat REAL, lon REAL,
  details_json    TEXT NOT NULL DEFAULT '{}',
  created_at      TEXT NOT NULL
);
CREATE INDEX events_type_ts ON events(type, ts_utc);

CREATE TABLE clips (
  id              INTEGER PRIMARY KEY,
  event_id        INTEGER REFERENCES events(id) ON DELETE SET NULL,
  path            TEXT NOT NULL,
  layout          TEXT NOT NULL DEFAULT 'single', -- single | side_by_side
  blurred         INTEGER NOT NULL DEFAULT 0,
  created_at      TEXT NOT NULL
);

CREATE TABLE labels (                        -- your ground truth for accuracy numbers
  id              INTEGER PRIMARY KEY,
  ts_utc          TEXT NOT NULL,             -- keyed by time, so labels survive re-imports
  camera          TEXT,
  type            TEXT NOT NULL,
  note            TEXT
);

CREATE TABLE bench_runs (
  id              INTEGER PRIMARY KEY,
  ran_at          TEXT NOT NULL,
  git_sha         TEXT NOT NULL,
  config_json     TEXT NOT NULL,
  results_json    TEXT NOT NULL
);
```

## Added later

- `0002_clock_offset.sql` (task 1.7): `segments.clock_offset_s`, GPS time minus file-name time, so `started_at` can be corrected again without compounding.
- `0003_search.sql` (task 7.4): `keyframes(id, job_id, segment_id, t_offset_s, model, vector BLOB)`, one CLIP vector per keyframe and crop (`model` is `clip-vit-b32`, `clip-vit-b32/left` or `clip-vit-b32/right`), stored as 512 little-endian float32 values (2,048 bytes). `job_id` lets an interrupted embed job be rolled back like observations.

The driving score and incident reports needed no new tables: `trips.score` was in `0001` and PDFs are written next to the clips.

**Event ids are stable**: re-running `events detect` or fusion updates an event with the same type, camera and time in place, so ids and the clips pointing at them survive.

**Size**: the full library (5.35 h per camera) holds 38,128 GPS points, 57,798 keyframe vectors (about 118 MB) and 696,531 observation rows for every analysed front and rear frame.
