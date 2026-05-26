package off

import (
	"bufio"
	"compress/gzip"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"
)

// DefaultDumpURL e' l'URL del dump JSONL.gz completo OpenFoodFacts. ~0.9GB
// compressi, ~9-15GB decompressi, ~3.5M prodotti totali. Lo streamiamo
// riga per riga e filtriamo ai ~250k italiani.
const DefaultDumpURL = "https://static.openfoodfacts.org/data/openfoodfacts-products.jsonl.gz"

// italyTag e' il tag canonico OFF per i prodotti venduti in Italia.
// countries_tags e' un array di stringhe normalizzate (en:<paese>).
const italyTag = "en:italy"

// dumpBatchSize e' il numero di prodotti accumulati per ogni transaction
// di INSERT. 1000 e' un buon compromesso: lock breve, throughput alto.
const dumpBatchSize = 1000

// scannerBufferMax e' la dimensione massima della riga JSONL accettata dal
// Scanner. Alcuni prodotti hanno descrizioni / ingredienti molto lunghi,
// fino a centinaia di KB. 4MB e' largo abbastanza per non scartare nulla.
const scannerBufferMax = 4 * 1024 * 1024

// ErrSyncInProgress e' restituito da SyncDump se un altro sync sta gia'
// girando (il mutex impedisce concorrenza, anche dall'admin endpoint).
var ErrSyncInProgress = errors.New("sync OFF gia' in corso")

// SyncStats riassume l'esito di una SyncDump.
type SyncStats struct {
	ScannedTotal int   // righe processate (qualunque paese)
	Imported     int   // prodotti italiani inseriti
	Skipped      bool  // true se ETag invariato → niente download
	ETag         string
	DurationMs   int64
}

// Syncer scarica il dump pubblico OFF e popola off_products. Pensato per
// girare in background (cron settimanale + trigger admin manuale). Usa un
// mutex per garantire UN solo sync in volo alla volta.
type Syncer struct {
	DB     *sql.DB
	URL    string
	HTTP   *http.Client
	Logger *slog.Logger

	mu      sync.Mutex
	syncing bool
}

// NewSyncer costruisce un Syncer con i default sani. Timeout HTTP lungo
// perché il download del dump completo può durare 5-15 minuti.
func NewSyncer(db *sql.DB, url string, logger *slog.Logger) *Syncer {
	if url == "" {
		url = DefaultDumpURL
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Syncer{
		DB:     db,
		URL:    url,
		HTTP:   &http.Client{Timeout: 30 * time.Minute},
		Logger: logger,
	}
}

// IsSyncing ritorna true se un sync sta correntemente girando. Usato
// dall'admin endpoint per restituire 409 senza accodare richieste.
func (s *Syncer) IsSyncing() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.syncing
}

// SyncDump scarica il dump JSONL.gz, ne stream-parsa i prodotti, filtra
// quelli italiani e re-popola off_products. Idempotente: a fine import
// la tabella riflette esattamente il dump rilasciato da OFF.
//
// Usa If-None-Match con l'ETag salvato per saltare il download se il
// server non ha aggiornato. In caso di errore aggiorna last_error in
// off_sync_state ma NON cancella i dati esistenti.
func (s *Syncer) SyncDump(ctx context.Context) (SyncStats, error) {
	s.mu.Lock()
	if s.syncing {
		s.mu.Unlock()
		return SyncStats{}, ErrSyncInProgress
	}
	s.syncing = true
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		s.syncing = false
		s.mu.Unlock()
	}()

	start := time.Now()
	s.markAttempt(ctx)

	stats, err := s.doSync(ctx)
	stats.DurationMs = time.Since(start).Milliseconds()
	if err != nil {
		s.markError(ctx, err)
		return stats, err
	}
	s.markSuccess(ctx, stats)
	return stats, nil
}

