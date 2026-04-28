package app

import (
	"crypto/subtle"
	"encoding/json"
	"net/http"
	"strings"
)

// requireAdmin protegge gli endpoint admin con un token in header
// `X-Admin-Token` confrontato (constant-time) con cfg.AdminToken. Se il
// token in config e' vuoto, l'endpoint resta disabilitato (404).
func (a *App) requireAdmin(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		expected := strings.TrimSpace(a.Cfg.AdminToken)
		if expected == "" {
			http.NotFound(w, r)
			return
		}
		got := strings.TrimSpace(r.Header.Get("X-Admin-Token"))
		if got == "" || subtle.ConstantTimeCompare([]byte(got), []byte(expected)) != 1 {
			http.Error(w, "admin token non valido", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	}
}

// handleAdminReconcile esegue una RunOnce manuale del reconciler e ritorna
// le statistiche come JSON. Utile per smoke test e per forzare la
// riconciliazione senza aspettare il ticker.
func (a *App) handleAdminReconcile(w http.ResponseWriter, r *http.Request) {
	if a.Reconciler == nil {
		http.Error(w, "reconciler non configurato", http.StatusServiceUnavailable)
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
