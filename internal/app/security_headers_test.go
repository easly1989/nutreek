package app

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Lo scanner (#59) usa la fotocamera dalla pagina stessa e, senza
// BarcodeDetector nativo, compila il lettore WebAssembly vendorizzato.
func TestSecurityHeadersAllowScanner(t *testing.T) {
	h := securityHeaders(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	if got := rec.Header().Get("Permissions-Policy"); !strings.Contains(got, "camera=(self)") || !strings.Contains(got, "microphone=()") {
		t.Errorf("Permissions-Policy = %q, voglio camera=(self) e microfono spento", got)
	}
	csp := rec.Header().Get("Content-Security-Policy")
	var script string
	for _, d := range strings.Split(csp, ";") {
		if d = strings.TrimSpace(d); strings.HasPrefix(d, "script-src ") {
			script = d
		}
	}
	if !strings.Contains(script, "'wasm-unsafe-eval'") {
		t.Errorf("script-src = %q, manca 'wasm-unsafe-eval'", script)
	}
	if strings.Contains(script, "'unsafe-eval'") {
		t.Errorf("script-src = %q: 'unsafe-eval' non serve", script)
	}
}

// Le miniature dei prodotti OFF arrivano dal loro CDN (#133): consentito
// solo quell'host, non tutto openfoodfacts.org ne' altre origini.
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
		t.Fatalf("img-src = %v, voglio esattamente %v", img, want)
	}
	for _, s := range img {
		if !want[s] {
			t.Errorf("img-src: sorgente inattesa %q", s)
		}
	}
}
