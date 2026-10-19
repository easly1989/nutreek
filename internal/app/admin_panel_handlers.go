package app

import (
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/amon2126/nutreek/internal/domain"
	"github.com/amon2126/nutreek/internal/i18n"
)

// ---- /admin/panel -----------------------------------------------------------

// adminPanelPageData is the Overview tab: instance numbers, the features
// with their two switches, the OpenFoodFacts state (via JS).
type adminPanelPageData struct {
	authPageData
	UserCount      int
	CoachCount     int
	PremiumCount   int
	Features       []featureRow
	ActiveCount    int
	PremiumFeats   int // active features marked premium
	PremiumEnabled bool
	HasOFF         bool        // the OpenFoodFacts sync is configured
	Updates        *updateView // the update check (#167); nil in the demo build
}

type featureRow struct {
	Key         string
	Name        string
	Description string
	IsPremium   bool
	Enabled     bool
	SortOrder   int
}

// newFeatureRow names a feature in lang by its key (#158); a feature with no
// translation keeps the name and description stored in the database.
func newFeatureRow(lang string, f domain.Feature) featureRow {
	k := "reference.feature." + f.Key
	return featureRow{
		Key: f.Key, Name: i18n.TOr(lang, k+".name", f.Name), Description: i18n.TOr(lang, k+".description", f.Description),
		IsPremium: f.IsPremium, Enabled: f.Enabled, SortOrder: f.SortOrder,
	}
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
	data := adminPanelPageData{
		authPageData:   a.baseData(r),
		UserCount:      len(users),
		PremiumCount:   premiumCount,
		PremiumEnabled: a.Cfg.PremiumEnabled,
		HasOFF:         a.OFFSyncer != nil,
	}
	if a.Updates != nil {
		st, err := a.Updates.Status(r.Context())
		if err != nil {
			a.serverError(w, r, err)
			return
		}
		data.Updates = a.updateView(r, st)
	}
	for _, u := range users {
		if u.Role == "coach" {
			data.CoachCount++
		}
	}
	for _, f := range features {
		data.Features = append(data.Features, newFeatureRow(i18n.FromContext(r.Context()), f))
		if f.Enabled {
			data.ActiveCount++
			if f.IsPremium {
				data.PremiumFeats++
			}
		}
	}
	if err := a.Render.Render(w, r, http.StatusOK, "admin_panel", data); err != nil {
		a.serverError(w, r, err)
	}
}

// handleAdminFeatures GET /admin/panel/features: the features now live in the
// Overview; the old address redirects there.
func (a *App) handleAdminFeatures(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/admin/panel#funzionalita", http.StatusFound)
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
	http.Redirect(w, r, "/admin/panel#funzionalita", http.StatusSeeOther)
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
	http.Redirect(w, r, "/admin/panel#funzionalita", http.StatusSeeOther)
}

// ---- /admin/panel/users -----------------------------------------------------

type adminUsersPageData struct {
	authPageData
	Users          []*adminUserRow
	PremiumUsers   int // for the filters
	CoachUsers     int
	PremiumEnabled bool
	Now            time.Time
}

type adminUserRow struct {
	ID          int64
	Email       string
	DisplayName string
	CreatedAt   time.Time
	Role        string // user | coach
	IsOwner     bool
	Premium     bool       // active, unexpired subscription
	SubSource   string     // manual | license | payment
	SubExpires  *time.Time // nil = no expiry
	ExpiresSoon bool       // expires within 30 days
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
	var premiumUsers, coachUsers int
	for i, u := range users {
		row := &adminUserRow{
			ID:          u.ID,
			Email:       u.Email,
			DisplayName: u.DisplayName,
			CreatedAt:   u.CreatedAt,
			Role:        u.Role,
			IsOwner:     a.isOwner(u),
		}
		if sub, ok := subs[u.ID]; ok && sub.IsActive(now) {
			row.Premium = true
			row.SubSource = sub.Source
			if sub.ExpiresAt != nil {
				row.SubExpires = sub.ExpiresAt
				row.ExpiresSoon = sub.ExpiresAt.Before(now.AddDate(0, 0, 30))
			}
		}
		rows[i] = row
		if row.Premium {
			premiumUsers++
		}
		if row.Role == "coach" {
			coachUsers++
		}
	}
	data := adminUsersPageData{
		authPageData:   a.baseData(r),
		Users:          rows,
		PremiumUsers:   premiumUsers,
		CoachUsers:     coachUsers,
		PremiumEnabled: a.Cfg.PremiumEnabled,
		Now:            now,
	}
	if err := a.Render.Render(w, r, http.StatusOK, "admin_users", data); err != nil {
		a.serverError(w, r, err)
	}
}

// handleAdminUserGrantPremium POST /admin/panel/users/{id}/premium — grants
// (or reactivates) the premium subscription. Optional expires_at field (date).
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
			a.Sessions.Put(r.Context(), "flash", "admin.flash.bad_expiry")
			http.Redirect(w, r, "/admin/panel/users", http.StatusSeeOther)
			return
		}
		// end of day: the expiry includes the given day
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
// revokes the premium subscription (the row stays for auditing).
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

// handleAdminUserRole POST /admin/panel/users/{id}/role — changes the user's
// role ('user' | 'coach').
func (a *App) handleAdminUserRole(w http.ResponseWriter, r *http.Request) {
	userID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	role := r.PostFormValue("role")
	if err := a.Users.UpdateRole(r.Context(), userID, role); err != nil {
		a.Sessions.Put(r.Context(), "flash", "admin.flash.bad_role")
		http.Redirect(w, r, "/admin/panel/users", http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/admin/panel/users", http.StatusSeeOther)
}

// ---- /upgrade ---------------------------------------------------------------

type upgradePageData struct {
	authPageData
	PremiumFeatures []featureRow
	BlockedName     string // the feature the user came from (?da=key)
	HasPremium      bool
	PremiumEnabled  bool
}

// handleUpgradePage GET /upgrade — an information page about the premium features.
// Where requireFeature redirects when access is denied.
func (a *App) handleUpgradePage(w http.ResponseWriter, r *http.Request) {
	features, err := a.Features.List(r.Context())
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	var premium []featureRow
	var blocked string
	from := r.URL.Query().Get("da")
	lang := i18n.FromContext(r.Context())
	for _, f := range features {
		row := newFeatureRow(lang, f)
		if f.Key == from && f.IsPremium {
			blocked = row.Name
		}
		if f.IsPremium && f.Enabled {
			premium = append(premium, row)
		}
	}
	hasPremium := false
	if u := userFromContext(r.Context()); u != nil {
		hasPremium, _ = a.Subscriptions.IsActive(r.Context(), u.ID)
	}
	data := upgradePageData{
		authPageData:    a.baseData(r),
		PremiumFeatures: premium,
		BlockedName:     blocked,
		HasPremium:      hasPremium,
		PremiumEnabled:  a.Cfg.PremiumEnabled,
	}
	if err := a.Render.Render(w, r, http.StatusOK, "upgrade", data); err != nil {
		a.serverError(w, r, err)
	}
}
