package demo

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"strconv"
	"time"

	"github.com/amon2126/nutreek/internal/db"
)

// The seed (#183): a lived-in account in one language. Items and amounts are
// defined once below; every name they show comes from the vocabulary's text,
// so the languages hold the same data.

// historyWeeks is how many weeks before the current one are filled in for
// the person the visitor plays; otherWeeks for the rest of their group.
const (
	historyWeeks = 12
	otherWeeks   = 4
)

// demoBrand is the brand of the demo's OpenFoodFacts products, which are not
// real ones: their barcodes are in the GS1 range for in-store use (prefix 200).
const demoBrand = "Nutreek Demo"

// food is an ingredient; its name is the text "food.<key>". Macros are per
// 100 g (or ml).
type food struct {
	key                       string
	unit                      string // g or ml
	kcal, protein, carbs, fat float64
	pieceG                    float64 // weight of one piece, 0 when not counted in pieces
	category                  string  // "category.<key>", "" for none
	// OpenFoodFacts demo products only:
	ean      string // last two digits of the barcode, after the vocabulary's prefix
	pack     string // package quantity
	servingG float64
}

var categories = []string{"produce", "protein", "dairy", "pantry", "snacks"}

var foods = []food{
	{key: "oats", unit: "g", kcal: 372, protein: 13.5, carbs: 58.7, fat: 7, category: "pantry"},
	{key: "milk", unit: "ml", kcal: 46, protein: 3.4, carbs: 4.8, fat: 1.6, category: "dairy"},
	{key: "blueberries", unit: "g", kcal: 57, protein: 0.7, carbs: 14.5, fat: 0.3, category: "produce"},
	{key: "banana", unit: "g", kcal: 89, protein: 1.1, carbs: 22.8, fat: 0.3, pieceG: 120, category: "produce"},
	{key: "apple", unit: "g", kcal: 52, protein: 0.3, carbs: 13.8, fat: 0.2, pieceG: 180, category: "produce"},
	{key: "almonds", unit: "g", kcal: 579, protein: 21.2, carbs: 21.6, fat: 49.9, category: "snacks"},
	{key: "eggs", unit: "g", kcal: 143, protein: 12.6, carbs: 0.7, fat: 9.5, pieceG: 60, category: "protein"},
	{key: "bread", unit: "g", kcal: 247, protein: 13, carbs: 41, fat: 3.4, category: "pantry"},
	{key: "chicken", unit: "g", kcal: 110, protein: 23, carbs: 0, fat: 1.6, category: "protein"},
	{key: "rice", unit: "g", kcal: 356, protein: 8, carbs: 78, fat: 0.9, category: "pantry"},
	{key: "salmon", unit: "g", kcal: 208, protein: 20, carbs: 0, fat: 13, category: "protein"},
	{key: "potatoes", unit: "g", kcal: 77, protein: 2, carbs: 17, fat: 0.1, category: "produce"},
	{key: "broccoli", unit: "g", kcal: 34, protein: 2.8, carbs: 7, fat: 0.4, category: "produce"},
	{key: "salad", unit: "g", kcal: 17, protein: 1.3, carbs: 2.9, fat: 0.2, category: "produce"},
	{key: "olive_oil", unit: "g", kcal: 884, protein: 0, carbs: 0, fat: 100, category: "pantry"},
	{key: "pasta", unit: "g", kcal: 348, protein: 13, carbs: 64, fat: 2.5, category: "pantry"},
	{key: "passata", unit: "g", kcal: 32, protein: 1.4, carbs: 5.6, fat: 0.2, category: "pantry"},
	{key: "lentils", unit: "g", kcal: 352, protein: 24.6, carbs: 63.4, fat: 1.1, category: "pantry"},
	{key: "parmesan", unit: "g", kcal: 392, protein: 33, carbs: 0, fat: 29, category: "dairy"},
	{key: "pizza", unit: "g", kcal: 250, protein: 10, carbs: 33, fat: 8.5},
	{key: "yogurt", unit: "g", kcal: 57, protein: 10, carbs: 3.6, fat: 0.2, category: "dairy", ean: "01", pack: "500 g", servingG: 150},
	{key: "tuna", unit: "g", kcal: 103, protein: 23.5, carbs: 0, fat: 1, category: "protein", ean: "02", pack: "2 x 80 g", servingG: 80},
	{key: "crackers", unit: "g", kcal: 420, protein: 11, carbs: 68, fat: 10, category: "snacks", ean: "03", pack: "250 g", servingG: 30},
	{key: "chocolate", unit: "g", kcal: 580, protein: 9, carbs: 34, fat: 43, category: "snacks", ean: "04", pack: "100 g", servingG: 20},
}

