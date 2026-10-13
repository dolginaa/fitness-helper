-- When the workout was ended; NULL for workouts ended before it was kept and for imported ones.
ALTER TABLE workouts ADD COLUMN finished_at TIMESTAMPTZ;
