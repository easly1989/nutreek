package db

import (
	"context"
	"errors"
	"testing"
	"time"
)

// Moving a row between meals (#228): the row keeps its id (so its quantity and
// customizations), takes the place it was dropped in, never leaves the week, and
// only its owner can move it.
func TestMoveItem(t *testing.T) {
	store := newTestStore(t)
	users := NewUsers(store.DB)
	ings := NewIngredients(store.DB)
	recipes := NewRecipes(store.DB)
	plans := NewWeeklyPlans(store.DB)
	ctx := context.Background()

	alice, _ := users.Create(ctx, "alice@x.com", "h", "Alice")
	bob, _ := users.Create(ctx, "bob@x.com", "h", "Bob")
	oats, _ := ings.CreateLocal(ctx, "Oats", nil, "g", 380, 13, 60, 7, &alice, nil, nil, nil, nil, nil, nil)
	milk, _ := ings.CreateLocal(ctx, "Milk", nil, "g", 46, 3.4, 4.8, 1.6, &alice, nil, nil, nil, nil, nil, nil)
	rice, _ := ings.CreateLocal(ctx, "Rice", nil, "g", 350, 7, 78, 1, &alice, nil, nil, nil, nil, nil, nil)
	soup, _ := recipes.Create(ctx, alice, "Soup", nil, 2, false)

	monday, _ := time.Parse("2006-01-02", "2026-04-27")
	plan, _ := plans.GetOrCreateForWeek(ctx, alice, monday)
	breakfast, _ := plans.AddMeal(ctx, plan.ID, alice, 0, "Breakfast")
	lunch, _ := plans.AddMeal(ctx, plan.ID, alice, 0, "Lunch")
	snack, _ := plans.AddMeal(ctx, plan.ID, alice, 0, "Snack")
	tuesdayBreakfast, _ := plans.AddMeal(ctx, plan.ID, alice, 1, "breakfast") // same name, other case
	tuesdayDinner, _ := plans.AddMeal(ctx, plan.ID, alice, 1, "Dinner")
	oatsRow, _ := plans.AddItem(ctx, breakfast, alice, nil, &oats, 40, "g")
	milkRow, _ := plans.AddItem(ctx, breakfast, alice, nil, &milk, 200, "g")
	soupRow, _ := plans.AddItem(ctx, lunch, alice, &soup, nil, 1, "g")
	riceRow, _ := plans.AddItem(ctx, lunch, alice, nil, &rice, 80, "g")
	if err := plans.RemoveMeal(ctx, snack, alice); err != nil { // an empty meal is a valid target
		t.Fatal(err)
	}
	snack, _ = plans.AddMeal(ctx, plan.ID, alice, 0, "Snack")

	meal := func(item int64) (int64, int) {
		var m int64
		var o int
		if err := store.DB.QueryRowContext(ctx, `SELECT meal_id, sort_order FROM meal_items WHERE id = ?`, item).Scan(&m, &o); err != nil {
			t.Fatal(err)
		}
		return m, o
	}

	// The menu: the other meals of the day, and only the user's.
	mc, err := plans.MoveContextFor(ctx, oatsRow, alice)
	if err != nil || mc.MealID != breakfast || mc.DayOfWeek != 0 || len(mc.Meals) != 2 || mc.Meals[0].ID != lunch || mc.Meals[1].ID != snack {
		t.Fatalf("MoveContextFor = %+v, %v; want Lunch and Snack", mc, err)
	}
	if _, err := plans.MoveContextFor(ctx, oatsRow, bob); !errors.Is(err, ErrMealItemNotFound) {
		t.Errorf("menu of another user's row: %v, want ErrMealItemNotFound", err)
	}

	// A drop between two rows of Lunch: the order says where.
	res, err := plans.MoveItem(ctx, oatsRow, alice, lunch, []int64{soupRow, oatsRow, riceRow})
	if err != nil || res.FromDay != 0 || res.ToDay != 0 {
		t.Fatalf("MoveItem = %+v, %v", res, err)
	}
	if m, o := meal(oatsRow); m != lunch || o != 1 {
		t.Errorf("oats = meal %d order %d, want Lunch, 1", m, o)
	}
	if _, o := meal(riceRow); o != 2 {
		t.Errorf("rice order = %d, want 2", o)
	}
	if m, _ := meal(milkRow); m != breakfast {
		t.Error("the other rows of the source meal must stay")
	}

	// No order: last. An empty meal takes it.
	if _, err := plans.MoveItem(ctx, milkRow, alice, snack, nil); err != nil {
		t.Fatal(err)
	}
	if m, o := meal(milkRow); m != snack || o != 0 {
		t.Errorf("milk = meal %d order %d, want Snack, 0", m, o)
	}

	// Refused: an order that is not the meal's rows plus the moved one, a meal of
	// another user, another plan (the user's own, but another week), someone
	// else's row, and a recipe row moves like any other.
	if _, err := plans.MoveItem(ctx, riceRow, alice, snack, []int64{riceRow}); !errors.Is(err, ErrReorderMismatch) {
		t.Errorf("a partial order: %v, want ErrReorderMismatch", err)
	}
	if m, _ := meal(riceRow); m != lunch {
		t.Error("a refused move must leave the row where it was")
	}
	bobPlan, _ := plans.GetOrCreateForWeek(ctx, bob, monday)
	bobMeal, _ := plans.AddMeal(ctx, bobPlan.ID, bob, 0, "Breakfast")
	if _, err := plans.MoveItem(ctx, riceRow, alice, bobMeal, nil); !errors.Is(err, ErrMealNotFound) {
		t.Errorf("a meal of another user: %v, want ErrMealNotFound", err)
	}
	nextPlan, _ := plans.GetOrCreateForWeek(ctx, alice, monday.AddDate(0, 0, 7))
	nextMeal, _ := plans.AddMeal(ctx, nextPlan.ID, alice, 0, "Breakfast")
	if _, err := plans.MoveItem(ctx, riceRow, alice, nextMeal, nil); !errors.Is(err, ErrMealNotFound) {
		t.Errorf("a meal of another week: %v, want ErrMealNotFound", err)
	}
	if _, err := plans.MoveItem(ctx, riceRow, bob, lunch, nil); !errors.Is(err, ErrMealItemNotFound) {
		t.Errorf("another user's row: %v, want ErrMealItemNotFound", err)
	}
	if _, err := plans.MoveItem(ctx, soupRow, alice, snack, nil); err != nil {
		t.Errorf("a recipe row must move: %v", err)
	}

	// Another day: the meal with the same name (any case), else the last meal, else
	// a new one with the name it leaves.
	res, err = plans.MoveItemToDay(ctx, oatsRow, alice, 1) // from Lunch: no «Lunch» on Tuesday
	if err != nil || res.FromDay != 0 || res.ToDay != 1 {
		t.Fatalf("MoveItemToDay = %+v, %v", res, err)
	}
	if m, _ := meal(oatsRow); m != tuesdayDinner {
		t.Errorf("no meal of that name: oats in %d, want the last meal of the day (Dinner)", m)
	}
	if _, err := plans.MoveItem(ctx, riceRow, alice, breakfast, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := plans.MoveItemToDay(ctx, riceRow, alice, 1); err != nil { // from Breakfast: «breakfast» on Tuesday
		t.Fatal(err)
	}
	if m, _ := meal(riceRow); m != tuesdayBreakfast {
		t.Errorf("same name in another case: rice in %d, want Tuesday's breakfast", m)
	}
	if _, err := plans.MoveItemToDay(ctx, riceRow, alice, 4); err != nil { // Friday has no meal
		t.Fatal(err)
	}
	var name string
	var day int
	if err := store.DB.QueryRowContext(ctx, `SELECT m.name, m.day_of_week FROM meals m JOIN meal_items mi ON mi.meal_id = m.id WHERE mi.id = ?`, riceRow).Scan(&name, &day); err != nil {
		t.Fatal(err)
	}
	if name != "breakfast" || day != 4 {
		t.Errorf("an empty day: rice in %q on day %d, want a new «breakfast» on 4", name, day)
	}
	if _, err := plans.MoveItemToDay(ctx, riceRow, alice, 7); err == nil {
		t.Error("a day out of range must be refused")
	}
	if _, err := plans.MoveItemToDay(ctx, riceRow, bob, 2); !errors.Is(err, ErrMealItemNotFound) {
		t.Errorf("another user's row to a day: %v, want ErrMealItemNotFound", err)
	}
	var meals int
	_ = store.DB.QueryRowContext(ctx, `SELECT count(*) FROM meals WHERE weekly_plan_id = ? AND day_of_week = 2`, plan.ID).Scan(&meals)
	if meals != 0 {
		t.Error("a refused move must not create a meal")
	}
}
