package app

import (
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"sync"
	"sync/atomic"
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
// (the loopback address, or the proxies listed in NUTREEK_TRUSTED_PROXIES). A client
// connecting directly cannot pick its IP by sending the header. The chain is read from the right: the first untrusted address is
// the one added by our last proxy, so it cannot be forged. When the whole
// chain is private (a client on the LAN), the first address counts.
func remoteIP(r *http.Request) string { return bucketIP(clientIP(r)) }

// bucketIP keys an IPv6 address by its /64: one network owns that many addresses, and a
// client must not get a fresh limit for each of them.
func bucketIP(ip string) string {
	a, err := netip.ParseAddr(ip)
	if err != nil || !a.Is6() || a.Is4In6() {
		return ip
	}
	p, err := a.Prefix(64)
	if err != nil {
		return ip
	}
	return p.String()
}

func clientIP(r *http.Request) string {
	peer := r.RemoteAddr
	if host, _, err := net.SplitHostPort(peer); err == nil {
		peer = host
	}
	peerAddr, err := netip.ParseAddr(peer)
	if err != nil || !isTrustedProxy(peerAddr) {
		if err == nil && r.Header.Get("X-Forwarded-For") != "" && (peerAddr.Unmap().IsPrivate() || peerAddr.Unmap().IsLoopback()) {
			warnUnlistedProxy.Do(func() {
				slog.Warn("X-Forwarded-For arrived from a private address that is not in NUTREEK_TRUSTED_PROXIES: it is ignored, so every visitor behind it counts as that address. If this is your reverse proxy, list it in NUTREEK_TRUSTED_PROXIES (and give its container a fixed address)", "peer", peer)
			})
		}
		return peer
	}

	// The chain is read from the right. An entry that does not parse (a proxy that writes
	// ip:port, or "unknown") stops the walk: skipping it would pick an address the client wrote.
	var parts []string
	for _, v := range r.Header.Values("X-Forwarded-For") {
		for part := range strings.SplitSeq(v, ",") {
			parts = append(parts, strings.TrimSpace(part))
		}
	}
	if len(parts) == 0 {
		return peer
	}
	for i := len(parts) - 1; i >= 0; i-- {
		a, err := netip.ParseAddr(parts[i])
		if err != nil {
			return peer
		}
		a = a.Unmap()
		if !isTrustedProxy(a) {
			return a.String()
		}
	}
	first, _ := netip.ParseAddr(parts[0])
	return first.Unmap().String()
}

var warnUnlistedProxy sync.Once

// trustedProxies is the list set by NUTREEK_TRUSTED_PROXIES (#346); nil means it was not set.
var trustedProxies atomic.Pointer[[]netip.Prefix]

// SetTrustedProxies says which proxy addresses may tell us the client's address in
// X-Forwarded-For. With set false the default holds (loopback only); an empty list trusts nobody.
func SetTrustedProxies(list []netip.Prefix, set bool) {
	if !set {
		trustedProxies.Store(nil)
		return
	}
	if list == nil {
		list = []netip.Prefix{}
	}
	trustedProxies.Store(&list)
}

// isTrustedProxy: with NUTREEK_TRUSTED_PROXIES set, an address in the list; otherwise only the
// loopback address (a proxy on the same host). A reverse proxy in another container or machine
// has to be listed.
func isTrustedProxy(a netip.Addr) bool {
	a = a.Unmap()
	if list := trustedProxies.Load(); list != nil {
		for _, p := range *list {
			if p.Contains(a) {
				return true
			}
		}
		return false
	}
	return a.IsLoopback()
}
