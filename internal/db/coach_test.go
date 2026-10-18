package db

import (
	"context"
	"errors"
	"testing"
)

func TestNutritionistPlans_LibraryAndActive(t *testing.T) {
	store := newTestStore(t)
	np := NewNutritionistPlans(store.DB)
	ctx := context.Background()
	coach := newTestUser(t, store, "coach@test.it")

	// The active plan is created on first access.
	active, err := np.GetOrCreateActivePlan(ctx, coach)
	if err != nil {
		t.Fatalf("GetOrCreateActivePlan: %v", err)
	}
	if !active.IsActive {
		t.Error("the created plan must be active")
	}

	// A new library plan: not active.
	libID, err := np.CreatePlan(ctx, coach, "Massa 2700")
	if err != nil {
		t.Fatalf("CreatePlan: %v", err)
	}
	plans, err := np.ListPlans(ctx, coach)
	if err != nil {
		t.Fatalf("ListPlans: %v", err)
	}
	if len(plans) != 2 {
		t.Fatalf("ListPlans: want 2 piani, got %d", len(plans))
	}
	if !plans[0].IsActive || plans[1].IsActive {
		t.Errorf("wrong order/active flag: %+v", plans)
	}

	// SetActivePlan moves the flag and keeps the invariant.
	if err := np.SetActivePlan(ctx, coach, libID); err != nil {
		t.Fatalf("SetActivePlan: %v", err)
	}
	nowActive, err := np.GetOrCreateActivePlan(ctx, coach)
	if err != nil {
		t.Fatalf("GetOrCreateActivePlan after switch: %v", err)
	}
	if nowActive.ID != libID {
		t.Errorf("active plan: want %d, got %d", libID, nowActive.ID)
	}

	// SetActivePlan on another user's plan: rejected.
	other := newTestUser(t, store, "other@test.it")
	if err := np.SetActivePlan(ctx, other, libID); !errors.Is(err, ErrNutritionistPlanNotFound) {
		t.Errorf("SetActivePlan on another user's plan: want ErrNutritionistPlanNotFound, got %v", err)
	}

	// The active plan cannot be deleted; an inactive one can.
	if err := np.DeletePlan(ctx, libID, coach); !errors.Is(err, ErrNutritionistPlanNotFound) {
		t.Errorf("DeletePlan on the active plan: want an error, got %v", err)
	}
	if err := np.DeletePlan(ctx, active.ID, coach); err != nil {
		t.Errorf("DeletePlan inattivo: %v", err)
	}
}

func TestCoachRepository_ClientsAndAssignment(t *testing.T) {
	store := newTestStore(t)
	np := NewNutritionistPlans(store.DB)
	coachRepo := NewCoachRepository(store.DB)
	ctx := context.Background()

	coach := newTestUser(t, store, "coach@test.it")
	client := newTestUser(t, store, "cliente@test.it")
	otherCoach := newTestUser(t, store, "altro-coach@test.it")

	if err := coachRepo.AddClient(ctx, coach, client); err != nil {
		t.Fatalf("AddClient: %v", err)
	}
	// A client has at most one coach.
	if err := coachRepo.AddClient(ctx, otherCoach, client); !errors.Is(err, ErrClientHasCoach) {
		t.Errorf("AddClient duplicato: want ErrClientHasCoach, got %v", err)
	}

	// Without an assignment: GetAssignment has a nil AssignedPlanID.
	assignment, err := coachRepo.GetAssignment(ctx, client)
	if err != nil {
		t.Fatalf("GetAssignment: %v", err)
	}
	if assignment == nil || assignment.AssignedPlanID != nil {
		t.Errorf("assignment inatteso: %+v", assignment)
	}

	// Assign a plan from the coach's library.
	planID, err := np.CreatePlan(ctx, coach, "Definizione 2000")
	if err != nil {
		t.Fatalf("CreatePlan: %v", err)
	}
	if err := coachRepo.AssignPlan(ctx, coach, client, planID); err != nil {
		t.Fatalf("AssignPlan: %v", err)
	}
	assignment, err = coachRepo.GetAssignment(ctx, client)
	if err != nil {
		t.Fatalf("GetAssignment after assign: %v", err)
	}
	if assignment.AssignedPlanID == nil || *assignment.AssignedPlanID != planID {
		t.Errorf("AssignedPlanID: want %d, got %+v", planID, assignment.AssignedPlanID)
	}

	// A plan that is not the coach's cannot be assigned.
	foreignPlan, err := np.CreatePlan(ctx, otherCoach, "Piano altrui")
	if err != nil {
		t.Fatalf("CreatePlan altrui: %v", err)
	}
	if err := coachRepo.AssignPlan(ctx, coach, client, foreignPlan); !errors.Is(err, ErrCoachClientNotFound) {
		t.Errorf("AssignPlan with another coach's plan: want an error, got %v", err)
	}

	// ListClients reports the assigned plan.
	clients, err := coachRepo.ListClients(ctx, coach)
	if err != nil {
		t.Fatalf("ListClients: %v", err)
	}
	if len(clients) != 1 || clients[0].AssignedPlanName != "Definizione 2000" {
		t.Errorf("ListClients inatteso: %+v", clients)
	}

	// Unassign e remove.
	if err := coachRepo.UnassignPlan(ctx, coach, client); err != nil {
		t.Fatalf("UnassignPlan: %v", err)
	}
	if err := coachRepo.RemoveClient(ctx, coach, client); err != nil {
		t.Fatalf("RemoveClient: %v", err)
	}
	assignment, err = coachRepo.GetAssignment(ctx, client)
	if err != nil {
		t.Fatalf("GetAssignment after remove: %v", err)
	}
	if assignment != nil {
		t.Errorf("after remove the client must have no coach: %+v", assignment)
	}
}

