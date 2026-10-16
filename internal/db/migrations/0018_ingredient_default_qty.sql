-- +goose Up
-- Default quantity suggested in the picker (e.g. 1 pz for a banana).
ALTER TABLE ingredients ADD COLUMN default_quantity REAL;

-- +goose Down
SELECT 1; -- SQLite < 3.35 non supporta DROP COLUMN
