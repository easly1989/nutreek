package app

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/alexedwards/scs/v2"

	"github.com/amon2126/nutreek/internal/config"
	"github.com/amon2126/nutreek/internal/db"
	"github.com/amon2126/nutreek/internal/domain"
	"github.com/amon2126/nutreek/internal/off"
	"github.com/amon2126/nutreek/internal/service"
	"github.com/amon2126/nutreek/internal/web"
)

// App raccoglie le dipendenze condivise fra handler e middleware.
type App struct {
	Cfg                    *config.Config
	Store                  *db.Store
	Users                  *db.Users
	Ingredients            *db.Ingredients
	Recipes                *db.Recipes
	MealTemplates          *db.MealTemplates
	WeeklyPlans            *db.WeeklyPlans
	MealItemCustomizations *db.MealItemCustomizations
	ShoppingLists          *db.ShoppingLists
	WeightLogs             *db.WeightLogs
	WorkoutLogs            *db.WorkoutLogs
	WaterLogs              *db.WaterLogs
	Categories             *db.Categories
	NutritionistPlans      *db.NutritionistPlans
	Features               *db.Features
	Subscriptions          *db.Subscriptions
	Coach                  *db.CoachRepository
	APITokens              *db.APITokens
	TDEE                   *service.TDEEService
	Planner                *service.PlannerService
	OFF                    *off.Client
	OFFSyncer              *off.Syncer
	Reconciler             *off.Reconciler
	Sessions               *scs.SessionManager
	Render                 *web.Renderer
	Logger                 *slog.Logger

	// loginLimiter limita i tentativi POST /login per IP (5/minuto).
	// Creato una volta sola da loginRateLimiter(): l'App e' costruita come
	// struct literal, quindi niente costruttore in cui inizializzarlo.
	loginLimiter     *ipRateLimiter
	loginLimiterOnce sync.Once
}

// loginRateLimiter ritorna il limiter del login, creandolo al primo uso in
// modo sicuro rispetto alle richieste concorrenti.
func (a *App) loginRateLimiter() *ipRateLimiter {
	a.loginLimiterOnce.Do(func() {
		a.loginLimiter = newIPRateLimiter(5, time.Minute)
	})
	return a.loginLimiter
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
