package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/amon2126/nutreek/internal/domain"
)

var ErrNutritionistItemNotFound = errors.New("elemento piano nutrizionista non trovato")

type NutritionistPlans struct {
	db *sql.DB
}

func NewNutritionistPlans(db *sql.DB) *NutritionistPlans {
	return &NutritionistPlans{db: db}
}

// GetOrCreatePlan restituisce il piano nutrizionista dell'utente, creandolo se non esiste.
func (n *NutritionistPlans) GetOrCreatePlan(ctx context.Context, userID int64) (*domain.NutritionistPlan, error) {
	var plan domain.NutritionistPlan
	err := n.db.QueryRowContext(ctx,
		`SELECT id, user_id, name, notes, created_at, updated_at FROM nutritionist_plans WHERE user_id = ?`,
		userID,
	).Scan(&plan.ID, &plan.UserID, &plan.Name, &plan.Notes, &plan.CreatedAt, &plan.UpdatedAt)
	if err == nil {
		return &plan, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("get nutritionist plan: %w", err)
	}

	// Crea il piano.
	res, err := n.db.ExecContext(ctx,
		`INSERT INTO nutritionist_plans (user_id) VALUES (?)`, userID,
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
	err := n.db.QueryRowContext(ctx,
		`SELECT id, user_id, name, notes, created_at, updated_at FROM nutritionist_plans WHERE id = ?`, id,
	).Scan(&plan.ID, &plan.UserID, &plan.Name, &notes, &plan.CreatedAt, &plan.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("get plan by id: %w", err)
	}
	if notes.Valid {
		plan.Notes = &notes.String
	}
	return &plan, nil
}

// LoadPlanFull carica il piano completo con tutti i tipi pasto, varianti e item.
func (n *NutritionistPlans) LoadPlanFull(ctx context.Context, userID int64) (*domain.NutritionistPlanFull, error) {
	plan, err := n.GetOrCreatePlan(ctx, userID)
	if err != nil {
		return nil, err
	}

	full := &domain.NutritionistPlanFull{NutritionistPlan: *plan}

	// Carica tipi pasto.
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

	// Carica variazioni.
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

	// Carica items per ogni variazione e risolve recipe/ingredient.
	for i, mt := range mealTypes {
		vars := varMap[mt.ID]
		for j, v := range vars {
			itemRows, err := n.db.QueryContext(ctx,
				`SELECT ni.id, ni.variation_id, ni.recipe_id, ni.ingredient_id,
				        ni.quantity, ni.quantity_unit, ni.sort_order,
				        r.name, r.servings,
				        ing.id, ing.name, ing.brand, ing.unit,
				        ing.kcal_per_100, ing.protein_per_100, ing.carbs_per_100, ing.fat_per_100
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
	var ingKcal, ingProtein, ingCarbs, ingFat sql.NullFloat64

	err := rows.Scan(
		&item.ID, &item.VariationID, &recipeID, &ingredientID,
		&item.Quantity, &item.QuantityUnit, &item.SortOrder,
		&recipeName, &recipeServings,
		&ingID, &ingName, &ingBrand, &ingUnit,
		&ingKcal, &ingProtein, &ingCarbs, &ingFat,
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
		item.Ingredient = ing
	}
	return item, nil
}

// HasVariations indica se l'utente ha almeno una variante con almeno un item
// nel proprio piano nutrizionista: requisito minimo per la generazione
// automatica del piano settimanale.
func (n *NutritionistPlans) HasVariations(ctx context.Context, userID int64) (bool, error) {
	var x int
	err := n.db.QueryRowContext(ctx, `
		SELECT 1
		FROM nutritionist_meal_items ni
		JOIN nutritionist_meal_variations nv ON nv.id = ni.variation_id
		JOIN nutritionist_meal_types nmt ON nmt.id = nv.meal_type_id
		JOIN nutritionist_plans np ON np.id = nmt.plan_id
		WHERE np.user_id = ?
		LIMIT 1`, userID).Scan(&x)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("check nutritionist variations: %w", err)
	}
	return true, nil
}

// SearchSuggestedIngredients restituisce gli ingredienti del piano nutrizionista
// che appartengono a un tipo pasto il cui nome corrisponde (case-insensitive) a mealTypeName.
func (n *NutritionistPlans) SearchSuggestedIngredients(ctx context.Context, userID int64, mealTypeName string) ([]*domain.Ingredient, error) {
	rows, err := n.db.QueryContext(ctx,
		`SELECT DISTINCT ing.id, ing.name, ing.brand, ing.unit,
		        ing.kcal_per_100, ing.protein_per_100, ing.carbs_per_100, ing.fat_per_100
		 FROM nutritionist_meal_items ni
		 JOIN nutritionist_meal_variations nv ON nv.id = ni.variation_id
		 JOIN nutritionist_meal_types nmt ON nmt.id = nv.meal_type_id
		 JOIN nutritionist_plans np ON np.id = nmt.plan_id
		 JOIN ingredients ing ON ing.id = ni.ingredient_id
		 WHERE np.user_id = ? AND lower(nmt.name) = lower(?)
		   AND ni.ingredient_id IS NOT NULL`,
		userID, mealTypeName,
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

func (n *NutritionistPlans) CreateVariation(ctx context.Context, mealTypeID int64, name *string) (int64, error) {
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

func (n *NutritionistPlans) AddItem(ctx context.Context, variationID int64, recipeID, ingredientID *int64, quantity float64, unit string) (int64, error) {
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

// EnsureMealType crea un nutritionist_meal_type con il nome dato se non esiste già
// (confronto case-insensitive). Idempotente.
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
