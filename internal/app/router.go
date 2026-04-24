package app

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/amon2126/nutreek/internal/web"
)

// Router costruisce il chi.Mux con tutti gli handler montati, nel giusto
// ordine di middleware. Il chiamante lo passa come http.Handler del server.
func (a *App) Router() http.Handler {
	r := chi.NewRouter()

	r.Use(middleware.Recoverer)
	r.Use(middleware.StripSlashes)
	r.Use(a.requestLogger)
	r.Use(a.Sessions.LoadAndSave)
	r.Use(a.csrfMiddleware)
	r.Use(a.loadUserMiddleware)

	// Static assets: serviti dal filesystem embedded.
	r.Handle("/static/*", http.StripPrefix("/static/", http.FileServer(http.FS(web.Static()))))

	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		if err := a.Store.DB.PingContext(r.Context()); err != nil {
			http.Error(w, "db down", http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte("ok"))
	})

	r.Get("/", a.handleHome)

	r.Get("/login", a.handleLoginForm)
	r.Post("/login", a.handleLogin)
	r.Get("/register", a.handleRegisterForm)
	r.Post("/register", a.handleRegister)
	r.Post("/logout", a.handleLogout)

	return r
}
