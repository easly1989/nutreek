-- +goose Up
ALTER TABLE users ADD COLUMN water_target_ml INTEGER NOT NULL DEFAULT 2000;

CREATE TABLE water_logs (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    logged_at  TEXT    NOT NULL,  -- YYYY-MM-DD (solo data, non datetime)
    amount_ml  INTEGER NOT NULL CHECK(amount_ml > 0),
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_water_logs_user_date ON water_logs(user_id, logged_at);

-- +goose Down
DROP TABLE IF EXISTS water_logs;
ALTER TABLE users DROP COLUMN water_target_ml;
