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

// poolThreeSlots e' un pool sintetico con 3 slot, ognuno con varianti che
// spaziano sopra e sotto la quota ideale (~667 kcal per slot su 2000).
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
		t.Fatalf("attese 3 selezioni, ottenute %d", len(day.Selections))
	}
	total := dayTotal(day)
	// Tolleranza realistica: +/-20% sulle kcal.
	lo, hi := target.Kcal*0.8, target.Kcal*1.2
	if total.Kcal < lo || total.Kcal > hi {
		t.Errorf("kcal totali %.0f fuori da [%.0f, %.0f]", total.Kcal, lo, hi)
	}
}

func TestGenerateDeterministic(t *testing.T) {
	target := DayTarget{Kcal: 2000, Protein: 150, Carbs: 200, Fat: 67}
	slots := poolThreeSlots()
	daySlots := [][]Slot{slots, slots, slots, slots, slots, slots, slots}

	a := Generate(daySlots, target, 123456)
	b := Generate(daySlots, target, 123456)
	if !reflect.DeepEqual(a, b) {
		t.Fatal("stesso seed deve produrre lo stesso piano")
	}
	if len(a) != 7 {
		t.Fatalf("attesi 7 giorni, ottenuti %d", len(a))
	}
	for i, d := range a {
		if d.DayOfWeek != i {
			t.Errorf("giorno %d: DayOfWeek=%d", i, d.DayOfWeek)
		}
	}
}

func TestGenerateDaySingleVariationForced(t *testing.T) {
	// Con una sola variante per slot, la selezione e' obbligata.
	slots := []Slot{
		{Name: "A", Variations: []Variation{mk(1, 500, 30, 50, 15)}},
		{Name: "B", Variations: []Variation{mk(2, 700, 50, 70, 20)}},
	}
	day := GenerateDay(slots, DayTarget{Kcal: 1200, Protein: 80, Carbs: 120, Fat: 35}, 7)
	if len(day.Selections) != 2 {
		t.Fatalf("attese 2 selezioni, ottenute %d", len(day.Selections))
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
		t.Fatalf("atteso 1 slot riempito, ottenuti %d", len(day.Selections))
	}
	if day.Selections[0].SlotName != "Pieno" {
		t.Errorf("slot atteso 'Pieno', ottenuto %q", day.Selections[0].SlotName)
	}
}

func TestGenerateDayKcalOnlyTarget(t *testing.T) {
	// Solo kcal impostate (protein/carbs/fat = 0): la selezione deve comunque
	// avvicinarsi alle kcal target senza dividere per zero.
	target := DayTarget{Kcal: 1400}
	day := GenerateDay(poolThreeSlots(), target, 99)
	total := dayTotal(day)
	if math.Abs(total.Kcal-target.Kcal) > target.Kcal*0.25 {
		t.Errorf("kcal totali %.0f troppo lontane dal target %.0f", total.Kcal, target.Kcal)
	}
}

func TestGenerateEmptyDaySlots(t *testing.T) {
	day := GenerateDay(nil, DayTarget{Kcal: 2000}, 1)
	if len(day.Selections) != 0 {
		t.Errorf("nessuno slot => nessuna selezione, ottenute %d", len(day.Selections))
	}
}
