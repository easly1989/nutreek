// Package ssotest is a small OpenID Connect provider for tests: discovery, keys,
// the token and userinfo endpoints. It checks PKCE and signs real ID tokens.
package ssotest

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"
)

// Server is the provider. Set the fields before the flow starts to shape the
// answer; the zero values give a normal sign-in.
type Server struct {
	*httptest.Server
	ClientID, ClientSecret string
	Email, Name            string
	Sub                    string // who the person is here; stays the same when the email changes
	UserinfoSub            string // when set, userinfo answers about someone else
	Verified               bool   // email_verified
	EmailInToken           bool   // false: the email is only in userinfo, as Authelia sends it
	NoEmail                bool   // the provider has no email at all
	WrongNonce             bool   // the ID token carries another nonce

	key       *rsa.PrivateKey
	nonce     string
	challenge string
}

// New starts a provider that stops with the test.
func New(t *testing.T) *Server {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("rsa key: %v", err)
	}
	s := &Server{ClientID: "nutreek", ClientSecret: "s3cret", Email: "carlo@example.test", Name: "Carlo", Sub: "u1",
		Verified: true, EmailInToken: true, key: key}
	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{
			"issuer":                                s.URL,
			"authorization_endpoint":                s.URL + "/authorize",
			"token_endpoint":                        s.URL + "/token",
			"jwks_uri":                              s.URL + "/jwks",
			"userinfo_endpoint":                     s.URL + "/userinfo",
			"id_token_signing_alg_values_supported": []string{"RS256"},
			"response_types_supported":              []string{"code"},
			"subject_types_supported":               []string{"public"},
		})
	})
	mux.HandleFunc("/jwks", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, jose.JSONWebKeySet{Keys: []jose.JSONWebKey{{Key: &key.PublicKey, KeyID: "k1", Algorithm: "RS256", Use: "sig"}}})
	})
	mux.HandleFunc("/token", s.token)
	mux.HandleFunc("/userinfo", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer access-ok" {
			http.Error(w, "no", http.StatusUnauthorized)
			return
		}
		writeJSON(w, s.claims(true))
	})
	s.Server = httptest.NewServer(mux)
	t.Cleanup(s.Close)
	return s
}

// Visit plays the browser at the provider: it takes the address the app sent the
// person to, remembers its nonce and PKCE challenge, and returns the code and the
// state the provider would send back to the callback.
func (s *Server) Visit(t *testing.T, authURL string) (code, state string) {
	t.Helper()
	u, err := url.Parse(authURL)
	if err != nil || u.Path != "/authorize" {
		t.Fatalf("not an authorize address: %q", authURL)
	}
	q := u.Query()
	if q.Get("client_id") != s.ClientID || q.Get("response_type") != "code" || q.Get("code_challenge_method") != "S256" ||
		q.Get("code_challenge") == "" || q.Get("nonce") == "" || q.Get("state") == "" || q.Get("redirect_uri") == "" {
		t.Fatalf("authorize request is missing something: %v", q)
	}
	s.nonce, s.challenge = q.Get("nonce"), q.Get("code_challenge")
	return "good", q.Get("state")
}

func (s *Server) claims(userinfo bool) map[string]any {
	sub := s.Sub
	if userinfo && s.UserinfoSub != "" {
		sub = s.UserinfoSub
	}
	c := map[string]any{"sub": sub, "preferred_username": "carlo"}
	if s.Name != "" {
		c["name"] = s.Name
	}
	if !s.NoEmail && (userinfo || s.EmailInToken) {
		c["email"] = s.Email
		c["email_verified"] = s.Verified
	}
	return c
}

func (s *Server) token(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	id, secret, ok := r.BasicAuth()
	if !ok {
		id, secret = r.PostForm.Get("client_id"), r.PostForm.Get("client_secret")
	}
	sum := sha256.Sum256([]byte(r.PostForm.Get("code_verifier")))
	if id != s.ClientID || secret != s.ClientSecret || r.PostForm.Get("code") != "good" ||
		base64.RawURLEncoding.EncodeToString(sum[:]) != s.challenge {
		w.WriteHeader(http.StatusBadRequest)
		writeJSON(w, map[string]string{"error": "invalid_grant"})
		return
	}
	nonce := s.nonce
	if s.WrongNonce {
		nonce = "another"
	}
	signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: s.key},
		(&jose.SignerOptions{}).WithType("JWT").WithHeader("kid", "k1"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	std := jwt.Claims{Issuer: s.URL, Subject: s.Sub, Audience: jwt.Audience{s.ClientID},
		Expiry: jwt.NewNumericDate(time.Now().Add(time.Hour)), IssuedAt: jwt.NewNumericDate(time.Now())}
	raw, err := jwt.Signed(signer).Claims(std).Claims(map[string]any{"nonce": nonce}).Claims(s.claims(false)).Serialize()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{"access_token": "access-ok", "token_type": "Bearer", "expires_in": 3600, "id_token": raw})
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
