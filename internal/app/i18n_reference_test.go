package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
)

// TestReferenceDataByCode shows allergens, workout activities and features
// in the page's language, translated by their code and not by the name
// stored in the database (#158). Rows without a code or a translation keep
// their stored name, and the allergen filter works in both languages.
func TestReferenceDataByCode(t *testing.T) {
	a := newTestApp(t)
	a.Cfg.PremiumEnabled = true
	ctx := context.Background()

	userID, err := a.Users.Create(ctx, "ada@example.test", "x", "Ada")
	if err != nil {
		t.Fatal(err)
	}
	user, _ := a.Users.GetByID(ctx, userID)

	mustExec := func(q string, args ...any) {
		t.Helper()
		if _, err := a.Store.DB.Exec(q, args...); err != nil {
			t.Fatalf("exec %q: %v", q, err)
		}
	}
	// Rows added by hand, with no code: they keep their stored name.
	mustExec(`INSERT INTO allergen_tags (name) VALUES ('zafferano')`)
	mustExec(`INSERT INTO workout_activities (name, kcal_per_hour, sort_order) VALUES ('Sci', 400, 12)`)
	mustExec(`INSERT INTO features (key, name, description, is_premium, enabled, sort_order)
	          VALUES ('custom_x', 'Funzione su misura', 'Solo su questa istanza', 1, 1, 9)`)

	allergens, err := a.Categories.ListAllergens(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var gluten int64
	for _, al := range allergens {
		if al.Code == "gluten" {
			gluten = al.ID
		}
	}
	if gluten == 0 {
		t.Fatal("no gluten allergen")
	}
	bread, err := a.Ingredients.CreateLocal(ctx, "Bread", nil, "g", 250, 8, 50, 1, &userID, nil, nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.Ingredients.CreateLocal(ctx, "Quinoa", nil, "g", 360, 7, 79, 0.6, &userID, nil, nil, nil, nil, nil, nil); err != nil {
		t.Fatal(err)
	}
	if err := a.Categories.SetIngredientAllergens(ctx, bread, []int64{gluten}); err != nil {
		t.Fatal(err)
	}

	acts, err := a.WorkoutLogs.ListActivities(ctx)
	if err != nil {
		t.Fatal(err)
	}
	day := time.Date(2026, 10, 12, 0, 0, 0, 0, time.UTC)
	for _, act := range acts {
		if act.Code == "running" || act.Name == "Sci" {
			if _, err := a.WorkoutLogs.CreateWorkoutLog(ctx, userID, act.ID, day, 30, nil); err != nil {
				t.Fatal(err)
			}
		}
	}

	render := func(target string, id int64, lang string, handler http.HandlerFunc) string {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, target, nil)
		if lang != "" {
			req.AddCookie(&http.Cookie{Name: langCookie, Value: lang})
		}
		if id != 0 {
			rctx := chi.NewRouteContext()
			rctx.URLParams.Add("id", strconv.FormatInt(id, 10))
			req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
		}
		rec := httptest.NewRecorder()
		languageMiddleware(a.Sessions.LoadAndSave(handler)).ServeHTTP(rec, requestWithUser(req, user))
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: status %d", target, rec.Code)
		}
		return rec.Body.String()
	}

	filtered := "/ingredients?allergen=" + strconv.FormatInt(gluten, 10)
	pages := []struct {
		name, target string
		id           int64
		handler      http.HandlerFunc
		en, it       []string // texts the English / Italian page must show
		notEn        []string // Italian names the English page must not show
	}{
		{
			name: "ingredient form", target: "/ingredients/x/edit", id: bread, handler: a.handleIngredientEditForm,
			en:    []string{"</span>gluten", "sulfur dioxide", "tree nuts", "zafferano"},
			it:    []string{"</span>glutine", "anidride solforosa", "frutta a guscio", "zafferano"},
			notEn: []string{"glutine", "anidride solforosa"},
		},
		{
			name: "list filtered by allergen", target: filtered, handler: a.handleIngredientsList,
			en:    []string{">gluten</option>", "Contains gluten", `chip-warn">gluten<`, ">zafferano</option>", "Bread"},
			it:    []string{">glutine</option>", `chip-warn">glutine<`, ">zafferano</option>", "Bread"},
			notEn: []string{"glutine", "Quinoa"},
		},
		{
			name: "week workouts", target: "/?start=2026-10-12&day=2026-10-12", handler: a.handleDashboard,
			en:    []string{"Running <span", "Elimina allenamento Running", "Gym (300 kcal/h)", "Sci <span", "Sci (400 kcal/h)"},
			it:    []string{"Corsa <span", "Palestra (300 kcal/h)", "Sci <span", "Sci (400 kcal/h)"},
			notEn: []string{"Corsa", "Palestra"},
		},
		{
			name: "upgrade", target: "/upgrade?da=auto_plan", handler: a.handleUpgradePage,
			en:    []string{"Automatic plan generation fa parte", "Generates the week to hit your targets", "Adaptive TDEE", "Funzione su misura", "Solo su questa istanza"},
			it:    []string{"Generazione piano auto fa parte", "Genera la settimana per centrare i target", "TDEE adattivo", "Funzione su misura"},
			notEn: []string{"Generazione piano auto", "TDEE adattivo"},
		},
		{
			name: "admin features", target: "/admin/panel", handler: a.handleAdminPanel,
			en:    []string{"<b>Adaptive TDEE</b>", "<b>REST API</b>", "<b>Funzione su misura</b>"},
			it:    []string{"<b>TDEE adattivo</b>", "<b>API REST</b>", "<b>Funzione su misura</b>"},
			notEn: []string{"TDEE adattivo", "API REST"},
		},
	}
	for _, p := range pages {
		t.Run(p.name, func(t *testing.T) {
			en := render(p.target, p.id, "en", p.handler)
			for _, s := range p.en {
				if !strings.Contains(en, s) {
					t.Errorf("English page lacks %q", s)
				}
			}
			for _, s := range p.notEn {
				if strings.Contains(en, s) {
					t.Errorf("English page shows %q", s)
				}
			}
			it := render(p.target, p.id, "", p.handler)
			for _, s := range p.it {
				if !strings.Contains(it, s) {
					t.Errorf("Italian page lacks %q", s)
				}
			}
			if strings.Contains(it, "Quinoa") && p.target == filtered {
				t.Error("Italian filtered list shows an ingredient without the allergen")
			}
		})
	}
}
