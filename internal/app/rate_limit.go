package app

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// loginRateLimiter limita i tentativi POST /login per indirizzo IP.
// In-memory, per-process: si azzera al restart. Per l'MVP basta a fermare
// il brute-force banale; per persistenza serverebbe una tabella DB con
// pulizia periodica.
type loginRateLimiter struct {
	mu      sync.Mutex
	attempts map[string]*ipBucket
	max      int
	window   time.Duration
}

type ipBucket struct {
	count int
	first time.Time
}

func newLoginRateLimiter(max int, window time.Duration) *loginRateLimiter {
	return &loginRateLimiter{
		attempts: make(map[string]*ipBucket, 64),
		max:      max,
		window:   window,
	}
}

// allow ritorna true se l'IP puo' tentare un altro login. Va chiamata PRIMA
// di processare la POST. Se nega, il chiamante deve restituire 429.
// La finestra e' fissa: il primo tentativo apre il bucket, la window scorre
// da li, scaduta si resetta al prossimo allow().
func (l *loginRateLimiter) allow(ip string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	b, ok := l.attempts[ip]
	if !ok || now.Sub(b.first) > l.window {
		l.attempts[ip] = &ipBucket{count: 1, first: now}
		return true
	}
	if b.count >= l.max {
		return false
	}
	b.count++
	return true
}

// reset cancella il bucket dell'IP — chiamato dopo un login *riuscito*
// cosi' un utente legittimo non resta bloccato dai tentativi precedenti.
func (l *loginRateLimiter) reset(ip string) {
	l.mu.Lock()
	delete(l.attempts, ip)
	l.mu.Unlock()
}

// gc rimuove i bucket scaduti. Da invocare periodicamente per non far
// crescere indefinitamente la mappa. Per MVP non lanciamo una goroutine
// dedicata: chiamiamo gc() da allow() ogni N tentativi (vedi prune).
func (l *loginRateLimiter) prune(now time.Time) {
	for k, b := range l.attempts {
		if now.Sub(b.first) > l.window {
			delete(l.attempts, k)
		}
	}
}

// ipRateLimiter è un rate limiter generico per IP usabile come middleware chi.
// Stessa logica di loginRateLimiter ma esposto come http.Handler chain.
type ipRateLimiter struct {
	mu      sync.Mutex
	buckets map[string]*ipBucket
	max     int
	window  time.Duration
}

func newIPRateLimiter(max int, window time.Duration) *ipRateLimiter {
	return &ipRateLimiter{
		buckets: make(map[string]*ipBucket, 64),
		max:     max,
		window:  window,
	}
}

func (l *ipRateLimiter) allow(ip string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	b, ok := l.buckets[ip]
	if !ok || now.Sub(b.first) > l.window {
		l.buckets[ip] = &ipBucket{count: 1, first: now}
		return true
	}
	if b.count >= l.max {
		return false
	}
	b.count++
	return true
}

// Middleware ritorna un chi-compatible middleware che blocca le richieste oltre
// la soglia con 429 Too Many Requests.
func (l *ipRateLimiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !l.allow(remoteIP(r)) {
			http.Error(w, "Troppe richieste. Riprova tra qualche minuto.", http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// remoteIP estrae l'IP del client. Preferisce X-Forwarded-For (primo hop) se
// presente — assume reverse proxy fidato in prod. In dev legge RemoteAddr.
func remoteIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if i := strings.IndexByte(xff, ','); i >= 0 {
			return strings.TrimSpace(xff[:i])
		}
		return strings.TrimSpace(xff)
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
