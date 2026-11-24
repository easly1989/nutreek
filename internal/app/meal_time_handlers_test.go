package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"
)

// Meal times and reminders (#236): the time of a new meal, the dialog and the post
// of a reminder, the time popover, the Settings list, and the CSV column.
func TestMealTimeHandlers(t *testing.T) {
	a := newTestApp(t)
	ctx := context.Background()
	alice := mustCreateTestUser(t, a, "alice@x.com", "password-di-prova")
	bob := mustCreateTestUser(t, a, "bob@x.com", "password-di-prova")
	monday, _ := time.Parse("2006-01-02", "2026-10-12")
	const start = "?start=2026-10-12"

	call := func(h http.HandlerFunc, method, target string, form url.Values, u int, kv ...string) *httptest.ResponseRecorder {
		body := strings.NewReader("")
		if form != nil {
			body = strings.NewReader(form.Encode())
		}
		r := httptest.NewRequest(method, target, body)
		r.Header.Set("HX-Request", "true")
		if form != nil {
			r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		}
		who := alice
		if u == 2 {
			who = bob
		}
		w := httptest.NewRecorder()
		a.Sessions.LoadAndSave(h).ServeHTTP(w, withParams(requestWithUser(r, who), kv...))
		return w
	}
	mealTime := func(name string) (string, string) {
		var s, e *string
		if err := a.Store.DB.QueryRowContext(ctx, `SELECT start_time, end_time FROM meals WHERE name = ?`, name).Scan(&s, &e); err != nil {
			t.Fatalf("meal %s: %v", name, err)
		}
		str := func(p *string) string {
			if p == nil {
				return ""
			}
			return *p
		}
		return str(s), str(e)
	}

	// Settings: the time of a meal name, listed on the page; a bad time is refused.
	if w := call(a.handleSettingsMealTime, http.MethodPost, "/settings/meal-times", url.Values{"name": {"Pranzo"}, "start_time": {"12:30"}, "end_time": {"13:30"}}, 1); w.Code != http.StatusSeeOther {
		t.Fatalf("set a time: %d", w.Code)
	}
	call(a.handleSettingsMealTime, http.MethodPost, "/settings/meal-times", url.Values{"name": {"Cena"}, "start_time": {"25:00"}}, 1)
	page := call(a.handleSettingsSection, http.MethodGet, "/settings/orari", nil, 1).Body.String()
	if !strings.Contains(page, `id="orari"`) || !strings.Contains(page, `value="12:30"`) || !strings.Contains(page, `value="13:30"`) || strings.Contains(page, `value="25:00"`) {
		t.Errorf("the Settings list must show Pranzo 12:30–13:30 and not the bad time")
	}

	// A new meal: empty time takes the name's, a typed one wins, no default no time.
	add := func(name, startTime string) *httptest.ResponseRecorder {
		return call(a.handleAddMeal, http.MethodPost, "/meals"+start, url.Values{"day_of_week": {"0"}, "name": {name}, "start_time": {startTime}}, 1)
	}
	if w := add("pranzo", ""); w.Code != http.StatusOK {
		t.Fatalf("add a meal: %d", w.Code)
	}
	add("Colazione", "07:45")
	add("Spuntino", "")
	if s, e := mealTime("pranzo"); s != "12:30" || e != "13:30" {
		t.Errorf("pranzo = %q–%q, want 12:30–13:30", s, e)
	}
	if s, _ := mealTime("Colazione"); s != "07:45" {
		t.Errorf("Colazione = %q, want 07:45", s)
	}
	if s, _ := mealTime("Spuntino"); s != "" {
		t.Errorf("Spuntino = %q, want no time", s)
	}
	if w := add("Cena", "7pm"); w.Header().Get("HX-Redirect") == "" {
		t.Errorf("a bad time must flash and go back: %d", w.Code)
	}

	// The reminder dialog and post.
	dlg := call(a.handleReminderForm, http.MethodGet, "/week/reminder?day=2&start=2026-10-12", nil, 1)
	if dlg.Code != http.StatusOK || !strings.Contains(dlg.Body.String(), `name="day_of_week" value="2"`) || !strings.Contains(dlg.Body.String(), `action="/reminders?start=2026-10-12"`) {
		t.Fatalf("reminder dialog: %d\n%s", dlg.Code, dlg.Body.String())
	}
	w := call(a.handleAddReminder, http.MethodPost, "/reminders"+start, url.Values{"day_of_week": {"2"}, "name": {"Vitamina D"}, "note": {"1 compressa"}, "start_time": {"08:00"}}, 1)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "Vitamina D") || !strings.Contains(w.Body.String(), "1 compressa") || !strings.Contains(w.Body.String(), "Promemoria") {
		t.Fatalf("add a reminder: %d\n%s", w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), `/items/new`) {
		t.Error("a day with only a reminder must not offer to add food to it")
	}
	if w := call(a.handleAddReminder, http.MethodPost, "/reminders"+start, url.Values{"day_of_week": {"2"}, "name": {"X"}}, 1); w.Header().Get("HX-Redirect") == "" {
		t.Errorf("a reminder with no time must flash and go back: %d", w.Code)
	}
	plan, _ := a.WeeklyPlans.GetOrCreateForWeek(ctx, alice.ID, monday)
	full, _ := a.WeeklyPlans.LoadFull(ctx, plan)
	if s := summarizeWeek(full); s.PlannedDays != 1 {
		t.Errorf("planned days = %d, want 1 (the Monday with meals; Wednesday has only a reminder)", s.PlannedDays)
	}

	// The time popover: the owner changes it, another user is refused.
	var breakfast int64
	_ = a.Store.DB.QueryRowContext(ctx, `SELECT id FROM meals WHERE name = 'Colazione'`).Scan(&breakfast)
	key := strconv.FormatInt(breakfast, 10)
	if w := call(a.handleMealTime, http.MethodPost, "/x"+start, url.Values{"start_time": {"06:30"}, "end_time": {"07:00"}}, 1, "mealID", key); w.Code != http.StatusOK {
		t.Fatalf("change a time: %d", w.Code)
	}
	if s, e := mealTime("Colazione"); s != "06:30" || e != "07:00" {
		t.Errorf("Colazione = %q–%q, want 06:30–07:00", s, e)
	}
	if w := call(a.handleMealTime, http.MethodPost, "/x"+start, url.Values{"start_time": {"06:30"}}, 2, "mealID", key); w.Code != http.StatusNotFound {
		t.Errorf("another user's meal: %d, want 404", w.Code)
	}

	// The CSV: a last «ora» column only when the user asked, with the reminder.
	csvFor := func() string {
		r := httptest.NewRequest(http.MethodGet, "/week/2026-10-12/export", nil)
		u, _ := a.Users.GetByID(ctx, alice.ID)
		w := httptest.NewRecorder()
		a.Sessions.LoadAndSave(http.HandlerFunc(a.handleWeekExport)).ServeHTTP(w, withParams(requestWithUser(r, u), "start", "2026-10-12"))
		return w.Body.String()
	}
	if got := csvFor(); strings.Contains(got, "ora") || strings.Contains(got, "promemoria") {
		t.Errorf("the CSV has times by default:\n%s", got)
	}
	if err := a.Users.SetShowMealTimes(ctx, alice.ID, true); err != nil {
		t.Fatal(err)
	}
	if got := csvFor(); !strings.Contains(got, "grassi,ora\n") || !strings.Contains(got, "Mercoledi,Vitamina D,promemoria,Vitamina D,,,,,,,08:00") {
		t.Errorf("the CSV must end with an ora column and hold the reminder:\n%s", got)
	}
	if w := call(a.handleSettingsShowMealTimes, http.MethodPost, "/settings/show-meal-times", url.Values{"show_meal_times": {"0"}}, 1); w.Code != http.StatusSeeOther {
		t.Fatalf("switch off: %d", w.Code)
	}
	if u, _ := a.Users.GetByID(ctx, alice.ID); u.ShowMealTimes {
		t.Error("the switch must turn it off")
	}
}
