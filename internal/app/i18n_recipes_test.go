package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/amon2126/nutreek/internal/domain"
	"github.com/amon2126/nutreek/internal/off"
)

// rpEnv renders the pages of Recipes, the nutrition plan and the weekly
// templates (#153) as the router would, in a language chosen per request.
type rpEnv struct {
	t   *testing.T
	a   *App
	ctx context.Context
}

// rpReq describes one request to a handler.
type rpReq struct {
	method, target string
	form           url.Values
	params         map[string]string // chi URL parameters
	hx             bool              // an HTMX request that targets #recipe-results
	as             *domain.User
	flash          string // a flash key stored in the session first
	handler        http.HandlerFunc
	cookies        []*http.Cookie
}

func (e *rpEnv) do(lang string, q rpReq) *httptest.ResponseRecorder {
	e.t.Helper()
	if q.handler == nil {
		e.t.Fatalf("%s: no handler", q.target)
	}
	method := q.method
	if method == "" {
		method = http.MethodGet
	}
	req := httptest.NewRequest(method, q.target, strings.NewReader(q.form.Encode()))
	if q.form != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	if lang != "" {
		req.AddCookie(&http.Cookie{Name: langCookie, Value: lang})
	}
	for _, c := range q.cookies {
		req.AddCookie(c)
	}
	if q.hx {
		req.Header.Set("HX-Request", "true")
		req.Header.Set("HX-Target", "recipe-results")
	}
	if len(q.params) > 0 {
		rctx := chi.NewRouteContext()
		for k, v := range q.params {
			rctx.URLParams.Add(k, v)
		}
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	}
	req = requestWithUser(req, q.as)
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if q.flash != "" {
			e.a.Sessions.Put(r.Context(), "flash", q.flash)
		}
		q.handler(w, r)
	})
	rec := httptest.NewRecorder()
	languageMiddleware(e.a.Sessions.LoadAndSave(h)).ServeHTTP(rec, req)
	return rec
}

func rpID(id int64) string { return strconv.FormatInt(id, 10) }

