package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
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
	// Sub-comando di healthcheck: usato dal Dockerfile (distroless non ha
	// shell ne' wget/curl, quindi facciamo l'HTTP probe in-process). Esce 0
	// se /healthz risponde 200, 1 altrimenti.
	healthcheck := flag.Bool("healthcheck", false, "esegue una GET su /healthz e termina con exit code 0/1")
	flag.Parse()
	if *healthcheck {
		os.Exit(runHealthcheck())
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	if err := run(logger); err != nil {
		logger.Error("fatal", "err", err)
		os.Exit(1)
	}
}

// runHealthcheck contatta il server locale e ritorna il codice exit per
// docker HEALTHCHECK. Cerca l'addr in NUTREEK_ADDR (es. ":8080") e si
// connette su localhost.
func runHealthcheck() int {
	addr := os.Getenv("NUTREEK_ADDR")
	if addr == "" {
		addr = ":8080"
	}
	// ":8080" -> "localhost:8080"
	if strings.HasPrefix(addr, ":") {
		addr = "localhost" + addr
	}
	url := "http://" + addr + "/healthz"
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		fmt.Fprintf(os.Stderr, "healthcheck: %v\n", err)
		return 1
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		fmt.Fprintf(os.Stderr, "healthcheck: status %d\n", resp.StatusCode)
		return 1
	}
	return 0
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

	ingredientsRepo := db.NewIngredients(store.DB)
	offClient := off.NewClient(cfg.OFFBaseURL, logger)
	if cfg.OFFSearchBaseURL != "" {
		offClient.SearchBaseURL = cfg.OFFSearchBaseURL
	}
	reconciler := &off.Reconciler{
		Ingredients: ingredientsRepo,
		Client:      offClient,
		Logger:      logger,
		MaxBatch:    50,
		MinAge:      7 * 24 * time.Hour,
	}

	weeklyPlansRepo := db.NewWeeklyPlans(store.DB)
	a := &app.App{
		Cfg:           cfg,
		Store:         store,
		Users:         db.NewUsers(store.DB),
		Ingredients:   ingredientsRepo,
		Recipes:       db.NewRecipes(store.DB),
		MealTemplates: db.NewMealTemplates(store.DB),
		WeeklyPlans:   weeklyPlansRepo,
		ShoppingLists: db.NewShoppingLists(store.DB, weeklyPlansRepo),
		OFF:           offClient,
		Reconciler:    reconciler,
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

	if cfg.ReconcileEnabled {
		go reconciler.Run(ctx, cfg.ReconcileInterval)
	} else {
		logger.Info("reconciler disabilitato (NUTREEK_RECONCILE_ENABLED=false)")
	}

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
