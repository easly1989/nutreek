package app

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/amon2126/nutreek/internal/domain"
)

// featureCacheEntry stores a snapshot of the feature list with a timestamp.
type featureCacheEntry struct {
	features []domain.Feature
	at       time.Time
}

const featureCacheTTL = 60 * time.Second

var (
	featureCacheMu sync.RWMutex
	featureCache   *featureCacheEntry
)

// featuresFromCache returns the feature list from the cache (when still valid)
// or reloads it from the DB.
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
	// double-check after taking the write lock
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

// invalidateFeatureCache empties the cache so the next call reloads from the
// DB. Call it after every enabled/premium toggle.
func invalidateFeatureCache() {
	featureCacheMu.Lock()
	featureCache = nil
	featureCacheMu.Unlock()
}

// HasFeature reports whether the user has access to the requested feature.
//   - With PremiumEnabled=false: always true (everything is available).
//   - With PremiumEnabled=true: the owner always has everything; disabled
//     features (kill switch) are false for everyone; non-premium features are
//     true; premium features need an active subscription.
func (a *App) HasFeature(ctx context.Context, u *domain.User, key string) bool {
	if !a.Cfg.PremiumEnabled {
		return true
	}
	if a.isOwner(u) {
		return true
	}
	features, err := a.featuresFromCache(ctx)
	if err != nil {
		return true // permissive fallback on a DB error
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
				return true // permissive fallback on a DB error
			}
			return active
		}
	}
	return true // unknown feature = not blocked
}

// requireFeature is a chi middleware that blocks access when the user lacks
// the feature. Browser requests are redirected to the /upgrade page, API/HTMX
// requests get 403.
func (a *App) requireFeature(key string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			u := userFromContext(r.Context())
			if !a.HasFeature(r.Context(), u, key) {
				if r.Header.Get("HX-Request") == "true" {
					// HTMX: client-side redirect to the upgrade page.
					w.Header().Set("HX-Redirect", "/upgrade?da="+url.QueryEscape(key))
					w.WriteHeader(http.StatusForbidden)
					return
				}
				if strings.Contains(r.Header.Get("Accept"), "text/html") {
					http.Redirect(w, r, "/upgrade?da="+url.QueryEscape(key), http.StatusSeeOther)
					return
				}
				// API/JSON: a structured response.
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusForbidden)
				_ = json.NewEncoder(w).Encode(map[string]string{
					"error": "feature not available on your plan",
				})
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
