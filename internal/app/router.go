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
		r.Post("/recipes/bulk-delete", a.handleRecipesBulkDelete)
		r.Get("/recipes/{id}", a.handleRecipeEdit)
		r.Get("/recipes/{id}/picker", a.handleRecipePicker)
		r.Get("/recipes/{id}/duplicate", a.handleRecipeDuplicateForm)
		r.Post("/recipes/{id}/duplicate", a.handleRecipeDuplicate)
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
		r.Post("/week/reset", a.handleResetWeek)
		r.Get("/days/{date}", a.handleDayCard) // partial HTMX per una singola card
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

		// Sgarri (meal_cheat_items).
		r.Post("/meals/{mealID}/cheat/items", a.handleAddMealCheat)
		r.Post("/meals/{mealID}/cheat/clear", a.handleClearMealCheats)
		r.Post("/meal-cheat-items/{cheatID}/delete", a.handleRemoveMealCheat)

		// Lista della spesa.
		// Profilo utente.
		r.Get("/profile", a.handleProfilePage)
		r.Post("/profile", a.handleProfileSave)

		// Owner-only: sync OFF manuale dal pannello profilo (vedi isOwner).
		// Non in /admin/* perche' va via sessione, non X-Admin-Token.
		r.Get("/profile/off-status", a.requireOwner(a.handleOwnerOFFStatus))
		r.Post("/profile/off-sync", a.requireOwner(a.handleOwnerOFFRefresh))

		r.Get("/shopping-list", a.handleShoppingList)
		r.Post("/shopping-list/generate", a.handleShoppingGenerate)
		r.Post("/shopping-list/items/{id}/toggle", a.handleShoppingToggle)
		r.Post("/shopping-list/{id}/user-items", a.handleAddShoppingUserItem)
		r.Post("/shopping-list/user-items/{id}/toggle", a.handleToggleShoppingUserItem)
		r.Post("/shopping-list/user-items/{id}/delete", a.handleRemoveShoppingUserItem)
	})

	// Admin: protetto da X-Admin-Token (vedi requireAdmin). Esente CSRF
	// (esclusione configurata in csrfMiddleware via ExemptGlob).
	r.Post("/admin/reconcile-ingredients", a.requireAdmin(a.handleAdminReconcile))
	r.Post("/admin/refresh-off-dump", a.requireAdmin(a.handleAdminOFFRefresh))
	r.Get("/admin/off-status", a.requireAdmin(a.handleAdminOFFStatus))

	return r
}
