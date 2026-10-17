package app

import (
	"net/http"

	"github.com/amon2126/nutreek/internal/i18n"
)

// langCookie forces the interface language. Nothing sets it yet: until every
// area is translated (#141) it is how English gets tested; the language
// picker arrives with #159.
const langCookie = "lang"

// languageMiddleware puts the request's language in the context: the
// cookie's when it names a supported language, otherwise the default.
func languageMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		lang := i18n.Default
		if c, err := r.Cookie(langCookie); err == nil && i18n.Supported(c.Value) {
			lang = c.Value
		}
		next.ServeHTTP(w, r.WithContext(i18n.WithLang(r.Context(), lang)))
	})
}

// t translates a catalog key into the request's language. Flash messages
// and validation errors carry keys and go through it when shown; a text
// that is not a key comes back unchanged.
func (a *App) t(r *http.Request, key string, args ...any) string {
	return i18n.T(i18n.FromContext(r.Context()), key, args...)
}
