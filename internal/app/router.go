package app

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/amon2126/nutreek/internal/off"
	"github.com/amon2126/nutreek/internal/web"
)

const oneMinute = time.Minute

// securityHeaders sets the HTTP security headers on every response.
func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Frame-Options", "DENY")
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		// Camera only for the pickers' scanner (#59); 'wasm-unsafe-eval'
		// compiles the polyfill's WebAssembly reader, without opening eval().
		h.Set("Permissions-Policy", "camera=(self), microphone=()")
		h.Set("Content-Security-Policy",
			"default-src 'self'; "+
				"script-src 'self' 'unsafe-inline' 'wasm-unsafe-eval'; "+
				"style-src 'self' 'unsafe-inline'; "+
				// OFF product thumbnails (#133): only their CDN, see off.ThumbURL.
				"img-src 'self' data: https://"+off.ThumbHost+"; "+
				"font-src 'self'")
		next.ServeHTTP(w, r)
	})
}

// Router builds the chi.Mux with every handler mounted, in the right
// middleware order. The caller passes it as the server's http.Handler.
func (a *App) Router() http.Handler {
	r := chi.NewRouter()

	r.Use(middleware.Recoverer)
	if a.Demo != nil && a.DemoLimits.MaxBody > 0 {
		r.Use(limitBody(a.DemoLimits.MaxBody))
	}
	r.Use(middleware.StripSlashes)
	r.Use(securityHeaders)
	// A page left open across an update learns of it from its next HTMX request (#167).
	r.Use(middleware.SetHeader("X-Nutreek-Build", web.StaticVersion()))
	r.Use(requestIDMiddleware)
	r.Use(a.requestLogger)
	r.Use(a.Sessions.LoadAndSave)
	r.Use(a.csrfMiddleware)
	r.Use(a.loadUserMiddleware)
	r.Use(languageMiddleware) // after the user: their choice comes first

	// Styled 404 page for unmatched paths. chi calls this handler after
	// running the middleware chain above: the page sees the
	// session/csrf/user as usual.
	r.NotFound(a.notFound)

	// Static assets: served from the embedded filesystem.
	r.Handle("/static/*", http.StripPrefix("/static/", http.FileServer(http.FS(web.Static()))))

	// Installable app (#59): at the root, outside /static.
	r.Get("/manifest.webmanifest", a.handleManifest)
	r.Get("/sw.js", a.handleServiceWorker)

	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		if err := a.Store.DB.PingContext(r.Context()); err != nil {
			http.Error(w, "db down", http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte("ok"))
	})

	r.Get("/", a.handleDashboard)

	r.Post("/language", a.handleLanguage)
	if a.Demo != nil {
		// The demo's only way in (#137): no sign-in, no sign-up.
		start := http.Handler(http.HandlerFunc(a.handleDemoSession))
		if n := a.DemoLimits.StartsPerHour; n > 0 {
			start = newIPRateLimiter(n, time.Hour).Middleware(start)
		}
		r.Method(http.MethodPost, "/demo/session", start)
		r.With(a.requireAuth).Post("/demo/as", a.handleDemoAs)
	} else {
		registerLimiter := newIPRateLimiter(5, oneMinute)
		r.Get("/login", a.handleLoginForm)
		r.Post("/login", a.handleLogin)
		r.Get("/register", a.handleRegisterForm)
		r.With(registerLimiter.Middleware).Post("/register", a.handleRegister)
	}
	r.Post("/logout", a.handleLogout)

	searchLimiter := newIPRateLimiter(60, oneMinute)
	// In the demo, hand-made ingredients and recipes per user are capped (#185).
	ingredientCap := a.demoCap(a.DemoLimits.MaxIngredients, a.Ingredients.CountLocalBy, "demo.limit.ingredients", "/ingredients")
	recipeCap := a.demoCap(a.DemoLimits.MaxRecipes, a.Recipes.CountBy, "demo.limit.recipes", "/recipes")

	// Protected sections: ingredients / recipes / plan. All under requireAuth.
	r.Group(func(r chi.Router) {
		r.Use(a.requireAuth)
		r.Get("/ingredients", a.handleIngredientsList)
		r.With(searchLimiter.Middleware).Get("/ingredients/search", a.handleIngredientsSearchPartial)
		r.Get("/ingredients/new", a.handleIngredientNewForm)
		r.With(ingredientCap).Post("/ingredients", a.handleIngredientCreate)
		r.Get("/ingredients/{id}/edit", a.handleIngredientEditForm)
		r.Post("/ingredients/{id}", a.handleIngredientUpdate)
		r.Post("/ingredients/off-import", a.handleIngredientOFFImport)
		r.Post("/ingredients/categories", a.handleCategoryCreate)
		r.Post("/ingredients/categories/{id}/delete", a.handleCategoryDelete)

		// Quick-create modals (creating on the fly from pickers / lists).
		r.Get("/quick/ingredient", a.handleQuickIngredientForm)
		r.With(ingredientCap).Post("/quick/ingredient", a.handleQuickIngredientCreate)
		r.Get("/quick/recipe", a.handleQuickRecipeForm)
		r.With(recipeCap).Post("/quick/recipe", a.handleQuickRecipeCreate)

		r.Get("/recipes", a.handleRecipesList)
		r.Get("/recipes/new", a.handleRecipeNewForm)
		if a.Demo == nil { // fetches any URL: no use in the demo, and an SSRF risk
			r.Post("/recipes/import", a.handleRecipeImport)
		}
		r.With(recipeCap).Post("/recipes", a.handleRecipeCreate)
		r.Post("/recipes/bulk-delete", a.handleRecipesBulkDelete)
		r.Get("/recipes/{id}", a.handleRecipeEdit)
		r.Get("/recipes/{id}/picker", a.handleRecipePicker)
		r.Get("/recipes/{id}/duplicate", a.handleRecipeDuplicateForm)
		r.With(recipeCap).Post("/recipes/{id}/duplicate", a.handleRecipeDuplicate)
		r.Post("/recipes/{id}", a.handleRecipeUpdate)
		r.Post("/recipes/{id}/delete", a.handleRecipeDelete)
		r.Post("/recipes/{id}/items", a.handleRecipeAddItem)
		r.Post("/recipes/{id}/items/{itemID}/delete", a.handleRecipeRemoveItem)

		// Onboarding wizard (after sign-up and "Guida rapida" from the sidebar).
		r.Get("/onboarding", a.handleOnboardingPage)
		r.Post("/onboarding", a.handleOnboardingStep)

		// Guided tour (dismiss + restart).
		r.Post("/tour/dismiss", a.handleTourDismiss)
		r.Post("/tour/restart", a.handleTourRestart)

		// Nutrition plan (the unified nutritionist + template page).
		r.Get("/piano-nutrizionale", a.handlePianoNutrizionale)

		r.Get("/templates", a.handleTemplatesList)
		r.Get("/templates/new", a.handleTemplateNewForm)
		r.Post("/templates", a.handleTemplateCreate)
		r.Get("/templates/{id}", a.handleTemplateEdit)
		r.Post("/templates/{id}", a.handleTemplateRename)
		r.Post("/templates/{id}/activate", a.handleTemplateActivate)
		r.Post("/templates/{id}/delete", a.handleTemplateDelete)
		r.Post("/templates/{id}/slots", a.handleTemplateAddSlot)
		r.Post("/templates/{id}/slots/{slotID}/delete", a.handleTemplateRemoveSlot)
		r.Post("/templates/{id}/days/{day}/slots/reorder", a.handleReorderTemplateSlots)

		// Weekly plan (the dashboard is mounted on /).
		r.Post("/week/apply-template", a.handleApplyTemplate)
		// Automatic generation: a premium feature (gating active only with
		// NUTREEK_PREMIUM_ENABLED=true, see HasFeature).
		r.With(a.requireFeature("auto_plan")).Post("/week/generate", a.handleGeneratePlan)
		r.Post("/week/copy-previous", a.handleCopyPrevious)
		r.Post("/week/reset", a.handleResetWeek)
		r.Get("/days/{date}", a.handleDayCard) // HTMX partial for a single card
		r.Post("/meals", a.handleAddMeal)
		r.Post("/meals/reorder", a.handleReorderMeals)
		r.Post("/meals/{mealID}/delete", a.handleRemoveMeal)
		r.Get("/meals/{mealID}/items/new", a.handleAddItemForm)
		r.Get("/meals/{mealID}/picker", a.handleMealPicker)
		r.Post("/meals/{mealID}/items", a.handleAddMealItem)
		r.Post("/meals/{mealID}/items/reorder", a.handleReorderMealItems)
		r.Post("/meal-items/{itemID}/delete", a.handleRemoveMealItem)
		r.Get("/meal-items/{itemID}/customize", a.handleMealItemCustomizeForm)
		r.Post("/meal-items/{itemID}/customize", a.handleMealItemCustomizeSave)
		r.Post("/meal-items/{itemID}/customize/reset", a.handleMealItemCustomizeReset)

		// Cheats (meal_cheat_items).
		r.Post("/meals/{mealID}/cheat/items", a.handleAddMealCheat)
		r.Post("/meals/{mealID}/cheat/clear", a.handleClearMealCheats)
		r.Post("/meal-cheat-items/{cheatID}/delete", a.handleRemoveMealCheat)

		// Shopping list.
		// User profile.
		r.Get("/profile", a.handleProfilePage)
		r.Post("/profile", a.handleProfileSave)

		// Weight tracking.
		r.Get("/profile/peso", a.handleWeightPage)
		r.Post("/profile/peso", a.handleWeightLog)
		r.Post("/profile/peso/{id}/delete", a.handleWeightLogDelete)

		// Workout log (entered from the dashboard's day card).
		r.Post("/allenamento", a.handleWorkoutLog)
		r.Post("/allenamento/{id}/delete", a.handleWorkoutLogDelete)

		// Hydration tracking (quick log from the day card).
		r.Post("/water", a.handleWaterLog)
		r.Post("/water/delete-last", a.handleWaterDelete)

		// Statistics and adaptive TDEE (accepting the suggestion: premium).
		r.Get("/stats", a.handleStats)
		r.With(a.requireFeature("tdee_adaptive")).Post("/tdee/accept", a.handleAcceptTDEESuggestion)

		// Weekly plan export/import.
		r.Get("/week/{start}/export", a.handleWeekExport)
		r.Post("/week/{start}/import", a.handleWeekImport)

		// Nutrition plan.
		r.Get("/nutrizionista", a.handleNutritionistPage)
		r.Get("/nutrizionista/picker", a.handleNutritionistPicker)
		r.Post("/nutrizionista/meal-types", a.handleNutritionistCreateMealType)
		r.Post("/nutrizionista/meal-types/{id}/delete", a.handleNutritionistDeleteMealType)
		r.Post("/nutrizionista/variations", a.handleNutritionistCreateVariation)
		r.Post("/nutrizionista/variations/{id}/delete", a.handleNutritionistDeleteVariation)
		r.Post("/nutrizionista/items", a.handleNutritionistAddItem)
		r.Post("/nutrizionista/items/{id}/delete", a.handleNutritionistDeleteItem)

		// Owner-only: manual OFF sync from the profile panel (see isOwner).
		// Not under /admin/* because it goes through the session, not X-Admin-Token.
		// The demo has no owner.
		if a.Demo == nil {
			r.Get("/profile/off-status", a.requireOwner(a.handleOwnerOFFStatus))
			r.Post("/profile/off-sync", a.requireOwner(a.handleOwnerOFFRefresh))
		}

		r.Get("/shopping-list", a.handleShoppingList)
		r.Post("/shopping-list/generate", a.handleShoppingGenerate)
		r.Post("/shopping-list/items/{id}/toggle", a.handleShoppingToggle)
		r.Post("/shopping-list/{id}/user-items", a.handleAddShoppingUserItem)
		r.Post("/shopping-list/user-items/{id}/toggle", a.handleToggleShoppingUserItem)
		r.Post("/shopping-list/user-items/{id}/delete", a.handleRemoveShoppingUserItem)

		// Premium upgrade page (target of requireFeature's redirects).
		r.Get("/upgrade", a.handleUpgradePage)

		// Account settings (GDPR export, password change, deletion, API tokens).
		r.Get("/settings", a.handleSettingsPage)
		r.Get("/settings/export", a.handleUserDataExport)
		if a.Demo == nil { // both ask for the password a demo account does not have
			r.Post("/settings/password", a.handleChangePassword)
			r.Post("/settings/delete-account", a.handleDeleteAccount)
			r.With(a.requireFeature("api_access")).Post("/settings/api-tokens", a.handleSettingsAPITokenCreate)
			r.Post("/settings/api-tokens/{id}/revoke", a.handleSettingsAPITokenRevoke)
		}
	})

	if a.Demo == nil {
		a.mountAdminAndAPI(r)
	}

	// Coach mode: requires the 'coach' role (a premium feature, prepared).
	r.Group(func(r chi.Router) {
		r.Use(a.requireAuth)
		r.Use(a.requireCoach)
		r.Use(a.requireFeature("coach_mode"))
		r.Get("/coach", a.handleCoachDashboard)
		r.Post("/coach/clients", a.handleCoachAddClient)
		r.Post("/coach/clients/{id}/remove", a.handleCoachRemoveClient)
		r.Post("/coach/clients/{id}/assign-plan", a.handleCoachAssignPlan)
		r.Post("/coach/plans", a.handleCoachCreatePlan)
		r.Post("/coach/plans/{id}/open", a.handleCoachOpenPlan)
		r.Post("/coach/plans/{id}/rename", a.handleCoachRenamePlan)
		r.Post("/coach/plans/{id}/delete", a.handleCoachDeletePlan)
	})

	return r
}

