package macros

import (
	"math"
	"testing"

	"github.com/amon2126/nutreek/internal/domain"
)

func almostEqual(a, b float64) bool {
	return math.Abs(a-b) < 1e-9
}

func TestAddAndScale(t *testing.T) {
	a := Macros{Kcal: 100, Protein: 10, Carbs: 20, Fat: 5}
	b := Macros{Kcal: 50, Protein: 4, Carbs: 8, Fat: 2}
	got := a.Add(b)
	want := Macros{Kcal: 150, Protein: 14, Carbs: 28, Fat: 7}
	if got != want {
		t.Fatalf("Add: got %+v want %+v", got, want)
	}

	// Scale 1.5x
	scaled := want.Scale(1.5)
	if !almostEqual(scaled.Kcal, 225) || !almostEqual(scaled.Protein, 21) {
		t.Errorf("Scale: got %+v", scaled)
	}

	// Add does not mutate
	if a.Kcal != 100 {
		t.Error("Add mutated the receiver")
	}
}

func TestIsZero(t *testing.T) {
	if !(Macros{}).IsZero() {
		t.Error("zero value !IsZero()")
	}
	if (Macros{Kcal: 1}).IsZero() {
		t.Error("non-zero IsZero()")
	}
}

func TestFromIngredient(t *testing.T) {
	ing := domain.Ingredient{
		KcalPer100:    240,
		ProteinPer100: 8,
		CarbsPer100:   44,
		FatPer100:     2,
		Unit:          domain.UnitGram,
	}
	cases := []struct {
		name string
		qty  float64
		want Macros
	}{
		{"50g", 50, Macros{Kcal: 120, Protein: 4, Carbs: 22, Fat: 1}},
		{"100g identita'", 100, Macros{Kcal: 240, Protein: 8, Carbs: 44, Fat: 2}},
		{"0g zero", 0, Macros{}},
		{"negativi zero", -10, Macros{}},
		{"250g", 250, Macros{Kcal: 600, Protein: 20, Carbs: 110, Fat: 5}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := FromIngredient(ing, tc.qty)
			if got != tc.want {
				t.Errorf("got %+v want %+v", got, tc.want)
			}
		})
	}
}

func TestForRecipeAndPerServing(t *testing.T) {
	pane := domain.Ingredient{KcalPer100: 240, ProteinPer100: 8, CarbsPer100: 44, FatPer100: 2}
	olio := domain.Ingredient{KcalPer100: 900, ProteinPer100: 0, CarbsPer100: 0, FatPer100: 100}
	items := []IngredientItem{
		{Ingredient: pane, Quantity: 200}, // 480 kcal
		{Ingredient: olio, Quantity: 10},  // 90 kcal
	}
	total := ForRecipe(items)
	if !almostEqual(total.Kcal, 570) {
		t.Errorf("kcal totali: got %v want 570", total.Kcal)
	}
	if !almostEqual(total.Fat, 14) { // 200g pane = 4g + 10g olio = 10g
		t.Errorf("fat totali: got %v want 14", total.Fat)
	}

	per := PerServing(total, 2)
	if !almostEqual(per.Kcal, 285) {
		t.Errorf("per serving kcal: got %v want 285", per.Kcal)
	}

	// servings=0 → zero, no panic
	if !PerServing(total, 0).IsZero() {
		t.Error("servings=0 dovrebbe ritornare Macros{}")
	}
	if !PerServing(total, -1).IsZero() {
		t.Error("servings<0 dovrebbe ritornare Macros{}")
	}
}

func TestForRecipeEmpty(t *testing.T) {
	if !ForRecipe(nil).IsZero() {
		t.Error("an empty recipe should be zero")
	}
}

func TestForMealItemIngredient(t *testing.T) {
	pane := domain.Ingredient{KcalPer100: 240, ProteinPer100: 8, CarbsPer100: 44, FatPer100: 2}
	item := domain.MealItemFull{
		MealItem:   domain.MealItem{Quantity: 50},
		Ingredient: &pane,
	}
	got := ForMealItem(item)
	want := Macros{Kcal: 120, Protein: 4, Carbs: 22, Fat: 1}
	if got != want {
		t.Errorf("got %+v want %+v", got, want)
	}
}

func TestForMealItemRecipe(t *testing.T) {
	pane := domain.Ingredient{KcalPer100: 240, ProteinPer100: 8, CarbsPer100: 44, FatPer100: 2}
	olio := domain.Ingredient{KcalPer100: 900, FatPer100: 100}
	recipe := domain.Recipe{Servings: 2}
	// Recipe total: 200g bread (480 kcal) + 10g oil (90 kcal) = 570 kcal,
	// 14g fat. For 2 servings: 285 kcal, 7g fat per serving.
	rItems := []domain.RecipeItemFull{
		{RecipeItem: domain.RecipeItem{Quantity: 200}, Ingredient: pane},
		{RecipeItem: domain.RecipeItem{Quantity: 10}, Ingredient: olio},
	}
	// A 1.5-serving item → 285 * 1.5 = 427.5 kcal.
	item := domain.MealItemFull{
		MealItem:    domain.MealItem{Quantity: 1.5},
		Recipe:      &recipe,
		RecipeItems: rItems,
	}
	got := ForMealItem(item)
	if !almostEqual(got.Kcal, 427.5) {
		t.Errorf("kcal: got %v want 427.5", got.Kcal)
	}
	if !almostEqual(got.Fat, 10.5) { // 7 * 1.5
		t.Errorf("fat: got %v want 10.5", got.Fat)
	}
}

