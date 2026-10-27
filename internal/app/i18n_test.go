package app

import (
	"cmp"
	"context"
	"errors"
	"html"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/amon2126/nutreek/internal/domain"
	"github.com/amon2126/nutreek/internal/i18n"
	"github.com/amon2126/nutreek/internal/off"
)

// italianWords are words that give away Italian text in an English page
// ("per" and "tue" are left out: English has them too).
var italianWords = map[string]bool{}

func init() {
	for _, w := range strings.Fields(`di da dal dalla il la le lo gli del della dei delle con non una uno che
		sono alla nel nella tuoi tuo tua ancora cerca cerco nuovo nuova salva modifica annulla chiudi
		esci settimana ricette ingredienti ingrediente spesa impostazioni errore pagina aspetto colore
		peso profilo altro categoria allergene allergeni tutti tutte nessun nessuna prodotto nome marca
		porzione valori grassi proteine carboidrati zuccheri sodio fibre importa codice indietro torna
		scansiona fotocamera cifre chiaro scuro sistema bosco tramonto grafite accedi registrati sezioni
		statistiche guida rapida pannello amministrazione riprova rete oppure qui sotto pezzo
		facoltativo facoltativa facoltativi grammi millilitri contiene azzera filtri applica risultato
		risultati verifico macro es conferma crea completo collegato valida valido mancante`) {
		italianWords[w] = true
	}
}

var (
	reScript   = regexp.MustCompile(`(?s)<script\b[^>]*>(.*?)</script>`)
	reStyle    = regexp.MustCompile(`(?s)<style\b[^>]*>.*?</style>`)
	reJSString = regexp.MustCompile(`"((?:[^"\\]|\\.)*)"|'((?:[^'\\]|\\.)*)'`)
	reAttr     = regexp.MustCompile(`\b(?:aria-label|title|placeholder|alt|data-(?:msg|confirm|placeholder)[\w-]*)="([^"]*)"`)
	// A tag ends at the first ">" outside quotes: Tailwind classes hold some.
	reTag     = regexp.MustCompile(`(?s)<(?:[^>"']|"[^"]*"|'[^']*')*>`)
	reComment = regexp.MustCompile(`(?s)<!--.*?-->`)
	reWord    = regexp.MustCompile(`[\p{L}]+`)
)

// pageChunks returns the pieces of user-visible text in page: the text, the
// attributes people read (or scripts show) and the strings in inline scripts.
func pageChunks(page string) []string {
	var chunks []string
	for _, m := range reScript.FindAllStringSubmatch(page, -1) {
		for _, s := range reJSString.FindAllStringSubmatch(m[1], -1) {
			chunks = append(chunks, s[1]+s[2])
		}
	}
	return append(chunks, pageTextChunks(page)...)
}

// pageTextChunks is pageChunks without the strings of inline scripts.
func pageTextChunks(page string) []string {
	var chunks []string
	page = reStyle.ReplaceAllString(reScript.ReplaceAllString(page, "\n"), "\n")
	for _, m := range reAttr.FindAllStringSubmatch(page, -1) {
		chunks = append(chunks, m[1])
	}
	page = reComment.ReplaceAllString(page, "\n")
	return append(chunks, strings.Split(reTag.ReplaceAllString(page, "\n"), "\n")...)
}

// italianIn returns the pieces of user-visible text in page that look Italian.
// data are texts that come from the database, not from the interface.
func italianIn(page string, data []string) []string {
	var found []string
	for _, c := range pageChunks(page) {
		c = strings.TrimSpace(html.UnescapeString(c))
		for _, d := range data {
			c = strings.ReplaceAll(c, d, "")
		}
		if c == "" {
			continue
		}
		if strings.ContainsAny(c, "àèéìòùÀÈÉÌÒÙ«»") {
			found = append(found, c)
			continue
		}
		for _, w := range reWord.FindAllString(c, -1) {
			if italianWords[strings.ToLower(w)] {
				found = append(found, c)
				break
			}
		}
	}
	return found
}

