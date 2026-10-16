package app

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/amon2126/nutreek/internal/off"
)

// Le miniature OFF escono solo dall'host consentito dalla CSP, senza referrer,
// e con l'icona del codice sotto per quando la foto non arriva (#133).
func TestOFFThumbnailInResults(t *testing.T) {
	a := newTestApp(t)
	a.OFF = off.NewClient(a.Store.DB, "http://127.0.0.1:1", nil)
	user := mustCreateTestUser(t, a, "foto@esempio.test", "password-demo")
	if _, err := a.Store.DB.Exec(`INSERT INTO off_products (barcode, product_name, brands, quantity,
		kcal_per_100, protein_per_100, carbs_per_100, fat_per_100, has_full_nutrients, image_url, updated_at) VALUES
		('8012345678907', 'Fiocchi d''avena', 'Molino', '500 g', 372, 13.5, 58.7, 7, 1,
		 'https://static.openfoodfacts.org/images/products/801/234/567/8907/front_it.3.200.jpg', 0),
		('8015555555556', 'Fiocchi di farro', 'Molino', '375 g', 358, 14.1, 64.2, 2.6, 1,
		 'https://tracker.example.com/pixel.gif', 0)`); err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/ingredients/search?q=fiocchi&off=1", nil)
	a.handleIngredientsSearchPartial(rec, requestWithUser(req, user))
	body := rec.Body.String()
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d\n%s", rec.Code, body)
	}

	if !strings.Contains(body, `src="https://images.openfoodfacts.org/images/products/801/234/567/8907/front_it.3.200.jpg"`) {
		t.Errorf("la foto dal vecchio host static. non e' stata spostata su images.:\n%s", body)
	}
	if strings.Contains(body, "static.openfoodfacts.org") || strings.Contains(body, "tracker.example.com") {
		t.Errorf("nel risultato resta un host non consentito:\n%s", body)
	}
	// Per il farro l'URL del dump non e' mostrabile: si ripiega su quello dal codice.
	if !strings.Contains(body, "/images/products/801/555/555/5556/") {
		t.Errorf("manca la miniatura costruita dal codice per il farro:\n%s", body)
	}
	if n := strings.Count(body, `referrerpolicy="no-referrer"`); n != 2 {
		t.Errorf("immagini senza no-referrer: %d su 2", n)
	}
	if n := strings.Count(body, `href="#i-barcode"`); n < 2 {
		t.Errorf("l'icona del codice deve stare sotto ogni foto: trovate %d", n)
	}
}
