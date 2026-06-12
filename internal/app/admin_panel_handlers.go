package app

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

// ---- /admin/panel -----------------------------------------------------------

type adminPanelPageData struct {
	authPageData
	UserCount      int
	FeatureCount   int
	ActiveCount    int
	PremiumEnabled bool
}

func (a *App) handleAdminPanel(w http.ResponseWriter, r *http.Request) {
	features, err := a.Features.List(r.Context())
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	users, err := a.Users.ListAll(r.Context())
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	active := 0
	for _, f := range features {
		if f.Enabled {
			active++
		}
	}
	data := adminPanelPageData{
		authPageData:   a.baseData(r),
		UserCount:      len(users),
		FeatureCount:   len(features),
		ActiveCount:    active,
		PremiumEnabled: a.Cfg.PremiumEnabled,
	}
	if err := a.Render.Render(w, http.StatusOK, "admin_panel", data); err != nil {
		a.serverError(w, r, err)
	}
}

// ---- /admin/panel/features --------------------------------------------------

type adminFeaturesPageData struct {
	authPageData
	Features       []featureRow
	PremiumEnabled bool
}

type featureRow struct {
	Key         string
	Name        string
	Description string
	IsPremium   bool
	Enabled     bool
	SortOrder   int
}

func (a *App) handleAdminFeatures(w http.ResponseWriter, r *http.Request) {
	features, err := a.Features.List(r.Context())
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	rows := make([]featureRow, len(features))
	for i, f := range features {
		rows[i] = featureRow{
			Key:         f.Key,
			Name:        f.Name,
			Description: f.Description,
			IsPremium:   f.IsPremium,
			Enabled:     f.Enabled,
			SortOrder:   f.SortOrder,
		}
	}
	data := adminFeaturesPageData{
		authPageData:   a.baseData(r),
		Features:       rows,
		PremiumEnabled: a.Cfg.PremiumEnabled,
	}
	if err := a.Render.Render(w, http.StatusOK, "admin_features", data); err != nil {
		a.serverError(w, r, err)
	}
}

func (a *App) handleAdminFeatureToggle(w http.ResponseWriter, r *http.Request) {
	key := chi.URLParam(r, "key")
	features, err := a.Features.List(r.Context())
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	var current bool
	for _, f := range features {
		if f.Key == key {
			current = f.Enabled
			break
		}
	}
	if err := a.Features.SetEnabled(r.Context(), key, !current); err != nil {
		a.serverError(w, r, err)
		return
	}
	invalidateFeatureCache()
	http.Redirect(w, r, "/admin/panel/features", http.StatusSeeOther)
}

func (a *App) handleAdminFeaturePremium(w http.ResponseWriter, r *http.Request) {
	key := chi.URLParam(r, "key")
	features, err := a.Features.List(r.Context())
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	var current bool
	for _, f := range features {
		if f.Key == key {
			current = f.IsPremium
			break
		}
	}
	if err := a.Features.SetPremium(r.Context(), key, !current); err != nil {
		a.serverError(w, r, err)
		return
	}
	invalidateFeatureCache()
	http.Redirect(w, r, "/admin/panel/features", http.StatusSeeOther)
}

// ---- /admin/panel/users -----------------------------------------------------

type adminUsersPageData struct {
	authPageData
	Users []*adminUserRow
}

type adminUserRow struct {
	ID          int64
	Email       string
	DisplayName string
	CreatedAt   string
}

func (a *App) handleAdminUsers(w http.ResponseWriter, r *http.Request) {
	users, err := a.Users.ListAll(r.Context())
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	rows := make([]*adminUserRow, len(users))
	for i, u := range users {
		rows[i] = &adminUserRow{
			ID:          u.ID,
			Email:       u.Email,
			DisplayName: u.DisplayName,
			CreatedAt:   u.CreatedAt.Format("02/01/2006 15:04"),
		}
	}
	data := adminUsersPageData{
		authPageData: a.baseData(r),
		Users:        rows,
	}
	if err := a.Render.Render(w, http.StatusOK, "admin_users", data); err != nil {
		a.serverError(w, r, err)
	}
}
