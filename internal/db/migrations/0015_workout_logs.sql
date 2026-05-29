-- +goose Up
CREATE TABLE workout_activities (
    id           INTEGER PRIMARY KEY,
    name         TEXT    NOT NULL UNIQUE,
    kcal_per_hour REAL   NOT NULL CHECK(kcal_per_hour > 0),
    sort_order   INTEGER NOT NULL DEFAULT 0
);

INSERT INTO workout_activities (name, kcal_per_hour, sort_order) VALUES
    ('Palestra',           300, 1),
    ('Beach Volley',       300, 2),
    ('Pallavolo',          250, 3),
    ('Tennis',             350, 4),
    ('Calcio',             350, 5),
    ('Nuoto',              380, 6),
    ('Corsa',              480, 7),
    ('Ciclismo',           330, 8),
    ('Camminata veloce',   230, 9),
    ('Yoga / Stretching',  160, 10),
    ('Altro',              220, 11);

CREATE TABLE workout_logs (
    id               INTEGER PRIMARY KEY,
    user_id          INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    activity_id      INTEGER NOT NULL REFERENCES workout_activities(id),
    logged_at        DATE    NOT NULL,
    duration_minutes INTEGER NOT NULL CHECK(duration_minutes > 0 AND duration_minutes <= 720),
    kcal_burned      REAL    NOT NULL,
    note             TEXT,
    created_at       DATETIME DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_workout_logs_user_date ON workout_logs(user_id, logged_at DESC);

-- +goose Down
DROP TABLE workout_logs;
DROP TABLE workout_activities;
