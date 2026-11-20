package budget

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/llm"
)

type caseKey struct{}

// WithCase tags a context with the case being evaluated.
func WithCase(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, caseKey{}, id)
}

// Opts configures a Guard. Zero values pick the conservative defaults.
type Opts struct {
	AllowModelCalls  bool // must be set by the explicit CLI flag
	RunID            string
	AttemptTimeout   time.Duration // default 120s
	MaxInputTokens   int64         // default 40000 (upper bound, bytes based)
	MaxOutputCeiling int64         // default 8192
	// MaxCaseAttempts bounds raw HTTP attempts for one case. A verifier run
	// legitimately makes up to MaxToolCalls+1 model calls, each possibly
	// retried, so derive it: (tool calls + 1) * (retries + 1). Default 14.
	MaxCaseAttempts int
	// MaxCaseNanos bounds spend + reservations for one case. Default $1.00.
	MaxCaseNanos     int64
	MaxFailureStreak int // consecutive failed attempts across cases; default 3
	// AllowedCases, when non-nil, is the frozen eligibility list: a request
	// whose case id is not in it is refused before any reservation.
	AllowedCases map[string]bool
	// RunEndsBy: prices must stay valid until then (zero: only now).
	RunEndsBy time.Time
	Now       func() time.Time
}

// Guard wraps the raw transport (gateway.Client). Every Generate is exactly
// one reservation and at most one wire request; retries belong above it.
type Guard struct {
	inner  llm.Model
	ledger *Ledger
	prices *PriceTable
	o      Opts
	sem    chan struct{} // concurrency 1

	mu     sync.Mutex
	halted error // in-memory sticky halt (budget exhausted, bound violations)

	hookAfterReserve func() // tests
}

// NewGuard builds the wrapper.
func NewGuard(inner llm.Model, l *Ledger, p *PriceTable, o Opts) *Guard {
	if o.AttemptTimeout <= 0 {
		o.AttemptTimeout = 120 * time.Second
	}
	if o.MaxInputTokens <= 0 {
		o.MaxInputTokens = 40000
	}
	if o.MaxOutputCeiling <= 0 {
		o.MaxOutputCeiling = 8192
	}
	if o.MaxCaseAttempts <= 0 {
		o.MaxCaseAttempts = 14
	}
	if o.MaxCaseNanos <= 0 {
		o.MaxCaseNanos = nanosPerUSD
	}
	if o.MaxFailureStreak <= 0 {
		o.MaxFailureStreak = 3
	}
	if o.Now == nil {
		o.Now = time.Now
	}
	return &Guard{inner: inner, ledger: l, prices: p, o: o, sem: make(chan struct{}, 1)}
}

// Provider implements llm.Model.
func (g *Guard) Provider() string { return g.inner.Provider() }

// Halted reports the sticky in-memory halt, if any. The runner must check it
// after every case and stop, instead of recording per-case errors and going on.
func (g *Guard) Halted() error {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.halted
}

func (g *Guard) halt(err error, persist string) error {
	g.mu.Lock()
	if g.halted == nil {
		g.halted = err
	}
	g.mu.Unlock()
	if persist != "" {
		_ = g.ledger.Halt(persist)
	}
	return err
}

// InputTokensUpper is a deliberately loose bound: one token per serialized
// byte (a token never covers less than one byte) plus a per-message and
// per-tool allowance for the wire format's field names. It is a bound by
// construction for UTF-8 text, and any provider-reported violation is a
// sticky halt.
func InputTokensUpper(req llm.Request) (int64, error) {
	b, err := json.Marshal(struct {
		S string
		M []llm.Message
		T []llm.Tool
		J map[string]any
	}{req.System, req.Messages, req.Tools, req.JSONSchema})
	if err != nil {
		return 0, err
	}
	return int64(len(b)) + 1024 + 128*int64(len(req.Messages)+len(req.Tools)+1), nil
}

