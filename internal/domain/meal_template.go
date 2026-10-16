package domain

import "time"

// MealTemplate is a reusable scheme of weekly meals (e.g. "5 pasti",
// "Solo lavorativi"). The user can have several templates but only one
// "active" at a time — the one applied by default to new weeks.
type MealTemplate struct {
	ID        int64
	UserID    int64
	Name      string
	IsActive  bool
	CreatedAt time.Time
}

// MealTemplateSlot is a "meal card" inside a template: meal name + day of
// the week. DayOfWeek=-1 means "every day" (expanded to 7 occurrences when
// the template is applied).
type MealTemplateSlot struct {
	ID         int64
	TemplateID int64
	DayOfWeek  int // -1 = every day; 0 = Monday; 6 = Sunday
	Name       string
	SortOrder  int
}

// DayOfWeekEvery is the sentinel for "every day of the week".
const DayOfWeekEvery = -1

// MealTemplateWithSlots joins the template + its slots for rendering.
type MealTemplateWithSlots struct {
	MealTemplate
	Slots []MealTemplateSlot
}
