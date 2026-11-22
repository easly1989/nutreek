-- +goose Up
-- off_aligned says an ingredient imported from OpenFoodFacts is still as it was
-- imported: nobody edited its name, brand, unit or nutrients (#225). Such a row
-- is safe to refresh from its product. CreateOFF sets it; Update clears it when
-- an edit changes one of those fields.
--
-- Rows imported before this migration: aligned when the product is still in
-- off_products with the same four macros (a row we cannot compare stays not
-- aligned, which is the safe side: it is never overwritten).
ALTER TABLE ingredients ADD COLUMN off_aligned INTEGER NOT NULL DEFAULT 0 CHECK (off_aligned IN (0, 1));

UPDATE ingredients SET off_aligned = 1
WHERE source = 'openfoodfacts' AND off_barcode IS NOT NULL
  AND EXISTS (
    SELECT 1 FROM off_products p
    WHERE p.barcode = ingredients.off_barcode
      AND abs(p.kcal_per_100 - ingredients.kcal_per_100) < 0.01
      AND abs(p.protein_per_100 - ingredients.protein_per_100) < 0.01
      AND abs(p.carbs_per_100 - ingredients.carbs_per_100) < 0.01
      AND abs(p.fat_per_100 - ingredients.fat_per_100) < 0.01
  );

-- +goose Down
ALTER TABLE ingredients DROP COLUMN off_aligned;
