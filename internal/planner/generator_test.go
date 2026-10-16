package planner

import (
	"math"
	"reflect"
	"testing"

	"github.com/amon2126/nutreek/internal/macros"
)

func mk(id int64, kcal, prot, carbs, fat float64) Variation {
	return Variation{ID: id, Macros: macros.Macros{Kcal: kcal, Protein: prot, Carbs: carbs, Fat: fat}}
}

// poolThreeSlots is a synthetic pool with 3 slots, each with variations that
// range above and below the ideal share (~667 kcal per slot out of 2000).
func poolThreeSlots() []Slot {
	return []Slot{
		{Name: "Colazione", Variations: []Variation{
			mk(11, 400, 20, 50, 10),
			mk(12, 667, 40, 70, 18),
			mk(13, 900, 50, 90, 30),
		}},
		{Name: "Pranzo", Variations: []Variation{
			mk(21, 500, 35, 55, 12),
			mk(22, 700, 55, 70, 20),
			mk(23, 1000, 70, 110, 35),
		}},
		{Name: "Cena", Variations: []Variation{
			mk(31, 600, 45, 50, 18),
			mk(32, 700, 50, 60, 22),
			mk(33, 800, 60, 75, 28),
		}},
	}
}

func dayTotal(d GeneratedDay) macros.Macros {
	var out macros.Macros
	for _, s := range d.Selections {
		out = out.Add(s.Macros)
	}
	return out
}

func TestGenerateDayHitsTargetWithinTolerance(t *testing.T) {
	target := DayTarget{Kcal: 2000, Protein: 150, Carbs: 200, Fat: 67}
	day := GenerateDay(poolThreeSlots(), target, 42)

	if len(day.Selections) != 3 {
		t.Fatalf("want 3 selections, got %d", len(day.Selections))
	}
	total := dayTotal(day)
	// Tolleranza realistica: +/-20% sulle kcal.
	lo, hi := target.Kcal*0.8, target.Kcal*1.2
	if total.Kcal < lo || total.Kcal > hi {
		t.Errorf("total kcal %.0f outside [%.0f, %.0f]", total.Kcal, lo, hi)
	}
}

func TestGenerateDeterministic(t *testing.T) {
	target := DayTarget{Kcal: 2000, Protein: 150, Carbs: 200, Fat: 67}
	slots := poolThreeSlots()
	daySlots := [][]Slot{slots, slots, slots, slots, slots, slots, slots}

	a := Generate(daySlots, target, 123456)
	b := Generate(daySlots, target, 123456)
	if !reflect.DeepEqual(a, b) {
		t.Fatal("the same seed must produce the same plan")
	}
	if len(a) != 7 {
		t.Fatalf("want 7 days, got %d", len(a))
	}
	for i, d := range a {
		if d.DayOfWeek != i {
			t.Errorf("day %d: DayOfWeek=%d", i, d.DayOfWeek)
		}
	}
}

func TestGenerateDaySingleVariationForced(t *testing.T) {
	// With a single variation per slot, the selection is forced.
	slots := []Slot{
		{Name: "A", Variations: []Variation{mk(1, 500, 30, 50, 15)}},
		{Name: "B", Variations: []Variation{mk(2, 700, 50, 70, 20)}},
	}
	day := GenerateDay(slots, DayTarget{Kcal: 1200, Protein: 80, Carbs: 120, Fat: 35}, 7)
	if len(day.Selections) != 2 {
		t.Fatalf("want 2 selections, got %d", len(day.Selections))
	}
	if day.Selections[0].VariationID != 1 || day.Selections[1].VariationID != 2 {
		t.Errorf("selezioni inattese: %+v", day.Selections)
	}
}

func TestGenerateDaySkipsEmptySlots(t *testing.T) {
	slots := []Slot{
		{Name: "Vuoto", Variations: nil},
		{Name: "Pieno", Variations: []Variation{mk(9, 600, 40, 60, 18)}},
	}
	day := GenerateDay(slots, DayTarget{Kcal: 600, Protein: 40, Carbs: 60, Fat: 18}, 1)
	if len(day.Selections) != 1 {
		t.Fatalf("want 1 filled slot, got %d", len(day.Selections))
	}
	if day.Selections[0].SlotName != "Pieno" {
		t.Errorf("want slot 'Pieno', got %q", day.Selections[0].SlotName)
	}
}

func TestGenerateDayKcalOnlyTarget(t *testing.T) {
	// Only kcal set (protein/carbs/fat = 0): the selection must still get
	// close to the target kcal without dividing by zero.
	target := DayTarget{Kcal: 1400}
	day := GenerateDay(poolThreeSlots(), target, 99)
	total := dayTotal(day)
	if math.Abs(total.Kcal-target.Kcal) > target.Kcal*0.25 {
		t.Errorf("total kcal %.0f too far from the target %.0f", total.Kcal, target.Kcal)
	}
}

func TestGenerateEmptyDaySlots(t *testing.T) {
	day := GenerateDay(nil, DayTarget{Kcal: 2000}, 1)
	if len(day.Selections) != 0 {
		t.Errorf("no slots => no selections, got %d", len(day.Selections))
	}
}
