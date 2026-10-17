package app

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/amon2126/nutreek/internal/db"
)

func (a *App) handleCategoryCreate(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	name := strings.TrimSpace(r.PostFormValue("name"))
	if name == "" || len(name) > 100 {
		a.Sessions.Put(r.Context(), "flash", "ingredients.flash.category_invalid")
		http.Redirect(w, r, "/ingredients", http.StatusSeeOther)
		return
	}
	if _, err := a.Categories.CreateCategory(r.Context(), user.ID, name); err != nil {
		if !errors.Is(err, db.ErrCategoryExists) {
			a.serverError(w, r, err)
			return
		}
		a.Sessions.Put(r.Context(), "flash", "ingredients.flash.category_exists")
		http.Redirect(w, r, "/ingredients", http.StatusSeeOther)
		return
	}
	a.Sessions.Put(r.Context(), "flash", "ingredients.flash.category_created")
	http.Redirect(w, r, "/ingredients", http.StatusSeeOther)
}

func (a *App) handleCategoryDelete(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := a.Categories.DeleteCategory(r.Context(), id, user.ID); err != nil {
		if errors.Is(err, db.ErrCategoryNotFound) {
			http.NotFound(w, r)
			return
		}
		a.serverError(w, r, err)
		return
	}
	a.Sessions.Put(r.Context(), "flash", "ingredients.flash.category_deleted")
	http.Redirect(w, r, "/ingredients", http.StatusSeeOther)
}
