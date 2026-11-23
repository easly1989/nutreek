package domain

import (
	"math"
	"time"
)

type WorkoutActivity struct {
	ID          int64
	Code        string // stable key for translation, empty for rows not seeded by the migrations
	Name        string
	KcalPerHour float64
	SortOrder   int
	Custom      bool // added by the user (shown after the built-in list)
}

// ReferenceWeightKg is the body weight the kcal per hour of the built-in
// activities are meant for; a workout is scaled to the weight in the profile.
const ReferenceWeightKg = 70.0

// WorkoutKcal is the kcal burned by minutes of an activity at kcalPerHour, for
// someone weighing weightKg (0 = unknown: the figure of the list as it is).
func WorkoutKcal(kcalPerHour float64, minutes int, weightKg float64) float64 {
	if weightKg > 0 {
		kcalPerHour *= weightKg / ReferenceWeightKg
	}
	return math.Round(kcalPerHour/60*float64(minutes)*10) / 10
}

type WorkoutLog struct {
	ID              int64
	UserID          int64
	ActivityID      int64
	ActivityCode    string
	ActivityName    string
	LoggedAt        time.Time
	DurationMinutes int
	KcalBurned      float64
	AtTime          string // "HH:MM", empty when no time was given
	Note            *string
	CreatedAt       time.Time
}
