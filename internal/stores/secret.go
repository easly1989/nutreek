package stores

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"io"
)

// MinSecretLen is the shortest NUTREEK_SECRET_KEY accepted.
const MinSecretLen = 16

// Box seals the tokens of the plugins before they go in the database (AES-256-GCM,
// the key being the SHA-256 of NUTREEK_SECRET_KEY). Without the key a stolen copy of
// the database holds no usable token.
type Box struct{ aead cipher.AEAD }

// NewBox builds a Box from the secret; it returns an error when the secret is too short.
func NewBox(secret string) (*Box, error) {
	if len(secret) < MinSecretLen {
		return nil, errors.New("NUTREEK_SECRET_KEY: at least 16 characters required (e.g. openssl rand -hex 32)")
	}
	key := sha256.Sum256([]byte(secret))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &Box{aead: aead}, nil
}

// Seal returns the text to store for a token.
func (b *Box) Seal(token string) (string, error) {
	nonce := make([]byte, b.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(b.aead.Seal(nonce, nonce, []byte(token), nil)), nil
}

// Open returns the token of a sealed text; it fails when the key is not the one that
// sealed it.
func (b *Box) Open(sealed string) (string, error) {
	raw, err := base64.StdEncoding.DecodeString(sealed)
	if err != nil || len(raw) < b.aead.NonceSize() {
		return "", errors.New("stores: the stored token is not readable")
	}
	n := b.aead.NonceSize()
	plain, err := b.aead.Open(nil, raw[:n], raw[n:], nil)
	if err != nil {
		return "", errors.New("stores: the stored token is not readable with this key")
	}
	return string(plain), nil
}
