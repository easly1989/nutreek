-- +goose Up
-- +goose StatementBegin
-- is_draft = 1 marca una ricetta come "bozza non confermata", tipicamente
-- creata da Duplicate. I draft sono:
--   - nascosti dalla lista ricette
--   - non selezionabili nei piani settimanali
--   - non salvabili (handleRecipeUpdate blocca il save finché is_draft = 1)
-- Vengono promossi a is_draft = 0 al primo add/remove ingrediente che
-- rende il contenuto diverso da tutte le altre ricette dell'utente.
ALTER TABLE recipes ADD COLUMN is_draft INTEGER NOT NULL DEFAULT 0;
CREATE INDEX idx_recipes_is_draft ON recipes(is_draft);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS idx_recipes_is_draft;
ALTER TABLE recipes DROP COLUMN is_draft;
-- +goose StatementEnd
