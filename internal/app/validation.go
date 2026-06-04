package app

import "unicode/utf8"

// MaxNameLen e' il limite di lunghezza standard per campi "nome" (recipe,
// template, slot, meal). Volutamente generoso per non frustrare l'utente
// ma piccolo abbastanza da non far esplodere il DB su input malevoli.
const MaxNameLen = 200

// MaxDescriptionLen vale per i campi descrizione lunga (recipe.description).
const MaxDescriptionLen = 2000

// withinLen ritorna true se la stringa s ha al piu' max RUNE (non byte:
// UTF-8 italiano puo' avere caratteri 2-byte e contarli come 2 sarebbe
// scorretto). Stringa vuota e' sempre ok dal punto di vista del limite.
func withinLen(s string, max int) bool {
	return utf8.RuneCountInString(s) <= max
}

// truncateRunes taglia s a max rune, rispettando i confini UTF-8. Usato per
// normalizzare i campi importati (es. da una pagina web) entro i limiti dei form.
func truncateRunes(s string, max int) string {
	if utf8.RuneCountInString(s) <= max {
		return s
	}
	runes := []rune(s)
	return string(runes[:max])
}
