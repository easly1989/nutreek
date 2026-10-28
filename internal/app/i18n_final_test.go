package app

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/amon2126/nutreek/internal/db"
	"github.com/amon2126/nutreek/internal/domain"
	"github.com/amon2126/nutreek/internal/i18n"
	"github.com/amon2126/nutreek/internal/off"
)

// notHTML lists the GET routes that do not return a page, and why. Every other
// GET route of the router must have a page in TestEnglishEveryPage, so a new
// route cannot slip through without being checked (#160).
//
// The REST API (/api/v1/*) is JSON in English by design and skipped as a whole.
var notHTML = map[string]string{
	"/static/*":                 "static files",
	"/sw.js":                    "the service worker script",
	"/healthz":                  "plain \"ok\" for the health check",
	"/week/{start}/export":      "CSV file; its Italian day names are a file format, see the week import",
	"/settings/export":          "JSON file of the user's data",
	"/recipes/{id}/photo":       "the recipe's JPEG photo",
	"/recipes/{id}/photo/thumb": "the recipe's JPEG thumbnail",
	"/profile/off-status":       "JSON for the sync card script",
	"/admin/off-status":         "JSON for scripts, behind X-Admin-Token",
}

var reJSBlockComment = regexp.MustCompile(`(?s)/\*.*?\*/`)

// reCatalogToken finds things shaped like a catalog key: words joined by dots.
var reCatalogToken = regexp.MustCompile(`\b[a-z][a-z0-9_]*(?:\.[a-z0-9_]+)+\b`)

// catalogKeysIn returns the catalog keys a page shows as text: a key shown as
// it is means the catalog lacks it. Anything that starts with the name of a
// catalog area ("week.", "stats.") counts, so a mistyped key is found too.
func catalogKeysIn(page string, areas map[string]bool) []string {
	var found []string
	for _, c := range pageChunks(page) {
		for _, tok := range reCatalogToken.FindAllString(html.UnescapeString(c), -1) {
			if areas[tok[:strings.Index(tok, ".")]] {
				found = append(found, tok)
			}
		}
	}
	return found
}

var reLetters3 = regexp.MustCompile(`\p{L}{3,}`)

// sameInBothOK are the words of the interface that are the same in English and
// Italian: brands, units, and words Italian borrows.
var sameInBothOK = map[string]bool{
	"nutreek": true, "kcal": true, "min": true, "openfoodfacts": true, "italiano": true, "english": true,
	"premium": true, "account": true, "template": true, "password": true, "email": true, "coach": true,
	"bmi": true, "free": true, "per": true, "true": true, "enabled": true,
}

// sameInBoth returns the pieces of text the English and the Italian page share
// that hold a word other than sameInBothOK: Italian text left in the English
// page is also in the Italian one, whether or not italianIn knows its words.
func sameInBoth(en, it string, data []string) []string {
	clean := func(c string) string {
		c = strings.TrimSpace(html.UnescapeString(c))
		for _, d := range data {
			c = strings.ReplaceAll(c, d, "")
		}
		return strings.TrimSpace(c)
	}
	inIt := map[string]bool{}
	for _, c := range pageTextChunks(it) {
		inIt[clean(c)] = true
	}
	var same []string
	for _, c := range pageTextChunks(en) {
		if c = clean(c); !inIt[c] || slices.Contains(same, c) {
			continue
		}
		for _, w := range reLetters3.FindAllString(c, -1) {
			if !sameInBothOK[strings.ToLower(w)] {
				same = append(same, c)
				break
			}
		}
	}
	return same
}

// finalPage is one request of TestEnglishEveryPage.
type finalPage struct {
	route    string // the router's pattern this page covers; "" for an extra page
	target   string
	as       string // ada (default), eve, cat, dan, bob, out (signed out)
	htmx     bool
	hxTarget string
	premium  bool // NUTREEK_PREMIUM_ENABLED
	status   int  // 0 = 200
	redirect string
}

