// Package demo is the public demo (#137): throwaway accounts on a database of
// their own. Only the demo build imports it (cmd/nutreek/demo_on.go), so the
// normal binary contains none of it.
package demo

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"math/big"
	"os"
	"sync"
	"time"

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

// Config holds the demo's settings, read from NUTREEK_DEMO_* variables.
type Config struct {
	Idle time.Duration // a session ends after this long without a request
}

// LoadConfig reads the demo's settings from the environment.
func LoadConfig() (Config, error) {
	c := Config{Idle: 2 * time.Hour}
	if v := os.Getenv("NUTREEK_DEMO_IDLE"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil || d <= 0 {
			return c, fmt.Errorf("NUTREEK_DEMO_IDLE: %q is not a positive duration", v)
		}
		c.Idle = d
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

// Start creates a group with one user who follows a plan, seeded in lang,
// and returns that user's id. The user has a complete profile, onboarding and
// tour done, and no email or password anyone could use.
func (s *Service) Start(ctx context.Context, lang string) (int64, error) {
	lang = pickCode(lang)
	w := words[lang]

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

	userID, err := createUser(ctx, tx, oneOf(w.names), persona)
	if err != nil {
		return 0, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO demo_members (user_id, group_id) VALUES (?, ?)`, userID, groupID); err != nil {
		return 0, fmt.Errorf("demo member: %w", err)
	}
	if err := seed(ctx, tx, userID, w, time.Now()); err != nil {
		return 0, fmt.Errorf("demo seed: %w", err)
	}
	return userID, tx.Commit()
}

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

func createUser(ctx context.Context, tx *sql.Tx, name string, b body) (int64, error) {
	tag := make([]byte, 8)
	if _, err := rand.Read(tag); err != nil {
		return 0, err
	}
	birthYear := time.Now().Year() - b.age
	bmi := profile.BMI(b.weightKg, b.heightCm)
	bmr := profile.BMR(b.weightKg, b.heightCm, profile.AgeFromBirthYear(birthYear), b.sex)
	// "!" is no bcrypt hash: no password ever matches it.
	res, err := tx.ExecContext(ctx, `
		INSERT INTO users (email, password_hash, display_name,
		                   weight_kg, height_cm, birth_year, sex, activity_level,
		                   target_kcal, target_protein, target_carbs, target_fat, bmi, bmr,
		                   onboarding_completed_at, tour_dismissed_at)
		VALUES (?, '!', ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`,
		"demo-"+hex.EncodeToString(tag)+emailDomain, name,
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

func oneOf(xs []string) string {
	n, err := rand.Int(rand.Reader, big.NewInt(int64(len(xs))))
	if err != nil {
		return xs[0]
	}
	return xs[n.Int64()]
}
