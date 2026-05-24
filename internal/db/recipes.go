package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/amon2126/nutreek/internal/domain"
)

// ErrRecipeNotFound e' restituito dalle Get quando la ricetta non esiste.
var ErrRecipeNotFound = errors.New("ricetta non trovata")

// ErrRecipeForbidden segnala un accesso non consentito (es. utente che
// tenta update/delete di una ricetta non sua).
var ErrRecipeForbidden = errors.New("operazione non consentita su questa ricetta")

// ErrRecipeItemNotFound = item recipe_ingredients inesistente / non appartiene.
var ErrRecipeItemNotFound = errors.New("voce ricetta non trovata")

// ErrRecipeNameTaken segnala che esiste già una ricetta dello stesso utente
// con lo stesso nome (case-insensitive). Vincolo applicato sia a livello DB
// (UNIQUE INDEX) sia in handler per messaggi leggibili.
var ErrRecipeNameTaken = errors.New("esiste già una ricetta con questo nome")

// ErrRecipeIdenticalContent segnala che il contenuto della ricetta (set di
// ingredienti+quantità) coincide con un'altra ricetta dell'utente.
var ErrRecipeIdenticalContent = errors.New("esiste già una ricetta con gli stessi ingredienti e quantità")

// Recipes e' il repository delle ricette.
type Recipes struct {
	db *sql.DB
}

func NewRecipes(db *sql.DB) *Recipes {
	return &Recipes{db: db}
}

