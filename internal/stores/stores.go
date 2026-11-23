// Package stores talks to store plugins (#239): small HTTP services a user connects
// to Nutreek that search a grocery store's catalogue and answer with prices and
// availability. Nutreek ships no plugin, only this contract (docs/store-plugins.md):
//
//	GET {url}/search?q=text          -> {"products": [product, ...]}
//	GET {url}/product?id=ID          -> product
//	GET {url}/availability?id=A&id=B -> {"products": [{"id", "price", "available"}, ...]}
//
// Every call carries "Authorization: Bearer <token>". A product is
//
//	{"id", "name", "brand", "size": {"quantity": 700, "unit": "g"}, "price": 1.09,
//	 "currency": "EUR", "available": true, "unit_weight_g": 120,
//	 "nutrition": {"kcal", "protein", "carbs", "fat", "fiber", "sugars", "saturated_fat", "sodium"}}
//
// with the nutrition per 100 g (or ml). A product without the four macros is left
// out of a search: it cannot become an ingredient.
package stores

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/amon2126/nutreek/internal/notify"
)

const (
	maxBody     = 1 << 20 // a plugin answer is read up to 1 MiB
	maxProducts = 20      // products kept from a search
	maxIDs      = 20      // ids asked in one availability call: what a plugin can look at in its time
)

// Sizes a package can be measured in: grams, millilitres or pieces.
const (
	UnitGram  = "g"
	UnitMl    = "ml"
	UnitPiece = "pz"
)

// ErrBadURL is returned for an address that is not an http(s) URL with a host.
var ErrBadURL = errors.New("stores: the address is not an https URL (http only where private addresses are allowed)")

// ErrBadProduct is returned when a plugin's product lacks what Nutreek needs.
var ErrBadProduct = errors.New("stores: the product has no id, name or nutrition")

// Product is what a plugin knows about one article.
type Product struct {
	ID, Name, Brand string
	SizeQty         float64 // the package's size, in SizeUnit; 0 when unknown
	SizeUnit        string  // g, ml or pz
	PriceCents      int
	HasPrice        bool
	Currency        string
	Available       bool
	UnitWeightG     *float64

	// Nutrition per 100 g or ml; HasNutrition says the four macros are there.
	HasNutrition                bool
	Kcal, Protein, Carbs, Fat   float64
	Fiber, Sugars, SaturatedFat *float64
	Sodium                      *float64
}

// Status is the answer of an availability call about one product.
type Status struct {
	PriceCents int
	HasPrice   bool
	Available  bool
}

// Client calls plugins. The zero value is not usable: use NewClient.
type Client struct {
	HTTP *http.Client
	// Builtin are the integrated stores (#277): handlers that run in this process, by the
	// ID they are connected with («builtin://<id>»). They need no address, token or network
	// access of ours; what a handler reads from its store is its own business.
	Builtin map[string]http.Handler
}

// NewClient builds the plugin client. A plugin address is set by the user, so by
// default only public addresses are reached (the same guard as the push sender);
// allowPrivate lifts it for a plugin on the local network (NUTREEK_STORES_ALLOW_PRIVATE).
// Redirects are never followed: the token would go with them.
func NewClient(allowPrivate bool) *Client {
	timeout := 6 * time.Second
	if !allowPrivate {
		return &Client{HTTP: notify.SafeClient(timeout)}
	}
	return &Client{HTTP: &http.Client{
		Timeout:       timeout,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		Transport:     &http.Transport{Proxy: nil, TLSHandshakeTimeout: timeout},
	}}
}

// CleanURL checks a plugin address and returns it without a trailing slash. The token
// is sent in the clear over http, so http is accepted only where private addresses are
// (allowPrivate): a plugin on a public address has to speak https.
func CleanURL(raw string, allowPrivate bool) (string, error) {
	raw = strings.TrimSpace(raw)
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "https" && !(u.Scheme == "http" && allowPrivate)) || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || len(raw) > 300 {
		return "", ErrBadURL
	}
	return strings.TrimRight(raw, "/"), nil
}

// Search asks the plugin for the products matching q.
func (c *Client) Search(ctx context.Context, base, token, q string) ([]Product, error) {
	var out struct {
		Products []wireProduct `json:"products"`
	}
	if err := c.get(ctx, base, token, "/search", url.Values{"q": {q}}, &out); err != nil {
		return nil, err
	}
	var products []Product
	for _, w := range out.Products {
		p, err := w.product()
		if err != nil || !p.HasNutrition {
			continue
		}
		products = append(products, p)
		if len(products) == maxProducts {
			break
		}
	}
	return products, nil
}

// Product asks the plugin for one product (it is what an import trusts, never the
// data of a form).
func (c *Client) Product(ctx context.Context, base, token, id string) (Product, error) {
	var w wireProduct
	if err := c.get(ctx, base, token, "/product", url.Values{"id": {id}}, &w); err != nil {
		return Product{}, err
	}
	p, err := w.product()
	if err != nil {
		return Product{}, err
	}
	if p.ID != id {
		return Product{}, ErrBadProduct
	}
	return p, nil
}

