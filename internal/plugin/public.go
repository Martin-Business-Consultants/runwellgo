package plugin

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"sync"
	"syscall"
	"time"
)

// PublicHTTP is a client for any address on the public internet (granted
// "http:public"): a client's own website, say, which no manifest can name
// ahead of time. It never reaches this server, its network or a cloud's
// metadata service: the address actually dialed is checked (so neither DNS
// nor a redirect gets around it), and responses are read to 4 MB.
func (a *API) PublicHTTP() (*http.Client, error) {
	if err := a.need("http:public", a.grants.PublicHTTP); err != nil {
		return nil, err
	}
	return publicClient, nil
}

var publicClient = &http.Client{
	Timeout: callTimeout,
	Transport: &capping{next: &http.Transport{
		DialContext:           (&net.Dialer{Timeout: 5 * time.Second, Control: publicOnly}).DialContext,
		TLSHandshakeTimeout:   5 * time.Second,
		ResponseHeaderTimeout: 8 * time.Second,
		MaxIdleConns:          4,
		IdleConnTimeout:       30 * time.Second,
	}},
	CheckRedirect: func(_ *http.Request, via []*http.Request) error {
		if len(via) >= 5 {
			return errors.New("too many redirects")
		}
		return nil
	},
}

// cgnat is carrier-grade NAT space, not the public internet.
var cgnat = netip.MustParsePrefix("100.64.0.0/10")

// publicOnly refuses to dial anything but a public unicast address.
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
	if !ip.IsGlobalUnicast() || ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() || cgnat.Contains(ip) {
		return fmt.Errorf("%s isn't a public address", ip)
	}
	return nil
}

// capping caps every response at 4 MB.
type capping struct{ next http.RoundTripper }

func (c *capping) RoundTrip(r *http.Request) (*http.Response, error) {
	res, err := c.next.RoundTrip(r)
	if err != nil {
		return nil, err
	}
	res.Body = &capped{ReadCloser: res.Body, left: maxHTTPResponse}
	return res, nil
}

// backgroundLimit is how long work a plugin runs in the background may take,
// and how many of a plugin's may run at once.
const (
	backgroundLimit = 5 * time.Minute
	backgroundMax   = 4
)

var running sync.Map // plugin key → *int (its jobs running), under runningMu
var runningMu sync.Mutex

// Background runs fn after the call that asked for it, for up to five
// minutes (a probe of a slow site, say), with the same API: writes are
// recorded as the same caller. At most four of a plugin's run at once; it
// answers an error when that many are running. A panic is logged, not fatal.
func (a *API) Background(name string, fn func(ctx context.Context, api *API)) error {
	runningMu.Lock()
	n, _ := running.LoadOrStore(a.key, new(int))
	count := n.(*int) //nolint:errcheck,forcetypeassert // running holds only *int
	if *count >= backgroundMax {
		runningMu.Unlock()
		return fmt.Errorf("this plugin already has %d jobs running; try again in a moment", backgroundMax)
	}
	*count++
	runningMu.Unlock()
	done := func() { runningMu.Lock(); *count--; runningMu.Unlock() }
	api := *a
	api.live = nil // no longer the caller's own request: tools only read
	ok := a.h.spawn(context.Background(), "plugin "+a.key+" "+name, func(ctx context.Context) {
		defer done()
		ctx, cancel := context.WithTimeout(ctx, backgroundLimit)
		defer cancel()
		defer func() {
			if r := recover(); r != nil {
				a.h.log.Error("plugin background work panicked", "plugin", a.key, "job", name, "panic", r)
			}
		}()
		fn(ctx, &api)
	})
	if !ok {
		done()
		return errors.New("Runwell is shutting down") //nolint:staticcheck // ST1005: Runwell is a name
	}
	return nil
}

// PrivateHTTP is a client for any address (granted "http:private"),
// this server's own network included: a server the owner runs beside
// Runwell (a model server on the office network, say), at an address only
// an owner sets. Switching a plugin on never grants it; an owner ticks it.
// It doesn't wait on a slow answer beyond the context's deadline (set
// Timeout on a copy for less), and reads at most 4 MB of one.
func (a *API) PrivateHTTP() (*http.Client, error) {
	if err := a.need("http:private", a.grants.PrivateHTTP); err != nil {
		return nil, err
	}
	return privateClient, nil
}

var privateClient = &http.Client{
	Transport: &capping{next: &http.Transport{
		Proxy:               http.ProxyFromEnvironment,
		DialContext:         (&net.Dialer{Timeout: 5 * time.Second}).DialContext,
		TLSHandshakeTimeout: 5 * time.Second,
		MaxIdleConns:        4,
		IdleConnTimeout:     30 * time.Second,
	}},
	CheckRedirect: func(_ *http.Request, via []*http.Request) error {
		if len(via) >= 5 {
			return errors.New("too many redirects")
		}
		return nil
	},
}
