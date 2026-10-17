// Package profile holds pure calculations about the user profile:
// BMI, BMR and the MacroProgress structure used by the visual indicators.
package profile

import (
	"math"
	"time"

	"github.com/amon2126/nutreek/internal/macros"
)

// BMI computes the body mass index (weight kg / height m²).
func BMI(weightKg, heightCm float64) float64 {
	hm := heightCm / 100.0
	return weightKg / (hm * hm)
}

// BMICategory returns the category code of a BMI value: underweight,
// normal, overweight or obese. The interface translates it.
func BMICategory(bmi float64) string {
	switch {
	case bmi < 18.5:
		return "underweight"
	case bmi < 25.0:
		return "normal"
	case bmi < 30.0:
		return "overweight"
	default:
		return "obese"
	}
}

// BMR computes the basal metabolic rate with the Mifflin-St Jeor formula.
// sex must be "M" (male) or "F" (female).
func BMR(weightKg, heightCm float64, ageYears int, sex string) float64 {
	base := 10*weightKg + 6.25*heightCm - 5*float64(ageYears)
	if sex == "M" {
		return base + 5
	}
	return base - 161
}

// AgeFromBirthYear computes the age in whole years from the birth year.
func AgeFromBirthYear(birthYear int) int {
	return time.Now().Year() - birthYear
}

// activityFactors maps the level codes to the Harris-Benedict/Mifflin multiplier.
var activityFactors = map[string]float64{
	"sedentary": 1.2,
	"light":     1.375,
	"moderate":  1.55,
	"active":    1.725,
	"athlete":   1.9,
}

// ActivityFactor returns the multiplier for the given level.
// It returns 1.4 (a generic estimate) when the code is unknown or empty.
func ActivityFactor(code string) float64 {
	if f, ok := activityFactors[code]; ok {
		return f
	}
	return 1.4
}

// TDEE computes the total daily energy expenditure (BMR × activity factor).
func TDEE(bmr, factor float64) float64 {
	return bmr * factor
}

// CanComputeIndices checks that all the physical data is present and
// plausible, so BMI and BMR can be computed.
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

// MacroBar holds the precomputed data for a single progress bar.
type MacroBar struct {
	Label    string  // catalog key of the macro's name (common.protein...)
	Actual   float64 // actual value (kcal or grams)
	Target   float64 // target value (kcal or grams)
	Pct      float64 // 0..100, capped at 100 for the bar's width
	Raw      float64 // the real percentage (can exceed 100)
	Overflow bool    // true when over the target (>100%)
}

// MacroProgress collects the four bars and the aggregate flag.
// HasTarget is false when the user has set no goal: then the templates do
// not show the bars.
type MacroProgress struct {
	HasTarget  bool
	Kcal       MacroBar
	Protein    MacroBar
	Carbs      MacroBar
	Fat        MacroBar
	AnyExceeds bool
}

// BuildMacroProgress computes the MacroProgress from the day's actual macros
// and the user's targets (pointers: nil = not set).
// When no target is set or all are zero, it returns MacroProgress{} with
// HasTarget=false.
func BuildMacroProgress(actual macros.Macros, targetKcal, targetProtein, targetCarbs, targetFat *float64) MacroProgress {
	if targetKcal == nil && targetProtein == nil && targetCarbs == nil && targetFat == nil {
		return MacroProgress{}
	}
	// At least one target must be > 0 to show anything.
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
			// No target set for this macro: the bar cannot be shown; we show
			// it as a neutral "100%" (no overflow, pct=0).
			return MacroBar{Label: label, Actual: actual}
		}
		raw := (actual / target) * 100
		pct := math.Min(raw, 100)
		return MacroBar{
			Label:    label,
			Actual:   actual,
			Target:   target,
			Pct:      pct,
			Raw:      raw,
			Overflow: raw > 100,
		}
	}

	kb := bar("common.calories", actual.Kcal, tk)
	pb := bar("common.protein", actual.Protein, tp)
	cb := bar("common.carbs", actual.Carbs, tc)
	fb := bar("common.fat", actual.Fat, tf)

	return MacroProgress{
		HasTarget:  true,
		Kcal:       kb,
		Protein:    pb,
		Carbs:      cb,
		Fat:        fb,
		AnyExceeds: kb.Overflow || pb.Overflow || cb.Overflow || fb.Overflow,
	}
}
