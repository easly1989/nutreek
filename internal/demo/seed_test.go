package demo

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/amon2126/nutreek/internal/db"
	"github.com/amon2126/nutreek/internal/off"
	"github.com/amon2126/nutreek/internal/service"
)

// seedKeys lists every text key the seed shows, from its definitions.
func seedKeys() []string {
	var keys []string
	for _, p := range slices.Concat(ownPlan, library) {
		keys = append(keys, p.key)
	}
	for _, c := range categories {
		keys = append(keys, "category."+c)
	}
	for _, f := range foods {
		keys = append(keys, "food."+f.key)
	}
	for _, r := range recipes {
		keys = append(keys, "recipe."+r.key)
	}
	for _, m := range mealTypes {
		keys = append(keys, "meal."+m.key)
		for _, v := range m.variants {
			keys = append(keys, "variant."+v.key)
		}
	}
	for _, s := range shopping {
		keys = append(keys, "shop."+s.key)
	}
	return keys
}

// Every language has a text for every key the seed shows, and nothing else:
// the seed's items and amounts are defined once, so the languages differ only
// in these texts.
func TestSeedWords(t *testing.T) {
	keys := seedKeys()
	if len(keys) != len(uniq(keys)) {
		t.Fatalf("duplicate keys in the seed: %v", keys)
	}
	for lang, w := range words {
		for _, k := range keys {
			if w.text[k] == "" {
				t.Errorf("%s: no text for %q", lang, k)
			}
		}
		for k := range w.text {
			if !slices.Contains(keys, k) {
				t.Errorf("%s: %q is not used by the seed", lang, k)
			}
		}
		for _, f := range foods {
			if f.ean == "" {
				continue
			}
			if _, ok := off.NormalizeBarcode(barcode(w, f)); !ok {
				t.Errorf("%s: %s: %q is not a valid barcode", lang, f.key, barcode(w, f))
			}
		}
	}
	for _, f := range foods {
		if f.category != "" && !slices.Contains(categories, f.category) {
			t.Errorf("%s: unknown category %q", f.key, f.category)
		}
	}
}

func uniq(xs []string) []string {
	out := slices.Clone(xs)
	slices.Sort(out)
	return slices.Compact(out)
}

// English and Italian accounts hold the same items and amounts, each with the
// names of its own language only.
func TestSeedLanguages(t *testing.T) {
	ctx := context.Background()
	d := newStore(t)
	if err := Prepare(ctx, d); err != nil {
		t.Fatal(err)
	}
	svc := New(d, Config{})
	ids := map[string]int64{}
	for _, lang := range []string{"en", "it", "en"} { // a second English account reuses the shared products
		id, err := svc.Start(ctx, lang, AsPlan)
		if err != nil {
			t.Fatalf("%s: start: %v", lang, err)
		}
		ids[lang] = id
	}

	en, it := fingerprint(t, d, ids["en"]), fingerprint(t, d, ids["it"])
	if !slices.Equal(en, it) {
		t.Errorf("the seeds differ:\nen %v\nit %v", en, it)
	}

	for lang, id := range ids {
		own := map[string]bool{}
		for _, s := range words[lang].text {
			own[s] = true
		}
		for _, s := range seededTexts(t, d, id) {
			if !own[s] {
				t.Errorf("%s account shows %q, not in its vocabulary", lang, s)
			}
		}
	}
}

// fingerprint sums the seeded rows of a user, language aside.
func fingerprint(t *testing.T, d *sql.DB, userID int64) []string {
	t.Helper()
	queries := []string{
		`SELECT count(*), total(kcal_per_100 + protein_per_100 + carbs_per_100 + fat_per_100 + coalesce(unit_weight_g, 0)) FROM ingredients WHERE created_by = ?1`,
		`SELECT count(*), total(servings) FROM recipes WHERE user_id = ?1`,
		`SELECT count(*), total(ri.quantity) FROM recipe_ingredients ri JOIN recipes r ON r.id = ri.recipe_id WHERE r.user_id = ?1`,
		`SELECT count(*), total(i.quantity) FROM nutritionist_meal_items i
		   JOIN nutritionist_meal_variations v ON v.id = i.variation_id
		   JOIN nutritionist_meal_types mt ON mt.id = v.meal_type_id
		   JOIN nutritionist_plans p ON p.id = mt.plan_id WHERE p.user_id = ?1`,
		`SELECT count(*), total(i.quantity * coalesce(g.kcal_per_100, 1)) FROM meal_items i
		   JOIN meals m ON m.id = i.meal_id JOIN weekly_plans w ON w.id = m.weekly_plan_id
		   LEFT JOIN ingredients g ON g.id = i.ingredient_id WHERE w.user_id = ?1`,
		`SELECT count(*), total(c.quantity * coalesce(g.kcal_per_100, 1)) FROM meal_cheat_items c
		   JOIN meals m ON m.id = c.meal_id JOIN weekly_plans w ON w.id = m.weekly_plan_id
		   LEFT JOIN ingredients g ON g.id = c.ingredient_id WHERE w.user_id = ?1`,
		`SELECT count(*), total(weight_kg) FROM weight_logs WHERE user_id = ?1`,
		`SELECT count(*), total(amount_ml) FROM water_logs WHERE user_id = ?1`,
		`SELECT count(*), total(kcal_burned) FROM workout_logs WHERE user_id = ?1`,
		`SELECT count(*), total(ui.checked) FROM shopping_user_items ui JOIN shopping_lists l ON l.id = ui.shopping_list_id WHERE l.user_id = ?1`,
		`SELECT count(*), 0 FROM ingredient_user_categories WHERE user_id = ?1`,
	}
	out := make([]string, 0, len(queries))
	for _, q := range queries {
		var n int
		var sum float64
		if err := d.QueryRow(q, userID).Scan(&n, &sum); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
		out = append(out, fmt.Sprintf("%d/%.1f", n, sum))
	}
	return out
}

