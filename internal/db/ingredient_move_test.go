package db

import (
	"context"
	"errors"
	"testing"

	"github.com/amon2126/nutreek/internal/domain"
)

// MoveUses repoints the caller's own rows only, keeps pieces back when the two
// ingredients weigh a piece differently, and refuses mismatched units.
func TestMoveUses(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	ing := NewIngredients(store.DB)
	users := NewUsers(store.DB)
	me, _ := users.Create(ctx, "me@example.com", "h", "Me")
	other, _ := users.Create(ctx, "other@example.com", "h", "Other")

	mk := func(name string, unit domain.IngredientUnit, pieceG *float64) *domain.Ingredient {
		id, err := ing.CreateLocal(ctx, name, nil, unit, 100, 10, 10, 1, &me, pieceG, nil, nil, nil, nil, nil)
		if err != nil {
			t.Fatalf("CreateLocal %s: %v", name, err)
		}
		got, err := ing.GetByID(ctx, id)
		if err != nil {
			t.Fatalf("GetByID: %v", err)
		}
		return got
	}
	from, to := mk("Uova", domain.UnitGram, nil), mk("Uova fresche", domain.UnitGram, nil)
	mlIng := mk("Latte", domain.UnitMilliliter, nil)
	piece := 60.0
	pcsFrom := mk("Uovo", domain.UnitGram, &piece)

	exec := func(q string, args ...any) int64 {
		t.Helper()
		res, err := store.DB.ExecContext(ctx, q, args...)
		if err != nil {
			t.Fatalf("%s: %v", q, err)
		}
		id, _ := res.LastInsertId()
		return id
	}
	meal := func(user int64) int64 {
		plan := exec(`INSERT INTO weekly_plans (user_id, week_start) VALUES (?, '2026-11-30')`, user)
		return exec(`INSERT INTO meals (weekly_plan_id, day_of_week, name) VALUES (?, 0, 'Pranzo')`, plan)
	}
	mine, theirs := meal(me), meal(other)
	exec(`INSERT INTO meal_items (meal_id, ingredient_id, quantity) VALUES (?, ?, 100)`, mine, from.ID)
	exec(`INSERT INTO meal_items (meal_id, ingredient_id, quantity) VALUES (?, ?, 100)`, theirs, from.ID)
	exec(`INSERT INTO meal_items (meal_id, ingredient_id, quantity, quantity_unit) VALUES (?, ?, 2, 'pz')`, mine, pcsFrom.ID)
	recipe := exec(`INSERT INTO recipes (user_id, name) VALUES (?, 'Frittata')`, me)
	exec(`INSERT INTO recipe_ingredients (recipe_id, ingredient_id, quantity) VALUES (?, ?, 120)`, recipe, from.ID)

	count := func(ingredient int64) (n int) {
		_ = store.DB.QueryRowContext(ctx, `SELECT (SELECT count(*) FROM meal_items WHERE ingredient_id = ?1)
			+ (SELECT count(*) FROM recipe_ingredients WHERE ingredient_id = ?1)`, ingredient).Scan(&n)
		return n
	}

	res, err := ing.MoveUses(ctx, me, from, to)
	if err != nil {
		t.Fatalf("MoveUses: %v", err)
	}
	if res.Moved != 2 || res.Skipped != 0 {
		t.Errorf("moved %d skipped %d, want 2 and 0", res.Moved, res.Skipped)
	}
	if got := count(to.ID); got != 2 {
		t.Errorf("target has %d uses, want 2 (a meal item and a recipe row)", got)
	}
	if got := count(from.ID); got != 1 {
		t.Errorf("source keeps %d uses, want 1: the other user's", got)
	}

	// A piece weighs 60 g on one, nothing on the other: the row in pieces stays.
	res, err = ing.MoveUses(ctx, me, pcsFrom, to)
	if err != nil {
		t.Fatalf("MoveUses pieces: %v", err)
	}
	if res.Moved != 0 || res.Skipped != 1 {
		t.Errorf("pieces: moved %d skipped %d, want 0 and 1", res.Moved, res.Skipped)
	}

	if _, err := ing.MoveUses(ctx, me, from, mlIng); !errors.Is(err, ErrMoveUses) {
		t.Errorf("g to ml: err %v, want ErrMoveUses", err)
	}
	if _, err := ing.MoveUses(ctx, me, from, from); !errors.Is(err, ErrMoveUses) {
		t.Errorf("same ingredient: err %v, want ErrMoveUses", err)
	}
}
