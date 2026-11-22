-- +goose Up
-- Keep the phone screen on while a page is open (#249): a per-user setting, off
-- by default. The page asks the browser for a Screen Wake Lock when it is set.
ALTER TABLE users ADD COLUMN keep_awake INTEGER NOT NULL DEFAULT 0 CHECK (keep_awake IN (0, 1));

-- +goose Down
ALTER TABLE users DROP COLUMN keep_awake;
