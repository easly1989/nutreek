// Package sso signs people in through an OpenID Connect provider such as
// Authelia, Keycloak or Authentik (#333): authorization code flow with PKCE.
package sso

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

// ErrNoEmail means the provider did not return a verified email: without one the
// identity cannot be matched to an account.
var ErrNoEmail = errors.New("sso: the provider returned no verified email")

// Config is what the operator sets with NUTREEK_OIDC_*.
type Config struct {
	Issuer       string
	ClientID     string
	ClientSecret string
	RedirectURL  string // <NUTREEK_BASE_URL>/auth/oidc/callback
	Name         string // the button label
}

// Identity is what Nutreek keeps of a sign-in: nothing is stored from the tokens.
type Identity struct {
	Email string
	Name  string
}

// Client talks to one provider. The provider's discovery document is fetched on
// first use and again after a failure, so a provider that is down at startup does
// not keep the app from starting.
type Client struct {
	cfg  Config
	http *http.Client

	mu       sync.Mutex
	provider *oidc.Provider
}

// New returns a client; it makes no network call.
func New(cfg Config, httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 10 * time.Second}
	}
	return &Client{cfg: cfg, http: httpClient}
}

// Name is the button label.
func (c *Client) Name() string { return c.cfg.Name }

func (c *Client) discover(ctx context.Context) (*oidc.Provider, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.provider != nil {
		return c.provider, nil
	}
	p, err := oidc.NewProvider(oidc.ClientContext(ctx, c.http), c.cfg.Issuer)
	if err != nil {
		return nil, fmt.Errorf("sso discovery: %w", err)
	}
	c.provider = p
	return p, nil
}

func (c *Client) oauth(p *oidc.Provider) *oauth2.Config {
	return &oauth2.Config{
		ClientID:     c.cfg.ClientID,
		ClientSecret: c.cfg.ClientSecret,
		Endpoint:     p.Endpoint(),
		RedirectURL:  c.cfg.RedirectURL,
		Scopes:       []string{oidc.ScopeOpenID, "profile", "email"},
	}
}

// AuthURL is where to send the browser. state, nonce and verifier are random and
// kept by the caller until the callback.
func (c *Client) AuthURL(ctx context.Context, state, nonce, verifier string) (string, error) {
	p, err := c.discover(ctx)
	if err != nil {
		return "", err
	}
	return c.oauth(p).AuthCodeURL(state,
		oidc.Nonce(nonce),
		oauth2.S256ChallengeOption(verifier),
	), nil
}

// Exchange trades the code for tokens and returns who signed in. The ID token's
// signature, issuer, audience, expiry and nonce are checked; if it carries no
// email the provider's userinfo endpoint is asked (Authelia puts claims there by
// default).
func (c *Client) Exchange(ctx context.Context, code, nonce, verifier string) (*Identity, error) {
	p, err := c.discover(ctx)
	if err != nil {
		return nil, err
	}
	ctx = oidc.ClientContext(ctx, c.http)
	oc := c.oauth(p)
	tok, err := oc.Exchange(ctx, code, oauth2.VerifierOption(verifier))
	if err != nil {
		return nil, fmt.Errorf("sso token exchange: %w", err)
	}
	raw, _ := tok.Extra("id_token").(string)
	if raw == "" {
		return nil, errors.New("sso: no id_token in the token response")
	}
	idt, err := p.Verifier(&oidc.Config{ClientID: c.cfg.ClientID}).Verify(ctx, raw)
	if err != nil {
		return nil, fmt.Errorf("sso id_token: %w", err)
	}
	if idt.Nonce != nonce {
		return nil, errors.New("sso: nonce mismatch")
	}
	var cl struct {
		Email         string `json:"email"`
		EmailVerified *bool  `json:"email_verified"`
		Name          string `json:"name"`
		Preferred     string `json:"preferred_username"`
	}
	if err := idt.Claims(&cl); err != nil {
		return nil, fmt.Errorf("sso claims: %w", err)
	}
	if cl.Email == "" {
		ui, err := p.UserInfo(ctx, oauth2.StaticTokenSource(tok))
		if err != nil {
			return nil, fmt.Errorf("sso userinfo: %w", err)
		}
		var u struct {
			Email         string `json:"email"`
			EmailVerified *bool  `json:"email_verified"`
			Name          string `json:"name"`
			Preferred     string `json:"preferred_username"`
		}
		if err := ui.Claims(&u); err != nil {
			return nil, fmt.Errorf("sso userinfo claims: %w", err)
		}
		cl.Email, cl.EmailVerified, cl.Name, cl.Preferred = u.Email, u.EmailVerified, u.Name, u.Preferred
	}
	email := strings.TrimSpace(cl.Email)
	if email == "" || cl.EmailVerified == nil || !*cl.EmailVerified {
		return nil, ErrNoEmail
	}
	name := strings.TrimSpace(cl.Name)
	if name == "" {
		name = strings.TrimSpace(cl.Preferred)
	}
	if name == "" {
		name = email
	}
	return &Identity{Email: email, Name: name}, nil
}
