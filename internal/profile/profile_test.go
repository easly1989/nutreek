package profile

import (
	"math"
	"testing"

	"github.com/amon2126/nutreek/internal/macros"
)

func TestBMI(t *testing.T) {
	cases := []struct {
		weight, height, want float64
	}{
		{70, 175, 22.86},
		{90, 180, 27.78},
		{50, 160, 19.53},
	}
	for _, tc := range cases {
		got := BMI(tc.weight, tc.height)
		if math.Abs(got-tc.want) > 0.01 {
			t.Errorf("BMI(%.0f, %.0f) = %.2f, want %.2f", tc.weight, tc.height, got, tc.want)
		}
	}
}

func TestBMICategory(t *testing.T) {
	cases := []struct {
		bmi  float64
		want string
	}{
		{16, "underweight"},
		{22, "normal"},
		{27, "overweight"},
		{35, "obese"},
	}
	for _, tc := range cases {
		if got := BMICategory(tc.bmi); got != tc.want {
			t.Errorf("BMICategory(%.0f) = %q, want %q", tc.bmi, got, tc.want)
		}
	}
}

func TestBMR(t *testing.T) {
	// Mifflin-St Jeor: base = 10w + 6.25h - 5a
	// M: +5, F: -161
	cases := []struct {
		weight, height float64
		age            int
		sex            string
		want           float64
	}{
		{70, 175, 30, "M", 1648.75}, // 10*70 + 6.25*175 - 5*30 + 5 = 700+1093.75-150+5
		{60, 165, 25, "F", 1345.25}, // 10*60 + 6.25*165 - 5*25 - 161 = 600+1031.25-125-161
	}
	for _, tc := range cases {
		got := BMR(tc.weight, tc.height, tc.age, tc.sex)
		if math.Abs(got-tc.want) > 0.01 {
			t.Errorf("BMR(%v,%v,%v,%v) = %.2f, want %.2f", tc.weight, tc.height, tc.age, tc.sex, got, tc.want)
		}
	}
}

func TestBuildMacroProgress(t *testing.T) {
	actual := macros.Macros{Kcal: 2200, Protein: 110, Carbs: 250, Fat: 80}
	tk := ptr(2000.0)
	tp := ptr(100.0)
	tc2 := ptr(300.0)
	tf := ptr(70.0)

	p := BuildMacroProgress(actual, tk, tp, tc2, tf)

	if !p.HasTarget {
		t.Fatal("HasTarget dovrebbe essere true")
	}
	// Kcal: 2200/2000 = 110% → overflow
	if !p.Kcal.Overflow {
		t.Errorf("Kcal dovrebbe essere overflow (%.0f%%)", p.Kcal.Raw)
	}
	if p.Kcal.Pct != 100 {
		t.Errorf("Kcal.Pct cappato a 100, got %.0f", p.Kcal.Pct)
	}
	// Protein: 110/100 = 110% → overflow
	if !p.Protein.Overflow {
		t.Errorf("Protein dovrebbe essere overflow")
	}
	// Carbs: 250/300 = 83% → no overflow
	if p.Carbs.Overflow {
		t.Errorf("Carbs should not overflow (%.0f%%)", p.Carbs.Raw)
	}
	// Fat: 80/70 ≈ 114% → overflow
	if !p.Fat.Overflow {
		t.Errorf("Fat dovrebbe essere overflow")
	}
	if !p.AnyExceeds {
		t.Errorf("AnyExceeds dovrebbe essere true")
	}

	// No target set → HasTarget=false
	none := BuildMacroProgress(actual, nil, nil, nil, nil)
	if none.HasTarget {
		t.Errorf("no target: HasTarget should be false")
	}
}

func TestActivityFactor(t *testing.T) {
	cases := []struct {
		code string
		want float64
	}{
		{"sedentary", 1.2},
		{"light", 1.375},
		{"moderate", 1.55},
		{"active", 1.725},
		{"athlete", 1.9},
		{"", 1.4},
		{"unknown", 1.4},
	}
	for _, tc := range cases {
		if got := ActivityFactor(tc.code); got != tc.want {
			t.Errorf("ActivityFactor(%q) = %v, want %v", tc.code, got, tc.want)
		}
	}
}

func TestTDEE(t *testing.T) {
	got := TDEE(2000, 1.55)
	if got != 3100 {
		t.Errorf("TDEE(2000, 1.55) = %.2f, want 3100", got)
	}
}

func TestProteinWeight(t *testing.T) {
	for _, tc := range []struct {
		w, h float64
		sex  string
		want float64
	}{
		{120, 194, "M", 95.892},  // BMI 31.9: IBW 87.856, adjusted
		{100, 165, "F", 67.7245}, // BMI 36.7: IBW 56.966, adjusted
		{70, 175, "M", 70},       // BMI 22.9: the body weight
		{120, 194, "", 120},      // no sex, no IBW
		{120, 0, "M", 120},       // no height
	} {
		if got := ProteinWeight(tc.w, tc.h, tc.sex); math.Abs(got-tc.want) > 0.001 {
			t.Errorf("ProteinWeight(%v, %v, %q) = %.4f, want %.4f", tc.w, tc.h, tc.sex, got, tc.want)
		}
	}
}

func ptr(v float64) *float64 { return &v }
