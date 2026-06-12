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

	// Il piano attivo viene creato al primo accesso.
	active, err := np.GetOrCreateActivePlan(ctx, coach)
	if err != nil {
		t.Fatalf("GetOrCreateActivePlan: %v", err)
	}
	if !active.IsActive {
		t.Error("il piano creato deve essere attivo")
	}

	// Nuovo piano libreria: non attivo.
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
		t.Errorf("ordinamento/flag attivo errato: %+v", plans)
	}

	// SetActivePlan sposta il flag mantenendo l'invariante.
	if err := np.SetActivePlan(ctx, coach, libID); err != nil {
		t.Fatalf("SetActivePlan: %v", err)
	}
	nowActive, err := np.GetOrCreateActivePlan(ctx, coach)
	if err != nil {
		t.Fatalf("GetOrCreateActivePlan dopo switch: %v", err)
	}
	if nowActive.ID != libID {
		t.Errorf("piano attivo: want %d, got %d", libID, nowActive.ID)
	}

	// SetActivePlan su piano altrui: rifiutato.
	other := newTestUser(t, store, "other@test.it")
	if err := np.SetActivePlan(ctx, other, libID); !errors.Is(err, ErrNutritionistPlanNotFound) {
		t.Errorf("SetActivePlan piano altrui: want ErrNutritionistPlanNotFound, got %v", err)
	}

	// Il piano attivo non si puo' cancellare; quello inattivo si'.
	if err := np.DeletePlan(ctx, libID, coach); !errors.Is(err, ErrNutritionistPlanNotFound) {
		t.Errorf("DeletePlan attivo: want errore, got %v", err)
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
	// Un cliente ha al massimo un coach.
	if err := coachRepo.AddClient(ctx, otherCoach, client); !errors.Is(err, ErrClientHasCoach) {
		t.Errorf("AddClient duplicato: want ErrClientHasCoach, got %v", err)
	}

	// Senza assegnazione: GetAssignment ha AssignedPlanID nil.
	assignment, err := coachRepo.GetAssignment(ctx, client)
	if err != nil {
		t.Fatalf("GetAssignment: %v", err)
	}
	if assignment == nil || assignment.AssignedPlanID != nil {
		t.Errorf("assignment inatteso: %+v", assignment)
	}

	// Assegna un piano della libreria del coach.
	planID, err := np.CreatePlan(ctx, coach, "Definizione 2000")
	if err != nil {
		t.Fatalf("CreatePlan: %v", err)
	}
	if err := coachRepo.AssignPlan(ctx, coach, client, planID); err != nil {
		t.Fatalf("AssignPlan: %v", err)
	}
	assignment, err = coachRepo.GetAssignment(ctx, client)
	if err != nil {
		t.Fatalf("GetAssignment dopo assign: %v", err)
	}
	if assignment.AssignedPlanID == nil || *assignment.AssignedPlanID != planID {
		t.Errorf("AssignedPlanID: want %d, got %+v", planID, assignment.AssignedPlanID)
	}

	// Un piano non del coach non e' assegnabile.
	foreignPlan, err := np.CreatePlan(ctx, otherCoach, "Piano altrui")
	if err != nil {
		t.Fatalf("CreatePlan altrui: %v", err)
	}
	if err := coachRepo.AssignPlan(ctx, coach, client, foreignPlan); !errors.Is(err, ErrCoachClientNotFound) {
		t.Errorf("AssignPlan piano altrui: want errore, got %v", err)
	}

	// ListClients riporta il piano assegnato.
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
		t.Fatalf("GetAssignment dopo remove: %v", err)
	}
	if assignment != nil {
		t.Errorf("dopo remove il cliente non deve avere coach: %+v", assignment)
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
		t.Errorf("ruolo dopo update: want coach, got %q", u.Role)
	}
	if err := users.UpdateRole(ctx, id, "superadmin"); err == nil {
		t.Error("UpdateRole con ruolo non valido deve fallire")
	}
}
