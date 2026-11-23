-- +goose Up
-- +goose StatementBegin

-- Workouts, part 2 (#251): activities of one's own, a time of day on a workout, and a
-- setting that lets the day's workouts raise the day's goal.
--
-- 1) workout_activities gets user_id (NULL = the built-in list, shared). Its name
--    was UNIQUE for everyone, so two people could not both add «Crossfit»: the rule
--    becomes unique per owner, which SQLite only does by rebuilding the table.
--    PRAGMA foreign_keys is a no-op inside the goose transaction, so with FKs on,
--    dropping the parent would fail on its children (see 0025): workout_logs is
--    rebuilt too, children dropped before parents, renamed parents before children.

CREATE TABLE workout_activities_new (
    id            INTEGER PRIMARY KEY,
    name          TEXT    NOT NULL,
    kcal_per_hour REAL    NOT NULL CHECK(kcal_per_hour > 0),
    sort_order    INTEGER NOT NULL DEFAULT 0,
    code          TEXT,
    user_id       INTEGER REFERENCES users(id) ON DELETE CASCADE
);
INSERT INTO workout_activities_new (id, name, kcal_per_hour, sort_order, code)
  SELECT id, name, kcal_per_hour, sort_order, code FROM workout_activities;

CREATE TABLE workout_logs_new (
    id               INTEGER PRIMARY KEY,
    user_id          INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    activity_id      INTEGER NOT NULL REFERENCES workout_activities_new(id),
    logged_at        DATE    NOT NULL,
    duration_minutes INTEGER NOT NULL CHECK(duration_minutes > 0 AND duration_minutes <= 720),
    kcal_burned      REAL    NOT NULL,
    note             TEXT,
    created_at       DATETIME DEFAULT CURRENT_TIMESTAMP,
    at_time          TEXT CHECK(at_time IS NULL OR at_time GLOB '[0-2][0-9]:[0-5][0-9]')
);
INSERT INTO workout_logs_new (id, user_id, activity_id, logged_at, duration_minutes, kcal_burned, note, created_at)
  SELECT id, user_id, activity_id, logged_at, duration_minutes, kcal_burned, note, created_at FROM workout_logs;

DROP TABLE workout_logs;
DROP TABLE workout_activities;

ALTER TABLE workout_activities_new RENAME TO workout_activities;
ALTER TABLE workout_logs_new       RENAME TO workout_logs;

CREATE UNIQUE INDEX idx_workout_activities_code ON workout_activities(code);
CREATE UNIQUE INDEX idx_workout_activities_name ON workout_activities(COALESCE(user_id, 0), name);
CREATE INDEX idx_workout_activities_user ON workout_activities(user_id);
CREATE INDEX idx_workout_logs_user_date ON workout_logs(user_id, logged_at DESC);

-- 2) Off by default: workouts stay an estimate beside the day, not part of the goal.
ALTER TABLE users ADD COLUMN workouts_in_goal INTEGER NOT NULL DEFAULT 0 CHECK (workouts_in_goal IN (0, 1));

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE users DROP COLUMN workouts_in_goal;

-- Activities of one's own, and the logs that use them, go away with the schema that held them.
DELETE FROM workout_logs WHERE activity_id IN (SELECT id FROM workout_activities WHERE user_id IS NOT NULL);
DELETE FROM workout_activities WHERE user_id IS NOT NULL;

CREATE TABLE workout_activities_old (
    id            INTEGER PRIMARY KEY,
    name          TEXT    NOT NULL UNIQUE,
    kcal_per_hour REAL    NOT NULL CHECK(kcal_per_hour > 0),
    sort_order    INTEGER NOT NULL DEFAULT 0,
    code          TEXT
);
INSERT INTO workout_activities_old (id, name, kcal_per_hour, sort_order, code)
  SELECT id, name, kcal_per_hour, sort_order, code FROM workout_activities;

CREATE TABLE workout_logs_old (
    id               INTEGER PRIMARY KEY,
    user_id          INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    activity_id      INTEGER NOT NULL REFERENCES workout_activities_old(id),
    logged_at        DATE    NOT NULL,
    duration_minutes INTEGER NOT NULL CHECK(duration_minutes > 0 AND duration_minutes <= 720),
    kcal_burned      REAL    NOT NULL,
    note             TEXT,
    created_at       DATETIME DEFAULT CURRENT_TIMESTAMP
);
INSERT INTO workout_logs_old (id, user_id, activity_id, logged_at, duration_minutes, kcal_burned, note, created_at)
  SELECT id, user_id, activity_id, logged_at, duration_minutes, kcal_burned, note, created_at FROM workout_logs;

DROP TABLE workout_logs;
DROP TABLE workout_activities;

ALTER TABLE workout_activities_old RENAME TO workout_activities;
ALTER TABLE workout_logs_old       RENAME TO workout_logs;

CREATE UNIQUE INDEX idx_workout_activities_code ON workout_activities(code);
CREATE INDEX idx_workout_logs_user_date ON workout_logs(user_id, logged_at DESC);
-- +goose StatementEnd
