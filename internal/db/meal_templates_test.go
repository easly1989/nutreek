package db

import (
	"context"
	"errors"
	"testing"

	"github.com/amon2126/nutreek/internal/domain"
)

func TestMealTemplatesCRUDAndActivate(t *testing.T) {
	store := newTestStore(t)
	users := NewUsers(store.DB)
	tpls := NewMealTemplates(store.DB)
	ctx := context.Background()

	alice, _ := users.Create(ctx, "alice@x.com", "h", "Alice")
	bob, _ := users.Create(ctx, "bob@x.com", "h", "Bob")

	// Alice creates two templates, Bob one.
	a1, err := tpls.Create(ctx, alice, "5 pasti")
	if err != nil {
		t.Fatalf("create a1: %v", err)
	}
	a2, err := tpls.Create(ctx, alice, "Solo lavorativi")
	if err != nil {
		t.Fatalf("create a2: %v", err)
	}
	bTpl, err := tpls.Create(ctx, bob, "Bob standard")
	if err != nil {
		t.Fatalf("create bob: %v", err)
	}

	// List for Alice → 2; for Bob → 1.
	aList, _ := tpls.ListByUser(ctx, alice)
	if len(aList) != 2 {
		t.Errorf("Alice has %d, want 2", len(aList))
	}
	bList, _ := tpls.ListByUser(ctx, bob)
	if len(bList) != 1 || bList[0].ID != bTpl {
		t.Errorf("unexpected Bob list: %+v", bList)
	}

	// SetActive a1, then a2 → only a2 active for Alice; bTpl stays inactive.
	if err := tpls.SetActive(ctx, a1, alice); err != nil {
		t.Fatalf("SetActive a1: %v", err)
	}
	if err := tpls.SetActive(ctx, a2, alice); err != nil {
		t.Fatalf("SetActive a2: %v", err)
	}
	aList, _ = tpls.ListByUser(ctx, alice)
	for _, t1 := range aList {
		if t1.ID == a2 && !t1.IsActive {
			t.Error("a2 should be active")
		}
		if t1.ID == a1 && t1.IsActive {
			t.Error("a1 should be deactivated after SetActive on a2")
		}
	}

	// Bob tries to activate Alice's template → forbidden.
	if err := tpls.SetActive(ctx, a1, bob); !errors.Is(err, ErrMealTemplateForbidden) {
		t.Errorf("want forbidden, got %v", err)
	}

	// Bob adds a slot to Alice's template → forbidden.
	if _, err := tpls.AddSlot(ctx, a1, bob, 0, "x"); !errors.Is(err, ErrMealTemplateForbidden) {
		t.Errorf("want forbidden, got %v", err)
	}

	// Alice adds 3 slots and checks the order.
	if _, err := tpls.AddSlot(ctx, a2, alice, domain.DayOfWeekEvery, "Colazione"); err != nil {
		t.Fatalf("slot1: %v", err)
	}
	if _, err := tpls.AddSlot(ctx, a2, alice, domain.DayOfWeekEvery, "Pranzo"); err != nil {
		t.Fatalf("slot2: %v", err)
	}
	wedSlotID, err := tpls.AddSlot(ctx, a2, alice, 2, "Allenamento")
	if err != nil {
		t.Fatalf("slot3: %v", err)
	}
	got, err := tpls.GetWithSlots(ctx, a2, alice)
	if err != nil {
		t.Fatalf("GetWithSlots: %v", err)
	}
	if len(got.Slots) != 3 {
		t.Fatalf("attese 3 slot, got %d", len(got.Slots))
	}
	// Order: -1 < 2 → Colazione/Pranzo first, then Allenamento.
	wantOrder := []string{"Colazione", "Pranzo", "Allenamento"}
	for i, s := range got.Slots {
		if s.Name != wantOrder[i] {
			t.Errorf("slot[%d] = %q want %q", i, s.Name, wantOrder[i])
		}
	}

	// Bob tries to remove Alice's slot → not found (privilege check via JOIN).
	if err := tpls.RemoveSlot(ctx, wedSlotID, bob); !errors.Is(err, ErrSlotNotFound) {
		t.Errorf("want ErrSlotNotFound, got %v", err)
	}
	if err := tpls.RemoveSlot(ctx, wedSlotID, alice); err != nil {
		t.Fatalf("Alice RemoveSlot: %v", err)
	}

	// Bob does not see Alice's templates.
	if _, err := tpls.GetWithSlots(ctx, a2, bob); !errors.Is(err, ErrMealTemplateForbidden) {
		t.Errorf("Bob GetWithSlots on a2 should be forbidden, got %v", err)
	}

	// Alice renames, then deletes.
	if err := tpls.Rename(ctx, a2, alice, "5 pasti v2"); err != nil {
		t.Fatalf("Rename: %v", err)
	}
	if err := tpls.Delete(ctx, a2, alice); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := tpls.GetWithSlots(ctx, a2, alice); !errors.Is(err, ErrMealTemplateNotFound) {
		t.Errorf("after delete want ErrMealTemplateNotFound, got %v", err)
	}
}

func TestReorderSlots(t *testing.T) {
	store := newTestStore(t)
	users := NewUsers(store.DB)
	tpls := NewMealTemplates(store.DB)
	ctx := context.Background()

	alice, _ := users.Create(ctx, "alice@x.com", "h", "Alice")
	bob, _ := users.Create(ctx, "bob@x.com", "h", "Bob")
	tplID, _ := tpls.Create(ctx, alice, "Test")

	// 3 slots for day -1 (every).
	s1, _ := tpls.AddSlot(ctx, tplID, alice, domain.DayOfWeekEvery, "A")
	s2, _ := tpls.AddSlot(ctx, tplID, alice, domain.DayOfWeekEvery, "B")
	s3, _ := tpls.AddSlot(ctx, tplID, alice, domain.DayOfWeekEvery, "C")

	if err := tpls.ReorderSlots(ctx, tplID, alice, domain.DayOfWeekEvery, []int64{s3, s1, s2}); err != nil {
		t.Fatalf("ReorderSlots: %v", err)
	}
	got, _ := tpls.GetWithSlots(ctx, tplID, alice)
	if len(got.Slots) != 3 || got.Slots[0].ID != s3 || got.Slots[1].ID != s1 || got.Slots[2].ID != s2 {
		t.Errorf("order = [%d %d %d], want [%d %d %d]",
			got.Slots[0].ID, got.Slots[1].ID, got.Slots[2].ID, s3, s1, s2)
	}

	// Mismatch + ownership.
	if err := tpls.ReorderSlots(ctx, tplID, alice, domain.DayOfWeekEvery, []int64{s1}); !errors.Is(err, ErrReorderMismatch) {
		t.Errorf("partial: want ErrReorderMismatch, got %v", err)
	}
	if err := tpls.ReorderSlots(ctx, tplID, bob, domain.DayOfWeekEvery, []int64{s1, s2, s3}); !errors.Is(err, ErrMealTemplateForbidden) {
		t.Errorf("bob: want forbidden, got %v", err)
	}
}
