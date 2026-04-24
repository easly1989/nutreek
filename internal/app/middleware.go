package app

import (
	"errors"
	"net/http"
	"time"

	"github.com/justinas/nosurf"

	"github.com/amon2126/nutreek/internal/config"
	"github.com/amon2126/nutreek/internal/db"
)

// csrfMiddleware installa la protezione CSRF via nosurf. Usa cookie
// Secure+HttpOnly+SameSite=Lax. In dev il flag Secure e' disattivato
// cosi' i browser accettano il cookie anche senza HTTPS.
func (a *App) csrfMiddleware(next http.Handler) http.Handler {
	csrf := nosurf.New(next)
	csrf.SetBaseCookie(http.Cookie{
		Path:     "/",
		HttpOnly: true,
		Secure:   a.Cfg.Env == config.EnvProd,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int((12 * time.Hour).Seconds()),
	})
	// Nosurf di default assume che ogni richiesta sia TLS: in dev (plain HTTP)
	// questo fa fallire il confronto same-origin sul Referer. Qui deleghiamo
	// la detection a r.TLS e a X-Forwarded-Proto, che e' quello che imposta
	// un reverse proxy (Caddy/Traefik) davanti al container in prod.
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
	return csrf
}

// loadUserMiddleware legge user_id dalla sessione e se presente carica
// l'utente dal DB, iniettandolo nel context. Se l'utente referenziato e'
// stato cancellato, ripulisce la sessione silenziosamente.
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

// requireAuth redirige a /login se non c'e' utente in context.
func (a *App) requireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if userFromContext(r.Context()) == nil {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// requestLogger emette una riga slog per ogni richiesta con metodo, path,
// status e durata. E' volutamente essenziale: upgrade a middleware piu'
// evoluto (trace id, remote addr) in tappe successive.
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
