-- +goose Up
ALTER TABLE ingredients ADD COLUMN fiber_per_100        REAL;
ALTER TABLE ingredients ADD COLUMN sugars_per_100       REAL;
ALTER TABLE ingredients ADD COLUMN saturated_fat_per_100 REAL;
ALTER TABLE ingredients ADD COLUMN sodium_per_100       REAL;

-- +goose Down
SELECT 1;
