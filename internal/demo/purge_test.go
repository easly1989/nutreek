package demo

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/amon2126/nutreek/internal/domain"
)

// shared lists the tables that hold no user's data: catalogs, the
// bookkeeping of goose, scs and OpenFoodFacts, and the demo's marker.
var shared = map[string]bool{
	"allergen_tags": true, "features": true, "goose_db_version": true, "sessions": true,
	"sqlite_sequence": true, "workout_activities": true, "demo_meta": true,
	"off_products": true, "off_products_fts": true, "off_products_fts_config": true,
	"off_products_fts_data": true, "off_products_fts_docsize": true, "off_products_fts_idx": true,
	"off_sync_state": true, "instance_settings": true,
}

// counts returns the rows in every table.
func counts(t *testing.T, d *sql.DB) map[string]int {
	t.Helper()
	rows, err := d.Query(`SELECT name FROM sqlite_master WHERE type = 'table'`)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			t.Fatal(err)
		}
		names = append(names, n)
	}
	if err := rows.Close(); err != nil {
		t.Fatal(err)
	}
	out := map[string]int{}
	for _, n := range names {
		var c int
		if err := d.QueryRow(`SELECT count(*) FROM "` + n + `"`).Scan(&c); err != nil {
			t.Fatal(err)
		}
		out[n] = c
	}
	return out
}

// fill gives a demo group a second user, a coach, and a row in every table
// that holds a user's data, the way the app writes them. It returns the
// ingredient the user created.
func fill(t *testing.T, d *sql.DB, userID int64) int64 {
	t.Helper()
	ctx := context.Background()
	exec := func(q string, args ...any) int64 {
		t.Helper()
		res, err := d.ExecContext(ctx, q, args...)
		if err != nil {
			t.Fatalf("%s: %v", q, err)
		}
		id, _ := res.LastInsertId()
		return id
	}
	var groupID int64
	if err := d.QueryRow(`SELECT group_id FROM demo_members WHERE user_id = ?`, userID).Scan(&groupID); err != nil {
		t.Fatal(err)
	}
	tx, err := d.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	coach, err := createUser(ctx, tx, "Coach", persona, domain.RoleCoach)
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	exec(`INSERT INTO demo_members (user_id, group_id) VALUES (?, ?)`, coach, groupID)

	cat := exec(`INSERT INTO ingredient_categories (user_id, name) VALUES (?, 'Mine')`, userID)
	ing := exec(`INSERT INTO ingredients (source, name, kcal_per_100, protein_per_100, carbs_per_100, fat_per_100, created_by, category_id)
		VALUES ('local', 'Oats', 380, 13, 60, 7, ?, ?)`, userID, cat)
	exec(`INSERT INTO ingredient_user_categories (user_id, ingredient_id, category_id) VALUES (?, ?, ?)`, userID, ing, cat)
	exec(`INSERT INTO ingredient_allergens (ingredient_id, allergen_id) SELECT ?, id FROM allergen_tags LIMIT 1`, ing)
	recipe := exec(`INSERT INTO recipes (user_id, name) VALUES (?, 'Porridge')`, userID)
	exec(`INSERT INTO recipe_ingredients (recipe_id, ingredient_id, quantity) VALUES (?, ?, 80)`, recipe, ing)
	exec(`INSERT INTO recipe_photos (recipe_id, photo, thumb, etag) VALUES (?, x'FFD8', x'FFD8', 'e')`, recipe)
	tpl := exec(`INSERT INTO meal_templates (user_id, name) VALUES (?, 'Week')`, userID)
	exec(`INSERT INTO meal_template_slots (template_id, day_of_week, name) VALUES (?, 1, 'Breakfast')`, tpl)
	plan := exec(`INSERT INTO nutritionist_plans (user_id) VALUES (?)`, coach)
	mt := exec(`INSERT INTO nutritionist_meal_types (plan_id, name) VALUES (?, 'Breakfast')`, plan)
	v := exec(`INSERT INTO nutritionist_meal_variations (meal_type_id) VALUES (?)`, mt)
	exec(`INSERT INTO nutritionist_meal_items (variation_id, ingredient_id, quantity) VALUES (?, ?, 50)`, v, ing)
	exec(`INSERT INTO coach_clients (coach_id, client_id, assigned_plan_id) VALUES (?, ?, ?)`, coach, userID, plan)
	week := exec(`INSERT INTO weekly_plans (user_id, week_start, template_id, source_plan_id) VALUES (?, '2020-01-06', ?, ?)`, userID, tpl, plan)
	meal := exec(`INSERT INTO meals (weekly_plan_id, day_of_week, name) VALUES (?, 1, 'Breakfast')`, week)
	item := exec(`INSERT INTO meal_items (meal_id, ingredient_id, quantity) VALUES (?, ?, 80)`, meal, ing)
	exec(`INSERT INTO meal_items (meal_id, recipe_id, quantity) VALUES (?, ?, 1)`, meal, recipe)
	exec(`INSERT INTO meal_item_customizations (meal_item_id, ingredient_id, quantity) VALUES (?, ?, 90)`, item, ing)
	exec(`INSERT INTO meal_cheat_items (meal_id, ingredient_id, quantity) VALUES (?, ?, 30)`, meal, ing)
	list := exec(`INSERT INTO shopping_lists (user_id, week_start) VALUES (?, '2020-01-06')`, userID)
	exec(`INSERT INTO shopping_list_items (shopping_list_id, ingredient_id, total_quantity) VALUES (?, ?, 80)`, list, ing)
	exec(`INSERT INTO shopping_user_items (shopping_list_id, description) VALUES (?, 'Milk')`, list)
	exec(`INSERT INTO weight_logs (user_id, measured_at, weight_kg) VALUES (?, '2020-01-06', 72)`, userID)
	exec(`INSERT INTO water_logs (user_id, logged_at, amount_ml) VALUES (?, '2020-01-06', 250)`, userID)
	exec(`INSERT INTO workout_logs (user_id, activity_id, logged_at, duration_minutes, kcal_burned)
		SELECT ?, id, '2020-01-06', 30, 200 FROM workout_activities LIMIT 1`, userID)
	exec(`INSERT INTO api_tokens (user_id, name, token_hash) VALUES (?, 'cli', ?)`, userID, "hash-"+time.Now().String())
	exec(`INSERT INTO subscriptions (user_id) VALUES (?)`, userID)
	return ing
}

