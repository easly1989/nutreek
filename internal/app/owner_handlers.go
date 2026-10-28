package app

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/amon2126/nutreek/internal/domain"
	"github.com/amon2126/nutreek/internal/off"
)

// isOwner returns true when the user is the instance owner (NUTREEK_OWNER_EMAIL).
// Case-insensitive on the email. When NUTREEK_OWNER_EMAIL is not set, nobody
// is the owner and the owner-only features are disabled.
func (a *App) isOwner(u *domain.User) bool {
	if u == nil || a.Cfg.OwnerEmail == "" {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(u.Email), a.Cfg.OwnerEmail)
}

// requireOwner blocks anyone who is not the owner. It answers 404 (not 403)
// so as not to reveal the endpoint to ordinary users.
func (a *App) requireOwner(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !a.isOwner(userFromContext(r.Context())) {
			http.NotFound(w, r)
			return
		}
		next.ServeHTTP(w, r)
	}
}

// requireOwnerMiddleware is the middleware version of requireOwner.
func (a *App) requireOwnerMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !a.isOwner(userFromContext(r.Context())) {
			http.NotFound(w, r)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// handleOwnerOFFStatus GET /profile/off-status — the current state of the OFF sync.
// The owner's equivalent of /admin/off-status, reached via the session instead
// of X-Admin-Token. It also includes a diagnostic sample of the products in the DB.
func (a *App) handleOwnerOFFStatus(w http.ResponseWriter, r *http.Request) {
	if a.OFFSyncer == nil {
		http.Error(w, "OFF syncer not configured", http.StatusServiceUnavailable)
		return
	}
	state, err := a.OFFSyncer.LoadState(r.Context())
	if err != nil {
		a.serverError(w, r, err)
		return
	}

	out := offStatusJSON(state)

	// Diagnostic: count of products with/without complete nutrients.
	var full, partial int
	_ = a.Store.DB.QueryRowContext(r.Context(),
		`SELECT COUNT(*) FROM off_products WHERE has_full_nutrients = 1`).Scan(&full)
	_ = a.Store.DB.QueryRowContext(r.Context(),
		`SELECT COUNT(*) FROM off_products WHERE has_full_nutrients = 0`).Scan(&partial)
	out["diag_full_nutrients"] = full
	out["diag_partial_nutrients"] = partial

	// Sample: 3 products with complete nutrients (when any exist).
	type sampleRow struct {
		Barcode string  `json:"barcode"`
		Name    string  `json:"name"`
		Kcal    float64 `json:"kcal"`
		Protein float64 `json:"protein"`
		Carbs   float64 `json:"carbs"`
		Fat     float64 `json:"fat"`
	}
	var sample []sampleRow
	rows, _ := a.Store.DB.QueryContext(r.Context(), `
		SELECT barcode, product_name, kcal_per_100, protein_per_100, carbs_per_100, fat_per_100
		FROM off_products WHERE has_full_nutrients = 1 LIMIT 3`)
	if rows != nil {
		defer rows.Close()
		for rows.Next() {
			var s sampleRow
			_ = rows.Scan(&s.Barcode, &s.Name, &s.Kcal, &s.Protein, &s.Carbs, &s.Fat)
			sample = append(sample, s)
		}
	}
	out["diag_sample_full"] = sample

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(out)
}

// handleOwnerOFFRefresh POST /profile/off-sync — triggers an OFF sync in the
// background. Accepts `force=true` (form or query) to bootstrap from the CSV
// export even when the daily deltas would do.
func (a *App) handleOwnerOFFRefresh(w http.ResponseWriter, r *http.Request) {
	if a.OFFSyncer == nil {
		http.Error(w, "OFF syncer not configured", http.StatusServiceUnavailable)
		return
	}
	if a.OFFSyncer.IsSyncing() {
		state, _ := a.OFFSyncer.LoadState(r.Context())
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		_ = json.NewEncoder(w).Encode(offStatusJSON(state))
		return
	}

	force := r.URL.Query().Get("force") == "true" || r.PostFormValue("force") == "true"
	run := a.OFFSyncer.Sync
	if force {
		run = a.OFFSyncer.Bootstrap
	}

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Hour)
		defer cancel()
		stats, err := run(ctx)
		if err != nil && !errors.Is(err, off.ErrSyncInProgress) {
			a.Logger.Warn("owner OFF sync failed", "err", err)
			return
		}
		a.Logger.Info("owner OFF sync done",
			"imported", stats.Imported, "deltas", stats.Deltas,
			"duration_ms", stats.DurationMs, "force", force)
	}()

	state, _ := a.OFFSyncer.LoadState(r.Context())
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(offStatusJSON(state))
}