// Create inserisce una nuova ricetta vuota (gli items si aggiungono dopo).
// Ritorna l'id assegnato. Errore ErrRecipeNameTaken se esiste già una ricetta
// con lo stesso nome (case-insensitive) per lo stesso utente.
func (r *Recipes) Create(
	ctx context.Context,
	userID int64,
	name string, description *string,
	servings int, isPublic bool,
) (int64, error) {
	if servings <= 0 {
		servings = 1
	}
	res, err := r.db.ExecContext(ctx, `
		INSERT INTO recipes (user_id, name, description, servings, is_public)
		VALUES (?, ?, ?, ?, ?)
	`, userID, strings.TrimSpace(name), description, servings, boolToInt(isPublic))
	if err != nil {
		if isUniqueRecipeNameErr(err) {
			return 0, ErrRecipeNameTaken
		}
		return 0, fmt.Errorf("insert recipe: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("last insert id: %w", err)
	}
	return id, nil
}

// Update modifica i campi base. Verifica owner: se userID non e' il
// proprietario, ritorna ErrRecipeForbidden senza toccare la riga.
func (r *Recipes) Update(
	ctx context.Context,
	id, userID int64,
	name string, description *string,
	servings int, isPublic bool,
) error {
	if servings <= 0 {
		servings = 1
	}
	res, err := r.db.ExecContext(ctx, `
		UPDATE recipes
		SET name = ?, description = ?, servings = ?, is_public = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ? AND user_id = ?
	`, strings.TrimSpace(name), description, servings, boolToInt(isPublic), id, userID)
	if err != nil {
		if isUniqueRecipeNameErr(err) {
			return ErrRecipeNameTaken
		}
		return fmt.Errorf("update recipe: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return r.notFoundOrForbidden(ctx, id, userID)
	}
	return nil
}

// Delete cancella la ricetta (gli items vanno via per ON DELETE CASCADE).
func (r *Recipes) Delete(ctx context.Context, id, userID int64) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM recipes WHERE id = ? AND user_id = ?`, id, userID)
	if err != nil {
		return fmt.Errorf("delete recipe: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return r.notFoundOrForbidden(ctx, id, userID)
	}
	return nil
}

// notFoundOrForbidden distingue 404 da 403 quando UPDATE/DELETE non tocca righe.
func (r *Recipes) notFoundOrForbidden(ctx context.Context, id, userID int64) error {
	var owner int64
	err := r.db.QueryRowContext(ctx, `SELECT user_id FROM recipes WHERE id = ?`, id).Scan(&owner)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrRecipeNotFound
	}
	if err != nil {
		return fmt.Errorf("check owner: %w", err)
	}
	if owner != userID {
		return ErrRecipeForbidden
	}
	// Owner combacia ma RowsAffected=0: non dovrebbe succedere → fallback.
	return ErrRecipeNotFound
}

// GetWithItems carica la ricetta + tutti gli item joinati con l'ingrediente.
// Per ricette altrui ritorna comunque i dati se isPublic; altrimenti
// ErrRecipeForbidden. ErrRecipeNotFound se l'id non esiste.
func (r *Recipes) GetWithItems(ctx context.Context, id, viewerID int64) (*domain.RecipeWithItems, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT r.id, r.user_id, r.name, r.description, r.servings, r.is_public,
		       r.created_at, r.updated_at, u.display_name
		FROM recipes r
		JOIN users u ON u.id = r.user_id
		WHERE r.id = ?`, id)
	var (
		out      domain.RecipeWithItems
		isPubInt int
	)
	err := row.Scan(
		&out.ID, &out.UserID, &out.Name, &out.Description, &out.Servings, &isPubInt,
		&out.CreatedAt, &out.UpdatedAt, &out.AuthorName,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrRecipeNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("scan recipe: %w", err)
	}
	out.IsPublic = isPubInt == 1
	if out.UserID != viewerID && !out.IsPublic {
		return nil, ErrRecipeForbidden
	}

	rows, err := r.db.QueryContext(ctx, `
		SELECT ri.id, ri.recipe_id, ri.ingredient_id, ri.quantity, ri.sort_order,
		       i.id, i.source, i.off_barcode, i.name, i.brand, i.unit,
		       i.kcal_per_100, i.protein_per_100, i.carbs_per_100, i.fat_per_100,
		       i.created_by, i.created_at, i.last_verified_at, i.superseded_by
		FROM recipe_ingredients ri
		JOIN ingredients i ON i.id = ri.ingredient_id
		WHERE ri.recipe_id = ?
		ORDER BY ri.sort_order, ri.id
	`, id)
	if err != nil {
		return nil, fmt.Errorf("query items: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var it domain.RecipeItemFull
		var srcStr, unitStr string
		err := rows.Scan(
			&it.ID, &it.RecipeID, &it.IngredientID, &it.Quantity, &it.SortOrder,
			&it.Ingredient.ID, &srcStr, &it.Ingredient.OFFBarcode, &it.Ingredient.Name,
			&it.Ingredient.Brand, &unitStr,
			&it.Ingredient.KcalPer100, &it.Ingredient.ProteinPer100,
			&it.Ingredient.CarbsPer100, &it.Ingredient.FatPer100,
			&it.Ingredient.CreatedBy, &it.Ingredient.CreatedAt,
			&it.Ingredient.LastVerifiedAt, &it.Ingredient.SupersededBy,
		)
		if err != nil {
			return nil, fmt.Errorf("scan item: %w", err)
		}
		it.Ingredient.Source = domain.IngredientSource(srcStr)
		it.Ingredient.Unit = domain.IngredientUnit(unitStr)
		out.Items = append(out.Items, it)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate items: %w", err)
	}
	return &out, nil
}

// ListVisible ritorna ricette visibili a viewerID: tutte le sue + le pubbliche
// di altri. Ordinate alfabetico per nome.
func (r *Recipes) ListVisible(ctx context.Context, viewerID int64) ([]*domain.Recipe, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, user_id, name, description, servings, is_public, created_at, updated_at
		FROM recipes
		WHERE user_id = ? OR is_public = 1
		ORDER BY name COLLATE NOCASE
	`, viewerID)
	if err != nil {
		return nil, fmt.Errorf("list visible recipes: %w", err)
	}
	defer rows.Close()
	out := make([]*domain.Recipe, 0, 16)
	for rows.Next() {
		var rec domain.Recipe
		var pubInt int
		err := rows.Scan(&rec.ID, &rec.UserID, &rec.Name, &rec.Description,
			&rec.Servings, &pubInt, &rec.CreatedAt, &rec.UpdatedAt)
		if err != nil {
			return nil, fmt.Errorf("scan recipe: %w", err)
		}
		rec.IsPublic = pubInt == 1
		out = append(out, &rec)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// AddItem inserisce un nuovo ingrediente nella ricetta. sortOrder default
// = max(sort_order)+1. Verifica owner.
func (r *Recipes) AddItem(
	ctx context.Context,
	recipeID, userID, ingredientID int64,
	quantity float64,
) (int64, error) {
	if err := r.assertOwner(ctx, recipeID, userID); err != nil {
		return 0, err
	}
	if quantity <= 0 {
		return 0, fmt.Errorf("quantita' deve essere > 0")
	}
	var nextOrder int
	if err := r.db.QueryRowContext(ctx,
		`SELECT COALESCE(MAX(sort_order)+1, 0) FROM recipe_ingredients WHERE recipe_id = ?`,
		recipeID,
	).Scan(&nextOrder); err != nil {
		return 0, fmt.Errorf("compute sort_order: %w", err)
	}
	res, err := r.db.ExecContext(ctx, `
		INSERT INTO recipe_ingredients (recipe_id, ingredient_id, quantity, sort_order)
		VALUES (?, ?, ?, ?)
	`, recipeID, ingredientID, quantity, nextOrder)
	if err != nil {
		return 0, fmt.Errorf("insert item: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	// touch updated_at
	if _, err := r.db.ExecContext(ctx,
		`UPDATE recipes SET updated_at = CURRENT_TIMESTAMP WHERE id = ?`, recipeID); err != nil {
		return 0, fmt.Errorf("touch recipe: %w", err)
	}
	return id, nil
}

// RemoveItem cancella un item; verifica che l'item appartenga a una ricetta
// di userID.
func (r *Recipes) RemoveItem(ctx context.Context, itemID, userID int64) error {
	res, err := r.db.ExecContext(ctx, `
		DELETE FROM recipe_ingredients
		WHERE id = ? AND recipe_id IN (SELECT id FROM recipes WHERE user_id = ?)
	`, itemID, userID)
	if err != nil {
		return fmt.Errorf("delete item: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrRecipeItemNotFound
	}
	return nil
}

func (r *Recipes) assertOwner(ctx context.Context, recipeID, userID int64) error {
	var owner int64
	err := r.db.QueryRowContext(ctx, `SELECT user_id FROM recipes WHERE id = ?`, recipeID).Scan(&owner)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrRecipeNotFound
	}
	if err != nil {
		return fmt.Errorf("assert owner: %w", err)
	}
	if owner != userID {
		return ErrRecipeForbidden
	}
	return nil
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// NameExists ritorna true se l'utente ha già una ricetta con questo nome
// (case-insensitive). excludeID = id da ignorare nel check (utile in update).
func (r *Recipes) NameExists(ctx context.Context, userID int64, name string, excludeID int64) (bool, error) {
	var n int
	err := r.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM recipes
		WHERE user_id = ? AND name = ? COLLATE NOCASE AND id != ?
	`, userID, strings.TrimSpace(name), excludeID).Scan(&n)
	if err != nil {
		return false, fmt.Errorf("name exists: %w", err)
	}
	return n > 0, nil
}

