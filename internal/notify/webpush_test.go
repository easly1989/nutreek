package notify

import (
	"context"
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/amon2126/nutreek/internal/db"
)

// The VAPID keys are made once and kept; the sender signs with them, encrypts the
// payload for the device, and tells a device that is gone from one that failed.
func TestWebPush(t *testing.T) {
	store, err := db.Open(filepath.Join(t.TempDir(), "w.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	ctx := context.Background()
	if err := store.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	settings := db.NewInstanceSettings(store.DB)
	keys, err := EnsureVAPID(ctx, settings)
	if err != nil || keys.Public == "" || keys.Private == "" {
		t.Fatalf("EnsureVAPID = %+v, %v", keys, err)
	}
	again, _ := EnsureVAPID(ctx, settings)
	if again != keys {
		t.Error("the keys must be made once and kept")
	}

	// A device: a P-256 key and an auth secret, as a browser makes them.
	dev, _ := ecdh.P256().GenerateKey(rand.Reader)
	auth := make([]byte, 16)
	_, _ = rand.Read(auth)
	status := http.StatusCreated
	var gotAuth, gotEnc string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth, gotEnc = r.Header.Get("Authorization"), r.Header.Get("Content-Encoding")
		w.WriteHeader(status)
	}))
	defer srv.Close()
	sub := db.PushSub{
		Endpoint: srv.URL,
		P256dh:   base64.RawURLEncoding.EncodeToString(dev.PublicKey().Bytes()),
		Auth:     base64.RawURLEncoding.EncodeToString(auth),
	}
	sender := WebPush{Keys: keys, Subscriber: "https://example.org/contact"}

	if gone, err := sender.Send(ctx, sub, Message{Title: "t", Body: "b"}); err != nil || gone {
		t.Fatalf("a good send: gone=%v err=%v", gone, err)
	}
	if !strings.HasPrefix(gotAuth, "vapid t=") || !strings.Contains(gotAuth, "k="+keys.Public) || gotEnc != "aes128gcm" {
		t.Errorf("headers = %q, %q; want a VAPID signature with our key and aes128gcm", gotAuth, gotEnc)
	}
	status = http.StatusGone
	if gone, err := sender.Send(ctx, sub, Message{Title: "t"}); err != nil || !gone {
		t.Errorf("410: gone=%v err=%v, want gone", gone, err)
	}
	status = http.StatusInternalServerError
	if gone, err := sender.Send(ctx, sub, Message{Title: "t"}); err == nil || gone {
		t.Errorf("500: gone=%v err=%v, want an error and not gone", gone, err)
	}
}
