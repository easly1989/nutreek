package domain

import "time"

// User rappresenta un account registrato.
type User struct {
	ID           int64
	Email        string
	PasswordHash string
	DisplayName  string
	CreatedAt    time.Time

	// Dati fisici — tutti opzionali (NULL in DB se non impostati).
	WeightKg  *float64
	HeightCm  *float64
	BirthYear *int
	Sex       *string // "M" o "F" (sesso biologico, usato per il calcolo BMR)

	// Obiettivi macro giornalieri.
	TargetKcal    *float64
	TargetProtein *float64
	TargetCarbs   *float64
	TargetFat     *float64

	// Indici calcolati e salvati al momento dell'aggiornamento del profilo.
	BMI *float64
	BMR *float64
}
