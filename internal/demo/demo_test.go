package demo

import (
	"context"
	"database/sql"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/amon2126/nutreek/internal/db"
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
		if _, err := New(d, Config{}).Start(ctx, "en"); err != nil {
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
		if _, err := New(d, Config{}).Start(ctx, "en"); err != nil {
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
		id, err := svc.Start(ctx, lang)
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
