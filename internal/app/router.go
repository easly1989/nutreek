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

	// Pagina 404 stylata per i path non matchati. chi chiama questo
	// handler dopo aver girato la chain dei middleware sopra: la pagina
	// vede sessione/csrf/utente normalmente.
	r.NotFound(a.notFound)

	// Static assets: serviti dal filesystem embedded.
	r.Handle("/static/*", http.StripPrefix("/static/", http.FileServer(http.FS(web.Static()))))

	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		if err := a.Store.DB.PingContext(r.Context()); err != nil {
			http.Error(w, "db down", http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte("ok"))
	})

	r.Get("/", a.handleDashboard)

	r.Get("/login", a.handleLoginForm)
	r.Post("/login", a.handleLogin)
	r.Get("/register", a.handleRegisterForm)
	r.Post("/register", a.handleRegister)
	r.Post("/logout", a.handleLogout)

	// Sezioni protette: ingredienti / ricette / piano. Tutte sotto requireAuth.
	r.Group(func(r chi.Router) {
		r.Use(a.requireAuth)
		r.Get("/ingredients", a.handleIngredientsList)
		r.Get("/ingredients/search", a.handleIngredientsSearchPartial)
		r.Get("/ingredients/new", a.handleIngredientNewForm)
		r.Post("/ingredients", a.handleIngredientCreate)
		r.Post("/ingredients/off-import", a.handleIngredientOFFImport)

		// Quick-create modali (creazione al volo da picker / liste).
		r.Get("/quick/ingredient", a.handleQuickIngredientForm)
		r.Post("/quick/ingredient", a.handleQuickIngredientCreate)
		r.Get("/quick/recipe", a.handleQuickRecipeForm)
		r.Post("/quick/recipe", a.handleQuickRecipeCreate)

		r.Get("/recipes", a.handleRecipesList)
		r.Get("/recipes/new", a.handleRecipeNewForm)
		r.Post("/recipes", a.handleRecipeCreate)
		r.Get("/recipes/{id}", a.handleRecipeEdit)
		r.Get("/recipes/{id}/picker", a.handleRecipePicker)
		r.Post("/recipes/{id}", a.handleRecipeUpdate)
		r.Post("/recipes/{id}/delete", a.handleRecipeDelete)
		r.Post("/recipes/{id}/items", a.handleRecipeAddItem)
		r.Post("/recipes/{id}/items/{itemID}/delete", a.handleRecipeRemoveItem)

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

		// Piano settimanale (la dashboard e' montata su /).
		r.Post("/week/apply-template", a.handleApplyTemplate)
		r.Post("/week/copy-previous", a.handleCopyPrevious)
		r.Get("/days/{date}", a.handleDayCard) // partial HTMX per una singola card
		r.Post("/meals", a.handleAddMeal)
		r.Post("/meals/reorder", a.handleReorderMeals)
		r.Post("/meals/{mealID}/delete", a.handleRemoveMeal)
		r.Get("/meals/{mealID}/items/new", a.handleAddItemForm)
		r.Get("/meals/{mealID}/picker", a.handleMealPicker)
		r.Post("/meals/{mealID}/items", a.handleAddMealItem)
		r.Post("/meals/{mealID}/items/reorder", a.handleReorderMealItems)
		r.Post("/meal-items/{itemID}/delete", a.handleRemoveMealItem)
	})

	// Admin: protetto da X-Admin-Token (vedi requireAdmin). Esente CSRF
	// (esclusione configurata in csrfMiddleware via ExemptGlob).
	r.Post("/admin/reconcile-ingredients", a.requireAdmin(a.handleAdminReconcile))

	return r
}
