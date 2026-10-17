package domain

import "time"

type WorkoutActivity struct {
	ID          int64
	Code        string // stable key for translation, empty for rows not seeded by the migrations
	Name        string
	KcalPerHour float64
	SortOrder   int
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
	Note            *string
	CreatedAt       time.Time
}