// seededTexts returns every name a user's seeded data shows.
func seededTexts(t *testing.T, d *sql.DB, userID int64) []string {
	t.Helper()
	rows, err := d.Query(`
		WITH used(id) AS (
			SELECT id FROM ingredients WHERE created_by = ?1
			UNION SELECT ri.ingredient_id FROM recipe_ingredients ri JOIN recipes r ON r.id = ri.recipe_id WHERE r.user_id = ?1
			UNION SELECT i.ingredient_id FROM meal_items i JOIN meals m ON m.id = i.meal_id
			        JOIN weekly_plans w ON w.id = m.weekly_plan_id WHERE w.user_id = ?1 AND i.ingredient_id IS NOT NULL
			UNION SELECT c.ingredient_id FROM meal_cheat_items c JOIN meals m ON m.id = c.meal_id
			        JOIN weekly_plans w ON w.id = m.weekly_plan_id WHERE w.user_id = ?1 AND c.ingredient_id IS NOT NULL
		)
		SELECT name FROM ingredients WHERE id IN used
		UNION SELECT product_name FROM off_products WHERE barcode IN (SELECT off_barcode FROM ingredients WHERE id IN used)
		UNION SELECT name FROM ingredient_categories WHERE user_id = ?1
		UNION SELECT name FROM recipes WHERE user_id = ?1
		UNION SELECT name FROM nutritionist_plans WHERE user_id = ?1
		UNION SELECT mt.name FROM nutritionist_meal_types mt JOIN nutritionist_plans p ON p.id = mt.plan_id WHERE p.user_id = ?1
		UNION SELECT v.name FROM nutritionist_meal_variations v JOIN nutritionist_meal_types mt ON mt.id = v.meal_type_id
		        JOIN nutritionist_plans p ON p.id = mt.plan_id WHERE p.user_id = ?1
		UNION SELECT m.name FROM meals m JOIN weekly_plans w ON w.id = m.weekly_plan_id WHERE w.user_id = ?1
		UNION SELECT ui.description FROM shopping_user_items ui JOIN shopping_lists l ON l.id = ui.shopping_list_id WHERE l.user_id = ?1`,
		userID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var s sql.NullString
		if err := rows.Scan(&s); err != nil {
			t.Fatal(err)
		}
		out = append(out, s.String)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if len(out) < 20 {
		t.Fatalf("only %d names seeded: %v", len(out), out)
	}
	return out
}

// A new account has something on every page: the week and its history,
// recipes, the plan with its variants, statistics with a TDEE suggestion and
// a weight trend, the shopping list. Nothing is logged in the future.
func TestSeedFillsThePages(t *testing.T) {
	ctx := context.Background()
	d := newStore(t)
	if err := Prepare(ctx, d); err != nil {
		t.Fatal(err)
	}
	id, err := New(d, Config{}).Start(ctx, "en", AsPlan)
	if err != nil {
		t.Fatal(err)
	}
	monday := db.MondayOf(time.Now())
	today := time.Now().Format("2006-01-02")

	count := func(q string, args ...any) int {
		t.Helper()
		var n int
		if err := d.QueryRowContext(ctx, q, args...).Scan(&n); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
		return n
	}
	if n := count(`SELECT count(DISTINCT w.id) FROM weekly_plans w JOIN meals m ON m.weekly_plan_id = w.id
		WHERE w.user_id = ? AND w.week_start BETWEEN ? AND ? AND w.source_plan_id IS NOT NULL`,
		id, monday.AddDate(0, 0, -7*historyWeeks).Format("2006-01-02"), monday.Format("2006-01-02")); n != historyWeeks+1 {
		t.Errorf("%d planned weeks, want %d", n, historyWeeks+1)
	}
	if n := count(`SELECT count(*) FROM meals m JOIN weekly_plans w ON w.id = m.weekly_plan_id
		WHERE w.user_id = ? AND NOT EXISTS (SELECT 1 FROM meal_items i WHERE i.meal_id = m.id)`, id); n != 0 {
		t.Errorf("%d meals without items", n)
	}
	if n := count(`SELECT count(*) FROM recipes WHERE user_id = ? AND servings > 1`, id); n == 0 {
		t.Error("no recipe with servings")
	}
	for what, q := range map[string]string{
		"cheats":   `SELECT count(*) FROM meal_cheat_items c JOIN meals m ON m.id = c.meal_id JOIN weekly_plans w ON w.id = m.weekly_plan_id WHERE w.user_id = ?`,
		"water":    `SELECT count(*) FROM water_logs WHERE user_id = ?`,
		"workouts": `SELECT count(*) FROM workout_logs WHERE user_id = ?`,
	} {
		if count(q, id) == 0 {
			t.Errorf("no %s", what)
		}
	}
	for what, q := range map[string]string{
		"cheat":    `SELECT count(*) FROM meal_cheat_items c JOIN meals m ON m.id = c.meal_id JOIN weekly_plans w ON w.id = m.weekly_plan_id WHERE w.user_id = ? AND date(w.week_start, '+' || m.day_of_week || ' days') > ?`,
		"water":    `SELECT count(*) FROM water_logs WHERE user_id = ? AND logged_at > ?`,
		"workout":  `SELECT count(*) FROM workout_logs WHERE user_id = ? AND logged_at > ?`,
		"weigh-in": `SELECT count(*) FROM weight_logs WHERE user_id = ? AND measured_at > ?`,
	} {
		if n := count(q, id, today); n != 0 {
			t.Errorf("%d %s rows in the future", n, what)
		}
	}

	plan, err := db.NewNutritionistPlans(d).LoadPlanFull(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.MealTypes) != len(mealTypes) {
		t.Errorf("plan has %d meals, want %d", len(plan.MealTypes), len(mealTypes))
	}
	for _, mt := range plan.MealTypes {
		if len(mt.Variations) < 2 {
			t.Errorf("%s: %d variants", mt.Name, len(mt.Variations))
		}
		for _, v := range mt.Variations {
			if len(v.Items) == 0 {
				t.Errorf("%s: empty variant", mt.Name)
			}
		}
	}

	ings := db.NewIngredients(d)
	// Prepare took every language's demo codes: a visitor cannot link one to a food of theirs.
	mine, err := ings.CreateLocal(ctx, "Visitor's yogurt", nil, "g", 50, 5, 5, 1, &id, nil, nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, w := range words {
		code := barcode(w, foods[slices.IndexFunc(foods, func(f food) bool { return f.ean != "" })])
		if err := ings.SetLocalBarcode(ctx, mine, code); !errors.Is(err, db.ErrBarcodeAlreadyUsed) {
			t.Errorf("linking demo code %s: %v", code, err)
		}
	}

	for _, f := range foods {
		if f.ean == "" {
			continue
		}
		code := barcode(words["en"], f)
		if _, err := ings.GetByBarcode(ctx, code); err != nil {
			t.Errorf("%s: barcode %s not among the ingredients: %v", f.key, code, err)
		}
		if n := count(`SELECT count(*) FROM off_products WHERE barcode = ? AND has_full_nutrients = 1`, code); n != 1 {
			t.Errorf("%s: barcode %s not among the OpenFoodFacts products", f.key, code)
		}
	}

	user, err := db.NewUsers(d).GetByID(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	weights := db.NewWeightLogs(d)
	plans := db.NewWeeklyPlans(d)
	report, err := (&service.TDEEService{WeightLogs: weights, WeeklyPlans: plans}).Report(ctx, user, 12, monday)
	if err != nil {
		t.Fatal(err)
	}
	if !report.HasEnoughData || report.Confidence < 100 || report.SuggestionDiff == 0 {
		t.Errorf("no TDEE suggestion: %+v", report)
	}
	t.Logf("adaptive TDEE %.0f kcal, %+.0f on the target; formula %.0f", report.EstimatedTDEE, report.SuggestionDiff, report.FormulaBasedTDEE)
	logs, err := weights.ForRange(ctx, id, monday.AddDate(0, 0, -7*historyWeeks), monday.AddDate(0, 0, 6))
	if err != nil {
		t.Fatal(err)
	}
	if len(logs) < 20 {
		t.Fatalf("%d weigh-ins", len(logs))
	}
	first, last := logs[0], logs[len(logs)-1]
	if first.MeasuredAt.After(last.MeasuredAt) {
		first, last = last, first
	}
	if last.WeightKg >= first.WeightKg {
		t.Errorf("no downward trend: %.1f kg → %.1f kg", first.WeightKg, last.WeightKg)
	}

	shop := db.NewShoppingLists(d, plans)
	list, err := shop.GetOrCreateList(ctx, id, monday, 1)
	if err != nil {
		t.Fatal(err)
	}
	if err := shop.Generate(ctx, list.ID, id); err != nil {
		t.Fatal(err)
	}
	full, err := shop.LoadFull(ctx, list.ID, id)
	if err != nil {
		t.Fatal(err)
	}
	if len(full.Items) == 0 || len(full.UserItems) != len(shopping) {
		t.Errorf("shopping list: %d items, %d by hand", len(full.Items), len(full.UserItems))
	}
}

// Seeding a group, the visitor's account and the others', stays well under a
// second (startBudget).
func TestStartIsFast(t *testing.T) {
	ctx := context.Background()
	d := newStore(t)
	if err := Prepare(ctx, d); err != nil {
		t.Fatal(err)
	}
	svc := New(d, Config{})
	if _, err := svc.Start(ctx, "en", AsPlan); err != nil { // the first group warms up the statement cache
		t.Fatal(err)
	}
	for _, as := range []string{AsPlan, AsCoach, AsClient} {
		start := time.Now()
		if _, err := svc.Start(ctx, "en", as); err != nil {
			t.Fatal(err)
		}
		took := time.Since(start)
		t.Logf("seeding a %s group took %v", as, took)
		if took > startBudget {
			t.Errorf("seeding a %s group took %v", as, took)
		}
	}
}

func BenchmarkStart(b *testing.B) {
	ctx := context.Background()
	store, err := db.Open(b.TempDir() + "/bench.db")
	if err != nil {
		b.Fatal(err)
	}
	defer store.Close()
	if err := store.Migrate(ctx); err != nil {
		b.Fatal(err)
	}
	if err := Prepare(ctx, store.DB); err != nil {
		b.Fatal(err)
	}
	svc := New(store.DB, Config{})
	for b.Loop() {
		if _, err := svc.Start(ctx, "en", AsPlan); err != nil {
			b.Fatal(err)
		}
	}
}

// SeedAccount fills an account someone signed up for, for the README
// screenshots, and refuses to seed it twice.
func TestSeedAccount(t *testing.T) {
	ctx := context.Background()
	d := newStore(t)
	id, err := db.NewUsers(d).Create(ctx, "shots@example.test", "hash", "Shots")
	if err != nil {
		t.Fatal(err)
	}
	// What the pages create just by being shown, and the weigh-in of the onboarding.
	plans := db.NewWeeklyPlans(d)
	monday := db.MondayOf(time.Now())
	if _, err := plans.GetOrCreateForWeek(ctx, id, monday); err != nil {
		t.Fatal(err)
	}
	if _, err := db.NewShoppingLists(d, plans).GetOrCreateList(ctx, id, monday, 1); err != nil {
		t.Fatal(err)
	}
	if _, err := db.NewNutritionistPlans(d).LoadPlanFull(ctx, id); err != nil {
		t.Fatal(err)
	}
	if _, err := db.NewWeightLogs(d).LogWeight(ctx, id, time.Now(), 80, nil); err != nil {
		t.Fatal(err)
	}

	if err := SeedAccount(ctx, d, "shots@example.test", "it"); err != nil {
		t.Fatalf("seed: %v", err)
	}
	var plan string
	if err := d.QueryRowContext(ctx, `SELECT name FROM nutritionist_plans WHERE user_id = ? AND is_active = 1`, id).Scan(&plan); err != nil || plan != words["it"].text["plan"] {
		t.Errorf("plan %q (%v)", plan, err)
	}
	var kg float64
	if err := d.QueryRowContext(ctx, `SELECT weight_kg FROM weight_logs WHERE user_id = ? AND measured_at = ?`, id, time.Now().Format("2006-01-02")).Scan(&kg); err != nil || kg != persona.weightKg {
		t.Errorf("today's weigh-in %.1f kg, want the trend's %.1f (%v)", kg, persona.weightKg, err)
	}
	if err := SeedAccount(ctx, d, "shots@example.test", "it"); err == nil {
		t.Error("seeded the same account twice")
	}
	if err := SeedAccount(ctx, d, "nobody@example.test", "en"); err == nil {
		t.Error("seeded an account that does not exist")
	}
}
