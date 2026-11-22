package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/amon2126/nutreek/internal/db"
	"github.com/amon2126/nutreek/internal/notify"
)

type recordingSender struct{ sent []notify.Message }

func (s *recordingSender) Send(_ context.Context, _ db.PushSub, m notify.Message) (bool, error) {
	s.sent = append(s.sent, m)
	return false, nil
}

// The notifications page and its forms (#237): the reminders and the quiet hours are
// saved per user and refused when out of range; a device is added, tried and removed;
// nothing is sent or stored without the service (the demo).
func TestNotificationsHandlers(t *testing.T) {
	a := newTestApp(t)
	ctx := context.Background()
	alice := mustCreateTestUser(t, a, "alice@x.com", "password-di-prova")
	bob := mustCreateTestUser(t, a, "bob@x.com", "password-di-prova")
	rec := &recordingSender{}
	a.Notify = &notify.Service{Notifications: a.Notifications, Users: a.Users, Sender: rec}
	a.NotifyPublicKey = "BPublicKeyForTheTest"

	call := func(h http.HandlerFunc, method, target string, body string, ctype string, u int) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, target, strings.NewReader(body))
		if ctype != "" {
			r.Header.Set("Content-Type", ctype)
		}
		who := alice
		if u == 2 {
			who = bob
		}
		w := httptest.NewRecorder()
		a.Sessions.LoadAndSave(h).ServeHTTP(w, requestWithUser(r, who))
		return w
	}
	form := func(h http.HandlerFunc, target string, v url.Values, u int) *httptest.ResponseRecorder {
		return call(h, http.MethodPost, target, v.Encode(), "application/x-www-form-urlencoded", u)
	}

	// The page: the three built-in blocks, off, with the VAPID key for the script.
	page := call(a.handleNotificationsPage, http.MethodGet, "/settings/notifications", "", "", 1)
	body := page.Body.String()
	if page.Code != http.StatusOK || !strings.Contains(body, `data-vapid="BPublicKeyForTheTest"`) || !strings.Contains(body, `id="water"`) || !strings.Contains(body, `id="gym"`) || !strings.Contains(body, `id="meals"`) || !strings.Contains(body, `id="silenzio"`) {
		t.Fatalf("page: %d", page.Code)
	}

	// Water: on, every 90 minutes, a window, three days.
	w := form(a.handleNotificationsReminder, "/settings/notifications/reminder", url.Values{
		"kind": {"water"}, "enabled": {"0", "1"}, "start_time": {"09:00"}, "end_time": {"18:00"}, "every_min": {"90"}, "skip_done": {"1"}, "day": {"0", "2", "4"}}, 1)
	if w.Code != http.StatusSeeOther {
		t.Fatalf("save water: %d", w.Code)
	}
	reminders, _ := a.Notifications.Reminders(ctx, alice.ID)
	var water db.Reminder
	for _, r := range reminders {
		if r.Kind == db.KindWater {
			water = r
		}
	}
	if !water.Enabled || water.Start != "09:00" || water.End != "18:00" || water.EveryMin != 90 || water.Days != 1|1<<2|1<<4 || !water.SkipDone {
		t.Errorf("water = %+v", water)
	}
	// Out of range: nothing is saved (an end before the start, a step of 5 minutes, no day).
	for _, bad := range []url.Values{
		{"kind": {"water"}, "enabled": {"1"}, "start_time": {"18:00"}, "end_time": {"09:00"}, "every_min": {"90"}, "day": {"1"}},
		{"kind": {"water"}, "enabled": {"1"}, "start_time": {"09:00"}, "end_time": {"18:00"}, "every_min": {"5"}, "day": {"1"}},
		{"kind": {"gym"}, "enabled": {"1"}, "start_time": {"18:00"}, "lead_min": {"30"}},
		{"kind": {"gym"}, "enabled": {"1"}, "start_time": {"6pm"}, "lead_min": {"30"}, "day": {"1"}},
	} {
		form(a.handleNotificationsReminder, "/settings/notifications/reminder", bad, 1)
	}
	if reminders, _ = a.Notifications.Reminders(ctx, alice.ID); reminders[1].Enabled || reminders[0].EveryMin != 90 {
		t.Errorf("a refused save changed something: %+v", reminders)
	}
	if w := form(a.handleNotificationsReminder, "/settings/notifications/reminder", url.Values{"kind": {"nope"}}, 1); w.Code != http.StatusBadRequest {
		t.Errorf("an unknown kind: %d, want 400", w.Code)
	}

	// Meals: the names left unticked are muted.
	if err := a.WeeklyPlans.SetMealTimeDefault(ctx, alice.ID, "Pranzo", "12:30", ""); err != nil {
		t.Fatal(err)
	}
	if err := a.WeeklyPlans.SetMealTimeDefault(ctx, alice.ID, "Cena", "20:00", ""); err != nil {
		t.Fatal(err)
	}
	form(a.handleNotificationsReminder, "/settings/notifications/reminder", url.Values{"kind": {"meals"}, "enabled": {"1"}, "lead_min": {"15"}, "meal": {"Pranzo"}}, 1)
	if mutes, _ := a.Notifications.MealMutes(ctx, alice.ID); !mutes["cena"] || mutes["pranzo"] || len(mutes) != 1 {
		t.Errorf("mutes = %v, want only cena", mutes)
	}

	// A custom reminder: added, edited by its owner only, deleted.
	form(a.handleNotificationsReminder, "/settings/notifications/reminder", url.Values{"kind": {"custom"}, "enabled": {"1"}, "title": {"Vitamina D"}, "start_time": {"08:00"}, "day": {"0", "1", "2", "3", "4", "5", "6"}}, 1)
	reminders, _ = a.Notifications.Reminders(ctx, alice.ID)
	custom := reminders[len(reminders)-1]
	if custom.Kind != db.KindCustom || custom.Title != "Vitamina D" || custom.Days != 127 {
		t.Fatalf("custom = %+v", custom)
	}
	id := url.Values{"kind": {"custom"}, "id": {itoa(custom.ID)}, "title": {"Hacked"}, "start_time": {"09:00"}, "day": {"1"}}
	if w := form(a.handleNotificationsReminder, "/settings/notifications/reminder", id, 2); w.Code != http.StatusNotFound {
		t.Errorf("another user's custom reminder: %d, want 404", w.Code)
	}
	form(a.handleNotificationsCustomDelete, "/settings/notifications/delete", url.Values{"id": {itoa(custom.ID)}}, 2) // not theirs: nothing happens
	if reminders, _ = a.Notifications.Reminders(ctx, alice.ID); reminders[len(reminders)-1].Kind != db.KindCustom {
		t.Error("another user deleted a reminder")
	}
	form(a.handleNotificationsCustomDelete, "/settings/notifications/delete", url.Values{"id": {itoa(custom.ID)}}, 1)
	if reminders, _ = a.Notifications.Reminders(ctx, alice.ID); len(reminders) != 3 {
		t.Errorf("after the delete: %d reminders, want the 3 built-in", len(reminders))
	}

	// Quiet hours.
	form(a.handleNotificationsQuiet, "/settings/notifications/quiet", url.Values{"quiet_on": {"0"}, "quiet_from": {"23:00"}, "quiet_to": {"06:30"}}, 1)
	if s, _ := a.Notifications.Settings(ctx, alice.ID); s.QuietOn || s.QuietFrom != "23:00" || s.QuietTo != "06:30" {
		t.Errorf("quiet = %+v", s)
	}
	form(a.handleNotificationsQuiet, "/settings/notifications/quiet", url.Values{"quiet_on": {"1"}, "quiet_from": {"25:00"}, "quiet_to": {"06:30"}}, 1)
	if s, _ := a.Notifications.Settings(ctx, alice.ID); s.QuietOn {
		t.Error("a bad time must not be saved")
	}

	// A device: added with the time zone, tried, removed; a bad one is refused.
	sub := `{"endpoint":"https://push.example/abc","keys":{"p256dh":"k","auth":"a"},"timezone":"Europe/Rome"}`
	if w := call(a.handleNotificationsSubscribe, http.MethodPost, "/notifications/subscribe", sub, "application/json", 1); w.Code != http.StatusNoContent {
		t.Fatalf("subscribe: %d", w.Code)
	}
	if s, _ := a.Notifications.Settings(ctx, alice.ID); s.TimeZone != "Europe/Rome" {
		t.Errorf("time zone = %q", s.TimeZone)
	}
	if w := call(a.handleNotificationsSubscribe, http.MethodPost, "/notifications/subscribe", `{"endpoint":"http://insecure","keys":{"p256dh":"k","auth":"a"}}`, "application/json", 1); w.Code != http.StatusBadRequest {
		t.Errorf("an http endpoint: %d, want 400", w.Code)
	}
	if w := call(a.handleNotificationsSubscribe, http.MethodPost, "/notifications/subscribe", `not json`, "application/json", 1); w.Code != http.StatusBadRequest {
		t.Errorf("bad json: %d, want 400", w.Code)
	}
	if w := call(a.handleNotificationsTest, http.MethodPost, "/notifications/test", "", "", 1); w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"sent":1`) || len(rec.sent) != 1 {
		t.Errorf("test: %d %s (sent %d)", w.Code, w.Body.String(), len(rec.sent))
	}
	if w := call(a.handleNotificationsTest, http.MethodPost, "/notifications/test", "", "", 2); !strings.Contains(w.Body.String(), `"sent":0`) || len(rec.sent) != 1 {
		t.Error("another user's devices must not be reached")
	}
	call(a.handleNotificationsUnsubscribe, http.MethodPost, "/notifications/unsubscribe", `{"endpoint":"https://push.example/abc"}`, "application/json", 2) // not theirs
	if subs, _ := a.Notifications.Subs(ctx, alice.ID); len(subs) != 1 {
		t.Error("another user removed a device")
	}
	call(a.handleNotificationsUnsubscribe, http.MethodPost, "/notifications/unsubscribe", `{"endpoint":"https://push.example/abc"}`, "application/json", 1)
	if subs, _ := a.Notifications.Subs(ctx, alice.ID); len(subs) != 0 {
		t.Error("the device must be removed")
	}

	// Without the service (the demo): the page says so, and nothing is stored.
	a.Notify = nil
	if page := call(a.handleNotificationsPage, http.MethodGet, "/settings/notifications", "", "", 1); !strings.Contains(page.Body.String(), `data-available="false"`) {
		t.Error("the demo page must say notifications are not available")
	}
	if w := call(a.handleNotificationsSubscribe, http.MethodPost, "/notifications/subscribe", sub, "application/json", 1); w.Code != http.StatusNotFound {
		t.Errorf("subscribe in the demo: %d, want 404", w.Code)
	}
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }
