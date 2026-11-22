package notify

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/amon2126/nutreek/internal/db"
)

func at0(day, hhmm string) time.Time {
	t, _ := time.ParseInLocation("2006-01-02 15:04", day+" "+hhmm, time.UTC)
	return t
}

// Quiet hours run through midnight and end where they say.
func TestInQuiet(t *testing.T) {
	for _, tc := range []struct {
		now, from, to string
		want          bool
	}{
		{"23:30", "22:00", "07:00", true}, {"06:59", "22:00", "07:00", true}, {"07:00", "22:00", "07:00", false},
		{"12:00", "22:00", "07:00", false}, {"22:00", "22:00", "07:00", true}, {"13:00", "12:00", "14:00", true},
		{"14:00", "12:00", "14:00", false}, {"03:00", "00:00", "00:00", false},
	} {
		if got := InQuiet(at0("2026-11-23", tc.now), tc.from, tc.to); got != tc.want {
			t.Errorf("InQuiet(%s, %s-%s) = %v, want %v", tc.now, tc.from, tc.to, got, tc.want)
		}
	}
}

// What is due, per kind: the water slots, the gym lead, the meals (muted ones and
// reminders with notes), the custom ones, the days, and the water skips.
func TestDue(t *testing.T) {
	const day = "2026-11-23" // a Monday
	in := func(hhmm string) Input { return Input{Now: at0(day, hhmm), Window: 2 * time.Minute, Lang: "en"} }
	water := db.Reminder{Kind: db.KindWater, Enabled: true, Start: "08:00", End: "20:00", EveryMin: 120, Days: 127, SkipDone: true}

	if got := Due(water, in("10:00")); len(got) != 1 || got[0].Key != "water:"+day+":10:00" {
		t.Errorf("water at 10:00 = %+v, want the 10:00 slot", got)
	}
	if got := Due(water, in("10:01")); len(got) != 1 { // a late tick still catches the slot
		t.Errorf("water at 10:01 = %d, want the 10:00 slot", len(got))
	}
	if got := Due(water, in("10:03")); len(got) != 0 {
		t.Errorf("water at 10:03 = %d, want none (the window is 2 minutes)", len(got))
	}
	if got := Due(water, in("21:00")); len(got) != 0 {
		t.Error("water after the last slot")
	}
	done := in("12:00")
	done.Water = &WaterState{Total: 2100, Target: 2000}
	if got := Due(water, done); len(got) != 0 {
		t.Error("water must be skipped when the goal is reached")
	}
	lately := in("12:00")
	lately.Water = &WaterState{Total: 500, Target: 2000, Last: at0(day, "11:30")}
	if got := Due(water, lately); len(got) != 0 {
		t.Error("water must be skipped when it was logged lately")
	}
	lately.Water.Last = at0(day, "10:00")
	if got := Due(water, lately); len(got) != 1 || !strings.Contains(got[0].Message.Body, "500 of 2000") {
		t.Errorf("water with old logs = %+v, want one, with the total in the body", got)
	}
	noSkip := water
	noSkip.SkipDone = false
	if got := Due(noSkip, done); len(got) != 1 {
		t.Error("the skip must be optional")
	}
	if got := Due(db.Reminder{Kind: db.KindWater, Start: "08:00", End: "20:00", EveryMin: 120, Days: 127}, in("10:00")); len(got) != 0 {
		t.Error("a reminder that is off sends nothing")
	}

	gym := db.Reminder{Kind: db.KindGym, Enabled: true, Start: "18:30", LeadMin: 30, Days: 1 << 0} // Monday only
	if got := Due(gym, in("18:00")); len(got) != 1 || got[0].Key != "gym:"+day || !strings.Contains(got[0].Message.Body, "18:30") {
		t.Errorf("gym 30 minutes before = %+v", got)
	}
	gym.Days = 1 << 1
	if got := Due(gym, in("18:00")); len(got) != 0 {
		t.Error("gym on another weekday")
	}

	meals := db.Reminder{Kind: db.KindMeals, Enabled: true, LeadMin: 10, Days: 127}
	mi := in("12:20")
	mi.Meals = []db.MealSlot{{ID: 1, Name: "Pranzo", Start: "12:30"}, {ID: 2, Name: "Vitamina D", Start: "12:30", Note: "1 compressa"}, {ID: 3, Name: "Cena", Start: "20:00"}}
	mi.Muted = map[string]bool{"vitamina d": false, "cena": true}
	got := Due(meals, mi)
	if len(got) != 2 || got[0].Message.Title != "Pranzo" || got[1].Message.Body != "1 compressa · 12:30" {
		t.Errorf("meals = %+v, want Pranzo and the reminder with its note", got)
	}
	mi.Muted["pranzo"] = true
	if got := Due(meals, mi); len(got) != 1 {
		t.Errorf("a muted meal must be left out: %d", len(got))
	}

	custom := db.Reminder{ID: 7, Kind: db.KindCustom, Enabled: true, Title: "Vitamina", Start: "08:00", Days: 127}
	if got := Due(custom, in("08:00")); len(got) != 1 || got[0].Message.Title != "Vitamina" || got[0].Key != "custom:7:"+day {
		t.Errorf("custom = %+v", got)
	}
}

