package db

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/amon2126/nutreek/internal/domain"
)

// Meal times and reminders (#236): a time per meal name that new meals take, a
// start and an optional end, the day sorted by time, and reminders that hold no
// food and are copied with the week.
func TestMealTimes(t *testing.T) {
	store := newTestStore(t)
	users := NewUsers(store.DB)
	ings := NewIngredients(store.DB)
	plans := NewWeeklyPlans(store.DB)
	templates := NewMealTemplates(store.DB)
	ctx := context.Background()

	alice, _ := users.Create(ctx, "alice@x.com", "h", "Alice")
	bob, _ := users.Create(ctx, "bob@x.com", "h", "Bob")
	oats, _ := ings.CreateLocal(ctx, "Oats", nil, "g", 380, 13, 60, 7, &alice, nil, nil, nil, nil, nil, nil)
	monday, _ := time.Parse("2006-01-02", "2026-04-27")
	plan, _ := plans.GetOrCreateForWeek(ctx, alice, monday)

	// Times are «HH:MM»; the end is optional but must follow the start.
	for _, tc := range []struct {
		start, end string
		ok         bool
	}{{"07:30", "", true}, {"12:30", "13:30", true}, {"00:00", "23:59", true}, {"7:30", "", false}, {"24:00", "", false}, {"12:30", "12:30", false}, {"12:30", "12:00", false}, {"12:30", "9", false}, {"", "", false}} {
		if err := ValidTimes(tc.start, tc.end); (err == nil) != tc.ok {
			t.Errorf("ValidTimes(%q, %q) = %v, want ok=%v", tc.start, tc.end, err, tc.ok)
		}
	}

	// The defaults: found by name in any case, per user.
	if err := plans.SetMealTimeDefault(ctx, alice, "Pranzo", "12:30", "13:30"); err != nil {
		t.Fatal(err)
	}
	if err := plans.SetMealTimeDefault(ctx, alice, "Colazione", "07:30", ""); err != nil {
		t.Fatal(err)
	}
	if err := plans.SetMealTimeDefault(ctx, alice, "pranzo", "13:00", ""); err != nil { // the same name: changes it
		t.Fatal(err)
	}
	if err := plans.SetMealTimeDefault(ctx, alice, "Cena", "25:00", ""); !errors.Is(err, ErrInvalidTime) {
		t.Errorf("a bad time: %v, want ErrInvalidTime", err)
	}
	def, err := plans.MealTimeDefaults(ctx, alice)
	if err != nil || len(def.List) != 2 || def.List[0].Name != "Colazione" || def.List[1].Name != "pranzo" || def.List[1].Start != "13:00" || def.List[1].End != "" {
		t.Fatalf("defaults = %+v, %v; want Colazione 07:30 then pranzo 13:00 (the name as last written)", def.List, err)
	}
	if other, _ := plans.MealTimeDefaults(ctx, bob); len(other.List) != 0 {
		t.Error("another user's defaults must not show")
	}

	// A new meal takes the time of its name; an explicit time wins; no default, no time.
	lunch, err := plans.AddMealAt(ctx, plan.ID, alice, 0, "PRANZO", "", "")
	if err != nil {
		t.Fatal(err)
	}
	breakfast, _ := plans.AddMealAt(ctx, plan.ID, alice, 0, "Colazione", "08:15", "09:00")
	snack, _ := plans.AddMealAt(ctx, plan.ID, alice, 0, "Spuntino", "", "")
	dinner, _ := plans.AddMealAt(ctx, plan.ID, alice, 0, "Cena", "20:00", "")
	if _, err := plans.AddMealAt(ctx, plan.ID, alice, 0, "Brunch", "9:00", ""); !errors.Is(err, ErrInvalidTime) {
		t.Errorf("a bad start: %v, want ErrInvalidTime", err)
	}
	if _, err := plans.AddItem(ctx, lunch, alice, nil, &oats, 40, "g"); err != nil {
		t.Fatal(err)
	}

	// A reminder has no food and a note; it sorts by its time with the meals.
	reminder, err := plans.AddReminder(ctx, plan.ID, alice, 0, "Vitamina D", "1 compressa", "08:00", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := plans.AddReminder(ctx, plan.ID, alice, 0, "X", "", "", ""); !errors.Is(err, ErrInvalidTime) {
		t.Errorf("a reminder with no time: %v, want ErrInvalidTime", err)
	}
	if _, err := plans.AddItem(ctx, reminder, alice, nil, &oats, 10, "g"); !errors.Is(err, ErrMealNotFound) {
		t.Errorf("food in a reminder: %v, want ErrMealNotFound", err)
	}

	names := func() []string {
		full, err := plans.LoadFull(ctx, plan)
		if err != nil {
			t.Fatal(err)
		}
		var out []string
		for _, m := range full.Days[0] {
			s := m.Name
			if l := m.TimeLabel(); l != "" {
				s += "@" + l
			}
			out = append(out, s)
		}
		return out
	}
	want := []string{"Colazione@08:15–09:00", "Vitamina D@08:00", "PRANZO@13:00", "Cena@20:00", "Spuntino"}
	want[0], want[1] = want[1], want[0] // 08:00 comes before 08:15
	got := names()
	if len(got) != len(want) {
		t.Fatalf("day = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("day = %v, want %v (by time, a meal with no time last)", got, want)
		}
	}

	// Changing a time re-sorts the day, for its owner only.
	if day, err := plans.SetMealTime(ctx, snack, alice, "06:00", "06:30"); err != nil || day != 0 {
		t.Fatalf("SetMealTime = %d, %v", day, err)
	}
	if got := names(); got[0] != "Spuntino@06:00–06:30" {
		t.Errorf("after the change: %v, want Spuntino first", got)
	}
	if _, err := plans.SetMealTime(ctx, snack, bob, "07:00", ""); !errors.Is(err, ErrMealNotFound) {
		t.Errorf("another user: %v, want ErrMealNotFound", err)
	}
	if _, err := plans.SetMealTime(ctx, snack, alice, "07:00", "06:00"); !errors.Is(err, ErrInvalidTime) {
		t.Errorf("an end before the start: %v, want ErrInvalidTime", err)
	}
	_, _ = breakfast, dinner

	// The week copied carries the times and the reminder; a template gives its
	// meals the time of their name.
	next, _ := plans.GetOrCreateForWeek(ctx, alice, monday.AddDate(0, 0, 7))
	if err := plans.CopyFrom(ctx, plan.ID, next.ID, alice); err != nil {
		t.Fatal(err)
	}
	copied, _ := plans.LoadFull(ctx, next)
	var gotReminder bool
	for _, m := range copied.Days[0] {
		if m.IsReminder && m.Name == "Vitamina D" && m.Note == "1 compressa" && m.StartTime == "08:00" {
			gotReminder = true
		}
	}
	if !gotReminder || len(copied.Days[0]) != 5 || copied.Days[0][0].TimeLabel() != "06:00–06:30" {
		t.Errorf("the copied day = %+v, want the same five with times and the reminder", copied.Days[0])
	}
	tpl, _ := templates.Create(ctx, alice, "Standard")
	if _, err := templates.AddSlot(ctx, tpl, alice, domain.DayOfWeekEvery, "Colazione"); err != nil {
		t.Fatal(err)
	}
	if _, err := templates.AddSlot(ctx, tpl, alice, domain.DayOfWeekEvery, "Merenda"); err != nil {
		t.Fatal(err)
	}
	third, _ := plans.GetOrCreateForWeek(ctx, alice, monday.AddDate(0, 0, 14))
	if err := plans.ApplyTemplate(ctx, third.ID, alice, tpl); err != nil {
		t.Fatal(err)
	}
	applied, _ := plans.LoadFull(ctx, third)
	if m := applied.Days[2]; len(m) != 2 || m[0].Name != "Colazione" || m[0].StartTime != "07:30" || m[1].Name != "Merenda" || m[1].StartTime != "" {
		t.Errorf("template day = %+v, want Colazione 07:30 then Merenda with no time", m)
	}

	// A row cannot move into a reminder, and the menu leaves it out.
	row, _ := plans.AddItem(ctx, lunch, alice, nil, &oats, 20, "g")
	if _, err := plans.MoveItem(ctx, row, alice, reminder, nil); !errors.Is(err, ErrMealNotFound) {
		t.Errorf("a row into a reminder: %v, want ErrMealNotFound", err)
	}
	mc, _ := plans.MoveContextFor(ctx, row, alice)
	for _, target := range mc.Meals {
		if target.ID == reminder {
			t.Error("a reminder must not be in the «Move to» menu")
		}
	}

	if err := plans.DeleteMealTimeDefault(ctx, alice, "Pranzo"); err != nil {
		t.Fatal(err)
	}
	if def, _ := plans.MealTimeDefaults(ctx, alice); len(def.List) != 1 {
		t.Errorf("after the delete: %+v, want one", def.List)
	}
	if err := users.SetShowMealTimes(ctx, alice, true); err != nil {
		t.Fatal(err)
	}
	if u, _ := users.GetByID(ctx, alice); !u.ShowMealTimes {
		t.Error("ShowMealTimes must be saved")
	}
	if err := users.SetShowMealTimes(ctx, 9999, true); !errors.Is(err, ErrUserNotFound) {
		t.Errorf("an unknown user: %v, want ErrUserNotFound", err)
	}
}
