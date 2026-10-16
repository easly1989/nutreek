-- +goose Up
-- Porzione del prodotto in grammi (o ml), da serving_quantity di OFF (#136).
-- Il picker la propone come quantita' al posto della confezione intera.
-- NULL = porzione non nota; i sync successivi la riempiono.
ALTER TABLE off_products ADD COLUMN serving_g REAL;

-- +goose Down
-- Come in 0012: SQLite <3.35 non supporta DROP COLUMN, la colonna resta orfana.
