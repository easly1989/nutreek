-- +goose Up
-- Aggiunge la URL del thumbnail del prodotto OFF (image_front_small_url dal dump
-- JSONL o dall'API live). Usata per mostrare miniature nella ricerca ingredienti.
-- DEFAULT '' per compatibilità con righe già presenti (sync successivi la populano).
ALTER TABLE off_products ADD COLUMN image_url TEXT NOT NULL DEFAULT '';

-- +goose Down
-- SQLite <3.35 non supporta DROP COLUMN; lascio la colonna orfana.
-- Su SQLite >=3.35 si potrebbe eseguire: ALTER TABLE off_products DROP COLUMN image_url;
