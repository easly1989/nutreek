-- +goose Up
-- Indexes for lookups that had none and read whole tables.
--
-- off_products: the owner's OFF status counts products with and without complete nutrients.
-- Each count read the whole table (about 250,000 rows with their text columns): 3 s once in
-- production. With the index each count reads only the index.
CREATE INDEX IF NOT EXISTS idx_off_products_full_nutrients ON off_products(has_full_nutrients);
--
-- The columns that point at a recipe or an ingredient, which the recipe pages, the edit of an
-- ingredient and the delete of a recipe or an ingredient look up by value (and SQLite checks
-- the foreign keys with them). Until now the only index on them was the primary key of
-- ingredient_allergens.
CREATE INDEX IF NOT EXISTS idx_meal_items_recipe ON meal_items(recipe_id);
CREATE INDEX IF NOT EXISTS idx_meal_items_ingredient ON meal_items(ingredient_id);
CREATE INDEX IF NOT EXISTS idx_recipe_ingredients_ingredient ON recipe_ingredients(ingredient_id);
CREATE INDEX IF NOT EXISTS idx_meal_cheat_items_recipe ON meal_cheat_items(recipe_id);
CREATE INDEX IF NOT EXISTS idx_meal_cheat_items_ingredient ON meal_cheat_items(ingredient_id);
CREATE INDEX IF NOT EXISTS idx_nutr_items_recipe ON nutritionist_meal_items(recipe_id);
CREATE INDEX IF NOT EXISTS idx_nutr_items_ingredient ON nutritionist_meal_items(ingredient_id);
--
-- recipe_photos keeps the photo and its thumbnail before the ETag, so reading the ETag of a
-- row walks the pages of both pictures. The list of recipes reads one ETag per recipe, on
-- every request and every letter typed in its search; this index holds them without the pictures.
CREATE INDEX IF NOT EXISTS idx_recipe_photos_etag ON recipe_photos(recipe_id, etag);

-- +goose Down
DROP INDEX IF EXISTS idx_recipe_photos_etag;
DROP INDEX IF EXISTS idx_nutr_items_ingredient;
DROP INDEX IF EXISTS idx_nutr_items_recipe;
DROP INDEX IF EXISTS idx_meal_cheat_items_ingredient;
DROP INDEX IF EXISTS idx_meal_cheat_items_recipe;
DROP INDEX IF EXISTS idx_recipe_ingredients_ingredient;
DROP INDEX IF EXISTS idx_meal_items_ingredient;
DROP INDEX IF EXISTS idx_meal_items_recipe;
DROP INDEX IF EXISTS idx_off_products_full_nutrients;
