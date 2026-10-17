package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/amon2126/nutreek/internal/domain"
)

// POST /language saves the choice in the account and in the cookie, and
// goes back only to a page of this site.
func TestHandleLanguage(t *testing.T) {
	a := newTestApp(t)
	ctx := context.Background()
	id, err := a.Users.Create(ctx, "lang@example.test", "x", "Lang")
	if err != nil {
		t.Fatal(err)
	}

	post := func(user *domain.User, lang, next string) *httptest.ResponseRecorder {
		t.Helper()
		form := url.Values{"lang": {lang}, "next": {next}}
		req := httptest.NewRequest(http.MethodPost, "/language", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req = requestWithUser(req, user)
		rec := httptest.NewRecorder()
		a.handleLanguage(rec, req)
		return rec
	}
	cookie := func(rec *httptest.ResponseRecorder) *http.Cookie {
		for _, c := range rec.Result().Cookies() {
			if c.Name == langCookie {
				return c
			}
		}
		return nil
	}
	locale := func() string {
		t.Helper()
		u, err := a.Users.GetByID(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		return u.Locale
	}
	user := &domain.User{ID: id}

	rec := post(user, "en", "/settings")
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/settings" {
		t.Errorf("en: %d to %q", rec.Code, rec.Header().Get("Location"))
	}
	if c := cookie(rec); c == nil || c.Value != "en" || c.MaxAge <= 0 {
		t.Errorf("en: cookie %+v", c)
	}
	if got := locale(); got != "en" {
		t.Errorf("en: locale = %q", got)
	}

	// "Like the browser": empty locale, cookie removed.
	rec = post(user, "", "/settings")
	if c := cookie(rec); c == nil || c.MaxAge >= 0 {
		t.Errorf("browser: cookie %+v, want it removed", c)
	}
	if got := locale(); got != "" {
		t.Errorf("browser: locale = %q", got)
	}

	// Signed out: only the cookie; next outside the site falls back to "/".
	rec = post(nil, "it", "//evil.example/x")
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/" {
		t.Errorf("signed out: %d to %q", rec.Code, rec.Header().Get("Location"))
	}
	if c := cookie(rec); c == nil || c.Value != "it" {
		t.Errorf("signed out: cookie %+v", c)
	}

	if rec := post(user, "xx", "/"); rec.Code != http.StatusBadRequest {
		t.Errorf("unsupported language: %d", rec.Code)
	}
	if got := locale(); got != "" {
		t.Errorf("after the rejected choice: locale = %q", got)
	}
}

// The REST API answers in English, whatever the user's language.
func TestAPIErrorsAreEnglish(t *testing.T) {
	a := newTestApp(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
	req.AddCookie(&http.Cookie{Name: langCookie, Value: "it"})
	req.Header.Set("Authorization", "Bearer nope")
	rec := httptest.NewRecorder()
	a.Router().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized || !strings.Contains(rec.Body.String(), `"invalid token"`) {
		t.Errorf("%d %s", rec.Code, rec.Body.String())
	}
}
