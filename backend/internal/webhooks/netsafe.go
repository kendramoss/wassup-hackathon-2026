package webhooks

import (
	"context"
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

// errBlocked is a target an endpoint may not have: private, loopback,
// link-local and similar addresses (SSRF).
var errBlocked = errors.New("webhooks: refusing a private, loopback or link-local address")

var cgnat = netip.MustParsePrefix("100.64.0.0/10")

func blockedIP(ip netip.Addr) bool {
	ip = ip.Unmap()
	return !ip.IsGlobalUnicast() || ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() ||
		ip.IsMulticast() || ip.IsUnspecified() || cgnat.Contains(ip)
}

// publicOnly is a dialer Control: it runs on the address actually dialled,
// after DNS, so a name that points inward (or changes its answer after
// registration) is refused at delivery too.
func publicOnly(_, address string, _ syscall.RawConn) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return err
	}
	ip, err := netip.ParseAddr(host)
	if err != nil {
		return err
	}
	if blockedIP(ip) {
		return fmt.Errorf("%w (%s)", errBlocked, ip.Unmap())
	}
	return nil
}

// Resolver looks up the addresses of a host.
type Resolver interface {
	LookupNetIP(ctx context.Context, network, host string) ([]netip.Addr, error)
}

// SafeClient is the delivery client: public addresses only, no redirects (a
// redirect is a failed delivery), no proxy, short timeouts.
func SafeClient(timeout time.Duration) *http.Client {
	d := &net.Dialer{Timeout: 5 * time.Second, Control: publicOnly}
	t := &http.Transport{
		Proxy:                 nil,
		DialContext:           d.DialContext,
		TLSHandshakeTimeout:   5 * time.Second,
		ResponseHeaderTimeout: timeout,
		MaxIdleConns:          10,
		IdleConnTimeout:       60 * time.Second,
	}
	return &http.Client{Transport: t, Timeout: timeout, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
}

// CheckURL validates a callback URL and returns it normalised. It must be
// https (unless Options.AllowInsecure, for tests), carry no credentials, and
// name a public host: not localhost, an internal name, or an address, or a
// name resolving to one, that is private, loopback or link-local.
func (s *Service) CheckURL(ctx context.Context, raw string) (string, error) {
	bad := func(format string, a ...any) (string, error) {
		return "", fmt.Errorf("%w: %s", ErrInvalid, fmt.Sprintf(format, a...))
	}
	raw = strings.TrimSpace(raw)
	if raw == "" || len(raw) > MaxURLLen {
		return bad("url is required and at most %d characters", MaxURLLen)
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || u.Opaque != "" {
		return bad("url is not a valid absolute URL")
	}
	if u.Scheme != "https" && !(s.opts.AllowInsecure && u.Scheme == "http") {
		return bad("url must use https")
	}
	if u.User != nil {
		return bad("url must not carry credentials")
	}
	if u.Fragment != "" {
		return bad("url must not have a fragment")
	}
	host := strings.ToLower(strings.TrimSuffix(u.Hostname(), "."))
	if host == "" {
		return bad("url has no host")
	}
	if p := u.Port(); p != "" {
		if n, err := net.LookupPort("tcp", p); err != nil || n < 1 {
			return bad("url has an invalid port")
		}
	}
	if s.opts.AllowInsecure {
		return u.String(), nil
	}
	if ip, err := netip.ParseAddr(host); err == nil {
		if blockedIP(ip) {
			return bad("url points at a private, loopback or link-local address")
		}
		return u.String(), nil
	}
	if !strings.Contains(host, ".") || host == "localhost" || strings.HasSuffix(host, ".localhost") || strings.HasSuffix(host, ".local") ||
		strings.HasSuffix(host, ".internal") || strings.HasSuffix(host, ".localdomain") || strings.HasSuffix(host, ".home.arpa") {
		return bad("url must name a public host")
	}
	rctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	ips, err := s.resolver().LookupNetIP(rctx, "ip", host)
	if err != nil || len(ips) == 0 {
		return bad("url host does not resolve")
	}
	for _, ip := range ips {
		if blockedIP(ip) {
			return bad("url host resolves to a private, loopback or link-local address")
		}
	}
	return u.String(), nil
}

func (s *Service) resolver() Resolver {
	if s.opts.Resolver != nil {
		return s.opts.Resolver
	}
	return net.DefaultResolver
}
