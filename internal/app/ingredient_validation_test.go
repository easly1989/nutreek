package app

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/amon2126/nutreek/internal/domain"
)

func TestValidateIngredient(t *testing.T) {
	a := newTestApp(t)
	r := httptest.NewRequest("POST", "/ingredients", nil)
	ok := ingredientForm{Name: "Oats", Unit: "g", Kcal: "372", Protein: "13,5", Carbs: "58.7", Fat: "7"}

	vals, msg := a.validateIngredient(r, ok)
	if msg != "" {
		t.Fatalf("valid form rejected: %q", msg)
	}
	want := ingredientValues{Unit: domain.UnitGram, Kcal: 372, Protein: 13.5, Carbs: 58.7, Fat: 7}
	if vals != want {
		t.Errorf("values = %+v, want %+v", vals, want)
	}

	tests := []struct {
		name string
		edit func(*ingredientForm)
		want string
	}{
		{"no name", func(f *ingredientForm) { f.Name = "" }, "Il nome è obbligatorio."},
		{"name too long", func(f *ingredientForm) { f.Name = strings.Repeat("à", MaxNameLen+1) }, "max 200 caratteri"},
		{"name of 200 accented letters", func(f *ingredientForm) { f.Name = strings.Repeat("à", MaxNameLen) }, ""},
		{"bad unit", func(f *ingredientForm) { f.Unit = "kg" }, "Unità non valida"},
		{"kcal missing", func(f *ingredientForm) { f.Kcal = "" }, "Kcal/100 non valida: valore mancante"},
		{"protein negative", func(f *ingredientForm) { f.Protein = "-1" }, "Proteine/100 non valide: valore negativo"},
		{"carbs not a number", func(f *ingredientForm) { f.Carbs = "abc" }, "Carbo/100 non validi: non è un numero"},
		{"fat infinite", func(f *ingredientForm) { f.Fat = "Inf" }, "Grassi/100 non validi: non è un numero"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := ok
			tt.edit(&f)
			_, msg := a.validateIngredient(r, f)
			if tt.want == "" && msg != "" || tt.want != "" && !strings.Contains(msg, tt.want) {
				t.Errorf("message = %q, want %q", msg, tt.want)
			}
		})
	}
}
