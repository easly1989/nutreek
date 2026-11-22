package app

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/amon2126/nutreek/internal/db"
	"github.com/amon2126/nutreek/internal/i18n"
)

// Notifications (#237): the page of the reminders, and the three calls the page's
// script makes to add, remove and try a device. The sending is in internal/notify.

// notificationsPageData is the view model of /settings/notifications.
type notificationsPageData struct {
	authPageData
	Available bool   // false in the demo, where nobody receives anything
	PublicKey string // the VAPID public key the browser subscribes with
	Water     db.Reminder
	Gym       db.Reminder
	Meals     db.Reminder
	Customs   []db.Reminder
	NewCustom db.Reminder // the form that adds one: every day on
	Quiet     db.NotifySettings
	MealNames []mealNameRow
	Devices   int
}

// mealNameRow is a meal name with its time, and whether the «Meals» reminder covers it.
type mealNameRow struct {
	Name string
	Time string
	On   bool
}

func (a *App) handleNotificationsPage(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	data := notificationsPageData{authPageData: a.baseData(r), Available: a.Notify != nil, PublicKey: a.NotifyPublicKey, NewCustom: db.Reminder{Kind: db.KindCustom, Days: 127}}
	data.ActiveNav = "/settings"
	reminders, err := a.Notifications.Reminders(r.Context(), user.ID)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	for _, rem := range reminders {
		switch rem.Kind {
		case db.KindWater:
			data.Water = rem
		case db.KindGym:
			data.Gym = rem
		case db.KindMeals:
			data.Meals = rem
		default:
			data.Customs = append(data.Customs, rem)
		}
	}
	if data.Quiet, err = a.Notifications.Settings(r.Context(), user.ID); err != nil {
		a.serverError(w, r, err)
		return
	}
	subs, err := a.Notifications.Subs(r.Context(), user.ID)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	data.Devices = len(subs)
	mutes, err := a.Notifications.MealMutes(r.Context(), user.ID)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	times, err := a.WeeklyPlans.MealTimeDefaults(r.Context(), user.ID)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	for _, d := range times.List {
		data.MealNames = append(data.MealNames, mealNameRow{Name: d.Name, Time: times.Label(d.Name), On: !mutes[strings.ToLower(d.Name)]})
	}
	if err := a.Render.Render(w, r, http.StatusOK, "notifications", data); err != nil {
		a.serverError(w, r, err)
	}
}

// reminderFromForm reads a reminder's fields: enabled (a hidden 0 and a 1), start_time,
// end_time, every_min, lead_min, day (repeated 0..6), skip_done, title.
func reminderFromForm(r *http.Request, kind string) db.Reminder {
	rem := db.Reminder{Kind: kind, Title: r.PostFormValue("title"), Start: strings.TrimSpace(r.PostFormValue("start_time")), End: strings.TrimSpace(r.PostFormValue("end_time"))}
	for _, v := range r.PostForm["enabled"] {
		rem.Enabled = rem.Enabled || v == "1"
	}
	rem.SkipDone = r.PostFormValue("skip_done") == "1"
	rem.EveryMin, _ = strconv.Atoi(r.PostFormValue("every_min"))
	rem.LeadMin, _ = strconv.Atoi(r.PostFormValue("lead_min"))
	for _, d := range r.PostForm["day"] {
		if n, err := strconv.Atoi(d); err == nil && n >= 0 && n < 7 {
			rem.Days |= 1 << n
		}
	}
	return rem
}

// handleNotificationsReminder POST /settings/notifications/reminder, field kind
// (water, gym, meals or custom, with id for one that exists).
func (a *App) handleNotificationsReminder(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	if err := r.ParseForm(); err != nil {
		http.Error(w, a.t(r, "week.error.invalid_ids"), http.StatusBadRequest)
		return
	}
	kind := r.PostFormValue("kind")
	switch kind {
	case db.KindWater, db.KindGym, db.KindMeals, db.KindCustom:
	default:
		http.Error(w, a.t(r, "week.error.invalid_kind"), http.StatusBadRequest)
		return
	}
	rem := reminderFromForm(r, kind)
	if kind == db.KindMeals {
		// «Meals» has no time of its own: the meals' are used. Keep a valid start.
		rem.Start, rem.Days = "08:00", 127
	}
	if id, err := strconv.ParseInt(r.PostFormValue("id"), 10, 64); err == nil {
		rem.ID = id
	}
	err := a.Notifications.SaveReminder(r.Context(), user.ID, rem)
	switch {
	case errors.Is(err, db.ErrInvalidReminder):
		a.Sessions.Put(r.Context(), "flash", "notifications.flash.invalid")
	case errors.Is(err, db.ErrReminderNotFound):
		http.Error(w, a.t(r, "notifications.error.not_found"), http.StatusNotFound)
		return
	case err != nil:
		a.serverError(w, r, err)
		return
	}
	if kind == db.KindMeals && err == nil {
		// Each meal name of the list is on when its box is ticked, quiet otherwise.
		on := map[string]bool{}
		for _, n := range r.PostForm["meal"] {
			on[strings.ToLower(n)] = true
		}
		times, terr := a.WeeklyPlans.MealTimeDefaults(r.Context(), user.ID)
		if terr != nil {
			a.serverError(w, r, terr)
			return
		}
		var mutes []string
		for _, d := range times.List {
			if !on[strings.ToLower(d.Name)] {
				mutes = append(mutes, d.Name)
			}
		}
		if err := a.Notifications.SetMealMutes(r.Context(), user.ID, mutes); err != nil {
			a.serverError(w, r, err)
			return
		}
	}
	http.Redirect(w, r, "/settings/notifications#"+kind, http.StatusSeeOther)
}

