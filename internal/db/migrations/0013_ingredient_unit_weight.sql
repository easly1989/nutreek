-- +goose Up
-- unit_weight_g: grammi per singola unità dell'ingrediente (es. 150 per una
-- banana da 150g). NULL = ingrediente senza supporto "pezzi" (solo g/ml).
ALTER TABLE ingredients ADD COLUMN unit_weight_g REAL;

-- quantity_unit: unità con cui l'utente ha specificato la quantità nel piano.
-- 'g' = grammi/ml (comportamento storico, DEFAULT compatibile con righe esistenti).
-- 'pz' = pezzi; il calcolo macro usa quantity × ingredient.unit_weight_g.
ALTER TABLE meal_items ADD COLUMN quantity_unit TEXT NOT NULL DEFAULT 'g'
    CHECK(quantity_unit IN ('g','ml','pz'));

-- +goose Down
-- SQLite <3.35 non supporta DROP COLUMN inline. Su SQLite >=3.35 si potrebbe
-- eseguire ALTER TABLE ... DROP COLUMN. Lascio le colonne orfane in Down.
