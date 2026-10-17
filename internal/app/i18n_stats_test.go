package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/amon2126/nutreek/internal/db"
	"github.com/amon2126/nutreek/internal/domain"
)

var (
	jsLineComment = regexp.MustCompile(`(?m)^\s*//.*$`)
	reCatalogKey  = regexp.MustCompile(`\b(?:shopping|stats|weight|profile)\.[a-z_]+(?:\.[a-z_]+)*\b`)
	reURLAttr     = regexp.MustCompile(`\b(?:src|href|action)="[^"]*"`)
	reJSLiteral   = regexp.MustCompile(`"((?:[^"\\\n]|\\.)*)"|'((?:[^'\\\n]|\\.)*)'`)
	reJSIdent     = regexp.MustCompile(`^[\w.#:\-]*$`)
)

// italianMonths are the month names the templates write (#157 turns them into
// formatting that follows the language), so the English pages still show them.
var italianMonths = []string{"gennaio", "febbraio", "marzo", "aprile", "maggio", "giugno", "luglio",
	"agosto", "settembre", "ottobre", "novembre", "dicembre"}

// statsPage is one request of TestEnglishShoppingStatsWeightProfile.
type statsPage struct {
	name, method, target string
	form                 url.Values
	id                   int64        // the {id} route parameter
	as                   *domain.User // the viewer, when not Ada
	italian              string       // Italian text the page shows without the cookie
	want                 string       // English text the page must show
	status               int          // 0 = 200
	handler              http.HandlerFunc
}

