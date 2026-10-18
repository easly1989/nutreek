package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/amon2126/nutreek/internal/domain"
)

var ErrNutritionistItemNotFound = errors.New("elemento piano nutrizionista non trovato")

// ErrNutritionistPlanNotFound is returned when the requested plan does not
// exist or does not belong to the given user.
var ErrNutritionistPlanNotFound = errors.New("piano nutrizionista non trovato")

type NutritionistPlans struct {
	db *sql.DB
}

func NewNutritionistPlans(db *sql.DB) *NutritionistPlans {
	return &NutritionistPlans{db: db}
}

// GetOrCreateActivePlan returns the user's active plan, creating it when it
// does not exist. Every user always has exactly one active plan.
func (n *NutritionistPlans) GetOrCreateActivePlan(ctx context.Context, userID int64) (*domain.NutritionistPlan, error) {
	var plan domain.NutritionistPlan
	var notes sql.NullString
	var isActive int
	err := n.db.QueryRowContext(ctx,
		`SELECT id, user_id, name, notes, is_active, created_at, updated_at
		 FROM nutritionist_plans WHERE user_id = ? AND is_active = 1`,
		userID,
	).Scan(&plan.ID, &plan.UserID, &plan.Name, &notes, &isActive, &plan.CreatedAt, &plan.UpdatedAt)
	if err == nil {
		if notes.Valid {
			plan.Notes = &notes.String
		}
		plan.IsActive = isActive == 1
		return &plan, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("get nutritionist plan: %w", err)
	}

	// Create the active plan.
	res, err := n.db.ExecContext(ctx,
		`INSERT INTO nutritionist_plans (user_id, is_active) VALUES (?, 1)`, userID,
	)
	if err != nil {
		return nil, fmt.Errorf("create nutritionist plan: %w", err)
	}
	id, _ := res.LastInsertId()
	return n.getPlanByID(ctx, id)
}

func (n *NutritionistPlans) getPlanByID(ctx context.Context, id int64) (*domain.NutritionistPlan, error) {
	var plan domain.NutritionistPlan
	var notes sql.NullString
	var isActive int
	err := n.db.QueryRowContext(ctx,
		`SELECT id, user_id, name, notes, is_active, created_at, updated_at
		 FROM nutritionist_plans WHERE id = ?`, id,
	).Scan(&plan.ID, &plan.UserID, &plan.Name, &notes, &isActive, &plan.CreatedAt, &plan.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNutritionistPlanNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get plan by id: %w", err)
	}
	if notes.Valid {
		plan.Notes = &notes.String
	}
	plan.IsActive = isActive == 1
	return &plan, nil
}

// CreatePlan creates a new inactive plan in the user's library.
func (n *NutritionistPlans) CreatePlan(ctx context.Context, userID int64, name string) (int64, error) {
	res, err := n.db.ExecContext(ctx,
		`INSERT INTO nutritionist_plans (user_id, name, is_active) VALUES (?, ?, 0)`,
		userID, name,
	)
	if err != nil {
		return 0, fmt.Errorf("create library plan: %w", err)
	}
	id, _ := res.LastInsertId()
	return id, nil
}

// ListPlans returns every plan of the user (the active one first).
func (n *NutritionistPlans) ListPlans(ctx context.Context, userID int64) ([]domain.NutritionistPlan, error) {
	rows, err := n.db.QueryContext(ctx,
		`SELECT id, user_id, name, notes, is_active, created_at, updated_at
		 FROM nutritionist_plans WHERE user_id = ?
		 ORDER BY is_active DESC, created_at, id`, userID,
	)
	if err != nil {
		return nil, fmt.Errorf("list plans: %w", err)
	}
	defer rows.Close()
	var out []domain.NutritionistPlan
	for rows.Next() {
		var plan domain.NutritionistPlan
		var notes sql.NullString
		var isActive int
		if err := rows.Scan(&plan.ID, &plan.UserID, &plan.Name, &notes, &isActive,
			&plan.CreatedAt, &plan.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan plan: %w", err)
		}
		if notes.Valid {
			plan.Notes = &notes.String
		}
		plan.IsActive = isActive == 1
		out = append(out, plan)
	}
	return out, rows.Err()
}

// GetPlanOwned returns the plan only when it belongs to the user,
// otherwise ErrNutritionistPlanNotFound.
func (n *NutritionistPlans) GetPlanOwned(ctx context.Context, planID, userID int64) (*domain.NutritionistPlan, error) {
	plan, err := n.getPlanByID(ctx, planID)
	if err != nil {
		return nil, err
	}
	if plan.UserID != userID {
		return nil, ErrNutritionistPlanNotFound
	}
	return plan, nil
}

