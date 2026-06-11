package db

import (
	"context"
	"testing"
	"time"
)

func TestWaterLogTotalAndDelete(t *testing.T) {
	store := newTestStore(t)
	users := NewUsers(store.DB)
	water := NewWaterLogs(store.DB)
	ctx := context.Background()

	uid, err := users.Create(ctx, "h2o@example.com", "h", "Bevitore")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	day := time.Date(2026, 6, 4, 0, 0, 0, 0, time.UTC)

	// Nessun log: totale 0.
	if total, _ := water.TotalForDate(ctx, uid, day); total != 0 {
		t.Fatalf("totale iniziale atteso 0, got %d", total)
	}

	// Tre quick-log si sommano.
	for _, ml := range []int{200, 250, 500} {
		if err := water.Log(ctx, uid, day, ml); err != nil {
			t.Fatalf("Log %d: %v", ml, err)
		}
	}
	total, err := water.TotalForDate(ctx, uid, day)
	if err != nil {
		t.Fatalf("TotalForDate: %v", err)
	}
	if total != 950 {
		t.Fatalf("totale atteso 950, got %d", total)
	}

	// DeleteLast rimuove solo l'ultimo (500).
	if err := water.DeleteLast(ctx, uid, day); err != nil {
		t.Fatalf("DeleteLast: %v", err)
	}
	if total, _ := water.TotalForDate(ctx, uid, day); total != 450 {
		t.Fatalf("dopo delete atteso 450, got %d", total)
	}

	// amount non positivo è rifiutato.
	if err := water.Log(ctx, uid, day, 0); err == nil {
		t.Fatalf("Log(0) doveva fallire")
	}
}

func TestWaterForDateRange(t *testing.T) {
	store := newTestStore(t)
	users := NewUsers(store.DB)
	water := NewWaterLogs(store.DB)
	ctx := context.Background()

	uid, _ := users.Create(ctx, "range@example.com", "h", "Tester")
	base := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC) // lunedì

	_ = water.Log(ctx, uid, base, 300)
	_ = water.Log(ctx, uid, base, 200)                   // lun: 500
	_ = water.Log(ctx, uid, base.AddDate(0, 0, 2), 1000) // mer: 1000

	m, err := water.ForDateRange(ctx, uid, base, base.AddDate(0, 0, 6))
	if err != nil {
		t.Fatalf("ForDateRange: %v", err)
	}
	if m["2026-06-01"] != 500 {
		t.Errorf("lun atteso 500, got %d", m["2026-06-01"])
	}
	if m["2026-06-03"] != 1000 {
		t.Errorf("mer atteso 1000, got %d", m["2026-06-03"])
	}
	if _, ok := m["2026-06-02"]; ok {
		t.Errorf("mar senza log non deve comparire")
	}
}

func TestWaterTargetDefaultAndUpdate(t *testing.T) {
	store := newTestStore(t)
	users := NewUsers(store.DB)
	ctx := context.Background()

	uid, _ := users.Create(ctx, "target@example.com", "h", "Tester")
	u, err := users.GetByID(ctx, uid)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if u.WaterTargetML != 2000 {
		t.Fatalf("default target atteso 2000, got %d", u.WaterTargetML)
	}

	// UpdateProfile con WaterTargetML=0 NON deve azzerare il default.
	if err := users.UpdateProfile(ctx, uid, UserProfileInput{WaterTargetML: 0}); err != nil {
		t.Fatalf("UpdateProfile(0): %v", err)
	}
	u, _ = users.GetByID(ctx, uid)
	if u.WaterTargetML != 2000 {
		t.Fatalf("dopo update con sentinella 0, atteso 2000, got %d", u.WaterTargetML)
	}

	// Valore esplicito viene salvato.
	if err := users.UpdateProfile(ctx, uid, UserProfileInput{WaterTargetML: 2500}); err != nil {
		t.Fatalf("UpdateProfile(2500): %v", err)
	}
	u, _ = users.GetByID(ctx, uid)
	if u.WaterTargetML != 2500 {
		t.Fatalf("atteso 2500, got %d", u.WaterTargetML)
	}
}
