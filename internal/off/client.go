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

// SearchByName chiama Search-a-licious su search.openfoodfacts.org.
//
// Invia la query as-is, senza operatori booleani: il parser di
// Search-a-licious NON interpreta in modo affidabile né AND né il
// prefisso '+' (osservato empiricamente: con "+muesli +croccante" il
// servizio restituisce 0 hit perché tratta "+muesli" come token letterale).
// La restrizione AND-tra-termini è applicata lato chiamante via
// FilterByAllTerms — più robusta e indipendente dalla sintassi server.
//
// page_size è aumentato (3×) perché il filtro client-side può scartare
// molti risultati: serviamo abbastanza candidati al filtro per averne
// di buoni dopo.
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
	v.Set("q", q)
	v.Set("page_size", strconv.Itoa(pageSize))
	v.Set("fields", "code,product_name,brands,quantity,nutriments")
	// langs=it,en: cerca nei campi lingua-specifica italiano E inglese.
	// Senza questo parametro il default è langs=en, che esclude i prodotti
	// italiani indicizzati in product_name_it (es. prodotti Conad, Barilla).
	v.Set("langs", "it,en")
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

// tokenizeWords spezza una query in parole "vere": separa su qualunque
// carattere non-alfanumerico (whitespace, apostrofi, trattini, ecc.).
// "Müesli croccante all'avena" → ["Müesli", "croccante", "all", "avena"].
// strings.Fields da solo non basterebbe: lascerebbe "all'avena" attaccato
// come un unico token, che poi non matcha l'indice di Search-a-licious né
// si trova come substring del prodotto.
func tokenizeWords(q string) []string {
	return strings.FieldsFunc(q, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
}

// FilterByAllTerms mantiene solo i prodotti in cui ogni termine compare
// (case-insensitive, diacritics-folded, substring) in nome o brand. È un
// filtro strict AND-like usato dalla UI picker per compensare il default OR
// di Search-a-licious. Il reconciler invece NON lo usa, perché vuole il
// match fuzzy nome-locale → nome-OFF (gestito da significantWords).
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
