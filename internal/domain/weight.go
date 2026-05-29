package domain

import "time"

type WeightLog struct {
	ID         int64
	UserID     int64
	MeasuredAt time.Time
	WeightKg   float64
	Note       *string
	CreatedAt  time.Time
}