// portion is a food in grams/ml or pieces ("pz"), or a recipe in servings.
type portion struct {
	food, recipe string
	qty          float64
	unit         string
}

func grams(key string, g float64) portion  { return portion{food: key, qty: g, unit: "g"} }
func pieces(key string, n float64) portion { return portion{food: key, qty: n, unit: "pz"} }
func servings(key string, n float64) portion {
	return portion{recipe: key, qty: n, unit: "g"} // a recipe's quantity is servings; the unit is unused
}

// recipe's name is the text "recipe.<key>"; its items are in grams/ml.
type recipe struct {
	key      string
	servings int
	items    []portion
}

var recipes = []recipe{
	{"overnight_oats", 1, []portion{grams("oats", 70), grams("milk", 200), grams("blueberries", 100), grams("almonds", 20)}},
	{"chicken_bowl", 2, []portion{grams("chicken", 300), grams("rice", 160), grams("broccoli", 300), grams("olive_oil", 20)}},
	{"tuna_pasta", 2, []portion{grams("pasta", 180), grams("passata", 300), grams("tuna", 160), grams("olive_oil", 15), grams("parmesan", 20)}},
	{"lentil_soup", 4, []portion{grams("lentils", 320), grams("passata", 400), grams("potatoes", 400), grams("olive_oil", 30)}},
	{"salmon_potatoes", 2, []portion{grams("salmon", 300), grams("potatoes", 500), grams("olive_oil", 15)}},
}

// mealType is a meal of the nutrition plan ("meal.<key>") with its variants
// ("variant.<key>"); the week's meals take the same names.
type mealType struct {
	key      string
	variants []variant
}

type variant struct {
	key   string
	items []portion
}

var mealTypes = []mealType{
	{"breakfast", []variant{
		{"oats_blueberries", []portion{servings("overnight_oats", 1)}},
		{"yogurt_banana", []portion{grams("yogurt", 200), grams("oats", 50), pieces("banana", 1)}},
		{"eggs_toast", []portion{grams("bread", 100), pieces("eggs", 2)}},
	}},
	{"lunch", []variant{
		{"chicken_bowl", []portion{servings("chicken_bowl", 1)}},
		{"tuna_pasta", []portion{servings("tuna_pasta", 1)}},
		{"soup_bread", []portion{servings("lentil_soup", 1), grams("bread", 50)}},
	}},
	{"snack", []variant{
		{"apple_almonds", []portion{pieces("apple", 1), grams("almonds", 25)}},
		{"yogurt_crackers", []portion{grams("yogurt", 150), grams("blueberries", 80), grams("crackers", 30)}},
	}},
	{"dinner", []variant{
		{"salmon_salad", []portion{servings("salmon_potatoes", 1), grams("salad", 100)}},
		{"omelette", []portion{pieces("eggs", 3), grams("bread", 80), grams("salad", 150), grams("olive_oil", 10)}},
	}},
}

// planDef is a nutrition plan named by the text key, its foods weighing scale
// times the amounts below.
type planDef struct {
	key   string
	scale float64
}

// ownPlan is a person's own plan; library is a coach's, which they assign.
var (
	ownPlan = []planDef{{"plan", 1}}
	library = []planDef{{"plan.balanced", 1}, {"plan.light", 0.8}, {"plan.training", 1.25}}
)

// cheat replaces a meal so many days ago; its foods are in grams.
type cheat struct {
	daysAgo int
	meal    string
	items   []portion
}

