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
	"unicode"
)

// DefaultUserAgent identifica le chiamate verso OpenFoodFacts. OFF chiede
// (https://wiki.openfoodfacts.org/API) di mettere un UA parlante con un
// punto di contatto, anche per evitare rate-limit aggressivi sui generic UA.
const DefaultUserAgent = "Nutreek/0.1 (+https://github.com/amon2126/nutreek)"

// DefaultTimeout e' il timeout HTTP per singola request verso OFF.
// Il CGI it.openfoodfacts.org impiega tipicamente 3-12s; 15s dà margine.
const DefaultTimeout = 15 * time.Second

// DefaultSearchBaseURL e' l'host del CGI di ricerca OFF italiano.
// it.openfoodfacts.org/cgi/search.pl usa il DB principale di OFF — stessa
// fonte del sito web, con copertura completa dei prodotti italiani.
const DefaultSearchBaseURL = "https://it.openfoodfacts.org"

// searchBrowserUA simula il browser per evitare rate-limit aggressivi sul CGI.
const searchBrowserUA = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/125.0.0.0 Safari/537.36"

// ErrNotFound e' restituito da GetByBarcode quando il prodotto non esiste su OFF.
var ErrNotFound = errors.New("prodotto OFF non trovato")

// ErrUnavailable e' restituito da SearchByName quando il CGI risponde 503.
var ErrUnavailable = errors.New("servizio OFF temporaneamente non disponibile")

// ErrIncompleteNutrients indica che il prodotto OFF non ha tutti i 4 valori
// macro che ci servono (kcal, prot, carbs, fat). Importarlo creerebbe una
// riga con dati mancanti, meglio chiedere all'utente di crearlo locale.
var ErrIncompleteNutrients = errors.New("nutrienti incompleti su OFF")

// Client e' un wrapper minimale sull'API OpenFoodFacts.
//
// OFF espone due servizi distinti:
//   - BaseURL (world.openfoodfacts.org): product lookup per barcode (JSON API v2).
//   - SearchBaseURL (it.openfoodfacts.org): ricerca testuale via CGI italiano,
//     che usa il DB principale OFF con copertura completa.
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

// SearchByName chiama it.openfoodfacts.org/cgi/search.pl con headers
// browser-like e retry automatico su 503. Il CGI usa il DB principale di OFF
// (stessa fonte del sito web italiano): copertura completa dei prodotti.
//
// Il 503 è intermittente sull'infrastruttura OFF (~20% senza headers browser,
// <5% con headers corretti). Con 2 retry e backoff di 1s, la probabilità di
// fallimento scende a ~0.25%. La chiamata è pensata per girare in background
// (HTMX lazy-load) quindi la latenza extra non blocca la UI.
//
// page_size aumentato (3×) perché FilterByAllTerms può scartare risultati.
const searchMaxRetries = 2

func (c *Client) SearchByName(ctx context.Context, q string) ([]Product, error) {
	q = strings.TrimSpace(q)
	if q == "" {
		return nil, nil
	}
	base := c.SearchBaseURL
	if base == "" {
		base = DefaultSearchBaseURL
	}

	pageSize := c.MaxResults * 3
	if pageSize > 100 {
		pageSize = 100
	}

	v := url.Values{}
	v.Set("search_terms", q)
	v.Set("search_simple", "1")
	v.Set("action", "process")
	v.Set("json", "1")
	v.Set("page_size", strconv.Itoa(pageSize))
	v.Set("fields", "code,product_name,brands,quantity,nutriments")
	endpoint := strings.TrimRight(base, "/") + "/cgi/search.pl?" + v.Encode()

	var lastErr error
	for attempt := 0; attempt <= searchMaxRetries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(time.Duration(attempt) * time.Second):
			}
		}
		var resp cgiSearchResponse
		err := c.doSearch(ctx, endpoint, &resp)
		if err == nil {
			out := make([]Product, 0, len(resp.Products))
			for _, p := range resp.Products {
				prod := p.toProduct()
				if prod.Code == "" || prod.Name == "" {
					continue
				}
				out = append(out, prod)
			}
			return out, nil
		}
		if errors.Is(err, ErrUnavailable) {
			lastErr = err
			continue // retry solo su 503
		}
		return nil, err
	}
	return nil, lastErr
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

// tokenizeWords spezza una query in parole "vere": separa su qualunque
// carattere non-alfanumerico (whitespace, apostrofi, trattini, ecc.).
// "Müesli croccante all'avena" → ["Müesli", "croccante", "all", "avena"].
// strings.Fields da solo non basterebbe: lascerebbe "all'avena" attaccato
// come unico token non trovabile come substring.
func tokenizeWords(q string) []string {
	return strings.FieldsFunc(q, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
}

// FilterByAllTerms mantiene solo i prodotti in cui ogni termine compare
// (case-insensitive, diacritics-folded, substring) in nome o brand. Filtro
// AND-like per ridurre falsi positivi del CGI (ricerca semplice). Il
// reconciler NON lo usa: vuole match fuzzy nome-locale→nome-OFF.
func FilterByAllTerms(products []Product, q string) []Product {
	terms := tokenizeWords(q)
	if len(terms) <= 1 {
		return products
	}
	normalized := make([]string, len(terms))
	for i, t := range terms {
		normalized[i] = foldText(t)
	}
	out := make([]Product, 0, len(products))
	for _, p := range products {
		// Confronto su testo normalizzato: "muesli" trova "Müesli",
		// "avena" trova "all'avena", ecc.
		combined := foldText(p.Name) + " " + foldText(p.Brand)
		match := true
		for _, t := range normalized {
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

// diacriticFold mappa lettere accentate latine comuni al loro equivalente
// ASCII. Applicato dopo strings.ToLower, quindi gestisce solo minuscole.
// Evita la dipendenza da golang.org/x/text solo per questa operazione.
var diacriticFold = strings.NewReplacer(
	"à", "a", "á", "a", "â", "a", "ã", "a", "ä", "a", "å", "a",
	"è", "e", "é", "e", "ê", "e", "ë", "e",
	"ì", "i", "í", "i", "î", "i", "ï", "i",
	"ò", "o", "ó", "o", "ô", "o", "õ", "o", "ö", "o", "ø", "o",
	"ù", "u", "ú", "u", "û", "u", "ü", "u",
	"ñ", "n", "ç", "c", "ý", "y", "ÿ", "y",
	"ß", "ss", "æ", "ae", "œ", "oe",
	"’", "'", "‘", "'", // apostrofi curvi → dritto
)

// foldText normalizza una stringa per il confronto: minuscolo + diacritici
// rimossi. "Müesli" → "muesli", "all'avena" → "all'avena" (invariato).
func foldText(s string) string {
	return diacriticFold.Replace(strings.ToLower(s))
}

// doSearch esegue la GET verso il CGI di ricerca con headers browser-like.
// Restituisce ErrUnavailable su 503 per permettere il retry in SearchByName.
func (c *Client) doSearch(ctx context.Context, endpoint string, target any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("User-Agent", searchBrowserUA)
	req.Header.Set("Accept", "application/json, text/javascript, */*; q=0.01")
	req.Header.Set("Accept-Language", "it-IT,it;q=0.9,en-US;q=0.8,en;q=0.7")
	req.Header.Set("Referer", "https://it.openfoodfacts.org/")

	res, err := c.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("call OFF: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode == 503 {
		return ErrUnavailable
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
