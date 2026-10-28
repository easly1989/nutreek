package app

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/amon2126/nutreek/internal/db"
	"github.com/amon2126/nutreek/internal/photo"
)

// maxPhotoBytes caps an uploaded photo (#207).
const maxPhotoBytes = 10 << 20

// limitPhotoBody caps a photo upload before nosurf reads the whole multipart
// body looking for the CSRF token; the slack is the rest of the form.
func limitPhotoBody(next http.Handler) http.Handler {
	capped := limitBody(maxPhotoBytes + 64<<10)(next)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if ok, _ := path.Match("/recipes/*/photo", strings.TrimSuffix(r.URL.Path, "/")); ok && r.Method == http.MethodPost {
			capped.ServeHTTP(w, r)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// photoUploadMaxBytes is the largest picture the page's script uploads: in
// the demo the request limit, minus room for the rest of the form.
func (a *App) photoUploadMaxBytes() int64 {
	if a.Demo != nil && a.DemoLimits.MaxBody > 0 && a.DemoLimits.MaxBody-8<<10 < maxPhotoBytes {
		return a.DemoLimits.MaxBody - 8<<10
	}
	return maxPhotoBytes
}

// editableRecipe loads recipe {id} for a change by the signed-in user, or
// answers 404/403 and returns false.
func (a *App) editableRecipe(w http.ResponseWriter, r *http.Request) (int64, bool) {
	user := userFromContext(r.Context())
	id, err := parseID(r, "id")
	if err != nil {
		a.notFound(w, r)
		return 0, false
	}
	rec, err := a.Recipes.GetWithItems(r.Context(), id, user.ID)
	if err != nil {
		a.handleRecipeError(w, r, err)
		return 0, false
	}
	if rec.UserID != user.ID {
		http.Error(w, a.t(r, "recipes.error.not_author"), http.StatusForbidden)
		return 0, false
	}
	return id, true
}

// handleRecipePhotoUpload POST /recipes/{id}/photo (multipart, field photo).
// The page's script sends a picture it has already shrunk and answers 204 or
// the error as text; a plain form gets a redirect, with the error as flash.
func (a *App) handleRecipePhotoUpload(w http.ResponseWriter, r *http.Request) {
	id, ok := a.editableRecipe(w, r)
	if !ok {
		return
	}
	script := r.Header.Get("X-Requested-With") == "fetch"
	back := "/recipes/" + strconv.FormatInt(id, 10)
	fail := func(status int, key string) {
		msg := a.t(r, key)
		if script {
			http.Error(w, msg, status)
			return
		}
		a.Sessions.Put(r.Context(), "flash", msg)
		http.Redirect(w, r, back, http.StatusSeeOther)
	}

	f, _, err := r.FormFile("photo")
	if err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			fail(http.StatusRequestEntityTooLarge, "recipes.photo.error.too_large")
			return
		}
		fail(http.StatusUnprocessableEntity, "recipes.photo.error.missing")
		return
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, maxPhotoBytes+1))
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	if len(data) > maxPhotoBytes {
		fail(http.StatusRequestEntityTooLarge, "recipes.photo.error.too_large")
		return
	}
	big, thumb, err := photo.Process(r.Context(), data)
	switch {
	case errors.Is(err, photo.ErrFormat):
		fail(http.StatusUnprocessableEntity, "recipes.photo.error.format")
		return
	case errors.Is(err, photo.ErrDimensions):
		fail(http.StatusUnprocessableEntity, "recipes.photo.error.dimensions")
		return
	case err != nil && r.Context().Err() != nil:
		return // the client left while waiting its turn
	case err != nil:
		a.serverError(w, r, err)
		return
	}
	if err := a.Recipes.SetPhoto(r.Context(), id, userFromContext(r.Context()).ID, big, thumb); err != nil {
		a.handleRecipeError(w, r, err)
		return
	}
	if script {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	http.Redirect(w, r, back, http.StatusSeeOther)
}

// handleRecipePhotoDelete POST /recipes/{id}/photo/delete.
func (a *App) handleRecipePhotoDelete(w http.ResponseWriter, r *http.Request) {
	id, ok := a.editableRecipe(w, r)
	if !ok {
		return
	}
	if err := a.Recipes.DeletePhoto(r.Context(), id, userFromContext(r.Context()).ID); err != nil {
		a.handleRecipeError(w, r, err)
		return
	}
	http.Redirect(w, r, "/recipes/"+strconv.FormatInt(id, 10), http.StatusSeeOther)
}

// handleRecipePhoto GET /recipes/{id}/photo and handleRecipePhotoThumb GET
// /recipes/{id}/photo/thumb serve the JPEG to whoever may see the recipe.
func (a *App) handleRecipePhoto(w http.ResponseWriter, r *http.Request) {
	a.serveRecipePhoto(w, r, false)
}

func (a *App) handleRecipePhotoThumb(w http.ResponseWriter, r *http.Request) {
	a.serveRecipePhoto(w, r, true)
}

func (a *App) serveRecipePhoto(w http.ResponseWriter, r *http.Request, thumb bool) {
	user := userFromContext(r.Context())
	id, err := parseID(r, "id")
	if err != nil {
		a.notFound(w, r)
		return
	}
	// The same check as the recipe page: whoever can open it sees its photo.
	if _, err := a.Recipes.GetWithItems(r.Context(), id, user.ID); err != nil {
		a.handleRecipeError(w, r, err)
		return
	}
	data, etag, err := a.Recipes.Photo(r.Context(), id, thumb)
	if errors.Is(err, db.ErrPhotoNotFound) {
		a.notFound(w, r)
		return
	}
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	h := w.Header()
	h.Set("Content-Type", "image/jpeg")
	h.Set("ETag", `"`+etag+`"`)
	// The pages link ?v=<etag>: that URL never changes content.
	if r.URL.Query().Get("v") == etag {
		h.Set("Cache-Control", "private, max-age=31536000, immutable")
	} else {
		h.Set("Cache-Control", "private, no-cache")
	}
	http.ServeContent(w, r, "", time.Time{}, bytes.NewReader(data))
}
