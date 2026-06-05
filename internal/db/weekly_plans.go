package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/amon2126/nutreek/internal/domain"
)

// ErrPlanNotFound = piano inesistente.
var ErrPlanNotFound = errors.New("piano settimanale non trovato")

// ErrMealNotFound / ErrMealItemNotFound: stessa logica di RecipeItem.
var (
	ErrMealNotFound     = errors.New("pasto non trovato")
	ErrMealItemNotFound = errors.New("voce di pasto non trovata")
)

// ErrPlanNotEmpty / ErrSourcePlanEmpty per CopyFrom: rispettivamente il
// piano destinazione contiene gia' meals (refuse il copy per evitare
// merge ambigui) o il piano sorgente e' vuoto (niente da copiare).
var (
	ErrPlanNotEmpty    = errors.New("piano destinazione gia' popolato")
	ErrSourcePlanEmpty = errors.New("piano sorgente vuoto")
)

// WeeklyPlans e' il repo dei piani settimanali.
type WeeklyPlans struct {
	db *sql.DB
}

func NewWeeklyPlans(db *sql.DB) *WeeklyPlans {
	return &WeeklyPlans{db: db}
}

// MondayOf normalizza una data al lunedi' della stessa settimana (ISO).
func MondayOf(t time.Time) time.Time {
	wd := int(t.Weekday()) // domenica=0, lunedi'=1
	// Trasformo a 0=lun..6=dom:
	off := (wd + 6) % 7
	return time.Date(t.Year(), t.Month(), t.Day()-off, 0, 0, 0, 0, t.Location())
}

// GetOrCreateForWeek ritorna il piano per (user, weekStart). Se non esiste,
// lo crea (idempotente). weekStart deve essere un lunedi'.
func (r *WeeklyPlans) GetOrCreateForWeek(ctx context.Context, userID int64, weekStart time.Time) (*domain.WeeklyPlan, error) {
	weekStart = MondayOf(weekStart)
	dateStr := weekStart.Format("2006-01-02")

	// INSERT OR IGNORE per evitare race su unique(user, week_start).
	if _, err := r.db.ExecContext(ctx,
		`INSERT OR IGNORE INTO weekly_plans (user_id, week_start) VALUES (?, ?)`,
		userID, dateStr,
	); err != nil {
		return nil, fmt.Errorf("upsert plan: %w", err)
	}

	row := r.db.QueryRowContext(ctx,
		`SELECT id, user_id, week_start, template_id
		 FROM weekly_plans WHERE user_id = ? AND week_start = ?`,
		userID, dateStr,
	)
	var (
		p          domain.WeeklyPlan
		dateScan   string
	)
	if err := row.Scan(&p.ID, &p.UserID, &dateScan, &p.TemplateID); err != nil {
		return nil, fmt.Errorf("read plan: %w", err)
	}
	if t, err := time.Parse("2006-01-02", strings.Split(dateScan, "T")[0]); err == nil {
		p.WeekStart = t
	} else {
		p.WeekStart = weekStart
	}
	return &p, nil
}

