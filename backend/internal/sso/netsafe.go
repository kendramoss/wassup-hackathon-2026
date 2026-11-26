package sso

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"syscall"
	"time"
)

// errBlockedAddress is a dial to an address an identity provider cannot have.
var errBlockedAddress = errors.New("sso: refusing to connect to a private, loopback or link-local address")

// publicOnly is a dialer Control that refuses non-public addresses, so a
// workspace admin's issuer URL cannot reach the metadata server or other
// internal services (SSRF). It runs after DNS resolution, on the address
// actually dialled, so a DNS name pointing inward is refused too.
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
	if !ip.IsGlobalUnicast() || ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsMulticast() || ip.IsUnspecified() || cgnat.Contains(ip) {
		return fmt.Errorf("%w (%s)", errBlockedAddress, ip)
	}
	return nil
}

var cgnat = netip.MustParsePrefix("100.64.0.0/10")

// SafeHTTPClient is the client for identity provider calls: public addresses
// only, no redirects to elsewhere beyond 3 hops, short timeouts, small bodies
// are enforced by the callers' decoders.
func SafeHTTPClient() *http.Client {
	d := &net.Dialer{Timeout: 5 * time.Second, Control: publicOnly}
	t := &http.Transport{
		Proxy:                 nil,
		DialContext:           d.DialContext,
		TLSHandshakeTimeout:   5 * time.Second,
		ResponseHeaderTimeout: 10 * time.Second,
		MaxIdleConns:          10,
		IdleConnTimeout:       60 * time.Second,
	}
	return &http.Client{Transport: t, Timeout: 15 * time.Second, CheckRedirect: func(_ *http.Request, via []*http.Request) error {
		if len(via) >= 3 {
			return errors.New("sso: too many redirects")
		}
		return nil
	}}
}

// Resolver looks up the TXT records that verify a domain.
type Resolver interface {
	LookupTXT(ctx context.Context, name string) ([]string, error)
}

// VerifyRecordName is the DNS name whose TXT record proves a domain.
func VerifyRecordName(domain string) string { return "_undelabs-verify." + domain }
