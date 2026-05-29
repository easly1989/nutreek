-- +goose Up
-- Quantità predefinita suggerita nel picker (es. 1 pz per una banana).
ALTER TABLE ingredients ADD COLUMN default_quantity REAL;

-- +goose Down
SELECT 1; -- SQLite < 3.35 non supporta DROP COLUMN