// HasMeals indica se il piano ha gia' almeno un meal (per evitare double apply).
func (r *WeeklyPlans) HasMeals(ctx context.Context, planID int64) (bool, error) {
	var n int
	err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM meals WHERE weekly_plan_id = ?`, planID,
	).Scan(&n)
	if err != nil {
		return false, fmt.Errorf("count meals: %w", err)
	}
	return n > 0, nil
}

// ApplyTemplate espande gli slot del template in righe `meals`. Slot con
// day_of_week=-1 vengono espansi a 7 righe (una per giorno). Esegue in
// transazione. Se il piano gia' contiene meals, ritorna senza fare niente
// (idempotente lato chiamante: usare HasMeals per decidere prima).
func (r *WeeklyPlans) ApplyTemplate(ctx context.Context, planID, userID, templateID int64) error {
	// Verifica ownership del piano + del template.
	var pUser int64
	err := r.db.QueryRowContext(ctx, `SELECT user_id FROM weekly_plans WHERE id = ?`, planID).Scan(&pUser)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrPlanNotFound
	}
	if err != nil {
		return fmt.Errorf("plan owner: %w", err)
	}
	if pUser != userID {
		return ErrMealTemplateForbidden
	}
	var tUser int64
	err = r.db.QueryRowContext(ctx, `SELECT user_id FROM meal_templates WHERE id = ?`, templateID).Scan(&tUser)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrMealTemplateNotFound
	}
	if err != nil {
		return fmt.Errorf("template owner: %w", err)
	}
	if tUser != userID {
		return ErrMealTemplateForbidden
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// Tag the plan with the template id.
	if _, err := tx.ExecContext(ctx,
		`UPDATE weekly_plans SET template_id = ? WHERE id = ?`, templateID, planID,
	); err != nil {
		return fmt.Errorf("tag plan: %w", err)
	}

	// Carica le slot.
	rows, err := tx.QueryContext(ctx,
		`SELECT day_of_week, name, sort_order
		 FROM meal_template_slots WHERE template_id = ?
		 ORDER BY day_of_week, sort_order`, templateID,
	)
	if err != nil {
		return fmt.Errorf("load slots: %w", err)
	}
	type slot struct {
		Day       int
		Name      string
		SortOrder int
	}
	var slots []slot
	for rows.Next() {
		var s slot
		if err := rows.Scan(&s.Day, &s.Name, &s.SortOrder); err != nil {
			rows.Close()
			return fmt.Errorf("scan slot: %w", err)
		}
		slots = append(slots, s)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}

	// Espandi e inserisci. day=-1 → loop 0..6.
	stmt, err := tx.PrepareContext(ctx,
		`INSERT INTO meals (weekly_plan_id, day_of_week, name, sort_order) VALUES (?, ?, ?, ?)`,
	)
	if err != nil {
		return fmt.Errorf("prepare insert meal: %w", err)
	}
	defer stmt.Close()
	for _, s := range slots {
		if s.Day == domain.DayOfWeekEvery {
			for d := 0; d < 7; d++ {
				if _, err := stmt.ExecContext(ctx, planID, d, s.Name, s.SortOrder); err != nil {
					return fmt.Errorf("insert meal: %w", err)
				}
			}
		} else {
			if _, err := stmt.ExecContext(ctx, planID, s.Day, s.Name, s.SortOrder); err != nil {
				return fmt.Errorf("insert meal: %w", err)
			}
		}
	}
	return tx.Commit()
}

// CopyFrom clona meals e meal_items dal piano sorgente al piano destinazione,
// in transazione. Verifica ownership di entrambi (devono essere dello stesso
// userID), rifiuta se dst contiene gia' meals (ErrPlanNotEmpty) o se src e'
// vuoto (ErrSourcePlanEmpty). NON copia righe dalla tabella meal_cheat_items
// (introdotta in 0004) — gli sgarri sono per definizione contestuali alla
// settimana in cui sono avvenuti, non vanno propagati.
func (r *WeeklyPlans) CopyFrom(ctx context.Context, srcPlanID, dstPlanID, userID int64) error {
	if srcPlanID == dstPlanID {
		return fmt.Errorf("src e dst coincidono")
	}
	// Ownership src.
	var srcUser int64
	err := r.db.QueryRowContext(ctx, `SELECT user_id FROM weekly_plans WHERE id = ?`, srcPlanID).Scan(&srcUser)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrPlanNotFound
	}
	if err != nil {
		return fmt.Errorf("src plan owner: %w", err)
	}
	if srcUser != userID {
		return ErrPlanNotFound
	}
	// Ownership dst.
	var dstUser int64
	err = r.db.QueryRowContext(ctx, `SELECT user_id FROM weekly_plans WHERE id = ?`, dstPlanID).Scan(&dstUser)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrPlanNotFound
	}
	if err != nil {
		return fmt.Errorf("dst plan owner: %w", err)
	}
	if dstUser != userID {
		return ErrPlanNotFound
	}
	// Refuse se dst gia' popolato (analogo a ApplyTemplate).
	dstHas, err := r.HasMeals(ctx, dstPlanID)
	if err != nil {
		return err
	}
	if dstHas {
		return ErrPlanNotEmpty
	}
	srcHas, err := r.HasMeals(ctx, srcPlanID)
	if err != nil {
		return err
	}
	if !srcHas {
		return ErrSourcePlanEmpty
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// 1. Copia i meals e mantiene una mappa srcMealID -> dstMealID.
	mealRows, err := tx.QueryContext(ctx,
		`SELECT id, day_of_week, name, sort_order
		 FROM meals WHERE weekly_plan_id = ?
		 ORDER BY id`, srcPlanID)
	if err != nil {
		return fmt.Errorf("load src meals: %w", err)
	}
	type srcMeal struct {
		ID        int64
		Day       int
		Name      string
		SortOrder int
	}
	var meals []srcMeal
	for mealRows.Next() {
		var m srcMeal
		if err := mealRows.Scan(&m.ID, &m.Day, &m.Name, &m.SortOrder); err != nil {
			mealRows.Close()
			return fmt.Errorf("scan src meal: %w", err)
		}
		meals = append(meals, m)
	}
	mealRows.Close()
	if err := mealRows.Err(); err != nil {
		return err
	}

	insMeal, err := tx.PrepareContext(ctx,
		`INSERT INTO meals (weekly_plan_id, day_of_week, name, sort_order) VALUES (?, ?, ?, ?)`)
	if err != nil {
		return fmt.Errorf("prepare insert meal: %w", err)
	}
	defer insMeal.Close()
	idMap := make(map[int64]int64, len(meals))
	for _, m := range meals {
		res, err := insMeal.ExecContext(ctx, dstPlanID, m.Day, m.Name, m.SortOrder)
		if err != nil {
			return fmt.Errorf("insert dst meal: %w", err)
		}
		newID, err := res.LastInsertId()
		if err != nil {
			return fmt.Errorf("lastinsertid meal: %w", err)
		}
		idMap[m.ID] = newID
	}

	// 2. Copia i meal_items per ogni meal sorgente.
	if len(meals) > 0 {
		insItem, err := tx.PrepareContext(ctx,
			`INSERT INTO meal_items (meal_id, recipe_id, ingredient_id, quantity, quantity_unit, sort_order) VALUES (?, ?, ?, ?, ?, ?)`)
		if err != nil {
			return fmt.Errorf("prepare insert item: %w", err)
		}
		defer insItem.Close()
		// Una singola query batch per tutti gli items dei meals sorgente.
		srcIDs := make([]any, 0, len(meals))
		for _, m := range meals {
			srcIDs = append(srcIDs, m.ID)
		}
		ph := strings.Repeat("?,", len(srcIDs)-1) + "?"
		itemRows, err := tx.QueryContext(ctx,
			`SELECT id, meal_id, recipe_id, ingredient_id, quantity, sort_order, quantity_unit
			 FROM meal_items WHERE meal_id IN (`+ph+`)
			 ORDER BY meal_id, sort_order, id`, srcIDs...)
		if err != nil {
			return fmt.Errorf("load src items: %w", err)
		}
		type srcItem struct {
			ID           int64
			MealID       int64
			RecipeID     *int64
			IngID        *int64
			Quantity     float64
			SortOrder    int
			QuantityUnit string
		}
		var items []srcItem
		for itemRows.Next() {
			var it srcItem
			if err := itemRows.Scan(&it.ID, &it.MealID, &it.RecipeID, &it.IngID, &it.Quantity, &it.SortOrder, &it.QuantityUnit); err != nil {
				itemRows.Close()
				return fmt.Errorf("scan src item: %w", err)
			}
			items = append(items, it)
		}
		itemRows.Close()
		if err := itemRows.Err(); err != nil {
			return err
		}
		// Mappa srcItemID → dstItemID per copiare le personalizzazioni.
		itemIDMap := make(map[int64]int64, len(items))
		for _, srcIt := range items {
			dstMealID, ok := idMap[srcIt.MealID]
			if !ok {
				continue
			}
			res, err := insItem.ExecContext(ctx, dstMealID, srcIt.RecipeID, srcIt.IngID, srcIt.Quantity, srcIt.QuantityUnit, srcIt.SortOrder)
			if err != nil {
				return fmt.Errorf("insert dst item: %w", err)
			}
			newItemID, err := res.LastInsertId()
			if err != nil {
				return fmt.Errorf("lastinsertid item: %w", err)
			}
			itemIDMap[srcIt.ID] = newItemID
		}

		// Copia le personalizzazioni seguendo la mappa srcItemID → dstItemID.
		if len(itemIDMap) > 0 {
			srcItemIDs := make([]any, 0, len(itemIDMap))
			for srcID := range itemIDMap {
				srcItemIDs = append(srcItemIDs, srcID)
			}
			ph := strings.Repeat("?,", len(srcItemIDs)-1) + "?"
			custRows, err := tx.QueryContext(ctx,
				`SELECT meal_item_id, ingredient_id, quantity, sort_order
				 FROM meal_item_customizations WHERE meal_item_id IN (`+ph+`)
				 ORDER BY meal_item_id, sort_order, id`, srcItemIDs...)
			if err != nil {
				return fmt.Errorf("load src customizations: %w", err)
			}
			type custRow struct {
				SrcItemID    int64
				IngredientID int64
				Quantity     float64
				SortOrder    int
			}
			var custs []custRow
			for custRows.Next() {
				var c custRow
				if err := custRows.Scan(&c.SrcItemID, &c.IngredientID, &c.Quantity, &c.SortOrder); err != nil {
					custRows.Close()
					return fmt.Errorf("scan src customization: %w", err)
				}
				custs = append(custs, c)
			}
			custRows.Close()
			if err := custRows.Err(); err != nil {
				return err
			}
			if len(custs) > 0 {
				insCust, err := tx.PrepareContext(ctx,
					`INSERT INTO meal_item_customizations (meal_item_id, ingredient_id, quantity, sort_order)
					 VALUES (?, ?, ?, ?)`)
				if err != nil {
					return fmt.Errorf("prepare insert customization: %w", err)
				}
				defer insCust.Close()
				for _, c := range custs {
					dstItemID, ok := itemIDMap[c.SrcItemID]
					if !ok {
						continue
					}
					if _, err := insCust.ExecContext(ctx, dstItemID, c.IngredientID, c.Quantity, c.SortOrder); err != nil {
						return fmt.Errorf("insert dst customization: %w", err)
					}
				}
			}
		}
	}

	return tx.Commit()
}

// ClearMeals cancella TUTTI i meals di un piano (e a cascata gli items
// e i cheat_items). Usato per "reset settimana" UI: l'utente puo'
// annullare una copia/template applicato per sbaglio. Idempotente: se
// il piano e' gia' vuoto non e' un errore.
func (r *WeeklyPlans) ClearMeals(ctx context.Context, planID, userID int64) error {
	res, err := r.db.ExecContext(ctx, `
		DELETE FROM meals
		WHERE weekly_plan_id = ?
		  AND weekly_plan_id IN (SELECT id FROM weekly_plans WHERE user_id = ?)
	`, planID, userID)
	if err != nil {
		return fmt.Errorf("clear meals: %w", err)
	}
	_, _ = res.RowsAffected()
	return nil
}

// AddMeal manualmente (override del template).
func (r *WeeklyPlans) AddMeal(ctx context.Context, planID, userID int64, day int, name string) (int64, error) {
	if err := r.assertPlanOwner(ctx, planID, userID); err != nil {
		return 0, err
	}
	if day < 0 || day > 6 {
		return 0, fmt.Errorf("day_of_week fuori range")
	}
	if strings.TrimSpace(name) == "" {
		return 0, fmt.Errorf("nome pasto vuoto")
	}
	var nextOrder int
	if err := r.db.QueryRowContext(ctx,
		`SELECT COALESCE(MAX(sort_order)+1, 0) FROM meals WHERE weekly_plan_id = ? AND day_of_week = ?`,
		planID, day,
	).Scan(&nextOrder); err != nil {
		return 0, fmt.Errorf("compute sort_order: %w", err)
	}
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO meals (weekly_plan_id, day_of_week, name, sort_order) VALUES (?, ?, ?, ?)`,
		planID, day, strings.TrimSpace(name), nextOrder,
	)
	if err != nil {
		return 0, fmt.Errorf("insert meal: %w", err)
	}
	return res.LastInsertId()
}