// TestEnglishLayoutAndIngredients renders the shell and every Ingredients
// page with the English cookie and finds no Italian left (#151). The same
// pages without the cookie must still be Italian, which also proves that
// italianIn sees Italian when there is some.
func TestEnglishLayoutAndIngredients(t *testing.T) {
	a := newTestApp(t)
	a.OFF = off.NewClient(a.Store.DB, "http://127.0.0.1:1", nil)
	ctx := context.Background()

	userID, err := a.Users.Create(ctx, "ada@example.test", "x", "Ada")
	if err != nil {
		t.Fatal(err)
	}
	user, _ := a.Users.GetByID(ctx, userID)
	user.Role = "coach" // the coach link in the menus too
	a.Cfg.OwnerEmail = user.Email
	otherID, err := a.Users.Create(ctx, "bob@example.test", "x", "Bob")
	if err != nil {
		t.Fatal(err)
	}
	bob, _ := a.Users.GetByID(ctx, otherID)
	carolID, err := a.Users.Create(ctx, "carol@example.test", "x", "Carol")
	if err != nil {
		t.Fatal(err)
	}
	carol, _ := a.Users.GetByID(ctx, carolID)
	if _, err := a.Categories.CreateCategory(ctx, carolID, "Snacks"); err != nil {
		t.Fatal(err)
	}

	brand := "Mill"
	oats, err := a.Ingredients.CreateLocal(ctx, "Oats", &brand, domain.UnitGram, 372, 13.5, 58.7, 7, &userID, nil, nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	rice, err := a.Ingredients.CreateLocal(ctx, "Rice", nil, domain.UnitGram, 360, 7, 79, 0.6, &otherID, nil, nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := a.Ingredients.SetLocalBarcode(ctx, oats, "8000000000002"); err != nil {
		t.Fatal(err)
	}
	catID, err := a.Categories.CreateCategory(ctx, userID, "Breakfast")
	if err != nil {
		t.Fatal(err)
	}
	if err := a.Categories.SetIngredientCategory(ctx, userID, oats, &catID); err != nil {
		t.Fatal(err)
	}
	allergens, _ := a.Categories.ListAllergens(ctx)
	if len(allergens) == 0 {
		t.Fatal("no allergens seeded")
	}
	if err := a.Categories.SetIngredientAllergens(ctx, oats, []int64{allergens[0].ID}); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Store.DB.Exec(`INSERT INTO off_products (barcode, product_name, brands, quantity,
		kcal_per_100, protein_per_100, carbs_per_100, fat_per_100, has_full_nutrients, updated_at) VALUES
		('8012345678907', 'Oat flakes', 'Farm', '500 g', 372, 13.5, 58.7, 7, 1, 0),
		('8015555555556', 'Oat bran', 'Farm', '500 g', 0, 0, 0, 0, 0, 0)`); err != nil {
		t.Fatal(err)
	}
	// Reference data (#158) and what users typed are not interface text.
	data := []string{"Ada", "Bob", "Carol", "Snacks", "Oats", "Rice", "Mill", "Breakfast", "Oat flakes", "Oat bran", "Farm"}
	for _, al := range allergens {
		data = append(data, al.Name)
	}

	type page struct {
		name, method, target string
		form                 url.Values
		id                   int64        // the {id} route parameter
		as                   *domain.User // the viewer, when not Ada (the owner)
		flash                string
		italian              string // what gives the Italian page away, when not a word
		want                 string // English text the page must show
		handler              func(http.ResponseWriter, *http.Request)
	}
	pages := []page{
		{name: "list with a flash", target: "/ingredients", flash: "ingredients.flash.created", want: "Ingredient created.", handler: a.handleIngredientsList},
		{name: "list with a query", target: "/ingredients?q=oat", handler: a.handleIngredientsList},
		{name: "list, nothing found", target: "/ingredients?q=zzz", handler: a.handleIngredientsList},
		{name: "list, filtered", target: "/ingredients?src=mine&allergen=" + strconv.FormatInt(allergens[len(allergens)-1].ID, 10) + "&category=" + strconv.FormatInt(catID, 10), handler: a.handleIngredientsList},
		{name: "search partial", target: "/ingredients/search?q=oat&off=1", handler: a.handleIngredientsSearchPartial},
		{name: "new", target: "/ingredients/new", handler: a.handleIngredientNewForm},
		{name: "new, name missing", method: http.MethodPost, target: "/ingredients", form: url.Values{"unit": {"g"}}, handler: a.handleIngredientCreate},
		{name: "new, bad number", method: http.MethodPost, target: "/ingredients", form: url.Values{"name": {"Kale"}, "unit": {"g"}, "kcal": {"lots"}}, want: "Invalid kcal/100: not a number", handler: a.handleIngredientCreate},
		{name: "new, NaN", method: http.MethodPost, target: "/ingredients", form: url.Values{"name": {"Kale"}, "unit": {"g"}, "kcal": {"NaN"}}, want: "Invalid kcal/100: not a number", handler: a.handleIngredientCreate},
		{name: "edit own", target: "/ingredients/x/edit", id: oats, handler: a.handleIngredientEditForm},
		{name: "edit someone else's", target: "/ingredients/x/edit", id: rice, handler: a.handleIngredientEditForm},
		{name: "read-only, no categories", target: "/ingredients/x/edit", id: oats, as: bob, want: "first create one in", handler: a.handleIngredientEditForm},
		{name: "read-only with categories", target: "/ingredients/x/edit", id: oats, as: carol, want: "Only Ada, who created it", handler: a.handleIngredientEditForm},
		{name: "edit, negative number", method: http.MethodPost, target: "/ingredients/x", id: oats, form: url.Values{"name": {"Oats"}, "unit": {"g"}, "kcal": {"1"}, "protein": {"-1"}}, want: "Invalid protein/100: negative value", handler: a.handleIngredientUpdate},
		{name: "quick new", target: "/quick/ingredient", handler: a.handleQuickIngredientForm},
		{name: "quick new, code taken", method: http.MethodPost, target: "/quick/ingredient", form: url.Values{"name": {"Kale"}, "barcode": {"8000000000002"}}, want: "already linked to", handler: a.handleQuickIngredientCreate},
		{name: "not found", target: "/nowhere", handler: a.notFound},
		{name: "server error", target: "/", handler: func(w http.ResponseWriter, r *http.Request) {
			a.serverError(w, r, errors.New("boom"))
		}},
	}

	render := func(p page, lang string) string {
		method := p.method
		if method == "" {
			method = http.MethodGet
		}
		req := httptest.NewRequest(method, p.target, strings.NewReader(p.form.Encode()))
		if p.form != nil {
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		}
		req.AddCookie(&http.Cookie{Name: langCookie, Value: cmp.Or(lang, "it")}) // "" is the Italian run
		if p.id != 0 {
			rctx := chi.NewRouteContext()
			rctx.URLParams.Add("id", strconv.FormatInt(p.id, 10))
			req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
		}
		viewer := user
		if p.as != nil {
			viewer = p.as
		}
		req = requestWithUser(req, viewer)
		h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if p.flash != "" {
				a.Sessions.Put(r.Context(), "flash", p.flash)
			}
			p.handler(w, r)
		})
		rec := httptest.NewRecorder()
		languageMiddleware(a.Sessions.LoadAndSave(h)).ServeHTTP(rec, req)
		if rec.Code >= 300 && rec.Code < 400 {
			t.Fatalf("%s: redirected to %s", p.name, rec.Header().Get("Location"))
		}
		return rec.Body.String()
	}

	for _, p := range pages {
		t.Run(p.name, func(t *testing.T) {
			body := render(p, "en")
			if !strings.Contains(body, p.want) {
				t.Errorf("%q missing from the English page:\n%s", p.want, body)
			}
			if found := italianIn(body, data); len(found) > 0 {
				t.Errorf("Italian in the English page:\n  %s", strings.Join(found, "\n  "))
			}
			it := render(p, "")
			if p.italian != "" && !strings.Contains(it, p.italian) || p.italian == "" && len(italianIn(it, data)) == 0 {
				t.Errorf("the page without the cookie is not Italian:\n%s", it)
			}
		})
	}

	t.Run("empty list", func(t *testing.T) {
		b := newTestApp(t)
		id, err := b.Users.Create(ctx, "eve@example.test", "x", "Eve")
		if err != nil {
			t.Fatal(err)
		}
		eve, _ := b.Users.GetByID(ctx, id)
		req := requestWithUser(httptest.NewRequest(http.MethodGet, "/ingredients", nil), eve)
		req.AddCookie(&http.Cookie{Name: langCookie, Value: "en"})
		rec := httptest.NewRecorder()
		languageMiddleware(b.Sessions.LoadAndSave(http.HandlerFunc(b.handleIngredientsList))).ServeHTTP(rec, req)
		if !strings.Contains(rec.Body.String(), "You have no ingredients yet") {
			t.Fatalf("no empty state:\n%s", rec.Body.String())
		}
		if found := italianIn(rec.Body.String(), []string{"Eve"}); len(found) > 0 {
			t.Errorf("Italian in the English page:\n  %s", strings.Join(found, "\n  "))
		}
	})
}

