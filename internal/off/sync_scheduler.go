package off

import (
	"context"
	"errors"
	"log/slog"
	"time"
)

// SyncScheduler periodically checks whether the OFF data is "old" and, if
// so, starts a Sync. The loop runs often (1h) but a sync starts only when
// last_synced_at is older than Interval — daily by default, like the deltas
// (configurable via NUTREEK_OFF_SYNC_INTERVAL).
//
// Meant to run in a goroutine from main; it reacts to ctx.Done.
type SyncScheduler struct {
	Syncer   *Syncer
	Interval time.Duration // default 24 hours
	Logger   *slog.Logger
}

// Run starts the loop. The first run is immediate (useful on first start:
// empty DB → sync right away). Then it checks every hour; when the last
// successful sync is >= Interval ago, it starts a new sync.
//
// The ticker check is cheap (one SELECT); the real sync starts only when due,
// so the goroutine uses almost no resources.
func (s *SyncScheduler) Run(ctx context.Context) {
	interval := s.Interval
	if interval <= 0 {
		interval = 24 * time.Hour
	}
	logger := s.Logger
	if logger == nil {
		logger = slog.Default()
	}

	logger.Info("off sync scheduler: started", "interval", interval)
	s.checkAndRun(ctx, interval, logger)

	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			logger.Info("off sync scheduler: stopped")
			return
		case <-ticker.C:
			s.checkAndRun(ctx, interval, logger)
		}
	}
}

// checkAndRun starts a sync when the last success is older than interval,
// when there has never been one, or when another parser version imported the
// data (ParserVersion). Sync itself decides between a bootstrap and deltas. It does not block: when a sync is already running it
// returns ErrSyncInProgress with no visible error.
func (s *SyncScheduler) checkAndRun(ctx context.Context, interval time.Duration, logger *slog.Logger) {
	state, err := s.Syncer.LoadState(ctx)
	if err != nil {
		logger.Warn("off sync scheduler: load state failed", "err", err)
		return
	}
	if state.LastSyncedAt != nil && time.Since(*state.LastSyncedAt) < interval && state.ParserVersion == ParserVersion {
		return
	}
	logger.Info("off sync scheduler: triggering sync",
		"last_synced", state.LastSyncedAt, "products_count", state.ProductsCount)

	stats, err := s.Syncer.Sync(ctx)
	if err != nil {
		if errors.Is(err, ErrSyncInProgress) {
			return // an admin trigger already started it
		}
		logger.Warn("off sync scheduler: sync failed", "err", err)
		return
	}
	logger.Info("off sync scheduler: sync done",
		"bootstrap", stats.Bootstrap, "deltas", stats.Deltas, "imported", stats.Imported,
		"duration_ms", stats.DurationMs)
}