// SetActivePlan makes the given plan active (it must belong to the user) and
// deactivates every other one. Transactional: the "one active plan"
// invariant is never broken.
func (n *NutritionistPlans) SetActivePlan(ctx context.Context, userID, planID int64) error {
	tx, err := n.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin set active plan: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var owner int64
	err = tx.QueryRowContext(ctx,
		`SELECT user_id FROM nutritionist_plans WHERE id = ?`, planID).Scan(&owner)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && owner != userID) {
		return ErrNutritionistPlanNotFound
	}
	if err != nil {
		return fmt.Errorf("check plan owner: %w", err)
	}

	if _, err := tx.ExecContext(ctx,
		`UPDATE nutritionist_plans SET is_active = 0 WHERE user_id = ?`, userID); err != nil {
		return fmt.Errorf("deactivate plans: %w", err)
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE nutritionist_plans SET is_active = 1, updated_at = CURRENT_TIMESTAMP WHERE id = ?`, planID); err != nil {
		return fmt.Errorf("activate plan: %w", err)
	}
	return tx.Commit()
}

// RenamePlan renames one of the user's plans.
func (n *NutritionistPlans) RenamePlan(ctx context.Context, planID, userID int64, name string) error {
	res, err := n.db.ExecContext(ctx,
		`UPDATE nutritionist_plans SET name = ?, updated_at = CURRENT_TIMESTAMP
		 WHERE id = ? AND user_id = ?`, name, planID, userID)
	if err != nil {
		return fmt.Errorf("rename plan: %w", err)
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		return ErrNutritionistPlanNotFound
	}
	return nil
}

// DeletePlan deletes one of the user's inactive plans (the active plan cannot
// be deleted: the invariant always needs an active plan).
func (n *NutritionistPlans) DeletePlan(ctx context.Context, planID, userID int64) error {
	res, err := n.db.ExecContext(ctx,
		`DELETE FROM nutritionist_plans WHERE id = ? AND user_id = ? AND is_active = 0`,
		planID, userID)
	if err != nil {
		return fmt.Errorf("delete plan: %w", err)
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		return ErrNutritionistPlanNotFound
	}
	return nil
}

// LoadPlanFull loads the user's active plan with its meal types, variants and
// items, creating it when it does not exist.
func (n *NutritionistPlans) LoadPlanFull(ctx context.Context, userID int64) (*domain.NutritionistPlanFull, error) {
	plan, err := n.GetOrCreateActivePlan(ctx, userID)
	if err != nil {
		return nil, err
	}
	return n.loadFull(ctx, plan)
}

// LoadPlanFullByID loads a specific plan by id, with its contents.
// It does not check ownership: the caller must authorize the access (e.g.
// the owning coach, or a client with the plan assigned).
func (n *NutritionistPlans) LoadPlanFullByID(ctx context.Context, planID int64) (*domain.NutritionistPlanFull, error) {
	plan, err := n.getPlanByID(ctx, planID)
	if err != nil {
		return nil, err
	}
	return n.loadFull(ctx, plan)
}

func (n *NutritionistPlans) loadFull(ctx context.Context, plan *domain.NutritionistPlan) (*domain.NutritionistPlanFull, error) {
	full := &domain.NutritionistPlanFull{NutritionistPlan: *plan}

	// Load the meal types.
	typeRows, err := n.db.QueryContext(ctx,
		`SELECT id, plan_id, name, sort_order FROM nutritionist_meal_types WHERE plan_id = ? ORDER BY sort_order, id`,
		plan.ID,
	)
	if err != nil {
		return nil, fmt.Errorf("load meal types: %w", err)
	}
	defer typeRows.Close()

	var mealTypes []domain.NutritionistMealTypeFull
	var typeIDs []int64
	for typeRows.Next() {
		var mt domain.NutritionistMealType
		if err := typeRows.Scan(&mt.ID, &mt.PlanID, &mt.Name, &mt.SortOrder); err != nil {
			return nil, fmt.Errorf("scan meal type: %w", err)
		}
		mealTypes = append(mealTypes, domain.NutritionistMealTypeFull{NutritionistMealType: mt})
		typeIDs = append(typeIDs, mt.ID)
	}
	if err := typeRows.Close(); err != nil {
		return nil, err
	}

	if len(typeIDs) == 0 {
		full.MealTypes = mealTypes
		return full, nil
	}

	// Load the variants.
	varMap := make(map[int64][]domain.NutritionistVariationFull)
	for _, tid := range typeIDs {
		varRows, err := n.db.QueryContext(ctx,
			`SELECT id, meal_type_id, name, sort_order FROM nutritionist_meal_variations
			 WHERE meal_type_id = ? ORDER BY sort_order, id`, tid,
		)
		if err != nil {
			return nil, fmt.Errorf("load variations: %w", err)
		}
		for varRows.Next() {
			var v domain.NutritionistVariation
			var name sql.NullString
			if err := varRows.Scan(&v.ID, &v.MealTypeID, &name, &v.SortOrder); err != nil {
				varRows.Close()
				return nil, fmt.Errorf("scan variation: %w", err)
			}
			if name.Valid {
				v.Name = &name.String
			}
			varMap[tid] = append(varMap[tid], domain.NutritionistVariationFull{NutritionistVariation: v})
		}
		varRows.Close()
	}

	// Load the items of each variant and resolve recipe/ingredient.
	for i, mt := range mealTypes {
		vars := varMap[mt.ID]
		for j, v := range vars {
			itemRows, err := n.db.QueryContext(ctx,
				`SELECT ni.id, ni.variation_id, ni.recipe_id, ni.ingredient_id,
				        ni.quantity, ni.quantity_unit, ni.sort_order,
				        r.name, r.servings,
				        ing.id, ing.name, ing.brand, ing.unit,
				        ing.kcal_per_100, ing.protein_per_100, ing.carbs_per_100, ing.fat_per_100,
				        ing.unit_weight_g
				 FROM nutritionist_meal_items ni
				 LEFT JOIN recipes r ON r.id = ni.recipe_id
				 LEFT JOIN ingredients ing ON ing.id = ni.ingredient_id
				 WHERE ni.variation_id = ? ORDER BY ni.sort_order, ni.id`,
				v.ID,
			)
			if err != nil {
				return nil, fmt.Errorf("load items: %w", err)
			}
			for itemRows.Next() {
				item, err := scanNutritionistItem(itemRows)
				if err != nil {
					itemRows.Close()
					return nil, err
				}
				vars[j].Items = append(vars[j].Items, item)
			}
			itemRows.Close()
		}
		mealTypes[i].Variations = vars
	}

	full.MealTypes = mealTypes
	return full, nil
}

func scanNutritionistItem(rows *sql.Rows) (domain.NutritionistItem, error) {
	var item domain.NutritionistItem
	var recipeID, ingredientID sql.NullInt64
	var recipeName sql.NullString
	var recipeServings sql.NullInt64
	var ingID sql.NullInt64
	var ingName, ingBrand, ingUnit sql.NullString
	var ingKcal, ingProtein, ingCarbs, ingFat, ingUnitWeight sql.NullFloat64

	err := rows.Scan(
		&item.ID, &item.VariationID, &recipeID, &ingredientID,
		&item.Quantity, &item.QuantityUnit, &item.SortOrder,
		&recipeName, &recipeServings,
		&ingID, &ingName, &ingBrand, &ingUnit,
		&ingKcal, &ingProtein, &ingCarbs, &ingFat,
		&ingUnitWeight,
	)
	if err != nil {
		return item, fmt.Errorf("scan nutritionist item: %w", err)
	}

	if recipeID.Valid {
		item.RecipeID = &recipeID.Int64
		item.Recipe = &domain.Recipe{
			ID:       recipeID.Int64,
			Name:     recipeName.String,
			Servings: int(recipeServings.Int64),
		}
	}
	if ingredientID.Valid {
		item.IngredientID = &ingredientID.Int64
		ing := &domain.Ingredient{
			ID:            ingID.Int64,
			Name:          ingName.String,
			Unit:          domain.IngredientUnit(ingUnit.String),
			KcalPer100:    ingKcal.Float64,
			ProteinPer100: ingProtein.Float64,
			CarbsPer100:   ingCarbs.Float64,
			FatPer100:     ingFat.Float64,
		}
		if ingBrand.Valid {
			ing.Brand = &ingBrand.String
		}
		if ingUnitWeight.Valid {
			w := ingUnitWeight.Float64
			ing.UnitWeightG = &w
		}
		item.Ingredient = ing
	}
	return item, nil
}

// effectivePlans matches the plan np a user draws from, as
// service.PlannerService.EffectivePlanFull: the one their coach assigned,
// else their active one (#186). It takes the user's id three times.
const effectivePlans = `(np.id = (SELECT assigned_plan_id FROM coach_clients WHERE client_id = ?)
	OR (np.user_id = ? AND np.is_active = 1 AND NOT EXISTS (SELECT 1 FROM coach_clients WHERE client_id = ? AND assigned_plan_id IS NOT NULL)))`

// HasVariations reports whether the user has at least one variant with at
// least one item in their active nutrition plan, or the one their coach assigned:
// the minimum requirement for automatic weekly plan generation.
func (n *NutritionistPlans) HasVariations(ctx context.Context, userID int64) (bool, error) {
	var x int
	err := n.db.QueryRowContext(ctx, `
		SELECT 1
		FROM nutritionist_meal_items ni
		JOIN nutritionist_meal_variations nv ON nv.id = ni.variation_id
		JOIN nutritionist_meal_types nmt ON nmt.id = nv.meal_type_id
		JOIN nutritionist_plans np ON np.id = nmt.plan_id
		WHERE `+effectivePlans+`
		LIMIT 1`, userID, userID, userID).Scan(&x)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("check nutritionist variations: %w", err)
	}
	return true, nil
}

// SearchSuggestedIngredients returns the ingredients of the user's active
// nutrition plan, or the one their coach assigned, that belong to a meal type whose name
// matches mealTypeName (case-insensitive).
func (n *NutritionistPlans) SearchSuggestedIngredients(ctx context.Context, userID int64, mealTypeName string) ([]*domain.Ingredient, error) {
	rows, err := n.db.QueryContext(ctx,
		`SELECT DISTINCT ing.id, ing.name, ing.brand, ing.unit,
		        ing.kcal_per_100, ing.protein_per_100, ing.carbs_per_100, ing.fat_per_100
		 FROM nutritionist_meal_items ni
		 JOIN nutritionist_meal_variations nv ON nv.id = ni.variation_id
		 JOIN nutritionist_meal_types nmt ON nmt.id = nv.meal_type_id
		 JOIN nutritionist_plans np ON np.id = nmt.plan_id
		 JOIN ingredients ing ON ing.id = ni.ingredient_id
		 WHERE `+effectivePlans+` AND lower(nmt.name) = lower(?)
		   AND ni.ingredient_id IS NOT NULL`,
		userID, userID, userID, mealTypeName,
	)
	if err != nil {
		return nil, fmt.Errorf("search suggested ingredients: %w", err)
	}
	defer rows.Close()

	var result []*domain.Ingredient
	for rows.Next() {
		ing := &domain.Ingredient{}
		var brand sql.NullString
		if err := rows.Scan(&ing.ID, &ing.Name, &brand, &ing.Unit,
			&ing.KcalPer100, &ing.ProteinPer100, &ing.CarbsPer100, &ing.FatPer100); err != nil {
			return nil, fmt.Errorf("scan suggested ingredient: %w", err)
		}
		if brand.Valid {
			ing.Brand = &brand.String
		}
		result = append(result, ing)
	}
	return result, rows.Err()
}

func (n *NutritionistPlans) CreateMealType(ctx context.Context, planID int64, name string) (int64, error) {
	var maxOrder int
	_ = n.db.QueryRowContext(ctx,
		`SELECT COALESCE(MAX(sort_order), -1) FROM nutritionist_meal_types WHERE plan_id = ?`, planID,
	).Scan(&maxOrder)

	res, err := n.db.ExecContext(ctx,
		`INSERT INTO nutritionist_meal_types (plan_id, name, sort_order) VALUES (?, ?, ?)`,
		planID, name, maxOrder+1,
	)
	if err != nil {
		return 0, fmt.Errorf("create meal type: %w", err)
	}
	id, _ := res.LastInsertId()
	return id, nil
}

func (n *NutritionistPlans) DeleteMealType(ctx context.Context, id, planID int64) error {
	res, err := n.db.ExecContext(ctx,
		`DELETE FROM nutritionist_meal_types WHERE id = ? AND plan_id = ?`, id, planID,
	)
	if err != nil {
		return fmt.Errorf("delete meal type: %w", err)
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		return ErrNutritionistItemNotFound
	}
	return nil
}

// CreateVariation adds a variant to the meal type mealTypeID, which must be
// in plan planID: otherwise ErrNutritionistItemNotFound.
func (n *NutritionistPlans) CreateVariation(ctx context.Context, planID, mealTypeID int64, name *string) (int64, error) {
	var ok int
	err := n.db.QueryRowContext(ctx,
		`SELECT 1 FROM nutritionist_meal_types WHERE id = ? AND plan_id = ?`, mealTypeID, planID,
	).Scan(&ok)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrNutritionistItemNotFound
	}
	if err != nil {
		return 0, fmt.Errorf("meal type owner: %w", err)
	}

	var maxOrder int
	_ = n.db.QueryRowContext(ctx,
		`SELECT COALESCE(MAX(sort_order), -1) FROM nutritionist_meal_variations WHERE meal_type_id = ?`, mealTypeID,
	).Scan(&maxOrder)

	res, err := n.db.ExecContext(ctx,
		`INSERT INTO nutritionist_meal_variations (meal_type_id, name, sort_order) VALUES (?, ?, ?)`,
		mealTypeID, name, maxOrder+1,
	)
	if err != nil {
		return 0, fmt.Errorf("create variation: %w", err)
	}
	id, _ := res.LastInsertId()
	return id, nil
}

func (n *NutritionistPlans) DeleteVariation(ctx context.Context, id int64, planID int64) error {
	res, err := n.db.ExecContext(ctx,
		`DELETE FROM nutritionist_meal_variations
		 WHERE id = ? AND meal_type_id IN (SELECT id FROM nutritionist_meal_types WHERE plan_id = ?)`,
		id, planID,
	)
	if err != nil {
		return fmt.Errorf("delete variation: %w", err)
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		return ErrNutritionistItemNotFound
	}
	return nil
}

// AddItem adds an item to the variant variationID, which must be in plan
// planID (otherwise ErrNutritionistItemNotFound). A recipe must be visible
// to the plan's owner (otherwise ErrRecipeNotFound).
func (n *NutritionistPlans) AddItem(ctx context.Context, planID, variationID int64, recipeID, ingredientID *int64, quantity float64, unit string) (int64, error) {
	var owner int64
	err := n.db.QueryRowContext(ctx, `
		SELECT p.user_id
		FROM nutritionist_meal_variations nv
		JOIN nutritionist_meal_types nmt ON nmt.id = nv.meal_type_id
		JOIN nutritionist_plans p ON p.id = nmt.plan_id
		WHERE nv.id = ? AND p.id = ?`, variationID, planID,
	).Scan(&owner)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrNutritionistItemNotFound
	}
	if err != nil {
		return 0, fmt.Errorf("variation owner: %w", err)
	}
	if recipeID != nil {
		if err := recipeVisibleTo(ctx, n.db, *recipeID, owner); err != nil {
			return 0, err
		}
	}
	if ingredientID != nil {
		if err := ingredientVisibleTo(ctx, n.db, *ingredientID, owner); err != nil {
			return 0, err
		}
	}

	var maxOrder int
	_ = n.db.QueryRowContext(ctx,
		`SELECT COALESCE(MAX(sort_order), -1) FROM nutritionist_meal_items WHERE variation_id = ?`, variationID,
	).Scan(&maxOrder)

	res, err := n.db.ExecContext(ctx,
		`INSERT INTO nutritionist_meal_items (variation_id, recipe_id, ingredient_id, quantity, quantity_unit, sort_order)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		variationID, recipeID, ingredientID, quantity, unit, maxOrder+1,
	)
	if err != nil {
		return 0, fmt.Errorf("add nutritionist item: %w", err)
	}
	id, _ := res.LastInsertId()
	return id, nil
}