// CountMealUsages ritorna il numero di meal_items che referenziano questa
// ricetta. Usato per mostrare warning prima dell'eliminazione.
func (r *Recipes) CountMealUsages(ctx context.Context, recipeID int64) (int, error) {
	var n int
	err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM meal_items WHERE recipe_id = ?`, recipeID).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("count meal usages: %w", err)
	}
	return n, nil
}

// IdenticalContentRecipe verifica se esiste un'altra ricetta dello stesso
// utente con lo stesso identico set (ingredient_id, quantity) di ingredienti.
// Ritorna il nome di quella ricetta (o "") e nil se non c'è. excludeID = id
// da ignorare (utile per duplicate/update).
//
// Implementazione: due ricette sono "identiche" sse hanno lo stesso numero
// di item e tutti gli (ingredient_id, quantity) coincidono. Confrontiamo
// firma serializzata "id:qty;id:qty;..." su tuple ordinate.
func (r *Recipes) IdenticalContentRecipe(
	ctx context.Context,
	userID int64,
	items []domain.RecipeItem,
	excludeID int64,
) (string, error) {
	target := contentSignature(items)
	if target == "" {
		// Ricetta senza ingredienti: non consideriamo identica nessuna altra.
		return "", nil
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, name FROM recipes WHERE user_id = ? AND id != ?
	`, userID, excludeID)
	if err != nil {
		return "", fmt.Errorf("list recipes for identity: %w", err)
	}
	defer rows.Close()
	type pair struct {
		id   int64
		name string
	}
	var candidates []pair
	for rows.Next() {
		var p pair
		if err := rows.Scan(&p.id, &p.name); err != nil {
			return "", fmt.Errorf("scan candidate: %w", err)
		}
		candidates = append(candidates, p)
	}
	if err := rows.Err(); err != nil {
		return "", err
	}
	for _, c := range candidates {
		sig, err := r.signatureForRecipe(ctx, c.id)
		if err != nil {
			return "", err
		}
		if sig == target {
			return c.name, nil
		}
	}
	return "", nil
}

