package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"path"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/amon2126/nutreek/internal/auth"
	"github.com/amon2126/nutreek/internal/db"
	"github.com/amon2126/nutreek/internal/domain"
)

type settingsPageData struct {
	authPageData
	DeleteError   string
	PasswordError string
	PasswordOK    bool
	APITokens     []domain.APIToken
	NewTokenPlain string // set only right after creation

	// MealTimes is the time each meal name gets (#236).
	MealTimes []db.MealTimeDefault

	// Section is the page of Settings shown (aspetto, orari, elenchi, app, account), "" for
	// the index; SectionTitle is the catalog key of its title.
	Section      string
	SectionTitle string
	// Security is the page that lists the second step (#335); nil elsewhere.
	Security *securityView
}

// settingsSections are the pages of Settings (#251) with the catalog key of their title.
var settingsSections = map[string]string{
	"aspetto":   "settings.page.appearance",
	"orari":     "meal_times.settings.title",
	"elenchi":   "settings.page.lists",
	"app":       "settings.app.title",
	"account":   "settings.page.account",
	"sicurezza": "security.title",
}

// withSection says which page of Settings a view model is for.
func (d *settingsPageData) withSection(section string) {
	d.Section, d.SectionTitle = section, settingsSections[section]
}

// fillMealTimes loads the page's «Meal times» list.
func (a *App) fillMealTimes(r *http.Request, data *settingsPageData) {
	if u := userFromContext(r.Context()); u != nil {
		if t, err := a.WeeklyPlans.MealTimeDefaults(r.Context(), u.ID); err == nil {
			data.MealTimes = t.List
		}
	}
}

// handleSettingsKeepAwake POST /settings/keep-awake
// Body: keep_awake (repeated: a hidden 0 and, when the switch is on, a 1).
// Saves whether the phone screen stays on while a page is open (#249).
func (a *App) handleSettingsKeepAwake(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	if err := r.ParseForm(); err != nil {
		http.Error(w, a.t(r, "week.error.invalid_ids"), http.StatusBadRequest)
		return
	}
	on := false
	for _, v := range r.PostForm["keep_awake"] {
		on = on || v == "1"
	}
	if err := a.Users.SetKeepAwake(r.Context(), user.ID, on); err != nil {
		a.serverError(w, r, err)
		return
	}
	http.Redirect(w, r, "/settings/aspetto", http.StatusSeeOther)
}

// userProfileExport is the sanitized user profile for the export (no hash).
type userProfileExport struct {
	ID          int64   `json:"id"`
	Email       string  `json:"email"`
	DisplayName string  `json:"display_name"`
	WeightKg    float64 `json:"weight_kg,omitempty"`
	HeightCm    float64 `json:"height_cm,omitempty"`
	CreatedAt   string  `json:"created_at"`
}

// userDataExport is the payload of the JSON downloaded from GET /settings/export.
type userDataExport struct {
	ExportedAt  string              `json:"exported_at"`
	User        userProfileExport   `json:"user"`
	WeightLogs  []domain.WeightLog  `json:"weight_logs"`
	WaterLogs   map[string]int      `json:"water_logs"`
	WorkoutLogs []domain.WorkoutLog `json:"workout_logs"`
	Recipes     []*domain.Recipe    `json:"recipes"`
}

// handleSettingsPage GET /settings: the index of the pages of Settings.
func (a *App) handleSettingsPage(w http.ResponseWriter, r *http.Request) {
	data := settingsPageData{authPageData: a.baseData(r)}
	data.ActiveNav = "/settings"
	if err := a.Render.Render(w, r, http.StatusOK, "settings", data); err != nil {
		a.serverError(w, r, err)
	}
}

// handleSettingsSection GET /settings/{section}: one page of Settings (aspetto, orari,
// elenchi, app or account); another name is a 404.
func (a *App) handleSettingsSection(w http.ResponseWriter, r *http.Request) {
	section := chi.URLParam(r, "section")
	if section == "" { // a call that did not go through the router
		section = path.Base(r.URL.Path)
	}
	if _, ok := settingsSections[section]; !ok {
		a.notFound(w, r)
		return
	}
	user := userFromContext(r.Context())
	data := settingsPageData{authPageData: a.baseData(r)}
	data.ActiveNav = "/settings"
	data.withSection(section)
	if section == "sicurezza" {
		if a.TwoFactor == nil || a.Demo != nil {
			a.notFound(w, r)
			return
		}
		sv, err := a.loadSecurity(r, user)
		if err != nil {
			a.serverError(w, r, err)
			return
		}
		data.Security = sv
	}
	if flash := r.URL.Query().Get("ok"); flash == "password" {
		data.PasswordOK = true
	}
	tokens, err := a.APITokens.List(r.Context(), user.ID)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	data.APITokens = tokens
	a.fillMealTimes(r, &data)
	if err := a.Render.Render(w, r, http.StatusOK, "settings", data); err != nil {
		a.serverError(w, r, err)
	}
}

