package app

import (
	"net"
	"net/http"
	"net/netip"
	"strings"
	"sync"
	"time"

	"github.com/amon2126/nutreek/internal/i18n"
)

// ipRateLimiter limits requests per IP with a fixed window: the first attempt
// opens the bucket, the window runs from there, and once expired it starts
// over. In memory, per process: it resets on restart. Used both as a chi
// middleware and directly by handleLogin (allow/reset).
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

// allow returns true when the IP may make another request. Call it BEFORE
// processing the request; when it denies, the caller answers 429.
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

// reset deletes the IP's bucket — called after a *successful* login so a
// legitimate user does not stay blocked by earlier attempts.
func (l *ipRateLimiter) reset(ip string) {
	l.mu.Lock()
	delete(l.buckets, ip)
	l.mu.Unlock()
}

// pruneLocked removes the expired buckets at most once per window, so the
// map does not grow without bound with ever-new IPs. Requires l.mu.
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

// size returns the number of buckets in memory (used by tests).
func (l *ipRateLimiter) size() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.buckets)
}

// Middleware returns a chi-compatible middleware that blocks requests over
// the threshold with 429 Too Many Requests.
func (l *ipRateLimiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !l.allow(remoteIP(r)) {
			http.Error(w, i18n.T(i18n.FromContext(r.Context()), "auth.error.too_many_requests"), http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// remoteIP extracts the client's IP for rate limiting.
//
// X-Forwarded-For counts only when the connection comes from a trusted proxy
// (loopback or a private network: Traefik/nginx on the same host or in
// Docker). A client connecting directly cannot pick its IP by sending the
// header. The chain is read from the right: the first untrusted address is
// the one added by our last proxy, so it cannot be forged. When the whole
// chain is private (a client on the LAN), the first address counts.
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

// isTrustedProxy: loopback and private networks (10/8, 172.16/12, 192.168/16,
// fc00::/7) count as our own infrastructure.
func isTrustedProxy(a netip.Addr) bool {
	a = a.Unmap()
	return a.IsLoopback() || a.IsPrivate()
}
