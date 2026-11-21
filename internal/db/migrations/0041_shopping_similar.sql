-- +goose Up
-- Similar shopping items (#233). When two rows of a list are the same
-- ingredient and the user merges them, the one dropped is remembered per list
-- (from_ingredient_id becomes to_ingredient_id when the list is generated, so
-- what comes from a recipe is merged too), and when the user keeps two similar
-- rows apart, the pair is remembered so it is not offered again. A row is
-- named by its key: «i<ingredient id>» for a row from the meals, «u<id>» for
-- one added by hand.
CREATE TABLE shopping_list_merges (
    shopping_list_id   INTEGER NOT NULL REFERENCES shopping_lists(id) ON DELETE CASCADE,
    from_ingredient_id INTEGER NOT NULL REFERENCES ingredients(id) ON DELETE CASCADE,
    to_ingredient_id   INTEGER NOT NULL REFERENCES ingredients(id) ON DELETE CASCADE,
    PRIMARY KEY (shopping_list_id, from_ingredient_id)
);

CREATE TABLE shopping_similar_dismissed (
    shopping_list_id INTEGER NOT NULL REFERENCES shopping_lists(id) ON DELETE CASCADE,
    key_a            TEXT    NOT NULL,
    key_b            TEXT    NOT NULL,
    PRIMARY KEY (shopping_list_id, key_a, key_b)
);

-- +goose Down
DROP TABLE shopping_similar_dismissed;
DROP TABLE shopping_list_merges;
