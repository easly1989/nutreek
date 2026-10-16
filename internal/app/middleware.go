package app

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/justinas/nosurf"

	"github.com/amon2126/nutreek/internal/config"
	"github.com/amon2126/nutreek/internal/db"
)

// csrfMiddleware installs CSRF protection via nosurf. It uses a
// Secure+HttpOnly+SameSite=Lax cookie. In dev the Secure flag is off so
// browsers accept the cookie without HTTPS too.
func (a *App) csrfMiddleware(next http.Handler) http.Handler {
	csrf := nosurf.New(next)
	csrf.SetBaseCookie(http.Cookie{
		Path:     "/",
		HttpOnly: true,
		Secure:   a.Cfg.Env == config.EnvProd,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int((12 * time.Hour).Seconds()),
	})
	// By default nosurf assumes every request is TLS: in dev (plain HTTP)
	// that makes the same-origin check on the Referer fail. Here we delegate
	// the detection to r.TLS and X-Forwarded-Proto, which is what a reverse
	// proxy (Caddy/Traefik) in front of the container sets in prod.
	csrf.SetIsTLSFunc(func(r *http.Request) bool {
		if r.TLS != nil {
			return true
		}
		return r.Header.Get("X-Forwarded-Proto") == "https"
	})
	csrf.SetFailureHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		a.Logger.Warn("csrf failure",
			"path", r.URL.Path,
			"method", r.Method,
			"reason", nosurf.Reason(r),
		)
		http.Error(w, "CSRF verification failed", http.StatusForbidden)
	}))
	// Admin and API endpoints are machine-to-machine: exempt from CSRF.
	csrf.ExemptGlob("/admin/*")
	csrf.ExemptGlob("/api/*")
	return csrf
}

// loadUserMiddleware reads user_id from the session and, when present, loads
// the user from the DB and injects it into the context. When the referenced
// user has been deleted, it silently clears the session.
func (a *App) loadUserMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		uid := a.Sessions.GetInt64(r.Context(), sessionKeyUserID)
		if uid == 0 {
			next.ServeHTTP(w, r)
			return
		}
		user, err := a.Users.GetByID(r.Context(), uid)
		if err != nil {
			if errors.Is(err, db.ErrUserNotFound) {
				a.Sessions.Remove(r.Context(), sessionKeyUserID)
			} else {
				a.Logger.Error("load user from session", "err", err, "user_id", uid)
			}
			next.ServeHTTP(w, r)
			return
		}
		next.ServeHTTP(w, r.WithContext(contextWithUser(r.Context(), user)))
	})
}

// requireAuth redirects to /login when there is no user in the context.
func (a *App) requireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if userFromContext(r.Context()) == nil {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// requestLogger emits one slog line per request with method, path, status
// and duration. Deliberately minimal: an upgrade to a richer middleware
// (trace id, remote addr) comes in later stages.
func (a *App) requestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(sw, r)
		a.Logger.Info("http",
			"method", r.Method,
			"path", r.URL.Path,
			"status", sw.status,
			"bytes", sw.bytes,
			"duration_ms", time.Since(start).Milliseconds(),
		)
	})
}

type statusWriter struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (s *statusWriter) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}

func (s *statusWriter) Write(b []byte) (int, error) {
	n, err := s.ResponseWriter.Write(b)
	s.bytes += n
	return n, err
}

// requireAPIToken authenticates API requests via a Bearer token.
// It injects the user into the context; answers 401 JSON when missing or invalid.
func (a *App) requireAPIToken(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		plain, ok := strings.CutPrefix(authHeader, "Bearer ")
		if !ok || plain == "" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "token mancante"})
			return
		}
		user, err := a.APITokens.Authenticate(r.Context(), plain)
		if errors.Is(err, db.ErrAPITokenNotFound) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "token non valido"})
			return
		}
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "errore interno"})
			return
		}
		next.ServeHTTP(w, r.WithContext(contextWithUser(r.Context(), user)))
	})
}