// handleNotificationsCustomDelete POST /settings/notifications/delete, field id.
func (a *App) handleNotificationsCustomDelete(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	id, err := strconv.ParseInt(r.PostFormValue("id"), 10, 64)
	if err != nil {
		a.notFound(w, r)
		return
	}
	if err := a.Notifications.DeleteReminder(r.Context(), user.ID, id); err != nil && !errors.Is(err, db.ErrReminderNotFound) {
		a.serverError(w, r, err)
		return
	}
	http.Redirect(w, r, "/settings/notifications#custom", http.StatusSeeOther)
}

// handleNotificationsQuiet POST /settings/notifications/quiet: quiet_on (a hidden 0
// and a 1), quiet_from, quiet_to.
func (a *App) handleNotificationsQuiet(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	if err := r.ParseForm(); err != nil {
		http.Error(w, a.t(r, "week.error.invalid_ids"), http.StatusBadRequest)
		return
	}
	on := false
	for _, v := range r.PostForm["quiet_on"] {
		on = on || v == "1"
	}
	err := a.Notifications.SaveQuiet(r.Context(), user.ID, on, strings.TrimSpace(r.PostFormValue("quiet_from")), strings.TrimSpace(r.PostFormValue("quiet_to")))
	if errors.Is(err, db.ErrInvalidReminder) {
		a.Sessions.Put(r.Context(), "flash", "notifications.flash.invalid")
	} else if err != nil {
		a.serverError(w, r, err)
		return
	}
	http.Redirect(w, r, "/settings/notifications#silenzio", http.StatusSeeOther)
}

// subscribeBody is what the page's script sends for a device.
type subscribeBody struct {
	Endpoint string `json:"endpoint"`
	Keys     struct {
		P256dh string `json:"p256dh"`
		Auth   string `json:"auth"`
	} `json:"keys"`
	TimeZone string `json:"timezone"`
}

// handleNotificationsSubscribe POST /notifications/subscribe (JSON, the CSRF token in
// the header): adds the device and keeps the browser's time zone.
func (a *App) handleNotificationsSubscribe(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	if a.Notify == nil {
		http.NotFound(w, r)
		return
	}
	var body subscribeBody
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10)).Decode(&body); err != nil {
		http.Error(w, a.t(r, "week.error.invalid_form"), http.StatusBadRequest)
		return
	}
	if err := a.Notifications.AddSub(r.Context(), user.ID, body.Endpoint, body.Keys.P256dh, body.Keys.Auth, r.UserAgent()); err != nil {
		if errors.Is(err, db.ErrInvalidReminder) {
			http.Error(w, a.t(r, "week.error.invalid_form"), http.StatusBadRequest)
			return
		}
		a.serverError(w, r, err)
		return
	}
	if body.TimeZone != "" {
		_ = a.Notifications.SetTimeZone(r.Context(), user.ID, body.TimeZone) // a name Go does not know keeps the last one
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleNotificationsUnsubscribe POST /notifications/unsubscribe (JSON {endpoint}).
func (a *App) handleNotificationsUnsubscribe(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	var body subscribeBody
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10)).Decode(&body); err != nil {
		http.Error(w, a.t(r, "week.error.invalid_form"), http.StatusBadRequest)
		return
	}
	if err := a.Notifications.DeleteSub(r.Context(), user.ID, body.Endpoint); err != nil {
		a.serverError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleNotificationsTest POST /notifications/test: sends a notification to the
// user's devices.
func (a *App) handleNotificationsTest(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	if a.Notify == nil {
		http.NotFound(w, r)
		return
	}
	lang := user.Locale
	if lang == "" {
		lang = i18n.FromContext(r.Context())
	}
	n, err := a.Notify.SendTest(r.Context(), user.ID, lang)
	if err != nil {
		a.Logger.Warn("test notification failed", "err", err)
		http.Error(w, a.t(r, "notifications.error.send"), http.StatusBadGateway)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"sent":` + strconv.Itoa(n) + `}`))
}