// signatureForRecipe legge gli item della ricetta e ritorna la firma normalizzata.
func (r *Recipes) signatureForRecipe(ctx context.Context, recipeID int64) (string, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT ingredient_id, quantity FROM recipe_ingredients WHERE recipe_id = ?
	`, recipeID)
	if err != nil {
		return "", fmt.Errorf("load items for signature: %w", err)
	}
	defer rows.Close()
	var items []domain.RecipeItem
	for rows.Next() {
		var it domain.RecipeItem
		if err := rows.Scan(&it.IngredientID, &it.Quantity); err != nil {
			return "", fmt.Errorf("scan item: %w", err)
		}
		items = append(items, it)
	}
	if err := rows.Err(); err != nil {
		return "", err
	}
	return contentSignature(items), nil
}

// contentSignature produce una firma normalizzata di una lista di item:
// tuple (ingredient_id, quantità arrotondata a 3 decimali) ordinate, concatenate.
// Due liste con stesso set di tuple → stessa firma (indipendentemente da sort_order).
func contentSignature(items []domain.RecipeItem) string {
	if len(items) == 0 {
		return ""
	}
	pairs := make([]string, 0, len(items))
	for _, it := range items {
		pairs = append(pairs, fmt.Sprintf("%d:%.3f", it.IngredientID, it.Quantity))
	}
	sort.Strings(pairs)
	return strings.Join(pairs, ";")
}

// Duplicate crea una copia profonda di una ricetta (meta + tutti i recipe_ingredients)
// con il nuovo nome specificato. Transazionale.
//
// Errori:
//   - ErrRecipeNotFound: source non esiste
//   - ErrRecipeForbidden: source non appartiene a userID e non è pubblica
//   - ErrRecipeNameTaken: nome già esistente per l'utente
func (r *Recipes) Duplicate(ctx context.Context, srcID, userID int64, newName string) (int64, error) {
	newName = strings.TrimSpace(newName)
	if newName == "" {
		return 0, errors.New("nome ricetta vuoto")
	}
	src, err := r.GetWithItems(ctx, srcID, userID)
	if err != nil {
		return 0, err
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var descPtr *string
	if src.Description != nil {
		s := *src.Description
		descPtr = &s
	}
	res, err := tx.ExecContext(ctx, `
		INSERT INTO recipes (user_id, name, description, servings, is_public)
		VALUES (?, ?, ?, ?, ?)
	`, userID, newName, descPtr, src.Servings, boolToInt(src.IsPublic))
	if err != nil {
		if isUniqueRecipeNameErr(err) {
			return 0, ErrRecipeNameTaken
		}
		return 0, fmt.Errorf("insert duplicate: %w", err)
	}
	newID, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("last insert id: %w", err)
	}
	for _, it := range src.Items {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO recipe_ingredients (recipe_id, ingredient_id, quantity, sort_order)
			VALUES (?, ?, ?, ?)
		`, newID, it.IngredientID, it.Quantity, it.SortOrder); err != nil {
			return 0, fmt.Errorf("copy item: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit duplicate: %w", err)
	}
	return newID, nil
}

