package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"

	"github.com/amon2126/nutreek/internal/domain"
)

// ErrUserNotFound e' restituito dalle Get quando la riga non esiste.
var ErrUserNotFound = errors.New("user non trovato")

// ErrEmailAlreadyUsed e' restituito da CreateUser quando l'email esiste gia'.
var ErrEmailAlreadyUsed = errors.New("email gia' registrata")

// Users e' il repository degli utenti.
type Users struct {
	db *sql.DB
}

func NewUsers(db *sql.DB) *Users {
	return &Users{db: db}
}

// Create inserisce un nuovo utente. Ritorna l'id assegnato dal DB.
// Se l'email e' gia' presente ritorna ErrEmailAlreadyUsed.
func (u *Users) Create(ctx context.Context, email, passwordHash, displayName string) (int64, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	res, err := u.db.ExecContext(ctx,
		`INSERT INTO users (email, password_hash, display_name) VALUES (?, ?, ?)`,
		email, passwordHash, displayName,
	)
	if err != nil {
		if isUniqueViolation(err) {
			return 0, ErrEmailAlreadyUsed
		}
		return 0, fmt.Errorf("insert user: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("last insert id: %w", err)
	}
	return id, nil
}

// GetByEmail ritorna l'utente con l'email data o ErrUserNotFound.
func (u *Users) GetByEmail(ctx context.Context, email string) (*domain.User, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	row := u.db.QueryRowContext(ctx,
		`SELECT id, email, password_hash, display_name, created_at,
		        weight_kg, height_cm, birth_year, sex,
		        target_kcal, target_protein, target_carbs, target_fat,
		        bmi, bmr, activity_level, onboarding_completed_at, tour_dismissed_at,
		        water_target_ml
		 FROM users WHERE email = ?`, email,
	)
	return scanUser(row)
}

// GetByID ritorna l'utente con l'id dato o ErrUserNotFound.
func (u *Users) GetByID(ctx context.Context, id int64) (*domain.User, error) {
	row := u.db.QueryRowContext(ctx,
		`SELECT id, email, password_hash, display_name, created_at,
		        weight_kg, height_cm, birth_year, sex,
		        target_kcal, target_protein, target_carbs, target_fat,
		        bmi, bmr, activity_level, onboarding_completed_at, tour_dismissed_at,
		        water_target_ml
		 FROM users WHERE id = ?`, id,
	)
	return scanUser(row)
}

// UserProfileInput raccoglie i campi aggiornabili dalla pagina profilo.
// I pointer nil significano "non modificare / cancella il valore".
type UserProfileInput struct {
	WeightKg      *float64
	HeightCm      *float64
	BirthYear     *int
	Sex           *string
	TargetKcal    *float64
	TargetProtein *float64
	TargetCarbs   *float64
	TargetFat     *float64
	BMI           *float64
	BMR           *float64
	ActivityLevel *string
	WaterTargetML int
}

// UpdateProfile aggiorna i dati del profilo dell'utente.
func (u *Users) UpdateProfile(ctx context.Context, id int64, p UserProfileInput) error {
	_, err := u.db.ExecContext(ctx,
		`UPDATE users SET
		   weight_kg      = ?,
		   height_cm      = ?,
		   birth_year     = ?,
		   sex            = ?,
		   target_kcal    = ?,
		   target_protein = ?,
		   target_carbs   = ?,
		   target_fat     = ?,
		   bmi            = ?,
		   bmr            = ?,
		   activity_level = ?,
		   -- 0 = sentinella "non modificare" cosi' i flussi che non gestiscono
		   -- l'idratazione (es. onboarding) non azzerano il target esistente.
		   water_target_ml = CASE WHEN ? > 0 THEN ? ELSE water_target_ml END
		 WHERE id = ?`,
		p.WeightKg, p.HeightCm, p.BirthYear, p.Sex,
		p.TargetKcal, p.TargetProtein, p.TargetCarbs, p.TargetFat,
		p.BMI, p.BMR, p.ActivityLevel, p.WaterTargetML, p.WaterTargetML,
		id,
	)
	if err != nil {
		return fmt.Errorf("update user profile: %w", err)
	}
	return nil
}

func scanUser(row *sql.Row) (*domain.User, error) {
	var user domain.User
	err := row.Scan(
		&user.ID, &user.Email, &user.PasswordHash, &user.DisplayName, &user.CreatedAt,
		&user.WeightKg, &user.HeightCm, &user.BirthYear, &user.Sex,
		&user.TargetKcal, &user.TargetProtein, &user.TargetCarbs, &user.TargetFat,
		&user.BMI, &user.BMR, &user.ActivityLevel, &user.OnboardingCompletedAt, &user.TourDismissedAt,
		&user.WaterTargetML,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrUserNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("scan user: %w", err)
	}
	return &user, nil
}

// CompleteOnboarding segna il wizard come completato per l'utente.
func (u *Users) CompleteOnboarding(ctx context.Context, id int64) error {
	_, err := u.db.ExecContext(ctx,
		`UPDATE users SET onboarding_completed_at = CURRENT_TIMESTAMP WHERE id = ?`, id)
	return err
}

// ResetOnboarding azzera il timestamp di completamento wizard (per il "Guida rapida").
func (u *Users) ResetOnboarding(ctx context.Context, id int64) error {
	_, err := u.db.ExecContext(ctx,
		`UPDATE users SET onboarding_completed_at = NULL WHERE id = ?`, id)
	return err
}

// DismissTour segna il tour guidato come dismesso dall'utente.
func (u *Users) DismissTour(ctx context.Context, id int64) error {
	_, err := u.db.ExecContext(ctx,
		`UPDATE users SET tour_dismissed_at = CURRENT_TIMESTAMP WHERE id = ?`, id)
	return err
}

// ResetTour azzera il timestamp del tour (permette di riavviarlo).
func (u *Users) ResetTour(ctx context.Context, id int64) error {
	_, err := u.db.ExecContext(ctx,
		`UPDATE users SET tour_dismissed_at = NULL WHERE id = ?`, id)
	return err
}

// isUniqueViolation identifica i codici errore SQLite per UNIQUE constraint.
// modernc.org/sqlite espone *sqlite.Error con Code() int.
func isUniqueViolation(err error) bool {
	var sqlErr *sqlite.Error
	if !errors.As(err, &sqlErr) {
		return false
	}
	code := sqlErr.Code()
	return code == sqlite3.SQLITE_CONSTRAINT_UNIQUE || code == sqlite3.SQLITE_CONSTRAINT_PRIMARYKEY
}
