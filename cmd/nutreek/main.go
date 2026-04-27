package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/alexedwards/scs/sqlite3store"
	"github.com/alexedwards/scs/v2"

	"github.com/amon2126/nutreek/internal/app"
	"github.com/amon2126/nutreek/internal/config"
	"github.com/amon2126/nutreek/internal/db"
	"github.com/amon2126/nutreek/internal/off"
	"github.com/amon2126/nutreek/internal/web"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	if err := run(logger); err != nil {
		logger.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	store, err := db.Open(cfg.DBPath)
	if err != nil {
		return err
	}
	defer func() { _ = store.Close() }()
	logger.Info("db opened", "path", cfg.DBPath)

	if err := store.Migrate(ctx); err != nil {
		return err
	}
	logger.Info("db migrations applied")

	sessions := scs.New()
	sessions.Store = sqlite3store.New(store.DB)
	sessions.Lifetime = 30 * 24 * time.Hour
	sessions.IdleTimeout = 7 * 24 * time.Hour
	sessions.Cookie.Name = "nutreek_session"
	sessions.Cookie.HttpOnly = true
	sessions.Cookie.SameSite = http.SameSiteLaxMode
	sessions.Cookie.Secure = cfg.Env == config.EnvProd
	sessions.Cookie.Path = "/"

	renderer, err := web.NewRenderer(web.Templates())
	if err != nil {
		return err
	}

	a := &app.App{
		Cfg:           cfg,
		Store:         store,
		Users:         db.NewUsers(store.DB),
		Ingredients:   db.NewIngredients(store.DB),
		Recipes:       db.NewRecipes(store.DB),
		MealTemplates: db.NewMealTemplates(store.DB),
		OFF:           off.NewClient(cfg.OFFBaseURL, logger),
		Sessions:      sessions,
		Render:        renderer,
		Logger:        logger,
	}

	srv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           a.Router(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		logger.Info("nutreek listening", "addr", cfg.Addr, "env", cfg.Env)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("server error", "err", err)
			stop()
		}
	}()

	<-ctx.Done()
	logger.Info("shutdown requested")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return err
	}
	logger.Info("bye")
	return nil
}
