package app

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// The scanner (#59) uses the camera from the page itself and, without a
// native BarcodeDetector, compiles the vendored WebAssembly reader.
func TestSecurityHeadersAllowScanner(t *testing.T) {
	h := securityHeaders(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	if got := rec.Header().Get("Permissions-Policy"); !strings.Contains(got, "camera=(self)") || !strings.Contains(got, "microphone=()") {
		t.Errorf("Permissions-Policy = %q, want camera=(self) and the microphone off", got)
	}
	csp := rec.Header().Get("Content-Security-Policy")
	var script string
	for _, d := range strings.Split(csp, ";") {
		if d = strings.TrimSpace(d); strings.HasPrefix(d, "script-src ") {
			script = d
		}
	}
	if !strings.Contains(script, "'wasm-unsafe-eval'") {
		t.Errorf("script-src = %q, missing 'wasm-unsafe-eval'", script)
	}
	if strings.Contains(script, "'unsafe-eval'") {
		t.Errorf("script-src = %q: 'unsafe-eval' is not needed", script)
	}
}

// OFF product thumbnails come from their CDN (#133): only that host is
// allowed, not all of openfoodfacts.org nor other origins.
func TestSecurityHeadersAllowOFFImages(t *testing.T) {
	h := securityHeaders(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	var img []string
	for _, d := range strings.Split(rec.Header().Get("Content-Security-Policy"), ";") {
		if f := strings.Fields(d); len(f) > 0 && f[0] == "img-src" {
			img = f[1:]
		}
	}
	want := map[string]bool{"'self'": true, "data:": true, "https://images.openfoodfacts.org": true}
	if len(img) != len(want) {
		t.Fatalf("img-src = %v, want exactly %v", img, want)
	}
	for _, s := range img {
		if !want[s] {
			t.Errorf("img-src: unexpected source %q", s)
		}
	}
}
