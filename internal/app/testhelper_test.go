package app

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"path/filepath"
	"testing"

	"github.com/alexedwards/scs/v2"
	"golang.org/x/crypto/bcrypt"

	"github.com/amon2126/nutreek/internal/config"
	"github.com/amon2126/nutreek/internal/db"
	"github.com/amon2126/nutreek/internal/domain"
	"github.com/amon2126/nutreek/internal/service"
	"github.com/amon2126/nutreek/internal/web"
)

// newTestApp costruisce un'istanza App per i test con DB SQLite temporaneo.
func newTestApp(t *testing.T) *App {
	t.Helper()
	store, err := db.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := store.Migrate(context.Background()); err != nil {
		t.Fatalf("db.Migrate: %v", err)
	}

	sessions := scs.New() // store in-memory di default

	renderer, err := web.NewRenderer(web.Templates())
	if err != nil {
		t.Fatalf("web.NewRenderer: %v", err)
	}

	weeklyPlansRepo := db.NewWeeklyPlans(store.DB)
	recipesRepo := db.NewRecipes(store.DB)
	nutritionistRepo := db.NewNutritionistPlans(store.DB)
	coachRepo := db.NewCoachRepository(store.DB)
	usersRepo := db.NewUsers(store.DB)

	return &App{
		Cfg:                    &config.Config{},
		Store:                  store,
		Users:                  usersRepo,
		Ingredients:            db.NewIngredients(store.DB),
		Recipes:                recipesRepo,
		MealTemplates:          db.NewMealTemplates(store.DB),
		WeeklyPlans:            weeklyPlansRepo,
		MealItemCustomizations: db.NewMealItemCustomizations(store.DB),
		ShoppingLists:          db.NewShoppingLists(store.DB, weeklyPlansRepo),
		WeightLogs:             db.NewWeightLogs(store.DB),
		WorkoutLogs:            db.NewWorkoutLogs(store.DB),
		WaterLogs:              db.NewWaterLogs(store.DB),
		Categories:             db.NewCategories(store.DB),
		NutritionistPlans:      nutritionistRepo,
		Features:               db.NewFeatures(store.DB),
		Subscriptions:          db.NewSubscriptions(store.DB),
		Coach:                  coachRepo,
		APITokens:              db.NewAPITokens(store.DB, usersRepo),
		TDEE: &service.TDEEService{
			WeightLogs:  db.NewWeightLogs(store.DB),
			WeeklyPlans: weeklyPlansRepo,
		},
		Planner: &service.PlannerService{
			WeeklyPlans:       weeklyPlansRepo,
			NutritionistPlans: nutritionistRepo,
			Recipes:           recipesRepo,
			Coach:             coachRepo,
		},
		Sessions: sessions,
		Render:   renderer,
		Logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
}

// mustCreateTestUser crea un utente con hash bcrypt a costo minimo per la
// velocità dei test (bcrypt.MinCost = 4, vs costo produzione = 12).
func mustCreateTestUser(t *testing.T, a *App, email, password string) *domain.User {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("bcrypt: %v", err)
	}
	id, err := a.Users.Create(context.Background(), email, string(hash), "Utente Test")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	user, err := a.Users.GetByID(context.Background(), id)
	if err != nil {
		t.Fatalf("get user: %v", err)
	}
	return user
}

// requestWithUser restituisce una copia della richiesta con l'utente iniettato
// nel context. Usata per chiamare handler direttamente senza passare per il
// middleware loadUserMiddleware.
func requestWithUser(r *http.Request, u *domain.User) *http.Request {
	return r.WithContext(contextWithUser(r.Context(), u))
}