// BulkDelete cancella più ricette dell'utente. Ritorna il numero di ricette
// effettivamente cancellate (le altre erano forbidden / inesistenti, ignorate).
// I meal_items collegati alle ricette cancellate vengono rimossi a cascata
// (FK ON DELETE CASCADE, vedi migrazione 0008).
func (r *Recipes) BulkDelete(ctx context.Context, ids []int64, userID int64) (int, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	placeholders := strings.TrimRight(strings.Repeat("?,", len(ids)), ",")
	args := make([]any, 0, len(ids)+1)
	for _, id := range ids {
		args = append(args, id)
	}
	args = append(args, userID)
	q := `DELETE FROM recipes WHERE id IN (` + placeholders + `) AND user_id = ?`
	res, err := r.db.ExecContext(ctx, q, args...)
	if err != nil {
		return 0, fmt.Errorf("bulk delete recipes: %w", err)
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

// UsagesByRecipe ritorna una mappa recipe_id → numero di meal_items che la
// referenziano, limitata alle ricette dell'utente. Recipe senza usi non sono
// presenti nella mappa (chiamante può assumere 0).
func (r *Recipes) UsagesByRecipe(ctx context.Context, userID int64) (map[int64]int, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT mi.recipe_id, COUNT(*) AS n
		FROM meal_items mi
		JOIN recipes r ON r.id = mi.recipe_id
		WHERE r.user_id = ? AND mi.recipe_id IS NOT NULL
		GROUP BY mi.recipe_id
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("usages by recipe: %w", err)
	}
	defer rows.Close()
	out := make(map[int64]int, 8)
	for rows.Next() {
		var id int64
		var n int
		if err := rows.Scan(&id, &n); err != nil {
			return nil, fmt.Errorf("scan usage: %w", err)
		}
		out[id] = n
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// CountMealUsagesMulti somma le occorrenze meal_items su più ricette.
func (r *Recipes) CountMealUsagesMulti(ctx context.Context, recipeIDs []int64) (int, error) {
	if len(recipeIDs) == 0 {
		return 0, nil
	}
	placeholders := strings.TrimRight(strings.Repeat("?,", len(recipeIDs)), ",")
	args := make([]any, 0, len(recipeIDs))
	for _, id := range recipeIDs {
		args = append(args, id)
	}
	var n int
	err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM meal_items WHERE recipe_id IN (`+placeholders+`)`, args...).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("count meal usages multi: %w", err)
	}
	return n, nil
}

// isUniqueRecipeNameErr rileva la violazione del vincolo uniq_recipes_user_name.
// modernc.org/sqlite ritorna l'errore come testo contenente il nome dell'indice.
func isUniqueRecipeNameErr(err error) bool {
	if err == nil {
		return false
	}
	s := err.Error()
	return strings.Contains(s, "uniq_recipes_user_name") ||
		(strings.Contains(s, "UNIQUE constraint failed") && strings.Contains(s, "recipes"))
}
