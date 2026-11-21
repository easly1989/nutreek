-- +goose Up
-- «My order» for the shopping list (#231): the users' grouping setting gains a
-- third value, 'custom', and the order they drag the rows into is stored per
-- list. SQLite cannot change a CHECK, so the column is rebuilt.
ALTER TABLE users ADD COLUMN shopping_group_new TEXT NOT NULL DEFAULT 'none' CHECK (shopping_group_new IN ('none', 'category', 'custom'));
UPDATE users SET shopping_group_new = shopping_group;
ALTER TABLE users DROP COLUMN shopping_group;
ALTER TABLE users RENAME COLUMN shopping_group_new TO shopping_group;

-- row_key is «i<ingredient id>» for a row computed from the meals (so the
-- order survives a regenerate) and «u<id>» for one added by hand.
CREATE TABLE shopping_list_order (
    shopping_list_id INTEGER NOT NULL REFERENCES shopping_lists(id) ON DELETE CASCADE,
    row_key          TEXT    NOT NULL,
    position         INTEGER NOT NULL,
    PRIMARY KEY (shopping_list_id, row_key)
);

-- +goose Down
DROP TABLE shopping_list_order;
ALTER TABLE users ADD COLUMN shopping_group_old TEXT NOT NULL DEFAULT 'none' CHECK (shopping_group_old IN ('none', 'category'));
UPDATE users SET shopping_group_old = CASE WHEN shopping_group = 'category' THEN 'category' ELSE 'none' END;
ALTER TABLE users DROP COLUMN shopping_group;
ALTER TABLE users RENAME COLUMN shopping_group_old TO shopping_group;
