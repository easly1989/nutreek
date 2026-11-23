package household

import (
	"testing"

	"github.com/amon2126/nutreek/internal/domain"
)

func ing(id int64, kcal float64) *domain.Ingredient {
	return &domain.Ingredient{ID: id, Name: "x", KcalPer100: kcal, Unit: domain.UnitGram}
}

func item(id, meal int64, qty float64, unit string, in *domain.Ingredient) domain.MealItemFull {
	return domain.MealItemFull{
		MealItem:   domain.MealItem{ID: id, MealID: meal, Quantity: qty, QuantityUnit: unit, IngredientID: &in.ID},
		Ingredient: in,
	}
}

// Quantities follow the ratio of the goals, round to a step that fits the unit, and a
// hand-set quantity or a skipped meal wins.
func TestQuantity(t *testing.T) {
	pasta := item(1, 10, 100, "g", ing(1, 350))
	egg := item(2, 10, 3, "pz", ing(2, 140))
	p := Person{Ratio: Ratio(1800, 2400)} // 0.75
	if got := p.Quantity(pasta); got != 75 {
		t.Errorf("100 g at 0.75 = %v, want 75", got)
	}
	if got := p.Quantity(egg); got != 2.5 { // 2.25 pieces -> half a piece
		t.Errorf("3 pz at 0.75 = %v, want 2.5", got)
	}
	small := Person{Ratio: 0.58}
	if got := small.Quantity(item(3, 10, 10, "g", ing(3, 900))); got != 5 { // 5.8 g -> 5
		t.Errorf("10 g at 0.58 = %v, want 5", got)
	}
	tiny := Person{Ratio: 0.1}
	if got := tiny.Quantity(item(4, 10, 10, "g", ing(4, 900))); got != 5 { // never zero
		t.Errorf("a small quantity rounds to the step, not to nothing: %v", got)
	}
	if got := (Person{Ratio: 0.5}).Quantity(item(6, 10, 3, "g", ing(6, 0))); got != 3 { // 1.5 g would round up to 5 g
		t.Errorf("a person never gets more than the owner: %v, want 3", got)
	}
	if got := (Person{Ratio: 1}).Quantity(item(5, 10, 103, "g", ing(5, 100))); got != 103 {
		t.Errorf("the owner's own quantity is not rounded: %v", got)
	}

	p.Quantities = map[int64]float64{1: 90, 2: 0}
	if got, manual := p.Quantity(pasta), p.Manual(pasta); got != 90 || !manual {
		t.Errorf("by hand = %v (manual %v), want 90", got, manual)
	}
	if p.Auto(pasta) != 75 || p.Quantity(egg) != 0 {
		t.Errorf("auto stays 75 behind a hand-set quantity, and 0 means not eaten")
	}
	p.Skipped = map[int64]bool{10: true}
	if p.Quantity(pasta) != 0 || p.Factor(pasta) != 0 {
		t.Error("a skipped meal is not eaten")
	}
}

// Macros and what to buy scale with the quantity; a skipped meal and a cheat meal buy
// nothing, and the unknown goal means ratio 1.
func TestMacrosAndNeeds(t *testing.T) {
	pasta := item(1, 10, 100, "g", ing(1, 350))
	meal := domain.MealWithItems{Meal: domain.Meal{ID: 10}, Items: []domain.MealItemFull{pasta}}
	cheated := domain.MealWithItems{Meal: domain.Meal{ID: 11}, Items: []domain.MealItemFull{item(2, 11, 100, "g", ing(2, 100))},
		Cheats: []domain.MealItemFull{item(3, 11, 50, "g", ing(3, 200))}}

	p := Person{Ratio: 0.5}
	if got := p.DayMacros([]domain.MealWithItems{meal}).Kcal; got != 175 { // 50 g of 350 kcal/100 g
		t.Errorf("kcal = %v, want 175", got)
	}
	if got := p.MealMacros(cheated).Kcal; got != 50 { // the cheat's 100 kcal, halved
		t.Errorf("a cheat meal counts what was eaten, scaled: %v", got)
	}

	need := map[int64]float64{}
	p.Needs([]domain.MealWithItems{meal, cheated}, need)
	if len(need) != 1 || need[1] != 50 {
		t.Errorf("needs = %v, want only 50 g of ingredient 1", need)
	}
	p.Skipped = map[int64]bool{10: true}
	need = map[int64]float64{}
	p.Needs([]domain.MealWithItems{meal}, need)
	if len(need) != 0 {
		t.Errorf("a skipped meal needs nothing: %v", need)
	}
	if Ratio(0, 2000) != 1 || Ratio(1800, 0) != 1 {
		t.Error("an unknown goal means ratio 1")
	}
}
