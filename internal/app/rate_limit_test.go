package app

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestLoginRateLimiter(t *testing.T) {
	rl := newIPRateLimiter(3, 1*time.Minute)
	for i := 0; i < 3; i++ {
		if !rl.allow("1.2.3.4") {
			t.Fatalf("tentativo %d non dovrebbe essere bloccato", i+1)
		}
	}
	if rl.allow("1.2.3.4") {
		t.Errorf("4° tentativo dovrebbe essere bloccato")
	}
	// Altro IP non interferisce.
	if !rl.allow("5.6.7.8") {
		t.Errorf("IP diverso non dovrebbe essere bloccato")
	}
	// reset (login successful) → ricomincia da capo.
	rl.reset("1.2.3.4")
	if !rl.allow("1.2.3.4") {
		t.Errorf("dopo reset, primo tentativo dovrebbe essere ammesso")
	}
}

func TestLoginRateLimiterWindowExpiry(t *testing.T) {
	rl := newIPRateLimiter(2, 50*time.Millisecond)
	rl.allow("ip")
	rl.allow("ip")
	if rl.allow("ip") {
		t.Errorf("3° dentro la window dovrebbe essere bloccato")
	}
	time.Sleep(60 * time.Millisecond)
	if !rl.allow("ip") {
		t.Errorf("dopo expiry window dovrebbe essere ammesso di nuovo")
	}
}

func TestRemoteIP(t *testing.T) {
	cases := []struct {
		name     string
		xff      string
		remote   string
		expected string
	}{
		{"no XFF, host:port", "", "1.2.3.4:55555", "1.2.3.4"},
		{"RemoteAddr senza porta", "", "5.6.7.8", "5.6.7.8"},
		// Client diretto (IP pubblico): l'header e' ignorato.
		{"client diretto, XFF falsificato", "9.9.9.9", "1.2.3.4:55555", "1.2.3.4"},
		// Dietro proxy fidato: vale l'ultimo IP non fidato della catena.
		{"proxy locale, XFF singolo", "9.9.9.9", "127.0.0.1:40000", "9.9.9.9"},
		{"proxy docker, catena con IP falsificato a sinistra", "6.6.6.6, 9.9.9.9", "172.18.0.2:40000", "9.9.9.9"},
		{"due proxy privati", "9.9.9.9, 10.0.0.5", "172.18.0.2:40000", "9.9.9.9"},
		{"client in LAN, catena tutta privata", "192.168.1.50", "172.18.0.2:40000", "192.168.1.50"},
		{"proxy fidato, XFF non valido", "garbage", "127.0.0.1:40000", "127.0.0.1"},
		{"proxy fidato IPv6 loopback", "9.9.9.9", "[::1]:40000", "9.9.9.9"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest("POST", "/login", nil)
			r.RemoteAddr = tc.remote
			if tc.xff != "" {
				r.Header.Set("X-Forwarded-For", tc.xff)
			}
			if got := remoteIP(r); got != tc.expected {
				t.Errorf("got %q want %q", got, tc.expected)
			}
		})
	}
}

// TestRateLimiterSpoofedXFF: un client diretto che cambia X-Forwarded-For a
// ogni richiesta resta nello stesso bucket e viene bloccato.
func TestRateLimiterSpoofedXFF(t *testing.T) {
	rl := newIPRateLimiter(5, time.Minute)
	handler := rl.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	for i := 1; i <= 6; i++ {
		req := httptest.NewRequest(http.MethodPost, "/login", nil)
		req.RemoteAddr = "203.0.113.7:5000"
		req.Header.Set("X-Forwarded-For", fmt.Sprintf("198.51.100.%d", i))
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		want := http.StatusOK
		if i == 6 {
			want = http.StatusTooManyRequests
		}
		if rec.Code != want {
			t.Fatalf("richiesta %d: atteso %d, got %d", i, want, rec.Code)
		}
	}
}

// TestRateLimiterPrunesExpired: i bucket scaduti vengono eliminati.
func TestRateLimiterPrunesExpired(t *testing.T) {
	rl := newIPRateLimiter(5, 20*time.Millisecond)
	for i := range 100 {
		rl.allow(fmt.Sprintf("198.51.100.%d", i))
	}
	if got := rl.size(); got != 100 {
		t.Fatalf("bucket attesi 100, got %d", got)
	}
	time.Sleep(30 * time.Millisecond)
	rl.allow("203.0.113.1")
	if got := rl.size(); got != 1 {
		t.Errorf("dopo la pulizia atteso 1 bucket, got %d", got)
	}
}

// TestLoginRateLimiterConcurrent: il limiter del login e' creato una volta
// sola anche con richieste concorrenti (go test -race).
func TestLoginRateLimiterConcurrent(t *testing.T) {
	a := &App{}
	var wg sync.WaitGroup
	seen := make([]*ipRateLimiter, 20)
	for i := range seen {
		wg.Add(1)
		go func() {
			defer wg.Done()
			seen[i] = a.loginRateLimiter()
		}()
	}
	wg.Wait()
	for i, l := range seen {
		if l == nil || l != seen[0] {
			t.Fatalf("goroutine %d: limiter diverso o nil", i)
		}
	}
}

func TestValidRequestID(t *testing.T) {
	cases := map[string]bool{
		"":                                     false,
		"a1b2c3d4e5f60718":                     true,
		"123e4567-e89b-12d3-a456-426614174000": true,
		"bad id":                               false,
		"inject\nlog":                          false,
		strings.Repeat("a", 65):                false,
	}
	for id, want := range cases {
		if got := validRequestID(id); got != want {
			t.Errorf("validRequestID(%q) = %v, want %v", id, got, want)
		}
	}
}

func TestIPRateLimiterAllow(t *testing.T) {
	rl := newIPRateLimiter(3, time.Minute)
	for i := 0; i < 3; i++ {
		if !rl.allow("1.2.3.4") {
			t.Fatalf("richiesta %d non dovrebbe essere bloccata", i+1)
		}
	}
	if rl.allow("1.2.3.4") {
		t.Error("4a richiesta dovrebbe essere bloccata")
	}
	if !rl.allow("5.6.7.8") {
		t.Error("IP diverso non dovrebbe essere bloccato")
	}
}

func TestIPRateLimiterMiddleware429(t *testing.T) {
	rl := newIPRateLimiter(2, time.Minute)
	handler := rl.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.RemoteAddr = "1.2.3.4:1234"
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("richiesta %d: atteso 200, got %d", i+1, rec.Code)
		}
	}
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "1.2.3.4:1234"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusTooManyRequests {
		t.Errorf("3a richiesta: atteso 429, got %d", rec.Code)
	}
}
