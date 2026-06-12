package app

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestLoginRateLimiter(t *testing.T) {
	rl := newLoginRateLimiter(3, 1*time.Minute)
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
	rl := newLoginRateLimiter(2, 50*time.Millisecond)
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
		{"XFF singolo", "9.9.9.9", "1.2.3.4:55555", "9.9.9.9"},
		{"XFF multi: prendiamo primo", "9.9.9.9, 10.0.0.1", "1.2.3.4:55555", "9.9.9.9"},
		{"RemoteAddr senza porta", "", "5.6.7.8", "5.6.7.8"},
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
