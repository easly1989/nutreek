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

// DefaultTimeout limits the page fetch to 10s (the user waits inline).
const DefaultTimeout = 10 * time.Second

// maxBodyBytes limits the body read to 5 MB: more than enough for any recipe
// page, and it protects against huge responses.
const maxBodyBytes = 5 << 20

// userAgent identifies Nutreek; some sites reject requests without a UA.
const userAgent = "NutreekRecipeImporter/1.0 (+https://github.com/easly1989/nutreek)"

// ErrInvalidURL means a scheme other than http/https or a non-public host.
var ErrInvalidURL = errors.New("URL non valido: usa http(s) verso un sito pubblico")

// Fetcher downloads and parses a recipe from a URL. It exposes Parse via
// FetchAndParse so the caller does not handle the HTTP directly.
type Fetcher struct {
	HTTP *http.Client
}

// NewFetcher builds a Fetcher with a sane timeout and SSRF protection: it
// blocks redirects to private/loopback hosts.
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

// FetchAndParse downloads the page and extracts the structured recipe.
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

// validatePublicURL accepts only http/https to hosts that do not resolve to
// private/loopback/link-local addresses. An SSRF defense: even though the app
// is self-hosted and single-user, a public URL could redirect to the host's
// internal network.
func validatePublicURL(u *url.URL) error {
	if u.Scheme != "http" && u.Scheme != "https" {
		return ErrInvalidURL
	}
	host := u.Hostname()
	if host == "" {
		return ErrInvalidURL
	}
	// Resolve and check every IP associated with the host.
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
