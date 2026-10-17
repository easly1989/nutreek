package app

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestHandleLogin_Success(t *testing.T) {
	a := newTestApp(t)
	mustCreateTestUser(t, a, "utente@example.com", "password123")

	form := url.Values{
		"email":    {"utente@example.com"},
		"password": {"password123"},
	}.Encode()
	r := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(form))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	w := httptest.NewRecorder()
	a.Sessions.LoadAndSave(http.HandlerFunc(a.handleLogin)).ServeHTTP(w, r)

	if w.Code != http.StatusSeeOther {
		t.Errorf("login riuscito: want 303, got %d", w.Code)
	}
	if loc := w.Header().Get("Location"); loc != "/" {
		t.Errorf("redirect post-login: want /, got %q", loc)
	}
}

func TestHandleLogin_WrongPassword(t *testing.T) {
	a := newTestApp(t)
	mustCreateTestUser(t, a, "utente@example.com", "password123")

	form := url.Values{
		"email":    {"utente@example.com"},
		"password": {"sbagliata"},
	}.Encode()
	r := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(form))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	w := httptest.NewRecorder()
	a.Sessions.LoadAndSave(http.HandlerFunc(a.handleLogin)).ServeHTTP(w, r)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("password errata: want 401, got %d", w.Code)
	}
}

func TestHandleLogin_UnknownEmail(t *testing.T) {
	a := newTestApp(t)

	form := url.Values{
		"email":    {"nessuno@example.com"},
		"password": {"password123"},
	}.Encode()
	r := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(form))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	w := httptest.NewRecorder()
	a.Sessions.LoadAndSave(http.HandlerFunc(a.handleLogin)).ServeHTTP(w, r)

	// the same 401 answer, so as not to reveal whether the email exists (enumeration)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("email sconosciuta: want 401, got %d", w.Code)
	}
}

func TestHandleRegister_DuplicateEmail(t *testing.T) {
	a := newTestApp(t)
	mustCreateTestUser(t, a, "esistente@example.com", "password123")

	form := url.Values{
		"email":        {"esistente@example.com"},
		"display_name": {"Utente Duplicato"},
		"password":     {"password123"},
	}.Encode()
	r := httptest.NewRequest(http.MethodPost, "/register", strings.NewReader(form))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	w := httptest.NewRecorder()
	a.Sessions.LoadAndSave(http.HandlerFunc(a.handleRegister)).ServeHTTP(w, r)

	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf("email duplicata: want 422, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "già registrata") {
		t.Errorf("want the duplicate email error message in the body")
	}
}

func TestHandleRegister_MissingFields(t *testing.T) {
	a := newTestApp(t)

	form := url.Values{"email": {"solo@email.com"}}.Encode()
	r := httptest.NewRequest(http.MethodPost, "/register", strings.NewReader(form))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	w := httptest.NewRecorder()
	a.Sessions.LoadAndSave(http.HandlerFunc(a.handleRegister)).ServeHTTP(w, r)

	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf("campi mancanti: want 422, got %d", w.Code)
	}
}
