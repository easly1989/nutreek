package app

import (
	"bytes"
	"context"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/amon2126/nutreek/internal/db"
	"github.com/amon2126/nutreek/internal/domain"
	"github.com/amon2126/nutreek/internal/off"
)

func init() {
	// More Italian words, the ones of the week and the picker: italianIn would
	// not see them otherwise (it only knows words and accents).
	for _, w := range strings.Fields(`aggiungi aggiungine aggiunta elimina eliminare sgarro sgarri pasto pasti
		allenamento acqua oggi svuota svuotare genera sceglie applica ripristina personalizza porzioni porz
		obiettivo obiettivi pianificare pianificato pianificati rispettato aderenza scostamento trascina
		riordinare mangiato voce voci quantita attivo scegli pubblica ricetta suggeriti importati
		primo carico riprova esporta lista precedente successiva scorsa mancante invalido seleziona
		obbligatorio troppo lungo fallito impossibile giorno giorni durata minuti allenamenti variazioni
		consumato escludere originale rimane invariata`) {
		italianWords[w] = true
	}
}

// weekCall is one request of TestEnglishWeekAndPicker.
type weekCall struct {
	name, method, target string
	form                 url.Values
	params               map[string]string // chi route parameters
	htmx                 bool
	as                   *domain.User // the viewer, when not Ada
	week                 string       // after the request, show this week: the flash it set
	body                 []byte       // a multipart body, instead of form
	contentType          string
	italian              string // what gives the Italian page away, when not a word
	want                 string // English text the page must show
	handler              http.HandlerFunc
}

