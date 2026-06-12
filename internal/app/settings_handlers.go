package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/amon2126/nutreek/internal/auth"
	"github.com/amon2126/nutreek/internal/domain"
)

type settingsPageData struct {
	authPageData
	DeleteError   string
	PasswordError string
	PasswordOK    bool
}

// userProfileExport è il profilo utente sanitizzato per l'export (no hash).
type userProfileExport struct {
	ID          int64   `json:"id"`
	Email       string  `json:"email"`
	DisplayName string  `json:"display_name"`
	WeightKg    float64 `json:"weight_kg,omitempty"`
	HeightCm    float64 `json:"height_cm,omitempty"`
	CreatedAt   string  `json:"created_at"`
}

// userDataExport è il payload del JSON scaricato da GET /settings/export.
type userDataExport struct {
	ExportedAt  string              `json:"exported_at"`
	User        userProfileExport   `json:"user"`
	WeightLogs  []domain.WeightLog  `json:"weight_logs"`
	WaterLogs   map[string]int      `json:"water_logs"`
	WorkoutLogs []domain.WorkoutLog `json:"workout_logs"`
	Recipes     []*domain.Recipe    `json:"recipes"`
}

func (a *App) handleSettingsPage(w http.ResponseWriter, r *http.Request) {
	data := settingsPageData{authPageData: a.baseData(r)}
	data.ActiveNav = "/settings"
	if flash := r.URL.Query().Get("ok"); flash == "password" {
		data.PasswordOK = true
	}
	if err := a.Render.Render(w, http.StatusOK, "settings", data); err != nil {
		a.serverError(w, r, err)
	}
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
	if verr := auth.VerifyPassword(user.PasswordHash, password); verr != nil {
		data := settingsPageData{authPageData: a.baseData(r)}
		data.ActiveNav = "/settings"
		data.DeleteError = "Password non corretta."
		if rerr := a.Render.Render(w, http.StatusUnprocessableEntity, "settings", data); rerr != nil {
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

	renderErr := func(msg string) {
		data := settingsPageData{authPageData: a.baseData(r)}
		data.ActiveNav = "/settings"
		data.PasswordError = msg
		if err := a.Render.Render(w, http.StatusUnprocessableEntity, "settings", data); err != nil {
			a.serverError(w, r, err)
		}
	}

	if err := auth.VerifyPassword(user.PasswordHash, currentPw); err != nil {
		if errors.Is(err, auth.ErrInvalidCredentials) {
			renderErr("Password attuale non corretta.")
		} else {
			a.serverError(w, r, err)
		}
		return
	}
	if len(newPw) < 8 {
		renderErr("La nuova password deve essere di almeno 8 caratteri.")
		return
	}
	if newPw != confirmPw {
		renderErr("Le password non coincidono.")
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

	http.Redirect(w, r, "/settings?ok=password", http.StatusSeeOther)
}
