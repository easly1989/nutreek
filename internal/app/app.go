package app

import (
	"context"
	"log/slog"

	"github.com/alexedwards/scs/v2"

	"github.com/amon2126/nutreek/internal/config"
	"github.com/amon2126/nutreek/internal/db"
	"github.com/amon2126/nutreek/internal/domain"
	"github.com/amon2126/nutreek/internal/web"
)

// App raccoglie le dipendenze condivise fra handler e middleware.
type App struct {
	Cfg      *config.Config
	Store    *db.Store
	Users    *db.Users
	Sessions *scs.SessionManager
	Render   *web.Renderer
	Logger   *slog.Logger
}

type ctxKey int

const (
	ctxKeyUser ctxKey = iota
)

// userFromContext estrae l'utente autenticato dal context o ritorna nil.
func userFromContext(ctx context.Context) *domain.User {
	u, _ := ctx.Value(ctxKeyUser).(*domain.User)
	return u
}

func contextWithUser(ctx context.Context, u *domain.User) context.Context {
	return context.WithValue(ctx, ctxKeyUser, u)
}
