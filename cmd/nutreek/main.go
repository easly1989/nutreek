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

func main() {
	// Sub-comando di healthcheck: usato dal Dockerfile (distroless non ha
	// shell ne' wget/curl, quindi facciamo l'HTTP probe in-process). Esce 0
	// se /healthz risponde 200, 1 altrimenti.
	healthcheck := flag.Bool("healthcheck", false, "esegue una GET su /healthz e termina con exit code 0/1")
	flag.Parse()
	if *healthcheck {
		os.Exit(runHealthcheck())
	}

	// In prod: JSON strutturato per ingestione in stack di osservabilità.
	// In dev: testo leggibile con livello DEBUG.
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

	if cfg.BackupEnabled {
		go runBackupScheduler(ctx, store, cfg, logger)
	}

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

	if cfg.OFFSyncEnabled {
		scheduler := &off.SyncScheduler{
			Syncer:   offSyncer,
			Interval: cfg.OFFSyncInterval,
			Logger:   logger,
		}
		go scheduler.Run(ctx)
	} else {
		logger.Info("OFF sync scheduler disabilitato (NUTREEK_OFF_SYNC_ENABLED=false)")
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

// runBackupScheduler esegue un backup SQLite periodico usando VACUUM INTO.
// Attende BackupInterval tra un backup e l'altro, poi elimina i file più
// vecchi lasciandone al massimo BackupKeep.
func runBackupScheduler(ctx context.Context, store *db.Store, cfg *config.Config, logger *slog.Logger) {
	logger.Info("backup scheduler avviato", "dir", cfg.BackupDir, "interval", cfg.BackupInterval, "keep", cfg.BackupKeep)
	ticker := time.NewTicker(cfg.BackupInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case t := <-ticker.C:
			dest := db.BackupFileName(cfg.BackupDir, t)
			if err := store.BackupTo(ctx, dest); err != nil {
				logger.Error("backup fallito", "err", err, "dest", dest)
				continue
			}
			logger.Info("backup completato", "dest", dest)
			if err := db.PruneBackups(cfg.BackupDir, cfg.BackupKeep); err != nil {
				logger.Warn("prune backup fallito", "err", err)
			}
		}
	}
}
