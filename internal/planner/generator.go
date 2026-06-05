// Package planner genera automaticamente un piano settimanale selezionando,
// per ogni slot pasto di ogni giorno, una variante dal pool del piano
// nutrizionista che avvicini il totale giornaliero ai target macro.
//
// L'algoritmo e' greedy e deterministico: a parita' di seed (derivato dalla
// data di inizio settimana) produce sempre lo stesso piano. Modulo puro:
// dipende solo da internal/macros, completamente testabile senza DB.
package planner

import (
	"math"
	"math/rand"

	"github.com/amon2126/nutreek/internal/macros"
)

// Variation e' una variante candidata per uno slot, con i suoi macro totali
// gia' calcolati dal chiamante.
type Variation struct {
	ID     int64
	Name   string
	Macros macros.Macros
}

// Slot e' un pasto da riempire (es. "Colazione") con le varianti disponibili.
type Slot struct {
	Name       string
	Variations []Variation
}

// DayTarget sono i target macro giornalieri. Solo i campi > 0 contribuiscono
// al punteggio di selezione (un utente puo' aver impostato solo le kcal).
type DayTarget struct {
	Kcal    float64
	Protein float64
	Carbs   float64
	Fat     float64
}

// VariationSelection e' la variante scelta per uno slot.
type VariationSelection struct {
	SlotName    string
	VariationID int64
	Macros      macros.Macros
}

// GeneratedDay raccoglie le selezioni di un giorno (una per slot riempito).
type GeneratedDay struct {
	DayOfWeek  int
	Selections []VariationSelection
}

// Generate genera un piano per piu' giorni. daySlots[d] sono gli slot
// applicabili al giorno d (indice 0 = lunedi'). Il seed rende il risultato
// deterministico; ogni giorno usa seed+d cosi' giorni diversi possono variare.
func Generate(daySlots [][]Slot, target DayTarget, seed int64) []GeneratedDay {
	out := make([]GeneratedDay, len(daySlots))
	for d, slots := range daySlots {
		day := GenerateDay(slots, target, seed+int64(d))
		day.DayOfWeek = d
		out[d] = day
	}
	return out
}

// GenerateDay seleziona una variante per ogni slot avvicinandosi al target
// giornaliero. Strategia greedy: ad ogni slot calcola la "quota ideale"
// (target residuo / slot rimanenti) e sceglie la variante con lo scarto
// quadratico normalizzato minore rispetto a quella quota. L'ordine di
// valutazione e' mescolato (rng da seed) per dare varieta' a parita' di scarto.
func GenerateDay(slots []Slot, target DayTarget, seed int64) GeneratedDay {
	rng := rand.New(rand.NewSource(seed))

	// Conta gli slot utilizzabili (con almeno una variante).
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

// score misura quanto m si discosta dalla quota ideale share, normalizzando
// per il target giornaliero cosi' i 4 macro sono comparabili. Le kcal pesano
// di piu' (x4) perche' sono il vincolo primario.
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
