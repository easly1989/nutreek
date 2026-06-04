package domain

import "time"

// WaterLog e' una singola registrazione di assunzione d'acqua. A differenza
// del peso, ci possono essere piu' log per giorno (ogni "+250ml" e' un log):
// il totale giornaliero e' la somma di amount_ml per data.
type WaterLog struct {
	ID        int64
	UserID    int64
	LoggedAt  time.Time
	AmountML  int
	CreatedAt time.Time
}