func (s *Syncer) doSync(ctx context.Context) (SyncStats, error) {
	previousETag, _ := s.readLastETag(ctx)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.URL, nil)
	if err != nil {
		return SyncStats{}, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("User-Agent", DefaultUserAgent)
	if previousETag != "" {
		req.Header.Set("If-None-Match", previousETag)
	}

	s.Logger.Info("off sync: starting download", "url", s.URL, "prev_etag", previousETag)
	res, err := s.HTTP.Do(req)
	if err != nil {
		return SyncStats{}, fmt.Errorf("download dump: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode == http.StatusNotModified {
		s.Logger.Info("off sync: dump non modificato (304), skip")
		return SyncStats{Skipped: true, ETag: previousETag}, nil
	}
	if res.StatusCode/100 != 2 {
		body, _ := io.ReadAll(io.LimitReader(res.Body, 512))
		return SyncStats{}, fmt.Errorf("dump status %d: %s", res.StatusCode, strings.TrimSpace(string(body)))
	}

	contentLen := res.ContentLength
	s.Logger.Info("off sync: response received, starting decompress+ingest",
		"content_length_bytes", contentLen, "etag", res.Header.Get("ETag"))

	// Wrappa il body in un progressReader cosi' loggiamo i MB scaricati ogni
	// 50MB. Senza questo l'utente non vede attivita' durante il download dei
	// ~900MB compressi (5-30 min a seconda della banda).
	body := &progressReader{
		R:         res.Body,
		Total:     contentLen,
		Logger:    s.Logger,
		LogEvery:  50 * 1024 * 1024,
		LogPrefix: "off sync: download",
	}

	gz, err := gzip.NewReader(body)
	if err != nil {
		return SyncStats{}, fmt.Errorf("gzip reader: %w", err)
	}
	defer gz.Close()

	newETag := res.Header.Get("ETag")
	stats, err := s.ingest(ctx, gz, newETag)
	if err != nil {
		return stats, err
	}
	stats.ETag = newETag
	return stats, nil
}

// ingest legge il flusso JSONL decompresso, filtra per paese e batcha
// gli INSERT in transazioni da dumpBatchSize. Cancella off_products in
// transazione separata PRIMA di iniziare cosi' i nuovi dati rimpiazzano
// completamente i vecchi (no orfani da dump precedente).
func (s *Syncer) ingest(ctx context.Context, r io.Reader, newETag string) (SyncStats, error) {
	if _, err := s.DB.ExecContext(ctx, `DELETE FROM off_products`); err != nil {
		return SyncStats{}, fmt.Errorf("truncate off_products: %w", err)
	}

	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64*1024), scannerBufferMax)

	stats := SyncStats{}
	batch := make([]Product, 0, dumpBatchSize)
	now := time.Now().Unix()

	lastLogScanned := 0
	for scanner.Scan() {
		if err := ctx.Err(); err != nil {
			return stats, err
		}
		stats.ScannedTotal++

		// Log progresso ogni 100k righe scannate (anche se non sono italiane):
		// l'utente vede che il parser sta lavorando anche prima di importare
		// abbastanza prodotti IT per triggerare il log su Imported.
		if stats.ScannedTotal-lastLogScanned >= 100000 {
			s.Logger.Info("off sync: parsing",
				"scanned", stats.ScannedTotal, "imported_it", stats.Imported)
			lastLogScanned = stats.ScannedTotal
		}

		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var d dumpProduct
		if err := json.Unmarshal(line, &d); err != nil {
			// Riga malformata: skippa silenziosamente, il dump completo
			// e' best-effort. Loggare ogni errore intaserebbe i log.
			continue
		}
		if !containsTag(d.CountriesTags, italyTag) {
			continue
		}
		p := d.toProduct()
		if p.Code == "" || p.Name == "" {
			continue
		}
		batch = append(batch, p)

		if len(batch) >= dumpBatchSize {
			if err := s.flushBatch(ctx, batch, now); err != nil {
				return stats, err
			}
			stats.Imported += len(batch)
			batch = batch[:0]
		}
	}
	if err := scanner.Err(); err != nil {
		return stats, fmt.Errorf("scan dump: %w", err)
	}
	if len(batch) > 0 {
		if err := s.flushBatch(ctx, batch, now); err != nil {
			return stats, err
		}
		stats.Imported += len(batch)
	}
	s.Logger.Info("off sync: ingest completato",
		"imported", stats.Imported, "scanned", stats.ScannedTotal, "new_etag", newETag)
	return stats, nil
}

