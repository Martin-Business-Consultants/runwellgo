// Package outbound is the HTTP client webhooks and deploys post with, and
// the check on where it may go.
package outbound

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"syscall"
	"time"
)

// Client is what webhooks and deploys post with. Private and
// loopback addresses are allowed: a site on the same box or network is a
// normal setup for a one-file CMS. Link-local (a cloud's metadata service
// at 169.254.169.254), unspecified and multicast addresses aren't, checked
// on the address actually dialed, so neither DNS nor a redirect gets there.
var Client = &http.Client{
	Timeout: 15 * time.Second,
	Transport: &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   10 * time.Second,
			KeepAlive: 30 * time.Second,
			Control:   refuse,
		}).DialContext,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 15 * time.Second,
		MaxIdleConns:          10,
		IdleConnTimeout:       90 * time.Second,
	},
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 {
			return errors.New("too many redirects")
		}
		return CheckURL(req.URL.String())
	},
}

// refuse is the dialer's last word on where a post may go.
func refuse(_, address string, _ syscall.RawConn) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return err
	}
	ip, err := netip.ParseAddr(host)
	if err != nil {
		return fmt.Errorf("outbound: not an IP address: %q", host)
	}
	if blocked(ip) {
		return fmt.Errorf("outbound: %s isn't an address webhooks and deploys may reach", ip)
	}
	return nil
}

func blocked(ip netip.Addr) bool {
	ip = ip.Unmap()
	return ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsInterfaceLocalMulticast() ||
		ip.IsMulticast() || ip.IsUnspecified()
}

// CheckURL refuses what isn't http(s), and a literal address the
// dialer would refuse anyway (said early, when the URL is saved).
func CheckURL(u string) error {
	if !strings.HasPrefix(u, "https://") && !strings.HasPrefix(u, "http://") {
		return fmt.Errorf("only http(s) URLs: %q", u)
	}
	if parsed, err := url.Parse(u); err == nil {
		if ip, err := netip.ParseAddr(strings.Trim(parsed.Hostname(), "[]")); err == nil && blocked(ip) {
			return fmt.Errorf("%s isn't an address webhooks and deploys may reach", ip)
		}
	}
	return nil
}
