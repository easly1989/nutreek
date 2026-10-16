package off

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/amon2126/nutreek/internal/db"
	"github.com/amon2126/nutreek/internal/domain"
)

// Reconciler periodically searches OpenFoodFacts for an equivalent of every
// local ingredient not verified recently. When it finds a "good enough"
// match it imports it as source='openfoodfacts' (idempotent) and marks the
// local one as superseded_by that barcode. When it finds nothing, it updates
// last_verified_at anyway, so it does not retry soon.
type Reconciler struct {
	Ingredients *db.Ingredients
	Client      *Client
	Logger      *slog.Logger

	// MaxBatch is how many locals to process per RunOnce. Default 50.
	MaxBatch int
	// MinAge is how "old" last_verified_at must be for an ingredient to
	// enter the batch. Default 7 days.
	MinAge time.Duration
}

// Stats summarizes the outcome of a RunOnce.
type Stats struct {
	Considered int // candidates pulled from the DB
	Matched    int // match trovati e superseded
	NoMatch    int // no OFF candidate good enough
	Errors     int // errori HTTP / DB durante l'iterazione
}

// RunOnce processes a single batch. Safe on restart: idempotent per
// ingredient (last_verified_at acts as an implicit cursor).
func (r *Reconciler) RunOnce(ctx context.Context) (Stats, error) {
	maxBatch := r.MaxBatch
	if maxBatch <= 0 {
		maxBatch = 50
	}
	minAge := r.MinAge
	if minAge <= 0 {
		minAge = 7 * 24 * time.Hour
	}
	threshold := time.Now().Add(-minAge)

	candidates, err := r.Ingredients.LocalsToReconcile(ctx, maxBatch, threshold)
	if err != nil {
		return Stats{}, err
	}
	stats := Stats{Considered: len(candidates)}
	for _, local := range candidates {
		if ctx.Err() != nil {
			return stats, ctx.Err()
		}
		match, err := r.findBestMatch(ctx, local)
		if err != nil {
			r.logger().Warn("reconciler: search failed", "ingredient_id", local.ID, "name", local.Name, "err", err)
			stats.Errors++
			// Do not touch it: we will retry sooner.
			continue
		}
		if match == nil {
			stats.NoMatch++
			if err := r.Ingredients.TouchVerified(ctx, local.ID); err != nil {
				r.logger().Warn("reconciler: touch failed", "ingredient_id", local.ID, "err", err)
				stats.Errors++
			}
			continue
		}

		// Import the OFF product (or fetch the existing one with the same barcode).
		offID, err := r.upsertOFF(ctx, *match, local.Unit)
		if err != nil {
			r.logger().Warn("reconciler: upsert OFF failed", "barcode", match.Code, "err", err)
			stats.Errors++
			continue
		}
		if err := r.Ingredients.MarkSuperseded(ctx, local.ID, offID); err != nil {
			r.logger().Warn("reconciler: mark superseded failed", "ingredient_id", local.ID, "err", err)
			stats.Errors++
			continue
		}
		stats.Matched++
		r.logger().Info("reconciler: matched",
			"local_id", local.ID, "local_name", local.Name,
			"off_id", offID, "off_barcode", match.Code, "off_name", match.Name,
		)
	}
	return stats, nil
}

