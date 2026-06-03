-- +goose Up
ALTER TABLE users ADD COLUMN tour_dismissed_at TIMESTAMP;

-- +goose Down
SELECT 1;
