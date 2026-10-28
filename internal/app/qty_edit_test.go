package app

import (
	"bytes"
	"context"
	"html"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/amon2126/nutreek/internal/db"
	"github.com/amon2126/nutreek/internal/macros"
)

// postNoToken posts form without a CSRF token, as a forged request would.
func (b *browser) postNoToken(path string, form url.Values) int {
	b.t.Helper()
	req, _ := http.NewRequest(http.MethodPost, b.srv.URL+path, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Origin", b.srv.URL)
	resp, err := b.c.Do(req)
	if err != nil {
		b.t.Fatalf("POST %s: %v", path, err)
	}
	resp.Body.Close()
	return resp.StatusCode
}

// submitHX is submit as HTMX sends it: the answer is a fragment.
func (b *browser) submitHX(from, path string, form url.Values) (int, string) {
	b.t.Helper()
	_, _, page := b.get(from)
	m := csrfField.FindStringSubmatch(page)
	if m == nil {
		b.t.Fatalf("no CSRF token on %s", from)
	}
	form.Set("csrf_token", html.UnescapeString(m[1]))
	req, _ := http.NewRequest(http.MethodPost, b.srv.URL+path, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Origin", b.srv.URL)
	req.Header.Set("HX-Request", "true")
	resp, err := b.c.Do(req)
	if err != nil {
		b.t.Fatalf("POST %s: %v", path, err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(body)
}

// A recipe row added in pieces stays in pieces, and its quantity can be
// edited in place by the author only, with the rules of adding (#206).
func TestRecipeQuantityEditor(t *testing.T) {
	a := newTestApp(t)
	ctx := context.Background()
	alice, aliceID := photoUser(t, a, "alice@example.test")
	bob, _ := photoUser(t, a, "bob@example.test")
	weight := 60.0
	egg, _ := a.Ingredients.CreateLocal(ctx, "Egg", nil, "g", 143, 12.6, 0.7, 9.5, &aliceID, &weight, nil, nil, nil, nil, nil)
	rec, _ := a.Recipes.Create(ctx, aliceID, "Eggs", nil, 1, true)
	other, _ := a.Recipes.Create(ctx, aliceID, "Other", nil, 1, false)
	page := "/recipes/" + strconv.FormatInt(rec, 10)

	// Added as 2 pieces: shown as pieces, counted as 120 g (171.6 kcal).
	if code, _ := alice.post(page, page+"/items", url.Values{
		"ingredient_id": {strconv.FormatInt(egg, 10)}, "quantity": {"2"}, "qty_unit": {"pz"},
	}); code != http.StatusSeeOther {
		t.Fatalf("add 2 pz: %d", code)
	}
	full, _ := a.Recipes.GetWithItems(ctx, rec, aliceID)
	item := full.Items[0].ID
	itemPath := page + "/items/" + strconv.FormatInt(item, 10)
	_, _, body := alice.get(page)
	for _, want := range []string{`title="120 g"`, "2 pc", ">172<", "Change the quantity of Egg:", `action="` + itemPath + `"`} {
		if !strings.Contains(body, want) {
			t.Errorf("the recipe page lacks %q", want)
		}
	}

	// Saved: back to the page with the row's quantity focused, totals follow.
	if code, loc := alice.post(page, itemPath, url.Values{"quantity": {"180"}, "qty_unit": {"g"}}); code != http.StatusSeeOther || loc != page+"?saved="+strconv.FormatInt(item, 10) {
		t.Fatalf("edit: %d to %q", code, loc)
	}
	_, _, body = alice.get(page + "?saved=" + strconv.FormatInt(item, 10))
	if !strings.Contains(body, "180 g") || !strings.Contains(body, ">257<") || !strings.Contains(body, `?edit=`+strconv.FormatInt(item, 10)+`" data-qty-open autofocus`) {
		t.Error("after the edit the page does not show 180 g, 257 kcal and the focused quantity")
	}

	// Refused like adding: the editor stays open with the value and the error.
	for _, bad := range []string{"", "0", "-1", "abc", "100001"} {
		code, _, body := alice.submit(page, itemPath, url.Values{"quantity": {bad}, "qty_unit": {"g"}})
		if code != http.StatusUnprocessableEntity || !strings.Contains(body, "Invalid quantity.") || !strings.Contains(body, `data-qty-row="`+strconv.FormatInt(item, 10)+`" data-editing`) {
			t.Errorf("quantity %q: %d, want 422 with the editor open", bad, code)
		}
	}

	// Without script, ?edit= renders the editor open; a viewer gets none.
	if _, _, body := alice.get(page + "?edit=" + strconv.FormatInt(item, 10)); !strings.Contains(body, "data-editing") {
		t.Error("?edit= does not open the editor")
	}
	if _, _, body := bob.get(page + "?edit=" + strconv.FormatInt(item, 10)); strings.Contains(body, "data-qty-editor") || strings.Contains(body, "data-editing") {
		t.Error("a viewer gets the quantity editor")
	}

	// Only the author, only rows of this recipe, only with the CSRF token.
	if code, _ := bob.post(page, itemPath, url.Values{"quantity": {"1"}}); code != http.StatusForbidden {
		t.Errorf("Bob edits Alice's public recipe: %d, want 403", code)
	}
	otherPath := "/recipes/" + strconv.FormatInt(other, 10) + "/items/" + strconv.FormatInt(item, 10)
	if code, _ := alice.post(page, otherPath, url.Values{"quantity": {"1"}}); code != http.StatusNotFound {
		t.Errorf("row of another recipe: %d, want 404", code)
	}
	if code := alice.postNoToken(itemPath, url.Values{"quantity": {"1"}}); code != http.StatusForbidden {
		t.Errorf("no CSRF token: %d, want 403", code)
	}
	full, _ = a.Recipes.GetWithItems(ctx, rec, aliceID)
	if it := full.Items[0]; it.Quantity != 180 || it.QuantityUnit != "g" {
		t.Errorf("the refused edits changed the row: %v %s", it.Quantity, it.QuantityUnit)
	}
}

// A weekly plan item's quantity is edited in place: servings for a recipe,
// quantity and unit for an ingredient; the day card and its pill come back.
func TestMealItemQuantityEditor(t *testing.T) {
	a := newTestApp(t)
	ctx := context.Background()
	alice, aliceID := photoUser(t, a, "alice@example.test")
	bob, _ := photoUser(t, a, "bob@example.test")
	weight := 60.0
	egg, _ := a.Ingredients.CreateLocal(ctx, "Egg", nil, "g", 143, 12.6, 0.7, 9.5, &aliceID, &weight, nil, nil, nil, nil, nil)
	rec, _ := a.Recipes.Create(ctx, aliceID, "Eggs", nil, 2, false)
	if _, err := a.Recipes.AddItem(ctx, rec, aliceID, egg, 4, "pz"); err != nil { // 240 g, 120 g a serving
		t.Fatal(err)
	}
	monday := db.MondayOf(time.Now())
	start := monday.Format("2006-01-02")
	plan, _ := a.WeeklyPlans.GetOrCreateForWeek(ctx, aliceID, monday)
	meal, _ := a.WeeklyPlans.AddMeal(ctx, plan.ID, aliceID, 0, "Lunch")
	recItem, _ := a.WeeklyPlans.AddItem(ctx, meal, aliceID, &rec, nil, 1, "")
	eggItem, _ := a.WeeklyPlans.AddItem(ctx, meal, aliceID, nil, &egg, 100, "g")
	week := "/?start=" + start
	path := func(id int64) string { return "/meal-items/" + strconv.FormatInt(id, 10) + "?start=" + start }

	_, _, body := alice.get(week + "&day=" + start)
	if !strings.Contains(body, `data-qty-row="`+strconv.FormatInt(recItem, 10)+`"`) || !strings.Contains(body, "Servings of Eggs") || !strings.Contains(body, "Quantity of Egg") {
		t.Error("the day card has no quantity editors")
	}

	// The recipe: 2.5 servings (a unit sent along is ignored).
	code, frag := alice.submitHX(week, path(recItem), url.Values{"quantity": {"2.5"}, "qty_unit": {"pz"}})
	if code != http.StatusOK || !strings.Contains(frag, "hx-swap-oob") || !strings.Contains(frag, "2.5 of 2 servings") {
		t.Fatalf("servings edit: %d, fragment without the new servings or the OOB pill", code)
	}
	// The egg: 2 pieces.
	if code, _ := alice.submitHX(week, path(eggItem), url.Values{"quantity": {"2"}, "qty_unit": {"pz"}}); code != http.StatusOK {
		t.Fatalf("egg edit: %d", code)
	}
	full, _ := a.WeeklyPlans.LoadFull(ctx, plan)
	items := full.Days[0][0].Items
	if items[0].Quantity != 2.5 || items[0].QuantityUnit != "g" || items[1].Quantity != 2 || items[1].QuantityUnit != "pz" {
		t.Errorf("items after the edits: %v %s, %v %s", items[0].Quantity, items[0].QuantityUnit, items[1].Quantity, items[1].QuantityUnit)
	}
	// 2.5 × 120 g + 2 × 60 g = 420 g of egg = 600.6 kcal.
	if got := macros.ForMeal(items).Kcal; !approxEq(got, 600.6) {
		t.Errorf("meal = %.1f kcal, want 600.6", got)
	}

	// The CSV export counts the pieces of the recipe; the import keeps pieces.
	_, _, csv := alice.get("/week/" + start + "/export")
	for _, want := range []string{"ricetta,Eggs,2.5,porzioni,429.0", "ingrediente,Egg,2.0,pz,171.6"} {
		if !strings.Contains(csv, want) {
			t.Errorf("export lacks %q:\n%s", want, csv)
		}
	}
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	_, _, dash := alice.get(week)
	_ = mw.WriteField("csrf_token", html.UnescapeString(csrfField.FindStringSubmatch(dash)[1]))
	fw, _ := mw.CreateFormFile("csv_file", "week.csv")
	_, _ = fw.Write([]byte("giorno,pasto,tipo,nome,quantita,unita\nLunedi,Lunch,ingrediente,Egg,3,pz\n"))
	_ = mw.Close()
	req, _ := http.NewRequest(http.MethodPost, alice.srv.URL+"/week/"+start+"/import", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("Origin", alice.srv.URL)
	if resp, err := alice.c.Do(req); err != nil || resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("import: %v %v", resp, err)
	}
	full, _ = a.WeeklyPlans.LoadFull(ctx, plan)
	if it := full.Days[0][0].Items[2]; it.Quantity != 3 || it.QuantityUnit != "pz" {
		t.Errorf("imported row: %v %s, want 3 pz", it.Quantity, it.QuantityUnit)
	}

	// Refused: invalid quantities, another user's item, no CSRF token.
	for _, bad := range []string{"", "0", "-2", "x", "100001"} {
		if code, _ := alice.post(week, path(eggItem), url.Values{"quantity": {bad}}); code != http.StatusSeeOther {
			t.Errorf("quantity %q: %d, want a redirect with the flash", bad, code)
		}
	}
	if code, _ := bob.post("/", path(eggItem), url.Values{"quantity": {"5"}}); code != http.StatusNotFound {
		t.Errorf("Bob edits Alice's item: %d, want 404", code)
	}
	if code := alice.postNoToken(path(eggItem), url.Values{"quantity": {"5"}}); code != http.StatusForbidden {
		t.Errorf("no CSRF token: %d, want 403", code)
	}
	full, _ = a.WeeklyPlans.LoadFull(ctx, plan)
	if it := full.Days[0][0].Items[1]; it.Quantity != 2 || it.QuantityUnit != "pz" {
		t.Errorf("the refused edits changed the item: %v %s", it.Quantity, it.QuantityUnit)
	}
}

func approxEq(a, b float64) bool { return a-b < 0.01 && b-a < 0.01 }
