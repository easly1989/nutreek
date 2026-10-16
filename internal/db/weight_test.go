package db

import (
	"context"
	"testing"
	"time"
)

// La data della pesata deve tornare intatta da entrambe le letture: le
// Statistiche e il TDEE la usano per collocare il peso nelle settimane.
func TestWeightLogsKeepMeasuredAt(t *testing.T) {
	store := newTestStore(t)
	users := NewUsers(store.DB)
	logs := NewWeightLogs(store.DB)
	ctx := context.Background()

	alice, _ := users.Create(ctx, "alice@x.com", "h", "Alice")
	day := time.Date(2026, 10, 13, 0, 0, 0, 0, time.UTC)
	if _, err := logs.LogWeight(ctx, alice, day, 73.1, nil); err != nil {
		t.Fatal(err)
	}

	list, err := logs.ListWeightLogs(ctx, alice, 10)
	if err != nil || len(list) != 1 {
		t.Fatalf("ListWeightLogs = %v, %v", list, err)
	}
	if !list[0].MeasuredAt.Equal(day) {
		t.Errorf("ListWeightLogs: data = %v, attesa %v", list[0].MeasuredAt, day)
	}
	rng, err := logs.ForRange(ctx, alice, day.AddDate(0, 0, -7), day)
	if err != nil || len(rng) != 1 {
		t.Fatalf("ForRange = %v, %v", rng, err)
	}
	if !rng[0].MeasuredAt.Equal(day) {
		t.Errorf("ForRange: data = %v, attesa %v", rng[0].MeasuredAt, day)
	}
}
