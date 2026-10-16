-- +goose Up
-- +goose StatementBegin

ALTER TABLE meal_items ADD COLUMN sort_order INTEGER NOT NULL DEFAULT 0;

-- Fill the existing rows with sort_order = id, so they keep their creation
-- order after the column is introduced.
UPDATE meal_items SET sort_order = id WHERE sort_order = 0;

CREATE INDEX idx_meal_items_meal_sort ON meal_items(meal_id, sort_order);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS idx_meal_items_meal_sort;
ALTER TABLE meal_items DROP COLUMN sort_order;
-- +goose StatementEnd
