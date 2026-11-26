// Package mfa is the logic of the second sign-in step (#335): time-based one-time codes
// (RFC 6238), one-time recovery codes, the six-digit code sent by email, and the QR code
// that carries a secret to an authenticator app.
package mfa

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1" //nolint:gosec // RFC 6238 apps use HMAC-SHA1; it is not used as a hash for anything else
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base32"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"math/big"
	"net/url"
	"strings"
	"time"

	"github.com/boombuler/barcode"
	"github.com/boombuler/barcode/qr"
)

const (
	// Period is how long a time-based code lasts.
	Period = 30
	// Digits is the length of a time-based code and of the email code.
	Digits = 6
	// RecoveryCodes is how many recovery codes a person gets at a time.
	RecoveryCodes = 10
)

var b32 = base32.StdEncoding.WithPadding(base32.NoPadding)

// NewSecret returns a new 160-bit secret in base32, the form authenticator apps take.
func NewSecret() (string, error) {
	raw := make([]byte, 20)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return b32.EncodeToString(raw), nil
}

// FormatKey groups a secret in fours for reading aloud or typing.
func FormatKey(secret string) string {
	var parts []string
	for len(secret) > 4 {
		parts = append(parts, secret[:4])
		secret = secret[4:]
	}
	return strings.Join(append(parts, secret), " ")
}

// URI is the otpauth address an authenticator app reads from the QR code.
func URI(issuer, account, secret string) string {
	v := url.Values{"secret": {secret}, "issuer": {issuer}, "algorithm": {"SHA1"},
		"digits": {fmt.Sprint(Digits)}, "period": {fmt.Sprint(Period)}}
	return "otpauth://totp/" + url.PathEscape(issuer+":"+account) + "?" + v.Encode()
}

// code is the time-based code of a secret for one 30-second step.
func code(secret string, step int64) (string, error) {
	key, err := b32.DecodeString(strings.ToUpper(strings.ReplaceAll(secret, " ", "")))
	if err != nil {
		return "", fmt.Errorf("mfa: secret: %w", err)
	}
	var msg [8]byte
	binary.BigEndian.PutUint64(msg[:], uint64(step))
	mac := hmac.New(sha1.New, key)
	mac.Write(msg[:])
	sum := mac.Sum(nil)
	off := sum[len(sum)-1] & 0x0f
	n := binary.BigEndian.Uint32(sum[off:off+4]) & 0x7fffffff
	return fmt.Sprintf("%0*d", Digits, n%1000000), nil
}

// CodeAt is the code a secret shows at a moment: what an authenticator app would display.
func CodeAt(secret string, at time.Time) (string, error) { return code(secret, at.Unix()/Period) }

// Verify checks a code against the steps around now (one before and one after, for a
// phone clock that is a little off). It returns the step the code belongs to, which the
// caller keeps: a step at or before lastStep has been used and is refused.
func Verify(secret, input string, now time.Time, lastStep int64) (int64, bool) {
	input = strings.ReplaceAll(strings.TrimSpace(input), " ", "")
	if len(input) != Digits {
		return 0, false
	}
	cur := now.Unix() / Period
	var found int64
	for _, s := range []int64{cur - 1, cur, cur + 1} {
		c, err := code(secret, s)
		if err != nil {
			return 0, false
		}
		// every step is compared, so the time taken does not say which one matched
		if subtle.ConstantTimeCompare([]byte(c), []byte(input)) == 1 && s > lastStep {
			found = s
		}
	}
	return found, found != 0
}

// recoveryAlphabet leaves out the letters and digits that look alike.
const recoveryAlphabet = "abcdefghjkmnpqrstuvwxyz23456789"

// NewRecoveryCodes makes n codes of two groups of five characters (50 bits each).
func NewRecoveryCodes(n int) ([]string, error) {
	out := make([]string, n)
	for i := range out {
		var b strings.Builder
		for j := 0; j < 10; j++ {
			if j == 5 {
				b.WriteByte('-')
			}
			k, err := rand.Int(rand.Reader, big.NewInt(int64(len(recoveryAlphabet))))
			if err != nil {
				return nil, err
			}
			b.WriteByte(recoveryAlphabet[k.Int64()])
		}
		out[i] = b.String()
	}
	return out, nil
}

// NormalizeRecovery lowers a typed code and puts the dash back, so "4KQ7M 2XD9T" works.
func NormalizeRecovery(s string) string {
	s = strings.ToLower(strings.NewReplacer(" ", "", "-", "").Replace(s))
	if len(s) == 10 {
		return s[:5] + "-" + s[5:]
	}
	return s
}

// NewSalt returns a random salt for hashing a code.
func NewSalt() (string, error) {
	raw := make([]byte, 12)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw), nil
}

// HashCode is what is stored of a recovery or email code: its SHA-256 with a salt. A
// recovery code has 50 bits and lives only until used; the hash keeps a copy of the
// database from showing the codes.
func HashCode(salt, code string) string {
	sum := sha256.Sum256([]byte(salt + ":" + code))
	return hex.EncodeToString(sum[:])
}

// Equal compares two hashes without leaking where they differ.
func Equal(a, b string) bool { return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1 }

// NewEmailCode returns a six-digit code for the email step.
func NewEmailCode() (string, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(1000000))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%0*d", Digits, n.Int64()), nil
}

// QRSVG draws text as a QR code, an inline SVG with a quiet zone, black on white.
func QRSVG(text string) (string, error) {
	bc, err := qr.Encode(text, qr.M, qr.Auto)
	if err != nil {
		return "", err
	}
	bc, err = barcode.Scale(bc, bc.Bounds().Dx(), bc.Bounds().Dy())
	if err != nil {
		return "", err
	}
	w, h := bc.Bounds().Dx(), bc.Bounds().Dy()
	const quiet = 2
	var b strings.Builder
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %d %d" shape-rendering="crispEdges" aria-hidden="true"><rect width="100%%" height="100%%" fill="#fff"/><path fill="#000" d="`, w+2*quiet, h+2*quiet)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			if r, _, _, _ := bc.At(x, y).RGBA(); r == 0 {
				fmt.Fprintf(&b, "M%d %dh1v1h-1z", x+quiet, y+quiet)
			}
		}
	}
	b.WriteString(`"/></svg>`)
	return b.String(), nil
}
