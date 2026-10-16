-- +goose Up
ALTER TABLE users ADD COLUMN weight_kg      REAL;
ALTER TABLE users ADD COLUMN height_cm      REAL;
ALTER TABLE users ADD COLUMN birth_year     INTEGER;
ALTER TABLE users ADD COLUMN sex            TEXT;
ALTER TABLE users ADD COLUMN target_kcal    REAL;
ALTER TABLE users ADD COLUMN target_protein REAL;
ALTER TABLE users ADD COLUMN target_carbs   REAL;
ALTER TABLE users ADD COLUMN target_fat     REAL;
ALTER TABLE users ADD COLUMN bmi            REAL;
ALTER TABLE users ADD COLUMN bmr            REAL;

-- +goose Down
-- SQLite < 3.35 does not support DROP COLUMN; the columns are empty and harmless.
