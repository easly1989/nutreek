package app

import (
	"context"
	"net/http"
	"net/url"
	"testing"
)

// Each wizard step, and accepting the adaptive TDEE, changes only its own
// fields: UpdateProfile writes them all, so the others must be kept (#201).
func TestPartialProfileSavesKeepTheRest(t *testing.T) {
	a := newTestApp(t)
	b := signIn(t, a, "eve@example.test")
	steps := []url.Values{
		{"step": {"1"}, "weight_kg": {"120"}, "height_cm": {"194"}, "birth_year": {"1989"}, "sex": {"M"}, "activity_level": {"moderate"}},
		{"step": {"2"}, "target_kcal": {"1800"}, "target_protein": {"153"}, "target_carbs": {"171"}, "target_fat": {"56"}},
	}
	for _, form := range steps {
		if code, _ := b.post("/onboarding", "/onboarding", form); code != http.StatusSeeOther {
			t.Fatalf("step %s: %d", form.Get("step"), code)
		}
	}
	if code, _ := b.post("/stats", "/tdee/accept", url.Values{"estimated_tdee": {"2100"}}); code != http.StatusSeeOther {
		t.Fatalf("accept TDEE: %d", code)
	}

	u, err := a.Users.GetByEmail(context.Background(), "eve@example.test")
	if err != nil {
		t.Fatal(err)
	}
	switch {
	case u.WeightKg == nil || *u.WeightKg != 120, u.HeightCm == nil || *u.HeightCm != 194,
		u.BirthYear == nil || *u.BirthYear != 1989, u.Sex == nil || *u.Sex != "M",
		u.ActivityLevel == nil || *u.ActivityLevel != "moderate", u.BMR == nil:
		t.Errorf("the physical data from step 1 was lost: weight %v, height %v, year %v, sex %v, activity %v", u.WeightKg, u.HeightCm, u.BirthYear, u.Sex, u.ActivityLevel)
	case u.TargetProtein == nil || *u.TargetProtein != 153, u.TargetCarbs == nil || *u.TargetCarbs != 171,
		u.TargetFat == nil || *u.TargetFat != 56:
		t.Errorf("the macro goals from step 2 were lost: protein %v, carbs %v, fat %v", u.TargetProtein, u.TargetCarbs, u.TargetFat)
	case u.TargetKcal == nil || *u.TargetKcal != 2100:
		t.Errorf("the accepted TDEE was not saved: %v", u.TargetKcal)
	}
}
