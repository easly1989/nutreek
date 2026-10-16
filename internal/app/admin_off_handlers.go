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
// the local OFF dump (last_synced_at, products_count, any error).
// Useful for the admin panel and for operational checks with curl.
func (a *App) handleAdminOFFStatus(w http.ResponseWriter, r *http.Request) {
	if a.OFFSyncer == nil {
		http.Error(w, "OFF syncer non configurato", http.StatusServiceUnavailable)
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

// handleAdminOFFRefresh POST /admin/refresh-off-dump — starts a manual sync
// of the OFF dump in a separate goroutine (it does not block the response).
// Returns 202 + the current state, or 409 when a sync is already running.
//
// The real sync takes ~10 minutes (download + parse + insert): running it
// synchronously would keep the connection open too long. The caller can then
// poll /admin/off-status to see progress (products_count updates at the end
// of the sync, not while streaming — acceptable for an admin use case).
func (a *App) handleAdminOFFRefresh(w http.ResponseWriter, r *http.Request) {
	if a.OFFSyncer == nil {
		http.Error(w, "OFF syncer non configurato", http.StatusServiceUnavailable)
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
		stats, err := a.OFFSyncer.SyncDump(ctx)
		if err != nil && !errors.Is(err, off.ErrSyncInProgress) {
			a.Logger.Warn("admin OFF sync failed", "err", err)
			return
		}
		a.Logger.Info("admin OFF sync done",
			"imported", stats.Imported, "skipped", stats.Skipped,
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
		"dump_etag":      s.DumpETag,
	}
	if s.LastSyncedAt != nil {
		out["last_synced_at"] = s.LastSyncedAt.UTC().Format(time.RFC3339)
	}
	if s.LastAttemptAt != nil {
		out["last_attempt_at"] = s.LastAttemptAt.UTC().Format(time.RFC3339)
	}
	return out
}
