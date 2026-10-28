-- +goose Up
-- One photo per recipe (#207), already re-encoded as JPEG, with its square
-- thumbnail. A table of its own, so the recipe queries never read the blobs.
-- etag is a hash of the photo: it validates caches and busts them in the URL.
CREATE TABLE recipe_photos (
    recipe_id  INTEGER PRIMARY KEY REFERENCES recipes(id) ON DELETE CASCADE,
    photo      BLOB NOT NULL,
    thumb      BLOB NOT NULL,
    etag       TEXT NOT NULL,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- +goose Down
DROP TABLE recipe_photos;
