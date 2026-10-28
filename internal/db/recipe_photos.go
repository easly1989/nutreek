package db

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
)

// ErrPhotoNotFound means the recipe has no photo.
var ErrPhotoNotFound = errors.New("recipe photo not found")

// SetPhoto stores the recipe's photo and thumbnail (JPEG, see package photo),
// replacing the one it had. Only the owner may: ErrRecipeForbidden otherwise.
func (r *Recipes) SetPhoto(ctx context.Context, recipeID, userID int64, photo, thumb []byte) error {
	if err := r.assertOwner(ctx, recipeID, userID); err != nil {
		return err
	}
	sum := sha256.Sum256(photo)
	if _, err := r.db.ExecContext(ctx, `
		INSERT INTO recipe_photos (recipe_id, photo, thumb, etag) VALUES (?, ?, ?, ?)
		ON CONFLICT (recipe_id) DO UPDATE SET
			photo = excluded.photo, thumb = excluded.thumb, etag = excluded.etag,
			updated_at = CURRENT_TIMESTAMP
	`, recipeID, photo, thumb, hex.EncodeToString(sum[:8])); err != nil {
		return fmt.Errorf("set recipe photo: %w", err)
	}
	return nil
}

// DeletePhoto removes the recipe's photo, if any. Only the owner may.
func (r *Recipes) DeletePhoto(ctx context.Context, recipeID, userID int64) error {
	if err := r.assertOwner(ctx, recipeID, userID); err != nil {
		return err
	}
	if _, err := r.db.ExecContext(ctx, `DELETE FROM recipe_photos WHERE recipe_id = ?`, recipeID); err != nil {
		return fmt.Errorf("delete recipe photo: %w", err)
	}
	return nil
}

// Photo returns the recipe's photo, or its thumbnail, with its ETag. It checks
// no access: the caller has loaded the recipe for the viewer first.
func (r *Recipes) Photo(ctx context.Context, recipeID int64, thumb bool) ([]byte, string, error) {
	col := "photo"
	if thumb {
		col = "thumb"
	}
	var (
		data []byte
		etag string
	)
	err := r.db.QueryRowContext(ctx, `SELECT `+col+`, etag FROM recipe_photos WHERE recipe_id = ?`, recipeID).Scan(&data, &etag)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, "", ErrPhotoNotFound
	}
	if err != nil {
		return nil, "", fmt.Errorf("recipe photo: %w", err)
	}
	return data, etag, nil
}

// PhotoETag returns the ETag of the recipe's photo, or "" without one.
func (r *Recipes) PhotoETag(ctx context.Context, recipeID int64) (string, error) {
	var etag string
	err := r.db.QueryRowContext(ctx, `SELECT etag FROM recipe_photos WHERE recipe_id = ?`, recipeID).Scan(&etag)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("recipe photo etag: %w", err)
	}
	return etag, nil
}

// PhotoETagsByRecipe returns recipe_id → photo ETag for the recipes visible
// to viewerID (the same ones as ListVisible) that have a photo.
func (r *Recipes) PhotoETagsByRecipe(ctx context.Context, viewerID int64) (map[int64]string, error) {
	scope, args := inCatalog(r.db, "r.user_id", viewerID)
	rows, err := r.db.QueryContext(ctx, `
		SELECT p.recipe_id, p.etag
		FROM recipe_photos p
		JOIN recipes r ON r.id = p.recipe_id
		WHERE r.is_draft = 0 AND (r.user_id = ? OR (r.is_public = 1 AND `+scope+`))
	`, append([]any{viewerID}, args...)...)
	if err != nil {
		return nil, fmt.Errorf("photo etags: %w", err)
	}
	defer rows.Close()
	out := make(map[int64]string)
	for rows.Next() {
		var (
			id   int64
			etag string
		)
		if err := rows.Scan(&id, &etag); err != nil {
			return nil, fmt.Errorf("scan photo etag: %w", err)
		}
		out[id] = etag
	}
	return out, rows.Err()
}