// The request's language: the user's choice, the cookie, the browser, English.
func TestRequestLanguage(t *testing.T) {
	cases := []struct {
		name, locale, cookie, accept, want string
		signedIn                           bool
	}{
		{name: "nothing: English", want: "en"},
		{name: "browser", accept: "it-IT,it;q=0.9", want: "it"},
		{name: "browser in another language", accept: "de-DE", want: "en"},
		{name: "cookie beats the browser", cookie: "it", accept: "en-US", want: "it"},
		{name: "unsupported cookie", cookie: "xx", accept: "it", want: "it"},
		{name: "user beats the cookie", signedIn: true, locale: "en", cookie: "it", accept: "it", want: "en"},
		{name: "user following the browser", signedIn: true, locale: "", cookie: "en", accept: "it", want: "en"},
		{name: "user following the browser, no cookie", signedIn: true, accept: "it", want: "it"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var got string
			h := languageMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				got = i18n.FromContext(r.Context())
			}))
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			if c.cookie != "" {
				req.AddCookie(&http.Cookie{Name: langCookie, Value: c.cookie})
			}
			if c.accept != "" {
				req.Header.Set("Accept-Language", c.accept)
			}
			if c.signedIn {
				req = requestWithUser(req, &domain.User{ID: 1, Locale: c.locale})
			}
			h.ServeHTTP(httptest.NewRecorder(), req)
			if got != c.want {
				t.Errorf("language = %q, want %q", got, c.want)
			}
		})
	}
}
