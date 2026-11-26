package app

import (
	"net/http/httptest"
	"net/netip"
	"testing"
)

func ipFrom(peer, xff string) string {
	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = peer + ":40000"
	if xff != "" {
		r.Header.Set("X-Forwarded-For", xff)
	}
	return remoteIP(r)
}

func withProxies(t *testing.T, set bool, list ...string) {
	t.Helper()
	var prefixes []netip.Prefix
	for _, s := range list {
		prefixes = append(prefixes, netip.MustParsePrefix(s))
	}
	SetTrustedProxies(prefixes, set)
	t.Cleanup(func() { SetTrustedProxies(nil, false) })
}

// Not set: as before, loopback and private networks are believed.
func TestTrustedProxiesDefault(t *testing.T) {
	withProxies(t, false)
	if got := ipFrom("10.0.0.9", "8.8.8.8"); got != "8.8.8.8" {
		t.Fatalf("a private peer is trusted by default, got %q", got)
	}
}

// Set: only the listed proxies are believed. A private host that is not listed cannot choose its address.
func TestTrustedProxiesListed(t *testing.T) {
	withProxies(t, true, "10.0.0.5/32", "172.18.0.0/16")
	cases := []struct {
		name, peer, xff, want string
	}{
		{"listed proxy, forwarded client", "10.0.0.5", "8.8.8.8", "8.8.8.8"},
		{"listed range", "172.18.4.7", "9.9.9.9", "9.9.9.9"},
		{"private peer not listed picks nothing", "10.0.0.9", "8.8.8.8", "10.0.0.9"},
		{"loopback not listed either", "127.0.0.1", "8.8.8.8", "127.0.0.1"},
		{"a public peer is never a proxy", "203.0.113.4", "8.8.8.8", "203.0.113.4"},
		{"a listed proxy, a chain: the first address that is not ours", "10.0.0.5", "1.1.1.1, 8.8.8.8, 172.18.0.2", "8.8.8.8"},
		{"a listed proxy with no header", "10.0.0.5", "", "10.0.0.5"},
	}
	for _, tc := range cases {
		if got := ipFrom(tc.peer, tc.xff); got != tc.want {
			t.Errorf("%s: got %q, want %q", tc.name, got, tc.want)
		}
	}
}

// "none": nobody is believed, not even loopback.
func TestTrustedProxiesNone(t *testing.T) {
	withProxies(t, true)
	if got := ipFrom("127.0.0.1", "8.8.8.8"); got != "127.0.0.1" {
		t.Fatalf("got %q", got)
	}
}

// A client who forges the header on every try still lands in one bucket, so the limit holds.
func TestForgedForwardedForDoesNotMakeNewBuckets(t *testing.T) {
	withProxies(t, true, "10.0.0.5/32")
	seen := map[string]bool{}
	for _, forged := range []string{"1.1.1.1", "2.2.2.2", "3.3.3.3", "4.4.4.4"} {
		seen[ipFrom("10.0.0.9", forged)] = true
	}
	if len(seen) != 1 {
		t.Fatalf("forged addresses made %d buckets, want 1", len(seen))
	}
}
