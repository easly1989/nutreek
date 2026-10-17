//go:build demo

package main

import (
	"context"
	"log/slog"
	"time"

	"github.com/alexedwards/scs/v2"

	"github.com/amon2126/nutreek/internal/app"
	"github.com/amon2126/nutreek/internal/db"
	"github.com/amon2126/nutreek/internal/demo"
)

// applyDemo turns the server into the public demo (#137): it refuses a
// database with real users, makes sessions short, leaves nobody as owner and
// every feature open, and gives the app its only way in.
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
	svc := demo.New(store.DB, cfg)
	a.Demo = svc
	go svc.Run(ctx, logger)
	logger.Info("demo build: accounts are throwaway", "idle", cfg.Idle.String())
	return nil
}