// RemoveMeal cancella un pasto (e items via CASCADE).
func (r *WeeklyPlans) RemoveMeal(ctx context.Context, mealID, userID int64) error {
	res, err := r.db.ExecContext(ctx, `
		DELETE FROM meals
		WHERE id = ?
		  AND weekly_plan_id IN (SELECT id FROM weekly_plans WHERE user_id = ?)
	`, mealID, userID)
	if err != nil {
		return fmt.Errorf("delete meal: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrMealNotFound
	}
	return nil
}

// AddItem aggiunge una voce a un pasto. Esattamente uno fra recipeID e
// ingredientID dev'essere != 0 (= nil pointer). Quantity > 0.
// quantityUnit: 'g', 'ml', o 'pz' (ignorato per le ricette, usa sempre porzioni).
func (r *WeeklyPlans) AddItem(
	ctx context.Context,
	mealID, userID int64,
	recipeID, ingredientID *int64,
	quantity float64,
	quantityUnit string,
) (int64, error) {
	// Verifica che il pasto appartenga a un piano dell'utente.
	var planUser int64
	err := r.db.QueryRowContext(ctx, `
		SELECT wp.user_id
		FROM meals m JOIN weekly_plans wp ON wp.id = m.weekly_plan_id
		WHERE m.id = ?`, mealID,
	).Scan(&planUser)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrMealNotFound
	}
	if err != nil {
		return 0, fmt.Errorf("meal owner: %w", err)
	}
	if planUser != userID {
		return 0, ErrPlanNotFound
	}
	if (recipeID == nil) == (ingredientID == nil) {
		return 0, fmt.Errorf("specifica esattamente una fra recipe_id e ingredient_id")
	}
	if quantity <= 0 {
		return 0, fmt.Errorf("quantita' deve essere > 0")
	}
	var nextOrder int
	if err := r.db.QueryRowContext(ctx,
		`SELECT COALESCE(MAX(sort_order)+1, 0) FROM meal_items WHERE meal_id = ?`,
		mealID,
	).Scan(&nextOrder); err != nil {
		return 0, fmt.Errorf("compute item sort_order: %w", err)
	}
	unit := quantityUnit
	if unit == "" || recipeID != nil {
		unit = "g" // ricette usano sempre porzioni (non pz), default g per ingredienti
	}
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO meal_items (meal_id, recipe_id, ingredient_id, quantity, quantity_unit, sort_order) VALUES (?, ?, ?, ?, ?, ?)`,
		mealID, recipeID, ingredientID, quantity, unit, nextOrder,
	)
	if err != nil {
		return 0, fmt.Errorf("insert meal_item: %w", err)
	}
	return res.LastInsertId()
}

// GeneratedMealItem e' una voce da inserire in un pasto generato
// automaticamente (vedi CreateMeals).
type GeneratedMealItem struct {
	RecipeID     *int64
	IngredientID *int64
	Quantity     float64
	QuantityUnit string
}

// GeneratedMeal e' un pasto da creare con le sue voci, usato dal generatore
// automatico del piano (internal/service/planner_service.go).
type GeneratedMeal struct {
	DayOfWeek int
	Name      string
	SortOrder int
	Items     []GeneratedMealItem
}

// CreateMeals inserisce in un'unica transazione i pasti generati (con le
// rispettive voci) nel piano indicato. Verifica l'ownership del piano una
// sola volta. Pensato per il generatore automatico: il chiamante deve
// assicurarsi che il piano sia vuoto (vedi HasMeals).
func (r *WeeklyPlans) CreateMeals(ctx context.Context, planID, userID int64, meals []GeneratedMeal) error {
	var pUser int64
	err := r.db.QueryRowContext(ctx, `SELECT user_id FROM weekly_plans WHERE id = ?`, planID).Scan(&pUser)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrPlanNotFound
	}
	if err != nil {
		return fmt.Errorf("plan owner: %w", err)
	}
	if pUser != userID {
		return ErrPlanNotFound
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	mealStmt, err := tx.PrepareContext(ctx,
		`INSERT INTO meals (weekly_plan_id, day_of_week, name, sort_order) VALUES (?, ?, ?, ?)`)
	if err != nil {
		return fmt.Errorf("prepare insert meal: %w", err)
	}
	defer mealStmt.Close()
	itemStmt, err := tx.PrepareContext(ctx,
		`INSERT INTO meal_items (meal_id, recipe_id, ingredient_id, quantity, quantity_unit, sort_order) VALUES (?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return fmt.Errorf("prepare insert item: %w", err)
	}
	defer itemStmt.Close()

	for _, m := range meals {
		res, err := mealStmt.ExecContext(ctx, planID, m.DayOfWeek, m.Name, m.SortOrder)
		if err != nil {
			return fmt.Errorf("insert meal: %w", err)
		}
		mealID, err := res.LastInsertId()
		if err != nil {
			return fmt.Errorf("meal id: %w", err)
		}
		for i, it := range m.Items {
			unit := it.QuantityUnit
			if unit == "" || it.RecipeID != nil {
				unit = "g" // ricette usano sempre porzioni, default g per ingredienti
			}
			if _, err := itemStmt.ExecContext(ctx, mealID, it.RecipeID, it.IngredientID, it.Quantity, unit, i); err != nil {
				return fmt.Errorf("insert meal item: %w", err)
			}
		}
	}
	return tx.Commit()
}

