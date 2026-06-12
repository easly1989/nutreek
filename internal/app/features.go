package app

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/amon2126/nutreek/internal/domain"
)

// featureCacheEntry memorizza una snapshot della lista feature con timestamp.
type featureCacheEntry struct {
	features []domain.Feature
	at       time.Time
}

const featureCacheTTL = 60 * time.Second

var (
	featureCacheMu sync.RWMutex
	featureCache   *featureCacheEntry
)

// featuresFromCache restituisce la lista feature dalla cache (se ancora valida)
// oppure la ricarica dal DB.
func (a *App) featuresFromCache(ctx context.Context) ([]domain.Feature, error) {
	featureCacheMu.RLock()
	if featureCache != nil && time.Since(featureCache.at) < featureCacheTTL {
		f := featureCache.features
		featureCacheMu.RUnlock()
		return f, nil
	}
	featureCacheMu.RUnlock()

	featureCacheMu.Lock()
	defer featureCacheMu.Unlock()
	// double-check dopo aver acquisito il write lock
	if featureCache != nil && time.Since(featureCache.at) < featureCacheTTL {
		return featureCache.features, nil
	}
	features, err := a.Features.List(ctx)
	if err != nil {
		return nil, err
	}
	featureCache = &featureCacheEntry{features: features, at: time.Now()}
	return features, nil
}

// invalidateFeatureCache svuota la cache in modo che la prossima chiamata
// ricarichi dal DB. Va chiamata dopo ogni toggle enabled/premium.
func invalidateFeatureCache() {
	featureCacheMu.Lock()
	featureCache = nil
	featureCacheMu.Unlock()
}

// HasFeature verifica se l'utente ha accesso alla feature richiesta.
//   - Se PremiumEnabled=false: sempre true (tutto è accessibile).
//   - Se PremiumEnabled=true:  l'owner ha sempre tutto; le feature disabilitate
//     (kill-switch) sono false per tutti; le feature non-premium sono true;
//     le feature premium richiedono una sottoscrizione attiva.
func (a *App) HasFeature(ctx context.Context, u *domain.User, key string) bool {
	if !a.Cfg.PremiumEnabled {
		return true
	}
	if a.isOwner(u) {
		return true
	}
	features, err := a.featuresFromCache(ctx)
	if err != nil {
		return true // fallback permissivo in caso di errore DB
	}
	for _, f := range features {
		if f.Key == key {
			if !f.Enabled {
				return false
			}
			if !f.IsPremium {
				return true
			}
			if u == nil {
				return false
			}
			active, err := a.Subscriptions.IsActive(ctx, u.ID)
			if err != nil {
				return true // fallback permissivo in caso di errore DB
			}
			return active
		}
	}
	return true // feature sconosciuta = non bloccata
}

// requireFeature è un middleware chi blocca l'accesso se l'utente non ha la
// feature. Le richieste browser vengono redirette alla pagina /upgrade,
// le richieste API/HTMX ricevono 403.
func (a *App) requireFeature(key string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			u := userFromContext(r.Context())
			if !a.HasFeature(r.Context(), u, key) {
				if r.Header.Get("HX-Request") == "true" {
					// HTMX: redirect client-side alla pagina upgrade.
					w.Header().Set("HX-Redirect", "/upgrade")
					w.WriteHeader(http.StatusForbidden)
					return
				}
				if strings.Contains(r.Header.Get("Accept"), "text/html") {
					http.Redirect(w, r, "/upgrade", http.StatusSeeOther)
					return
				}
				http.Error(w, "funzionalità non disponibile nel tuo piano", http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