// A client with a plan assigned by their coach gets that plan's suggestions in
// the picker, and can generate from it without a plan of their own (#186).
func TestNutritionistPlans_AssignedPlanSuggestions(t *testing.T) {
	store := newTestStore(t)
	np := NewNutritionistPlans(store.DB)
	ings := NewIngredients(store.DB)
	coachRepo := NewCoachRepository(store.DB)
	ctx := context.Background()
	coach := newTestUser(t, store, "coach@test.example")
	client := newTestUser(t, store, "client@test.example")

	planWith := func(owner int64, plan, food string) int64 {
		t.Helper()
		planID, err := np.CreatePlan(ctx, owner, plan)
		if err != nil {
			t.Fatal(err)
		}
		if err := np.SetActivePlan(ctx, owner, planID); err != nil {
			t.Fatal(err)
		}
		mt, err := np.CreateMealType(ctx, planID, "Breakfast")
		if err != nil {
			t.Fatal(err)
		}
		v, err := np.CreateVariation(ctx, planID, mt, nil)
		if err != nil {
			t.Fatal(err)
		}
		ing, err := ings.CreateLocal(ctx, food, nil, "g", 100, 1, 1, 1, &owner, nil, nil, nil, nil, nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := np.AddItem(ctx, planID, v, nil, &ing, 50, "g"); err != nil {
			t.Fatal(err)
		}
		return planID
	}
	names := func(user int64) []string {
		t.Helper()
		got, err := np.SearchSuggestedIngredients(ctx, user, "breakfast")
		if err != nil {
			t.Fatal(err)
		}
		var out []string
		for _, g := range got {
			out = append(out, g.Name)
		}
		return out
	}

	coachPlan := planWith(coach, "Coach's plan", "Coach oats")
	if has, err := np.HasVariations(ctx, client); err != nil || has {
		t.Fatalf("a client without plans has variations: %v %v", has, err)
	}
	if err := coachRepo.AddClient(ctx, coach, client); err != nil {
		t.Fatal(err)
	}
	if err := coachRepo.AssignPlan(ctx, coach, client, coachPlan); err != nil {
		t.Fatal(err)
	}
	if has, err := np.HasVariations(ctx, client); err != nil || !has {
		t.Errorf("the assigned plan has no variations for the client: %v %v", has, err)
	}
	if got := names(client); len(got) != 1 || got[0] != "Coach oats" {
		t.Errorf("suggestions with the assigned plan: %v", got)
	}

	// The client's own plan does not count while the coach's is assigned...
	planWith(client, "Own plan", "Own muesli")
	if got := names(client); len(got) != 1 || got[0] != "Coach oats" {
		t.Errorf("suggestions with the assigned plan and an own one: %v", got)
	}
	// ...and is back once the assignment goes.
	if err := coachRepo.UnassignPlan(ctx, coach, client); err != nil {
		t.Fatal(err)
	}
	if got := names(client); len(got) != 1 || got[0] != "Own muesli" {
		t.Errorf("suggestions without the assignment: %v", got)
	}
	if got := names(coach); len(got) != 1 || got[0] != "Coach oats" {
		t.Errorf("the coach's suggestions: %v", got)
	}
}

func TestUsers_UpdateRole(t *testing.T) {
	store := newTestStore(t)
	users := NewUsers(store.DB)
	ctx := context.Background()
	id := newTestUser(t, store, "ruolo@test.it")

	u, err := users.GetByID(ctx, id)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if u.Role != "user" {
		t.Errorf("ruolo default: want user, got %q", u.Role)
	}
	if err := users.UpdateRole(ctx, id, "coach"); err != nil {
		t.Fatalf("UpdateRole: %v", err)
	}
	u, _ = users.GetByID(ctx, id)
	if u.Role != "coach" {
		t.Errorf("role after update: want coach, got %q", u.Role)
	}
	if err := users.UpdateRole(ctx, id, "superadmin"); err == nil {
		t.Error("UpdateRole with an invalid role must fail")
	}
}
