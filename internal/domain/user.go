package domain

import "time"

// Valid user roles.
const (
	RoleUser  = "user"
	RoleCoach = "coach"
)

// User rappresenta un account registrato.
type User struct {
	ID           int64
	Email        string
	PasswordHash string
	DisplayName  string
	CreatedAt    time.Time

	// Role: 'user' (default) or 'coach' (multi-client management).
	Role string

	// Physical data — all optional (NULL in the DB when not set).
	WeightKg  *float64
	HeightCm  *float64
	BirthYear *int
	Sex       *string // "M" or "F" (biological sex, used for the BMR calculation)

	// Obiettivi macro giornalieri.
	TargetKcal    *float64
	TargetProtein *float64
	TargetCarbs   *float64
	TargetFat     *float64

	// Indices computed and saved when the profile is updated.
	BMI *float64
	BMR *float64

	// Physical activity level: 'sedentary'|'light'|'moderate'|'active'|'athlete'
	ActivityLevel *string

	// Onboarding wizard: NULL = not completed, set = completed.
	OnboardingCompletedAt *time.Time

	// Guided tour: NULL = show the tour, set = tour dismissed.
	TourDismissedAt *time.Time

	// Daily hydration target in ml (default 2000, never NULL).
	WaterTargetML int
}
