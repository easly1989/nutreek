package sso_test

import (
	"context"
	"errors"
	"testing"

	"golang.org/x/oauth2"

	"github.com/amon2126/nutreek/internal/sso"
	"github.com/amon2126/nutreek/internal/sso/ssotest"
)

func start(t *testing.T, p *ssotest.Server) (*sso.Client, string, string, string) {
	t.Helper()
	c := sso.New(sso.Config{Issuer: p.URL, ClientID: p.ClientID, ClientSecret: p.ClientSecret,
		RedirectURL: "http://app.test/auth/oidc/callback", Name: "Authelia"}, nil)
	verifier := oauth2.GenerateVerifier()
	u, err := c.AuthURL(context.Background(), "st", "nn", verifier)
	if err != nil {
		t.Fatalf("AuthURL: %v", err)
	}
	code, _ := p.Visit(t, u)
	return c, code, "nn", verifier
}

func TestExchange(t *testing.T) {
	cases := []struct {
		name    string
		shape   func(*ssotest.Server)
		nonce   string // overrides the nonce the app remembers
		code    string
		wantErr error
		anyErr  bool
	}{
		{name: "email in the token", shape: func(*ssotest.Server) {}},
		{name: "email only in userinfo", shape: func(p *ssotest.Server) { p.EmailInToken = false }},
		{name: "email not verified", shape: func(p *ssotest.Server) { p.Verified = false }, wantErr: sso.ErrNoEmail},
		{name: "no email at all", shape: func(p *ssotest.Server) { p.NoEmail = true }, wantErr: sso.ErrNoEmail},
		{name: "nonce of another sign-in", shape: func(p *ssotest.Server) { p.WrongNonce = true }, anyErr: true},
		{name: "nonce the app does not know", shape: func(*ssotest.Server) {}, nonce: "other", anyErr: true},
		{name: "bad code", shape: func(*ssotest.Server) {}, code: "bad", anyErr: true},
		{name: "userinfo about someone else", shape: func(p *ssotest.Server) { p.EmailInToken = false; p.UserinfoSub = "u2" }, anyErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := ssotest.New(t)
			tc.shape(p)
			c, code, nonce, verifier := start(t, p)
			if tc.nonce != "" {
				nonce = tc.nonce
			}
			if tc.code != "" {
				code = tc.code
			}
			id, err := c.Exchange(context.Background(), code, nonce, verifier)
			switch {
			case tc.wantErr != nil:
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("err = %v, want %v", err, tc.wantErr)
				}
			case tc.anyErr:
				if err == nil {
					t.Fatal("want an error")
				}
			default:
				if err != nil || id.Email != "carlo@example.test" || id.Name != "Carlo" || id.Subject != "u1" || id.Issuer != p.URL {
					t.Fatalf("got %+v, %v", id, err)
				}
			}
		})
	}
}

func TestWrongVerifier(t *testing.T) {
	p := ssotest.New(t)
	c, code, nonce, _ := start(t, p)
	if _, err := c.Exchange(context.Background(), code, nonce, oauth2.GenerateVerifier()); err == nil {
		t.Fatal("a code must not work with another PKCE verifier")
	}
}

func TestProviderDownAtStart(t *testing.T) {
	c := sso.New(sso.Config{Issuer: "http://127.0.0.1:1", ClientID: "x", ClientSecret: "y"}, nil)
	if _, err := c.AuthURL(context.Background(), "s", "n", oauth2.GenerateVerifier()); err == nil {
		t.Fatal("want an error while the provider is unreachable")
	}
}