type fakeSender struct {
	sent []Message
	gone map[string]bool
}

func (f *fakeSender) Send(_ context.Context, sub db.PushSub, m Message) (bool, error) {
	if f.gone[sub.Endpoint] {
		return true, nil
	}
	f.sent = append(f.sent, m)
	return false, nil
}

// The ticker sends once, in the user's time zone, outside their quiet hours, to every
// device, and forgets a device the push service says is gone.
func TestTick(t *testing.T) {
	store, err := db.Open(filepath.Join(t.TempDir(), "n.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	ctx := context.Background()
	if err := store.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	users, notif := db.NewUsers(store.DB), db.NewNotifications(store.DB)
	alice, _ := users.Create(ctx, "alice@x.com", "h", "Alice")
	bob, _ := users.Create(ctx, "bob@x.com", "h", "Bob")
	for _, u := range []int64{alice, bob} {
		if err := notif.SaveReminder(ctx, u, db.Reminder{Kind: db.KindCustom, Title: "Vitamina", Enabled: true, Start: "08:00", Days: 127}); err != nil {
			t.Fatal(err)
		}
	}
	// Alice has two devices and lives in Rome; Bob has one and no time zone yet (UTC).
	_ = notif.AddSub(ctx, alice, "https://push.example/a1", "k", "a", "")
	_ = notif.AddSub(ctx, alice, "https://push.example/a2", "k", "a", "")
	_ = notif.AddSub(ctx, bob, "https://push.example/b1", "k", "a", "")
	if err := notif.SetTimeZone(ctx, alice, "Europe/Rome"); err != nil {
		t.Fatal(err)
	}
	fake := &fakeSender{gone: map[string]bool{}}
	svc := &Service{Notifications: notif, Users: users, Sender: fake}

	// 07:00 UTC is 08:00 in Rome in November: Alice's two devices and Bob (7:00 UTC: not his 08:00).
	now, _ := time.Parse(time.RFC3339, "2026-11-23T07:00:00Z")
	if n, err := svc.Tick(ctx, now); err != nil || n != 2 {
		t.Fatalf("Tick = %d, %v; want 2 (Alice's two devices)", n, err)
	}
	if n, _ := svc.Tick(ctx, now.Add(30*time.Second)); n != 0 {
		t.Errorf("the same reminder again = %d, want 0", n)
	}
	// 08:00 UTC: Bob's 08:00, and Alice's reminder is not due again.
	if n, _ := svc.Tick(ctx, now.Add(time.Hour)); n != 1 {
		t.Errorf("Bob's reminder = %d, want 1", n)
	}

	// Quiet hours: the next day, 08:00 Rome falls in a band from 07:00 to 09:00.
	if err := notif.SaveQuiet(ctx, alice, true, "07:00", "09:00"); err != nil {
		t.Fatal(err)
	}
	next := now.Add(24 * time.Hour)
	before := len(fake.sent)
	if n, _ := svc.Tick(ctx, next); n != 0 || len(fake.sent) != before {
		t.Errorf("in the quiet hours = %d, want 0", n)
	}

	// A device the push service dropped is forgotten.
	if err := notif.SaveQuiet(ctx, alice, false, "07:00", "09:00"); err != nil {
		t.Fatal(err)
	}
	fake.gone["https://push.example/a1"] = true
	if n, _ := svc.Tick(ctx, next.Add(30*time.Second)); n != 1 {
		t.Errorf("with one device gone = %d, want 1", n)
	}
	if subs, _ := notif.Subs(ctx, alice); len(subs) != 1 || subs[0].Endpoint != "https://push.example/a2" {
		t.Errorf("devices left = %+v, want a2 only", subs)
	}
}
