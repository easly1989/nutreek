package db

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"
)

// The notification tables (#237): built-in reminders made on first use and off, saves
// per user, the quiet hours and the time zone, the devices, the muted meals, the
// log that makes a reminder go out once, and what the ticker reads of a day.
func TestNotifications(t *testing.T) {
	store := newTestStore(t)
	users := NewUsers(store.DB)
	n := NewNotifications(store.DB)
	plans := NewWeeklyPlans(store.DB)
	water := NewWaterLogs(store.DB)
	ctx := context.Background()
	alice, _ := users.Create(ctx, "alice@x.com", "h", "Alice")
	bob, _ := users.Create(ctx, "bob@x.com", "h", "Bob")

	rs, err := n.Reminders(ctx, alice)
	if err != nil || len(rs) != 3 || rs[0].Kind != KindWater || rs[1].Kind != KindGym || rs[2].Kind != KindMeals {
		t.Fatalf("Reminders = %+v, %v; want water, gym, meals", rs, err)
	}
	for _, r := range rs {
		if r.Enabled {
			t.Errorf("%s is on at first: reminders start off", r.Kind)
		}
	}
	if again, _ := n.Reminders(ctx, alice); len(again) != 3 {
		t.Error("the built-in reminders are made once")
	}
	if !rs[1].OnDay(0) || rs[1].OnDay(1) || !rs[1].OnDay(2) {
		t.Errorf("gym days = %d, want Monday, Wednesday and Friday", rs[1].Days)
	}

	// Saves: a built-in by kind, ranges checked, a custom one by id and owner.
	if err := n.SaveReminder(ctx, alice, Reminder{Kind: KindWater, Enabled: true, Start: "09:00", End: "21:00", EveryMin: 60, Days: 127, SkipDone: false}); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []Reminder{
		{Kind: KindWater, Start: "21:00", End: "09:00", EveryMin: 60, Days: 127},
		{Kind: KindWater, Start: "09:00", End: "21:00", EveryMin: 5, Days: 127},
		{Kind: KindGym, Start: "18:00", Days: 0},
		{Kind: KindGym, Start: "18:00", Days: 1, LeadMin: 999},
		{Kind: KindCustom, Start: "08:00", Days: 1},
		{Kind: KindCustom, Title: "x", Start: "8am", Days: 1},
	} {
		if err := n.SaveReminder(ctx, alice, bad); !errors.Is(err, ErrInvalidReminder) {
			t.Errorf("%+v: %v, want ErrInvalidReminder", bad, err)
		}
	}
	rs, _ = n.Reminders(ctx, alice)
	if w := rs[0]; !w.Enabled || w.Start != "09:00" || w.End != "21:00" || w.EveryMin != 60 || w.SkipDone {
		t.Errorf("water = %+v", w)
	}
	if other, _ := n.Reminders(ctx, bob); other[0].Enabled || other[0].Start != "08:00" {
		t.Error("another user's reminders must not change")
	}
	if err := n.SaveReminder(ctx, alice, Reminder{Kind: KindCustom, Title: "Vitamina", Enabled: true, Start: "08:00", Days: 127}); err != nil {
		t.Fatal(err)
	}
	rs, _ = n.Reminders(ctx, alice)
	custom := rs[len(rs)-1]
	if err := n.SaveReminder(ctx, bob, Reminder{ID: custom.ID, Kind: KindCustom, Title: "Mine now", Start: "09:00", Days: 1}); !errors.Is(err, ErrReminderNotFound) {
		t.Errorf("another user's custom reminder: %v, want ErrReminderNotFound", err)
	}
	if err := n.DeleteReminder(ctx, bob, custom.ID); !errors.Is(err, ErrReminderNotFound) {
		t.Errorf("another user's delete: %v, want ErrReminderNotFound", err)
	}
	if err := n.DeleteReminder(ctx, alice, rs[0].ID); !errors.Is(err, ErrReminderNotFound) {
		t.Errorf("deleting a built-in reminder: %v, want ErrReminderNotFound", err)
	}

	// Enabled lists only what is on for a user with a device.
	if got, _ := n.Enabled(ctx); len(got) != 0 {
		t.Errorf("Enabled with no device = %d, want 0", len(got))
	}
	if err := n.AddSub(ctx, alice, "https://push.example/1", "k", "a", "UA"); err != nil {
		t.Fatal(err)
	}
	if got, _ := n.Enabled(ctx); len(got) != 2 { // water and the custom one
		t.Errorf("Enabled = %d, want 2", len(got))
	}
	// The same device signing in as someone else moves to them.
	if err := n.AddSub(ctx, bob, "https://push.example/1", "k2", "a2", ""); err != nil {
		t.Fatal(err)
	}
	if subs, _ := n.Subs(ctx, alice); len(subs) != 0 {
		t.Error("a device belongs to one user")
	}
	for _, bad := range []string{"", "http://x", "ftp://x", "https://", "https://user:pw@push.example/x", "https:///x"} {
		if err := n.AddSub(ctx, bob, bad, "k", "a", ""); !errors.Is(err, ErrInvalidReminder) {
			t.Errorf("endpoint %q: %v, want ErrInvalidReminder", bad, err)
		}
	}
	// A user has a limited number of devices; a device they already have still updates.
	for i := 0; i < MaxDevices-1; i++ { // bob has push.example/1 already
		if err := n.AddSub(ctx, bob, fmt.Sprintf("https://push.example/b%d", i), "k", "a", ""); err != nil {
			t.Fatalf("device %d: %v", i, err)
		}
	}
	if err := n.AddSub(ctx, bob, "https://push.example/one-too-many", "k", "a", ""); !errors.Is(err, ErrInvalidReminder) {
		t.Errorf("device over the cap: %v, want ErrInvalidReminder", err)
	}
	if err := n.AddSub(ctx, bob, "https://push.example/1", "k3", "a3", ""); err != nil {
		t.Errorf("an existing device at the cap must still update: %v", err)
	}

	// Quiet hours and the time zone.
	if s, _ := n.Settings(ctx, alice); !s.QuietOn || s.QuietFrom != "22:00" || s.QuietTo != "07:00" || s.TimeZone != "" {
		t.Errorf("default settings = %+v", s)
	}
	if err := n.SaveQuiet(ctx, alice, false, "23:00", "06:00"); err != nil {
		t.Fatal(err)
	}
	if err := n.SetTimeZone(ctx, alice, "Europe/Rome"); err != nil {
		t.Fatal(err)
	}
	if err := n.SetTimeZone(ctx, alice, "Nowhere/Land"); !errors.Is(err, ErrInvalidReminder) {
		t.Errorf("a time zone Go does not know: %v", err)
	}
	if s, _ := n.Settings(ctx, alice); s.QuietOn || s.QuietFrom != "23:00" || s.TimeZone != "Europe/Rome" {
		t.Errorf("settings = %+v; the time zone must survive a quiet-hours save", s)
	}
	if err := n.SaveQuiet(ctx, alice, true, "25:00", "06:00"); !errors.Is(err, ErrInvalidReminder) {
		t.Errorf("a bad quiet time: %v", err)
	}
	if s, _ := n.Settings(ctx, alice); s.TimeZone != "Europe/Rome" {
		t.Error("a quiet-hours save must keep the time zone")
	}

	// Muted meals, any case; replaced as a whole.
	if err := n.SetMealMutes(ctx, alice, []string{"Cena", " pranzo "}); err != nil {
		t.Fatal(err)
	}
	if m, _ := n.MealMutes(ctx, alice); !m["cena"] || !m["pranzo"] || len(m) != 2 {
		t.Errorf("mutes = %v", m)
	}
	if err := n.SetMealMutes(ctx, alice, nil); err != nil {
		t.Fatal(err)
	}
	if m, _ := n.MealMutes(ctx, alice); len(m) != 0 {
		t.Error("mutes must be replaced")
	}

	// The log: the first mark counts, the second does not; a key is per user.
	if first, _ := n.MarkSent(ctx, alice, "k"); !first {
		t.Error("the first mark must count")
	}
	if first, _ := n.MarkSent(ctx, alice, "k"); first {
		t.Error("the second mark must not count")
	}
	if first, _ := n.MarkSent(ctx, bob, "k"); !first {
		t.Error("the log is per user")
	}

	// What the ticker reads of a day: meals and reminders with a time, the water.
	monday, _ := time.Parse("2006-01-02", "2026-11-23")
	plan, _ := plans.GetOrCreateForWeek(ctx, alice, monday)
	lunch, _ := plans.AddMealAt(ctx, plan.ID, alice, 0, "Pranzo", "12:30", "")
	if _, err := plans.AddMealAt(ctx, plan.ID, alice, 0, "No time", "", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := plans.AddReminder(ctx, plan.ID, alice, 0, "Vitamina D", "1 compressa", "08:00", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := plans.AddMealAt(ctx, plan.ID, alice, 1, "Tuesday", "09:00", ""); err != nil {
		t.Fatal(err)
	}
	slots, err := plans.MealSlotsOn(ctx, alice, monday.Add(15*time.Hour)) // any hour of the Monday
	if err != nil || len(slots) != 2 || slots[0].Name != "Vitamina D" || slots[0].Note != "1 compressa" || slots[1].ID != lunch || slots[1].Start != "12:30" {
		t.Errorf("MealSlotsOn = %+v, %v; want the reminder at 08:00 and the lunch, not the meals with no time or of another day", slots, err)
	}
	if err := water.Log(ctx, alice, monday, 250); err != nil {
		t.Fatal(err)
	}
	if err := water.Log(ctx, alice, monday, 500); err != nil {
		t.Fatal(err)
	}
	total, last, err := water.StateOn(ctx, alice, monday)
	if err != nil || total != 750 || last.IsZero() || time.Since(last) > time.Minute {
		t.Errorf("StateOn = %d, %v, %v; want 750 ml and a log from just now", total, last, err)
	}
	if total, last, _ := water.StateOn(ctx, bob, monday); total != 0 || !last.IsZero() {
		t.Error("another user's water must not show")
	}
}
