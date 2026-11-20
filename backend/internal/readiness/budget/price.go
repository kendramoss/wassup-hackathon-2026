// Package budget is the hard spend guard for the readiness evaluation: a
// pricing-proof table, an append-only hash-chained attempts ledger with a
// process-wide lock, and an llm.Model wrapper that reserves money before
// every raw provider attempt. Money is integer nano-USD throughout; the only
// float parse is none: prices and the shared file are parsed as decimal text.
package budget

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"strings"
	"time"
)

// Typed refusals. Every one of them means no HTTP request was made, except
// where a comment says otherwise. None of them is retryable: they match
// neither llm.ErrUnavailable nor *llm.APIError.
var (
	ErrNotAllowed      = errors.New("budget: model calls not allowed (missing --allow-model-calls)")
	ErrUnauthorized    = errors.New("budget: shared ledger calls_authorized is not true")
	ErrUnknownPrice    = errors.New("budget: no verified price for alias")
	ErrNoOutputCap     = errors.New("budget: request has no max output tokens")
	ErrOutputCap       = errors.New("budget: max output tokens above the configured ceiling")
	ErrInputBound      = errors.New("budget: request larger than the configured input bound")
	ErrBudget          = errors.New("budget: next attempt could exceed the hard cap")
	ErrCaseCap         = errors.New("budget: per-case attempt or spend ceiling reached")
	ErrCaseNotEligible = errors.New("budget: case is not in the frozen eligibility list")
	ErrFailureStreak   = errors.New("budget: consecutive failure stop")
	ErrTamper          = errors.New("budget: ledger inconsistent with shared budget file")
	ErrCorrupt         = errors.New("budget: attempts ledger corrupt or chain broken")
	ErrLocked          = errors.New("budget: ledger is locked by another process")
	ErrPoisoned        = errors.New("budget: guard latched closed after a ledger fault")
	ErrHalted          = errors.New("budget: sticky halt, a human must reconcile and acknowledge")
	ErrNonLocalFS      = errors.New("budget: ledger must live on a local, non-synced filesystem")
	ErrNoPermit        = errors.New("budget: request has no one-shot permit from the guard")
	ErrPermitUsed      = errors.New("budget: permit already used (redirect or replay refused)")
	ErrModelNotAllowed = errors.New("budget: resolved model id not in the verified allowlist")
	ErrNoRunWindow     = errors.New("budget: RunEndsBy is not set or already past; the price proof lifetime must cover the run")
	// ErrNotSent marks a failure raised before any byte could leave the
	// process (encoding, URL allowlist, permit). Only the gateway adapter may
	// wrap it; the guard then charges nothing for that attempt. Anything
	// ambiguous (dial, TLS, write, read, timeout) is NOT this error and is
	// charged in full.
	ErrNotSent = errors.New("budget: request known not to have been sent")
)

// HardCapNanos is the user-approved total for the whole batch ($20).
const HardCapNanos int64 = 20_000_000_000

const nanosPerUSD = 1_000_000_000

// Price is one verified gateway price. All fields are required except the
// worst-tier pair. The price the guard uses is the maximum of every figure
// given (list, gateway-reported, worst context tier), never a promo guess.
type Price struct {
	Alias                   string      `json:"alias"`
	ResolvedModelIDs        []string    `json:"resolved_model_ids"`
	ListIn                  json.Number `json:"list_usd_per_mtok_in"`
	ListOut                 json.Number `json:"list_usd_per_mtok_out"`
	GatewayIn               json.Number `json:"gateway_usd_per_mtok_in"`
	GatewayOut              json.Number `json:"gateway_usd_per_mtok_out"`
	WorstTierIn             json.Number `json:"worst_tier_usd_per_mtok_in,omitempty"`
	WorstTierOut            json.Number `json:"worst_tier_usd_per_mtok_out,omitempty"`
	Source                  string      `json:"source"`
	VerifiedAt              string      `json:"verified_at"`
	ValidUntil              string      `json:"valid_until"`
	MaxOutputEnforced       bool        `json:"max_output_enforced"`
	OutputIncludesReasoning bool        `json:"output_includes_reasoning"`

	inNano, outNano int64 // effective nano-USD per Mtok
}

// InNanoPerMtok and OutNanoPerMtok are the effective (maximum) prices.
func (p Price) InNanoPerMtok() int64  { return p.inNano }
func (p Price) OutNanoPerMtok() int64 { return p.outNano }

// PriceTable is a validated pricing-proof file.
type PriceTable struct {
	entries map[string]Price
	maxAge  time.Duration
}

type priceFile struct {
	Schema  int     `json:"schema"`
	Entries []Price `json:"entries"`
}

// DefaultMaxPriceAge is how old a verification may be at the time of a call.
const DefaultMaxPriceAge = 72 * time.Hour

// LoadPriceTable reads and validates a pricing-proof file.
func LoadPriceTable(path string, maxAge time.Duration) (*PriceTable, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("budget: read pricing proof: %w", err)
	}
	return ParsePriceTable(b, maxAge)
}