var cheats = []cheat{
	{2, "dinner", []portion{grams("pizza", 350)}},
	{6, "snack", []portion{grams("chocolate", 40)}},
	{12, "dinner", []portion{grams("pizza", 300)}},
	{20, "snack", []portion{grams("chocolate", 30)}},
	{33, "lunch", []portion{grams("pizza", 350)}},
	{47, "dinner", []portion{grams("pizza", 350)}},
	{61, "snack", []portion{grams("chocolate", 50)}},
	{75, "dinner", []portion{grams("pizza", 300)}},
}

// fewCheats and manyCheats set the adherence of the visitor's group.
var (
	fewCheats  = []cheat{{3, "snack", []portion{grams("chocolate", 30)}}, {17, "dinner", []portion{grams("pizza", 300)}}}
	manyCheats = []cheat{
		{1, "dinner", []portion{grams("pizza", 350)}},
		{4, "snack", []portion{grams("chocolate", 50)}},
		{6, "lunch", []portion{grams("pizza", 300)}},
		{9, "dinner", []portion{grams("pizza", 350)}},
		{13, "snack", []portion{grams("chocolate", 40)}},
		{16, "dinner", []portion{grams("pizza", 350)}},
		{20, "lunch", []portion{grams("pizza", 300)}},
		{23, "dinner", []portion{grams("pizza", 350)}},
		{27, "snack", []portion{grams("chocolate", 50)}},
	}
)

// workouts by day of the week (0 = Monday), as workout_activities codes.
var workouts = map[int]struct {
	code    string
	minutes int
}{
	0: {"gym", 60},
	3: {"running", 40},
	5: {"cycling", 75},
}

// shopItem is added by hand to this week's shopping list ("shop.<key>").
type shopItem struct {
	key, qty string
	checked  bool
}

var shopping = []shopItem{
	{"coffee", "2", false},
	{"dish_soap", "1", true},
	{"sparkling_water", "6", false},
}

// barcode is a demo product's EAN-13 in language w.
func barcode(w vocabulary, f food) string {
	code := w.ean + f.ean
	sum := 0
	for i, r := range code {
		d := int(r - '0')
		if i%2 == 1 {
			d *= 3
		}
		sum += d
	}
	return code + strconv.Itoa((10-sum%10)%10)
}

