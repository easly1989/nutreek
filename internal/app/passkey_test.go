package app

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/fxamacker/cbor/v2"
	"github.com/go-webauthn/webauthn/webauthn"

	"github.com/amon2126/nutreek/internal/db"
)

const (
	testRPID     = "n.test"
	testOrigin   = "https://n.test"
	flagUP       = 0x01
	flagUV       = 0x04
	flagAT       = 0x40
	attestNone   = "none"
	typeCreate   = "webauthn.create"
	typeGet      = "webauthn.get"
	credTypeName = "public-key"
)

var b64 = base64.RawURLEncoding

// softKey is a software authenticator: it makes the answers a phone would give.
type softKey struct {
	priv      *ecdsa.PrivateKey
	credID    []byte
	signCount uint32
	handle    []byte
}

func newSoftKey(t *testing.T) *softKey {
	t.Helper()
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	id := make([]byte, 32)
	_, _ = rand.Read(id)
	return &softKey{priv: priv, credID: id}
}

func rpHash() []byte { h := sha256.Sum256([]byte(testRPID)); return h[:] }

func clientData(typ, challenge string) []byte {
	b, _ := json.Marshal(map[string]any{"type": typ, "challenge": challenge, "origin": testOrigin, "crossOrigin": false})
	return b
}

// attestation answers a registration challenge.
func (k *softKey) attestation(challenge string) map[string]any {
	cose, _ := cbor.Marshal(map[int]any{1: 2, 3: -7, -1: 1, -2: padTo32(k.priv.X.Bytes()), -3: padTo32(k.priv.Y.Bytes())})
	var auth bytes.Buffer
	auth.Write(rpHash())
	auth.WriteByte(flagUP | flagUV | flagAT)
	_ = binary.Write(&auth, binary.BigEndian, k.signCount)
	auth.Write(make([]byte, 16)) // the authenticator model: none
	_ = binary.Write(&auth, binary.BigEndian, uint16(len(k.credID)))
	auth.Write(k.credID)
	auth.Write(cose)
	obj, _ := cbor.Marshal(map[string]any{"fmt": attestNone, "attStmt": map[string]any{}, "authData": auth.Bytes()})
	return map[string]any{"id": b64.EncodeToString(k.credID), "rawId": b64.EncodeToString(k.credID), "type": credTypeName,
		"response": map[string]any{"attestationObject": b64.EncodeToString(obj), "clientDataJSON": b64.EncodeToString(clientData(typeCreate, challenge))}}
}

func padTo32(b []byte) []byte {
	out := make([]byte, 32)
	copy(out[32-len(b):], b)
	return out
}

// assertion answers a sign-in challenge with the sign count it is given.
func (k *softKey) assertion(challenge string, count uint32, handle []byte) map[string]any {
	var auth bytes.Buffer
	auth.Write(rpHash())
	auth.WriteByte(flagUP | flagUV)
	_ = binary.Write(&auth, binary.BigEndian, count)
	cd := clientData(typeGet, challenge)
	hash := sha256.Sum256(cd)
	digest := sha256.Sum256(append(append([]byte{}, auth.Bytes()...), hash[:]...)) // ES256 signs the SHA-256 of the message
	sig, _ := ecdsa.SignASN1(rand.Reader, k.priv, digest[:])
	return map[string]any{"id": b64.EncodeToString(k.credID), "rawId": b64.EncodeToString(k.credID), "type": credTypeName,
		"response": map[string]any{"authenticatorData": b64.EncodeToString(auth.Bytes()), "clientDataJSON": b64.EncodeToString(cd),
			"signature": b64.EncodeToString(sig), "userHandle": b64.EncodeToString(handle)}}
}

