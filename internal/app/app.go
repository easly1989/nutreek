package app

import (
	"context"
	"log/slog"

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
	Cfg           *config.Config
	Store         *db.Store
	Users         *db.Users
	Ingredients           *db.Ingredients
	Recipes               *db.Recipes
	MealTemplates         *db.MealTemplates
	WeeklyPlans           *db.WeeklyPlans
	MealItemCustomizations *db.MealItemCustomizations
	ShoppingLists *db.ShoppingLists
	WeightLogs    *db.WeightLogs
	WorkoutLogs   *db.WorkoutLogs
	WaterLogs     *db.WaterLogs
	Categories    *db.Categories
	NutritionistPlans *db.NutritionistPlans
	TDEE          *service.TDEEService
	OFF           *off.Client
	OFFSyncer     *off.Syncer
	Reconciler    *off.Reconciler
	Sessions      *scs.SessionManager
	Render        *web.Renderer
	Logger        *slog.Logger

	// loginLimiter limita i tentativi POST /login per IP. Inizializzato
	// pigramente al primo accesso dentro handleLogin (non serve in test).
	loginLimiter *loginRateLimiter
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
