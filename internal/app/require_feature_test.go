package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/amon2126/nutreek/internal/domain"
)

// Whoever opens a premium feature without having it lands on /upgrade with
// the feature in the address, so the page can say which one it is.
func TestRequireFeatureRedirectNamesFeature(t *testing.T) {
	a := newTestApp(t)
	a.Cfg.PremiumEnabled = true
	invalidateFeatureCache()
	t.Cleanup(invalidateFeatureCache)

	ctx := context.Background()
	id, err := a.Users.Create(ctx, "free@x.test", "h", "Free")
	if err != nil {
		t.Fatal(err)
	}
	user := &domain.User{ID: id, Email: "free@x.test"}

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { t.Error("handler reached without premium") })
	h := a.requireFeature("auto_plan")(next)

	req := httptest.NewRequest("POST", "/week/generate", nil).WithContext(contextWithUser(ctx, user))
	req.Header.Set("Accept", "text/html")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/upgrade?da=auto_plan" {
		t.Errorf("browser: %d %q", rec.Code, rec.Header().Get("Location"))
	}

	req = httptest.NewRequest("POST", "/week/generate", nil).WithContext(contextWithUser(ctx, user))
	req.Header.Set("HX-Request", "true")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if got := rec.Header().Get("HX-Redirect"); got != "/upgrade?da=auto_plan" {
		t.Errorf("HTMX: HX-Redirect = %q", got)
	}
}