// RemoveItem cancella un meal_item; verifica ownership tramite join.
func (r *WeeklyPlans) RemoveItem(ctx context.Context, itemID, userID int64) error {
	res, err := r.db.ExecContext(ctx, `
		DELETE FROM meal_items
		WHERE id = ?
		  AND meal_id IN (
		    SELECT m.id FROM meals m
		    JOIN weekly_plans wp ON wp.id = m.weekly_plan_id
		    WHERE wp.user_id = ?
		  )
	`, itemID, userID)
	if err != nil {
		return fmt.Errorf("delete meal_item: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrMealItemNotFound
	}
	return nil
}

// LoadFull carica plan + meals + items + (recipes + ingredients) in 4 query.
// Mette tutto sotto WeeklyPlanFull.Days[0..6]. La risoluzione di Recipe/
// Ingredient e' caricata in batch (una query per tipo) e poi mappata.
func (r *WeeklyPlans) LoadFull(ctx context.Context, plan *domain.WeeklyPlan) (*domain.WeeklyPlanFull, error) {
	out := &domain.WeeklyPlanFull{WeeklyPlan: *plan}

	// 1. meals
	mealRows, err := r.db.QueryContext(ctx,
		`SELECT id, weekly_plan_id, day_of_week, name, sort_order
		 FROM meals WHERE weekly_plan_id = ?
		 ORDER BY day_of_week, sort_order, id`, plan.ID,
	)
	if err != nil {
		return nil, fmt.Errorf("load meals: %w", err)
	}
	mealByID := make(map[int64]*domain.MealWithItems)
	var mealIDs []int64
	for mealRows.Next() {
		var m domain.Meal
		if err := mealRows.Scan(&m.ID, &m.WeeklyPlanID, &m.DayOfWeek, &m.Name, &m.SortOrder); err != nil {
			mealRows.Close()
			return nil, fmt.Errorf("scan meal: %w", err)
		}
		mw := &domain.MealWithItems{Meal: m}
		mealByID[m.ID] = mw
		mealIDs = append(mealIDs, m.ID)
	}
	mealRows.Close()

	if len(mealIDs) == 0 {
		// Nessun pasto: ritorna struct vuota.
		return out, nil
	}

	// 2. items per quei meals.
	placeholders := strings.Repeat("?,", len(mealIDs)-1) + "?"
	args := make([]any, len(mealIDs))
	for i, id := range mealIDs {
		args[i] = id
	}
	itemRows, err := r.db.QueryContext(ctx, `
		SELECT id, meal_id, recipe_id, ingredient_id, quantity, sort_order, quantity_unit
		FROM meal_items WHERE meal_id IN (`+placeholders+`)
		ORDER BY meal_id, sort_order, id
	`, args...)
	if err != nil {
		return nil, fmt.Errorf("load items: %w", err)
	}
	itemsByMeal := make(map[int64][]*domain.MealItemFull)
	var recipeIDs, ingredientIDs []int64
	for itemRows.Next() {
		var it domain.MealItem
		if err := itemRows.Scan(&it.ID, &it.MealID, &it.RecipeID, &it.IngredientID, &it.Quantity, &it.SortOrder, &it.QuantityUnit); err != nil {
			itemRows.Close()
			return nil, fmt.Errorf("scan item: %w", err)
		}
		full := &domain.MealItemFull{MealItem: it}
		itemsByMeal[it.MealID] = append(itemsByMeal[it.MealID], full)
		if it.RecipeID != nil {
			recipeIDs = append(recipeIDs, *it.RecipeID)
		}
		if it.IngredientID != nil {
			ingredientIDs = append(ingredientIDs, *it.IngredientID)
		}
	}
	itemRows.Close()

	// 2b. cheats per quei meals (stessa struttura di meal_items).
	cheatRows, err := r.db.QueryContext(ctx, `
		SELECT id, meal_id, recipe_id, ingredient_id, quantity, sort_order
		FROM meal_cheat_items WHERE meal_id IN (`+placeholders+`)
		ORDER BY meal_id, sort_order, id
	`, args...)
	if err != nil {
		return nil, fmt.Errorf("load cheats: %w", err)
	}
	cheatsByMeal := make(map[int64][]*domain.MealItemFull)
	for cheatRows.Next() {
		var it domain.MealItem
		if err := cheatRows.Scan(&it.ID, &it.MealID, &it.RecipeID, &it.IngredientID, &it.Quantity, &it.SortOrder); err != nil {
			cheatRows.Close()
			return nil, fmt.Errorf("scan cheat: %w", err)
		}
		full := &domain.MealItemFull{MealItem: it}
		cheatsByMeal[it.MealID] = append(cheatsByMeal[it.MealID], full)
		if it.RecipeID != nil {
			recipeIDs = append(recipeIDs, *it.RecipeID)
		}
		if it.IngredientID != nil {
			ingredientIDs = append(ingredientIDs, *it.IngredientID)
		}
	}
	cheatRows.Close()

	// 2c. personalizzazioni per gli item ricetta (non per i cheat).
	// Raccoglie tutti i meal_item_id che puntano a una ricetta, poi carica
	// le eventuali righe meal_item_customizations con JOIN a ingredients.
	customMap := make(map[int64][]domain.MealItemCustomization)
	{
		var recipeItemIDs []int64
		for _, items := range itemsByMeal {
			for _, it := range items {
				if it.RecipeID != nil {
					recipeItemIDs = append(recipeItemIDs, it.ID)
				}
			}
		}
		if len(recipeItemIDs) > 0 {
			ph2c := strings.Repeat("?,", len(recipeItemIDs)-1) + "?"
			ca := make([]any, len(recipeItemIDs))
			for i, id := range recipeItemIDs {
				ca[i] = id
			}
			custRows, err := r.db.QueryContext(ctx, `
				SELECT mic.id, mic.meal_item_id, mic.ingredient_id, mic.quantity, mic.sort_order,
				       i.id, i.source, i.off_barcode, i.name, i.brand, i.unit,
				       i.kcal_per_100, i.protein_per_100, i.carbs_per_100, i.fat_per_100,
				       i.created_by, i.created_at, i.last_verified_at, i.superseded_by
				FROM meal_item_customizations mic
				JOIN ingredients i ON i.id = mic.ingredient_id
				WHERE mic.meal_item_id IN (`+ph2c+`)
				ORDER BY mic.meal_item_id, mic.sort_order, mic.id
			`, ca...)
			if err != nil {
				return nil, fmt.Errorf("load customizations: %w", err)
			}
			for custRows.Next() {
				var c domain.MealItemCustomization
				var ing domain.Ingredient
				var src, unit string
				err := custRows.Scan(
					&c.ID, &c.MealItemID, &c.IngredientID, &c.Quantity, &c.SortOrder,
					&ing.ID, &src, &ing.OFFBarcode, &ing.Name, &ing.Brand, &unit,
					&ing.KcalPer100, &ing.ProteinPer100, &ing.CarbsPer100, &ing.FatPer100,
					&ing.CreatedBy, &ing.CreatedAt, &ing.LastVerifiedAt, &ing.SupersededBy,
				)
				if err != nil {
					custRows.Close()
					return nil, fmt.Errorf("scan customization: %w", err)
				}
				ing.Source = domain.IngredientSource(src)
				ing.Unit = domain.IngredientUnit(unit)
				c.Ingredient = &ing
				customMap[c.MealItemID] = append(customMap[c.MealItemID], c)
			}
			custRows.Close()
			if err := custRows.Err(); err != nil {
				return nil, err
			}
		}
	}

	// 3. ingredients in batch.
	ingMap := make(map[int64]*domain.Ingredient)
	if len(ingredientIDs) > 0 {
		ph := strings.Repeat("?,", len(ingredientIDs)-1) + "?"
		ia := make([]any, len(ingredientIDs))
		for i, id := range ingredientIDs {
			ia[i] = id
		}
		ingRows, err := r.db.QueryContext(ctx, `
			SELECT id, source, off_barcode, name, brand, unit,
			       kcal_per_100, protein_per_100, carbs_per_100, fat_per_100,
			       created_by, created_at, last_verified_at, superseded_by
			FROM ingredients WHERE id IN (`+ph+`)
		`, ia...)
		if err != nil {
			return nil, fmt.Errorf("load ingredients: %w", err)
		}
		for ingRows.Next() {
			var ing domain.Ingredient
			var src, unit string
			err := ingRows.Scan(
				&ing.ID, &src, &ing.OFFBarcode, &ing.Name, &ing.Brand, &unit,
				&ing.KcalPer100, &ing.ProteinPer100, &ing.CarbsPer100, &ing.FatPer100,
				&ing.CreatedBy, &ing.CreatedAt, &ing.LastVerifiedAt, &ing.SupersededBy,
			)
			if err != nil {
				ingRows.Close()
				return nil, fmt.Errorf("scan ingredient: %w", err)
			}
			ing.Source = domain.IngredientSource(src)
			ing.Unit = domain.IngredientUnit(unit)
			ingMap[ing.ID] = &ing
		}
		ingRows.Close()
	}

	// 4. recipes (con i loro items joined a ingredients) in batch.
	recipeMap := make(map[int64]*domain.Recipe)
	recipeItemsMap := make(map[int64][]domain.RecipeItemFull)
	if len(recipeIDs) > 0 {
		ph := strings.Repeat("?,", len(recipeIDs)-1) + "?"
		ra := make([]any, len(recipeIDs))
		for i, id := range recipeIDs {
			ra[i] = id
		}
		recRows, err := r.db.QueryContext(ctx, `
			SELECT id, user_id, name, description, servings, is_public, is_draft, created_at, updated_at
			FROM recipes WHERE id IN (`+ph+`)
		`, ra...)
		if err != nil {
			return nil, fmt.Errorf("load recipes: %w", err)
		}
		for recRows.Next() {
			var rec domain.Recipe
			var pub, draft int
			err := recRows.Scan(&rec.ID, &rec.UserID, &rec.Name, &rec.Description,
				&rec.Servings, &pub, &draft, &rec.CreatedAt, &rec.UpdatedAt)
			if err != nil {
				recRows.Close()
				return nil, fmt.Errorf("scan recipe: %w", err)
			}
			rec.IsPublic = pub == 1
			rec.IsDraft = draft == 1
			recipeMap[rec.ID] = &rec
		}
		recRows.Close()

		// recipe_ingredients joined a ingredients per ognuna delle ricette.
		ph2 := strings.Repeat("?,", len(recipeIDs)-1) + "?"
		ra2 := make([]any, len(recipeIDs))
		for i, id := range recipeIDs {
			ra2[i] = id
		}
		riRows, err := r.db.QueryContext(ctx, `
			SELECT ri.id, ri.recipe_id, ri.ingredient_id, ri.quantity, ri.sort_order,
			       i.id, i.source, i.off_barcode, i.name, i.brand, i.unit,
			       i.kcal_per_100, i.protein_per_100, i.carbs_per_100, i.fat_per_100,
			       i.created_by, i.created_at, i.last_verified_at, i.superseded_by
			FROM recipe_ingredients ri JOIN ingredients i ON i.id = ri.ingredient_id
			WHERE ri.recipe_id IN (`+ph2+`)
			ORDER BY ri.recipe_id, ri.sort_order, ri.id
		`, ra2...)
		if err != nil {
			return nil, fmt.Errorf("load recipe_items: %w", err)
		}
		for riRows.Next() {
			var it domain.RecipeItemFull
			var src, unit string
			err := riRows.Scan(
				&it.ID, &it.RecipeID, &it.IngredientID, &it.Quantity, &it.SortOrder,
				&it.Ingredient.ID, &src, &it.Ingredient.OFFBarcode, &it.Ingredient.Name,
				&it.Ingredient.Brand, &unit,
				&it.Ingredient.KcalPer100, &it.Ingredient.ProteinPer100,
				&it.Ingredient.CarbsPer100, &it.Ingredient.FatPer100,
				&it.Ingredient.CreatedBy, &it.Ingredient.CreatedAt,
				&it.Ingredient.LastVerifiedAt, &it.Ingredient.SupersededBy,
			)
			if err != nil {
				riRows.Close()
				return nil, fmt.Errorf("scan recipe_item: %w", err)
			}
			it.Ingredient.Source = domain.IngredientSource(src)
			it.Ingredient.Unit = domain.IngredientUnit(unit)
			recipeItemsMap[it.RecipeID] = append(recipeItemsMap[it.RecipeID], it)
		}
		riRows.Close()
	}

	// 5. assemble: collega items e cheats con i loro target, distribuisce
	//    nei Days. Iteriamo `mealIDs` (ordinato dal SELECT iniziale per
	//    day_of_week, sort_order, id) per garantire l'ordine determinist
	//    nei Days[]. Items e cheats condividono i lookup recipeMap/ingMap.
	resolve := func(it *domain.MealItemFull) {
		if it.RecipeID != nil {
			it.Recipe = recipeMap[*it.RecipeID]
			it.RecipeItems = recipeItemsMap[*it.RecipeID]
			if c, ok := customMap[it.ID]; ok {
				it.Customizations = c
			}
		}
		if it.IngredientID != nil {
			it.Ingredient = ingMap[*it.IngredientID]
		}
	}
	for _, mealID := range mealIDs {
		mw := mealByID[mealID]
		for _, it := range itemsByMeal[mealID] {
			resolve(it)
			mw.Items = append(mw.Items, *it)
		}
		for _, it := range cheatsByMeal[mealID] {
			resolve(it)
			mw.Cheats = append(mw.Cheats, *it)
		}
		if mw.DayOfWeek >= 0 && mw.DayOfWeek < 7 {
			out.Days[mw.DayOfWeek] = append(out.Days[mw.DayOfWeek], *mw)
		}
	}
	return out, nil
}

// ErrReorderMismatch segnala che la lista di id passata a un Reorder non
// corrisponde esattamente all'insieme delle righe esistenti per quel
// container (id estranei, mancanti, duplicati).
var ErrReorderMismatch = errors.New("ordine non valido: ids non corrispondono alle righe esistenti")

// ReorderMealItems persiste l'ordine di tutti i meal_items di un meal.
// `ids` deve contenere esattamente l'insieme degli id correnti, una sola
// volta ognuno: scrive sort_order = indice. Verifica ownership tramite
// JOIN su weekly_plans.user_id.
func (r *WeeklyPlans) ReorderMealItems(ctx context.Context, mealID, userID int64, ids []int64) error {
	// Verifica meal -> plan -> user.
	var planUser int64
	err := r.db.QueryRowContext(ctx, `
		SELECT wp.user_id
		FROM meals m JOIN weekly_plans wp ON wp.id = m.weekly_plan_id
		WHERE m.id = ?`, mealID,
	).Scan(&planUser)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrMealNotFound
	}
	if err != nil {
		return fmt.Errorf("meal owner: %w", err)
	}
	if planUser != userID {
		return ErrPlanNotFound
	}

	// Carica gli id correnti per validare l'input.
	currentIDs, err := scanInt64Set(ctx, r.db,
		`SELECT id FROM meal_items WHERE meal_id = ?`, mealID)
	if err != nil {
		return fmt.Errorf("load current items: %w", err)
	}
	if !sameInt64Set(currentIDs, ids) {
		return ErrReorderMismatch
	}
	return runReorderTx(ctx, r.db,
		`UPDATE meal_items SET sort_order = ? WHERE id = ? AND meal_id = ?`,
		ids, mealID)
}