// ParsePriceTable validates bytes. Unknown fields, duplicate aliases and every
// malformed entry are errors, never skipped.
func ParsePriceTable(b []byte, maxAge time.Duration) (*PriceTable, error) {
	if maxAge <= 0 {
		maxAge = DefaultMaxPriceAge
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	dec.UseNumber()
	var f priceFile
	if err := dec.Decode(&f); err != nil {
		return nil, fmt.Errorf("budget: pricing proof: %w", err)
	}
	if f.Schema != 2 {
		return nil, errors.New("budget: pricing proof: schema must be 2")
	}
	t := &PriceTable{entries: map[string]Price{}, maxAge: maxAge}
	for i, p := range f.Entries {
		if err := p.finish(); err != nil {
			return nil, fmt.Errorf("budget: pricing proof entry %d: %w", i, err)
		}
		if _, dup := t.entries[p.Alias]; dup {
			return nil, fmt.Errorf("budget: pricing proof: duplicate alias %q", p.Alias)
		}
		t.entries[p.Alias] = p
	}
	return t, nil
}

func maxI(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

func (p *Price) finish() error {
	switch {
	case p.Alias == "":
		return errors.New("empty alias")
	case len(p.ResolvedModelIDs) == 0:
		return errors.New("resolved_model_ids must list the verified ids the alias resolves to")
	case p.Source == "":
		return errors.New("empty source")
	case !p.MaxOutputEnforced:
		return errors.New("max_output_enforced must be attested true")
	case !p.OutputIncludesReasoning:
		return errors.New("output_includes_reasoning must be attested true (hidden reasoning must be inside the output cap)")
	}
	for _, id := range p.ResolvedModelIDs {
		if id == "" {
			return errors.New("empty resolved model id")
		}
	}
	var err error
	get := func(n json.Number, required bool) (int64, error) {
		if n == "" {
			if required {
				return 0, errors.New("missing price field")
			}
			return 0, nil
		}
		v, err := ParseDecimalNanos(n.String(), 9)
		if err != nil {
			return 0, err
		}
		if v <= 0 || v > 1000*nanosPerUSD {
			return 0, errors.New("price must be > 0 and <= 1000 USD per Mtok")
		}
		return v, nil
	}
	var li, lo, gi, go_, ti, to int64
	if li, err = get(p.ListIn, true); err != nil {
		return err
	}
	if lo, err = get(p.ListOut, true); err != nil {
		return err
	}
	if gi, err = get(p.GatewayIn, true); err != nil {
		return err
	}
	if go_, err = get(p.GatewayOut, true); err != nil {
		return err
	}
	if ti, err = get(p.WorstTierIn, false); err != nil {
		return err
	}
	if to, err = get(p.WorstTierOut, false); err != nil {
		return err
	}
	p.inNano, p.outNano = maxI(maxI(li, gi), ti), maxI(maxI(lo, go_), to)
	if _, err := time.Parse(time.RFC3339, p.VerifiedAt); err != nil {
		return fmt.Errorf("verified_at: %w", err)
	}
	if _, err := time.Parse(time.RFC3339, p.ValidUntil); err != nil {
		return fmt.Errorf("valid_until: %w", err)
	}
	return nil
}

// Lookup returns the price for exactly alias (no prefix matching) when it is
// not stale, not expired before runEnd, and not dated in the future.
func (t *PriceTable) Lookup(alias string, now, runEnd time.Time) (Price, error) {
	if t == nil {
		return Price{}, fmt.Errorf("%w %q (no table)", ErrUnknownPrice, alias)
	}
	p, ok := t.entries[alias]
	if !ok {
		return Price{}, fmt.Errorf("%w %q", ErrUnknownPrice, alias)
	}
	v, _ := time.Parse(time.RFC3339, p.VerifiedAt)
	u, _ := time.Parse(time.RFC3339, p.ValidUntil)
	switch {
	case v.After(now):
		return Price{}, fmt.Errorf("%w %q: verified_at is in the future", ErrUnknownPrice, alias)
	case now.Sub(v) > t.maxAge:
		return Price{}, fmt.Errorf("%w %q: verification older than %s", ErrUnknownPrice, alias, t.maxAge)
	case !now.Before(u) || (!runEnd.IsZero() && u.Before(runEnd)):
		return Price{}, fmt.Errorf("%w %q: price validity ends before the run", ErrUnknownPrice, alias)
	}
	return p, nil
}

// Allows reports whether id is one of the verified resolved model ids.
func (p Price) Allows(id string) bool {
	for _, x := range p.ResolvedModelIDs {
		if x == id {
			return true
		}
	}
	return false
}

// CostNanos prices tokens at nanoPerMtok, rounded up, in integer arithmetic.
// It saturates (instead of overflowing) so an absurd input cannot wrap under
// the cap.
func CostNanos(tokens, nanoPerMtok int64) int64 {
	if tokens <= 0 || nanoPerMtok <= 0 {
		return 0
	}
	const sat = math.MaxInt64 / 4
	if tokens > sat/nanoPerMtok {
		return sat
	}
	return (tokens*nanoPerMtok + 999_999) / 1_000_000
}

// ParseDecimalNanos parses a non-negative decimal like "19.999" into
// fixed-point units of 10^-9 without floating point. More fraction digits
// than maxFrac, exponents and signs are errors (fail closed).
func ParseDecimalNanos(s string, maxFrac int) (int64, error) {
	s = strings.TrimSpace(s)
	if s == "" || strings.ContainsAny(s, "eE+- ") {
		return 0, fmt.Errorf("bad decimal %q", s)
	}
	whole, frac, _ := strings.Cut(s, ".")
	if whole == "" || len(frac) > maxFrac || len(whole) > 12 {
		return 0, fmt.Errorf("bad decimal %q", s)
	}
	var w, f int64
	for _, c := range whole {
		if c < '0' || c > '9' {
			return 0, fmt.Errorf("bad decimal %q", s)
		}
		w = w*10 + int64(c-'0')
	}
	frac += strings.Repeat("0", 9-len(frac))
	for _, c := range frac {
		if c < '0' || c > '9' {
			return 0, fmt.Errorf("bad decimal %q", s)
		}
		f = f*10 + int64(c-'0')
	}
	return w*nanosPerUSD + f, nil
}

// FormatNanos renders nano-USD as an exact decimal JSON number.
func FormatNanos(n int64) string { return fmt.Sprintf("%d.%09d", n/nanosPerUSD, n%nanosPerUSD) }