// mountAdminAndAPI mounts the REST API and the admin routes, which the demo
// has no use for (#185).
func (a *App) mountAdminAndAPI(r chi.Router) {
	// REST API v1: authenticated via Bearer token, exempt from CSRF.
	// requireFeature("api_access") blocks users without a subscription when PremiumEnabled=true.
	r.Route("/api/v1", func(r chi.Router) {
		r.Use(a.requireAPIToken)
		r.Use(a.requireFeature("api_access"))
		r.Use(middleware.SetHeader("Content-Type", "application/json"))
		r.Get("/me", a.handleAPIMe)
		r.Get("/week", a.handleAPIWeek)
		r.Get("/ingredients", a.handleAPIIngredients)
		r.Get("/activities", a.handleAPIActivities)
		r.Post("/water", a.handleAPILogWater)
		r.Post("/weight", a.handleAPILogWeight)
		r.Post("/workout", a.handleAPILogWorkout)
	})

	adminLimiter := newIPRateLimiter(30, oneMinute)

	// Admin: protected by X-Admin-Token (see requireAdmin). Exempt from CSRF
	// (exclusion configured in csrfMiddleware via ExemptGlob).
	r.Group(func(r chi.Router) {
		r.Use(adminLimiter.Middleware)
		r.Post("/admin/reconcile-ingredients", a.requireAdmin(a.handleAdminReconcile))
		r.Post("/admin/refresh-off-dump", a.requireAdmin(a.handleAdminOFFRefresh))
		r.Get("/admin/off-status", a.requireAdmin(a.handleAdminOFFStatus))
	})

	// Owner admin panel: reached via the session, protected by requireOwnerMiddleware.
	r.Group(func(r chi.Router) {
		r.Use(a.requireAuth)
		r.Use(a.requireOwnerMiddleware)
		r.Get("/admin/panel", a.handleAdminPanel)
		r.Get("/admin/panel/features", a.handleAdminFeatures)
		r.Post("/admin/panel/features/{key}/toggle", a.handleAdminFeatureToggle)
		r.Post("/admin/panel/features/{key}/premium", a.handleAdminFeaturePremium)
		r.Get("/admin/panel/users", a.handleAdminUsers)
		r.Post("/admin/panel/users/{id}/premium", a.handleAdminUserGrantPremium)
		r.Post("/admin/panel/users/{id}/premium/revoke", a.handleAdminUserRevokePremium)
		r.Post("/admin/panel/users/{id}/role", a.handleAdminUserRole)
		if a.Updates != nil { // nil in the demo build (#167)
			r.Get("/admin/update", a.handleUpdateHowTo)
			r.Post("/admin/panel/updates/check", a.handleUpdateCheck)
			r.Post("/admin/panel/updates/toggle", a.handleUpdateToggle)
		}
	})
}

type reqIDKey struct{}

// requestIDMiddleware generates a random hex request ID (8 bytes = 16 chars)
// for every request, injects it into the context and adds it to the
// X-Request-ID response header to ease correlation in the logs.
func requestIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// The client's ID (e.g. from a reverse proxy) ends up in logs and the
		// response: accept it only when short and made of harmless characters.
		id := r.Header.Get("X-Request-ID")
		if !validRequestID(id) {
			b := make([]byte, 8)
			_, _ = rand.Read(b)
			id = hex.EncodeToString(b)
		}
		w.Header().Set("X-Request-ID", id)
		ctx := context.WithValue(r.Context(), reqIDKey{}, id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// validRequestID accepts 1–64 characters among ASCII letters, digits, '-' and
// '_' (covers UUIDs and the hex IDs of common proxies).
func validRequestID(id string) bool {
	if id == "" || len(id) > 64 {
		return false
	}
	for _, c := range id {
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9', c == '-', c == '_':
		default:
			return false
		}
	}
	return true
}
