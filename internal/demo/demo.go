// Package demo is the public demo (#137): throwaway accounts on a database of
// their own. Only the demo build imports it (cmd/nutreek/demo_on.go), so the
// normal binary contains none of it.
package demo

import (
	"context"
	crand "crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"maps"
	"math/rand/v2"
	"os"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/amon2126/nutreek/internal/domain"
	"github.com/amon2126/nutreek/internal/profile"
)

// emailDomain marks the demo's generated addresses: .invalid never resolves.
const emailDomain = "@demo.invalid"

// schema is the demo's own bookkeeping, created at startup rather than by
// goose, so normal installs never get these tables (decided in #137).
const schema = `
CREATE TABLE IF NOT EXISTS demo_meta (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS demo_groups (
    id           INTEGER PRIMARY KEY,
    lang         TEXT     NOT NULL,
    created_at   DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    last_seen_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE TABLE IF NOT EXISTS demo_members (
    user_id  INTEGER PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    group_id INTEGER NOT NULL REFERENCES demo_groups(id) ON DELETE CASCADE
);`

// Peers lists the users whose hand-made ingredients and public recipes a
// visitor sees: their own demo group (#185). One ? for the visitor's id; see
// db.ScopeCatalog.
const Peers = `SELECT user_id FROM demo_members WHERE group_id = (SELECT group_id FROM demo_members WHERE user_id = ?)`

// HiddenProducts lists the demo's OpenFoodFacts products in a language other
// than the visitor's group's, kept out of their ingredient lists (#189). One ?
// for the visitor's id; see db.HideProducts.
var HiddenProducts = func() string {
	var prefix strings.Builder
	for _, lang := range slices.Sorted(maps.Keys(words)) {
		fmt.Fprintf(&prefix, " WHEN '%s' THEN '%s'", lang, words[lang].ean)
	}
	return `SELECT i.id FROM ingredients i WHERE i.source = 'openfoodfacts' AND i.brand = '` + demoBrand + `'
		AND i.off_barcode NOT LIKE coalesce((SELECT CASE g.lang` + prefix.String() + ` END
		  FROM demo_members m JOIN demo_groups g ON g.id = m.group_id WHERE m.user_id = ?), '') || '%'`
}()

// Config holds the demo's settings, read from NUTREEK_DEMO_* variables.
type Config struct {
	Idle time.Duration // a session ends after this long without a request

	// Limits (#185): one visitor must not hurt the server or the others.
	MaxUsers       int   // live demo users
	StartsPerHour  int   // demos started per address per hour
	MaxBody        int64 // request body in bytes
	MaxIngredients int   // hand-made ingredients per user
	MaxRecipes     int   // recipes per user
}

// LoadConfig reads the demo's settings from the environment.
func LoadConfig() (Config, error) {
	c := Config{Idle: 2 * time.Hour, MaxUsers: 200, StartsPerHour: 10, MaxBody: 1 << 20, MaxIngredients: 300, MaxRecipes: 100}
	if v := os.Getenv("NUTREEK_DEMO_IDLE"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil || d <= 0 {
			return c, fmt.Errorf("NUTREEK_DEMO_IDLE: %q is not a positive duration", v)
		}
		c.Idle = d
	}
	var body int
	for _, l := range []struct {
		name string
		to   *int
	}{
		{"NUTREEK_DEMO_MAX_USERS", &c.MaxUsers},
		{"NUTREEK_DEMO_STARTS_PER_HOUR", &c.StartsPerHour},
		{"NUTREEK_DEMO_MAX_BODY", &body},
		{"NUTREEK_DEMO_MAX_INGREDIENTS", &c.MaxIngredients},
		{"NUTREEK_DEMO_MAX_RECIPES", &c.MaxRecipes},
	} {
		if v := os.Getenv(l.name); v != "" {
			n, err := strconv.Atoi(v)
			if err != nil || n <= 0 {
				return c, fmt.Errorf("%s: %q is not a positive whole number", l.name, v)
			}
			*l.to = n
		}
	}
	if body > 0 {
		c.MaxBody = int64(body)
	}
	return c, nil
}