// age makes a group look unused for d.
func age(t *testing.T, d *sql.DB, userID int64, by time.Duration) {
	t.Helper()
	if _, err := d.Exec(`UPDATE demo_groups SET last_seen_at = ?
		WHERE id = (SELECT group_id FROM demo_members WHERE user_id = ?)`,
		time.Now().UTC().Add(-by).Format(sqliteTime), userID); err != nil {
		t.Fatal(err)
	}
}

func prepared(t *testing.T) *sql.DB {
	t.Helper()
	d := newStore(t)
	if err := Prepare(context.Background(), d); err != nil {
		t.Fatal(err)
	}
	return d
}

func TestPurgeIdle(t *testing.T) {
	ctx := context.Background()
	d := prepared(t)
	svc := New(d, Config{Idle: 2 * time.Hour})

	live, err := svc.Start(ctx, "en", AsPlan)
	if err != nil {
		t.Fatal(err)
	}
	fill(t, d, live)
	before := counts(t, d)

	idle, err := svc.Start(ctx, "it", AsPlan)
	if err != nil {
		t.Fatal(err)
	}
	fill(t, d, idle)
	// The fill must reach every table with a user's data, so the purge is
	// checked against each of them: a new table needs a row here.
	for name, n := range counts(t, d) {
		if !shared[name] && n <= before[name] {
			t.Errorf("fill wrote nothing to %s: add it, and check the purge deletes it", name)
		}
	}
	age(t, d, live, 2*time.Hour) // its session has just expired: it waits out the margin
	age(t, d, idle, 2*time.Hour+2*time.Minute)

	n, err := svc.Purge(ctx, false)
	if err != nil {
		t.Fatalf("purge: %v", err)
	}
	if n != 1 {
		t.Errorf("purged %d groups, want 1", n)
	}
	after := counts(t, d)
	for name, want := range before {
		if name != "sqlite_sequence" && after[name] != want {
			t.Errorf("%s: %d rows, want %d", name, after[name], want)
		}
	}
	if _, err := svc.Purge(ctx, false); err != nil {
		t.Fatalf("purge again: %v", err)
	}
	if got := counts(t, d)["users"]; got != before["users"] {
		t.Errorf("the live group lost users: %d, want %d", got, before["users"])
	}
}

// An ingredient another group still uses outlives its author, with no author.
func TestPurgeKeepsSharedIngredient(t *testing.T) {
	ctx := context.Background()
	d := prepared(t)
	svc := New(d, Config{Idle: time.Hour})
	idle, err := svc.Start(ctx, "en", AsPlan)
	if err != nil {
		t.Fatal(err)
	}
	ing := fill(t, d, idle)
	other, err := svc.Start(ctx, "en", AsPlan)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := d.Exec(`INSERT INTO recipes (user_id, name) VALUES (?, 'Bowl')`, other); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Exec(`INSERT INTO recipe_ingredients (recipe_id, ingredient_id, quantity)
		VALUES ((SELECT id FROM recipes WHERE user_id = ?), ?, 40)`, other, ing); err != nil {
		t.Fatal(err)
	}
	// A second one only another group's nutrition plan uses: that reference
	// would lose it silently (ON DELETE SET NULL).
	var planned int64
	if err := d.QueryRow(`INSERT INTO ingredients (source, name, kcal_per_100, protein_per_100, carbs_per_100, fat_per_100, created_by)
		VALUES ('local', 'Rice', 360, 7, 80, 1, ?) RETURNING id`, idle).Scan(&planned); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Exec(`INSERT INTO nutritionist_plans (user_id, is_active) VALUES (?, 0)`, other); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Exec(`INSERT INTO nutritionist_meal_types (plan_id, name) SELECT id, 'Lunch' FROM nutritionist_plans WHERE user_id = ? AND is_active = 0`, other); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Exec(`INSERT INTO nutritionist_meal_variations (meal_type_id) SELECT max(id) FROM nutritionist_meal_types`); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Exec(`INSERT INTO nutritionist_meal_items (variation_id, ingredient_id, quantity) SELECT max(id), ?, 70 FROM nutritionist_meal_variations`, planned); err != nil {
		t.Fatal(err)
	}
	age(t, d, idle, 2*time.Hour)
	if _, err := svc.Purge(ctx, false); err != nil {
		t.Fatalf("purge: %v", err)
	}
	for _, id := range []int64{ing, planned} {
		var author sql.NullInt64
		if err := d.QueryRow(`SELECT created_by FROM ingredients WHERE id = ?`, id).Scan(&author); err != nil {
			t.Fatalf("shared ingredient %d is gone: %v", id, err)
		}
		if author.Valid {
			t.Errorf("shared ingredient %d still names its deleted author %d", id, author.Int64)
		}
	}
}

