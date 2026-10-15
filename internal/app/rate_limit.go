package app

import (
	"net"
	"net/http"
	"net/netip"
	"strings"
	"sync"
	"time"
)

// ipRateLimiter limita le richieste per IP con una finestra fissa: il primo
// tentativo apre il bucket, la window scorre da li', scaduta si riparte.
// In-memory, per-process: si azzera al restart. Usato sia come middleware
// chi sia direttamente da handleLogin (allow/reset).
type ipRateLimiter struct {
	mu        sync.Mutex
	buckets   map[string]*ipBucket
	max       int
	window    time.Duration
	lastPrune time.Time
}

type ipBucket struct {
	count int
	first time.Time
}

func newIPRateLimiter(max int, window time.Duration) *ipRateLimiter {
	return &ipRateLimiter{
		buckets:   make(map[string]*ipBucket, 64),
		max:       max,
		window:    window,
		lastPrune: time.Now(),
	}
}

// allow ritorna true se l'IP puo' fare un'altra richiesta. Va chiamata PRIMA
// di processare la richiesta; se nega, il chiamante risponde 429.
func (l *ipRateLimiter) allow(ip string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	l.pruneLocked(now)
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

// reset cancella il bucket dell'IP — chiamato dopo un login *riuscito*
// cosi' un utente legittimo non resta bloccato dai tentativi precedenti.
func (l *ipRateLimiter) reset(ip string) {
	l.mu.Lock()
	delete(l.buckets, ip)
	l.mu.Unlock()
}

// pruneLocked elimina i bucket scaduti al massimo una volta per window, cosi'
// la mappa non cresce senza limite con IP sempre nuovi. Richiede l.mu.
func (l *ipRateLimiter) pruneLocked(now time.Time) {
	if now.Sub(l.lastPrune) < l.window {
		return
	}
	for ip, b := range l.buckets {
		if now.Sub(b.first) > l.window {
			delete(l.buckets, ip)
		}
	}
	l.lastPrune = now
}

// size ritorna il numero di bucket in memoria (usato dai test).
func (l *ipRateLimiter) size() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.buckets)
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

// remoteIP estrae l'IP del client per il rate limiting.
//
// X-Forwarded-For conta solo se la connessione arriva da un proxy fidato
// (loopback o rete privata: Traefik/nginx sullo stesso host o in Docker).
// Un client che si collega direttamente non puo' scegliersi l'IP mandando
// l'header. La catena si legge da destra: il primo indirizzo non fidato e'
// quello aggiunto dall'ultimo proxy nostro, quindi non falsificabile. Se la
// catena e' tutta privata (client in LAN) vale il primo indirizzo.
func remoteIP(r *http.Request) string {
	peer := r.RemoteAddr
	if host, _, err := net.SplitHostPort(peer); err == nil {
		peer = host
	}
	peerAddr, err := netip.ParseAddr(peer)
	if err != nil || !isTrustedProxy(peerAddr) {
		return peer
	}

	var chain []netip.Addr
	for _, v := range r.Header.Values("X-Forwarded-For") {
		for part := range strings.SplitSeq(v, ",") {
			if a, err := netip.ParseAddr(strings.TrimSpace(part)); err == nil {
				chain = append(chain, a.Unmap())
			}
		}
	}
	if len(chain) == 0 {
		return peer
	}
	for i := len(chain) - 1; i >= 0; i-- {
		if !isTrustedProxy(chain[i]) {
			return chain[i].String()
		}
	}
	return chain[0].String()
}

// isTrustedProxy: loopback e reti private (10/8, 172.16/12, 192.168/16,
// fc00::/7) sono considerate infrastruttura nostra.
func isTrustedProxy(a netip.Addr) bool {
	a = a.Unmap()
	return a.IsLoopback() || a.IsPrivate()
}
