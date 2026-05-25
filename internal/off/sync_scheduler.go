package off

import (
	"context"
	"errors"
	"log/slog"
	"time"
)

// SyncScheduler periodicamente verifica se il dump OFF e' "vecchio" e in
// caso lancia un SyncDump. Loop a frequenza alta (1h) ma sync attivato
// solo se last_synced_at e' piu' vecchio di Interval — settimanale per
// default (configurabile via NUTREEK_OFF_SYNC_INTERVAL).
//
// Pensato per girare in goroutine dal main; reagisce a ctx.Done.
type SyncScheduler struct {
	Syncer   *Syncer
	Interval time.Duration // default 7 giorni
	Logger   *slog.Logger
}

// Run avvia il loop. La prima esecuzione e' immediata (utile al primo
// avvio: DB vuoto → sync subito). Poi check ogni ora; se l'ultimo sync
// di successo e' >= Interval fa, lancia un nuovo sync.
//
// Il check via ticker e' cheap (una SELECT); il sync vero parte solo
// quando dovuto, quindi la goroutine consuma quasi zero risorse.
func (s *SyncScheduler) Run(ctx context.Context) {
	interval := s.Interval
	if interval <= 0 {
		interval = 7 * 24 * time.Hour
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

// checkAndRun lancia un sync se l'ultimo successo e' piu' vecchio di
// interval (o se non c'e' mai stato uno). Non blocca: se un sync e'
// gia' in corso ritorna ErrSyncInProgress senza errore visibile.
func (s *SyncScheduler) checkAndRun(ctx context.Context, interval time.Duration, logger *slog.Logger) {
	state, err := s.Syncer.LoadState(ctx)
	if err != nil {
		logger.Warn("off sync scheduler: load state failed", "err", err)
		return
	}
	if state.LastSyncedAt != nil && time.Since(*state.LastSyncedAt) < interval {
		return
	}
	logger.Info("off sync scheduler: triggering sync",
		"last_synced", state.LastSyncedAt, "products_count", state.ProductsCount)

	stats, err := s.Syncer.SyncDump(ctx)
	if err != nil {
		if errors.Is(err, ErrSyncInProgress) {
			return // un trigger admin l'ha gia' avviato
		}
		logger.Warn("off sync scheduler: sync failed", "err", err)
		return
	}
	logger.Info("off sync scheduler: sync done",
		"imported", stats.Imported, "skipped", stats.Skipped,
		"duration_ms", stats.DurationMs)
}
