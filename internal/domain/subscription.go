package domain

import "time"

// Subscription rappresenta l'abbonamento premium di un utente.
// Source indica l'origine: 'manual' (grant dall'admin panel),
// 'license' (chiave di licenza, futuro), 'payment' (pagamento, futuro).
// ExpiresAt nil = senza scadenza.
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

// IsActive ritorna true se la sottoscrizione e' attiva e non scaduta.
func (s *Subscription) IsActive(now time.Time) bool {
	if s == nil || s.Status != "active" {
		return false
	}
	return s.ExpiresAt == nil || s.ExpiresAt.After(now)
}