// flushBatch esegue un INSERT batch in singola transaction. Usa prepared
// statement per evitare overhead di parsing su ogni riga.
func (s *Syncer) flushBatch(ctx context.Context, batch []Product, ts int64) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO off_products (
			barcode, product_name, brands, quantity,
			kcal_per_100, protein_per_100, carbs_per_100, fat_per_100,
			has_full_nutrients, image_url, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(barcode) DO UPDATE SET
			product_name = excluded.product_name,
			brands = excluded.brands,
			quantity = excluded.quantity,
			kcal_per_100 = excluded.kcal_per_100,
			protein_per_100 = excluded.protein_per_100,
			carbs_per_100 = excluded.carbs_per_100,
			fat_per_100 = excluded.fat_per_100,
			has_full_nutrients = excluded.has_full_nutrients,
			image_url = excluded.image_url,
			updated_at = excluded.updated_at`)
	if err != nil {
		return fmt.Errorf("prepare insert: %w", err)
	}
	defer stmt.Close()

	for _, p := range batch {
		hasFull := 0
		if p.HasFullNutrients {
			hasFull = 1
		}
		if _, err := stmt.ExecContext(ctx,
			p.Code, p.Name, p.Brand, p.Quantity,
			p.Kcal, p.Protein, p.Carbs, p.Fat,
			hasFull, p.ImageURL, ts,
		); err != nil {
			return fmt.Errorf("insert %s: %w", p.Code, err)
		}
	}
	return tx.Commit()
}

// dumpProduct e' il subset di campi del dump OFF che ci interessa. Stesso
// schema di rawProduct piu' countries_tags per il filtro paese.
type dumpProduct struct {
	Code               string                     `json:"code"`
	ProductName        string                     `json:"product_name"`
	Brands             string                     `json:"brands"`
	Quantity           string                     `json:"quantity"`
	CountriesTags      []string                   `json:"countries_tags"`
	Nutriments         map[string]json.RawMessage `json:"nutriments"`
	ImageFrontSmallURL string                     `json:"image_front_small_url"`
	ImageURL           string                     `json:"image_url"`
}

// toProduct riusa la logica di rawProduct.toProduct() costruendo un
// rawProduct equivalente. Evitare duplicazione: il parsing dei nutrienti
// e' identico sia per il dump sia per la response API live.
func (d dumpProduct) toProduct() Product {
	raw := rawProduct{
		Code:               d.Code,
		ProductName:        d.ProductName,
		Brands:             d.Brands,
		Quantity:           d.Quantity,
		Nutriments:         d.Nutriments,
		ImageFrontSmallURL: d.ImageFrontSmallURL,
		ImageURL:           d.ImageURL,
	}
	return raw.toProduct()
}

func containsTag(tags []string, target string) bool {
	for _, t := range tags {
		if t == target {
			return true
		}
	}
	return false
}

// progressReader logga ogni LogEvery byte letti dallo stream sottostante.
// Usato per dare feedback durante il download del dump compresso (5-30 min
// di solo Read senza eventi visibili). Total = ContentLength se noto, 0
// se sconosciuto (chunked transfer).
type progressReader struct {
	R         io.Reader
	Total     int64
	Logger    *slog.Logger
	LogEvery  int64
	LogPrefix string

	read     int64
	nextLog  int64
}

func (p *progressReader) Read(buf []byte) (int, error) {
	n, err := p.R.Read(buf)
	p.read += int64(n)
	if p.nextLog == 0 {
		p.nextLog = p.LogEvery
	}
	if p.read >= p.nextLog {
		if p.Total > 0 {
			pct := float64(p.read) / float64(p.Total) * 100
			p.Logger.Info(p.LogPrefix,
				"read_mb", p.read/(1024*1024),
				"total_mb", p.Total/(1024*1024),
				"pct", fmt.Sprintf("%.1f", pct))
		} else {
			p.Logger.Info(p.LogPrefix, "read_mb", p.read/(1024*1024))
		}
		p.nextLog += p.LogEvery
	}
	return n, err
}

// ---- stato sync persistito --------------------------------------------------

func (s *Syncer) readLastETag(ctx context.Context) (string, error) {
	var etag sql.NullString
	err := s.DB.QueryRowContext(ctx, `SELECT dump_etag FROM off_sync_state WHERE id = 1`).Scan(&etag)
	if err != nil {
		return "", err
	}
	return etag.String, nil
}

func (s *Syncer) markAttempt(ctx context.Context) {
	_, err := s.DB.ExecContext(ctx, `UPDATE off_sync_state SET last_attempt_at = ? WHERE id = 1`, time.Now().Unix())
	if err != nil {
		s.Logger.Warn("off sync: markAttempt failed", "err", err)
	}
}

func (s *Syncer) markSuccess(ctx context.Context, stats SyncStats) {
	count := stats.Imported
	if stats.Skipped {
		// Non aggiorniamo il count: rifletteremmo lo stato attuale.
		row := s.DB.QueryRowContext(ctx, `SELECT products_count FROM off_sync_state WHERE id = 1`)
		_ = row.Scan(&count)
	}
	_, err := s.DB.ExecContext(ctx, `
		UPDATE off_sync_state
		SET last_synced_at = ?, last_error = NULL, products_count = ?, dump_etag = ?
		WHERE id = 1`,
		time.Now().Unix(), count, stats.ETag,
	)
	if err != nil {
		s.Logger.Warn("off sync: markSuccess failed", "err", err)
	}
}

func (s *Syncer) markError(ctx context.Context, syncErr error) {
	_, err := s.DB.ExecContext(ctx, `UPDATE off_sync_state SET last_error = ? WHERE id = 1`, syncErr.Error())
	if err != nil {
		s.Logger.Warn("off sync: markError failed", "err", err)
	}
}

// State e' la rappresentazione di off_sync_state usata dall'admin endpoint.
type State struct {
	LastSyncedAt  *time.Time
	LastAttemptAt *time.Time
	LastError     string
	ProductsCount int
	DumpETag      string
	Syncing       bool
}

// LoadState legge lo stato corrente dal DB. Errore solo su problemi reali
// (la riga singleton e' creata dalla migration).
func (s *Syncer) LoadState(ctx context.Context) (*State, error) {
	row := s.DB.QueryRowContext(ctx, `
		SELECT last_synced_at, last_attempt_at, last_error, products_count, dump_etag
		FROM off_sync_state WHERE id = 1`)
	var (
		synced, attempt sql.NullInt64
		errMsg, etag    sql.NullString
		count           int
	)
	if err := row.Scan(&synced, &attempt, &errMsg, &count, &etag); err != nil {
		return nil, err
	}
	st := &State{
		LastError:     errMsg.String,
		ProductsCount: count,
		DumpETag:      etag.String,
		Syncing:       s.IsSyncing(),
	}
	if synced.Valid {
		t := time.Unix(synced.Int64, 0)
		st.LastSyncedAt = &t
	}
	if attempt.Valid {
		t := time.Unix(attempt.Int64, 0)
		st.LastAttemptAt = &t
	}
	return st, nil
}
