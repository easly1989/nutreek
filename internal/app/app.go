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
	"github.com/amon2126/nutreek/internal/updates"
	"github.com/amon2126/nutreek/internal/web"
)

// App holds the dependencies shared by handlers and middleware.
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
	// Updates tells the owner a newer Nutreek is out (#167); nil in tests and
	// in the demo build.
	Updates *updates.Service

	// Demo is set only in the demo build (#137, cmd/nutreek/demo_on.go).
	Demo       Demo
	DemoLimits DemoLimits

	// loginLimiter limits POST /login attempts per IP (5/minute).
	// Created once by loginRateLimiter(): App is built as a struct literal,
	// so there is no constructor to initialize it in.
	loginLimiter     *ipRateLimiter
	loginLimiterOnce sync.Once
}

// loginRateLimiter returns the login limiter, creating it on first use in a
// way that is safe under concurrent requests.
func (a *App) loginRateLimiter() *ipRateLimiter {
	a.loginLimiterOnce.Do(func() {
		a.loginLimiter = newIPRateLimiter(5, time.Minute)
	})
	return a.loginLimiter
}

// Demo is the public demo: it creates a throwaway group of accounts in a
// language, the visitor playing as "plan", "coach" or "client", and deletes it
// after IdleHours without a request.
type Demo interface {
	Start(ctx context.Context, lang, as string) (userID int64, err error)
	IdleHours() int                               // how long a demo lasts without use, for the notices
	Seen(ctx context.Context, userID int64) error // a signed-in user made a request
	// Candidates are whom the coach may add as clients: the people of their
	// group without a coach (#186).
	Candidates(ctx context.Context, coachID int64) ([]domain.User, error)
	// Group is userID's demo group, the visitor first (#187).
	Group(ctx context.Context, userID int64) ([]domain.DemoMember, error)
}

// DemoLimits keep one demo visitor from hurting the server or the others
// (#185). Zero means no limit.
type DemoLimits struct {
	MaxUsers       int   // live demo users; then the entry says the demo is full
	StartsPerHour  int   // demos started per address per hour; then 429
	MaxBody        int64 // request body in bytes; then 413
	MaxIngredients int   // hand-made ingredients per user
	MaxRecipes     int   // recipes per user
}

// signInPath is where a signed-out visitor goes: the demo has no sign-in.
func (a *App) signInPath() string {
	if a.Demo != nil {
		return "/"
	}
	return "/login"
}

type ctxKey int

const (
	ctxKeyUser ctxKey = iota
)

// userFromContext extracts the authenticated user from the context, or returns nil.
func userFromContext(ctx context.Context) *domain.User {
	u, _ := ctx.Value(ctxKeyUser).(*domain.User)
	return u
}

// viewerID is the signed-in user's id, 0 for none.
func viewerID(ctx context.Context) int64 {
	if u := userFromContext(ctx); u != nil {
		return u.ID
	}
	return 0
}

func contextWithUser(ctx context.Context, u *domain.User) context.Context {
	return context.WithValue(ctx, ctxKeyUser, u)
}