// postJSON sends a JSON body the way the page's script does, with the CSRF token of the page at from.
func postJSON(b *browser, from, path string, v any) (int, map[string]any) {
	b.t.Helper()
	_, _, page := b.get(from)
	m := csrfField.FindStringSubmatch(page)
	if m == nil {
		b.t.Fatalf("no CSRF token on %s", from)
	}
	raw, _ := json.Marshal(v)
	req, _ := http.NewRequest(http.MethodPost, b.srv.URL+path, bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-CSRF-Token", strings.ReplaceAll(m[1], "&#43;", "+"))
	req.Header.Set("Origin", b.srv.URL)
	resp, err := b.c.Do(req)
	if err != nil {
		b.t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	var out map[string]any
	_ = json.Unmarshal(body, &out)
	return resp.StatusCode, out
}

func challengeOf(t *testing.T, out map[string]any) string {
	t.Helper()
	pk, _ := out["publicKey"].(map[string]any)
	c, _ := pk["challenge"].(string)
	if c == "" {
		t.Fatalf("no challenge in %v", out)
	}
	return c
}

func passkeyApp(t *testing.T) (*App, *browser) {
	t.Helper()
	a, _, _ := mfaApp(t)
	wa, err := webauthn.New(&webauthn.Config{RPID: testRPID, RPDisplayName: "Nutreek", RPOrigins: []string{testOrigin}})
	if err != nil {
		t.Fatal(err)
	}
	a.WebAuthn = wa
	return a, newBrowser(t, a) // the router is built again, now with the passkey routes
}

// addPasskey signs the person in with their password and adds the key through the page's calls.
func addPasskey(t *testing.T, a *App, b *browser, key *softKey, name string) {
	t.Helper()
	logIn(b, "carlo@example.test", "right-password")
	code, opts := postJSON(b, "/settings/security/passkeys/new", "/settings/security/passkeys/begin", map[string]string{"name": name, "password": "right-password"})
	if code != 200 {
		t.Fatalf("begin: %d %v", code, opts)
	}
	pk := opts["publicKey"].(map[string]any)
	key.handle, _ = b64.DecodeString(pk["user"].(map[string]any)["id"].(string))
	code, out := postJSON(b, "/settings/security/passkeys/new", "/settings/security/passkeys/finish", key.attestation(challengeOf(t, opts)))
	if code != 200 || out["redirect"] != "/settings/sicurezza" {
		t.Fatalf("finish: %d %v", code, out)
	}
}

func passkeyLogin(t *testing.T, b *browser, key *softKey, count uint32, handle []byte) (int, map[string]any) {
	t.Helper()
	code, opts := postJSON(b, "/login", "/login/passkey/begin", map[string]string{})
	if code != 200 {
		t.Fatalf("begin: %d", code)
	}
	return postJSON(b, "/login", "/login/passkey/finish", key.assertion(challengeOf(t, opts), count, handle))
}

func TestPasskeyAddAndSignIn(t *testing.T) {
	a, b := passkeyApp(t)
	user := mustCreateTestUser(t, a, "carlo@example.test", "right-password")
	key := newSoftKey(t)
	addPasskey(t, a, b, key, "Telefono di Carlo")

	if _, _, body := b.get("/settings/sicurezza"); !strings.Contains(body, "Telefono di Carlo") {
		t.Fatal("the passkey should be listed")
	}
	list, _ := a.Passkeys.List(t.Context(), user.ID)
	if len(list) != 1 || list[0].Name != "Telefono di Carlo" {
		t.Fatalf("stored = %+v", list)
	}

	// A new browser signs in with it: no email and no password.
	b2 := newBrowser(t, a)
	if _, _, body := b2.get("/login"); !strings.Contains(body, "data-passkey-login") {
		t.Fatal("the login page should offer the passkey")
	}
	code, out := passkeyLogin(t, b2, key, 1, key.handle)
	if code != 200 || out["redirect"] != "/" {
		t.Fatalf("sign-in: %d %v", code, out)
	}
	if !signedIn(b2) {
		t.Fatal("should be signed in")
	}
	if list, _ := a.Passkeys.List(t.Context(), user.ID); list[0].LastUsedAt == nil {
		t.Fatal("the use should be recorded")
	}
}

func TestPasskeySkipsTheSecondStep(t *testing.T) {
	a, b := passkeyApp(t)
	user := mustCreateTestUser(t, a, "carlo@example.test", "right-password")
	key := newSoftKey(t)
	addPasskey(t, a, b, key, "Phone")
	totpOn(t, a, user.ID)
	b2 := newBrowser(t, a)
	if code, out := passkeyLogin(t, b2, key, 1, key.handle); code != 200 || out["redirect"] != "/" {
		t.Fatalf("sign-in: %d %v", code, out)
	}
	if !signedIn(b2) {
		t.Fatal("a passkey is a full sign-in: no second step")
	}
}

func TestPasskeyRefusals(t *testing.T) {
	a, b := passkeyApp(t)
	mustCreateTestUser(t, a, "carlo@example.test", "right-password")
	key := newSoftKey(t)
	addPasskey(t, a, b, key, "Phone")

	// Wrong password while adding, and no password at all.
	b2 := newBrowser(t, a)
	logIn(b2, "carlo@example.test", "right-password")
	for _, pw := range []string{"", "wrong"} {
		if code, _ := postJSON(b2, "/settings/security/passkeys/new", "/settings/security/passkeys/begin", map[string]string{"name": "X", "password": pw}); code != 403 {
			t.Fatalf("password %q: %d, want 403", pw, code)
		}
	}

	// Another key's signature.
	other := newSoftKey(t)
	other.credID = key.credID
	if code, _ := passkeyLogin(t, newBrowser(t, a), other, 1, key.handle); code != 401 {
		t.Fatalf("a wrong signature: %d", code)
	}
	// A right key with another account's handle.
	if code, _ := passkeyLogin(t, newBrowser(t, a), key, 1, []byte("someone else")); code != 401 {
		t.Fatalf("a wrong user handle: %d", code)
	}
	// A key that is not known.
	if code, _ := passkeyLogin(t, newBrowser(t, a), newSoftKey(t), 1, key.handle); code != 401 {
		t.Fatalf("an unknown key: %d", code)
	}
	// Finishing with no begun sign-in.
	if code, _ := postJSON(newBrowser(t, a), "/login", "/login/passkey/finish", key.assertion("AAAA", 1, key.handle)); code != 401 {
		t.Fatalf("no ceremony: %d", code)
	}
}

func TestPasskeyCeremonyWorksOnceAndAClonedKeyIsRefused(t *testing.T) {
	a, b := passkeyApp(t)
	mustCreateTestUser(t, a, "carlo@example.test", "right-password")
	key := newSoftKey(t)
	addPasskey(t, a, b, key, "Phone")

	b2 := newBrowser(t, a)
	_, opts := postJSON(b2, "/login", "/login/passkey/begin", map[string]string{})
	answer := key.assertion(challengeOf(t, opts), 5, key.handle)
	if code, _ := postJSON(b2, "/login", "/login/passkey/finish", answer); code != 200 {
		t.Fatalf("first use: %d", code)
	}
	if code, _ := postJSON(b2, "/tu", "/login/passkey/finish", answer); code != 401 {
		t.Fatalf("the same answer twice: %d", code)
	}
	// The sign count goes back: another copy of the key was used.
	if code, _ := passkeyLogin(t, newBrowser(t, a), key, 3, key.handle); code != 401 {
		t.Fatalf("a sign count that went back: %d", code)
	}
}

func TestPasskeyRemoveNeedsThePassword(t *testing.T) {
	a, b := passkeyApp(t)
	user := mustCreateTestUser(t, a, "carlo@example.test", "right-password")
	key := newSoftKey(t)
	addPasskey(t, a, b, key, "Phone")
	list, _ := a.Passkeys.List(t.Context(), user.ID)
	path := "/settings/security/passkeys/" + itoa(list[0].ID) + "/delete"

	b.submit("/settings/sicurezza", path, url.Values{"password": {"wrong"}})
	if l, _ := a.Passkeys.List(t.Context(), user.ID); len(l) != 1 {
		t.Fatal("a wrong password must not remove it")
	}
	b.submit("/settings/sicurezza", path, url.Values{"password": {"right-password"}})
	if l, _ := a.Passkeys.List(t.Context(), user.ID); len(l) != 0 {
		t.Fatal("should be removed")
	}
	if code, _ := passkeyLogin(t, newBrowser(t, a), key, 9, key.handle); code != 401 {
		t.Fatalf("a removed passkey must not sign in: %d", code)
	}
}

func TestPasskeysAreOnlyForTheirOwner(t *testing.T) {
	a, b := passkeyApp(t)
	user := mustCreateTestUser(t, a, "carlo@example.test", "right-password")
	other := mustCreateTestUser(t, a, "eva@example.test", "eva-password")
	key := newSoftKey(t)
	addPasskey(t, a, b, key, "Phone")
	list, _ := a.Passkeys.List(t.Context(), user.ID)

	b2 := newBrowser(t, a)
	logIn(b2, "eva@example.test", "eva-password")
	b2.submit("/settings/sicurezza", "/settings/security/passkeys/"+itoa(list[0].ID)+"/delete", url.Values{"password": {"eva-password"}})
	if l, _ := a.Passkeys.List(t.Context(), user.ID); len(l) != 1 {
		t.Fatal("someone else must not remove it")
	}
	_ = other
}

func TestPasskeysOffWithoutAnAddress(t *testing.T) {
	a, _, b := mfaApp(t) // no WebAuthn
	if _, _, body := b.get("/login"); strings.Contains(body, "data-passkey-login") {
		t.Fatal("no address, no passkey button")
	}
	if code, _ := postJSON(b, "/login", "/login/passkey/begin", map[string]string{}); code != 404 {
		t.Fatalf("the route should not exist: %d", code)
	}
	_ = a
}

func TestPasskeyLimit(t *testing.T) {
	a, b := passkeyApp(t)
	user := mustCreateTestUser(t, a, "carlo@example.test", "right-password")
	for i := 0; i < db.MaxPasskeys; i++ {
		id := make([]byte, 16)
		_, _ = rand.Read(id)
		if err := a.Passkeys.Add(t.Context(), user.ID, []byte("h"), id, "k", "{}", time.Now()); err != nil {
			t.Fatal(err)
		}
	}
	logIn(b, "carlo@example.test", "right-password")
	if code, _ := postJSON(b, "/settings/security/passkeys/new", "/settings/security/passkeys/begin", map[string]string{"name": "one more", "password": "right-password"}); code == 200 {
		t.Fatal("an account keeps at most ten passkeys")
	}
}