// Availability asks the price and the availability of products, by id; the ones the
// plugin does not know are missing from the result.
func (c *Client) Availability(ctx context.Context, base, token string, ids []string) (map[string]Status, error) {
	out := map[string]Status{}
	for start := 0; start < len(ids); start += maxIDs {
		var res struct {
			Products []struct {
				ID        string   `json:"id"`
				Price     *float64 `json:"price"`
				Available *bool    `json:"available"`
			} `json:"products"`
		}
		q := url.Values{"id": ids[start:min(start+maxIDs, len(ids))]}
		if err := c.get(ctx, base, token, "/availability", q, &res); err != nil {
			return nil, err
		}
		for _, p := range res.Products {
			s := Status{Available: p.Available == nil || *p.Available}
			s.PriceCents, s.HasPrice = cents(p.Price)
			out[p.ID] = s
		}
	}
	return out, nil
}

func (c *Client) get(ctx context.Context, base, token, path string, q url.Values, dst any) error {
	status, body, err := c.fetch(ctx, base, token, path, q)
	if err != nil {
		return fmt.Errorf("stores: %s: %w", path, err)
	}
	if status != http.StatusOK {
		return fmt.Errorf("stores: %s answered %d", path, status)
	}
	if err := json.Unmarshal(body, dst); err != nil {
		return fmt.Errorf("stores: %s: %w", path, err)
	}
	return nil
}

// fetch makes the call, over the network or, for an integrated store, in the process.
func (c *Client) fetch(ctx context.Context, base, token, path string, q url.Values) (int, []byte, error) {
	if id, ok := strings.CutPrefix(base, BuiltinPrefix); ok {
		return c.fetchBuiltin(ctx, id, path, q)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+path+"?"+q.Encode(), nil)
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBody+1))
	if err != nil {
		return 0, nil, err
	}
	if len(body) > maxBody {
		return 0, nil, fmt.Errorf("answered more than %d bytes", maxBody)
	}
	return resp.StatusCode, body, nil
}

// wireProduct is a product as a plugin writes it.
type wireProduct struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Brand string `json:"brand"`
	Size  struct {
		Quantity float64 `json:"quantity"`
		Unit     string  `json:"unit"`
	} `json:"size"`
	Price       *float64 `json:"price"`
	Currency    string   `json:"currency"`
	Available   *bool    `json:"available"`
	UnitWeightG *float64 `json:"unit_weight_g"`
	Nutrition   *struct {
		Kcal         *float64 `json:"kcal"`
		Protein      *float64 `json:"protein"`
		Carbs        *float64 `json:"carbs"`
		Fat          *float64 `json:"fat"`
		Fiber        *float64 `json:"fiber"`
		Sugars       *float64 `json:"sugars"`
		SaturatedFat *float64 `json:"saturated_fat"`
		Sodium       *float64 `json:"sodium"`
	} `json:"nutrition"`
}

// product checks a wire product and turns it into a Product, with sizes, prices and
// nutrition in sane ranges; the text is cut to a length the pages can show.
func (w wireProduct) product() (Product, error) {
	p := Product{
		ID: strings.TrimSpace(w.ID), Name: cut(w.Name, 200), Brand: cut(w.Brand, 100),
		Currency: strings.ToUpper(cut(w.Currency, 3)), Available: w.Available == nil || *w.Available,
	}
	if p.ID == "" || len(p.ID) > 200 || p.Name == "" {
		return Product{}, ErrBadProduct
	}
	if p.Currency == "" {
		p.Currency = "EUR"
	}
	switch w.Size.Unit {
	case UnitGram, UnitMl, UnitPiece:
		if ok(w.Size.Quantity, 1e-3, 1e6) {
			p.SizeQty, p.SizeUnit = w.Size.Quantity, w.Size.Unit
		}
	}
	p.PriceCents, p.HasPrice = cents(w.Price)
	if w.UnitWeightG != nil && ok(*w.UnitWeightG, 0.1, 5000) {
		p.UnitWeightG = w.UnitWeightG
	}
	if n := w.Nutrition; n != nil && n.Kcal != nil && n.Protein != nil && n.Carbs != nil && n.Fat != nil &&
		ok(*n.Kcal, 0, 1000) && ok(*n.Protein, 0, 100) && ok(*n.Carbs, 0, 100) && ok(*n.Fat, 0, 100) {
		p.HasNutrition = true
		p.Kcal, p.Protein, p.Carbs, p.Fat = *n.Kcal, *n.Protein, *n.Carbs, *n.Fat
		p.Fiber, p.Sugars, p.SaturatedFat, p.Sodium = opt(n.Fiber, 100), opt(n.Sugars, 100), opt(n.SaturatedFat, 100), opt(n.Sodium, 100)
	}
	return p, nil
}

func ok(v, lo, hi float64) bool { return !math.IsNaN(v) && v >= lo && v <= hi }

// opt keeps an optional nutrient when it is in range.
func opt(v *float64, hi float64) *float64 {
	if v != nil && ok(*v, 0, hi) {
		return v
	}
	return nil
}

// cents turns a price into cents; a missing, negative or absurd price is "no price".
func cents(v *float64) (int, bool) {
	if v == nil || !ok(*v, 0, 100000) {
		return 0, false
	}
	return int(math.Round(*v * 100)), true
}

func cut(s string, n int) string {
	s = strings.TrimSpace(s)
	if r := []rune(s); len(r) > n {
		return string(r[:n])
	}
	return s
}
