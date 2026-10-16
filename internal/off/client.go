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

// DefaultUserAgent identifies the calls to OpenFoodFacts. OFF asks
// (https://wiki.openfoodfacts.org/API) for a descriptive UA with a point of
// contact, also to avoid aggressive rate limits.
const DefaultUserAgent = "Nutreek/0.1 (+https://github.com/amon2126/nutreek)"

// DefaultTimeout is the HTTP timeout for live OFF requests. Used only by
// GetByBarcode (a single, fast and reliable lookup). SearchByName no longer
// calls OFF live: it queries the off_products FTS5 locally.
const DefaultTimeout = 10 * time.Second

// ErrNotFound is returned by GetByBarcode when the product does not exist on OFF.
var ErrNotFound = errors.New("prodotto OFF non trovato")

// Client queries OpenFoodFacts. Text search → local SQLite FTS5 (filled by
// the Syncer). Barcode lookup → OFF's live API (single and reliable,
// separate from the search-as-you-type search).
type Client struct {
	DB         *sql.DB
	BaseURL    string
	HTTP       *http.Client
	UserAgent  string
	Logger     *slog.Logger
	MaxResults int
}

// NewClient builds a client with sane defaults. The DB is optional at
// construction time (it can be set later) but must be there before
// SearchByName is called.
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

// SearchByName queries the off_products table via FTS5. Search <50ms,
// no HTTP, no timeouts, no 503s. The table is filled by the Syncer from
// the public OFF dump (the Italian subset).
//
// It tokenizes the query and wraps the tokens in `"token"*` for a prefix
// match + an implicit AND between terms (the FTS5 default). Example:
//
//	"grana pad" → `"grana"* "pad"*` → matches "Grana Padano DOP"
func (c *Client) SearchByName(ctx context.Context, q string) ([]Product, error) {
	q = strings.TrimSpace(q)
	if q == "" {
		return nil, nil
	}
	if c.DB == nil {
		return nil, errors.New("OFF client: DB not configured")
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
		       p.has_full_nutrients, p.image_url, p.serving_g
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
			&hasFull, &p.ImageURL, &p.ServingG,
		); err != nil {
			return nil, fmt.Errorf("fts scan: %w", err)
		}
		p.HasFullNutrients = hasFull == 1
		p.UnitWeightG = ParseUnitWeightG(p.Quantity)
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("fts rows: %w", err)
	}
	return out, nil
}

// buildFTSMatch turns a user query into a safe FTS5 MATCH expression: it
// tokenizes into words (alphanumeric, unicode-aware), and wraps every token
// in double quotes as a quoted string + a `*` suffix for the prefix match.
// The quoting neutralizes the FTS5 metachars (`*`, `(`, `:`, `^`, etc.) and
// any apostrophe in the token.
//
//	"grana padano" → `"grana"* "padano"*`
//	"grana pad"    → `"grana"* "pad"*`     (prefix on the last term)
//	"all'avena"    → `"all"* "avena"*`     (tokenizes on the apostrophe)
//
// Implicit AND between tokens = every word must match (the FTS5 default).
// Inner double quotes (rare, e.g. product names) are doubled to escape them.
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

// GetByBarcode fetches the single product from live OFF. ErrNotFound when OFF
// answers 404 or status:0. A single, fast, reliable endpoint — used only on
// import, the first time a new product is used.
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

// tokenizeWords splits a query into "real" words: it splits on any
// non-alphanumeric character (whitespace, apostrophes, dashes, etc.).
// "Müesli croccante all'avena" → ["Müesli", "croccante", "all", "avena"].
func tokenizeWords(q string) []string {
	return strings.FieldsFunc(q, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
}

// do runs the GET for the product lookup (barcode). Standard Nutreek UA.
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
	// The v2 API answers 404 (status:0 in the body) to a code it does not know.
	if res.StatusCode == http.StatusNotFound {
		return ErrNotFound
	}
	if res.StatusCode/100 != 2 {
		body, _ := io.ReadAll(io.LimitReader(res.Body, 512))
		return fmt.Errorf("OFF status %d: %s", res.StatusCode, strings.TrimSpace(string(body)))
	}
	if err := json.NewDecoder(res.Body).Decode(target); err != nil {
		return fmt.Errorf("decode OFF response: %w", err)
	}
	return nil
}