func promptHash(req llm.Request) string {
	b, _ := json.Marshal(req)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

// Generate implements llm.Model.
func (g *Guard) Generate(ctx context.Context, req llm.Request) (llm.Response, error) {
	if !g.o.AllowModelCalls {
		return llm.Response{}, ErrNotAllowed
	}
	if err := g.Halted(); err != nil {
		return llm.Response{}, err
	}
	// Concurrency 1: wait for the slot, honouring cancellation without a
	// reservation.
	select {
	case g.sem <- struct{}{}:
		defer func() { <-g.sem }()
	case <-ctx.Done():
		return llm.Response{}, ctx.Err()
	}
	if err := g.Halted(); err != nil {
		return llm.Response{}, err
	}
	if err := ctx.Err(); err != nil {
		return llm.Response{}, err
	}
	caseID, _ := ctx.Value(caseKey{}).(string)
	if g.o.AllowedCases != nil && !g.o.AllowedCases[caseID] {
		return llm.Response{}, ErrCaseNotEligible
	}
	if req.MaxOutputTokens <= 0 {
		return llm.Response{}, ErrNoOutputCap
	}
	if int64(req.MaxOutputTokens) > g.o.MaxOutputCeiling {
		return llm.Response{}, ErrOutputCap
	}
	price, err := g.prices.Lookup(req.Model, g.o.Now(), g.o.RunEndsBy)
	if err != nil {
		return llm.Response{}, err
	}
	inUp, err := InputTokensUpper(req)
	if err != nil {
		return llm.Response{}, fmt.Errorf("budget: size request: %w", err)
	}
	if inUp > g.o.MaxInputTokens {
		return llm.Response{}, g.halt(ErrInputBound, "request exceeded the configured input bound")
	}
	maxOut := int64(req.MaxOutputTokens)
	base := CostNanos(inUp, price.inNano) + CostNanos(maxOut, price.outNano)
	margin := base / 50
	if margin < 1_000_000 { // 0.001 USD
		margin = 1_000_000
	}
	res, err := g.ledger.Reserve(ReserveReq{RunID: g.o.RunID, CaseID: caseID, Alias: req.Model,
		PriceSource: price.Source, PriceVerified: price.VerifiedAt, Nanos: base + margin,
		InUpper: inUp, MaxOut: maxOut, PromptSHA: promptHash(req),
		MaxCaseAttempts: g.o.MaxCaseAttempts, MaxCaseNanos: g.o.MaxCaseNanos, MaxFailureStreak: g.o.MaxFailureStreak})
	if err != nil {
		if errors.Is(err, ErrBudget) {
			return llm.Response{}, g.halt(err, "") // exhausted: nothing further this run
		}
		if errors.Is(err, ErrTamper) || errors.Is(err, ErrPoisoned) || errors.Is(err, ErrHalted) || errors.Is(err, ErrFailureStreak) {
			return llm.Response{}, g.halt(err, "")
		}
		return llm.Response{}, err
	}
	if g.hookAfterReserve != nil {
		g.hookAfterReserve()
	}
	// Reserved, transport not yet invoked: a cancel here is certain to cost 0.
	if cerr := ctx.Err(); cerr != nil {
		if serr := g.ledger.Settle(res, SettleReq{Outcome: OutcomeNotSent, ErrorClass: "canceled_before_send"}); serr != nil {
			return llm.Response{}, g.halt(serr, "")
		}
		return llm.Response{}, cerr
	}
	settled := false
	defer func() {
		if p := recover(); p != nil {
			if !settled {
				_ = g.ledger.Settle(res, SettleReq{Outcome: OutcomeFailure, Charged: res.Nanos, ErrorClass: "panic"})
			}
			panic(p)
		}
	}()
	actx, cancel := context.WithTimeout(WithPermit(ctx, &Permit{}), g.o.AttemptTimeout)
	defer cancel()
	resp, callErr := g.inner.Generate(actx, req)
	settled = true
	if callErr != nil {
		sr := SettleReq{Outcome: OutcomeFailure, Charged: res.Nanos, ErrorClass: classify(callErr)}
		var api *llm.APIError
		if errors.As(callErr, &api) {
			sr.HTTPStatus = api.StatusCode
		}
		switch {
		case errors.Is(callErr, context.DeadlineExceeded):
			sr.Outcome = OutcomeTimeout
		case errors.Is(callErr, context.Canceled):
			sr.Outcome = OutcomeCanceled
		}
		if serr := g.ledger.Settle(res, sr); serr != nil {
			return llm.Response{}, g.halt(errors.Join(callErr, serr), "")
		}
		return llm.Response{}, callErr
	}
	u := resp.Usage
	if u.InputTokens <= 0 || u.OutputTokens < 0 {
		// Unknown usage on a 200: the bound cannot be verified. Keep the
		// full reservation and stop until a human reconciles.
		if serr := g.ledger.Settle(res, SettleReq{Outcome: OutcomeOKUnknownUse, Charged: res.Nanos, ErrorClass: "no_usage"}); serr != nil {
			return llm.Response{}, g.halt(serr, "")
		}
		return llm.Response{}, g.halt(fmt.Errorf("%w: usage absent on a successful response", ErrHalted), "usage absent on a 200")
	}
	actual := CostNanos(int64(u.InputTokens), price.inNano) + CostNanos(int64(u.OutputTokens), price.outNano)
	sr := SettleReq{Outcome: OutcomeOK, Charged: actual, InTok: int64(u.InputTokens), OutTok: int64(u.OutputTokens)}
	switch {
	case !price.Allows(resp.ModelID):
		sr.Outcome, sr.ErrorClass = OutcomeBadModel, "resolved_model_not_allowed"
		if actual < res.Nanos {
			sr.Charged = res.Nanos // identity unknown: price may be wrong, keep the whole reservation
		}
		if serr := g.ledger.Settle(res, sr); serr != nil {
			return llm.Response{}, g.halt(serr, "")
		}
		return llm.Response{}, g.halt(ErrModelNotAllowed, "response model id not in the verified allowlist")
	case int64(u.OutputTokens) > maxOut || int64(u.InputTokens) > inUp || actual > res.Nanos:
		sr.Outcome, sr.ErrorClass = OutcomeOverrun, "bound_exceeded"
		if serr := g.ledger.Settle(res, sr); serr != nil {
			return llm.Response{}, g.halt(serr, "")
		}
		return llm.Response{}, g.halt(fmt.Errorf("%w: usage %d in / %d out exceeded the reserved bound", ErrHalted, u.InputTokens, u.OutputTokens),
			"provider usage exceeded the reserved bound")
	}
	if err := g.ledger.Settle(res, sr); err != nil {
		return llm.Response{}, g.halt(err, "")
	}
	return resp, nil
}

func classify(err error) string {
	var api *llm.APIError
	switch {
	case errors.As(err, &api):
		if api.StatusCode >= 300 && api.StatusCode < 400 {
			return "redirect"
		}
		return fmt.Sprintf("http_%d", api.StatusCode)
	case errors.Is(err, ErrNoPermit), errors.Is(err, ErrPermitUsed):
		return "permit"
	case errors.Is(err, context.DeadlineExceeded):
		return "timeout"
	case errors.Is(err, context.Canceled):
		return "canceled"
	case errors.Is(err, llm.ErrBadResponse):
		return "bad_response"
	case errors.Is(err, llm.ErrUnavailable):
		return "unavailable"
	}
	return "transport"
}

var _ llm.Model = (*Guard)(nil)

// CaseFrom returns the case id attached by WithCase, or "".
func CaseFrom(ctx context.Context) string {
	s, _ := ctx.Value(caseKey{}).(string)
	return s
}
