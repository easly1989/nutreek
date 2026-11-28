package app

import (
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Pages, styles and scripts are sent gzipped to a client that accepts it, and the body reads back
// the same; a client that does not ask gets the plain text.
func TestRouterCompressesText(t *testing.T) {
	a := newTestApp(t)
	h := a.Router()
	get := func(path string, gz bool) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodGet, path, nil)
		if gz {
			r.Header.Set("Accept-Encoding", "gzip")
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	plain := get("/login", false)
	if plain.Code != http.StatusOK || plain.Header().Get("Content-Encoding") != "" {
		t.Fatalf("plain: code %d, encoding %q", plain.Code, plain.Header().Get("Content-Encoding"))
	}
	zipped := get("/login", true)
	if zipped.Header().Get("Content-Encoding") != "gzip" {
		t.Fatalf("page: encoding %q, want gzip", zipped.Header().Get("Content-Encoding"))
	}
	zr, err := gzip.NewReader(strings.NewReader(zipped.Body.String()))
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(zr)
	if err != nil || !strings.Contains(string(body), "<html") {
		t.Fatalf("the gzipped page does not read back: %v", err)
	}
	if zipped.Body.Len() >= plain.Body.Len() {
		t.Errorf("gzipped %d bytes, plain %d: not smaller", zipped.Body.Len(), plain.Body.Len())
	}
	// tailwind.css is built, not committed, so the test uses a stylesheet that is in the repo.
	if css := get("/static/css/shepherd.css", true); css.Header().Get("Content-Encoding") != "gzip" {
		t.Errorf("stylesheet: code %d, encoding %q, want gzip", css.Code, css.Header().Get("Content-Encoding"))
	}
	// A byte range is of the file, not of its gzip: that answer is left alone.
	rr := httptest.NewRequest(http.MethodGet, "/static/css/shepherd.css", nil)
	rr.Header.Set("Accept-Encoding", "gzip")
	rr.Header.Set("Range", "bytes=0-9")
	rw := httptest.NewRecorder()
	h.ServeHTTP(rw, rr)
	if rw.Code != http.StatusPartialContent || rw.Header().Get("Content-Encoding") != "" || rw.Body.Len() != 10 {
		t.Errorf("range: code %d, encoding %q, %d bytes, want 206, none, 10", rw.Code, rw.Header().Get("Content-Encoding"), rw.Body.Len())
	}
}
