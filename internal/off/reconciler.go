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

// Reconciler periodicamente cerca su OpenFoodFacts un equivalente per ogni
// ingrediente locale non verificato di recente. Quando trova un match
// "abbastanza buono" lo importa come source='openfoodfacts' (idempotente)
// e segna il locale come superseded_by quel barcode. Quando non trova
// nulla, aggiorna comunque last_verified_at per non riprovare a breve.
type Reconciler struct {
	Ingredients *db.Ingredients
	Client      *Client
	Logger      *slog.Logger

	// MaxBatch e' quante locals processare per RunOnce. Default 50.
	MaxBatch int
	// MinAge e' quanto deve essere "vecchia" last_verified_at perche'
	// rientri nel batch. Default 7 giorni.
	MinAge time.Duration
}

// Stats riassume l'esito di una RunOnce.
type Stats struct {
	Considered int // candidati estratti dal DB
	Matched    int // match trovati e superseded
	NoMatch    int // nessun candidato OFF abbastanza buono
	Errors     int // errori HTTP / DB durante l'iterazione
}

// RunOnce processa un singolo batch. Sicuro a riavvio: idempotente
// per-ingrediente (last_verified_at fa da cursore implicito).
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
			// Non touch-iamo: ritenteremo prima.
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

		// Importa l'OFF (o recupera quello esistente per stesso barcode).
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

// findBestMatch cerca su OFF e ritorna il primo prodotto con macro completi
// che condivide almeno una parola "significativa" (>=4 char, non stop-word)
// col nome del local. Brand match obbligatorio se entrambi i lati lo hanno.
// Per MVP volutamente piu' tollerante di un substring stretto: "Grana
// Padano locale" deve agganciarsi a "Grana Padano DOP".
//
// Usa solo le prime 2 parole significative come seed: con FTS5 AND prefix
// (semantica di SearchByName) usare il nome intero filtrerebbe via parole
// come "locale"/"casereccio" che non esistono nel prodotto OFF target.
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
		// Se entrambi hanno brand non vuoto, devono coincidere (case-insensitive).
		if local.Brand != nil && strings.TrimSpace(*local.Brand) != "" && strings.TrimSpace(p.Brand) != "" {
			if !strings.EqualFold(strings.TrimSpace(*local.Brand), strings.TrimSpace(p.Brand)) {
				continue
			}
		}
		return p, nil
	}
	return nil, nil
}

// significantWords spezza il nome in parole minuscole, scartando token di
// 3 char o meno e qualche stop-word italiana. Volutamente piccola: lo scopo
// e' solo evitare match su "di / il / la / dop / bio".
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

// upsertOFF tenta CreateOFF; se il barcode e' gia' presente (un altro local
// e' gia' stato riconciliato verso lo stesso OFF) recupera l'id esistente
// con GetByBarcode.
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

// Run avvia il loop di reconciliation con `interval`. Esce su context.Done.
// La prima esecuzione parte dopo `interval` (non immediato): per un trigger
// on-demand usa RunOnce direttamente o l'endpoint admin.
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
