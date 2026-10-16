package llm

import (
	"context"
	"errors"
	"net"
)

// WithFallback returns a Model that calls primary, and, when primary's final
// error (after whatever retries primary already applied) is a 429, a 503 or
// 504, or a deadline, calls fallback once with the same request and Model set
// to fallbackModel. A 400 or an unusable response is returned as-is. An empty
// fallbackModel leaves primary unchanged, which is the default (D56).
func WithFallback(primary, fallback Model, fallbackModel string) Model {
	if fallbackModel == "" {
		return primary
	}
	return &fallingBack{primary: primary, next: fallback, model: fallbackModel}
}

type fallingBack struct {
	primary Model
	next    Model
	model   string
}

func (f *fallingBack) Provider() string { return f.primary.Provider() }

func (f *fallingBack) Generate(ctx context.Context, req Request) (Response, error) {
	resp, err := f.primary.Generate(ctx, req)
	if err == nil || !fallbackEligible(err) {
		return resp, err
	}
	alt := req
	alt.Model = f.model
	resp, err = f.next.Generate(ctx, alt)
	if err != nil {
		return Response{}, err
	}
	// The caller records this id in trace.model_id, so a fallback answer is
	// distinguishable from a primary answer even when the provider's version
	// string differs.
	resp.ModelID = f.model
	return resp, nil
}

// fallbackEligible is the set of final primary errors that may be answered
// by the fallback model. Other 4xx and bad responses are not.
func fallbackEligible(err error) bool {
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}
	var api *APIError
	if errors.As(err, &api) {
		switch api.StatusCode {
		case 429, 503, 504:
			return true
		}
	}
	return false
}
