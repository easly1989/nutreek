package demo

import (
	"context"
	"database/sql"
	"maps"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/amon2126/nutreek/internal/db"
	"github.com/amon2126/nutreek/internal/domain"
	"github.com/amon2126/nutreek/internal/service"
)

func newStore(t *testing.T) *sql.DB {
	t.Helper()
	store, err := db.Open(filepath.Join(t.TempDir(), "demo.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := store.Migrate(context.Background()); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return store.DB
}

func TestPrepare(t *testing.T) {
	ctx := context.Background()

	t.Run("empty database becomes a demo one", func(t *testing.T) {
		d := newStore(t)
		if err := Prepare(ctx, d); err != nil {
			t.Fatalf("prepare: %v", err)
		}
		// Again, as at every restart: still fine.
		if _, err := New(d, Config{}).Start(ctx, "en", AsPlan); err != nil {
			t.Fatalf("start: %v", err)
		}
		if err := Prepare(ctx, d); err != nil {
			t.Fatalf("prepare again: %v", err)
		}
	})

	t.Run("refuses a database with real users", func(t *testing.T) {
		d := newStore(t)
		if _, err := db.NewUsers(d).Create(ctx, "real@example.test", "hash", "Real"); err != nil {
			t.Fatal(err)
		}
		if err := Prepare(ctx, d); err == nil {
			t.Fatal("prepare accepted a database with a real user")
		}
		var tables int
		if err := d.QueryRowContext(ctx, `SELECT count(*) FROM sqlite_master WHERE name LIKE 'demo_%'`).Scan(&tables); err != nil || tables != 0 {
			t.Errorf("a refused database got %d demo tables (%v)", tables, err)
		}
	})

	t.Run("a demo user left without a group does not block a restart", func(t *testing.T) {
		d := newStore(t)
		if err := Prepare(ctx, d); err != nil {
			t.Fatal(err)
		}
		if _, err := New(d, Config{}).Start(ctx, "en", AsPlan); err != nil {
			t.Fatal(err)
		}
		if _, err := d.ExecContext(ctx, `DELETE FROM demo_groups`); err != nil {
			t.Fatal(err)
		}
		if err := Prepare(ctx, d); err != nil {
			t.Fatalf("prepare: %v", err)
		}
	})

	t.Run("refuses a demo database where a real user turned up", func(t *testing.T) {
		d := newStore(t)
		if err := Prepare(ctx, d); err != nil {
			t.Fatal(err)
		}
		if _, err := db.NewUsers(d).Create(ctx, "real@example.test", "hash", "Real"); err != nil {
			t.Fatal(err)
		}
		if err := Prepare(ctx, d); err == nil {
			t.Fatal("prepare accepted a user the demo did not create")
		}
	})
}

func TestStart(t *testing.T) {
	ctx := context.Background()
	d := newStore(t)
	if err := Prepare(ctx, d); err != nil {
		t.Fatal(err)
	}
	svc := New(d, Config{Idle: 90 * time.Minute})
	if h := svc.IdleHours(); h != 2 {
		t.Errorf("idle hours %d, want 2", h)
	}
	users := db.NewUsers(d)

	for _, lang := range []string{"en", "it", "xx"} {
		id, err := svc.Start(ctx, lang, AsPlan)
		if err != nil {
			t.Fatalf("%s: start: %v", lang, err)
		}
		u, err := users.GetByID(ctx, id)
		if err != nil {
			t.Fatalf("%s: get: %v", lang, err)
		}
		if u.OnboardingCompletedAt == nil || u.TourDismissedAt == nil {
			t.Errorf("%s: onboarding or tour still pending", lang)
		}
		if u.WeightKg == nil || u.HeightCm == nil || u.BirthYear == nil || u.Sex == nil || u.ActivityLevel == nil ||
			u.TargetKcal == nil || u.TargetProtein == nil || u.TargetCarbs == nil || u.TargetFat == nil ||
			u.BMI == nil || u.BMR == nil {
			t.Errorf("%s: incomplete profile: %+v", lang, u)
		}
		if !strings.HasSuffix(u.Email, emailDomain) {
			t.Errorf("%s: email %q", lang, u.Email)
		}
		names := words[pickCode(lang)].names
		if !slices.Contains(names, u.DisplayName) {
			t.Errorf("%s: name %q not among %v", lang, u.DisplayName, names)
		}
		var groupLang string
		if err := d.QueryRowContext(ctx, `SELECT g.lang FROM demo_members m JOIN demo_groups g ON g.id = m.group_id WHERE m.user_id = ?`, id).Scan(&groupLang); err != nil {
			t.Fatalf("%s: group: %v", lang, err)
		}
		if want := pickCode(lang); groupLang != want {
			t.Errorf("%s: group language %q, want %q", lang, groupLang, want)
		}
	}
}

// Each way in creates a whole group (#186): following a plan alone; a coach
// with clients and a plan library; a coach's client on the plan the coach
// assigned. The visitor gets the long history, the others a short one.
func TestStartGroups(t *testing.T) {
	ctx := context.Background()
	d := newStore(t)
	if err := Prepare(ctx, d); err != nil {
		t.Fatal(err)
	}
	svc := New(d, Config{})
	users, coaches, plans := db.NewUsers(d), db.NewCoachRepository(d), db.NewNutritionistPlans(d)
	planner := &service.PlannerService{NutritionistPlans: plans, Coach: coaches}

	count := func(q string, args ...any) int {
		t.Helper()
		var n int
		if err := d.QueryRowContext(ctx, q, args...).Scan(&n); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
		return n
	}
	weeks := func(id int64) int {
		return count(`SELECT count(DISTINCT w.id) FROM weekly_plans w JOIN meals m ON m.weekly_plan_id = w.id WHERE w.user_id = ?`, id)
	}
	cheats := func(id int64) int {
		return count(`SELECT count(*) FROM meal_cheat_items c JOIN meals m ON m.id = c.meal_id JOIN weekly_plans w ON w.id = m.weekly_plan_id WHERE w.user_id = ?`, id)
	}
	group := func(lang string, visitor int64) []int64 {
		t.Helper()
		rows, err := d.QueryContext(ctx, `SELECT user_id FROM demo_members WHERE group_id = (SELECT group_id FROM demo_members WHERE user_id = ?) ORDER BY user_id`, visitor)
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		var ids []int64
		seen := map[string]bool{}
		for rows.Next() {
			var id int64
			if err := rows.Scan(&id); err != nil {
				t.Fatal(err)
			}
			ids = append(ids, id)
			u, err := users.GetByID(ctx, id)
			if err != nil {
				t.Fatal(err)
			}
			if !slices.Contains(words[lang].names, u.DisplayName) || seen[u.DisplayName] {
				t.Errorf("%s: name %q not among %v, or twice in the group", lang, u.DisplayName, words[lang].names)
			}
			seen[u.DisplayName] = true
			if !strings.HasSuffix(u.Email, emailDomain) {
				t.Errorf("email %q", u.Email)
			}
			want := otherWeeks + 1
			if id == visitor {
				want = historyWeeks + 1
			}
			if n := weeks(id); n != want {
				t.Errorf("user %d: %d weeks, want %d", id, n, want)
			}
		}
		if len(ids) == 0 || ids[0] != visitor {
			t.Errorf("the visitor %d is not the group's first member: %v", visitor, ids)
		}
		// The group shares one set of hand-made foods, the visitor's.
		local := 0
		for _, f := range foods {
			if f.ean == "" {
				local++
			}
		}
		if n := count(`SELECT count(*) FROM ingredients WHERE created_by IN (SELECT user_id FROM demo_members WHERE group_id = (SELECT group_id FROM demo_members WHERE user_id = ?))`, visitor); n != local {
			t.Errorf("the group made %d foods, want %d", n, local)
		}
		return ids
	}

	t.Run("plan", func(t *testing.T) {
		id, err := svc.Start(ctx, "en", AsPlan)
		if err != nil {
			t.Fatal(err)
		}
		if g := group("en", id); len(g) != 1 {
			t.Errorf("group of %d, want the visitor alone", len(g))
		}
	})

	t.Run("coach", func(t *testing.T) {
		id, err := svc.Start(ctx, "it", AsCoach)
		if err != nil {
			t.Fatal(err)
		}
		if u, _ := users.GetByID(ctx, id); u.Role != domain.RoleCoach {
			t.Errorf("role %q", u.Role)
		}
		g := group("it", id)
		library, err := plans.ListPlans(ctx, id)
		if err != nil || len(library) < 2 {
			t.Fatalf("a library of %d plans (%v)", len(library), err)
		}
		for _, p := range library {
			if !slices.Contains(slices.Collect(maps.Values(words["it"].text)), p.Name) {
				t.Errorf("plan %q is not Italian", p.Name)
			}
		}
		clients, err := coaches.ListClients(ctx, id)
		if err != nil || len(clients) < 2 {
			t.Fatalf("%d clients (%v)", len(clients), err)
		}
		assigned := map[int64]bool{}
		adherence := map[int]bool{}
		for _, c := range clients {
			if c.AssignedPlanID == nil || !slices.ContainsFunc(library, func(p domain.NutritionistPlan) bool { return p.ID == *c.AssignedPlanID }) {
				t.Errorf("client %d: plan %v not from the library", c.ClientID, c.AssignedPlanID)
				continue
			}
			assigned[*c.AssignedPlanID] = true
			adherence[cheats(c.ClientID)] = true
		}
		if len(assigned) < 2 || len(adherence) < 2 {
			t.Errorf("clients alike: plans %v, cheats %v", assigned, adherence)
		}
		// One of the group is not a client yet, for the coach to add.
		if free := count(`SELECT count(*) FROM demo_members m WHERE m.group_id = (SELECT group_id FROM demo_members WHERE user_id = ?)
			AND m.user_id <> ? AND NOT EXISTS (SELECT 1 FROM coach_clients WHERE client_id = m.user_id)`, id, id); free != 1 || len(g) != len(clients)+2 {
			t.Errorf("%d of %d members free to add", free, len(g))
		}
	})

	t.Run("client", func(t *testing.T) {
		id, err := svc.Start(ctx, "en", AsClient)
		if err != nil {
			t.Fatal(err)
		}
		g := group("en", id)
		a, err := coaches.GetAssignment(ctx, id)
		if err != nil || a == nil || a.AssignedPlanID == nil {
			t.Fatalf("no plan assigned: %+v %v", a, err)
		}
		if !slices.Contains(g, a.CoachID) {
			t.Errorf("the coach %d is not in the group %v", a.CoachID, g)
		}
		if owner := count(`SELECT user_id FROM nutritionist_plans WHERE id = ?`, *a.AssignedPlanID); int64(owner) != a.CoachID {
			t.Errorf("the assigned plan is user %d's", owner)
		}
		eff, err := planner.EffectivePlanFull(ctx, id)
		if err != nil || eff.ID != *a.AssignedPlanID || len(eff.MealTypes) != len(mealTypes) {
			t.Fatalf("effective plan %+v (%v), want the assigned %d", eff, err, *a.AssignedPlanID)
		}
		if n := count(`SELECT count(*) FROM weekly_plans WHERE user_id = ? AND source_plan_id = ?`, id, *a.AssignedPlanID); n != historyWeeks+1 {
			t.Errorf("%d weeks from the assigned plan, want %d", n, historyWeeks+1)
		}
		if clients, _ := coaches.ListClients(ctx, a.CoachID); len(clients) < 2 {
			t.Errorf("the coach has %d clients", len(clients))
		}
	})
}

// Every language has a table with every word the demo shows.
func TestWords(t *testing.T) {
	for lang, w := range words {
		if len(w.names) == 0 {
			t.Errorf("%s: no names", lang)
		}
	}
	if _, ok := words["en"]; !ok {
		t.Fatal("English, the fallback, is missing")
	}
}

func TestLoadConfig(t *testing.T) {
	t.Setenv("NUTREEK_DEMO_IDLE", "")
	c, err := LoadConfig()
	if err != nil || c.Idle != 2*time.Hour {
		t.Fatalf("default: %v %v", c.Idle, err)
	}
	t.Setenv("NUTREEK_DEMO_IDLE", "30m")
	if c, err = LoadConfig(); err != nil || c.Idle != 30*time.Minute {
		t.Fatalf("30m: %v %v", c.Idle, err)
	}
	for _, bad := range []string{"soon", "-1h", "0s"} {
		t.Setenv("NUTREEK_DEMO_IDLE", bad)
		if _, err := LoadConfig(); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
	t.Setenv("NUTREEK_DEMO_IDLE", "")
}

func TestLoadConfigLimits(t *testing.T) {
	vars := []string{"NUTREEK_DEMO_MAX_USERS", "NUTREEK_DEMO_STARTS_PER_HOUR", "NUTREEK_DEMO_MAX_BODY", "NUTREEK_DEMO_MAX_INGREDIENTS", "NUTREEK_DEMO_MAX_RECIPES"}
	for _, v := range vars {
		t.Setenv(v, "")
	}
	c, err := LoadConfig()
	if err != nil || c.MaxUsers != 200 || c.StartsPerHour != 10 || c.MaxBody != 1<<20 || c.MaxIngredients != 300 || c.MaxRecipes != 100 {
		t.Fatalf("defaults: %+v %v", c, err)
	}
	for i, v := range vars {
		t.Setenv(v, strconv.Itoa(i+1))
	}
	if c, err = LoadConfig(); err != nil || c.MaxUsers != 1 || c.StartsPerHour != 2 || c.MaxBody != 3 || c.MaxIngredients != 4 || c.MaxRecipes != 5 {
		t.Fatalf("set: %+v %v", c, err)
	}
	for _, bad := range []string{"many", "0", "-5"} {
		t.Setenv("NUTREEK_DEMO_MAX_RECIPES", bad)
		if _, err := LoadConfig(); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
}
