package tdee

import (
	"time"

	"github.com/amon2126/nutreek/internal/domain"
)

const kcalPerKg = 7700.0 // kcal ≈ 1 kg grasso corporeo

// WeekData raccoglie i dati di una singola settimana necessari per la stima TDEE.
type WeekData struct {
	WeekStart   time.Time
	AvgKcal     float64 // media kcal/giorno dalla pianificazione pasti
	StartWeight float64 // peso interpolato a inizio settimana (0 = non disponibile)
	EndWeight   float64 // peso interpolato a fine settimana (0 = non disponibile)
}

// WeightDelta restituisce la variazione di peso in kg per la settimana.
func (w WeekData) WeightDelta() float64 { return w.EndWeight - w.StartWeight }

// EstimatedKcal è il TDEE stimato per questa settimana (0 se dati insufficienti).
// Esposto come metodo per poter essere chiamato direttamente dai template Go.
func (w WeekData) EstimatedKcal() float64 { return Estimate(w) }

// WeekLabel formatta la label della settimana (dd/mm) per i grafici.
func (w WeekData) WeekLabel() string { return w.WeekStart.Format("02/01") }

// Estimate stima il TDEE giornaliero da una singola settimana.
// Restituisce 0 se i dati sono insufficienti.
func Estimate(d WeekData) float64 {
	if d.AvgKcal == 0 || d.StartWeight == 0 || d.EndWeight == 0 {
		return 0
	}
	weeklyKcal := d.AvgKcal * 7
	// TDEE = kcal_assunte - kcal_nette_da_variazione_peso
	// Δpeso > 0 → surplus calorico → TDEE < kcal assunte
	// Δpeso < 0 → deficit calorico → TDEE > kcal assunte
	tdee := (weeklyKcal - d.WeightDelta()*kcalPerKg) / 7
	return tdee
}

// EstimateFromHistory calcola il TDEE medio pesato sulle settimane con dati
// completi. Le settimane più recenti hanno peso maggiore (media esponenziale).
// Restituisce (0, 0) se non ci sono settimane valide.
func EstimateFromHistory(weeks []WeekData) (tdee float64, confidence int) {
	var valid []float64
	// weeks in input sono in ordine dalla più vecchia alla più recente
	for _, w := range weeks {
		t := Estimate(w)
		if t > 500 && t < 8000 { // sanity check: TDEE umano plausibile
			valid = append(valid, t)
		}
	}
	if len(valid) == 0 {
		return 0, 0
	}
	var sum, totalWeight float64
	for i, v := range valid {
		w := float64(i + 1) // peso crescente: le ultime settimane contano di più
		sum += v * w
		totalWeight += w
	}
	n := len(valid)
	if n >= 4 {
		confidence = 100
	} else {
		confidence = n * 25
	}
	return sum / totalWeight, confidence
}

// InterpolateWeight stima il peso alla data `at` interpolando linearmente tra
// le misurazioni disponibili. I log devono essere ordinati per data (qualsiasi
// ordine funziona, la funzione li gestisce internamente).
// Restituisce 0 se non ci sono misurazioni sufficienti per interpolare.
func InterpolateWeight(logs []domain.WeightLog, at time.Time) float64 {
	if len(logs) == 0 {
		return 0
	}
	// Cerca la misurazione esatta
	for _, l := range logs {
		if sameDay(l.MeasuredAt, at) {
			return l.WeightKg
		}
	}
	// Trova il vicino precedente e successivo
	var before, after *domain.WeightLog
	for i := range logs {
		l := &logs[i]
		if l.MeasuredAt.Before(at) {
			if before == nil || l.MeasuredAt.After(before.MeasuredAt) {
				before = l
			}
		} else {
			if after == nil || l.MeasuredAt.Before(after.MeasuredAt) {
				after = l
			}
		}
	}
	if before == nil && after == nil {
		return 0
	}
	if before == nil {
		return after.WeightKg // estrapolazione: usa il primo disponibile
	}
	if after == nil {
		return before.WeightKg // estrapolazione: usa l'ultimo disponibile
	}
	// Interpolazione lineare
	span := after.MeasuredAt.Sub(before.MeasuredAt).Hours()
	elapsed := at.Sub(before.MeasuredAt).Hours()
	if span == 0 {
		return before.WeightKg
	}
	frac := elapsed / span
	return before.WeightKg + frac*(after.WeightKg-before.WeightKg)
}

func sameDay(a, b time.Time) bool {
	ay, am, ad := a.Date()
	by, bm, bd := b.Date()
	return ay == by && am == bm && ad == bd
}
