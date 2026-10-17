package app

import (
	"crypto/subtle"
	"encoding/json"
	"net/http"
	"strings"
)

// requireAdmin protects the admin endpoints with a token in the
// `X-Admin-Token` header, compared (constant time) with cfg.AdminToken. When
// the configured token is empty, the endpoint stays disabled (404).
func (a *App) requireAdmin(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		expected := strings.TrimSpace(a.Cfg.AdminToken)
		if expected == "" {
			http.NotFound(w, r)
			return
		}
		got := strings.TrimSpace(r.Header.Get("X-Admin-Token"))
		if got == "" || subtle.ConstantTimeCompare([]byte(got), []byte(expected)) != 1 {
			http.Error(w, "invalid admin token", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	}
}

// handleAdminReconcile runs a manual RunOnce of the reconciler and returns
// the stats as JSON. Useful for smoke tests and to force reconciliation
// without waiting for the ticker.
func (a *App) handleAdminReconcile(w http.ResponseWriter, r *http.Request) {
	if a.Reconciler == nil {
		http.Error(w, "reconciler not configured", http.StatusServiceUnavailable)
		return
	}
	stats, err := a.Reconciler.RunOnce(r.Context())
	if err != nil {
		a.Logger.Error("admin reconcile failed", "err", err)
		http.Error(w, "reconcile failed: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]int{
		"considered": stats.Considered,
		"matched":    stats.Matched,
		"no_match":   stats.NoMatch,
		"errors":     stats.Errors,
	})
}