// Prepare refuses a database holding users the demo did not create (the demo
// deletes users, so it must never run on real data), then creates the demo's
// tables, marks the database as a demo one and adds the demo's OpenFoodFacts
// products, which every group shares. A real database is refused
// before anything is written to it.
func Prepare(ctx context.Context, db *sql.DB) error {
	var foreign int
	if err := db.QueryRowContext(ctx,
		`SELECT count(*) FROM users WHERE email NOT LIKE ?`, "%"+emailDomain).Scan(&foreign); err != nil {
		return fmt.Errorf("demo: count users: %w", err)
	}
	if foreign > 0 {
		return fmt.Errorf("demo: the database holds %d users the demo did not create; use an empty database", foreign)
	}
	if _, err := db.ExecContext(ctx, schema); err != nil {
		return fmt.Errorf("demo schema: %w", err)
	}
	if _, err := db.ExecContext(ctx, `INSERT OR IGNORE INTO demo_meta (key, value) VALUES ('demo', '1')`); err != nil {
		return err
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err := products(ctx, tx, time.Now()); err != nil {
		return fmt.Errorf("demo products: %w", err)
	}
	return tx.Commit()
}

// Service creates the demo's accounts and deletes them (purge.go).
type Service struct {
	db  *sql.DB
	cfg Config

	mu   sync.Mutex
	seen map[int64]time.Time // user id → last write of their group's last_seen_at
}

// New returns the demo service on a database Prepare has accepted.
func New(db *sql.DB, cfg Config) *Service {
	return &Service{db: db, cfg: cfg, seen: map[int64]time.Time{}}
}

// IdleHours is how long a demo lasts without use, in whole hours rounded up,
// for the notices.
func (s *Service) IdleHours() int { return int((s.cfg.Idle + time.Hour - 1) / time.Hour) }

// The ways into the demo (#186): whom the visitor plays.
const (
	AsPlan   = "plan"   // someone following their own plan, alone
	AsCoach  = "coach"  // a coach with clients and a library of plans
	AsClient = "client" // a coach's client, on the plan the coach assigned
)

// Start creates a demo group seeded in lang, with the visitor playing as (an
// unknown one is AsPlan), and returns the visitor's id. The visitor is the
// group's first member. Everyone has a complete profile, onboarding and tour
// done, and no email or password anyone could use.
func (s *Service) Start(ctx context.Context, lang, as string) (int64, error) {
	lang = pickCode(lang)
	w := words[lang]
	now := time.Now()

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()

	res, err := tx.ExecContext(ctx, `INSERT INTO demo_groups (lang) VALUES (?)`, lang)
	if err != nil {
		return 0, fmt.Errorf("demo group: %w", err)
	}
	groupID, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	foods := map[string]int64{} // the group's, made by its first member
	names := slices.Clone(w.names)
	rand.Shuffle(len(names), func(i, j int) { names[i], names[j] = names[j], names[i] })
	add := func(name string, p person) (int64, []int64, error) {
		id, err := createUser(ctx, tx, name, p.body, p.role)
		if err != nil {
			return 0, nil, err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO demo_members (user_id, group_id) VALUES (?, ?)`, id, groupID); err != nil {
			return 0, nil, fmt.Errorf("demo member: %w", err)
		}
		plans, err := seed(ctx, tx, id, w, p, foods, now)
		if err != nil {
			return 0, nil, fmt.Errorf("demo seed: %w", err)
		}
		return id, plans, nil
	}

	if as != AsCoach && as != AsClient {
		id, _, err := add(names[0], visitor)
		if err != nil {
			return 0, err
		}
		return id, tx.Commit()
	}

	// A coach and three people: the first two the coach's clients, each on a
	// plan of the coach's library, the third one to add.
	people := []person{coach, steadyClient, struggling, steadyClient}
	order := []int{0, 1, 2, 3}
	if as == AsCoach {
		people[0].weeks = historyWeeks
	} else {
		people[1] = visitor
		order = []int{1, 0, 2, 3}
	}
	ids := make([]int64, len(people))
	var library []int64
	for _, i := range order {
		var plans []int64
		if ids[i], plans, err = add(names[i], people[i]); err != nil {
			return 0, err
		}
		if i == 0 {
			library = plans
		}
	}
	for i, client := range ids[1:3] {
		if _, err := tx.ExecContext(ctx, `INSERT INTO coach_clients (coach_id, client_id, assigned_plan_id) VALUES (?, ?, ?)`,
			ids[0], client, library[i]); err != nil {
			return 0, fmt.Errorf("demo client: %w", err)
		}
		// Their weeks came from the assigned plan.
		if _, err := tx.ExecContext(ctx, `UPDATE weekly_plans SET source_plan_id = ? WHERE user_id = ?`, library[i], client); err != nil {
			return 0, fmt.Errorf("demo client weeks: %w", err)
		}
	}
	return ids[order[0]], tx.Commit()
}

// Candidates are whom coachID may add as clients: the people of their group
// with no coach, but the coach.
func (s *Service) Candidates(ctx context.Context, coachID int64) ([]domain.User, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT u.id, u.display_name FROM demo_members m JOIN users u ON u.id = m.user_id
		WHERE m.group_id = (SELECT group_id FROM demo_members WHERE user_id = ?) AND u.id <> ?
		  AND NOT EXISTS (SELECT 1 FROM coach_clients c WHERE c.client_id = u.id)
		ORDER BY u.display_name, u.id`, coachID, coachID)
	if err != nil {
		return nil, fmt.Errorf("demo candidates: %w", err)
	}
	defer rows.Close()
	var out []domain.User
	for rows.Next() {
		var u domain.User
		if err := rows.Scan(&u.ID, &u.DisplayName); err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

// person is someone of a demo group.
type person struct {
	body   body
	role   string    // users.role
	weeks  int       // weeks of history before the current one
	cheats []cheat   // the fewer, the closer to the plan they keep
	plans  []planDef // their nutrition plans, the first one active
}

// visitor is whom the visitor plays when not the coach.
var visitor = person{body: persona, role: domain.RoleUser, weeks: historyWeeks, cheats: cheats, plans: ownPlan}

// The rest of a coach's group, with a short history and their own adherence.
var (
	coach = person{body: body{weightKg: 78, heightCm: 181, age: 41, sex: "M", activity: "active", kcal: 2600, protein: 170, carbs: 300, fat: 80},
		role: domain.RoleCoach, weeks: otherWeeks, cheats: fewCheats, plans: library}
	steadyClient = person{body: body{weightKg: 64, heightCm: 165, age: 29, sex: "F", activity: "light", kcal: 1800, protein: 115, carbs: 200, fat: 58},
		role: domain.RoleUser, weeks: otherWeeks, cheats: fewCheats, plans: ownPlan}
	struggling = person{body: body{weightKg: 88, heightCm: 178, age: 47, sex: "M", activity: "sedentary", kcal: 2000, protein: 150, carbs: 200, fat: 66},
		role: domain.RoleUser, weeks: otherWeeks, cheats: manyCheats, plans: ownPlan}
)

// body is a demo person's physical data and daily targets.
type body struct {
	weightKg, heightCm float64
	age                int
	sex, activity      string
	kcal, protein      float64
	carbs, fat         float64
}

// persona is the person the visitor plays: targets close to their energy needs.
var persona = body{weightKg: 72, heightCm: 172, age: 34, sex: "F", activity: "moderate", kcal: 2100, protein: 140, carbs: 230, fat: 70}

func createUser(ctx context.Context, tx *sql.Tx, name string, b body, role string) (int64, error) {
	tag := make([]byte, 8)
	if _, err := crand.Read(tag); err != nil {
		return 0, err
	}
	birthYear := time.Now().Year() - b.age
	bmi := profile.BMI(b.weightKg, b.heightCm)
	bmr := profile.BMR(b.weightKg, b.heightCm, profile.AgeFromBirthYear(birthYear), b.sex)
	// "!" is no bcrypt hash: no password ever matches it.
	res, err := tx.ExecContext(ctx, `
		INSERT INTO users (email, password_hash, display_name, role,
		                   weight_kg, height_cm, birth_year, sex, activity_level,
		                   target_kcal, target_protein, target_carbs, target_fat, bmi, bmr,
		                   onboarding_completed_at, tour_dismissed_at)
		VALUES (?, '!', ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`,
		"demo-"+hex.EncodeToString(tag)+emailDomain, name, role,
		b.weightKg, b.heightCm, birthYear, b.sex, b.activity,
		b.kcal, b.protein, b.carbs, b.fat, bmi, bmr)
	if err != nil {
		return 0, fmt.Errorf("demo user: %w", err)
	}
	return res.LastInsertId()
}

// vocabulary holds every name the demo shows, in one language.
type vocabulary struct {
	names []string          // first names for the generated people
	text  map[string]string // the seed's names, by key (seed.go)
	ean   string            // barcode prefix of the language's demo products
}

// words has one vocabulary per language code; English is the fallback for a
// language without its own.
var words = map[string]vocabulary{
	"en": {names: []string{"Alex", "Sam", "Jordan", "Taylor", "Morgan", "Casey", "Riley", "Jamie"}, text: textEN, ean: "2000000001"},
	"it": {names: []string{"Giulia", "Marco", "Chiara", "Luca", "Sara", "Matteo", "Elena", "Andrea"}, text: textIT, ean: "2000000002"},
}

// pickCode returns lang when the demo speaks it, else English.
func pickCode(lang string) string {
	if _, ok := words[lang]; ok {
		return lang
	}
	return "en"
}
