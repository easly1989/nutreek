package domain

import "time"

// MealTemplate e' uno schema riutilizzabile di pasti settimanali (es. "5
// pasti", "Solo lavorativi"). L'utente puo' avere piu' template ma uno
// solo "attivo" alla volta — quello applicato di default alle nuove
// settimane.
type MealTemplate struct {
	ID        int64
	UserID    int64
	Name      string
	IsActive  bool
	CreatedAt time.Time
}

// MealTemplateSlot e' una "scheda pasto" dentro un template: nome del
// pasto + giorno della settimana. DayOfWeek=-1 significa "ogni giorno"
// (verra' espanso a 7 occorrenze al momento di applicare il template).
type MealTemplateSlot struct {
	ID         int64
	TemplateID int64
	DayOfWeek  int // -1 = ogni giorno; 0 = lunedi'; 6 = domenica
	Name       string
	SortOrder  int
}

// DayOfWeekEvery e' il sentinel per "ogni giorno della settimana".
const DayOfWeekEvery = -1

// MealTemplateWithSlots aggrega il template + le sue slot per il render.
type MealTemplateWithSlots struct {
	MealTemplate
	Slots []MealTemplateSlot
}
