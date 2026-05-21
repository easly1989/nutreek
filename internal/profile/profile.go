// Package profile contiene calcoli puri relativi al profilo utente:
// BMI, BMR e la struttura MacroProgress usata dagli indicatori visivi.
package profile

import (
	"math"
	"time"

	"github.com/amon2126/nutreek/internal/macros"
)

// BMI calcola l'indice di massa corporea (peso kg / altezza m²).
func BMI(weightKg, heightCm float64) float64 {
	hm := heightCm / 100.0
	return weightKg / (hm * hm)
}

// BMICategory ritorna la categoria testuale per un valore BMI.
func BMICategory(bmi float64) string {
	switch {
	case bmi < 18.5:
		return "Sottopeso"
	case bmi < 25.0:
		return "Normopeso"
	case bmi < 30.0:
		return "Sovrappeso"
	default:
		return "Obesità"
	}
}

// BMR calcola il metabolismo basale con la formula Mifflin-St Jeor.
// sex deve essere "M" (maschio) o "F" (femmina).
func BMR(weightKg, heightCm float64, ageYears int, sex string) float64 {
	base := 10*weightKg + 6.25*heightCm - 5*float64(ageYears)
	if sex == "M" {
		return base + 5
	}
	return base - 161
}

// AgeFromBirthYear calcola l'età in anni interi partendo dall'anno di nascita.
func AgeFromBirthYear(birthYear int) int {
	return time.Now().Year() - birthYear
}

// activityFactors mappa i codici livello al moltiplicatore Harris-Benedict/Mifflin.
var activityFactors = map[string]float64{
	"sedentary": 1.2,
	"light":     1.375,
	"moderate":  1.55,
	"active":    1.725,
	"athlete":   1.9,
}

// ActivityFactor ritorna il moltiplicatore per il livello dato.
// Ritorna 1.4 (stima generica) se il codice non è riconosciuto o è vuoto.
func ActivityFactor(code string) float64 {
	if f, ok := activityFactors[code]; ok {
		return f
	}
	return 1.4
}

// TDEE calcola il fabbisogno calorico giornaliero totale (BMR × fattore attività).
func TDEE(bmr, factor float64) float64 {
	return bmr * factor
}

// CanComputeIndices verifica che tutti i dati fisici siano presenti
// e plausibili per poter calcolare BMI e BMR.
func CanComputeIndices(weightKg, heightCm *float64, birthYear *int, sex *string) bool {
	if weightKg == nil || heightCm == nil || birthYear == nil || sex == nil {
		return false
	}
	if *weightKg <= 0 || *heightCm <= 0 || *birthYear <= 0 {
		return false
	}
	if *sex != "M" && *sex != "F" {
		return false
	}
	age := AgeFromBirthYear(*birthYear)
	return age >= 5 && age <= 130
}

// ---- Indicatori visivi -------------------------------------------------

// MacroBar contiene i dati pre-calcolati per una singola barra di progresso.
type MacroBar struct {
	Label    string
	Pct      float64 // 0..100, cappato a 100 per la larghezza della barra
	Raw      float64 // percentuale reale (può superare 100)
	Overflow bool    // true se supera il target (>100%)
}

// MacroProgress raccoglie le quattro barre e il flag aggregato.
// HasTarget è false se l'utente non ha impostato nessun obiettivo:
// in quel caso i template non mostrano le barre.
type MacroProgress struct {
	HasTarget  bool
	Kcal       MacroBar
	Protein    MacroBar
	Carbs      MacroBar
	Fat        MacroBar
	AnyExceeds bool
}

// BuildMacroProgress calcola la MacroProgress a partire dai macro effettivi
// del giorno e dai target dell'utente (pointer: nil = non impostato).
// Se nessun target è impostato o tutti sono zero, ritorna MacroProgress{} con
// HasTarget=false.
func BuildMacroProgress(actual macros.Macros, targetKcal, targetProtein, targetCarbs, targetFat *float64) MacroProgress {
	if targetKcal == nil && targetProtein == nil && targetCarbs == nil && targetFat == nil {
		return MacroProgress{}
	}
	// Almeno un target deve essere > 0 per mostrare qualcosa.
	t := func(p *float64) float64 {
		if p == nil {
			return 0
		}
		return *p
	}
	tk, tp, tc, tf := t(targetKcal), t(targetProtein), t(targetCarbs), t(targetFat)
	if tk == 0 && tp == 0 && tc == 0 && tf == 0 {
		return MacroProgress{}
	}

	bar := func(label string, actual, target float64) MacroBar {
		if target <= 0 {
			// Target non impostato per questo macro: barra non visualizzabile;
			// la mostriamo come "100%" neutra (non overflow, pct=0).
			return MacroBar{Label: label}
		}
		raw := (actual / target) * 100
		pct := math.Min(raw, 100)
		return MacroBar{
			Label:    label,
			Pct:      pct,
			Raw:      raw,
			Overflow: raw > 100,
		}
	}

	kb := bar("Kcal", actual.Kcal, tk)
	pb := bar("P", actual.Protein, tp)
	cb := bar("C", actual.Carbs, tc)
	fb := bar("F", actual.Fat, tf)

	return MacroProgress{
		HasTarget:  true,
		Kcal:       kb,
		Protein:    pb,
		Carbs:      cb,
		Fat:        fb,
		AnyExceeds: kb.Overflow || pb.Overflow || cb.Overflow || fb.Overflow,
	}
}
