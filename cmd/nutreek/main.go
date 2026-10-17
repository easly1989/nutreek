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
	"github.com/amon2126/nutreek/internal/service"
	"github.com/amon2126/nutreek/internal/web"
)

// version is set at build time with -ldflags "-X main.version=vX.Y.Z"
// (see the Dockerfile and .github/workflows/release.yml). In development it stays "dev".
var version = "dev"

func main() {
	// Healthcheck subcommand: used by the Dockerfile (distroless has no
	// shell nor wget/curl, so the HTTP probe runs in process). Exits 0
	// when /healthz answers 200, 1 otherwise.
	healthcheck := flag.Bool("healthcheck", false, "esegue una GET su /healthz e termina con exit code 0/1")
	showVersion := flag.Bool("version", false, "stampa la versione e termina")
	flag.Parse()
	if *showVersion {
		fmt.Println(version)
		return
	}
	if *healthcheck {
		os.Exit(runHealthcheck())
	}

	// In prod: structured JSON for ingestion into an observability stack.
	// In dev: readable text at DEBUG level.
	env := os.Getenv("NUTREEK_ENV")
	var logHandler slog.Handler
	if env == "prod" {
		logHandler = slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})
	} else {
		logHandler = slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug})
	}
	logger := slog.New(logHandler)
	slog.SetDefault(logger)

	if err := run(logger); err != nil {
		logger.Error("fatal", "err", err)
		os.Exit(1)
	}
}

// runHealthcheck calls the local server and returns the exit code for the
// docker HEALTHCHECK. It reads the addr from NUTREEK_ADDR (e.g. ":8080") and
// connects to localhost.
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
	offClient := off.NewClient(store.DB, cfg.OFFBaseURL, logger)
	offSyncer := off.NewSyncer(store.DB, cfg.OFFDumpURL, logger)
	reconciler := &off.Reconciler{
		Ingredients: ingredientsRepo,
		Client:      offClient,
		Logger:      logger,
		MaxBatch:    50,
		MinAge:      7 * 24 * time.Hour,
	}

	weeklyPlansRepo := db.NewWeeklyPlans(store.DB)
	recipesRepo := db.NewRecipes(store.DB)
	nutritionistRepo := db.NewNutritionistPlans(store.DB)
	featuresRepo := db.NewFeatures(store.DB)
	coachRepo := db.NewCoachRepository(store.DB)
	usersRepo := db.NewUsers(store.DB)
	weightLogsRepo := db.NewWeightLogs(store.DB)
	a := &app.App{
		Cfg:                    cfg,
		Store:                  store,
		Users:                  usersRepo,
		Ingredients:            ingredientsRepo,
		Recipes:                recipesRepo,
		MealTemplates:          db.NewMealTemplates(store.DB),
		WeeklyPlans:            weeklyPlansRepo,
		MealItemCustomizations: db.NewMealItemCustomizations(store.DB),
		ShoppingLists:          db.NewShoppingLists(store.DB, weeklyPlansRepo),
		WeightLogs:             weightLogsRepo,
		WorkoutLogs:            db.NewWorkoutLogs(store.DB),
		WaterLogs:              db.NewWaterLogs(store.DB),
		Categories:             db.NewCategories(store.DB),
		NutritionistPlans:      nutritionistRepo,
		Features:               featuresRepo,
		Subscriptions:          db.NewSubscriptions(store.DB),
		Coach:                  coachRepo,
		APITokens:              db.NewAPITokens(store.DB, usersRepo),
		TDEE: &service.TDEEService{
			WeightLogs:  weightLogsRepo,
			WeeklyPlans: weeklyPlansRepo,
		},
		Planner: &service.PlannerService{
			WeeklyPlans:       weeklyPlansRepo,
			NutritionistPlans: nutritionistRepo,
			Recipes:           recipesRepo,
			Coach:             coachRepo,
		},
		OFF:        offClient,
		OFFSyncer:  offSyncer,
		Reconciler: reconciler,
		Sessions:   sessions,
		Render:     renderer,
		Logger:     logger,
	}
	if err := applyDemo(ctx, store, sessions, a, logger); err != nil {
		return err
	}
	if cfg.BackupEnabled { // after applyDemo, which switches them off
		go runBackupScheduler(ctx, store, cfg, logger)
	}

	srv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           a.Router(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		logger.Info("nutreek listening", "addr", cfg.Addr, "env", cfg.Env, "version", version)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("server error", "err", err)
			stop()
		}
	}()

	if cfg.ReconcileEnabled {
		go reconciler.Run(ctx, cfg.ReconcileInterval)
	} else {
		logger.Info("reconciler disabled (NUTREEK_RECONCILE_ENABLED=false)")
	}

	if cfg.OFFSyncEnabled {
		scheduler := &off.SyncScheduler{
			Syncer:   offSyncer,
			Interval: cfg.OFFSyncInterval,
			Logger:   logger,
		}
		go scheduler.Run(ctx)
	} else {
		logger.Info("OFF sync scheduler disabled (NUTREEK_OFF_SYNC_ENABLED=false)")
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

// runBackupScheduler runs a periodic SQLite backup using VACUUM INTO.
// It waits BackupInterval between backups, then deletes the oldest files,
// keeping at most BackupKeep.
func runBackupScheduler(ctx context.Context, store *db.Store, cfg *config.Config, logger *slog.Logger) {
	logger.Info("backup scheduler started", "dir", cfg.BackupDir, "interval", cfg.BackupInterval, "keep", cfg.BackupKeep)
	ticker := time.NewTicker(cfg.BackupInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case t := <-ticker.C:
			dest := db.BackupFileName(cfg.BackupDir, t)
			if err := store.BackupTo(ctx, dest); err != nil {
				logger.Error("backup failed", "err", err, "dest", dest)
				continue
			}
			logger.Info("backup done", "dest", dest)
			if err := db.PruneBackups(cfg.BackupDir, cfg.BackupKeep); err != nil {
				logger.Warn("backup prune failed", "err", err)
			}
		}
	}
}