func TestForMealItemRecipeCustomized(t *testing.T) {
	muesli := domain.Ingredient{KcalPer100: 480, ProteinPer100: 10, CarbsPer100: 60, FatPer100: 14, Unit: domain.UnitGram}
	latte := domain.Ingredient{KcalPer100: 48, ProteinPer100: 3.5, CarbsPer100: 5, FatPer100: 1.6, Unit: domain.UnitMilliliter}
	recipe := domain.Recipe{Servings: 1}
	// Base recipe: 100g muesli (480 kcal) + 300ml milk (144 kcal) = 624 kcal.
	rItems := []domain.RecipeItemFull{
		{RecipeItem: domain.RecipeItem{IngredientID: 1, Quantity: 100}, Ingredient: muesli},
		{RecipeItem: domain.RecipeItem{IngredientID: 2, Quantity: 300}, Ingredient: latte},
	}

	// Without variations: uses the base recipe → 624 kcal.
	itemNoCustom := domain.MealItemFull{
		MealItem:    domain.MealItem{Quantity: 1},
		Recipe:      &recipe,
		RecipeItems: rItems,
	}
	got := ForMealItem(itemNoCustom)
	if !almostEqual(got.Kcal, 624) {
		t.Errorf("without variations: got %.2f want 624", got.Kcal)
	}

	// With variations 45g+300ml: uses the customizations → 45*4.8 + 300*0.48 = 216+144 = 360 kcal.
	custom45 := domain.MealItemFull{
		MealItem:    domain.MealItem{Quantity: 1},
		Recipe:      &recipe,
		RecipeItems: rItems,
		Customizations: []domain.MealItemCustomization{
			{IngredientID: 1, Quantity: 45, Ingredient: &muesli},
			{IngredientID: 2, Quantity: 300, Ingredient: &latte},
		},
	}
	got45 := ForMealItem(custom45)
	if !almostEqual(got45.Kcal, 360) {
		t.Errorf("variazioni 45g+300ml: got %.2f want 360", got45.Kcal)
	}
	if almostEqual(got45.Kcal, got.Kcal) {
		t.Errorf("a variation must change the kcal compared with the base recipe")
	}

	// With variations 50g+250ml: 50*4.8 + 250*0.48 = 240+120 = 360 kcal (a mathematical coincidence, M=10L).
	custom50 := domain.MealItemFull{
		MealItem:    domain.MealItem{Quantity: 1},
		Recipe:      &recipe,
		RecipeItems: rItems,
		Customizations: []domain.MealItemCustomization{
			{IngredientID: 1, Quantity: 50, Ingredient: &muesli},
			{IngredientID: 2, Quantity: 250, Ingredient: &latte},
		},
	}
	got50 := ForMealItem(custom50)
	if !almostEqual(got50.Kcal, 360) {
		t.Errorf("variazioni 50g+250ml: got %.2f want 360", got50.Kcal)
	}

	// Changing the protein shows the values really change.
	// 45g muesli (10g prot/100g) + 300ml milk (3.5g/100ml) = 4.5 + 10.5 = 15g protein.
	// 50g muesli + 250ml milk = 5 + 8.75 = 13.75g protein.
	if !almostEqual(got45.Protein, 45*0.1+300*0.035) {
		t.Errorf("proteine 45g+300ml: got %.4f want %.4f", got45.Protein, 45*0.1+300*0.035)
	}
	if !almostEqual(got50.Protein, 50*0.1+250*0.035) {
		t.Errorf("proteine 50g+250ml: got %.4f want %.4f", got50.Protein, 50*0.1+250*0.035)
	}
	if almostEqual(got45.Protein, got50.Protein) {
		t.Errorf("the protein must differ between the two sets of variations")
	}
}

func TestForDayAndWeek(t *testing.T) {
	pane := domain.Ingredient{KcalPer100: 240, ProteinPer100: 8, CarbsPer100: 44, FatPer100: 2}
	makeMeal := func(name string, qty float64) domain.MealWithItems {
		return domain.MealWithItems{
			Meal: domain.Meal{Name: name},
			Items: []domain.MealItemFull{
				{MealItem: domain.MealItem{Quantity: qty}, Ingredient: &pane},
			},
		}
	}
	day := []domain.MealWithItems{
		makeMeal("Colazione", 100), // 240 kcal
		makeMeal("Pranzo", 200),    // 480 kcal
	}
	got := ForDay(day)
	if !almostEqual(got.Kcal, 720) {
		t.Errorf("ForDay kcal: %v want 720", got.Kcal)
	}

	var week [7][]domain.MealWithItems
	for i := range week {
		week[i] = day
	}
	gotW := ForWeek(week)
	if !almostEqual(gotW.Kcal, 720*7) {
		t.Errorf("ForWeek kcal: %v want 5040", gotW.Kcal)
	}
}
