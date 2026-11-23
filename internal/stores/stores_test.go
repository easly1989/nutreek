package stores

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const jam = `{"id":"p1","name":"Passata","brand":"Conad","size":{"quantity":700,"unit":"g"},"price":1.09,
 "available":true,"nutrition":{"kcal":32,"protein":1.4,"carbs":5.1,"fat":0.2,"fiber":1.2}}`

func plugin(t *testing.T) (*httptest.Server, *Client) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer secret" {
			http.Error(w, "no", http.StatusUnauthorized)
			return
		}
		switch r.URL.Path {
		case "/search":
			// The second product has no macros, the third no name: both are left out.
			w.Write([]byte(`{"products":[` + jam + `,{"id":"p2","name":"No macros","size":{"quantity":1,"unit":"pz"}},{"id":"p3","name":"","nutrition":{"kcal":1,"protein":1,"carbs":1,"fat":1}}]}`))
		case "/product":
			if r.URL.Query().Get("id") == "p1" {
				w.Write([]byte(jam))
				return
			}
			w.Write([]byte(strings.Replace(jam, `"p1"`, `"other"`, 1)))
		case "/availability":
			w.Write([]byte(`{"products":[{"id":"p1","price":1.19,"available":false}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, NewClient(true)
}

func TestPluginContract(t *testing.T) {
	srv, c := plugin(t)
	ctx := context.Background()

	got, err := c.Search(ctx, srv.URL, "secret", "passata")
	if err != nil || len(got) != 1 {
		t.Fatalf("Search = %v, %v: want only the product with a name and macros", got, err)
	}
	p := got[0]
	if p.PriceCents != 109 || p.SizeQty != 700 || p.SizeUnit != "g" || !p.Available || p.Kcal != 32 || p.Fiber == nil || p.Sugars != nil {
		t.Errorf("product = %+v", p)
	}
	if _, err := c.Search(ctx, srv.URL, "wrong", "x"); err == nil {
		t.Error("a wrong token must fail")
	}
	if _, err := c.Product(ctx, srv.URL, "secret", "p1"); err != nil {
		t.Errorf("Product: %v", err)
	}
	if _, err := c.Product(ctx, srv.URL, "secret", "p9"); err == nil {
		t.Error("a product whose id is not the one asked for must be refused")
	}
	st, err := c.Availability(ctx, srv.URL, "secret", []string{"p1", "gone"})
	if err != nil || len(st) != 1 || st["p1"].PriceCents != 119 || st["p1"].Available {
		t.Errorf("Availability = %v, %v", st, err)
	}
}

func TestPluginAnswersAreBounded(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/search":
			w.Write([]byte(`{"products":[` + strings.Repeat(" ", maxBody) + `]}`))
		default:
			http.Redirect(w, r, "http://127.0.0.1:1/elsewhere", http.StatusFound)
		}
	}))
	defer srv.Close()
	c := NewClient(true)
	if _, err := c.Search(context.Background(), srv.URL, "t", "x"); err == nil {
		t.Error("an answer over 1 MiB must be refused")
	}
	if _, err := c.Product(context.Background(), srv.URL, "t", "p"); err == nil {
		t.Error("a redirect must not be followed")
	}
	// The public-only client never connects to a loopback address.
	if _, err := NewClient(false).Search(context.Background(), srv.URL, "t", "x"); err == nil {
		t.Error("a private address must be refused unless allowed")
	}
}

func TestCleanURL(t *testing.T) {
	for in, want := range map[string]string{"https://a.example/x/": "https://a.example/x", " https://10.0.0.2:8080 ": "https://10.0.0.2:8080"} {
		if got, err := CleanURL(in, false); err != nil || got != want {
			t.Errorf("CleanURL(%q) = %q, %v", in, got, err)
		}
	}
	if got, err := CleanURL("http://10.0.0.2:8080", true); err != nil || got != "http://10.0.0.2:8080" {
		t.Errorf("http where private addresses are allowed: %q, %v", got, err)
	}
	for _, bad := range []string{"", "ftp://a.example", "a.example", "https://u:p@a.example", "https://a.example/?x=1", "javascript:alert(1)", "http://a.example"} {
		if _, err := CleanURL(bad, false); err == nil {
			t.Errorf("CleanURL(%q, false) must fail", bad)
		}
	}
}

func TestBox(t *testing.T) {
	if _, err := NewBox("short"); err == nil {
		t.Error("a short secret must be refused")
	}
	a, _ := NewBox("0123456789abcdef-a")
	b, _ := NewBox("0123456789abcdef-b")
	sealed, err := a.Seal("token")
	if err != nil || strings.Contains(sealed, "token") {
		t.Fatalf("Seal = %q, %v", sealed, err)
	}
	if got, err := a.Open(sealed); err != nil || got != "token" {
		t.Errorf("Open = %q, %v", got, err)
	}
	if _, err := b.Open(sealed); err == nil {
		t.Error("another key must not open it")
	}
	if _, err := a.Open("not base64!"); err == nil {
		t.Error("junk must not open")
	}
}

func TestEstimate(t *testing.T) {
	for _, c := range []struct {
		need        float64
		unit        string
		pieceG      float64
		size        float64
		sizeUnit    string
		price       int
		cents, pack int
		ok          bool
	}{
		{700, "g", 0, 700, "g", 109, 109, 1, true},    // exactly one package
		{701, "g", 0, 700, "g", 109, 218, 2, true},    // a bit more: a second one
		{50, "g", 0, 700, "g", 109, 109, 1, true},     // a little: still one
		{300, "ml", 0, 1000, "ml", 749, 749, 1, true}, //
		{600, "g", 120, 6, "pz", 250, 250, 1, true},   // 5 pieces of 120 g, packs of 6
		{800, "g", 120, 6, "pz", 250, 500, 2, true},   // 6.7 pieces: two packages
		{500, "g", 0, 1000, "ml", 100, 0, 0, false},   // g against ml
		{500, "g", 0, 6, "pz", 100, 0, 0, false},      // pieces without a piece weight
		{0, "g", 0, 700, "g", 109, 0, 0, false},       // nothing needed
		{500, "g", 0, 0, "g", 109, 0, 0, false},       // a package of unknown size
	} {
		cents, packs, ok := Estimate(c.need, c.unit, c.pieceG, c.size, c.sizeUnit, c.price)
		if ok != c.ok || cents != c.cents || packs != c.pack {
			t.Errorf("Estimate(%v %s, size %v %s) = %d, %d, %v; want %d, %d, %v", c.need, c.unit, c.size, c.sizeUnit, cents, packs, ok, c.cents, c.pack, c.ok)
		}
	}
}
