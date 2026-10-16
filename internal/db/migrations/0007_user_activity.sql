-- +goose Up
ALTER TABLE users ADD COLUMN activity_level TEXT;

-- +goose Down
-- SQLite: the column stays empty and harmless.
