package app

import (
	"net/http"
	"time"

	"github.com/amon2126/nutreek/internal/config"
	"github.com/amon2126/nutreek/internal/i18n"
)

// langCookie holds the language picked on the public pages or in Settings,
// for the pages a signed-out visitor sees.
const langCookie = "lang"

// languageMiddleware puts the request's language in the context (#159), the
// first of: the signed-in user's choice, the lang cookie, the best match of
// the browser's languages, English. It runs after loadUserMiddleware.
func languageMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r.WithContext(i18n.WithLang(r.Context(), requestLang(r))))
	})
}

func requestLang(r *http.Request) string {
	if u := userFromContext(r.Context()); u != nil && i18n.Supported(u.Locale) {
		return u.Locale
	}
	if c, err := r.Cookie(langCookie); err == nil && i18n.Supported(c.Value) {
		return c.Value
	}
	return i18n.Match(r.Header.Get("Accept-Language"))
}

// t translates a catalog key into the request's language. Flash messages
// and validation errors carry keys and go through it when shown; a text
// that is not a key comes back unchanged.
func (a *App) t(r *http.Request, key string, args ...any) string {
	return i18n.T(i18n.FromContext(r.Context()), key, args...)
}

// handleLanguage (POST /language) saves the language picked in Settings or on
// a public page: "" follows the browser. It goes in the user's account when
// signed in, and in the cookie for the signed-out pages; then back to next.
func (a *App) handleLanguage(w http.ResponseWriter, r *http.Request) {
	lang := r.PostFormValue("lang")
	if lang != "" && !i18n.Supported(lang) {
		http.Error(w, "unsupported language", http.StatusBadRequest)
		return
	}
	if u := userFromContext(r.Context()); u != nil {
		if err := a.Users.SetLocale(r.Context(), u.ID, lang); err != nil {
			a.Logger.Error("save user locale", "err", err, "user_id", u.ID)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
	}
	c := &http.Cookie{Name: langCookie, Value: lang, Path: "/", HttpOnly: true,
		Secure: a.Cfg.Env == config.EnvProd, SameSite: http.SameSiteLaxMode,
		MaxAge: int((365 * 24 * time.Hour).Seconds())}
	if lang == "" {
		c.MaxAge = -1
	}
	http.SetCookie(w, c)
	http.Redirect(w, r, localPath(r.PostFormValue("next"), "/"), http.StatusSeeOther)
}
