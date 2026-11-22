package notify

import (
	"context"
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"strings"
	"testing"
	"time"

	webpush "github.com/SherClockHolmes/webpush-go"

	"github.com/amon2126/nutreek/internal/db"
)

// The sender never connects to a private, loopback or link-local address, whatever
// endpoint a user gives, and an error does not carry the endpoint.
func TestPublicOnly(t *testing.T) {
	for addr, ok := range map[string]bool{
		"127.0.0.1:443":         false,
		"[::1]:443":             false,
		"10.0.0.5:8443":         false,
		"192.168.1.10:443":      false,
		"172.16.0.1:443":        false,
		"169.254.169.254:443":   false,
		"100.64.0.1:443":        false,
		"0.0.0.0:443":           false,
		"[::ffff:127.0.0.1]:80": false,
		"[fe80::1]:443":         false,
		"142.250.180.78:443":    true,
		"[2a00:1450::5]:443":    true,
	} {
		if err := publicOnly("tcp", addr, nil); (err == nil) != ok {
			t.Errorf("publicOnly(%s) = %v, want allowed=%v", addr, err, ok)
		}
	}

	priv, pub, err := webpush.GenerateVAPIDKeys()
	if err != nil {
		t.Fatal(err)
	}
	dev, _ := ecdh.P256().GenerateKey(rand.Reader)
	auth := make([]byte, 16)
	_, _ = rand.Read(auth)
	sub := db.PushSub{
		Endpoint: "https://127.0.0.1:9/secret-token-123",
		P256dh:   base64.RawURLEncoding.EncodeToString(dev.PublicKey().Bytes()),
		Auth:     base64.RawURLEncoding.EncodeToString(auth),
	}
	sender := WebPush{Keys: VAPID{Public: pub, Private: priv}, Subscriber: "https://example.org/c", Client: SafeClient(2 * time.Second)}
	_, err = sender.Send(context.Background(), sub, Message{Title: "t"})
	if err == nil || !strings.Contains(err.Error(), "not public") {
		t.Fatalf("a loopback endpoint: err = %v, want the address refused", err)
	}
	if strings.Contains(err.Error(), "secret-token-123") || errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("the error carries the endpoint or timed out: %v", err)
	}
}
