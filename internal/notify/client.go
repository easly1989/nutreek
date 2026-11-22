package notify

import (
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"syscall"
	"time"
)

// cgnat is the shared address space 100.64.0.0/10, which IsPrivate does not cover.
var cgnat = netip.MustParsePrefix("100.64.0.0/10")

// publicOnly refuses a connection to a loopback, private, link-local, multicast,
// shared or unspecified address. The endpoint of a device is chosen by the user, so
// without this the server would POST to whatever address they give it. It runs on
// the resolved address at dial time, which also covers a name that points inside.
func publicOnly(_, address string, _ syscall.RawConn) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return err
	}
	ip, err := netip.ParseAddr(host)
	if err != nil {
		return err
	}
	ip = ip.Unmap()
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() ||
		ip.IsMulticast() || ip.IsUnspecified() || cgnat.Contains(ip) {
		return fmt.Errorf("push endpoint address %s is not public", ip)
	}
	return nil
}

// SafeClient is the HTTP client for the push services: public addresses only, no
// redirects (a push service answers 201 or an error), no proxy, a short timeout.
func SafeClient(timeout time.Duration) *http.Client {
	dialer := &net.Dialer{Timeout: timeout, Control: publicOnly}
	return &http.Client{
		Timeout:       timeout,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		Transport: &http.Transport{
			DialContext:         dialer.DialContext,
			TLSHandshakeTimeout: timeout,
			MaxIdleConnsPerHost: 2,
			IdleConnTimeout:     time.Minute,
		},
	}
}
