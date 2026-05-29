-- +goose Up
CREATE TABLE weight_logs (
    id          INTEGER PRIMARY KEY,
    user_id     INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    measured_at DATE    NOT NULL,
    weight_kg   REAL    NOT NULL CHECK(weight_kg > 0 AND weight_kg < 500),
    note        TEXT,
    created_at  DATETIME DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(user_id, measured_at)
);
CREATE INDEX idx_weight_logs_user_date ON weight_logs(user_id, measured_at DESC);

-- +goose Down
DROP TABLE weight_logs;
