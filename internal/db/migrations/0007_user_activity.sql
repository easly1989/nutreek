-- +goose Up
ALTER TABLE users ADD COLUMN activity_level TEXT;

-- +goose Down
-- SQLite: colonna resta vuota e inoffensiva.
