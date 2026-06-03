-- +goose Up
ALTER TABLE users ADD COLUMN onboarding_completed_at TIMESTAMP;

-- +goose Down
SELECT 1;
