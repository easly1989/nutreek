package app

import (
	"math"
	"testing"

	"github.com/amon2126/nutreek/internal/domain"
	"github.com/amon2126/nutreek/internal/profile"
)

func TestOnboardingTDEE(t *testing.T) {
	w, h, y := 73.0, 178.0, 1990
	m, act := "M", "moderate"
	staleBMR := 1.0
	bmr := profile.BMR(w, h, profile.AgeFromBirthYear(y), m)

	tests := []struct {
		name string
		user domain.User
		want float64
	}{
		{"dati completi", domain.User{WeightKg: &w, HeightCm: &h, BirthYear: &y, Sex: &m, ActivityLevel: &act},
			profile.TDEE(bmr, profile.ActivityFactor(act))},
		{"attivita' non indicata", domain.User{WeightKg: &w, HeightCm: &h, BirthYear: &y, Sex: &m},
			profile.TDEE(bmr, 1.4)},
		// Il BMR salvato puo' mancare o essere vecchio: conta il profilo.
		{"BMR salvato ignorato", domain.User{WeightKg: &w, HeightCm: &h, BirthYear: &y, Sex: &m, BMR: &staleBMR},
			profile.TDEE(bmr, 1.4)},
		{"manca il sesso", domain.User{WeightKg: &w, HeightCm: &h, BirthYear: &y}, 0},
		{"profilo vuoto", domain.User{}, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := onboardingTDEE(&tt.user); math.Abs(got-tt.want) > 1e-9 {
				t.Errorf("onboardingTDEE() = %v, want %v", got, tt.want)
			}
		})
	}
}
