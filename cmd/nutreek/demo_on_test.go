//go:build demo

package main

import (
	"context"
	"io"
	"log/slog"
	"path/filepath"
	"testing"

	"github.com/alexedwards/scs/v2"

	"github.com/amon2126/nutreek/internal/app"
	"github.com/amon2126/nutreek/internal/config"
	"github.com/amon2126/nutreek/internal/db"
)

// The demo switches off backups and takes the limits' defaults (#185).
func TestApplyDemo(t *testing.T) {
	ctx := context.Background()
	store, err := db.Open(filepath.Join(t.TempDir(), "demo.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := store.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	a := &app.App{Cfg: &config.Config{BackupEnabled: true, OwnerEmail: "o@x.test"}}
	if err := applyDemo(ctx, store, scs.New(), a, slog.New(slog.NewTextHandler(io.Discard, nil))); err != nil {
		t.Fatal(err)
	}
	if a.Cfg.BackupEnabled || a.Cfg.OwnerEmail != "" || a.Demo == nil {
		t.Errorf("backups %v, owner %q, demo %v", a.Cfg.BackupEnabled, a.Cfg.OwnerEmail, a.Demo != nil)
	}
	want := app.DemoLimits{MaxUsers: 200, StartsPerHour: 10, MaxBody: 1 << 20, MaxIngredients: 300, MaxRecipes: 100}
	if a.DemoLimits != want {
		t.Errorf("limits %+v, want %+v", a.DemoLimits, want)
	}
}
