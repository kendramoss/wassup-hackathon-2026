package budget

import (
	"context"
	"net/http"
	"sync/atomic"
)

// Permit authorises exactly one wire request. The guard creates it after the
// reservation is durable; the gateway transport consumes it in RoundTrip, so
// no code path can reach the network without a reservation, and a redirect or
// replay (a second RoundTrip with the same permit) is refused.
type Permit struct{ used atomic.Bool }

type permitKey struct{}

// WithPermit attaches p to ctx.
func WithPermit(ctx context.Context, p *Permit) context.Context {
	return context.WithValue(ctx, permitKey{}, p)
}

// PermitFrom returns the permit on ctx, or nil.
func PermitFrom(ctx context.Context) *Permit {
	p, _ := ctx.Value(permitKey{}).(*Permit)
	return p
}

// Take consumes the permit once.
func (p *Permit) Take() error {
	if p == nil {
		return ErrNoPermit
	}
	if !p.used.CompareAndSwap(false, true) {
		return ErrPermitUsed
	}
	return nil
}

// DenyTransport refuses every request. The tool installs it as
// http.DefaultTransport so nothing can reach the network around the guard.
type DenyTransport struct{}

// RoundTrip implements http.RoundTripper.
func (DenyTransport) RoundTrip(*http.Request) (*http.Response, error) { return nil, ErrNoPermit }
