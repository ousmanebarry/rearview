-- 0003_search.sql (task 7.4)
-- One CLIP vector per keyframe: 512 little-endian float32 values (2,048 bytes).
-- job_id lets an interrupted embed job be rolled back like observations.
CREATE TABLE keyframes (
  id              INTEGER PRIMARY KEY,
  job_id          INTEGER REFERENCES jobs(id) ON DELETE CASCADE,
  segment_id      INTEGER NOT NULL REFERENCES segments(id) ON DELETE CASCADE,
  t_offset_s      REAL NOT NULL,
  model           TEXT NOT NULL,
  vector          BLOB NOT NULL
);
CREATE INDEX keyframes_seg_t ON keyframes(segment_id, t_offset_s);
