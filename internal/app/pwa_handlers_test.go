package app

import (
	"encoding/json"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/amon2126/nutreek/internal/web"
)

type manifestIcon struct {
	Src     string `json:"src"`
	Sizes   string `json:"sizes"`
	Type    string `json:"type"`
	Purpose string `json:"purpose"`
}

type manifestDoc struct {
	ID              string         `json:"id"`
	Name            string         `json:"name"`
	ShortName       string         `json:"short_name"`
	Lang            string         `json:"lang"`
	StartURL        string         `json:"start_url"`
	Scope           string         `json:"scope"`
	Display         string         `json:"display"`
	ThemeColor      string         `json:"theme_color"`
	BackgroundColor string         `json:"background_color"`
	Icons           []manifestIcon `json:"icons"`
}

func getManifest(t *testing.T, a *App, cookies ...*http.Cookie) (*httptest.ResponseRecorder, manifestDoc) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/manifest.webmanifest", nil)
	for _, c := range cookies {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	a.Router().ServeHTTP(rec, req)
	var m manifestDoc
	if rec.Code == http.StatusOK {
		if err := json.Unmarshal(rec.Body.Bytes(), &m); err != nil {
			t.Fatalf("manifest is not JSON: %v\n%s", err, rec.Body.String())
		}
	}
	return rec, m
}

func TestManifestFollowsAccent(t *testing.T) {
	a := newTestApp(t)

	rec, m := getManifest(t, a)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/manifest+json") {
		t.Errorf("Content-Type = %q", ct)
	}
	if m.Name != "Nutreek" || m.ShortName != "Nutreek" || m.Lang != "en" || m.Display != "standalone" ||
		m.StartURL != "/" || m.Scope != "/" || m.ID != "/" {
		t.Errorf("manifest base: %+v", m)
	}
	if m.ThemeColor != "#2F7A55" {
		t.Errorf("without a cookie theme_color = %q, want the forest one", m.ThemeColor)
	}

	_, m = getManifest(t, a, &http.Cookie{Name: cookieAccent, Value: "tramonto"})
	if m.ThemeColor != "#B4511A" {
		t.Errorf("tramonto: theme_color = %q", m.ThemeColor)
	}
	var any192, any512, mask512 bool
	static := web.Static()
	for _, ic := range m.Icons {
		if !strings.Contains(ic.Src, "-tramonto-") {
			t.Errorf("icon %q does not follow the color", ic.Src)
		}
		if _, err := fs.Stat(static, strings.TrimPrefix(ic.Src, "/static/")); err != nil {
			t.Errorf("icon %q does not exist: %v", ic.Src, err)
		}
		switch {
		case ic.Sizes == "192x192" && ic.Purpose == "any":
			any192 = true
		case ic.Sizes == "512x512" && ic.Purpose == "any":
			any512 = true
		case ic.Sizes == "512x512" && ic.Purpose == "maskable":
			mask512 = true
		}
	}
	if !any192 || !any512 || !mask512 {
		t.Errorf("icons: want 192 and 512 «any» and 512 «maskable», got %+v", m.Icons)
	}
}

func TestManifestDarkBackground(t *testing.T) {
	a := newTestApp(t)
	_, light := getManifest(t, a, &http.Cookie{Name: cookieMode, Value: "chiaro"})
	_, dark := getManifest(t, a, &http.Cookie{Name: cookieMode, Value: "scuro"})
	if light.BackgroundColor == dark.BackgroundColor {
		t.Errorf("the splash screen background does not change with dark mode: %q", dark.BackgroundColor)
	}
}

func TestServiceWorker(t *testing.T) {
	a := newTestApp(t)
	rec := httptest.NewRecorder()
	a.Router().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/sw.js", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/javascript") {
		t.Errorf("Content-Type = %q", ct)
	}
	// The browser must check again on every visit, otherwise an app update
	// would stay hidden behind the old worker.
	if cc := rec.Header().Get("Cache-Control"); !strings.Contains(cc, "no-cache") {
		t.Errorf("Cache-Control = %q", cc)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `"nutreek-static-`+web.StaticVersion()+`"`) {
		t.Errorf("the cache name does not carry the static assets' version:\n%s", body)
	}
	// Only /static: no pages (user data) in the cache.
	if !strings.Contains(body, `"/static/"`) {
		t.Errorf("the worker does not limit the cache to /static/")
	}
}
