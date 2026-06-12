package app

import (
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
)

// ---- /admin/panel -----------------------------------------------------------

type adminPanelPageData struct {
	authPageData
	UserCount      int
	FeatureCount   int
	ActiveCount    int
	PremiumCount   int
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
	premiumCount, err := a.Subscriptions.CountActive(r.Context())
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
		PremiumCount:   premiumCount,
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
	Users          []*adminUserRow
	PremiumEnabled bool
}

type adminUserRow struct {
	ID          int64
	Email       string
	DisplayName string
	CreatedAt   string
	Premium     bool   // sottoscrizione attiva e non scaduta
	SubSource   string // manual | license | payment
	SubExpires  string // vuoto = senza scadenza
}

func (a *App) handleAdminUsers(w http.ResponseWriter, r *http.Request) {
	users, err := a.Users.ListAll(r.Context())
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	subs, err := a.Subscriptions.MapByUser(r.Context())
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	now := time.Now()
	rows := make([]*adminUserRow, len(users))
	for i, u := range users {
		row := &adminUserRow{
			ID:          u.ID,
			Email:       u.Email,
			DisplayName: u.DisplayName,
			CreatedAt:   u.CreatedAt.Format("02/01/2006 15:04"),
		}
		if sub, ok := subs[u.ID]; ok && sub.IsActive(now) {
			row.Premium = true
			row.SubSource = sub.Source
			if sub.ExpiresAt != nil {
				row.SubExpires = sub.ExpiresAt.Format("02/01/2006")
			}
		}
		rows[i] = row
	}
	data := adminUsersPageData{
		authPageData:   a.baseData(r),
		Users:          rows,
		PremiumEnabled: a.Cfg.PremiumEnabled,
	}
	if err := a.Render.Render(w, http.StatusOK, "admin_users", data); err != nil {
		a.serverError(w, r, err)
	}
}

// handleAdminUserGrantPremium POST /admin/panel/users/{id}/premium — assegna
// (o riattiva) la sottoscrizione premium. Campo opzionale expires_at (date).
func (a *App) handleAdminUserGrantPremium(w http.ResponseWriter, r *http.Request) {
	userID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	var expires *time.Time
	if v := r.PostFormValue("expires_at"); v != "" {
		t, err := time.Parse("2006-01-02", v)
		if err != nil {
			a.Sessions.Put(r.Context(), "flash", "Data di scadenza non valida.")
			http.Redirect(w, r, "/admin/panel/users", http.StatusSeeOther)
			return
		}
		// fine giornata: la scadenza include il giorno indicato
		t = t.Add(24*time.Hour - time.Second)
		expires = &t
	}
	if err := a.Subscriptions.Grant(r.Context(), userID, "manual", expires, "", ""); err != nil {
		a.serverError(w, r, err)
		return
	}
	http.Redirect(w, r, "/admin/panel/users", http.StatusSeeOther)
}

// handleAdminUserRevokePremium POST /admin/panel/users/{id}/premium/revoke —
// revoca la sottoscrizione premium (la riga resta per audit).
func (a *App) handleAdminUserRevokePremium(w http.ResponseWriter, r *http.Request) {
	userID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := a.Subscriptions.Revoke(r.Context(), userID); err != nil {
		a.serverError(w, r, err)
		return
	}
	http.Redirect(w, r, "/admin/panel/users", http.StatusSeeOther)
}

// ---- /upgrade ---------------------------------------------------------------

type upgradePageData struct {
	authPageData
	PremiumFeatures []featureRow
	HasPremium      bool
	PremiumEnabled  bool
}

// handleUpgradePage GET /upgrade — pagina informativa sulle feature premium.
// Destinazione dei redirect di requireFeature quando l'accesso e' negato.
func (a *App) handleUpgradePage(w http.ResponseWriter, r *http.Request) {
	features, err := a.Features.List(r.Context())
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	var premium []featureRow
	for _, f := range features {
		if f.IsPremium && f.Enabled {
			premium = append(premium, featureRow{
				Key: f.Key, Name: f.Name, Description: f.Description,
				IsPremium: f.IsPremium, Enabled: f.Enabled, SortOrder: f.SortOrder,
			})
		}
	}
	hasPremium := false
	if u := userFromContext(r.Context()); u != nil {
		hasPremium, _ = a.Subscriptions.IsActive(r.Context(), u.ID)
	}
	data := upgradePageData{
		authPageData:    a.baseData(r),
		PremiumFeatures: premium,
		HasPremium:      hasPremium,
		PremiumEnabled:  a.Cfg.PremiumEnabled,
	}
	if err := a.Render.Render(w, http.StatusOK, "upgrade", data); err != nil {
		a.serverError(w, r, err)
	}
}
