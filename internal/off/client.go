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

// ErrNotFound e' restituito da GetByBarcode quando il prodotto non esiste su OFF.
var ErrNotFound = errors.New("prodotto OFF non trovato")

// ErrIncompleteNutrients indica che il prodotto OFF non ha tutti i 4 valori
// macro che ci servono (kcal, prot, carbs, fat). Importarlo creerebbe una
// riga con dati mancanti, meglio chiedere all'utente di crearlo locale.
var ErrIncompleteNutrients = errors.New("nutrienti incompleti su OFF")

// Client e' un wrapper minimale sull'API OpenFoodFacts.
type Client struct {
	BaseURL    string
	HTTP       *http.Client
	UserAgent  string
	Logger     *slog.Logger
	MaxResults int // limite di risultati per /search
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
		BaseURL:    strings.TrimRight(baseURL, "/"),
		HTTP:       &http.Client{Timeout: DefaultTimeout},
		UserAgent:  DefaultUserAgent,
		Logger:     logger,
		MaxResults: 20,
	}
}

// SearchByName chiama l'endpoint legacy /cgi/search.pl che ancora oggi
// resta il piu' affidabile per la ricerca testuale. La v2 search e' in
// beta e ha sintassi diversa per ogni filtro, lasciamola per dopo.
func (c *Client) SearchByName(ctx context.Context, q string) ([]Product, error) {
	q = strings.TrimSpace(q)
	if q == "" {
		return nil, nil
	}
	v := url.Values{}
	v.Set("search_terms", q)
	v.Set("search_simple", "1")
	v.Set("action", "process")
	v.Set("json", "1")
	v.Set("page_size", strconv.Itoa(c.MaxResults))
	v.Set("fields", "code,product_name,brands,quantity,nutriments")
	endpoint := c.BaseURL + "/cgi/search.pl?" + v.Encode()

	var resp searchResponse
	if err := c.do(ctx, endpoint, &resp); err != nil {
		return nil, err
	}
	out := make([]Product, 0, len(resp.Products))
	for _, raw := range resp.Products {
		p := raw.toProduct()
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
