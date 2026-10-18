package app

import (
	"net/url"
	"strings"
	"testing"

	"github.com/amon2126/nutreek/internal/demo"
	"github.com/amon2126/nutreek/internal/off"
)

// A visitor sees the demo's OpenFoodFacts products in their own language only,
// in the list and in the search; a barcode still finds any of them (#189).
func TestDemoProductsInOwnLanguage(t *testing.T) {
	a := newDemoApp(t, DemoLimits{})
	a.OFF = off.NewClient(a.Store.DB, "http://127.0.0.1:1", nil) // the local copy only
	names := func(prefix string) []string {
		t.Helper()
		rows, err := a.Store.DB.Query(`SELECT name FROM ingredients WHERE brand = 'Nutreek Demo' AND off_barcode LIKE ? || '%'`, prefix)
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		var out []string
		for rows.Next() {
			var n string
			if err := rows.Scan(&n); err != nil {
				t.Fatal(err)
			}
			out = append(out, n)
		}
		if len(out) == 0 {
			t.Fatalf("no demo products with prefix %s", prefix)
		}
		return out
	}
	english, italian := names("2000000001"), names("2000000002")

	b := newBrowser(t, a)
	b.setLang("it")
	enterAs(t, a, b, demo.AsPlan)
	for _, path := range []string{"/ingredients", "/ingredients/search?off=1&q=" + url.QueryEscape("Nutreek Demo")} {
		_, _, page := b.get(path)
		for _, n := range italian {
			if !strings.Contains(page, n) {
				t.Errorf("%s: no %q", path, n)
			}
		}
		for _, n := range english {
			if strings.Contains(page, n) {
				t.Errorf("%s: shows the English %q", path, n)
			}
		}
	}
	// An English product's barcode, typed in.
	var code, name string
	if err := a.Store.DB.QueryRow(`SELECT off_barcode, name FROM ingredients WHERE brand = 'Nutreek Demo' AND off_barcode LIKE '2000000001%' LIMIT 1`).Scan(&code, &name); err != nil {
		t.Fatal(err)
	}
	if _, _, page := b.get("/ingredients/search?q=" + code); !strings.Contains(page, name) {
		t.Errorf("barcode %s does not find %q", code, name)
	}
}