func (a *App) handleSettingsAPITokenCreate(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	name := r.PostFormValue("name")
	if name == "" || !withinLen(name, 100) {
		http.Redirect(w, r, "/settings/account", http.StatusSeeOther)
		return
	}
	plain, err := a.APITokens.Create(r.Context(), user.ID, name)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	tokens, err := a.APITokens.List(r.Context(), user.ID)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	data := settingsPageData{authPageData: a.baseData(r)}
	data.ActiveNav = "/settings"
	data.withSection("account")
	data.APITokens = tokens
	data.NewTokenPlain = plain
	a.fillMealTimes(r, &data)
	if err := a.Render.Render(w, r, http.StatusOK, "settings", data); err != nil {
		a.serverError(w, r, err)
	}
}

func (a *App) handleSettingsAPITokenRevoke(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if rErr := a.APITokens.Revoke(r.Context(), id, user.ID); rErr != nil && !errors.Is(rErr, db.ErrAPITokenNotFound) {
		a.serverError(w, r, rErr)
		return
	}
	http.Redirect(w, r, "/settings/account", http.StatusSeeOther)
}

func (a *App) handleUserDataExport(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	ctx := r.Context()

	from := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Now().AddDate(1, 0, 0)

	weightLogs, _ := a.WeightLogs.ListWeightLogs(ctx, user.ID, 10000)
	waterLogs, _ := a.WaterLogs.ForDateRange(ctx, user.ID, from, to)
	workoutLogs, _ := a.WorkoutLogs.ListWorkoutLogs(ctx, user.ID, from, to)
	recipes, _ := a.Recipes.ListVisible(ctx, user.ID)

	var weightKg, heightCm float64
	if user.WeightKg != nil {
		weightKg = *user.WeightKg
	}
	if user.HeightCm != nil {
		heightCm = *user.HeightCm
	}

	payload := userDataExport{
		ExportedAt: time.Now().UTC().Format(time.RFC3339),
		User: userProfileExport{
			ID:          user.ID,
			Email:       user.Email,
			DisplayName: user.DisplayName,
			WeightKg:    weightKg,
			HeightCm:    heightCm,
			CreatedAt:   user.CreatedAt.Format(time.RFC3339),
		},
		WeightLogs:  weightLogs,
		WaterLogs:   waterLogs,
		WorkoutLogs: workoutLogs,
		Recipes:     recipes,
	}

	filename := fmt.Sprintf("nutreek-export-%s.json", time.Now().Format("20060102"))
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="`+filename+`"`)
	_ = json.NewEncoder(w).Encode(payload)
}

func (a *App) handleDeleteAccount(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	ctx := r.Context()

	password := r.PostFormValue("password")
	// The tries are limited for the account: a stolen session must not guess the password freely (#345).
	allowed := a.pwTryAllowed(user.ID)
	if !allowed || auth.VerifyPassword(user.PasswordHash, password) != nil {
		data := settingsPageData{authPageData: a.baseData(r)}
		data.ActiveNav = "/settings"
		data.withSection("account")
		data.DeleteError = a.t(r, "settings.delete.err_password")
		status := http.StatusUnprocessableEntity
		if !allowed {
			data.DeleteError = a.t(r, "auth.error.too_many_attempts")
			status = http.StatusTooManyRequests
		}
		a.fillMealTimes(r, &data)
		if rerr := a.Render.Render(w, r, status, "settings", data); rerr != nil {
			a.serverError(w, r, rerr)
		}
		return
	}

	if err := a.Users.Delete(ctx, user.ID); err != nil {
		a.serverError(w, r, err)
		return
	}

	_ = a.Sessions.Destroy(ctx)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (a *App) handleChangePassword(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	ctx := r.Context()

	currentPw := r.PostFormValue("current_password")
	newPw := r.PostFormValue("new_password")
	confirmPw := r.PostFormValue("confirm_password")

	renderErrStatus := func(status int, msg string) {
		data := settingsPageData{authPageData: a.baseData(r)}
		data.ActiveNav = "/settings"
		data.withSection("account")
		data.PasswordError = msg
		a.fillMealTimes(r, &data)
		if err := a.Render.Render(w, r, status, "settings", data); err != nil {
			a.serverError(w, r, err)
		}
	}
	renderErr := func(msg string) { renderErrStatus(http.StatusUnprocessableEntity, msg) }

	// The tries are limited for the account: a stolen session must not guess the password freely (#345).
	if !a.pwTryAllowed(user.ID) {
		renderErrStatus(http.StatusTooManyRequests, a.t(r, "auth.error.too_many_attempts"))
		return
	}
	if err := auth.VerifyPassword(user.PasswordHash, currentPw); err != nil {
		if errors.Is(err, auth.ErrInvalidCredentials) {
			renderErr(a.t(r, "settings.password.err_current"))
		} else {
			a.serverError(w, r, err)
		}
		return
	}
	if len(newPw) < 8 {
		renderErr(a.t(r, "settings.password.err_short"))
		return
	}
	if newPw != confirmPw {
		renderErr(a.t(r, "settings.password.err_mismatch"))
		return
	}

	hash, err := auth.HashPassword(newPw)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	if err := a.Users.UpdatePassword(ctx, user.ID, hash); err != nil {
		a.serverError(w, r, err)
		return
	}
	a.Sessions.Put(ctx, sessionKeyPwStamp, pwStamp(hash)) // other sessions stop; this one goes on

	http.Redirect(w, r, "/settings/account?ok=password", http.StatusSeeOther)
}