// ReorderMeals persiste l'ordine dei meals di un giorno specifico per un
// piano settimanale. `ids` deve contenere ESATTAMENTE l'insieme dei
// meal id presenti per quel (planID, day).
func (r *WeeklyPlans) ReorderMeals(ctx context.Context, planID, userID int64, day int, ids []int64) error {
	if err := r.assertPlanOwner(ctx, planID, userID); err != nil {
		return err
	}
	if day < 0 || day > 6 {
		return fmt.Errorf("day_of_week fuori range")
	}
	currentIDs, err := scanInt64Set(ctx, r.db,
		`SELECT id FROM meals WHERE weekly_plan_id = ? AND day_of_week = ?`,
		planID, day)
	if err != nil {
		return fmt.Errorf("load current meals: %w", err)
	}
	if !sameInt64Set(currentIDs, ids) {
		return ErrReorderMismatch
	}
	return runReorderTx(ctx, r.db,
		`UPDATE meals SET sort_order = ? WHERE id = ? AND weekly_plan_id = ?`,
		ids, planID)
}

// ---- helpers di reorder (riusati anche da MealTemplates) -------------------

// scanInt64Set esegue una query e ritorna gli id come slice. Il chiamante
// passa la query e gli args; la funzione e' interna al package.
func scanInt64Set(ctx context.Context, db *sql.DB, query string, args ...any) ([]int64, error) {
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]int64, 0, 8)
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// sameInt64Set ritorna true se a e b contengono esattamente gli stessi
// elementi (ognuno una sola volta), ignorando l'ordine.
func sameInt64Set(a, b []int64) bool {
	if len(a) != len(b) {
		return false
	}
	m := make(map[int64]int, len(a))
	for _, v := range a {
		m[v]++
	}
	for _, v := range b {
		m[v]--
		if m[v] < 0 {
			return false
		}
	}
	return true
}

// runReorderTx esegue gli UPDATE sort_order=index in transazione, con un
// secondo argomento parent (mealID/planID/templateID) per ulteriore safety
// nel WHERE.
func runReorderTx(ctx context.Context, db *sql.DB, updateQuery string, ids []int64, parentID int64) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	stmt, err := tx.PrepareContext(ctx, updateQuery)
	if err != nil {
		return fmt.Errorf("prepare reorder: %w", err)
	}
	defer stmt.Close()
	for idx, id := range ids {
		if _, err := stmt.ExecContext(ctx, idx, id, parentID); err != nil {
			return fmt.Errorf("update sort_order id=%d: %w", id, err)
		}
	}
	return tx.Commit()
}

func (r *WeeklyPlans) assertPlanOwner(ctx context.Context, planID, userID int64) error {
	var owner int64
	err := r.db.QueryRowContext(ctx, `SELECT user_id FROM weekly_plans WHERE id = ?`, planID).Scan(&owner)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrPlanNotFound
	}
	if err != nil {
		return fmt.Errorf("plan owner: %w", err)
	}
	if owner != userID {
		return ErrPlanNotFound
	}
	return nil
}