func TestPurgeAll(t *testing.T) {
	ctx := context.Background()
	d := prepared(t)
	before := counts(t, d)
	svc := New(d, Config{Idle: time.Hour})
	for _, lang := range []string{"en", "it"} {
		id, err := svc.Start(ctx, lang, AsPlan)
		if err != nil {
			t.Fatal(err)
		}
		fill(t, d, id)
	}
	// A demo user left without a group goes too.
	if _, err := d.Exec(`DELETE FROM demo_groups WHERE id = (SELECT max(id) FROM demo_groups)`); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Exec(`INSERT INTO sessions (token, data, expiry) VALUES ('t', x'00', 9e9)`); err != nil {
		t.Fatal(err)
	}

	if _, err := svc.Purge(ctx, true); err != nil {
		t.Fatalf("purge: %v", err)
	}
	after := counts(t, d)
	for name, want := range before {
		if name != "sqlite_sequence" && after[name] != want {
			t.Errorf("%s: %d rows, want %d", name, after[name], want)
		}
	}
}

func TestNextNightly(t *testing.T) {
	rome := time.FixedZone("CEST", 2*3600)
	for _, c := range []struct{ now, want time.Time }{
		{time.Date(2026, 10, 17, 1, 0, 0, 0, time.UTC), time.Date(2026, 10, 17, 3, 0, 0, 0, time.UTC)},
		{time.Date(2026, 10, 17, 3, 0, 0, 0, time.UTC), time.Date(2026, 10, 18, 3, 0, 0, 0, time.UTC)},
		{time.Date(2026, 10, 17, 22, 30, 0, 0, time.UTC), time.Date(2026, 10, 18, 3, 0, 0, 0, time.UTC)},
		{time.Date(2026, 12, 31, 4, 0, 0, 0, time.UTC), time.Date(2027, 1, 1, 3, 0, 0, 0, time.UTC)},
		// 04:30 in Rome is 02:30 UTC: the purge is half an hour away.
		{time.Date(2026, 10, 17, 4, 30, 0, 0, rome), time.Date(2026, 10, 17, 3, 0, 0, 0, time.UTC)},
	} {
		if got := NextNightly(c.now); !got.Equal(c.want) {
			t.Errorf("NextNightly(%v) = %v, want %v", c.now, got, c.want)
		}
	}
}

func TestSeen(t *testing.T) {
	ctx := context.Background()
	d := prepared(t)
	svc := New(d, Config{Idle: time.Hour})
	id, err := svc.Start(ctx, "en", AsPlan)
	if err != nil {
		t.Fatal(err)
	}
	lastSeen := func() time.Duration {
		t.Helper()
		var s string
		if err := d.QueryRow(`SELECT g.last_seen_at FROM demo_groups g JOIN demo_members m ON m.group_id = g.id
			WHERE m.user_id = ?`, id).Scan(&s); err != nil {
			t.Fatal(err)
		}
		at, err := time.Parse(sqliteTime, s)
		if err != nil {
			// The driver may hand DATETIME back as RFC 3339.
			if at, err = time.Parse(time.RFC3339, s); err != nil {
				t.Fatal(err)
			}
		}
		return time.Since(at)
	}

	age(t, d, id, 30*time.Minute)
	if err := svc.Seen(ctx, id); err != nil {
		t.Fatal(err)
	}
	if ago := lastSeen(); ago > time.Minute {
		t.Errorf("seen %v ago after a request", ago)
	}
	// Within the minute, a request writes nothing.
	age(t, d, id, 30*time.Minute)
	if err := svc.Seen(ctx, id); err != nil {
		t.Fatal(err)
	}
	if ago := lastSeen(); ago < 29*time.Minute {
		t.Errorf("touched again within the minute: seen %v ago", ago)
	}
	// A user the demo does not know is no error.
	if err := svc.Seen(ctx, id+100); err != nil {
		t.Errorf("unknown user: %v", err)
	}
}