// seed fills userID's account as p in language w, with dates relative to
// now: the current week planned in full, the weeks before it eaten, with
// cheats, water, workouts and weigh-ins up to today. It returns the ids of
// p's plans.
//
// The foods are a group's: an empty ings gets them made by userID, by key;
// the next people of the group use them, so the group's lists show each food
// once.
func seed(ctx context.Context, tx *sql.Tx, userID int64, w vocabulary, p person, ings map[string]int64, now time.Time) ([]int64, error) {
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	monday := db.MondayOf(today)
	exec := func(q string, args ...any) (int64, error) {
		res, err := tx.ExecContext(ctx, q, args...)
		if err != nil {
			return 0, err
		}
		return res.LastInsertId()
	}

	cats := map[string]int64{}
	for i, c := range categories {
		id, err := exec(`INSERT INTO ingredient_categories (user_id, name, sort_order) VALUES (?, ?, ?)`, userID, w.text["category."+c], i)
		if err != nil {
			return nil, fmt.Errorf("category: %w", err)
		}
		cats[c] = id
	}
	categorize := func(f food, id int64) error {
		if f.category == "" {
			return nil
		}
		if _, err := exec(`INSERT INTO ingredient_user_categories (user_id, ingredient_id, category_id) VALUES (?, ?, ?)`,
			userID, id, cats[f.category]); err != nil {
			return fmt.Errorf("food %s category: %w", f.key, err)
		}
		return nil
	}

	for _, f := range foods {
		id, ok := ings[f.key]
		if ok {
			if err := categorize(f, id); err != nil {
				return nil, err
			}
			continue
		}
		var pieceG any
		if f.pieceG > 0 {
			pieceG = f.pieceG
		}
		name := w.text["food."+f.key]
		if f.ean == "" {
			var err error
			id, err = exec(`INSERT INTO ingredients (source, name, unit, kcal_per_100, protein_per_100, carbs_per_100, fat_per_100, created_by, unit_weight_g)
				VALUES ('local', ?, ?, ?, ?, ?, ?, ?, ?)`, name, f.unit, f.kcal, f.protein, f.carbs, f.fat, userID, pieceG)
			if err != nil {
				return nil, fmt.Errorf("food %s: %w", f.key, err)
			}
		} else {
			var err error
			if id, err = product(ctx, tx, w, f, now); err != nil {
				return nil, err
			}
		}
		ings[f.key] = id
		if err := categorize(f, id); err != nil {
			return nil, err
		}
	}

	recs := map[string]int64{}
	for _, r := range recipes {
		id, err := exec(`INSERT INTO recipes (user_id, name, servings) VALUES (?, ?, ?)`, userID, w.text["recipe."+r.key], r.servings)
		if err != nil {
			return nil, fmt.Errorf("recipe %s: %w", r.key, err)
		}
		recs[r.key] = id
		for i, it := range r.items {
			if _, err := exec(`INSERT INTO recipe_ingredients (recipe_id, ingredient_id, quantity, sort_order) VALUES (?, ?, ?, ?)`,
				id, ings[it.food], it.qty, i); err != nil {
				return nil, fmt.Errorf("recipe %s: %w", r.key, err)
			}
		}
	}
	// ids returns a portion's recipe and ingredient, one of them nil.
	ids := func(it portion) (recipeID, ingredientID any) {
		if it.recipe != "" {
			return recs[it.recipe], nil
		}
		return nil, ings[it.food]
	}

	var plans []int64
	for n, def := range p.plans {
		planID, err := exec(`INSERT INTO nutritionist_plans (user_id, name, is_active) VALUES (?, ?, ?)`, userID, w.text[def.key], n == 0)
		if err != nil {
			return nil, fmt.Errorf("plan: %w", err)
		}
		plans = append(plans, planID)
		for i, mt := range mealTypes {
			typeID, err := exec(`INSERT INTO nutritionist_meal_types (plan_id, name, sort_order) VALUES (?, ?, ?)`, planID, w.text["meal."+mt.key], i)
			if err != nil {
				return nil, fmt.Errorf("plan meal: %w", err)
			}
			for j, v := range mt.variants {
				varID, err := exec(`INSERT INTO nutritionist_meal_variations (meal_type_id, name, sort_order) VALUES (?, ?, ?)`, typeID, w.text["variant."+v.key], j)
				if err != nil {
					return nil, fmt.Errorf("plan variant: %w", err)
				}
				for k, it := range v.items {
					rid, iid := ids(it)
					qty := it.qty
					if it.food != "" && it.unit == "g" && def.scale != 1 {
						qty = max(5, math.Round(qty*def.scale/5)*5)
					}
					if _, err := exec(`INSERT INTO nutritionist_meal_items (variation_id, recipe_id, ingredient_id, quantity, quantity_unit, sort_order)
						VALUES (?, ?, ?, ?, ?, ?)`, varID, rid, iid, qty, it.unit, k); err != nil {
						return nil, fmt.Errorf("plan item: %w", err)
					}
				}
			}
		}
	}
	planID := plans[0]

	// The weeks, oldest first: the plan's variants in turn, then what was
	// logged on the days up to today.
	cheatOn := map[int]cheat{}
	for _, c := range p.cheats {
		cheatOn[c.daysAgo] = c
	}
	insMeal, err := tx.PrepareContext(ctx, `INSERT INTO meals (weekly_plan_id, day_of_week, name, sort_order) VALUES (?, ?, ?, ?)`)
	if err != nil {
		return nil, err
	}
	defer insMeal.Close()
	insItem, err := tx.PrepareContext(ctx, `INSERT INTO meal_items (meal_id, recipe_id, ingredient_id, quantity, quantity_unit, sort_order) VALUES (?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return nil, err
	}
	defer insItem.Close()
	insWater, err := tx.PrepareContext(ctx, `INSERT INTO water_logs (user_id, logged_at, amount_ml) VALUES (?, ?, ?)`)
	if err != nil {
		return nil, err
	}
	defer insWater.Close()

	for wk := p.weeks; wk >= 0; wk-- {
		start := monday.AddDate(0, 0, -7*wk)
		// The app creates empty weeks just by showing them (SeedAccount).
		var weekID int64
		if err := tx.QueryRowContext(ctx, `INSERT INTO weekly_plans (user_id, week_start, source_plan_id) VALUES (?, ?, ?)
			ON CONFLICT (user_id, week_start) DO UPDATE SET source_plan_id = excluded.source_plan_id RETURNING id`,
			userID, start.Format("2006-01-02"), planID).Scan(&weekID); err != nil {
			return nil, fmt.Errorf("week: %w", err)
		}
		for d := range 7 {
			date := start.AddDate(0, 0, d)
			daysAgo := int(math.Round(today.Sub(date).Hours() / 24))
			for i, mt := range mealTypes {
				res, err := insMeal.ExecContext(ctx, weekID, d, w.text["meal."+mt.key], i)
				if err != nil {
					return nil, fmt.Errorf("meal: %w", err)
				}
				mealID, err := res.LastInsertId()
				if err != nil {
					return nil, err
				}
				v := mt.variants[(d+wk+i)%len(mt.variants)]
				for k, it := range v.items {
					rid, iid := ids(it)
					if _, err := insItem.ExecContext(ctx, mealID, rid, iid, it.qty, it.unit, k); err != nil {
						return nil, fmt.Errorf("meal item: %w", err)
					}
				}
				if c, ok := cheatOn[daysAgo]; ok && daysAgo >= 0 && c.meal == mt.key {
					for k, it := range c.items {
						rid, iid := ids(it)
						if _, err := exec(`INSERT INTO meal_cheat_items (meal_id, recipe_id, ingredient_id, quantity, sort_order) VALUES (?, ?, ?, ?, ?)`,
							mealID, rid, iid, it.qty, k); err != nil {
							return nil, fmt.Errorf("cheat: %w", err)
						}
					}
				}
			}
			if daysAgo < 0 {
				continue // the rest of this week is still to come
			}
			if err := logDay(ctx, tx, insWater, userID, p.body.weightKg, date, d, daysAgo); err != nil {
				return nil, err
			}
		}
	}

	var listID int64
	if err := tx.QueryRowContext(ctx, `INSERT INTO shopping_lists (user_id, week_start, week_count) VALUES (?, ?, 1)
		ON CONFLICT (user_id, week_start) DO UPDATE SET week_count = week_count RETURNING id`,
		userID, monday.Format("2006-01-02")).Scan(&listID); err != nil {
		return nil, fmt.Errorf("shopping list: %w", err)
	}
	for i, s := range shopping {
		if _, err := exec(`INSERT INTO shopping_user_items (shopping_list_id, description, quantity_text, checked, sort_order) VALUES (?, ?, ?, ?, ?)`,
			listID, w.text["shop."+s.key], s.qty, s.checked, i); err != nil {
			return nil, fmt.Errorf("shopping item: %w", err)
		}
	}
	return plans, nil
}

// product returns the demo OpenFoodFacts product f in language w, creating
// it when missing. Products are shared by every account, as imported ones are;
// Prepare creates them all before any visitor can claim their barcodes.
func product(ctx context.Context, tx *sql.Tx, w vocabulary, f food, now time.Time) (int64, error) {
	code, name := barcode(w, f), w.text["food."+f.key]
	var pieceG any
	if f.pieceG > 0 {
		pieceG = f.pieceG
	}
	if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO off_products (barcode, product_name, brands, quantity,
		kcal_per_100, protein_per_100, carbs_per_100, fat_per_100, has_full_nutrients, serving_g, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, 1, ?, ?)`,
		code, name, demoBrand, f.pack, f.kcal, f.protein, f.carbs, f.fat, f.servingG, now.Unix()); err != nil {
		return 0, fmt.Errorf("product %s: %w", f.key, err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO ingredients (source, off_barcode, name, brand, unit, kcal_per_100, protein_per_100, carbs_per_100, fat_per_100,
		unit_weight_g, default_quantity, last_verified_at)
		VALUES ('openfoodfacts', ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT (off_barcode) DO NOTHING`,
		code, name, demoBrand, f.unit, f.kcal, f.protein, f.carbs, f.fat, pieceG, f.servingG); err != nil {
		return 0, fmt.Errorf("product %s: %w", f.key, err)
	}
	var id int64
	if err := tx.QueryRowContext(ctx, `SELECT id FROM ingredients WHERE off_barcode = ? AND source = 'openfoodfacts'`, code).Scan(&id); err != nil {
		return 0, fmt.Errorf("product %s (barcode %s): %w", f.key, code, err)
	}
	return id, nil
}

// products creates every language's demo products (Prepare).
func products(ctx context.Context, tx *sql.Tx, now time.Time) error {
	for _, w := range words {
		for _, f := range foods {
			if f.ean == "" {
				continue
			}
			if _, err := product(ctx, tx, w, f, now); err != nil {
				return err
			}
		}
	}
	return nil
}

// logDay writes what was logged on a past day (or today): water, the day's
// workout and, every third day, a weigh-in on a slow downward trend that
// ends at kg today.
func logDay(ctx context.Context, tx *sql.Tx, insWater *sql.Stmt, userID int64, kg float64, date time.Time, weekday, daysAgo int) error {
	day := date.Format("2006-01-02")
	water := []int{500, 750, 250 * (1 + daysAgo%3)}
	if daysAgo == 0 {
		water = []int{500, 250} // the day is not over
	}
	for _, ml := range water {
		if _, err := insWater.ExecContext(ctx, userID, day, ml); err != nil {
			return fmt.Errorf("water: %w", err)
		}
	}
	if wo, ok := workouts[weekday]; ok {
		if _, err := tx.ExecContext(ctx, `INSERT INTO workout_logs (user_id, activity_id, logged_at, duration_minutes, kcal_burned)
			SELECT ?, id, ?, ?, round(kcal_per_hour / 60.0 * ?, 1) FROM workout_activities WHERE code = ?`,
			userID, day, wo.minutes, wo.minutes, wo.code); err != nil {
			return fmt.Errorf("workout: %w", err)
		}
	}
	if daysAgo%3 == 0 {
		wobble := []float64{0, 0.3, -0.2, 0.2}[(daysAgo/3)%4]
		w := math.Round((kg+0.05*float64(daysAgo)+wobble)*10) / 10
		if _, err := tx.ExecContext(ctx, `INSERT INTO weight_logs (user_id, measured_at, weight_kg) VALUES (?, ?, ?)
			ON CONFLICT (user_id, measured_at) DO UPDATE SET weight_kg = excluded.weight_kg`, userID, day, w); err != nil {
			return fmt.Errorf("weigh-in: %w", err)
		}
	}
	return nil
}

// SeedAccount fills an account someone signed up for with the demo data in
// lang, so a local database can be dressed for the README screenshots. An
// account with meals already is refused; the plan it may have is kept,
// inactive.
func SeedAccount(ctx context.Context, d *sql.DB, email, lang string) error {
	var userID int64
	var meals int
	if err := d.QueryRowContext(ctx, `SELECT u.id, (SELECT count(*) FROM meals m JOIN weekly_plans w ON w.id = m.weekly_plan_id WHERE w.user_id = u.id)
		FROM users u WHERE u.email = ?`, email).Scan(&userID, &meals); err != nil {
		return fmt.Errorf("demo seed: account %q: %w", email, err)
	}
	if meals > 0 {
		return fmt.Errorf("demo seed: account %q already has meals; use a new one", email)
	}
	tx, err := d.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `UPDATE nutritionist_plans SET is_active = 0 WHERE user_id = ?`, userID); err != nil {
		return fmt.Errorf("demo seed: %w", err)
	}
	if _, err := seed(ctx, tx, userID, words[pickCode(lang)], visitor, map[string]int64{}, time.Now()); err != nil {
		return fmt.Errorf("demo seed: %w", err)
	}
	return tx.Commit()
}