// TestEnglishEveryPage signs in through the router and opens every page of the
// app with the English cookie (#160). It fails on a word or an accent that
// gives Italian away, on a catalog key shown as text, and on a GET route that
// no page covers. Pages are built from a seeded user so that the lists are
// full, plus accounts for the empty states, a free user, a coach's client and
// a signed-out visitor.
func TestEnglishEveryPage(t *testing.T) {
	a := newTestApp(t)
	a.OFF = off.NewClient(a.Store.DB, "http://127.0.0.1:1", nil)
	a.OFFSyncer = off.NewSyncer(a.Store.DB, "http://127.0.0.1:1", "http://127.0.0.1:1", nil)
	ctx := context.Background()
	t.Cleanup(invalidateFeatureCache)
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	id := func(v int64, err error) int64 {
		t.Helper()
		must(err)
		return v
	}
	idOf := func(v int64) string { return fmt.Sprint(v) }

	// ---- users
	ada := mustCreateTestUser(t, a, "ada@example.test", "x") // the owner and a coach
	eve := mustCreateTestUser(t, a, "eve@example.test", "x") // nothing at all
	cat := mustCreateTestUser(t, a, "cat@example.test", "x") // a client with the coach's plan
	bob := mustCreateTestUser(t, a, "bob@example.test", "x") // another author, with Premium
	dan := mustCreateTestUser(t, a, "dan@example.test", "x") // a client without a plan
	a.Cfg.OwnerEmail = ada.Email
	must(a.Users.UpdateRole(ctx, ada.ID, domain.RoleCoach))
	for _, u := range []*domain.User{ada, cat, bob, dan} {
		must(a.Users.CompleteOnboarding(ctx, u.ID))
		if u != ada {
			must(a.Users.DismissTour(ctx, u.ID))
		}
	}
	kcal, protein, carbs, fat, bmi, bmr := 2000.0, 120.0, 250.0, 70.0, 23.5, 1700.0
	weightKg, heightCm, birth, sex, activity := 72.0, 175.0, 1990, "M", "moderate"
	must(a.Users.UpdateProfile(ctx, ada.ID, db.UserProfileInput{
		WeightKg: &weightKg, HeightCm: &heightCm, BirthYear: &birth, Sex: &sex, ActivityLevel: &activity,
		TargetKcal: &kcal, TargetProtein: &protein, TargetCarbs: &carbs, TargetFat: &fat,
		BMI: &bmi, BMR: &bmr, WaterTargetML: 2000,
	}))
	soon := time.Now().AddDate(0, 0, 10)
	must(a.Subscriptions.Grant(ctx, bob.ID, "manual", &soon, "", ""))
	must(a.Subscriptions.Grant(ctx, dan.ID, "manual", nil, "", ""))
	if _, err := a.APITokens.Create(ctx, ada.ID, "Home Assistant"); err != nil {
		t.Fatal(err)
	}

	// ---- ingredients and categories
	pieceWeight := 60.0
	brand := "Mill"
	oats := id(a.Ingredients.CreateLocal(ctx, "Oats", &brand, domain.UnitGram, 372, 13.5, 58.7, 7, &ada.ID, nil, nil, nil, nil, nil, nil))
	eggs := id(a.Ingredients.CreateLocal(ctx, "Eggs", nil, domain.UnitGram, 143, 12.6, 0.7, 9.5, &ada.ID, &pieceWeight, nil, nil, nil, nil, nil))
	rice := id(a.Ingredients.CreateLocal(ctx, "Rice", nil, domain.UnitGram, 360, 7, 79, 0.6, &bob.ID, nil, nil, nil, nil, nil, nil))
	flakes := id(a.Ingredients.CreateOFF(ctx, "8012345678907", "Oat flakes", nil, domain.UnitGram, 372, 13.5, 58.7, 7, &ada.ID, nil, nil, nil, nil, nil, nil))
	must(a.Ingredients.SetLocalBarcode(ctx, oats, "8000000000002"))
	breakfastCat := id(a.Categories.CreateCategory(ctx, ada.ID, "Breakfast"))
	must(a.Categories.SetIngredientCategory(ctx, ada.ID, oats, &breakfastCat))
	allergens, err := a.Categories.ListAllergens(ctx)
	if err != nil || len(allergens) == 0 {
		t.Fatalf("allergens: %v", err)
	}
	must(a.Categories.SetIngredientAllergens(ctx, oats, []int64{allergens[0].ID}))
	if _, err := a.Store.DB.Exec(`INSERT INTO off_products (barcode, product_name, brands, quantity,
		kcal_per_100, protein_per_100, carbs_per_100, fat_per_100, has_full_nutrients, updated_at) VALUES
		('8012345678907', 'Oat flakes', 'Farm', '500 g', 372, 13.5, 58.7, 7, 1, 0),
		('8015555555556', 'Oat bran', 'Farm', '500 g', 0, 0, 0, 0, 0, 0)`); err != nil {
		t.Fatal(err)
	}

	// ---- recipes
	desc := "Soak the oats."
	porridge := id(a.Recipes.Create(ctx, ada.ID, "Porridge", &desc, 2, true))
	id(a.Recipes.AddItem(ctx, porridge, ada.ID, oats, 100, "g"))
	duo := id(a.Recipes.Create(ctx, ada.ID, "Duo", nil, 1, false))
	id(a.Recipes.AddItem(ctx, duo, ada.ID, oats, 100, "g"))
	id(a.Recipes.AddItem(ctx, duo, ada.ID, rice, 50, "g"))
	bowl := id(a.Recipes.Create(ctx, ada.ID, "Bowl", nil, 1, false))
	draft := id(a.Recipes.Duplicate(ctx, duo, ada.ID, "Duo copy"))
	theirs := id(a.Recipes.Create(ctx, bob.ID, "Pasta", &desc, 4, true))

	// ---- templates and the nutrition plan
	standard := id(a.MealTemplates.Create(ctx, ada.ID, "Standard week"))
	away := id(a.MealTemplates.Create(ctx, ada.ID, "Away from home"))
	must(a.MealTemplates.SetActive(ctx, standard, ada.ID))
	id(a.MealTemplates.AddSlot(ctx, standard, ada.ID, domain.DayOfWeekEvery, "Breakfast"))
	id(a.MealTemplates.AddSlot(ctx, standard, ada.ID, domain.DayOfWeekEvery, "Dinner"))
	id(a.MealTemplates.AddSlot(ctx, standard, ada.ID, 2, "Snack"))
	plan, err := a.NutritionistPlans.GetOrCreateActivePlan(ctx, ada.ID)
	must(err)
	must(a.NutritionistPlans.EnsureMealType(ctx, plan.ID, "Breakfast"))
	must(a.NutritionistPlans.EnsureMealType(ctx, plan.ID, "Dinner"))
	full, err := a.NutritionistPlans.LoadPlanFull(ctx, ada.ID)
	must(err)
	mainVar := id(a.NutritionistPlans.CreateVariation(ctx, plan.ID, full.MealTypes[0].ID, nil))
	light := "Light"
	lightVar := id(a.NutritionistPlans.CreateVariation(ctx, plan.ID, full.MealTypes[0].ID, &light))
	id(a.NutritionistPlans.AddItem(ctx, plan.ID, mainVar, &porridge, nil, 1, "g"))
	id(a.NutritionistPlans.AddItem(ctx, plan.ID, mainVar, nil, &oats, 50, "g"))
	id(a.NutritionistPlans.AddItem(ctx, plan.ID, lightVar, nil, &eggs, 2, "pz"))

	// ---- the coach: a client with a plan, one without
	coachPlan := id(a.NutritionistPlans.CreatePlan(ctx, ada.ID, "Coach plan"))
	lunchType := id(a.NutritionistPlans.CreateMealType(ctx, coachPlan, "Lunch"))
	coachVar := id(a.NutritionistPlans.CreateVariation(ctx, coachPlan, lunchType, nil))
	id(a.NutritionistPlans.AddItem(ctx, coachPlan, coachVar, nil, &oats, 80, "g"))
	must(a.Coach.AddClient(ctx, ada.ID, cat.ID))
	must(a.Coach.AssignPlan(ctx, ada.ID, cat.ID, coachPlan))
	must(a.Coach.AddClient(ctx, ada.ID, dan.ID))

	// ---- the week: this one with meals, cheats, a customized recipe; three
	// before it for the statistics
	mon := db.MondayOf(todayDate())
	today := (int(time.Now().Weekday()) + 6) % 7
	todayStr := formatDate(mon.AddDate(0, 0, today))
	otherStr := formatDate(mon.AddDate(0, 0, (today+1)%7))
	thisWeek, nextWeek := formatDate(mon), formatDate(mon.AddDate(0, 0, 7))
	var breakfast, lunch, soupItem, customItem int64
	for w := 0; w < 4; w++ {
		wp, err := a.WeeklyPlans.GetOrCreateForWeek(ctx, ada.ID, mon.AddDate(0, 0, -7*w))
		must(err)
		for day := 0; day < 5; day++ {
			meal := id(a.WeeklyPlans.AddMeal(ctx, wp.ID, ada.ID, day, "Lunch"))
			for _, ing := range []struct {
				id  int64
				qty float64
			}{{oats, 150}, {eggs, 120}, {rice, 90}} {
				id(a.WeeklyPlans.AddItem(ctx, meal, ada.ID, nil, &ing.id, ing.qty, "g"))
			}
			if day == 0 {
				id(a.WeeklyPlans.AddCheat(ctx, meal, ada.ID, nil, &rice, 100))
			}
		}
		if w == 0 {
			lunch = id(a.WeeklyPlans.AddMeal(ctx, wp.ID, ada.ID, today, "Lunch"))
			id(a.WeeklyPlans.AddItem(ctx, lunch, ada.ID, nil, &rice, 100, "g"))
			id(a.WeeklyPlans.AddCheat(ctx, lunch, ada.ID, &porridge, nil, 1))
			breakfast = id(a.WeeklyPlans.AddMeal(ctx, wp.ID, ada.ID, today, "Breakfast"))
			id(a.WeeklyPlans.AddItem(ctx, breakfast, ada.ID, nil, &oats, 80, "g"))
			id(a.WeeklyPlans.AddItem(ctx, breakfast, ada.ID, nil, &eggs, 2, "pz"))
			soupItem = id(a.WeeklyPlans.AddItem(ctx, breakfast, ada.ID, &porridge, nil, 2, "g"))
			dinner := id(a.WeeklyPlans.AddMeal(ctx, wp.ID, ada.ID, today, "Dinner"))
			customItem = id(a.WeeklyPlans.AddItem(ctx, dinner, ada.ID, &duo, nil, 1, "g"))
			must(a.MealItemCustomizations.Replace(ctx, customItem, ada.ID, []db.CustomizationInput{{IngredientID: oats, Quantity: 50}}))
		}
	}
	for _, ago := range []int{0, 8, 15, 22, 29, 36, 43} {
		id(a.WeightLogs.LogWeight(ctx, ada.ID, todayDate().AddDate(0, 0, -ago), 75-float64(ago)/20, nil))
	}
	acts, err := a.WorkoutLogs.ListActivities(ctx)
	if err != nil || len(acts) == 0 {
		t.Fatalf("workout activities: %v", err)
	}
	id(a.WorkoutLogs.CreateWorkoutLog(ctx, ada.ID, acts[0].ID, todayDate(), 45, nil))
	must(a.WaterLogs.Log(ctx, ada.ID, todayDate(), 1500))

	// ---- the shopping list
	list, err := a.ShoppingLists.GetOrCreateList(ctx, ada.ID, mon, 1)
	must(err)
	must(a.ShoppingLists.Generate(ctx, list.ID, ada.ID))
	listFull, err := a.ShoppingLists.LoadFull(ctx, list.ID, ada.ID)
	if err != nil || len(listFull.Items) == 0 {
		t.Fatalf("shopping list: %v", err)
	}
	must(a.ShoppingLists.ToggleItem(ctx, listFull.Items[0].ID, ada.ID))
	id(a.ShoppingLists.AddUserItem(ctx, list.ID, ada.ID, "Candles", "2"))

	// What users typed, the names of reference data (#158) and of months and
	// days (#157), and the OpenFoodFacts products are not interface text.
	data := []string{"Ada", "Eve", "Cat", "Bob", "Dan", "Oats", "Eggs", "Rice", "Mill", "Oat flakes", "Oat bran", "Farm",
		"Breakfast", "Lunch", "Dinner", "Snack", "Porridge", "Duo copy", "Duo", "Bowl", "Pasta", "Soak the oats.",
		"Standard week", "Away from home", "Light", "Coach plan", "Candles", "Home Assistant", "Utente Test",
		"Lunedì", "Martedì", "Mercoledì", "Giovedì", "Venerdì", "Sabato", "Domenica",
		"Lun", "Mar", "Mer", "Gio", "Ven", "Sab", "Dom"}
	data = append(data, italianMonths...)
	for _, al := range allergens {
		data = append(data, al.Name)
	}
	for _, ac := range acts {
		data = append(data, ac.Name)
	}
	features, err := a.Features.List(ctx)
	if err != nil || len(features) == 0 {
		t.Fatalf("features: %v", err)
	}
	for _, f := range features { // the key is an identifier the admin panel prints
		data = append(data, f.Name, f.Description, f.Key)
	}
	for _, u := range []*domain.User{ada, eve, cat, bob, dan} {
		data = append(data, u.Email)
	}
	must(a.Features.SetPremium(ctx, features[0].Key, true))

	// ---- sessions: a signed-in cookie for each viewer
	viewers := map[string]*domain.User{"ada": ada, "eve": eve, "cat": cat, "dan": dan, "bob": bob}
	sessions := map[string]*http.Cookie{}
	for name, u := range viewers {
		sctx, err := a.Sessions.Load(ctx, "")
		must(err)
		a.Sessions.Put(sctx, sessionKeyUserID, u.ID)
		token, _, err := a.Sessions.Commit(sctx)
		must(err)
		sessions[name] = &http.Cookie{Name: a.Sessions.Cookie.Name, Value: token}
	}

	// ---- the pages
	itemsNew := "/meals/" + idOf(breakfast) + "/items/new?start=" + thisWeek
	pickerQ := func(meal int64, q string) string {
		return "/meals/" + idOf(meal) + "/picker?" + q + "&start=" + thisWeek
	}
	pages := []finalPage{
		// Public and error pages.
		{route: "/", target: "/", as: "out"},
		{route: "/login", target: "/login", as: "out"},
		{route: "/register", target: "/register", as: "out"},
		{route: "/manifest.webmanifest", target: "/manifest.webmanifest", as: "out"},
		{target: "/manifest.webmanifest"},
		{target: "/nowhere", as: "out", status: http.StatusNotFound},
		{target: "/nowhere", status: http.StatusNotFound},
		{target: "/recipes/99999", status: http.StatusNotFound},
		{target: "/ingredients/99999/edit", status: http.StatusNotFound},
		{target: "/templates/99999", status: http.StatusNotFound},
		{target: "/coach", as: "eve", status: http.StatusNotFound},
		{target: "/admin/panel", as: "eve", status: http.StatusNotFound},

		// The week.
		{route: "/", target: "/"},
		{target: "/?start=" + thisWeek + "&day=" + otherStr},
		{target: "/?start=" + nextWeek},
		{target: "/", as: "eve"},
		{target: "/", as: "eve", premium: true},
		{target: "/", as: "cat"},
		{route: "/days/{date}", target: "/days/" + todayStr, htmx: true},
		{target: "/days/" + otherStr, htmx: true},
		{target: "/days/" + todayStr, as: "cat", htmx: true},
		{route: "/meals/{mealID}/items/new", target: itemsNew, htmx: true},
		{target: itemsNew},
		{target: "/meals/" + idOf(lunch) + "/items/new?cheat=1&start=" + thisWeek, htmx: true},
		{target: "/meals/" + idOf(lunch) + "/items/new?cheat=1&start=" + thisWeek},
		{route: "/meals/{mealID}/picker", target: pickerQ(breakfast, "kind=ingredient&q=&meal_name=Breakfast&dialog=1")},
		{target: pickerQ(breakfast, "kind=ingredient&q=o&off=1")},
		{target: pickerQ(breakfast, "kind=ingredient&q=egg")},
		{target: pickerQ(breakfast, "kind=ingredient&q=zzz")},
		{target: pickerQ(breakfast, "kind=ingredient&q=8012345678907&off=1")},
		{target: pickerQ(lunch, "kind=ingredient&q=oat&as=cheat")},
		{target: pickerQ(breakfast, "kind=recipe&q=")},
		{target: pickerQ(breakfast, "kind=recipe&q=pie")},
		{target: pickerQ(lunch, "kind=recipe&q=&as=cheat")},
		{target: pickerQ(breakfast, "kind=recipe&q=zzz")},
		{route: "/meal-items/{itemID}/customize", target: "/meal-items/" + idOf(soupItem) + "/customize?start=" + thisWeek, htmx: true},
		{target: "/meal-items/" + idOf(customItem) + "/customize?start=" + thisWeek, htmx: true},

		// Ingredients.
		{route: "/ingredients", target: "/ingredients"},
		{target: "/ingredients?q=oat"},
		{target: "/ingredients?q=zzz"},
		{target: "/ingredients?src=mine&allergen=" + idOf(allergens[0].ID) + "&category=" + idOf(breakfastCat)},
		{target: "/ingredients", as: "eve"},
		{route: "/ingredients/search", target: "/ingredients/search?q=oat&off=1", htmx: true},
		{route: "/ingredients/new", target: "/ingredients/new"},
		{route: "/ingredients/{id}/edit", target: "/ingredients/" + idOf(oats) + "/edit"},
		{target: "/ingredients/" + idOf(rice) + "/edit"},
		{target: "/ingredients/" + idOf(flakes) + "/edit"},
		{route: "/quick/ingredient", target: "/quick/ingredient", htmx: true},
		{target: "/quick/ingredient?barcode=8000000000019", htmx: true},
		{route: "/quick/recipe", target: "/quick/recipe", htmx: true},

		// Recipes.
		{route: "/recipes", target: "/recipes"},
		{target: "/recipes?f=others"},
		{target: "/recipes?f=used"},
		{target: "/recipes?f=public"},
		{target: "/recipes?q=zzz"},
		{target: "/recipes", as: "eve"},
		{target: "/recipes?q=po", htmx: true, hxTarget: "recipe-results"},
		{route: "/recipes/new", target: "/recipes/new"},
		{route: "/recipes/{id}", target: "/recipes/" + idOf(porridge)},
		{target: "/recipes/" + idOf(bowl)},
		{target: "/recipes/" + idOf(duo)},
		{target: "/recipes/" + idOf(draft)},
		{target: "/recipes/" + idOf(theirs)},
		{route: "/recipes/{id}/picker", target: "/recipes/" + idOf(bowl) + "/picker?q=oat&off=1"},
		{target: "/recipes/" + idOf(theirs) + "/picker", status: http.StatusForbidden},
		{route: "/recipes/{id}/duplicate", target: "/recipes/" + idOf(porridge) + "/duplicate", htmx: true},

		// The nutrition plan and the templates.
		{route: "/piano-nutrizionale", target: "/piano-nutrizionale"},
		{target: "/piano-nutrizionale?tab=template"},
		{target: "/piano-nutrizionale", as: "eve"},
		{target: "/piano-nutrizionale?tab=template", as: "eve"},
		{target: "/piano-nutrizionale", as: "cat"},
		{route: "/nutrizionista", target: "/nutrizionista", status: http.StatusFound, redirect: "/piano-nutrizionale"},
		{route: "/nutrizionista/picker", target: "/nutrizionista/picker?variation_id=" + idOf(mainVar) + "&kind=ingredient&q=oat&off=1"},
		{target: "/nutrizionista/picker?variation_id=" + idOf(mainVar) + "&kind=recipe&q="},
		{route: "/templates", target: "/templates", status: http.StatusSeeOther, redirect: "/piano-nutrizionale?tab=template"},
		{route: "/templates/new", target: "/templates/new"},
		{route: "/templates/{id}", target: "/templates/" + idOf(standard)},
		{target: "/templates/" + idOf(away)},

		// Body, statistics, shopping.
		{route: "/profile", target: "/profile"},
		{target: "/profile", as: "eve"},
		{route: "/profile/peso", target: "/profile/peso"},
		{target: "/profile/peso?giorni=30"},
		{target: "/profile/peso?giorni=365"},
		{target: "/profile/peso", as: "eve"},
		{route: "/stats", target: "/stats"},
		{target: "/stats", as: "eve"},
		{target: "/stats", premium: true},
		{target: "/stats", as: "eve", premium: true},
		{route: "/shopping-list", target: "/shopping-list?week=" + thisWeek},
		{target: "/shopping-list?week=" + thisWeek + "&weeks=2"},
		{target: "/shopping-list?week=" + thisWeek, as: "eve"},

		// Onboarding, account, premium.
		{route: "/onboarding", target: "/onboarding", as: "eve"},
		{target: "/onboarding?step=2", as: "eve"},
		{target: "/onboarding?step=3", as: "eve"},
		{target: "/onboarding?step=4", as: "eve"},
		{target: "/onboarding?step=5", as: "eve"},
		{target: "/onboarding?restart=true"},
		{target: "/onboarding?restart=true&step=4"},
		{route: "/upgrade", target: "/upgrade"},
		{target: "/upgrade", as: "eve", premium: true},
		{target: "/upgrade?da=" + features[0].Key, as: "eve", premium: true},
		{route: "/settings", target: "/settings"},
		{target: "/settings?ok=password"},
		{target: "/settings", as: "eve"},
		{target: "/settings", as: "eve", premium: true},
		{target: "/upgrade", as: "dan", premium: true},
		{target: "/settings", as: "dan", premium: true},
		{target: "/settings", as: "bob", premium: true},
		{target: "/piano-nutrizionale", as: "dan"},

		// Administration and coach.
		{route: "/admin/panel", target: "/admin/panel"},
		{target: "/admin/panel", premium: true},
		{route: "/admin/panel/users", target: "/admin/panel/users"},
		{target: "/admin/panel/users", premium: true},
		{route: "/admin/panel/features", target: "/admin/panel/features", status: http.StatusFound, redirect: "/admin/panel#funzionalita"},
		{route: "/coach", target: "/coach"},
	}

	// ---- every GET route of the router has a page, or a reason not to
	router := a.Router()
	var routes []string
	if err := chi.Walk(router.(chi.Routes), func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		if method == http.MethodGet && !strings.HasPrefix(route, "/api/v1/") && !slices.Contains(routes, route) {
			routes = append(routes, route)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	covered := map[string]bool{}
	for _, p := range pages {
		if p.route != "" {
			covered[p.route] = true
		}
	}
	for _, r := range routes {
		_, skipped := notHTML[r]
		switch {
		case skipped && covered[r]:
			t.Errorf("GET %s is skipped and has a page", r)
		case !skipped && !covered[r]:
			t.Errorf("GET %s has no page in TestEnglishEveryPage: add one, or add it to notHTML with the reason", r)
		}
	}
	for r := range notHTML {
		if !slices.Contains(routes, r) {
			t.Errorf("notHTML lists %s, which is not a GET route", r)
		}
	}
	for r := range covered {
		if !slices.Contains(routes, r) {
			t.Errorf("a page covers %s, which is not a GET route", r)
		}
	}

	// ---- the catalog areas: a token that starts with one of them is a key
	areas := map[string]bool{}
	for _, k := range i18n.Keys("en") {
		areas[k[:strings.Index(k, ".")]] = true
	}
	if len(areas) < 10 {
		t.Fatalf("found only %d catalog areas", len(areas))
	}

	get := func(p finalPage, lang string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, p.target, nil)
		req.AddCookie(&http.Cookie{Name: langCookie, Value: lang})
		if c := sessions[p.as]; c != nil {
			req.AddCookie(c)
		} else if p.as == "" {
			req.AddCookie(sessions["ada"])
		}
		if p.htmx {
			req.Header.Set("HX-Request", "true")
		}
		if p.hxTarget != "" {
			req.Header.Set("HX-Target", p.hxTarget)
		}
		a.Cfg.PremiumEnabled = p.premium
		invalidateFeatureCache()
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		return rec
	}
	check := func(t *testing.T, body string, full bool) {
		t.Helper()
		if found := italianIn(body, data); len(found) > 0 {
			t.Errorf("Italian in the English page:\n  %s", strings.Join(found, "\n  "))
		}
		if found := catalogKeysIn(body, areas); len(found) > 0 {
			t.Errorf("catalog keys shown as text:\n  %s", strings.Join(found, "\n  "))
		}
		if full && !strings.Contains(body, `<html lang="en"`) {
			t.Errorf("the page does not declare lang=\"en\"")
		}
	}

	for _, p := range pages {
		name := p.target
		if p.as != "" {
			name = p.as + " " + name
		}
		if p.htmx {
			name += " (htmx)"
		}
		if p.premium {
			name += " (premium on)"
		}
		t.Run(name, func(t *testing.T) {
			rec := get(p, "en")
			want := cmpStatus(p.status)
			if rec.Code != want {
				t.Fatalf("status %d, want %d:\n%s", rec.Code, want, rec.Body.String())
			}
			if p.redirect != "" {
				if loc := rec.Header().Get("Location"); loc != p.redirect {
					t.Errorf("redirects to %q, want %q", loc, p.redirect)
				}
				return
			}
			body := rec.Body.String()
			if strings.HasSuffix(p.target, ".webmanifest") {
				body = manifestText(t, rec.Body.Bytes())
			}
			full := strings.Contains(body, "<html")
			check(t, body, full)
			// The same full page in Italian proves that italianIn sees Italian.
			it := get(p, "it").Body.String()
			if full && len(italianIn(it, data)) == 0 {
				t.Errorf("the Italian page has no Italian text:\n%s", it)
			}
			if rec.Code == http.StatusOK && p.target != "/manifest.webmanifest" {
				if same := sameInBoth(body, it, data); len(same) > 0 {
					t.Errorf("text the English page shares with the Italian one:\n  %s", strings.Join(same, "\n  "))
				}
			}
		})
	}

	// Scripts keep no text of their own: it reaches them in data-* attributes.
	t.Run("scripts hold no Italian", func(t *testing.T) {
		files, err := filepath.Glob("../web/static/js/*.js")
		if err != nil || len(files) == 0 {
			t.Fatalf("scripts: %v (%d)", err, len(files))
		}
		sources := map[string]string{"/sw.js": get(finalPage{target: "/sw.js", as: "out"}, "en").Body.String()}
		for _, f := range files {
			if strings.HasSuffix(f, ".min.js") {
				continue
			}
			src, err := os.ReadFile(f)
			if err != nil {
				t.Fatal(err)
			}
			sources[filepath.Base(f)] = string(src)
		}
		for name, src := range sources {
			src = reJSBlockComment.ReplaceAllString(jsLineComment.ReplaceAllString(src, ""), "")
			var found []string
			for _, m := range reJSLiteral.FindAllStringSubmatch(src, -1) {
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

	// The errors a flash quotes read the same in Italian as before, and in
	// English they are not Italian.
	t.Run("reasons", func(t *testing.T) {
		for _, r := range reasons {
			if it := i18n.T("it", r.key); it != r.err.Error() {
				t.Errorf("%s: Italian text %q, the error says %q", r.key, it, r.err)
			}
			if en := i18n.T("en", r.key); en == r.key || len(italianIn(en, nil)) > 0 {
				t.Errorf("%s: English text %q", r.key, en)
			}
		}
	})

	// The server error page is not reachable by a GET: call the handler.
	t.Run("server error", func(t *testing.T) {
		for _, as := range []*domain.User{nil, ada} {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.AddCookie(&http.Cookie{Name: langCookie, Value: "en"})
			if as != nil {
				req = requestWithUser(req, as)
			}
			rec := httptest.NewRecorder()
			languageMiddleware(a.Sessions.LoadAndSave(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				a.serverError(w, r, fmt.Errorf("boom"))
			}))).ServeHTTP(rec, req)
			if rec.Code != http.StatusInternalServerError {
				t.Fatalf("status %d", rec.Code)
			}
			check(t, rec.Body.String(), true)
		}
	})
}

// manifestText returns the text of the app manifest: the other fields are
// paths and colors, and the icon names carry the accent's identifier.
func manifestText(t *testing.T, raw []byte) string {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("manifest: %v", err)
	}
	var text []string
	for _, k := range []string{"name", "short_name", "description"} {
		s, _ := m[k].(string)
		if s == "" {
			t.Fatalf("manifest has no %s", k)
		}
		text = append(text, s)
	}
	return strings.Join(text, "\n")
}

func cmpStatus(s int) int {
	if s == 0 {
		return http.StatusOK
	}
	return s
}