func TestEnglishRecipesPlanAndTemplates(t *testing.T) {
	a := newTestApp(t)
	a.OFF = off.NewClient(a.Store.DB, "http://127.0.0.1:1", nil)
	ctx := context.Background()
	e := &rpEnv{t: t, a: a, ctx: ctx}

	mkUser := func(email, name string) *domain.User {
		id, err := a.Users.Create(ctx, email, "x", name)
		if err != nil {
			t.Fatal(err)
		}
		u, err := a.Users.GetByID(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		return u
	}
	must := func(id int64, err error) int64 {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	ok := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}

	ada := mkUser("ada@example.test", "Ada")
	bob := mkUser("bob@example.test", "Bob")     // another author, and the coach
	noPlan := mkUser("eve@example.test", "Eve")  // nothing at all
	idle := mkUser("ivy@example.test", "Ivy")    // templates, none active
	empty := mkUser("max@example.test", "Max")   // an active template without meals
	planned := mkUser("zoe@example.test", "Zoe") // an active template, variants and foods
	client := mkUser("cat@example.test", "Cat")  // a plan assigned by the coach
	client2 := mkUser("dan@example.test", "Dan") // an empty plan assigned by the coach

	oats := must(a.Ingredients.CreateLocal(ctx, "Oats", nil, domain.UnitGram, 372, 13.5, 58.7, 7, &ada.ID, nil, nil, nil, nil, nil, nil))
	rice := must(a.Ingredients.CreateLocal(ctx, "Rice", nil, domain.UnitGram, 360, 7, 79, 0.6, &ada.ID, nil, nil, nil, nil, nil, nil))

	desc := "Soak the oats."
	porridge := must(a.Recipes.Create(ctx, ada.ID, "Porridge", &desc, 2, true))
	_ = must(a.Recipes.AddItem(ctx, porridge, ada.ID, oats, 100))
	duo := must(a.Recipes.Create(ctx, ada.ID, "Duo", nil, 1, false))
	_ = must(a.Recipes.AddItem(ctx, duo, ada.ID, oats, 100))
	duoRice := must(a.Recipes.AddItem(ctx, duo, ada.ID, rice, 50))
	bowl := must(a.Recipes.Create(ctx, ada.ID, "Bowl", nil, 1, false))
	theirs := must(a.Recipes.Create(ctx, bob.ID, "Pasta", &desc, 4, true))
	// A draft copy of "Duo" is identical to it; a draft whose source is gone is not.
	draftSame := must(a.Recipes.Duplicate(ctx, duo, ada.ID, "Duo copy"))
	lone := must(a.Recipes.Create(ctx, ada.ID, "Lone", nil, 1, false))
	_ = must(a.Recipes.AddItem(ctx, lone, ada.ID, rice, 10))
	draftLone := must(a.Recipes.Duplicate(ctx, lone, ada.ID, "Lone copy"))
	ok(a.Recipes.Delete(ctx, lone, ada.ID))

	// Weekly templates and the nutrition plan of the other users.
	ok2 := func(_ int64, err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	idleTpl := must(a.MealTemplates.Create(ctx, idle.ID, "Standard week"))
	emptyTpl := must(a.MealTemplates.Create(ctx, empty.ID, "Standard week"))
	ok(a.MealTemplates.SetActive(ctx, emptyTpl, empty.ID))
	plannedTpl := must(a.MealTemplates.Create(ctx, planned.ID, "Standard week"))
	spare := must(a.MealTemplates.Create(ctx, planned.ID, "Away from home"))
	ok(a.MealTemplates.SetActive(ctx, plannedTpl, planned.ID))
	everyDay := must(a.MealTemplates.AddSlot(ctx, plannedTpl, planned.ID, domain.DayOfWeekEvery, "Breakfast"))
	ok2(a.MealTemplates.AddSlot(ctx, plannedTpl, planned.ID, domain.DayOfWeekEvery, "Dinner"))
	ok2(a.MealTemplates.AddSlot(ctx, plannedTpl, planned.ID, 2, "Snack"))
	ok2(a.MealTemplates.AddSlot(ctx, plannedTpl, planned.ID, 5, "Brunch"))

	plannedPlan, err := a.NutritionistPlans.GetOrCreateActivePlan(ctx, planned.ID)
	ok(err)
	ok(a.NutritionistPlans.EnsureMealType(ctx, plannedPlan.ID, "Breakfast"))
	ok(a.NutritionistPlans.EnsureMealType(ctx, plannedPlan.ID, "Dinner"))
	full, err := a.NutritionistPlans.LoadPlanFull(ctx, planned.ID)
	ok(err)
	breakfastType := full.MealTypes[0].ID
	named := "Light"
	mainVar := must(a.NutritionistPlans.CreateVariation(ctx, plannedPlan.ID, breakfastType, nil))
	lightVar := must(a.NutritionistPlans.CreateVariation(ctx, plannedPlan.ID, breakfastType, &named))
	soup := must(a.Recipes.Create(ctx, planned.ID, "Soup bowl", nil, 2, false))
	ok2(a.NutritionistPlans.AddItem(ctx, plannedPlan.ID, mainVar, &soup, nil, 1, "g"))
	ok2(a.NutritionistPlans.AddItem(ctx, plannedPlan.ID, mainVar, nil, &oats, 50, "g"))
	ok2(a.NutritionistPlans.AddItem(ctx, plannedPlan.ID, lightVar, nil, &rice, 30, "g"))

	coachPlan := must(a.NutritionistPlans.CreatePlan(ctx, bob.ID, "Coach plan"))
	lunch := must(a.NutritionistPlans.CreateMealType(ctx, coachPlan, "Lunch"))
	coachVar := must(a.NutritionistPlans.CreateVariation(ctx, coachPlan, lunch, nil))
	ok2(a.NutritionistPlans.AddItem(ctx, coachPlan, coachVar, nil, &rice, 80, "g"))
	emptyCoachPlan := must(a.NutritionistPlans.CreatePlan(ctx, bob.ID, "Blank plan"))
	ok(a.Coach.AddClient(ctx, bob.ID, client.ID))
	ok(a.Coach.AssignPlan(ctx, bob.ID, client.ID, coachPlan))
	ok(a.Coach.AddClient(ctx, bob.ID, client2.ID))
	ok(a.Coach.AssignPlan(ctx, bob.ID, client2.ID, emptyCoachPlan))

	// User-entered names and day names (#157) are not interface text.
	data := []string{
		"Ada", "Bob", "Eve", "Ivy", "Max", "Zoe", "Cat", "Dan", "Oats", "Rice",
		"Porridge", "Duo", "Bowl", "Pasta", "Duo copy", "Lone copy", "Soup bowl", "Soak the oats.",
		"Standard week", "Away from home", "Breakfast", "Dinner", "Snack", "Brunch", "Lunch", "Light", "Coach plan", "Blank plan",
	}
	data = append(data, dayFullNames...) // #157

	longName := strings.Repeat("n", MaxNameLen+1)
	longDesc := strings.Repeat("d", MaxDescriptionLen+1)

	type page struct {
		name string
		rpReq
		want string // English text the page must show
		// italian is what the page without the cookie shows, when the page has
		// no Italian text that italianIn would find
		italian string
	}
	byID := func(id int64) map[string]string { return map[string]string{"id": rpID(id)} }
	post := func(target string, form url.Values) rpReq {
		return rpReq{method: http.MethodPost, target: target, form: form, as: ada}
	}
	with := func(r rpReq, h http.HandlerFunc) rpReq { r.handler = h; return r }
	forID := func(r rpReq, id int64) rpReq { r.params = byID(id); return r }

	pages := []page{
		// ---- recipes: list
		{name: "list", rpReq: rpReq{target: "/recipes", as: ada, handler: a.handleRecipesList}, want: "recipes of yours"},
		{name: "list with a flash", rpReq: rpReq{target: "/recipes", as: ada, flash: "recipes.flash.created", handler: a.handleRecipesList}, want: "Recipe created."},
		{name: "list, others", rpReq: rpReq{target: "/recipes?f=others", as: ada, handler: a.handleRecipesList}, want: "Public from others"},
		{name: "list, used", rpReq: rpReq{target: "/recipes?f=used", as: ada, handler: a.handleRecipesList}, want: "Used in plans"},
		{name: "list, public", rpReq: rpReq{target: "/recipes?f=public", as: ada, handler: a.handleRecipesList}, want: "Made public"},
		{name: "list, search", rpReq: rpReq{target: "/recipes?q=port", as: ada, handler: a.handleRecipesList}, want: "Search for a recipe"},
		{name: "list, nothing found", rpReq: rpReq{target: "/recipes?q=zzz", as: ada, handler: a.handleRecipesList}, want: "No recipe found for “zzz”."},
		{name: "list, nothing in use", rpReq: rpReq{target: "/recipes?f=used", as: noPlan, handler: a.handleRecipesList}, want: "No recipes in this list."},
		{name: "list, first recipe", rpReq: rpReq{target: "/recipes", as: noPlan, handler: a.handleRecipesList}, want: "Create your first recipe"},
		{name: "list, live search partial", rpReq: rpReq{target: "/recipes?q=po", hx: true, as: ada, handler: a.handleRecipesList}, want: "recipe_id"},
		{name: "list, bulk bar texts", rpReq: rpReq{target: "/recipes", as: ada, handler: a.handleRecipesList}, want: `data-msg-selected-other="{n} recipes selected"`},
		// ---- recipes: new and import
		{name: "new", rpReq: rpReq{target: "/recipes/new", as: ada, handler: a.handleRecipeNewForm}, want: "Import from URL"},
		{name: "import, no URL", rpReq: with(post("/recipes/import", url.Values{"import_url": {""}}), a.handleRecipeImport), want: "Paste the URL of a recipe to import."},
		{name: "import, invalid URL", rpReq: with(post("/recipes/import", url.Values{"import_url": {"ftp://example.test/recipe"}}), a.handleRecipeImport), want: "Invalid URL: use an http(s) link to a public site."},
		{name: "import, found ingredients", rpReq: rpReq{target: "/recipes/import", as: ada, handler: func(w http.ResponseWriter, r *http.Request) {
			data := recipeNewData{
				authPageData:        a.baseData(r),
				Form:                recipeForm{Name: "Carbonara", Servings: "2"},
				ImportedIngredients: []string{"200 g spaghetti", "2 eggs"},
				ImportSource:        "https://example.test/carbonara",
			}
			if err := a.Render.Render(w, r, http.StatusOK, "recipe_new", data); err != nil {
				a.serverError(w, r, err)
			}
		}}, want: "Ingredients found (2)"},
		{name: "create, name missing", rpReq: with(post("/recipes", url.Values{"servings": {"2"}}), a.handleRecipeCreate), want: "The name is required."},
		{name: "create, name too long", rpReq: with(post("/recipes", url.Values{"name": {longName}, "servings": {"2"}}), a.handleRecipeCreate), want: "The name is too long (max 200 characters)."},
		{name: "create, description too long", rpReq: with(post("/recipes", url.Values{"name": {"Tart"}, "description": {longDesc}, "servings": {"2"}}), a.handleRecipeCreate), want: "The description is too long (max 2000 characters)."},
		{name: "create, bad servings", rpReq: with(post("/recipes", url.Values{"name": {"Tart"}, "servings": {"0"}}), a.handleRecipeCreate), want: "Servings: enter a whole number above 0."},
		{name: "create, name taken", rpReq: with(post("/recipes", url.Values{"name": {"Porridge"}, "servings": {"2"}}), a.handleRecipeCreate), want: "You already have a recipe with this name."},
		// ---- recipes: detail
		{name: "edit own", rpReq: forID(rpReq{target: "/recipes/x", as: ada, handler: a.handleRecipeEdit}, porridge), want: "Whole recipe"},
		{name: "edit own, no ingredients", rpReq: forID(rpReq{target: "/recipes/x", as: ada, handler: a.handleRecipeEdit}, bowl), want: "No ingredients in this recipe. Search for one below."},
		{name: "edit own, two ingredients", rpReq: forID(rpReq{target: "/recipes/x", as: ada, handler: a.handleRecipeEdit}, duo), want: "Not used in the weekly plans yet."},
		{name: "edit, a draft", rpReq: forID(rpReq{target: "/recipes/x", as: ada, handler: a.handleRecipeEdit}, draftSame), want: "Unconfirmed draft."},
		{name: "read-only", rpReq: forID(rpReq{target: "/recipes/x", as: ada, handler: a.handleRecipeEdit}, theirs), want: "By Bob"},
		{name: "update, name missing", rpReq: forID(with(post("/recipes/x", url.Values{"servings": {"2"}}), a.handleRecipeUpdate), porridge), want: "The name is required."},
		{name: "update, name too long", rpReq: forID(with(post("/recipes/x", url.Values{"name": {longName}, "servings": {"2"}}), a.handleRecipeUpdate), porridge), want: "The name is too long (max 200 characters)."},
		{name: "update, description too long", rpReq: forID(with(post("/recipes/x", url.Values{"name": {"Porridge"}, "description": {longDesc}, "servings": {"2"}}), a.handleRecipeUpdate), porridge), want: "The description is too long (max 2000 characters)."},
		{name: "update, bad servings", rpReq: forID(with(post("/recipes/x", url.Values{"name": {"Porridge"}, "servings": {"x"}}), a.handleRecipeUpdate), porridge), want: "Servings: enter a whole number above 0."},
		{name: "update, name taken", rpReq: forID(with(post("/recipes/x", url.Values{"name": {"Bowl"}, "servings": {"2"}}), a.handleRecipeUpdate), porridge), want: "You already have a recipe with this name."},
		{name: "update, draft like another recipe", rpReq: forID(with(post("/recipes/x", url.Values{"name": {"Duo copy"}, "servings": {"1"}}), a.handleRecipeUpdate), draftSame), want: "This draft has the same ingredients as “Duo”."},
		{name: "update, plain draft", rpReq: forID(with(post("/recipes/x", url.Values{"name": {"Lone copy"}, "servings": {"1"}}), a.handleRecipeUpdate), draftLone), want: "This recipe is a draft."},
		{name: "add item, bad quantity", rpReq: forID(with(post("/recipes/x/items", url.Values{"quantity": {"abc"}, "ingredient_id": {rpID(oats)}}), a.handleRecipeAddItem), bowl), want: "Invalid quantity."},
		{name: "add item, no ingredient", rpReq: forID(with(post("/recipes/x/items", url.Values{"quantity": {"10"}}), a.handleRecipeAddItem), bowl), want: "Select an ingredient."},
		{name: "add item, OpenFoodFacts down", rpReq: forID(with(post("/recipes/x/items", url.Values{"quantity": {"10"}, "barcode": {"8000000000999"}}), a.handleRecipeAddItem), bowl), want: "OpenFoodFacts import failed."},
		{name: "add item, same as another recipe", rpReq: forID(with(post("/recipes/x/items", url.Values{"quantity": {"100"}, "ingredient_id": {rpID(oats)}}), a.handleRecipeAddItem), bowl), want: "Adding this ingredient would make the recipe identical to “Porridge”"},
		{name: "remove item, same as another recipe", rpReq: func() rpReq {
			r := forID(with(post("/recipes/x/items/y/delete", nil), a.handleRecipeRemoveItem), duo)
			r.params["itemID"] = rpID(duoRice)
			return r
		}(), want: "Removing this ingredient would make the recipe identical to “Porridge”"},
		{name: "picker, not the author", rpReq: forID(rpReq{target: "/recipes/x/picker", as: ada, handler: a.handleRecipePicker}, theirs), want: "you are not the author", italian: "non sei l'autore"},
		// ---- recipes: dialogs
		{name: "duplicate dialog", rpReq: forID(rpReq{target: "/recipes/x/duplicate", as: ada, handler: a.handleRecipeDuplicateForm}, porridge), want: "Copy of Porridge"},
		{name: "duplicate, no name", rpReq: forID(with(post("/recipes/x/duplicate", url.Values{"new_name": {""}}), a.handleRecipeDuplicate), porridge), want: "Enter a name for the copy."},
		{name: "duplicate, name too long", rpReq: forID(with(post("/recipes/x/duplicate", url.Values{"new_name": {longName}}), a.handleRecipeDuplicate), porridge), want: "The name is too long (max 200 characters)."},
		{name: "duplicate, same name", rpReq: forID(with(post("/recipes/x/duplicate", url.Values{"new_name": {"porridge"}}), a.handleRecipeDuplicate), porridge), want: "The name of the copy must differ from the original."},
		{name: "duplicate, name taken", rpReq: forID(with(post("/recipes/x/duplicate", url.Values{"new_name": {"Bowl"}}), a.handleRecipeDuplicate), porridge), want: "You already have a recipe with this name."},
		{name: "quick recipe", rpReq: rpReq{target: "/quick/recipe", as: ada, handler: a.handleQuickRecipeForm}, want: "The recipe starts empty"},
		{name: "quick recipe, no name", rpReq: with(post("/quick/recipe", url.Values{"servings": {"1"}}), a.handleQuickRecipeCreate), want: "Invalid recipe name."},
		{name: "quick recipe, bad servings", rpReq: with(post("/quick/recipe", url.Values{"name": {"Tart"}, "servings": {"0"}}), a.handleQuickRecipeCreate), want: "Invalid servings."},
		{name: "quick recipe, name taken", rpReq: with(post("/quick/recipe", url.Values{"name": {"Porridge"}, "servings": {"1"}}), a.handleQuickRecipeCreate), want: "You already have a recipe with this name."},
		{name: "bulk delete, bad form", rpReq: with(post("/recipes/bulk-delete?a=%zz", nil), a.handleRecipesBulkDelete), want: "invalid form", italian: "form invalido"},
		// ---- nutrition plan
		{name: "plan, no template", rpReq: rpReq{target: "/piano-nutrizionale", as: noPlan, handler: a.handlePianoNutrizionale}, want: "Choose a template"},
		{name: "plan, templates tab empty", rpReq: rpReq{target: "/piano-nutrizionale?tab=template", as: noPlan, handler: a.handlePianoNutrizionale}, want: "No templates yet"},
		{name: "plan, templates tab", rpReq: rpReq{target: "/piano-nutrizionale?tab=template", as: planned, handler: a.handlePianoNutrizionale}, want: "Delete the template “Away from home”?"},
		{name: "plan, no active template", rpReq: rpReq{target: "/piano-nutrizionale", as: idle, handler: a.handlePianoNutrizionale}, want: "Choose a template"},
		{name: "plan, template without meals", rpReq: rpReq{target: "/piano-nutrizionale", as: empty, handler: a.handlePianoNutrizionale}, want: "Add meals to the template"},
		{name: "plan, variants and foods", rpReq: rpReq{target: "/piano-nutrizionale", as: planned, handler: a.handlePianoNutrizionale}, want: "Delete the variant “Light” and its foods?"},
		{name: "plan, main variant", rpReq: rpReq{target: "/piano-nutrizionale", as: planned, handler: a.handlePianoNutrizionale}, want: "Delete the variant “Main variant” and its foods?"},
		{name: "plan from the coach", rpReq: rpReq{target: "/piano-nutrizionale", as: client, handler: a.handlePianoNutrizionale}, want: "assigned by your coach"},
		{name: "plan from the coach, empty", rpReq: rpReq{target: "/piano-nutrizionale", as: client2, handler: a.handlePianoNutrizionale}, want: "has no content yet."},
		{name: "plan, bad variation in the picker", rpReq: rpReq{target: "/nutrizionista/picker?variation_id=x", as: planned, handler: a.handleNutritionistPicker}, want: "invalid variation_id", italian: "variation_id non valido"},
		{name: "plan, bad kind in the picker", rpReq: rpReq{target: "/nutrizionista/picker?variation_id=" + rpID(mainVar) + "&kind=x", as: planned, handler: a.handleNutritionistPicker}, want: "invalid kind", italian: "kind non valido"},
		// ---- templates
		{name: "template, new", rpReq: rpReq{target: "/templates/new", as: ada, handler: a.handleTemplateNewForm}, want: "Give it a name"},
		{name: "template, no name", rpReq: with(post("/templates", url.Values{"name": {""}}), a.handleTemplateCreate), want: "The name is required."},
		{name: "template, name too long", rpReq: with(post("/templates", url.Values{"name": {longName}}), a.handleTemplateCreate), want: "The name is too long (max 200 characters)."},
		{name: "template, edit (active)", rpReq: func() rpReq {
			r := forID(rpReq{target: "/templates/x", as: planned, handler: a.handleTemplateEdit}, plannedTpl)
			return r
		}(), want: "Meals for one day only:"},
		{name: "template, edit (inactive)", rpReq: forID(rpReq{target: "/templates/x", as: planned, handler: a.handleTemplateEdit}, spare), want: "Add a meal to every day"},
		{name: "template, edit with a flash", rpReq: forID(rpReq{target: "/templates/x", as: planned, flash: "templates.flash.created", handler: a.handleTemplateEdit}, plannedTpl), want: "Template created."},
		{name: "template, rename without a name", rpReq: forID(with(rpReq{method: http.MethodPost, target: "/templates/x", form: url.Values{"name": {""}}, as: planned}, a.handleTemplateRename), plannedTpl), want: "The name is required."},
		{name: "template, rename to a long name", rpReq: forID(with(rpReq{method: http.MethodPost, target: "/templates/x", form: url.Values{"name": {longName}}, as: planned}, a.handleTemplateRename), plannedTpl), want: "The name is too long (max 200 characters)."},
		{name: "template, bad day", rpReq: forID(with(rpReq{method: http.MethodPost, target: "/templates/x/slots", form: url.Values{"day_of_week": {"9"}, "slot_name": {"Tea"}}, as: planned}, a.handleTemplateAddSlot), plannedTpl), want: "Invalid day."},
		{name: "template, meal without a name", rpReq: forID(with(rpReq{method: http.MethodPost, target: "/templates/x/slots", form: url.Values{"day_of_week": {"1"}}, as: planned}, a.handleTemplateAddSlot), plannedTpl), want: "The meal name is required."},
		{name: "template, long meal name", rpReq: forID(with(rpReq{method: http.MethodPost, target: "/templates/x/slots", form: url.Values{"day_of_week": {"-1"}, "slot_name": {longName}}, as: planned}, a.handleTemplateAddSlot), plannedTpl), want: "The name is too long (max 200 characters)."},
		{name: "template, unknown slot", rpReq: func() rpReq {
			r := forID(with(rpReq{method: http.MethodPost, target: "/templates/x/slots/y/delete", as: planned}, a.handleTemplateRemoveSlot), plannedTpl)
			r.params["slotID"] = "99999"
			return r
		}(), want: "meal slot not found", italian: "slot non trovata"},
		{name: "template, reorder with a bad day", rpReq: func() rpReq {
			r := forID(with(rpReq{method: http.MethodPost, target: "/templates/x/days/y/slots/reorder", form: url.Values{"ids": {"1"}}, as: planned}, a.handleReorderTemplateSlots), plannedTpl)
			r.params["day"] = "9"
			return r
		}(), want: "invalid day_of_week", italian: "day_of_week invalido"},
		{name: "template, reorder without ids", rpReq: func() rpReq {
			r := forID(with(rpReq{method: http.MethodPost, target: "/templates/x/days/y/slots/reorder", form: url.Values{"ids": {""}}, as: planned}, a.handleReorderTemplateSlots), plannedTpl)
			r.params["day"] = "1"
			return r
		}(), want: "invalid ids", italian: "ids non valido"},
		{name: "template, reorder out of order", rpReq: func() rpReq {
			r := forID(with(rpReq{method: http.MethodPost, target: "/templates/x/days/y/slots/reorder", form: url.Values{"ids": {rpID(everyDay)}}, as: planned}, a.handleReorderTemplateSlots), plannedTpl)
			r.params["day"] = "-1"
			return r
		}(), want: "invalid order", italian: "ordine non valido"},
	}

	check := func(t *testing.T, p page, lang string) string {
		t.Helper()
		rec := e.do(lang, p.rpReq)
		if rec.Code >= 300 && rec.Code < 400 {
			t.Fatalf("redirected to %s", rec.Header().Get("Location"))
		}
		return rec.Body.String()
	}
	for _, p := range pages {
		t.Run(p.name, func(t *testing.T) {
			body := check(t, p, "en")
			if !strings.Contains(body, p.want) {
				t.Errorf("%q missing from the English page:\n%s", p.want, body)
			}
			if found := italianIn(body, data); len(found) > 0 {
				t.Errorf("Italian in the English page:\n  %s", strings.Join(found, "\n  "))
			}
			it := check(t, p, "")
			if p.italian != "" && !strings.Contains(it, p.italian) || p.italian == "" && len(italianIn(it, data)) == 0 {
				t.Errorf("the page without the cookie is not Italian:\n%s", it)
			}
		})
	}

	// Flashes that go through the session: the handler redirects, the next
	// page shows the message.
	t.Run("flashes", func(t *testing.T) {
		type step struct {
			name   string
			prep   func() rpReq // the request that sets the flash, built anew per language
			then   rpReq        // the page that shows it
			wantEn string
			wantIt string
		}
		list := rpReq{target: "/recipes", as: ada, handler: a.handleRecipesList}
		planPage := func(u *domain.User, tab string) rpReq {
			return rpReq{target: "/piano-nutrizionale" + tab, as: u, handler: a.handlePianoNutrizionale}
		}
		n := 0
		unique := func(base string) string { n++; return base + " " + strconv.Itoa(n) }
		steps := []step{
			{name: "recipe created", prep: func() rpReq {
				return with(post("/recipes", url.Values{"name": {unique("Tart")}, "servings": {"2"}}), a.handleRecipeCreate)
			}, then: list, wantEn: "Recipe created.", wantIt: "Ricetta creata."},
			{name: "recipe updated", prep: func() rpReq {
				return forID(with(post("/recipes/x", url.Values{"name": {"Porridge"}, "servings": {"2"}}), a.handleRecipeUpdate), porridge)
			}, then: list, wantEn: "Recipe updated.", wantIt: "Ricetta aggiornata."},
			{name: "recipe deleted", prep: func() rpReq {
				id := must(a.Recipes.Create(ctx, ada.ID, unique("Gone"), nil, 1, false))
				return forID(with(post("/recipes/x/delete", nil), a.handleRecipeDelete), id)
			}, then: list, wantEn: "Recipe deleted.", wantIt: "Ricetta eliminata."},
			{name: "recipes deleted", prep: func() rpReq {
				x := must(a.Recipes.Create(ctx, ada.ID, unique("Gone"), nil, 1, false))
				y := must(a.Recipes.Create(ctx, ada.ID, unique("Gone"), nil, 1, false))
				return with(post("/recipes/bulk-delete", url.Values{"recipe_id": {rpID(x), rpID(y)}}), a.handleRecipesBulkDelete)
			}, then: list, wantEn: "2 recipes deleted.", wantIt: "2 ricette eliminate."},
			{name: "one recipe deleted", prep: func() rpReq {
				x := must(a.Recipes.Create(ctx, ada.ID, unique("Gone"), nil, 1, false))
				return with(post("/recipes/bulk-delete", url.Values{"recipe_id": {rpID(x)}}), a.handleRecipesBulkDelete)
			}, then: list, wantEn: "1 recipe deleted.", wantIt: "1 ricetta eliminata."},
			{name: "no recipe deleted", prep: func() rpReq {
				return with(post("/recipes/bulk-delete", url.Values{"recipe_id": {"99999"}}), a.handleRecipesBulkDelete)
			}, then: list, wantEn: "No recipe deleted.", wantIt: "Nessuna ricetta eliminata."},
			{name: "no recipe selected", prep: func() rpReq {
				return with(post("/recipes/bulk-delete", nil), a.handleRecipesBulkDelete)
			}, then: list, wantEn: "No recipe selected.", wantIt: "Nessuna ricetta selezionata."},
			{name: "recipe duplicated", prep: func() rpReq {
				return forID(with(post("/recipes/x/duplicate", url.Values{"new_name": {unique("Porridge copy")}}), a.handleRecipeDuplicate), porridge)
			}, then: list, wantEn: "Recipe duplicated.", wantIt: "Ricetta duplicata."},
			{name: "template created", prep: func() rpReq {
				return with(post("/templates", url.Values{"name": {unique("Extra week")}}), a.handleTemplateCreate)
			}, then: planPage(ada, "?tab=template"), wantEn: "Template created.", wantIt: "Template creato."},
			{name: "template renamed", prep: func() rpReq {
				return forID(with(rpReq{method: http.MethodPost, target: "/templates/x", form: url.Values{"name": {unique("Renamed")}}, as: idle}, a.handleTemplateRename), idleTpl)
			}, then: planPage(idle, "?tab=template"), wantEn: "Template renamed.", wantIt: "Template rinominato."},
			{name: "template activated", prep: func() rpReq {
				return forID(with(rpReq{method: http.MethodPost, target: "/templates/x/activate", as: idle}, a.handleTemplateActivate), idleTpl)
			}, then: planPage(idle, "?tab=template"), wantEn: "Template activated.", wantIt: "Template attivato."},
			{name: "template deleted", prep: func() rpReq {
				id := must(a.MealTemplates.Create(ctx, ada.ID, unique("Doomed")))
				return forID(with(rpReq{method: http.MethodPost, target: "/templates/x/delete", as: ada}, a.handleTemplateDelete), id)
			}, then: planPage(ada, "?tab=template"), wantEn: "Template deleted.", wantIt: "Template eliminato."},
			{name: "meal type without a name", prep: func() rpReq {
				return with(rpReq{method: http.MethodPost, target: "/nutrizionista/meal-types", form: url.Values{"name": {""}}, as: planned}, a.handleNutritionistCreateMealType)
			}, then: planPage(planned, ""), wantEn: "The meal type name is required.", wantIt: "Il nome del tipo pasto è obbligatorio."},
			{name: "bad meal type", prep: func() rpReq {
				return with(rpReq{method: http.MethodPost, target: "/nutrizionista/variations", form: url.Values{"meal_type_id": {"x"}}, as: planned}, a.handleNutritionistCreateVariation)
			}, then: planPage(planned, ""), wantEn: "Invalid meal type.", wantIt: "Tipo pasto non valido."},
			{name: "bad variant", prep: func() rpReq {
				return with(rpReq{method: http.MethodPost, target: "/nutrizionista/items", form: url.Values{"variation_id": {"x"}, "quantity": {"1"}}, as: planned}, a.handleNutritionistAddItem)
			}, then: planPage(planned, ""), wantEn: "Invalid variant.", wantIt: "Variante non valida."},
			{name: "bad quantity", prep: func() rpReq {
				return with(rpReq{method: http.MethodPost, target: "/nutrizionista/items", form: url.Values{"variation_id": {rpID(mainVar)}, "quantity": {"x"}}, as: planned}, a.handleNutritionistAddItem)
			}, then: planPage(planned, ""), wantEn: "Invalid quantity.", wantIt: "Quantità non valida."},
			{name: "nothing picked", prep: func() rpReq {
				return with(rpReq{method: http.MethodPost, target: "/nutrizionista/items", form: url.Values{"variation_id": {rpID(mainVar)}, "quantity": {"1"}}, as: planned}, a.handleNutritionistAddItem)
			}, then: planPage(planned, ""), wantEn: "Select an ingredient or a recipe.", wantIt: "Seleziona un ingrediente o una ricetta."},
			{name: "OpenFoodFacts down", prep: func() rpReq {
				return with(rpReq{method: http.MethodPost, target: "/nutrizionista/items", form: url.Values{"variation_id": {rpID(mainVar)}, "quantity": {"1"}, "barcode": {"8000000000999"}}, as: planned}, a.handleNutritionistAddItem)
			}, then: planPage(planned, ""), wantEn: "OpenFoodFacts import failed.", wantIt: "Import OFF fallito."},
		}
		for _, s := range steps {
			t.Run(s.name, func(t *testing.T) {
				for _, lang := range []string{"en", ""} {
					first := e.do(lang, s.prep())
					// A redirect, or the 204 with HX-Redirect of the dialogs.
					if first.Code != http.StatusNoContent && (first.Code < 300 || first.Code >= 400) {
						t.Fatalf("%q: status %d, not a redirect:\n%s", lang, first.Code, first.Body.String())
					}
					then := s.then
					then.cookies = first.Result().Cookies()
					body := e.do(lang, then).Body.String()
					want := s.wantEn
					if lang == "" {
						want = s.wantIt
					}
					if !strings.Contains(body, want) {
						t.Errorf("%q missing from the page (lang %q):\n%s", want, lang, body)
					}
					if lang == "en" {
						if found := italianIn(body, data); len(found) > 0 {
							t.Errorf("Italian in the English page:\n  %s", strings.Join(found, "\n  "))
						}
					}
				}
			})
		}
	})

	// The flash of a recipe that was in the weekly plans counts the entries.
	t.Run("counts", func(t *testing.T) {
		for lang, want := range map[string]string{
			"en": "Recipe deleted. Also removed from 2 entries in the weekly plans.",
			"":   "Ricetta eliminata. Rimossa anche da 2 voci nei piani settimanali.",
		} {
			if got := a.t(requestWithLang(lang), "recipes.flash.deleted_used", "Count", 2); got != want {
				t.Errorf("lang %q: %q, want %q", lang, got, want)
			}
		}
	})
}

// requestWithLang is a request that carries a language, to call a.t.
func requestWithLang(lang string) *http.Request {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	if lang != "" {
		req.AddCookie(&http.Cookie{Name: langCookie, Value: lang})
	}
	var out *http.Request
	languageMiddleware(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) { out = r })).
		ServeHTTP(httptest.NewRecorder(), req)
	return out
}