func (n *NutritionistPlans) DeleteItem(ctx context.Context, id, planID int64) error {
	res, err := n.db.ExecContext(ctx,
		`DELETE FROM nutritionist_meal_items
		 WHERE id = ?
		   AND variation_id IN (
		     SELECT nv.id FROM nutritionist_meal_variations nv
		     JOIN nutritionist_meal_types nmt ON nmt.id = nv.meal_type_id
		     WHERE nmt.plan_id = ?
		   )`,
		id, planID,
	)
	if err != nil {
		return fmt.Errorf("delete nutritionist item: %w", err)
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		return ErrNutritionistItemNotFound
	}
	return nil
}

// EnsureMealType creates a nutritionist_meal_type with the given name when it
// does not exist yet (case-insensitive comparison). Idempotent.
func (n *NutritionistPlans) EnsureMealType(ctx context.Context, planID int64, name string) error {
	_, err := n.db.ExecContext(ctx, `
		INSERT INTO nutritionist_meal_types (plan_id, name, sort_order)
		SELECT ?, ?,
			COALESCE((SELECT MAX(sort_order) FROM nutritionist_meal_types WHERE plan_id=?), 0) + 1
		WHERE NOT EXISTS (
			SELECT 1 FROM nutritionist_meal_types WHERE plan_id=? AND lower(name)=lower(?)
		)`, planID, name, planID, planID, name)
	return err
}