// findBestMatch searches OFF and returns the first product with complete
// macros that shares at least one "significant" word (>=4 chars, not a stop
// word) with the local's name. A brand match is required when both sides
// have one. For the MVP deliberately more tolerant than a strict substring:
// "Grana Padano locale" must latch onto "Grana Padano DOP".
//
// It uses only the first 2 significant words as the seed: with the FTS5 AND
// prefix (SearchByName's semantics) using the whole name would filter out
// words like "locale"/"casereccio" that do not exist in the target OFF product.
func (r *Reconciler) findBestMatch(ctx context.Context, local *domain.Ingredient) (*Product, error) {
	localWords := significantWords(local.Name)
	if len(localWords) == 0 {
		return nil, nil
	}
	seed := local.Name
	switch {
	case len(localWords) >= 2:
		seed = localWords[0] + " " + localWords[1]
	case len(localWords) == 1:
		seed = localWords[0]
	}
	products, err := r.Client.SearchByName(ctx, seed)
	if err != nil {
		return nil, err
	}
	for i := range products {
		p := &products[i]
		if !p.HasFullNutrients {
			continue
		}
		offWords := significantWords(p.Name)
		if !shareWord(localWords, offWords) {
			continue
		}
		// When both have a non-empty brand, they must match (case-insensitive).
		if local.Brand != nil && strings.TrimSpace(*local.Brand) != "" && strings.TrimSpace(p.Brand) != "" {
			if !strings.EqualFold(strings.TrimSpace(*local.Brand), strings.TrimSpace(p.Brand)) {
				continue
			}
		}
		return p, nil
	}
	return nil, nil
}

// significantWords splits the name into lowercase words, dropping tokens of
// 3 chars or fewer and a few Italian stop words. Deliberately small: the goal
// is only to avoid matches on "di / il / la / dop / bio".
var stopWords = map[string]bool{
	"di": true, "del": true, "della": true, "il": true, "lo": true, "la": true,
	"dop": true, "igp": true, "bio": true, "doc": true, "stg": true,
	"con": true, "per": true, "the": true, "and": true,
}

func significantWords(s string) []string {
	out := make([]string, 0, 4)
	for _, raw := range strings.FieldsFunc(strings.ToLower(s), func(r rune) bool {
		return r == ' ' || r == '\t' || r == ',' || r == '.' || r == '-' || r == '/' || r == '('
	}) {
		w := strings.TrimSpace(raw)
		if len(w) < 4 {
			continue
		}
		if stopWords[w] {
			continue
		}
		out = append(out, w)
	}
	return out
}

func shareWord(a, b []string) bool {
	for _, w := range a {
		for _, x := range b {
			if w == x {
				return true
			}
		}
	}
	return false
}

// upsertOFF tries CreateOFF; when the barcode is already there (another local
// was already reconciled to the same OFF product) it fetches the existing id
// with GetByBarcode.
func (r *Reconciler) upsertOFF(ctx context.Context, p Product, unit domain.IngredientUnit) (int64, error) {
	if unit == "" {
		unit = domain.UnitGram
	}
	var brandPtr *string
	if b := strings.TrimSpace(p.Brand); b != "" {
		brandPtr = &b
	}
	id, err := r.Ingredients.CreateOFF(ctx, p.Code, p.Name, brandPtr, unit, p.Kcal, p.Protein, p.Carbs, p.Fat, nil, p.UnitWeightG, p.ServingG,
		p.Fiber, p.Sugars, p.SaturatedFat, p.Sodium)
	if err == nil {
		return id, nil
	}
	if errors.Is(err, db.ErrBarcodeAlreadyUsed) {
		existing, gerr := r.Ingredients.GetByBarcode(ctx, p.Code)
		if gerr != nil {
			return 0, gerr
		}
		return existing.ID, nil
	}
	return 0, err
}

// Run starts the reconciliation loop with `interval`. It exits on
// context.Done. The first run starts after `interval` (not immediately): for
// an on-demand trigger use RunOnce directly or the admin endpoint.
func (r *Reconciler) Run(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = 24 * time.Hour
	}
	r.logger().Info("reconciler: started", "interval", interval, "max_batch", r.MaxBatch, "min_age", r.MinAge)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			r.logger().Info("reconciler: stopped")
			return
		case <-ticker.C:
			stats, err := r.RunOnce(ctx)
			if err != nil {
				r.logger().Warn("reconciler: run failed", "err", err)
				continue
			}
			r.logger().Info("reconciler: tick done",
				"considered", stats.Considered,
				"matched", stats.Matched,
				"no_match", stats.NoMatch,
				"errors", stats.Errors,
			)
		}
	}
}

func (r *Reconciler) logger() *slog.Logger {
	if r.Logger != nil {
		return r.Logger
	}
	return slog.Default()
}
