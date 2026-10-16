package domain

import "time"

// WaterLog is a single water intake entry. Unlike weight, there can be
// several logs per day (every "+250ml" is a log): the daily total is the
// sum of amount_ml per date.
type WaterLog struct {
	ID        int64
	UserID    int64
	LoggedAt  time.Time
	AmountML  int
	CreatedAt time.Time
}
