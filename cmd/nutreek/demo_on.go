//go:build demo

package main

import (
	"context"
	"flag"
	"log/slog"
	"time"

	"github.com/alexedwards/scs/v2"

	"github.com/amon2126/nutreek/internal/app"
	"github.com/amon2126/nutreek/internal/db"
	"github.com/amon2126/nutreek/internal/demo"
)

// applyDemo turns the server into the public demo (#137): it refuses a
// database with real users, makes sessions short, leaves nobody as owner and
// every feature open, switches off backups, scopes the shared catalog to each
// group, sets the limits and gives the app its only way in.
func applyDemo(ctx context.Context, store *db.Store, sessions *scs.SessionManager, a *app.App, logger *slog.Logger) error {
	cfg, err := demo.LoadConfig()
	if err != nil {
		return err
	}
	if err := demo.Prepare(ctx, store.DB); err != nil {
		return err
	}
	sessions.IdleTimeout = cfg.Idle
	sessions.Lifetime = 24 * time.Hour // the nightly purge ends every demo anyway
	a.Cfg.OwnerEmail = ""
	a.Cfg.AdminToken = ""
	a.Cfg.PremiumEnabled = false
	a.Cfg.BackupEnabled = false // throwaway data
	db.ScopeCatalog(store.DB, demo.Peers)
	svc := demo.New(store.DB, cfg)
	a.Demo = svc
	a.DemoLimits = app.DemoLimits{
		MaxUsers:       cfg.MaxUsers,
		StartsPerHour:  cfg.StartsPerHour,
		MaxBody:        cfg.MaxBody,
		MaxIngredients: cfg.MaxIngredients,
		MaxRecipes:     cfg.MaxRecipes,
	}
	go svc.Run(ctx, logger)
	logger.Info("demo build: accounts are throwaway", "idle", cfg.Idle.String(), "max_users", cfg.MaxUsers)
	return nil
}

var (
	seedInto = flag.String("demo-seed", "", "fill the account with this email with the demo data and exit (README screenshots)")
	seedLang = flag.String("demo-seed-lang", "en", "language of the -demo-seed data: en or it")
)

// demoCommand runs -demo-seed, which dresses a local database for the README
// screenshots: sign up in the normal build, seed the account, sign in again.
func demoCommand(ctx context.Context, store *db.Store, logger *slog.Logger) (bool, error) {
	if *seedInto == "" {
		return false, nil
	}
	if err := demo.SeedAccount(ctx, store.DB, *seedInto, *seedLang); err != nil {
		return true, err
	}
	logger.Info("demo data seeded", "lang", *seedLang)
	return true, nil
}
