-- +goose Up
-- +goose StatementBegin

ALTER TABLE meal_items ADD COLUMN sort_order INTEGER NOT NULL DEFAULT 0;

-- Popola le righe esistenti con sort_order = id, cosi' rimangono in ordine
-- di creazione anche dopo l'introduzione del campo.
UPDATE meal_items SET sort_order = id WHERE sort_order = 0;

CREATE INDEX idx_meal_items_meal_sort ON meal_items(meal_id, sort_order);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS idx_meal_items_meal_sort;
ALTER TABLE meal_items DROP COLUMN sort_order;
-- +goose StatementEnd
