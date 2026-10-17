package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/amon2126/nutreek/internal/demo"
	"github.com/amon2126/nutreek/internal/domain"
)

// enterAs starts a demo playing as, and returns the visitor's id and where
// the entry sent them.
func enterAs(t *testing.T, a *App, b *browser, as string) (int64, string) {
	t.Helper()
	code, loc := b.post("/", "/demo/session", url.Values{"role": {as}})
	if code != http.StatusSeeOther {
		t.Fatalf("POST /demo/session as %s: %d", as, code)
	}
	var id int64
	// The visitor is the newest group's first member (demo.Service.Start).
	if err := a.Store.DB.QueryRow(`SELECT min(user_id) FROM demo_members WHERE group_id = (SELECT max(id) FROM demo_groups)`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id, loc
}

// freeMember returns the one of visitor's group who has no coach yet.
func freeMember(t *testing.T, a *App, visitor int64) (int64, string) {
	t.Helper()
	var id int64
	var name string
	if err := a.Store.DB.QueryRow(`SELECT u.id, u.display_name FROM demo_members m JOIN users u ON u.id = m.user_id
		WHERE m.group_id = (SELECT group_id FROM demo_members WHERE user_id = ?) AND u.role <> 'coach'
		  AND NOT EXISTS (SELECT 1 FROM coach_clients WHERE client_id = u.id)`, visitor).Scan(&id, &name); err != nil {
		t.Fatal(err)
	}
	return id, name
}

// The entry offers three ways in, and each lands where it should (#186).
func TestDemoEntryWays(t *testing.T) {
	ctx := context.Background()
	a := newDemoApp(t, DemoLimits{})

	_, _, entry := newBrowser(t, a).get("/")
	for _, as := range []string{demo.AsPlan, demo.AsCoach, demo.AsClient} {
		if !strings.Contains(entry, `name="role" value="`+as+`"`) {
			t.Errorf("the entry does not offer %q", as)
		}
	}

	t.Run("plan", func(t *testing.T) {
		b := newBrowser(t, a)
		if _, loc := enterAs(t, a, b, demo.AsPlan); loc != "/" {
			t.Errorf("to %q, want the week", loc)
		}
	})

	t.Run("coach", func(t *testing.T) {
		b := newBrowser(t, a)
		id, loc := enterAs(t, a, b, demo.AsCoach)
		if loc != "/coach" {
			t.Errorf("to %q, want the coach panel", loc)
		}
		code, _, panel := b.get("/coach")
		if code != http.StatusOK {
			t.Fatalf("GET /coach: %d", code)
		}
		clients, err := a.Coach.ListClients(ctx, id)
		if err != nil || len(clients) < 2 {
			t.Fatalf("%d clients (%v)", len(clients), err)
		}
		for _, c := range clients {
			if !strings.Contains(panel, c.Client.DisplayName) || !strings.Contains(panel, c.AssignedPlanName) {
				t.Errorf("the panel does not show %s on %s", c.Client.DisplayName, c.AssignedPlanName)
			}
		}
		_, name := freeMember(t, a, id)
		if !strings.Contains(panel, `name="client_id"`) || !strings.Contains(panel, name) {
			t.Errorf("the panel does not offer %s to add", name)
		}
		if strings.Contains(panel, `type="email"`) || strings.Contains(panel, "@demo.invalid") {
			t.Error("the panel asks for, or shows, an email")
		}
	})

	t.Run("client", func(t *testing.T) {
		b := newBrowser(t, a)
		id, loc := enterAs(t, a, b, demo.AsClient)
		if loc != "/" {
			t.Errorf("to %q, want the week", loc)
		}
		if code, _, _ := b.get("/"); code != http.StatusOK {
			t.Fatalf("GET /: %d", code)
		}
		assignment, err := a.Coach.GetAssignment(ctx, id)
		if err != nil || assignment == nil || assignment.AssignedPlanID == nil {
			t.Fatalf("no plan assigned: %+v %v", assignment, err)
		}
		if _, _, plan := b.get("/piano-nutrizionale"); !strings.Contains(plan, assignment.CoachName) {
			t.Error("the nutrition plan does not say the coach assigned it")
		}

		// The picker suggests the assigned plan's foods.
		var meal, oats int64
		var mealName string
		if err := a.Store.DB.QueryRow(`SELECT m.id, m.name FROM meals m JOIN weekly_plans w ON w.id = m.weekly_plan_id
			WHERE w.user_id = ? AND m.sort_order = 0 ORDER BY w.week_start DESC LIMIT 1`, id).Scan(&meal, &mealName); err != nil {
			t.Fatal(err)
		}
		if err := a.Store.DB.QueryRow(`SELECT DISTINCT i.ingredient_id FROM nutritionist_meal_items i
			JOIN nutritionist_meal_variations v ON v.id = i.variation_id JOIN nutritionist_meal_types mt ON mt.id = v.meal_type_id
			WHERE mt.plan_id = ? AND mt.name = ? AND i.ingredient_id IS NOT NULL LIMIT 1`, *assignment.AssignedPlanID, mealName).Scan(&oats); err != nil {
			t.Fatal(err)
		}
		_, _, picker := b.get("/meals/" + strconv.FormatInt(meal, 10) + "/picker?kind=ingredient&meal_name=" + url.QueryEscape(mealName))
		suggested, _, _ := strings.Cut(picker, "Your ingredients")
		if !strings.Contains(suggested, "Suggested by the nutrition plan") || !strings.Contains(suggested, `name="ref_id" value="`+strconv.FormatInt(oats, 10)+`"`) {
			t.Error("the picker does not suggest the assigned plan's foods")
		}
	})
}

// A coach adds clients only from their own group: someone of another group
// can be neither added, seen nor touched (#186).
func TestDemoCoachGroupOnly(t *testing.T) {
	ctx := context.Background()
	a := newDemoApp(t, DemoLimits{})
	alice, bob := newBrowser(t, a), newBrowser(t, a)
	aliceID, _ := enterAs(t, a, alice, demo.AsCoach)
	bobID, _ := enterAs(t, a, bob, demo.AsCoach)
	aliceFree, _ := freeMember(t, a, aliceID)
	bobFree, _ := freeMember(t, a, bobID)
	bobClients, err := a.Coach.ListClients(ctx, bobID)
	if err != nil || len(bobClients) == 0 {
		t.Fatalf("Bob's clients: %d %v", len(bobClients), err)
	}
	bobClient := bobClients[0]
	id := func(n int64) string { return strconv.FormatInt(n, 10) }
	clientsOf := func(coach int64) map[int64]bool {
		t.Helper()
		cs, err := a.Coach.ListClients(ctx, coach)
		if err != nil {
			t.Fatal(err)
		}
		out := map[int64]bool{}
		for _, c := range cs {
			out[c.ClientID] = true
		}
		return out
	}

	// Not seen: Alice's choice holds her group's free member only.
	_, _, panel := alice.get("/coach")
	_, choice, _ := strings.Cut(panel, `name="client_id"`)
	choice, _, _ = strings.Cut(choice, "</select>")
	if options := strings.Count(choice, `<option value="`) - 1; options != 1 || !strings.Contains(choice, `value="`+id(aliceFree)+`"`) {
		t.Errorf("Alice's choice offers %d people, want her group's free one: %s", options, choice)
	}

	// Not added: by id, nor by an email the demo never asks for.
	var bobEmail string
	if err := a.Store.DB.QueryRow(`SELECT email FROM users WHERE id = ?`, bobFree).Scan(&bobEmail); err != nil {
		t.Fatal(err)
	}
	for _, form := range []url.Values{{"client_id": {id(bobFree)}}, {"client_id": {id(bobClient.ClientID)}}, {"email": {bobEmail}}, {"client_id": {id(aliceID)}}, {"client_id": {"x"}}} {
		alice.post("/coach", "/coach/clients", form)
	}
	if got := clientsOf(aliceID); got[bobFree] || got[bobClient.ClientID] || got[aliceID] || len(got) != 2 {
		t.Errorf("Alice's clients after adding outsiders: %v", got)
	}
	if _, _, page := alice.get("/coach"); !strings.Contains(page, "Choose someone of your demo group.") {
		t.Error("the panel does not say why")
	}

	// Not touched: Bob's client keeps Bob and their plan.
	alice.post("/coach", "/coach/clients/"+id(bobClient.ClientID)+"/assign-plan", url.Values{"plan_id": {""}})
	alice.post("/coach", "/coach/clients/"+id(bobClient.ClientID)+"/remove", nil)
	if after, err := a.Coach.GetAssignment(ctx, bobClient.ClientID); err != nil || after == nil || after.CoachID != bobID ||
		after.AssignedPlanID == nil || *after.AssignedPlanID != *bobClient.AssignedPlanID {
		t.Errorf("Bob's client after Alice: %+v %v", after, err)
	}

	// Alice adds the one of her group who is free, and then nobody is left.
	if code, loc := alice.post("/coach", "/coach/clients", url.Values{"client_id": {id(aliceFree)}}); code != http.StatusSeeOther || loc != "/coach" {
		t.Fatalf("adding from her group: %d to %q", code, loc)
	}
	if got := clientsOf(aliceID); !got[aliceFree] || len(got) != 3 {
		t.Errorf("Alice's clients: %v", got)
	}
	if _, _, page := alice.get("/coach"); strings.Contains(page, `name="client_id"`) {
		t.Error("the panel still offers someone to add")
	}
}

// The normal build keeps asking for the client's email; adding by email is
// in TestEnglishSettingsAdminCoach.
func TestCoachPanelAsksEmail(t *testing.T) {
	a := newTestApp(t)
	coach, err := a.Users.Create(context.Background(), "coach@example.test", "!", "Coach")
	if err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	req := requestWithUser(httptest.NewRequest(http.MethodGet, "/coach", nil), &domain.User{ID: coach, Role: domain.RoleCoach, DisplayName: "Coach"})
	a.Sessions.LoadAndSave(http.HandlerFunc(a.handleCoachDashboard)).ServeHTTP(rec, req)
	if panel := rec.Body.String(); !strings.Contains(panel, `type="email"`) || strings.Contains(panel, `name="client_id"`) {
		t.Errorf("the panel does not ask for the email (%d)", rec.Code)
	}
}
