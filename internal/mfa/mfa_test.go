package mfa

import (
	"regexp"
	"strings"
	"testing"
	"time"
)

// The RFC 6238 test secret ("12345678901234567890") and its published SHA-1 codes.
func TestCodeMatchesRFC6238(t *testing.T) {
	secret := b32.EncodeToString([]byte("12345678901234567890"))
	for _, tc := range []struct {
		unix int64
		want string
	}{{59, "287082"}, {1111111109, "081804"}, {1234567890, "005924"}, {2000000000, "279037"}} {
		got, err := code(secret, tc.unix/Period)
		if err != nil || got != tc.want {
			t.Errorf("t=%d: got %q (%v), want %q", tc.unix, got, err, tc.want)
		}
	}
}

func TestVerifyWindowAndReplay(t *testing.T) {
	secret := b32.EncodeToString([]byte("12345678901234567890"))
	now := time.Unix(1234567890, 0)
	cur := now.Unix() / Period
	good, _ := code(secret, cur)
	prev, _ := code(secret, cur-1)
	old, _ := code(secret, cur-3)

	if step, ok := Verify(secret, good, now, 0); !ok || step != cur {
		t.Fatalf("the current code: %d, %v", step, ok)
	}
	if _, ok := Verify(secret, prev, now, 0); !ok {
		t.Fatal("the code of the step before should pass: a phone clock is a little off")
	}
	if _, ok := Verify(secret, old, now, 0); ok {
		t.Fatal("a code three steps old must fail")
	}
	if _, ok := Verify(secret, good, now, cur); ok {
		t.Fatal("a step already used must fail")
	}
	if _, ok := Verify(secret, good[:5], now, 0); ok {
		t.Fatal("a short code must fail")
	}
	if _, ok := Verify(secret, strings.Join([]string{good[:3], good[3:]}, " "), now, 0); !ok {
		t.Fatal("a code typed with a space should pass")
	}
	if _, ok := Verify("not base32 !!", good, now, 0); ok {
		t.Fatal("a broken secret must fail")
	}
}

func TestURIAndKey(t *testing.T) {
	u := URI("Nutreek", "carlo@example.test", "JBSWY3DPEHPK3PXP")
	if !strings.HasPrefix(u, "otpauth://totp/Nutreek:carlo@example.test?") && !strings.HasPrefix(u, "otpauth://totp/Nutreek%3Acarlo@example.test?") {
		t.Fatalf("uri = %q", u)
	}
	for _, want := range []string{"secret=JBSWY3DPEHPK3PXP", "issuer=Nutreek", "digits=6", "period=30"} {
		if !strings.Contains(u, want) {
			t.Errorf("uri lacks %q: %s", want, u)
		}
	}
	if got := FormatKey("JBSWY3DPEHPK3PXP"); got != "JBSW Y3DP EHPK 3PXP" {
		t.Fatalf("key = %q", got)
	}
}

func TestRecoveryCodes(t *testing.T) {
	codes, err := NewRecoveryCodes(RecoveryCodes)
	if err != nil || len(codes) != RecoveryCodes {
		t.Fatal(err)
	}
	re := regexp.MustCompile(`^[a-hjkmnp-z2-9]{5}-[a-hjkmnp-z2-9]{5}$`)
	seen := map[string]bool{}
	for _, c := range codes {
		if !re.MatchString(c) || seen[c] {
			t.Fatalf("bad or repeated code %q", c)
		}
		seen[c] = true
	}
	if got := NormalizeRecovery(" 4KQ7M 2XD9t "); got != "4kq7m-2xd9t" {
		t.Fatalf("normalized = %q", got)
	}
	if NormalizeRecovery("4kq7m-2xd9t") != "4kq7m-2xd9t" {
		t.Fatal("a right code stays as it is")
	}
	s1, _ := NewSalt()
	h := HashCode(s1, "a")
	if h == HashCode(s1, "b") || h != HashCode(s1, "a") || h == HashCode(s1+"x", "a") {
		t.Fatal("the hash depends on the code and the salt, and nothing else")
	}
	if !Equal("x", "x") || Equal("x", "y") {
		t.Fatal("Equal")
	}
}

func TestEmailCodeAndQR(t *testing.T) {
	c, err := NewEmailCode()
	if err != nil || !regexp.MustCompile(`^\d{6}$`).MatchString(c) {
		t.Fatalf("code = %q, %v", c, err)
	}
	svg, err := QRSVG(URI("Nutreek", "carlo@example.test", "JBSWY3DPEHPK3PXP"))
	if err != nil || !strings.HasPrefix(svg, "<svg") || !strings.Contains(svg, "<path") || !strings.HasSuffix(svg, "</svg>") {
		t.Fatalf("svg: %v %.60s", err, svg)
	}
}
