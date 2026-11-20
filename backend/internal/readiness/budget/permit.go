package budget

import (
	"context"
	"net/http"
	"sync/atomic"
)

type permitState struct{ used atomic.Bool }

// Permit authorises exactly one wire request. Only the guard can create one:
// the state pointer is unexported and set only by mintPermit, so the zero
// value (or any value built outside this package) fails Take with
// ErrNoPermit. Copies share the same state, so a copy cannot replay it. The
// gateway transport consumes the permit in RoundTrip, which is why no code
// path reaches the network without a durable reservation, and why a redirect
// or replay (a second RoundTrip with the same permit) is refused.
type Permit struct{ st *permitState }

type permitKey struct{}

func mintPermit() *Permit { return &Permit{st: &permitState{}} }

// withPermit is unexported on purpose: attaching a permit is the guard's job.
func withPermit(ctx context.Context, p *Permit) context.Context {
	return context.WithValue(ctx, permitKey{}, p)
}

// PermitFrom returns the permit on ctx, or nil. The gateway only needs Take.
func PermitFrom(ctx context.Context) *Permit {
	p, _ := ctx.Value(permitKey{}).(*Permit)
	return p
}

// Take consumes the permit once.
func (p *Permit) Take() error {
	if p == nil || p.st == nil {
		return ErrNoPermit
	}
	if !p.st.used.CompareAndSwap(false, true) {
		return ErrPermitUsed
	}
	return nil
}

// DenyTransport refuses every request. The tool installs it as
// http.DefaultTransport and http.DefaultClient's transport as defense in
// depth against accidental stdlib use. It is a mitigation, not a sandbox:
// code can still dial sockets or build its own http.Transport, so an
// OS-level egress allowlist to the single gateway host remains mandatory
// before any live run.
type DenyTransport struct{}

// RoundTrip implements http.RoundTripper.
func (DenyTransport) RoundTrip(*http.Request) (*http.Response, error) { return nil, ErrNoPermit }
