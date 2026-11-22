-- +goose Up
-- Meal times and reminders (#236). A meal has a start time (HH:MM, wall clock, no
-- time zone) and an optional end; a reminder is a meal with no food (a supplement,
-- a medicine) that counts for nothing. Meals from before have no time (NULL) and
-- sort after the timed ones. meal_time_defaults holds one time per meal name and
-- user, copied to the meals created with that name.
ALTER TABLE meals ADD COLUMN start_time TEXT;
ALTER TABLE meals ADD COLUMN end_time TEXT;
ALTER TABLE meals ADD COLUMN is_reminder INTEGER NOT NULL DEFAULT 0 CHECK (is_reminder IN (0, 1));
ALTER TABLE meals ADD COLUMN note TEXT NOT NULL DEFAULT '';

CREATE TABLE meal_time_defaults (
    user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name       TEXT    NOT NULL COLLATE NOCASE,
    start_time TEXT    NOT NULL,
    end_time   TEXT,
    PRIMARY KEY (user_id, name)
);

-- A column «Ora» in the printed week and in the CSV export, off by default.
ALTER TABLE users ADD COLUMN show_meal_times INTEGER NOT NULL DEFAULT 0 CHECK (show_meal_times IN (0, 1));

-- +goose Down
ALTER TABLE users DROP COLUMN show_meal_times;
DROP TABLE meal_time_defaults;
ALTER TABLE meals DROP COLUMN note;
ALTER TABLE meals DROP COLUMN is_reminder;
ALTER TABLE meals DROP COLUMN end_time;
ALTER TABLE meals DROP COLUMN start_time;
