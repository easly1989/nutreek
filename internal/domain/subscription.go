package domain

import "time"

// Subscription is a user's premium subscription.
// Source says where it comes from: 'manual' (granted from the admin panel),
// 'license' (a license key, future), 'payment' (a payment, future).
// ExpiresAt nil = no expiry.
type Subscription struct {
	ID         int64
	UserID     int64
	Plan       string
	Status     string
	Source     string
	LicenseKey *string
	StartedAt  time.Time
	ExpiresAt  *time.Time
	Notes      string
}

// IsActive returns true when the subscription is active and not expired.
func (s *Subscription) IsActive(now time.Time) bool {
	if s == nil || s.Status != "active" {
		return false
	}
	return s.ExpiresAt == nil || s.ExpiresAt.After(now)
}
