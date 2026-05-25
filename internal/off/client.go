package off

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode"
)

// DefaultUserAgent identifica le chiamate verso OpenFoodFacts. OFF chiede
// (https://wiki.openfoodfacts.org/API) di mettere un UA parlante con un
// punto di contatto, anche per evitare rate-limit aggressivi.
const DefaultUserAgent = "Nutreek/0.1 (+https://github.com/amon2126/nutreek)"

// DefaultTimeout e' il timeout HTTP per le richieste OFF live. Usato solo
// da GetByBarcode (lookup puntuale, veloce e affidabile). SearchByName
// non chiama piu' OFF live: interroga off_products FTS5 in locale.
const DefaultTimeout = 10 * time.Second

// ErrNotFound e' restituito da GetByBarcode quando il prodotto non esiste su OFF.
var ErrNotFound = errors.New("prodotto OFF non trovato")

// Client interroga OpenFoodFacts. Search testuale → SQLite FTS5 locale
// (popolata dal Syncer). Lookup per barcode → API live di OFF (puntuale
// e affidabile, separato dalla ricerca search-as-you-type).
type Client struct {
	DB         *sql.DB
	BaseURL    string
	HTTP       *http.Client
	UserAgent  string
	Logger     *slog.Logger
	MaxResults int
}

// NewClient costruisce un client con i default sani. Il DB e' opzionale
// in fase di costruzione (puo' essere settato successivamente) ma deve
// essere presente prima di chiamare SearchByName.
func NewClient(db *sql.DB, baseURL string, logger *slog.Logger) *Client {
	if baseURL == "" {
		baseURL = "https://world.openfoodfacts.org"
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Client{
		DB:         db,
		BaseURL:    strings.TrimRight(baseURL, "/"),
		HTTP:       &http.Client{Timeout: DefaultTimeout},
		UserAgent:  DefaultUserAgent,
		Logger:     logger,
		MaxResults: 20,
	}
}

// SearchByName interroga la tabella off_products via FTS5. Ricerca <50ms,
// niente HTTP, niente timeout, niente 503. La tabella e' popolata dal
// Syncer dal dump pubblico OFF (sottoinsieme italiano).
//
// Tokenizza la query e wrappa i token in `"token"*` per match prefisso
// + AND implicito fra termini (default FTS5). Esempio:
//
//	"grana pad" → `"grana"* "pad"*` → match per "Grana Padano DOP"
func (c *Client) SearchByName(ctx context.Context, q string) ([]Product, error) {
	q = strings.TrimSpace(q)
	if q == "" {
		return nil, nil
	}
	if c.DB == nil {
		return nil, errors.New("OFF client: DB non configurato")
	}
	match := buildFTSMatch(q)
	if match == "" {
		return nil, nil
	}

	limit := c.MaxResults
	if limit <= 0 {
		limit = 20
	}

	rows, err := c.DB.QueryContext(ctx, `
		SELECT p.barcode, p.product_name, p.brands, p.quantity,
		       p.kcal_per_100, p.protein_per_100, p.carbs_per_100, p.fat_per_100,
		       p.has_full_nutrients
		FROM off_products p
		JOIN off_products_fts fts ON fts.rowid = p.rowid
		WHERE off_products_fts MATCH ?
		ORDER BY bm25(off_products_fts)
		LIMIT ?`, match, limit)
	if err != nil {
		return nil, fmt.Errorf("fts query: %w", err)
	}
	defer rows.Close()

	out := make([]Product, 0, limit)
	for rows.Next() {
		var p Product
		var hasFull int
		if err := rows.Scan(
			&p.Code, &p.Name, &p.Brand, &p.Quantity,
			&p.Kcal, &p.Protein, &p.Carbs, &p.Fat,
			&hasFull,
		); err != nil {
			return nil, fmt.Errorf("fts scan: %w", err)
		}
		p.HasFullNutrients = hasFull == 1
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("fts rows: %w", err)
	}
	return out, nil
}

// buildFTSMatch trasforma una query utente in espressione MATCH FTS5
// sicura: tokenizza in parole (alfanumeriche, supportando unicode), e
// wrappa ogni token in doppi apici come stringa quotata + suffisso `*`
// per il prefix match. Il wrap in apici neutralizza i metachar FTS5
// (`*`, `(`, `:`, `^`, ecc.) e l'eventuale apostrofo del token.
//
//	"grana padano" → `"grana"* "padano"*`
//	"grana pad"    → `"grana"* "pad"*`     (prefix sull'ultimo termine)
//	"all'avena"    → `"all"* "avena"*`     (tokenizza su apostrofo)
//
// AND implicito fra token = ogni parola deve matchare (default FTS5).
// Doppi apici interni (rari, es. nomi prodotto) raddoppiati per escape.
func buildFTSMatch(q string) string {
	tokens := tokenizeWords(q)
	if len(tokens) == 0 {
		return ""
	}
	parts := make([]string, 0, len(tokens))
	for _, t := range tokens {
		escaped := strings.ReplaceAll(t, `"`, `""`)
		parts = append(parts, `"`+escaped+`"*`)
	}
	return strings.Join(parts, " ")
}

// GetByBarcode prende il singolo prodotto da OFF live. Errore ErrNotFound
// se OFF risponde status:0. Endpoint puntuale, veloce, affidabile —
// usato solo on-import al primo utilizzo di un prodotto nuovo.
func (c *Client) GetByBarcode(ctx context.Context, code string) (*Product, error) {
	code = strings.TrimSpace(code)
	if code == "" {
		return nil, errors.New("barcode vuoto")
	}
	endpoint := c.BaseURL + "/api/v2/product/" + url.PathEscape(code) + ".json"

	var resp productResponse
	if err := c.do(ctx, endpoint, &resp); err != nil {
		return nil, err
	}
	if resp.Status == 0 {
		return nil, ErrNotFound
	}
	p := resp.Product.toProduct()
	if p.Code == "" {
		p.Code = code
	}
	return &p, nil
}

// tokenizeWords spezza una query in parole "vere": separa su qualunque
// carattere non-alfanumerico (whitespace, apostrofi, trattini, ecc.).
// "Müesli croccante all'avena" → ["Müesli", "croccante", "all", "avena"].
func tokenizeWords(q string) []string {
	return strings.FieldsFunc(q, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
}

// do esegue la GET per il lookup prodotto (barcode). UA Nutreek standard.
func (c *Client) do(ctx context.Context, endpoint string, target any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("User-Agent", c.UserAgent)
	req.Header.Set("Accept", "application/json")

	res, err := c.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("call OFF: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode/100 != 2 {
		body, _ := io.ReadAll(io.LimitReader(res.Body, 512))
		return fmt.Errorf("OFF status %d: %s", res.StatusCode, strings.TrimSpace(string(body)))
	}
	if err := json.NewDecoder(res.Body).Decode(target); err != nil {
		return fmt.Errorf("decode OFF response: %w", err)
	}
	return nil
}