// TestEnglishWeekAndPicker renders the week and the add-food picker with the
// English cookie and finds no Italian left (#152); the same pages without the
// cookie must still be Italian.
func TestEnglishWeekAndPicker(t *testing.T) {
	a := newTestApp(t)
	a.OFF = off.NewClient(a.Store.DB, "http://127.0.0.1:1", nil)
	ctx := context.Background()
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

	ada := mustCreateTestUser(t, a, "ada@example.test", "x")
	bea := mustCreateTestUser(t, a, "bea@example.test", "x")
	bob := mustCreateTestUser(t, a, "bob@example.test", "x")
	// Goals: the protein one is tight enough to be exceeded, so both bar states render.
	kcal, prot, carb, fat := 2000.0, 10.0, 250.0, 60.0
	must(a.Users.UpdateProfile(ctx, ada.ID, db.UserProfileInput{TargetKcal: &kcal, TargetProtein: &prot, TargetCarbs: &carb, TargetFat: &fat, WaterTargetML: 2000}))
	ada, _ = a.Users.GetByID(ctx, ada.ID)

	// Ingredients, recipes.
	weight, serving := 60.0, 30.0
	oats := id(a.Ingredients.CreateLocal(ctx, "Oats", nil, domain.UnitGram, 372, 13.5, 58.7, 7, &ada.ID, nil, nil, nil, nil, nil, nil))
	egg := id(a.Ingredients.CreateLocal(ctx, "Egg", nil, domain.UnitGram, 143, 12.6, 0.7, 9.5, &ada.ID, &weight, &serving, nil, nil, nil, nil))
	rice := id(a.Ingredients.CreateLocal(ctx, "Rice", nil, domain.UnitGram, 360, 7, 79, 0.6, &ada.ID, nil, nil, nil, nil, nil, nil))
	soup := id(a.Recipes.Create(ctx, ada.ID, "Oat soup", nil, 4, false))
	id(a.Recipes.AddItem(ctx, soup, ada.ID, oats, 200))
	shell := id(a.Recipes.Create(ctx, ada.ID, "Empty shell", nil, 1, false))
	id(a.Recipes.Create(ctx, bob.ID, "Public pie", nil, 1, true))
	if _, err := a.Store.DB.Exec(`INSERT INTO off_products (barcode, product_name, brands, quantity,
		kcal_per_100, protein_per_100, carbs_per_100, fat_per_100, has_full_nutrients, updated_at) VALUES
		('8012345678907', 'Oat flakes', 'Farm', '500 g', 372, 13.5, 58.7, 7, 1, 0),
		('8015555555556', 'Oat bran', 'Farm', '500 g', 0, 0, 0, 0, 0, 0)`); err != nil {
		t.Fatal(err)
	}
	// Suggestions of the nutrition plan for «Breakfast».
	plan, err := a.NutritionistPlans.GetOrCreateActivePlan(ctx, ada.ID)
	must(err)
	mealType := id(a.NutritionistPlans.CreateMealType(ctx, plan.ID, "Breakfast"))
	variation := id(a.NutritionistPlans.CreateVariation(ctx, plan.ID, mealType, nil))
	id(a.NutritionistPlans.AddItem(ctx, plan.ID, variation, nil, &oats, 80, "g"))

	// The week: the current one, so that «today» is in it.
	mon := db.MondayOf(todayDate())
	wkStr := func(i int) string { return formatDate(mon.AddDate(0, 0, 7*i)) }
	weekPlan := func(u *domain.User, i int) int64 {
		p, err := a.WeeklyPlans.GetOrCreateForWeek(ctx, u.ID, mon.AddDate(0, 0, 7*i))
		must(err)
		return p.ID
	}
	today := (int(time.Now().Weekday()) + 6) % 7
	other := (today + 1) % 7
	todayStr := formatDate(mon.AddDate(0, 0, today))
	otherStr := formatDate(mon.AddDate(0, 0, other))
	p0 := weekPlan(ada, 0)
	breakfast := id(a.WeeklyPlans.AddMeal(ctx, p0, ada.ID, today, "Breakfast"))
	lunch := id(a.WeeklyPlans.AddMeal(ctx, p0, ada.ID, today, "Lunch"))
	dinner := id(a.WeeklyPlans.AddMeal(ctx, p0, ada.ID, today, "Dinner"))
	id(a.WeeklyPlans.AddMeal(ctx, p0, ada.ID, today, "Snack"))
	id(a.WeeklyPlans.AddItem(ctx, breakfast, ada.ID, nil, &oats, 80, "g"))
	id(a.WeeklyPlans.AddItem(ctx, breakfast, ada.ID, nil, &egg, 2, "pz"))
	soupItem := id(a.WeeklyPlans.AddItem(ctx, breakfast, ada.ID, &soup, nil, 2, "g"))
	id(a.WeeklyPlans.AddItem(ctx, lunch, ada.ID, nil, &rice, 100, "g"))
	id(a.WeeklyPlans.AddCheat(ctx, lunch, ada.ID, nil, &oats, 50))
	id(a.WeeklyPlans.AddCheat(ctx, lunch, ada.ID, &soup, nil, 1))
	custom := id(a.WeeklyPlans.AddItem(ctx, dinner, ada.ID, &soup, nil, 1, "g"))
	shellItem := id(a.WeeklyPlans.AddItem(ctx, dinner, ada.ID, &shell, nil, 1, "g"))
	must(a.MealItemCustomizations.Replace(ctx, custom, ada.ID, []db.CustomizationInput{{IngredientID: oats, Quantity: 50}}))
	must(a.WaterLogs.Log(ctx, ada.ID, mon.AddDate(0, 0, today), 750))
	acts, err := a.WorkoutLogs.ListActivities(ctx)
	must(err)
	if len(acts) == 0 {
		t.Fatal("no workout activities seeded")
	}
	id(a.WorkoutLogs.CreateWorkoutLog(ctx, ada.ID, acts[0].ID, mon.AddDate(0, 0, today), 30, nil))
	// Last week has a meal (to copy from) and the week after is empty with a template.
	id(a.WeeklyPlans.AddMeal(ctx, weekPlan(ada, -1), ada.ID, 0, "Past meal"))
	tpl := id(a.MealTemplates.Create(ctx, ada.ID, "Standard week"))
	id(a.MealTemplates.AddSlot(ctx, tpl, ada.ID, 0, "Breakfast"))

	// Reference data (#157, #158) and what users typed are not interface text.
	data := []string{"Ada", "Bea", "Bob", "Oats", "Egg", "Rice", "Oat soup", "Empty shell", "Public pie", "Oat flakes", "Oat bran", "Farm",
		"Breakfast", "Lunch", "Dinner", "Snack", "Past meal", "Standard week",
		// Day and month names, and the week range (#157).
		"Lunedì", "Martedì", "Mercoledì", "Giovedì", "Venerdì", "Sabato", "Domenica",
		"gennaio", "febbraio", "marzo", "aprile", "maggio", "giugno", "luglio", "agosto", "settembre", "ottobre", "novembre", "dicembre",
		"Lun", "Mar", "Mer", "Gio", "Ven", "Sab", "Dom"}
	for _, ac := range acts { // workout activities (#158)
		data = append(data, ac.Name)
	}

	// The CSV is a file format with Italian day names: it stays as it is.
	csvDay := []string{"Lunedi", "Martedi", "Mercoledi", "Giovedi", "Venerdi", "Sabato", "Domenica"}[today]
	goodCSV := fmt.Sprintf("giorno,pasto,tipo,nome,quantita,unita\n%[1]s,Snack,ingrediente,Oats,50,g\n%[1]s,Nowhere,ingrediente,Oats,50,g\n%[1]s,Snack,ingrediente,Unknown,50,g\n", csvDay)
	multi := func(field, name, content string) ([]byte, string) {
		var buf bytes.Buffer
		mw := multipart.NewWriter(&buf)
		if field != "" {
			fw, _ := mw.CreateFormFile(field, name)
			_, _ = fw.Write([]byte(content))
		}
		_ = mw.Close()
		return buf.Bytes(), mw.FormDataContentType()
	}
	csvBody, csvType := multi("csv_file", "week.csv", goodCSV)
	badBody, badType := multi("csv_file", "week.csv", "a,b\nc\n")
	noBody, noType := multi("", "", "")

	mealID := func(m int64) map[string]string { return map[string]string{"mealID": strconv.FormatInt(m, 10)} }
	itemID := func(i int64) map[string]string { return map[string]string{"itemID": strconv.FormatInt(i, 10)} }
	picker := func(m int64, q string) string {
		return "/meals/" + strconv.FormatInt(m, 10) + "/picker?" + q
	}
	start := "&start=" + wkStr(0)
	postItem := func(name string, form url.Values, flash string) weekCall {
		return weekCall{name: name, method: http.MethodPost, target: "/meals/x/items?start=" + wkStr(0), form: form, params: mealID(breakfast), week: wkStr(0), want: flash, handler: a.handleAddMealItem}
	}
	postCheat := func(name string, form url.Values, flash string) weekCall {
		return weekCall{name: name, method: http.MethodPost, target: "/meals/x/cheat/items?start=" + wkStr(0), form: form, params: mealID(lunch), week: wkStr(0), want: flash, handler: a.handleAddMealCheat}
	}
	workout := func(name string, form url.Values, flash string) weekCall {
		return weekCall{name: name, method: http.MethodPost, target: "/allenamento?start=" + wkStr(0), form: form, week: wkStr(0), want: flash, handler: a.handleWorkoutLog}
	}
	apply := func(name string, form url.Values, flash string) weekCall {
		form.Set("start", wkStr(0))
		return weekCall{name: name, method: http.MethodPost, target: "/week/apply-template", form: form, week: wkStr(0), want: flash, handler: a.handleApplyTemplate}
	}
	actID := strconv.FormatInt(acts[0].ID, 10)

	calls := []weekCall{
		// The week.
		{name: "week with meals", target: "/?start=" + wkStr(0), want: "Add meal", handler: a.handleDashboard},
		{name: "week, another day", target: "/?start=" + wkStr(0) + "&day=" + otherStr, want: "No meals on this day", handler: a.handleDashboard},
		{name: "week, empty", target: "/?start=" + wkStr(1), want: "Start from a template", handler: a.handleDashboard},
		{name: "week, empty, without a template", target: "/?start=" + wkStr(1), as: bea, want: "Add meal", handler: a.handleDashboard},
		{name: "week, no goals set", target: "/?start=" + wkStr(1), as: bea, handler: a.handleDashboard},
		{name: "day card", target: "/days/" + todayStr, params: map[string]string{"date": todayStr}, htmx: true, want: "Log a cheat", handler: a.handleDayCard},
		{name: "day card, empty day", target: "/days/" + otherStr, params: map[string]string{"date": otherStr}, htmx: true, handler: a.handleDayCard},

		// The picker: dialog, page, cheat.
		{name: "picker dialog", target: "/meals/x/items/new?start=" + wkStr(0), params: mealID(breakfast), htmx: true, want: "Add to Breakfast", handler: a.handleAddItemForm},
		{name: "picker page", target: "/meals/x/items/new?start=" + wkStr(0), params: mealID(breakfast), want: "Add to Breakfast", handler: a.handleAddItemForm},
		{name: "picker dialog, cheat", target: "/meals/x/items/new?cheat=1&start=" + wkStr(0), params: mealID(lunch), htmx: true, want: "Log a cheat for Lunch", handler: a.handleAddItemForm},
		{name: "picker page, cheat", target: "/meals/x/items/new?cheat=1&start=" + wkStr(0), params: mealID(lunch), want: "Log a cheat for Lunch", handler: a.handleAddItemForm},

		// Its results.
		{name: "ingredients with suggestions", target: picker(breakfast, "kind=ingredient&q=&meal_name=Breakfast&dialog=1"+start), params: mealID(breakfast), want: "Suggested by the nutrition plan", handler: a.handleMealPicker},
		{name: "ingredients and OpenFoodFacts", target: picker(breakfast, "kind=ingredient&q=o&off=1"+start), params: mealID(breakfast), want: "imported on first use", handler: a.handleMealPicker},
		{name: "ingredients, a piece unit", target: picker(breakfast, "kind=ingredient&q=egg"+start), params: mealID(breakfast), want: "1 pc = 60 g", handler: a.handleMealPicker},
		{name: "ingredients, nothing found", target: picker(breakfast, "kind=ingredient&q=zzz"+start), params: mealID(breakfast), want: "No ingredient found for", handler: a.handleMealPicker},
		{name: "ingredients, cheat", target: picker(lunch, "kind=ingredient&q=oat&as=cheat"+start), params: mealID(lunch), want: "Log cheat", handler: a.handleMealPicker},
		{name: "a known barcode", target: picker(breakfast, "kind=ingredient&q=8012345678907&off=1"+start), params: mealID(breakfast), want: "Code", handler: a.handleMealPicker},
		{name: "a barcode, still looking", target: picker(breakfast, "kind=ingredient&q=8000000000002"+start), params: mealID(breakfast), want: "Looking up the code", handler: a.handleMealPicker},
		{name: "a barcode, not found", target: picker(breakfast, "kind=ingredient&q=8000000000002&off=1"+start), params: mealID(breakfast), want: "Product not found", handler: a.handleMealPicker},
		{name: "recipes", target: picker(breakfast, "kind=recipe&q="+start), params: mealID(breakfast), want: "kcal / serving", handler: a.handleMealPicker},
		{name: "recipes, filtered", target: picker(breakfast, "kind=recipe&q=pie"+start), params: mealID(breakfast), want: "public", handler: a.handleMealPicker},
		{name: "recipes, cheat", target: picker(lunch, "kind=recipe&q=&as=cheat"+start), params: mealID(lunch), want: "Log cheat", handler: a.handleMealPicker},
		{name: "recipes, nothing found", target: picker(breakfast, "kind=recipe&q=zzz"+start), params: mealID(breakfast), want: "No recipe found for", handler: a.handleMealPicker},
		{name: "unknown kind", target: picker(breakfast, "kind=x"+start), params: mealID(breakfast), want: "invalid type", handler: a.handleMealPicker},
		{name: "ingredients for a recipe", target: "/recipes/x/picker?q=oat&off=1", params: map[string]string{"id": strconv.FormatInt(soup, 10)}, want: "imported on first use", handler: a.handleRecipePicker},

		// The customize dialog.
		{name: "customize", target: "/meal-items/x/customize?start=" + wkStr(0), params: itemID(soupItem), htmx: true, want: "Variations for", handler: a.handleMealItemCustomizeForm},
		{name: "customize, no ingredients", target: "/meal-items/x/customize?start=" + wkStr(0), params: itemID(shellItem), htmx: true, want: "No ingredients in the recipe", handler: a.handleMealItemCustomizeForm},
		{name: "customize, customized", target: "/meal-items/x/customize?start=" + wkStr(0), params: itemID(custom), htmx: true, want: "Restore recipe", handler: a.handleMealItemCustomizeForm},

		// Mutations through HTMX answer with the day.
		{name: "add a meal", method: http.MethodPost, target: "/meals?start=" + wkStr(0), form: url.Values{"day_of_week": {strconv.Itoa(today)}, "name": {"Snack"}, "start": {wkStr(0)}}, htmx: true, want: "Add food", handler: a.handleAddMeal},
		{name: "add water", method: http.MethodPost, target: "/water", form: url.Values{"date": {todayStr}, "amount_ml": {"250"}}, htmx: true, want: "Water", handler: a.handleWaterLog},
		{name: "undo water", method: http.MethodPost, target: "/water/delete-last", form: url.Values{"date": {todayStr}}, htmx: true, want: "Undo the last addition", handler: a.handleWaterDelete},
		{name: "log a workout", method: http.MethodPost, target: "/allenamento?start=" + wkStr(0), form: url.Values{"activity_id": {actID}, "logged_at": {todayStr}, "duration_minutes": {"20"}}, htmx: true, want: "Workout", handler: a.handleWorkoutLog},
		{name: "add an ingredient", method: http.MethodPost, target: "/meals/x/items?start=" + wkStr(0), form: url.Values{"kind": {"ingredient"}, "ref_id": {strconv.FormatInt(rice, 10)}, "quantity": {"30"}}, params: mealID(breakfast), htmx: true, want: "Add food", handler: a.handleAddMealItem},
		{name: "add a recipe", method: http.MethodPost, target: "/meals/x/items?start=" + wkStr(0), form: url.Values{"kind": {"recipe"}, "ref_id": {strconv.FormatInt(shell, 10)}, "quantity": {"1"}}, params: mealID(breakfast), htmx: true, want: "Add food", handler: a.handleAddMealItem},
		{name: "add a cheat", method: http.MethodPost, target: "/meals/x/cheat/items?start=" + wkStr(0), form: url.Values{"kind": {"ingredient"}, "ref_id": {strconv.FormatInt(egg, 10)}, "quantity": {"1"}, "qty_unit": {"pz"}}, params: mealID(lunch), htmx: true, want: "Cancel cheat", handler: a.handleAddMealCheat},

		// Messages.
		apply("no template chosen", url.Values{}, "Select a template."),
		{name: "invalid template", method: http.MethodPost, target: "/week/apply-template", form: url.Values{"template_id": {"9999"}, "start": {wkStr(1)}}, week: wkStr(1), want: "Invalid template.", handler: a.handleApplyTemplate},
		apply("template over a full week", url.Values{"template_id": {strconv.FormatInt(tpl, 10)}}, "already has meals"),
		{name: "copy over a full week", method: http.MethodPost, target: "/week/copy-previous", form: url.Values{"start": {wkStr(0)}}, week: wkStr(0), want: "already has meals", handler: a.handleCopyPrevious},
		{name: "copy from an empty week", method: http.MethodPost, target: "/week/copy-previous", form: url.Values{"start": {wkStr(-8)}}, week: wkStr(-8), want: "nothing to copy", handler: a.handleCopyPrevious},
		{name: "generate without goals", method: http.MethodPost, target: "/week/generate", form: url.Values{"start": {wkStr(2)}}, as: bea, week: wkStr(2), want: "calorie target", handler: a.handleGeneratePlan},
		{name: "generate over a full week", method: http.MethodPost, target: "/week/generate", form: url.Values{"start": {wkStr(0)}}, week: wkStr(0), want: "clear it before generating", handler: a.handleGeneratePlan},
		{name: "invalid day", method: http.MethodPost, target: "/meals", form: url.Values{"day_of_week": {"9"}, "name": {"x"}, "start": {wkStr(0)}}, week: wkStr(0), want: "Invalid day.", handler: a.handleAddMeal},
		{name: "meal without a name", method: http.MethodPost, target: "/meals", form: url.Values{"day_of_week": {"1"}, "name": {" "}, "start": {wkStr(0)}}, week: wkStr(0), want: "Meal name is required.", handler: a.handleAddMeal},
		{name: "meal name too long", method: http.MethodPost, target: "/meals", form: url.Values{"day_of_week": {"1"}, "name": {strings.Repeat("x", 201)}, "start": {wkStr(0)}}, week: wkStr(0), want: "Name too long", handler: a.handleAddMeal},
		postItem("item, bad quantity", url.Values{"kind": {"ingredient"}, "quantity": {"0"}}, "Invalid quantity."),
		postItem("item, bad type", url.Values{"kind": {"x"}, "quantity": {"1"}}, "Invalid type."),
		postItem("item, no recipe", url.Values{"kind": {"recipe"}, "quantity": {"1"}}, "Select a recipe."),
		postItem("item, no ingredient", url.Values{"kind": {"ingredient"}, "quantity": {"1"}}, "Select an ingredient."),
		postItem("item, import fails", url.Values{"kind": {"ingredient"}, "quantity": {"1"}, "barcode": {"8000000000002"}}, "OpenFoodFacts import failed"),
		postItem("item, unknown ingredient", url.Values{"kind": {"ingredient"}, "quantity": {"1"}, "ref_id": {"99999"}}, "Could not add the item"),
		postCheat("cheat, bad quantity", url.Values{"kind": {"ingredient"}, "quantity": {"x"}}, "Invalid quantity."),
		postCheat("cheat, bad type", url.Values{"kind": {"x"}, "quantity": {"1"}}, "Invalid type."),
		postCheat("cheat, no recipe", url.Values{"kind": {"recipe"}, "quantity": {"1"}}, "Select a recipe."),
		postCheat("cheat, no ingredient", url.Values{"kind": {"ingredient"}, "quantity": {"1"}}, "Select an ingredient."),
		postCheat("cheat, import fails", url.Values{"kind": {"ingredient"}, "quantity": {"1"}, "barcode": {"8000000000002"}}, "OpenFoodFacts import failed"),
		postCheat("cheat, unknown ingredient", url.Values{"kind": {"ingredient"}, "quantity": {"1"}, "ref_id": {"99999"}}, "Could not add the cheat"),
		workout("workout, no activity", url.Values{"logged_at": {todayStr}, "duration_minutes": {"20"}}, "Select a valid activity."),
		workout("workout, bad date", url.Values{"activity_id": {actID}, "logged_at": {"x"}, "duration_minutes": {"20"}}, "Invalid date."),
		workout("workout, bad duration", url.Values{"activity_id": {actID}, "logged_at": {todayStr}, "duration_minutes": {"999"}}, "Invalid duration"),
		{name: "import, a CSV", method: http.MethodPost, target: "/week/" + wkStr(0) + "/import", params: map[string]string{"start": wkStr(0)}, body: csvBody, contentType: csvType, week: wkStr(0), want: "Imported 1 item. Meals not found", italian: "Importato 1 elemento. Pasti non trovati", handler: a.handleWeekImport},
		{name: "import, no file", method: http.MethodPost, target: "/week/" + wkStr(0) + "/import", params: map[string]string{"start": wkStr(0)}, body: noBody, contentType: noType, week: wkStr(0), want: "Select a CSV file.", handler: a.handleWeekImport},
		{name: "import, a broken CSV", method: http.MethodPost, target: "/week/" + wkStr(0) + "/import", params: map[string]string{"start": wkStr(0)}, body: badBody, contentType: badType, week: wkStr(0), want: "Invalid CSV file.", handler: a.handleWeekImport},

		// Plain-text answers.
		{name: "water, bad amount", method: http.MethodPost, target: "/water", form: url.Values{"amount_ml": {"0"}}, want: "invalid amount", handler: a.handleWaterLog},
		{name: "water, bad date", method: http.MethodPost, target: "/water", form: url.Values{"amount_ml": {"100"}, "date": {"x"}}, want: "invalid date", handler: a.handleWaterLog},
		{name: "export, bad date", target: "/week/x/export", params: map[string]string{"start": "x"}, want: "invalid date", handler: a.handleWeekExport},
		{name: "remove a missing meal", method: http.MethodPost, target: "/meals/x/delete", params: mealID(99999), want: "meal not found", handler: a.handleRemoveMeal},
		{name: "remove a missing item", method: http.MethodPost, target: "/meal-items/x/delete", params: itemID(99999), want: "item not found", handler: a.handleRemoveMealItem},
		{name: "remove a missing cheat", method: http.MethodPost, target: "/meal-cheat-items/x/delete", params: map[string]string{"cheatID": "99999"}, want: "cheat not found", handler: a.handleRemoveMealCheat},
		{name: "reorder, no day", method: http.MethodPost, target: "/meals/reorder", form: url.Values{"ids": {"1"}}, want: "day_of_week missing", handler: a.handleReorderMeals},
		{name: "reorder, wrong ids", method: http.MethodPost, target: "/meals/reorder?day_of_week=" + strconv.Itoa(today) + "&start=" + wkStr(0), form: url.Values{"ids": {"99999"}}, want: "invalid order", handler: a.handleReorderMeals},
		{name: "customize, bad form", method: http.MethodPost, target: "/meal-items/x/customize", params: itemID(soupItem), body: []byte("%zz"), contentType: "application/x-www-form-urlencoded", want: "invalid form", handler: a.handleMealItemCustomizeSave},
	}

	do := func(c weekCall, lang string, cookies []*http.Cookie) *httptest.ResponseRecorder {
		method := c.method
		if method == "" {
			method = http.MethodGet
		}
		var req *http.Request
		switch {
		case c.body != nil:
			req = httptest.NewRequest(method, c.target, bytes.NewReader(c.body))
			req.Header.Set("Content-Type", c.contentType)
		case c.form != nil:
			req = httptest.NewRequest(method, c.target, strings.NewReader(c.form.Encode()))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		default:
			req = httptest.NewRequest(method, c.target, nil)
		}
		if lang != "" {
			req.AddCookie(&http.Cookie{Name: langCookie, Value: lang})
		}
		for _, ck := range cookies {
			req.AddCookie(ck)
		}
		if c.htmx {
			req.Header.Set("HX-Request", "true")
		}
		if len(c.params) > 0 {
			rctx := chi.NewRouteContext()
			for k, v := range c.params {
				rctx.URLParams.Add(k, v)
			}
			req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
		}
		viewer := ada
		if c.as != nil {
			viewer = c.as
		}
		req = requestWithUser(req, viewer)
		rec := httptest.NewRecorder()
		languageMiddleware(a.Sessions.LoadAndSave(c.handler)).ServeHTTP(rec, req)
		return rec
	}
	// render runs the call and, for the ones that leave a message, shows the
	// week it is displayed on.
	render := func(c weekCall, lang string) string {
		rec := do(c, lang, nil)
		if c.week == "" {
			return rec.Body.String()
		}
		if rec.Code != http.StatusSeeOther && rec.Code != http.StatusNoContent {
			t.Fatalf("%s: code %d, want a redirect:\n%s", c.name, rec.Code, rec.Body.String())
		}
		next := weekCall{target: "/?start=" + c.week, as: c.as, handler: a.handleDashboard}
		return do(next, lang, rec.Result().Cookies()).Body.String()
	}

	for _, c := range calls {
		t.Run(c.name, func(t *testing.T) {
			body := render(c, "en")
			if !strings.Contains(body, c.want) {
				t.Errorf("%q missing from the English page:\n%s", c.want, body)
			}
			if found := italianIn(body, data); len(found) > 0 {
				t.Errorf("Italian in the English page:\n  %s", strings.Join(found, "\n  "))
			}
			it := render(c, "")
			if c.italian != "" && !strings.Contains(it, c.italian) || c.italian == "" && len(italianIn(it, data)) == 0 {
				t.Errorf("the page without the cookie is not Italian:\n%s", it)
			}
		})
	}

	// What changes the week gets a week of its own for each language.
	flows := []struct {
		name, path        string
		handler           http.HandlerFunc
		setup             func(plan int64, week string) url.Values
		want, italianWant string
	}{
		{"apply a template", "/week/apply-template", a.handleApplyTemplate, func(_ int64, week string) url.Values {
			return url.Values{"template_id": {strconv.FormatInt(tpl, 10)}, "start": {week}}
		}, "Template applied to the week.", "Template applicato alla settimana."},
		{"copy the previous week", "/week/copy-previous", a.handleCopyPrevious, func(_ int64, week string) url.Values {
			d, _ := time.Parse("2006-01-02", week)
			prev, err := a.WeeklyPlans.GetOrCreateForWeek(ctx, ada.ID, d.AddDate(0, 0, -7))
			must(err)
			id(a.WeeklyPlans.AddMeal(ctx, prev.ID, ada.ID, 0, "Breakfast"))
			return url.Values{"start": {week}}
		}, "Plan copied from the previous week.", "Piano copiato dalla settimana precedente."},
		{"clear the week", "/week/reset", a.handleResetWeek, func(plan int64, week string) url.Values {
			id(a.WeeklyPlans.AddMeal(ctx, plan, ada.ID, 0, "Breakfast"))
			return url.Values{"start": {week}}
		}, "Week cleared.", "Settimana svuotata."},
		{"generate the week", "/week/generate", a.handleGeneratePlan, func(_ int64, week string) url.Values {
			return url.Values{"start": {week}}
		}, "Plan generated automatically for the week.", "Piano generato automaticamente per la settimana."},
	}
	for i, f := range flows {
		t.Run(f.name, func(t *testing.T) {
			for j, lang := range []string{"en", ""} {
				idx := 10 + 3*(2*i+j)
				week := wkStr(idx)
				form := f.setup(weekPlan(ada, idx), week)
				c := weekCall{method: http.MethodPost, target: f.path, form: form, week: week, handler: f.handler}
				body := render(c, lang)
				if lang == "en" {
					if !strings.Contains(body, f.want) {
						t.Errorf("%q missing from the English page:\n%s", f.want, body)
					}
					if found := italianIn(body, data); len(found) > 0 {
						t.Errorf("Italian in the English page:\n  %s", strings.Join(found, "\n  "))
					}
				} else if !strings.Contains(body, f.italianWant) {
					t.Errorf("%q missing from the Italian page:\n%s", f.italianWant, body)
				}
			}
		})
	}

	t.Run("a bare instance", func(t *testing.T) {
		b := newTestApp(t)
		b.OFF = off.NewClient(b.Store.DB, "http://127.0.0.1:1", nil)
		cat := mustCreateTestUser(t, b, "cat@example.test", "x")
		p, err := b.WeeklyPlans.GetOrCreateForWeek(ctx, cat.ID, mon)
		must(err)
		m := id(b.WeeklyPlans.AddMeal(ctx, p.ID, cat.ID, 0, "Breakfast"))
		if _, err := b.Store.DB.Exec(`DROP TABLE off_products_fts`); err != nil {
			t.Fatal(err)
		}
		get := func(lang, query string) string {
			req := httptest.NewRequest(http.MethodGet, picker(m, query), nil)
			if lang != "" {
				req.AddCookie(&http.Cookie{Name: langCookie, Value: lang})
			}
			rctx := chi.NewRouteContext()
			rctx.URLParams.Add("mealID", strconv.FormatInt(m, 10))
			req = requestWithUser(req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx)), cat)
			rec := httptest.NewRecorder()
			languageMiddleware(b.Sessions.LoadAndSave(http.HandlerFunc(b.handleMealPicker))).ServeHTTP(rec, req)
			return rec.Body.String()
		}
		for _, c := range []struct{ name, query, want string }{
			{"OpenFoodFacts unavailable", "kind=ingredient&q=oat&off=1", "OpenFoodFacts"},
			{"no recipes yet", "kind=recipe&q=", "No recipes yet"},
			{"nothing typed", "kind=ingredient&q=", "Type to search"},
		} {
			en, it := get("en", c.query), get("", c.query)
			if !strings.Contains(en, c.want) {
				t.Errorf("%s: %q missing from the English page:\n%s", c.name, c.want, en)
			}
			if found := italianIn(en, nil); len(found) > 0 {
				t.Errorf("%s: Italian in the English page:\n  %s", c.name, strings.Join(found, "\n  "))
			}
			if len(italianIn(it, nil)) == 0 {
				t.Errorf("%s: the page without the cookie is not Italian:\n%s", c.name, it)
			}
		}
	})

	t.Run("the picker's search texts reach the script", func(t *testing.T) {
		body := render(weekCall{target: "/meals/x/items/new", params: mealID(breakfast), htmx: true, handler: a.handleAddItemForm}, "en")
		for _, want := range []string{`data-placeholder-ingredient="Search your ingredients and OpenFoodFacts"`, `data-placeholder-recipe="Search for a recipe"`} {
			if !strings.Contains(body, want) {
				t.Errorf("%s missing", want)
			}
		}
	})
}
