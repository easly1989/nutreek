package off

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// DefaultUserAgent identifica le chiamate verso OpenFoodFacts. OFF chiede
// (https://wiki.openfoodfacts.org/API) di mettere un UA parlante con un
// punto di contatto, anche per evitare rate-limit aggressivi sui generic UA.
const DefaultUserAgent = "Nutreek/0.1 (+https://github.com/amon2126/nutreek)"

// DefaultTimeout e' il timeout per singola request HTTP verso OFF.
// Volutamente corto: la UI deve degradare gracefully se OFF e' lento.
const DefaultTimeout = 3 * time.Second

// DefaultSearchBaseURL e' l'host del nuovo servizio di ricerca testuale
// OFF ("Search-a-licious"). Gli endpoint search legacy
// (/cgi/search.pl e /api/v2/search) restano 503 in modo permanente.
const DefaultSearchBaseURL = "https://search.openfoodfacts.org"

// ErrNotFound e' restituito da GetByBarcode quando il prodotto non esiste su OFF.
var ErrNotFound = errors.New("prodotto OFF non trovato")

// ErrIncompleteNutrients indica che il prodotto OFF non ha tutti i 4 valori
// macro che ci servono (kcal, prot, carbs, fat). Importarlo creerebbe una
// riga con dati mancanti, meglio chiedere all'utente di crearlo locale.
var ErrIncompleteNutrients = errors.New("nutrienti incompleti su OFF")

// Client e' un wrapper minimale sull'API OpenFoodFacts.
//
// OFF espone due servizi distinti su host separati:
//   - BaseURL (es. world.openfoodfacts.org): product lookup per barcode.
//   - SearchBaseURL (es. search.openfoodfacts.org): ricerca testuale
//     "Search-a-licious", l'unico endpoint search ancora attivo dopo la
//     dismissione del legacy /cgi/search.pl.
type Client struct {
	BaseURL       string
	SearchBaseURL string
	HTTP          *http.Client
	UserAgent     string
	Logger        *slog.Logger
	MaxResults    int // limite di risultati per /search
}

// NewClient costruisce un client con i default sani.
func NewClient(baseURL string, logger *slog.Logger) *Client {
	if baseURL == "" {
		baseURL = "https://world.openfoodfacts.org"
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Client{
		BaseURL:       strings.TrimRight(baseURL, "/"),
		SearchBaseURL: DefaultSearchBaseURL,
		HTTP:          &http.Client{Timeout: DefaultTimeout},
		UserAgent:     DefaultUserAgent,
		Logger:        logger,
		MaxResults:    20,
	}
}

// SearchByName chiama il nuovo endpoint "Search-a-licious" su
// search.openfoodfacts.org. Il legacy /cgi/search.pl e /api/v2/search
// restituiscono 503 permanente dal 2026; questo e' l'unico modo di
// fare ricerca testuale lato server.
//
// Per query multi-parola usa la sintassi simple_query_string con prefisso
// '+' su ogni termine ("+a +b +c" = AND-like). NON applica filtro stretto
// post-fetch: Search-a-licious può comunque restituire false positives
// (vedi FilterByAllTerms per uno strato di filtro UI-side).
func (c *Client) SearchByName(ctx context.Context, q string) ([]Product, error) {
	q = strings.TrimSpace(q)
	if q == "" {
		return nil, nil
	}
	base := c.SearchBaseURL
	if base == "" {
		base = DefaultSearchBaseURL
	}

	v := url.Values{}
	v.Set("q", buildSearchQuery(q))
	v.Set("page_size", strconv.Itoa(c.MaxResults))
	v.Set("fields", "code,product_name,brands,quantity,nutriments")
	endpoint := strings.TrimRight(base, "/") + "/search?" + v.Encode()

	var resp searchResponse
	if err := c.do(ctx, endpoint, &resp); err != nil {
		return nil, err
	}
	out := make([]Product, 0, len(resp.Hits))
	for _, h := range resp.Hits {
		p := h.toProduct()
		if p.Code == "" || p.Name == "" {
			continue
		}
		out = append(out, p)
	}
	return out, nil
}

// GetByBarcode prende il singolo prodotto. Errore ErrNotFound se OFF
// risponde status:0 (convenzione del loro API).
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

// buildSearchQuery usa il prefisso '+' (simple_query_string "must") per
// ogni termine: "Müesli Croccante Avena" → "+Müesli +Croccante +Avena".
// Search-a-licious usa simple_query_string con default OR; 'AND' letterale
// non è un operatore booleano in quella sintassi, '+' lo è.
func buildSearchQuery(q string) string {
	words := strings.Fields(q)
	if len(words) <= 1 {
		return q
	}
	parts := make([]string, len(words))
	for i, w := range words {
		parts[i] = "+" + w
	}
	return strings.Join(parts, " ")
}

// FilterByAllTerms mantiene solo i prodotti in cui ogni termine compare
// (case-insensitive, substring) in nome o brand. È un filtro strict
// AND-like usato dalla UI picker per compensare il default OR di
// Search-a-licious. Il reconciler invece NON lo usa, perché vuole il
// match fuzzy nome-locale → nome-OFF (gestito da significantWords).
func FilterByAllTerms(products []Product, q string) []Product {
	terms := strings.Fields(q)
	if len(terms) <= 1 {
		return products
	}
	lower := make([]string, len(terms))
	for i, t := range terms {
		lower[i] = strings.ToLower(t)
	}
	out := make([]Product, 0, len(products))
	for _, p := range products {
		combined := strings.ToLower(p.Name) + " " + strings.ToLower(p.Brand)
		match := true
		for _, t := range lower {
			if !strings.Contains(combined, t) {
				match = false
				break
			}
		}
		if match {
			out = append(out, p)
		}
	}
	return out
}

// do esegue la GET, decodifica JSON nel target. Aggiunge UA, log su errore.
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
