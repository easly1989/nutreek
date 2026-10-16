// Package planner generates a weekly plan automatically by picking, for
// every meal slot of every day, a variation from the nutrition plan's pool
// that brings the daily total close to the macro targets.
//
// The algorithm is greedy and deterministic: with the same seed (derived
// from the week's start date) it always produces the same plan. A pure
// module: it depends only on internal/macros, fully testable without a DB.
package planner

import (
	"math"
	"math/rand"

	"github.com/amon2126/nutreek/internal/macros"
)

// Variation is a candidate variation for a slot, with its total macros
// already computed by the caller.
type Variation struct {
	ID     int64
	Name   string
	Macros macros.Macros
}

// Slot is a meal to fill (e.g. "Colazione") with the available variations.
type Slot struct {
	Name       string
	Variations []Variation
}

// DayTarget is the daily macro targets. Only the fields > 0 contribute to the
// selection score (a user may have set only the kcal).
type DayTarget struct {
	Kcal    float64
	Protein float64
	Carbs   float64
	Fat     float64
}

// VariationSelection is the variation picked for a slot.
type VariationSelection struct {
	SlotName    string
	VariationID int64
	Macros      macros.Macros
}

// GeneratedDay collects the selections of a day (one per filled slot).
type GeneratedDay struct {
	DayOfWeek  int
	Selections []VariationSelection
}

// Generate generates a plan for several days. daySlots[d] are the slots that
// apply to day d (index 0 = Monday). The seed makes the result deterministic;
// every day uses seed+d so different days can vary.
func Generate(daySlots [][]Slot, target DayTarget, seed int64) []GeneratedDay {
	out := make([]GeneratedDay, len(daySlots))
	for d, slots := range daySlots {
		day := GenerateDay(slots, target, seed+int64(d))
		day.DayOfWeek = d
		out[d] = day
	}
	return out
}

// GenerateDay picks a variation for every slot, approaching the daily target.
// Greedy strategy: for every slot it computes the "ideal share" (remaining
// target / remaining slots) and picks the variation with the smallest
// normalized squared deviation from that share. The evaluation order is
// shuffled (an rng from the seed) to give variety among equal deviations.
func GenerateDay(slots []Slot, target DayTarget, seed int64) GeneratedDay {
	rng := rand.New(rand.NewSource(seed))

	// Count the usable slots (with at least one variation).
	slotsLeft := 0
	for _, s := range slots {
		if len(s.Variations) > 0 {
			slotsLeft++
		}
	}

	remaining := target
	var selections []VariationSelection
	for _, s := range slots {
		if len(s.Variations) == 0 {
			continue
		}
		share := DayTarget{
			Kcal:    remaining.Kcal / float64(slotsLeft),
			Protein: remaining.Protein / float64(slotsLeft),
			Carbs:   remaining.Carbs / float64(slotsLeft),
			Fat:     remaining.Fat / float64(slotsLeft),
		}
		order := rng.Perm(len(s.Variations))
		bestIdx := order[0]
		bestScore := math.MaxFloat64
		for _, idx := range order {
			sc := score(s.Variations[idx].Macros, share, target)
			if sc < bestScore {
				bestScore = sc
				bestIdx = idx
			}
		}
		chosen := s.Variations[bestIdx]
		selections = append(selections, VariationSelection{
			SlotName:    s.Name,
			VariationID: chosen.ID,
			Macros:      chosen.Macros,
		})
		remaining.Kcal -= chosen.Macros.Kcal
		remaining.Protein -= chosen.Macros.Protein
		remaining.Carbs -= chosen.Macros.Carbs
		remaining.Fat -= chosen.Macros.Fat
		slotsLeft--
	}
	return GeneratedDay{Selections: selections}
}

// score measures how far m is from the ideal share, normalizing by the daily
// target so the 4 macros are comparable. The kcal weigh more (x4) because
// they are the primary constraint.
func score(m macros.Macros, share, target DayTarget) float64 {
	var s float64
	if target.Kcal > 0 {
		d := (m.Kcal - share.Kcal) / target.Kcal
		s += 4 * d * d
	}
	if target.Protein > 0 {
		d := (m.Protein - share.Protein) / target.Protein
		s += d * d
	}
	if target.Carbs > 0 {
		d := (m.Carbs - share.Carbs) / target.Carbs
		s += d * d
	}
	if target.Fat > 0 {
		d := (m.Fat - share.Fat) / target.Fat
		s += d * d
	}
	return s
}
