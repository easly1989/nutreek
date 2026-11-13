package app

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func seedLocalIngredients(t *testing.T, a *App, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		code := fmt.Sprintf("8%05d", i)
		name := fmt.Sprintf("Ingrediente %03d", i)
		if _, err := a.Ingredients.CreateOFF(context.Background(), code, name, nil, "g", 1, 1, 1, 1, nil, nil, nil, nil, nil, nil, nil); err != nil {
			t.Fatal(err)
		}
	}
}

// The listing used to stop at the first 50 ingredients; page 2 must show the rest.
func TestIngredientsListPageTwoShowsRowsBeyondFirstFifty(t *testing.T) {
	a := newTestApp(t)
	u := mustCreateTestUser(t, a, "list@example.com", "password123")
	seedLocalIngredients(t, a, 75)

	get := func(target string) string {
		req := requestWithUser(httptest.NewRequest(http.MethodGet, target, nil), u)
		rec := httptest.NewRecorder()
		languageMiddleware(a.Sessions.LoadAndSave(http.HandlerFunc(a.handleIngredientsList))).ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: status %d", target, rec.Code)
		}
		return rec.Body.String()
	}
	p1 := get("/ingredients")
	if !strings.Contains(p1, "Ingrediente 000") || strings.Contains(p1, "Ingrediente 060") {
		t.Error("page 1 should hold the first 50 only")
	}
	p2 := get("/ingredients?page=2")
	if !strings.Contains(p2, "Ingrediente 060") || !strings.Contains(p2, "Ingrediente 074") || strings.Contains(p2, "Ingrediente 000") {
		t.Error("page 2 should hold rows 51-75")
	}
}
