package recipeimport

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// DefaultTimeout limita il fetch della pagina a 10s (l'utente attende inline).
const DefaultTimeout = 10 * time.Second

// maxBodyBytes limita la lettura del body a 5 MB: piu' che sufficiente per
// qualsiasi pagina di ricetta, protegge da risposte gigantesche.
const maxBodyBytes = 5 << 20

// userAgent identifica Nutreek; alcuni siti rifiutano richieste senza UA.
const userAgent = "NutreekRecipeImporter/1.0 (+https://github.com/easly1989/nutreek)"

// ErrInvalidURL indica uno schema non http/https o un host non pubblico.
var ErrInvalidURL = errors.New("URL non valido: usa http(s) verso un sito pubblico")

// Fetcher scarica e interpreta una ricetta da una URL. Espone Parse via
// FetchAndParse cosi' che il chiamante non gestisca direttamente l'HTTP.
type Fetcher struct {
	HTTP *http.Client
}

// NewFetcher costruisce un Fetcher con timeout sano e protezione SSRF:
// blocca i redirect verso host privati/loopback.
func NewFetcher() *Fetcher {
	return &Fetcher{
		HTTP: &http.Client{
			Timeout: DefaultTimeout,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if len(via) >= 5 {
					return errors.New("troppi redirect")
				}
				return validatePublicURL(req.URL)
			},
		},
	}
}

// FetchAndParse scarica la pagina e ne estrae la ricetta strutturata.
func (f *Fetcher) FetchAndParse(ctx context.Context, rawURL string) (*ParsedRecipe, error) {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return nil, ErrInvalidURL
	}
	if err := validatePublicURL(u); err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml")

	resp, err := f.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch pagina: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("la pagina ha risposto %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes))
	if err != nil {
		return nil, fmt.Errorf("lettura body: %w", err)
	}
	return Parse(body)
}

// validatePublicURL accetta solo http/https verso host che non risolvono a
// indirizzi privati/loopback/link-local. Difesa SSRF: anche se l'app e'
// self-hosted single-user, un URL pubblico potrebbe redirigere verso la rete
// interna dell'host.
func validatePublicURL(u *url.URL) error {
	if u.Scheme != "http" && u.Scheme != "https" {
		return ErrInvalidURL
	}
	host := u.Hostname()
	if host == "" {
		return ErrInvalidURL
	}
	// Risolvi e controlla tutti gli IP associati all'host.
	ips, err := net.LookupIP(host)
	if err != nil {
		return fmt.Errorf("host non risolvibile: %w", err)
	}
	for _, ip := range ips {
		if isBlockedIP(ip) {
			return ErrInvalidURL
		}
	}
	return nil
}

func isBlockedIP(ip net.IP) bool {
	return ip.IsLoopback() ||
		ip.IsPrivate() ||
		ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() ||
		ip.IsUnspecified()
}
