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

// ownerEmail e' l'email dell'unico utente "owner" dell'istanza, abilitato a
// triggerare operazioni privilegiate dalla UI (es. sync OFF on-demand).
// Hardcoded perche' Nutreek e' un'app single-tenant self-hosted: nessun
// pannello admin separato, l'owner usa la stessa sessione di tutti.
const ownerEmail = "carlo@ruggiero.cc"

// isOwner ritorna true se l'utente in context e' l'owner dell'istanza.
// Case-insensitive sull'email per resistere a variazioni di registrazione.
func isOwner(u *domain.User) bool {
	if u == nil {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(u.Email), ownerEmail)
}

// requireOwner blocca chi non e' l'owner. Risponde 404 (non 403) per non
// rivelare l'esistenza dell'endpoint a utenti normali.
func (a *App) requireOwner(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !isOwner(userFromContext(r.Context())) {
			http.NotFound(w, r)
			return
		}
		next.ServeHTTP(w, r)
	}
}

// handleOwnerOFFStatus GET /profile/off-status — stato corrente del sync OFF.
// Equivalente owner di /admin/off-status, accessibile via sessione invece
// che X-Admin-Token. Include anche un campione diagnostico dei prodotti in DB.
func (a *App) handleOwnerOFFStatus(w http.ResponseWriter, r *http.Request) {
	if a.OFFSyncer == nil {
		http.Error(w, "OFF syncer non configurato", http.StatusServiceUnavailable)
		return
	}
	state, err := a.OFFSyncer.LoadState(r.Context())
	if err != nil {
		a.serverError(w, r, err)
		return
	}

	out := offStatusJSON(state)

	// Diagnostic: conteggio prodotti con/senza nutrienti completi.
	var full, partial int
	_ = a.Store.DB.QueryRowContext(r.Context(),
		`SELECT COUNT(*) FROM off_products WHERE has_full_nutrients = 1`).Scan(&full)
	_ = a.Store.DB.QueryRowContext(r.Context(),
		`SELECT COUNT(*) FROM off_products WHERE has_full_nutrients = 0`).Scan(&partial)
	out["diag_full_nutrients"] = full
	out["diag_partial_nutrients"] = partial

	// Campione: 3 prodotti con nutrienti completi (se esistono).
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

// handleOwnerOFFRefresh POST /profile/off-sync — triggera sync OFF in
// background. Accetta `force=true` (form o query) per invalidare l'ETag
// salvato e forzare re-download completo (utile dopo cambi al parser).
func (a *App) handleOwnerOFFRefresh(w http.ResponseWriter, r *http.Request) {
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

	force := r.URL.Query().Get("force") == "true" || r.PostFormValue("force") == "true"
	if force {
		// Azzerando dump_etag il prossimo doSync non manda If-None-Match,
		// quindi OFF risponde 200 + dump completo invece di 304.
		if _, err := a.Store.DB.ExecContext(r.Context(),
			`UPDATE off_sync_state SET dump_etag = NULL WHERE id = 1`); err != nil {
			a.serverError(w, r, err)
			return
		}
		a.Logger.Info("owner OFF sync: forzato (etag azzerato)")
	}

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Hour)
		defer cancel()
		stats, err := a.OFFSyncer.SyncDump(ctx)
		if err != nil && !errors.Is(err, off.ErrSyncInProgress) {
			a.Logger.Warn("owner OFF sync failed", "err", err)
			return
		}
		a.Logger.Info("owner OFF sync done",
			"imported", stats.Imported, "skipped", stats.Skipped,
			"duration_ms", stats.DurationMs, "force", force)
	}()

	state, _ := a.OFFSyncer.LoadState(r.Context())
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(offStatusJSON(state))
}
