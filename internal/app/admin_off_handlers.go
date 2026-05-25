package app

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/amon2126/nutreek/internal/off"
)

// handleAdminOFFStatus GET /admin/off-status — ritorna stato corrente del
// dump OFF locale (last_synced_at, products_count, eventuale errore).
// Utile per pannello admin e per verifiche operative via curl.
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

// handleAdminOFFRefresh POST /admin/refresh-off-dump — lancia un sync
// manuale del dump OFF in goroutine separata (non blocca la response).
// Ritorna 202 + stato corrente, oppure 409 se un sync e' gia' in corso.
//
// Il sync vero dura ~10 minuti (download + parse + insert): farlo sincrono
// terrebbe la connessione aperta troppo a lungo. Il chiamante puo' poi
// pollare /admin/off-status per vedere il progresso (products_count si
// aggiorna a fine sync, non in streaming — accettabile per use case admin).
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

	// Lancia il sync con un context indipendente dalla request: la response
	// va via subito, il sync continua in background. Timeout generoso.
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

// offStatusJSON serializza off.State per le response admin.
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