// TestEnglishShoppingStatsWeightProfile renders the shopping list, the
// statistics, the weight log and the profile with the English cookie and
// finds no Italian left, charts' and profile form's texts included (#154).
// Without the cookie every page is still Italian.
func TestEnglishShoppingStatsWeightProfile(t *testing.T) {
	a := newTestApp(t)
	ctx := context.Background()

	adaID, err := a.Users.Create(ctx, "ada@example.test", "x", "Ada")
	if err != nil {
		t.Fatal(err)
	}
	eveID, err := a.Users.Create(ctx, "eve@example.test", "x", "Eve")
	if err != nil {
		t.Fatal(err)
	}

	// Ada has a complete profile, so the indices, the goals and the energy needs show.
	kcal, protein, carbs, fat, bmi, bmr := 2000.0, 120.0, 250.0, 70.0, 23.5, 1700.0
	weightKg, heightCm, birth, sex, activity := 72.0, 175.0, 1990, "M", "moderate"
	if err := a.Users.UpdateProfile(ctx, adaID, db.UserProfileInput{
		WeightKg: &weightKg, HeightCm: &heightCm, BirthYear: &birth, Sex: &sex, ActivityLevel: &activity,
		TargetKcal: &kcal, TargetProtein: &protein, TargetCarbs: &carbs, TargetFat: &fat,
		BMI: &bmi, BMR: &bmr, WaterTargetML: 2000,
	}); err != nil {
		t.Fatal(err)
	}
	ada, _ := a.Users.GetByID(ctx, adaID)
	eve, _ := a.Users.GetByID(ctx, eveID)

	// Meals in the last four weeks, with a food with a weight per piece, one
	// with a category and one without.
	pieceWeight := 60.0
	oats, err := a.Ingredients.CreateLocal(ctx, "Oats", nil, domain.UnitGram, 372, 13.5, 58.7, 7, &adaID, nil, nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	eggs, err := a.Ingredients.CreateLocal(ctx, "Eggs", nil, domain.UnitGram, 143, 12.6, 0.7, 9.5, &adaID, &pieceWeight, nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	rice, err := a.Ingredients.CreateLocal(ctx, "Rice", nil, domain.UnitGram, 360, 7, 79, 0.6, &adaID, nil, nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	catID, err := a.Categories.CreateCategory(ctx, adaID, "Breakfast")
	if err != nil {
		t.Fatal(err)
	}
	if err := a.Categories.SetIngredientCategory(ctx, adaID, oats, &catID); err != nil {
		t.Fatal(err)
	}
	thisWeek := db.MondayOf(time.Now())
	for w := 0; w < 4; w++ {
		plan, err := a.WeeklyPlans.GetOrCreateForWeek(ctx, adaID, thisWeek.AddDate(0, 0, -7*w))
		if err != nil {
			t.Fatal(err)
		}
		for day := 0; day < 5; day++ {
			meal, err := a.WeeklyPlans.AddMeal(ctx, plan.ID, adaID, day, "Lunch")
			if err != nil {
				t.Fatal(err)
			}
			for _, ing := range []struct {
				id  int64
				qty float64
			}{{oats, 150}, {eggs, 120}, {rice, 90}} {
				id := ing.id
				if _, err := a.WeeklyPlans.AddItem(ctx, meal, adaID, nil, &id, ing.qty, "g"); err != nil {
					t.Fatal(err)
				}
			}
			if day == 0 {
				if _, err := a.WeeklyPlans.AddCheat(ctx, meal, adaID, nil, &rice, 100); err != nil {
					t.Fatal(err)
				}
			}
		}
	}
	today := todayDate()
	for _, ago := range []int{0, 8, 15, 22, 29, 36, 43} {
		if _, err := a.WeightLogs.LogWeight(ctx, adaID, today.AddDate(0, 0, -ago), 75-float64(ago)/20, nil); err != nil {
			t.Fatal(err)
		}
	}
	acts, err := a.WorkoutLogs.ListActivities(ctx)
	if err != nil || len(acts) == 0 {
		t.Fatalf("workout activities: %v", err)
	}
	if _, err := a.WorkoutLogs.CreateWorkoutLog(ctx, adaID, acts[0].ID, today, 45, nil); err != nil {
		t.Fatal(err)
	}
	if err := a.WaterLogs.Log(ctx, adaID, today, 1500); err != nil {
		t.Fatal(err)
	}

	// Shopping list: items from the meals (one in the cart), items by hand
	// (one that repeats a food of the list).
	list, err := a.ShoppingLists.GetOrCreateList(ctx, adaID, thisWeek, 1)
	if err != nil {
		t.Fatal(err)
	}
	if err := a.ShoppingLists.Generate(ctx, list.ID, adaID); err != nil {
		t.Fatal(err)
	}
	full, err := a.ShoppingLists.LoadFull(ctx, list.ID, adaID)
	if err != nil || len(full.Items) == 0 {
		t.Fatalf("shopping list: %v", err)
	}
	if err := a.ShoppingLists.ToggleItem(ctx, full.Items[0].ID, adaID); err != nil {
		t.Fatal(err)
	}
	if _, err := a.ShoppingLists.AddUserItem(ctx, list.ID, adaID, "Oats", "1 kg"); err != nil {
		t.Fatal(err)
	}
	candles, err := a.ShoppingLists.AddUserItem(ctx, list.ID, adaID, "Candles", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := a.ShoppingLists.ToggleUserItem(ctx, candles, adaID); err != nil {
		t.Fatal(err)
	}
	userItems, err := a.ShoppingLists.LoadFull(ctx, list.ID, adaID)
	if err != nil || len(userItems.UserItems) == 0 {
		t.Fatalf("user items: %v", err)
	}

	// What the interface does not translate here: the months (#157), the
	// reference data (#158) and what users typed.
	data := append([]string{"Ada", "Eve", "Oats", "Eggs", "Rice", "Breakfast", "Lunch", "Candles"}, italianMonths...)

	week := thisWeek.Format("2006-01-02")
	badID := int64(999999)
	profileForm := func(over map[string]string) url.Values {
		v := url.Values{"weight_kg": {"72"}, "height_cm": {"175"}, "birth_year": {"1990"}, "sex": {"M"},
			"activity_level": {"moderate"}, "target_kcal": {"2000"}, "target_protein": {"120"},
			"target_carbs": {"250"}, "target_fat": {"70"}, "water_target_ml": {"2000"}}
		for k, s := range over {
			v.Set(k, s)
		}
		return v
	}
	weightForm := func(date, kg string) url.Values {
		return url.Values{"measured_at": {date}, "weight_kg": {kg}}
	}
	bad := func(name string, over map[string]string, want, italian string) statsPage {
		return statsPage{name: "profile, " + name, method: http.MethodPost, target: "/profile", form: profileForm(over),
			status: http.StatusUnprocessableEntity, want: want, italian: italian, handler: a.handleProfileSave}
	}

	pages := []statsPage{
		// Shopping list
		{name: "shopping list", target: "/shopping-list?week=" + week, want: "In the cart", italian: "Nel carrello", handler: a.handleShoppingList},
		{name: "shopping list, two weeks", target: "/shopping-list?week=" + week + "&weeks=2", want: "List for 2 weeks", italian: "Lista per 2 settimane", handler: a.handleShoppingList},
		{name: "shopping list, empty", target: "/shopping-list?week=" + week, as: eve, want: "No food in this", italian: "Nessun alimento", handler: a.handleShoppingList},
		{name: "shopping, unknown item", method: http.MethodPost, target: "/shopping-list/items/x/toggle", id: badID, status: http.StatusNotFound, want: "item not found", italian: "voce non trovata", handler: a.handleShoppingToggle},
		{name: "shopping, unknown user item toggle", method: http.MethodPost, target: "/shopping-list/user-items/x/toggle", id: badID, status: http.StatusNotFound, want: "item not found", italian: "voce non trovata", handler: a.handleToggleShoppingUserItem},
		{name: "shopping, unknown user item delete", method: http.MethodPost, target: "/shopping-list/user-items/x/delete", id: badID, status: http.StatusNotFound, want: "item not found", italian: "voce non trovata", handler: a.handleRemoveShoppingUserItem},
		// Statistics
		{name: "statistics", target: "/stats", want: "Week by week", italian: "Settimana per settimana", handler: a.handleStats},
		{name: "statistics, empty", target: "/stats", as: eve, want: "Nothing to show yet", italian: "Ancora niente da mostrare", handler: a.handleStats},
		{name: "tdee, bad value", method: http.MethodPost, target: "/tdee/accept", form: url.Values{"estimated_tdee": {"abc"}}, status: http.StatusBadRequest, want: "Invalid TDEE value", italian: "Valore TDEE non valido", handler: a.handleAcceptTDEESuggestion},
		// Weight
		{name: "weight", target: "/profile/peso", want: "Last weigh-in", italian: "Ultima pesata", handler: a.handleWeightPage},
		{name: "weight, 30 days", target: "/profile/peso?giorni=30", want: "30 days", italian: "30 giorni", handler: a.handleWeightPage},
		{name: "weight, one year", target: "/profile/peso?giorni=365", want: "1 year", italian: "1 anno", handler: a.handleWeightPage},
		{name: "weight, empty", target: "/profile/peso", as: eve, want: "No weigh-ins yet", italian: "Nessuna pesata ancora", handler: a.handleWeightPage},
		{name: "weight, bad date", method: http.MethodPost, target: "/profile/peso", form: weightForm("soon", "70"), status: http.StatusUnprocessableEntity, want: "Invalid date.", italian: "Data non valida.", handler: a.handleWeightLog},
		{name: "weight, bad weight", method: http.MethodPost, target: "/profile/peso", form: weightForm("2026-01-02", "heavy"), status: http.StatusUnprocessableEntity, want: "Invalid weight (range: 0.1–499 kg).", italian: "Peso non valido", handler: a.handleWeightLog},
		// Profile
		{name: "profile", target: "/profile", want: "Body data", italian: "Dati fisici", handler: a.handleProfilePage},
		{name: "profile, empty", target: "/profile", as: eve, want: "Fill in weight, height", italian: "Completa peso", handler: a.handleProfilePage},
		bad("weight", map[string]string{"weight_kg": "5"}, "Invalid weight (plausible range: 20–300 kg).", "Peso non valido"),
		bad("height", map[string]string{"height_cm": "5"}, "Invalid height (plausible range: 100–250 cm).", "Altezza non valida"),
		bad("birth year", map[string]string{"birth_year": "1800"}, "Implausible year of birth.", "Anno di nascita non plausibile"),
		bad("sex", map[string]string{"sex": "X"}, "Invalid sex.", "Sesso non valido"),
		bad("activity", map[string]string{"activity_level": "x"}, "Invalid activity level.", "Livello di attività non valido"),
		bad("kcal goal", map[string]string{"target_kcal": "lots"}, "Invalid kcal goal.", "Obiettivo Kcal non valido"),
		bad("protein goal", map[string]string{"target_protein": "lots"}, "Invalid protein goal.", "Obiettivo Proteine non valido"),
		bad("carbs goal", map[string]string{"target_carbs": "lots"}, "Invalid carbs goal.", "Obiettivo Carboidrati non valido"),
		bad("fat goal", map[string]string{"target_fat": "lots"}, "Invalid fat goal.", "Obiettivo Grassi non valido"),
		bad("water", map[string]string{"water_target_ml": "5"}, "Invalid hydration target (250–10000 ml).", "Target idratazione non valido"),
		bad("kcal too low", map[string]string{"target_kcal": "500"}, "Kcal goal too low", "Obiettivo Kcal troppo basso"),
		bad("protein over", map[string]string{"target_kcal": "1000", "target_protein": "300"}, "Protein alone exceeds the total calories (not possible).", "Le proteine da sole superano"),
		bad("fat over", map[string]string{"target_kcal": "1000", "target_protein": "10", "target_fat": "150"}, "Fat alone exceeds the total calories (not possible).", "I grassi da soli superano"),
		bad("macro sum", map[string]string{"target_kcal": "1000", "target_protein": "100", "target_carbs": "200", "target_fat": "50"}, "The macros exceed the total calories by more than 10%.", "La somma dei macro supera"),
		bad("protein too high", map[string]string{"target_kcal": "0", "target_protein": "500"}, "Unrealistic protein goal (maximum 400 g).", "Obiettivo Proteine non realistico"),
	}

	// serve runs the handler the way the router does: language, session, viewer.
	serve := func(p statsPage, lang string, cookies []*http.Cookie) *httptest.ResponseRecorder {
		method := p.method
		if method == "" {
			method = http.MethodGet
		}
		req := httptest.NewRequest(method, p.target, strings.NewReader(p.form.Encode()))
		if p.form != nil {
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		}
		if lang != "" {
			req.AddCookie(&http.Cookie{Name: langCookie, Value: lang})
		}
		for _, c := range cookies {
			req.AddCookie(c)
		}
		if p.id != 0 {
			rctx := chi.NewRouteContext()
			rctx.URLParams.Add("id", strconv.FormatInt(p.id, 10))
			req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
		}
		viewer := ada
		if p.as != nil {
			viewer = p.as
		}
		req = requestWithUser(req, viewer)
		rec := httptest.NewRecorder()
		languageMiddleware(a.Sessions.LoadAndSave(p.handler)).ServeHTTP(rec, req)
		return rec
	}

	checkEnglish := func(t *testing.T, body, want string) {
		t.Helper()
		if want != "" && !strings.Contains(body, want) {
			t.Errorf("%q missing from the English page:\n%s", want, body)
		}
		if found := italianIn(body, data); len(found) > 0 {
			t.Errorf("Italian in the English page:\n  %s", strings.Join(found, "\n  "))
		}
		// A key shown as it is means a catalog entry is missing.
		if m := reCatalogKey.FindString(reURLAttr.ReplaceAllString(body, "")); m != "" {
			t.Errorf("a catalog key shows as text: %s", m)
		}
	}

	for _, p := range pages {
		t.Run(p.name, func(t *testing.T) {
			status := p.status
			if status == 0 {
				status = http.StatusOK
			}
			en := serve(p, "en", nil)
			if en.Code != status {
				t.Fatalf("status %d, want %d:\n%s", en.Code, status, en.Body.String())
			}
			checkEnglish(t, en.Body.String(), p.want)

			it := serve(p, "", nil)
			if it.Code != status {
				t.Fatalf("Italian status %d, want %d", it.Code, status)
			}
			if !strings.Contains(it.Body.String(), p.italian) {
				t.Errorf("%q missing from the Italian page:\n%s", p.italian, it.Body.String())
			}
		})
	}

	// The fixtures reach the branches of the templates that hold text.
	t.Run("coverage", func(t *testing.T) {
		for target, words := range map[string][]string{
			"/shopping-list?week=" + week: {"also in the list", "pcs", "Other", "Added by hand", "Hide", "Print", "Remove Candles"},
			"/stats": {"Estimated needs", "Set the goal to", "Adherence by meal", "Workouts", "Cheats", "Water",
				"Show all 12", "You need about", "From the formula", "Current goal", "weigh-ins"},
			"/profile/peso": {"Show all", "Delete the weigh-in of", "in total", "Chart range"},
			"/profile":      {"Normal weight", "with moderate activity", "Basal metabolic rate", "kcal per day"},
		} {
			var h http.HandlerFunc
			switch {
			case strings.HasPrefix(target, "/shopping-list"):
				h = a.handleShoppingList
			case target == "/stats":
				h = a.handleStats
			case target == "/profile/peso":
				h = a.handleWeightPage
			default:
				h = a.handleProfilePage
			}
			body := serve(statsPage{target: target, handler: h}, "en", nil).Body.String()
			for _, w := range words {
				if !strings.Contains(body, w) {
					t.Errorf("%s: %q missing from the English page", target, w)
				}
			}
		}
	})

	// Flash messages travel as keys in the session and show on the next page.
	flashes := []struct {
		name          string
		post, then    statsPage
		want, italian string
	}{
		{name: "shopping, description missing",
			post: statsPage{method: http.MethodPost, target: "/shopping-list/" + strconv.FormatInt(list.ID, 10) + "/user-items", id: list.ID, form: url.Values{"description": {" "}}, handler: a.handleAddShoppingUserItem},
			then: statsPage{target: "/shopping-list?week=" + week, handler: a.handleShoppingList},
			want: "Description required.", italian: "Descrizione obbligatoria."},
		{name: "shopping, description too long",
			post: statsPage{method: http.MethodPost, target: "/shopping-list/x/user-items", id: list.ID, form: url.Values{"description": {strings.Repeat("a", 300)}}, handler: a.handleAddShoppingUserItem},
			then: statsPage{target: "/shopping-list?week=" + week, handler: a.handleShoppingList},
			want: "Description too long.", italian: "Descrizione troppo lunga."},
		{name: "shopping, regenerated",
			post: statsPage{method: http.MethodPost, target: "/shopping-list/generate?week=" + week, handler: a.handleShoppingGenerate},
			then: statsPage{target: "/shopping-list?week=" + week, handler: a.handleShoppingList},
			want: "List regenerated.", italian: "Lista rigenerata."},
		{name: "weight saved",
			post: statsPage{method: http.MethodPost, target: "/profile/peso", form: weightForm(today.Format("2006-01-02"), "71,5"), handler: a.handleWeightLog},
			then: statsPage{target: "/profile/peso", handler: a.handleWeightPage},
			want: "Weigh-in saved.", italian: "Pesata salvata."},
		{name: "weight deleted",
			post: statsPage{method: http.MethodPost, target: "/profile/peso/x/delete", id: -1, handler: a.handleWeightLogDelete},
			then: statsPage{target: "/profile/peso", handler: a.handleWeightPage},
			want: "Weigh-in deleted.", italian: "Pesata eliminata."},
		{name: "profile saved",
			post: statsPage{method: http.MethodPost, target: "/profile", form: profileForm(nil), handler: a.handleProfileSave},
			then: statsPage{target: "/profile", handler: a.handleProfilePage},
			want: "Profile saved.", italian: "Profilo salvato."},
		{name: "energy needs accepted",
			post: statsPage{method: http.MethodPost, target: "/tdee/accept", form: url.Values{"estimated_tdee": {"2350"}}, handler: a.handleAcceptTDEESuggestion},
			then: statsPage{target: "/stats", handler: a.handleStats},
			want: "Calorie goal updated to 2350 kcal/day.", italian: "Target calorico aggiornato a 2350 kcal/giorno."},
	}
	for _, f := range flashes {
		t.Run("flash, "+f.name, func(t *testing.T) {
			for _, lang := range []string{"en", ""} {
				req := f.post
				if req.id == -1 { // a weigh-in that still exists, once per language
					req.id = mustWeightID(t, a, adaID)
				}
				post := serve(req, lang, nil)
				if post.Code != http.StatusSeeOther {
					t.Fatalf("%s: status %d, want a redirect:\n%s", lang, post.Code, post.Body.String())
				}
				rec := serve(f.then, lang, post.Result().Cookies())
				if lang == "en" {
					checkEnglish(t, rec.Body.String(), f.want)
				} else if !strings.Contains(rec.Body.String(), f.italian) {
					t.Errorf("%q missing from the Italian page:\n%s", f.italian, rec.Body.String())
				}
			}
		})
	}

	// The texts the scripts show come from the page, not from the scripts.
	t.Run("scripts hold no Italian", func(t *testing.T) {
		for _, name := range []string{"charts.js", "profile.js"} {
			src, err := os.ReadFile("../web/static/js/" + name)
			if err != nil {
				t.Fatal(err)
			}
			var found []string
			for _, m := range reJSLiteral.FindAllStringSubmatch(jsLineComment.ReplaceAllString(string(src), ""), -1) {
				// Ids, class names and the "it-IT" number format (#157) are not text.
				if lit := m[1] + m[2]; !reJSIdent.MatchString(lit) {
					found = append(found, italianIn(lit, nil)...)
				}
			}
			if len(found) > 0 {
				t.Errorf("Italian in %s:\n  %s", name, strings.Join(found, "\n  "))
			}
		}
	})

	// The profile form and the charts carry their texts as data-msg-* in each language.
	t.Run("script texts", func(t *testing.T) {
		profileEN := serve(statsPage{target: "/profile", handler: a.handleProfilePage}, "en", nil).Body.String()
		for _, w := range []string{`data-msg-bmi-obese="Obesity"`, `data-msg-suggested="suggested: {value}"`,
			`data-msg-macro-over="The macros add up to {kcal} kcal: {diff} above the goal."`,
			`data-msg-err-fat-low="Value too low (min 20 g for essential fats)."`} {
			if !strings.Contains(profileEN, w) {
				t.Errorf("%s missing from the English profile", w)
			}
		}
		profileIT := serve(statsPage{target: "/profile", handler: a.handleProfilePage}, "", nil).Body.String()
		if !strings.Contains(profileIT, `data-msg-bmi-obese="Obesità"`) {
			t.Error("Italian BMI category missing from the profile")
		}
		statsEN := serve(statsPage{target: "/stats", handler: a.handleStats}, "en", nil).Body.String()
		for _, w := range []string{`data-msg-no-data="no data"`, `data-msg-total="Total"`, `data-msg-per-day="Per day"`, `data-msg-macro="Calories and macros per day"`} {
			if !strings.Contains(statsEN, w) {
				t.Errorf("%s missing from the English statistics", w)
			}
		}
		weightIT := serve(statsPage{target: "/profile/peso", handler: a.handleWeightPage}, "", nil).Body.String()
		if !strings.Contains(weightIT, `data-msg-no-data="nessun dato"`) {
			t.Error("Italian chart texts missing from the weight page")
		}
	})

	// The tiles and the trend are built in Go, in the request's language.
	t.Run("tiles and trend", func(t *testing.T) {
		in := statsFixture()
		in.Lang = "en"
		got := buildStatsView(in).Tiles
		want := []statsTile{
			{Label: "Daily average", Value: "1.917", Unit: "kcal", Sub: "goal 2.100"},
			{Label: "Weight", Value: "72,4", Unit: "kg", Sub: "−1,6 kg in 2 weeks"},
			{Label: "Workouts", Value: "2", Unit: "per week", Sub: "600 kcal on average"},
			{Label: "Cheats", Value: "1,5", Unit: "per week", Sub: "6 days without"},
			{Label: "Water", Value: "2", Unit: "l per day", Sub: "goal 2 l"},
		}
		for i := range want {
			if got[i] != want[i] {
				t.Errorf("tile %d = %+v, want %+v", i, got[i], want[i])
			}
		}
		if s := weightTrend("en", 0.4, 5*24*time.Hour); s != "+0,4 kg in 5 days" {
			t.Errorf("trend = %q", s)
		}
		if s := weightTrend("en", 0.01, 0); s != "steady" {
			t.Errorf("trend = %q", s)
		}
	})
}

// mustWeightID returns the id of one of the user's weigh-ins.
func mustWeightID(t *testing.T, a *App, userID int64) int64 {
	t.Helper()
	logs, err := a.WeightLogs.ListWeightLogs(context.Background(), userID, 365)
	if err != nil || len(logs) == 0 {
		t.Fatalf("weight logs: %v", err)
	}
	return logs[len(logs)-1].ID
}
