package llm

import (
	"context"
	"time"
)

// WithHedge returns a Model that sends the same request a second time when
// the first has not answered after `after`, and returns whichever answers
// first; the other is cancelled (D611). It cuts the slow tail of a provider
// that is sometimes slow on an ordinary request, at the cost of a second
// call for the requests that were slow. A non-positive after leaves m
// unchanged, which is the default.
//
// An error from one call is returned only when the other has failed too (or
// was never sent): a first call that fails before the hedge fires returns
// its error at once, so the retry policy outside sees it as before.
func WithHedge(m Model, after time.Duration) Model {
	if after <= 0 {
		return m
	}
	return &hedged{next: m, after: after}
}

type hedged struct {
	next  Model
	after time.Duration
}

func (h *hedged) Provider() string { return h.next.Provider() }

type hedgeResult struct {
	resp Response
	err  error
}

func (h *hedged) Generate(ctx context.Context, req Request) (Response, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel() // stops the call that lost
	out := make(chan hedgeResult, 2)
	send := func() {
		resp, err := h.next.Generate(ctx, req)
		out <- hedgeResult{resp, err}
	}
	go send()
	timer := time.NewTimer(h.after)
	defer timer.Stop()
	select {
	case r := <-out:
		return r.resp, r.err
	case <-timer.C:
	case <-ctx.Done():
		r := <-out
		return r.resp, r.err
	}
	go send()
	first := <-out
	if first.err == nil {
		return first.resp, nil
	}
	second := <-out
	if second.err == nil {
		return second.resp, nil
	}
	return first.resp, first.err
}
