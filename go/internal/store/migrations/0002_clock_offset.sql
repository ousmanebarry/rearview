-- 0002_clock_offset.sql
-- GPS time minus the file-name time, so started_at can be corrected again
-- without compounding (task 1.7).
ALTER TABLE segments ADD COLUMN clock_offset_s REAL NOT NULL DEFAULT 0;
