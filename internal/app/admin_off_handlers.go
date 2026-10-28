package app

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/amon2126/nutreek/internal/off"
)

// handleAdminOFFStatus GET /admin/off-status — returns the current state of
// the local OFF copy (last sync, bootstrap and delta, products_count, any error).
// Useful for the admin panel and for operational checks with curl.
func (a *App) handleAdminOFFStatus(w http.ResponseWriter, r *http.Request) {
	if a.OFFSyncer == nil {
		http.Error(w, "OFF syncer not configured", http.StatusServiceUnavailable)
		return
	}
	state, err := a.OFFSyncer.LoadState(r.Context())
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(offStatusJSON(state))
}

// handleAdminOFFRefresh POST /admin/refresh-off-dump — starts a CSV bootstrap
// of the OFF data in a separate goroutine (it does not block the response).
// Returns 202 + the current state, or 409 when a sync is already running.
//
// A bootstrap takes ~10 minutes (download + parse + insert): running it
// synchronously would keep the connection open too long. The caller can then
// poll /admin/off-status to see progress (products_count updates at the end
// of the sync, not while streaming — acceptable for an admin use case).
func (a *App) handleAdminOFFRefresh(w http.ResponseWriter, r *http.Request) {
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

	// Start the sync with a context independent of the request: the response
	// goes out at once, the sync continues in the background. Generous timeout.
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Hour)
		defer cancel()
		stats, err := a.OFFSyncer.Bootstrap(ctx)
		if err != nil && !errors.Is(err, off.ErrSyncInProgress) {
			a.Logger.Warn("admin OFF sync failed", "err", err)
			return
		}
		a.Logger.Info("admin OFF sync done",
			"imported", stats.Imported, "deltas", stats.Deltas,
			"duration_ms", stats.DurationMs)
	}()

	state, _ := a.OFFSyncer.LoadState(r.Context())
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(offStatusJSON(state))
}

// offStatusJSON serializes off.State for the admin responses.
func offStatusJSON(s *off.State) map[string]any {
	if s == nil {
		return map[string]any{}
	}
	out := map[string]any{
		"products_count": s.ProductsCount,
		"syncing":        s.Syncing,
		"last_error":     s.LastError,
	}
	for key, t := range map[string]*time.Time{
		"last_synced_at":    s.LastSyncedAt,
		"last_attempt_at":   s.LastAttemptAt,
		"last_bootstrap_at": s.LastBootstrapAt,
		"last_delta_at":     s.LastDeltaAt,
		"high_water_at":     s.HighWater,
	} {
		if t != nil {
			out[key] = t.UTC().Format(time.RFC3339)
		}
	}
	return out
}
