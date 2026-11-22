// Package jurisdiction is the market a case's origin question is asked for
// (D680): the list of markets, a deterministic detector over the case text,
// and the registry of markets that have an origin engine. The United States
// is registered, and the server adds Canada, whose engine is a rules check
// (D725). A case for any other market gets no prediction and goes to
// review; a new authority is added by registering it.
package jurisdiction

import (
	"context"
	"fmt"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/engine"
)

// Market codes.
const (
	US    = "us"
	EU    = "eu"
	UK    = "uk"
	CA    = "ca"
	JP    = "jp"
	KR    = "kr"
	AU    = "au"
	MX    = "mx"
	Other = "other"
)

// How a case's market was set.
const (
	SourceChosen   = "chosen"
	SourceDetected = "detected"
	SourceDefault  = "default"
)

// Market is one destination market.
type Market struct {
	Code string `json:"code"`
	// Name is the market's plain name ("European Union").
	Name string `json:"name"`
	// phrase is the name as it reads inside a sentence ("the European Union").
	phrase string
}

// Markets lists every market a case can name, United States first.
var Markets = []Market{
	{US, "United States", "the United States"},
	{EU, "European Union", "the European Union"},
	{UK, "United Kingdom", "the United Kingdom"},
	{CA, "Canada", "Canada"},
	{JP, "Japan", "Japan"},
	{KR, "South Korea", "South Korea"},
	{AU, "Australia", "Australia"},
	{MX, "Mexico", "Mexico"},
	{Other, "Other market", "this market"},
}

// Codes is every market code, in Markets order.
func Codes() []string {
	out := make([]string, len(Markets))
	for i, m := range Markets {
		out[i] = m.Code
	}
	return out
}

// Lookup returns the market with the code.
func Lookup(code string) (Market, bool) {
	for _, m := range Markets {
		if m.Code == code {
			return m, true
		}
	}
	return Market{}, false
}

// Valid reports whether code is a market code.
func Valid(code string) bool { _, ok := Lookup(code); return ok }

// Name is the market's plain name ("" for an unknown code).
func Name(code string) string { m, _ := Lookup(code); return m.Name }

// UnsupportedReason is the triage reason for a case whose market has no
// registered engine.
func UnsupportedReason(code string) string {
	m, ok := Lookup(code)
	if !ok {
		m = Market{phrase: "this market"}
	}
	return fmt.Sprintf("Origin rules for %s aren't supported yet", m.phrase)
}

// ConflictReason is the triage reason when the text names U.S. entry and
// another market with equal weight.
const ConflictReason = "The text names more than one market. Pick the market to get an answer"

// HintReason is the triage reason when the router could not tell the rule
// and the text hints at a non-U.S. market (D680).
func HintReason(code string) string {
	return fmt.Sprintf("The text suggests %s, not U.S. entry. Pick the market to get an answer", Name(code))
}

// Engine predicts origin under one market's rules.
type Engine interface {
	Predict(ctx context.Context, c domain.Case) (engine.Result, error)
}

// Authority is a registered market: who decides origin there, under which
// rules, from which precedents, and the engine that predicts it.
type Authority struct {
	Market string
	// Agency is the authority whose decisions the engine predicts.
	Agency string
	// Rules names the origin rules the engine applies.
	Rules string
	// Precedents names the ruling pool the engine retrieves from.
	Precedents string
	// Engine predicts origin. Nil only for the United States, whose cases go
	// through the router and the regime engines (httpapi dispatch).
	Engine Engine
	// FastPath is the auto-accept bar for a non-U.S. engine; 0 reviews
	// every case (a new authority without a held-out result).
	FastPath float64
}

// Registry maps a market to its authority.
type Registry struct{ m map[string]Authority }

// NewRegistry returns a registry with the given authorities.
func NewRegistry(as ...Authority) (*Registry, error) {
	r := &Registry{m: map[string]Authority{}}
	for _, a := range as {
		if err := r.Register(a); err != nil {
			return nil, err
		}
	}
	return r, nil
}

// USAuthority is the registered U.S. authority: CBP, through the router and
// the regime engines.
var USAuthority = Authority{
	Market:     US,
	Agency:     "U.S. Customs and Border Protection",
	Rules:      "substantial transformation; 19 CFR 102.21 (textiles); 19 CFR 102.11 and 102.20 (Part 102 marking)",
	Precedents: "CBP rulings",
}

// Default is the registry with the United States only. The server adds
// Canada as a rules-only market when the rule check is on (D725).
func Default() *Registry {
	r, _ := NewRegistry(USAuthority)
	return r
}

// Register adds an authority. A market can be registered once; "other" is
// never registered (it is not one market). Only the United States may have
// no engine.
func (r *Registry) Register(a Authority) error {
	switch {
	case !Valid(a.Market) || a.Market == Other:
		return fmt.Errorf("jurisdiction: cannot register market %q", a.Market)
	case r.m[a.Market].Market != "":
		return fmt.Errorf("jurisdiction: market %q is already registered", a.Market)
	case a.Engine == nil && a.Market != US:
		return fmt.Errorf("jurisdiction: market %q needs an engine", a.Market)
	case a.FastPath < 0 || a.FastPath > 1:
		return fmt.Errorf("jurisdiction: market %q fast path %v is outside [0, 1]", a.Market, a.FastPath)
	}
	r.m[a.Market] = a
	return nil
}

// Authority returns the market's registered authority. A nil registry has
// the United States only.
func (r *Registry) Authority(code string) (Authority, bool) {
	if r == nil {
		if code == US {
			return USAuthority, true
		}
		return Authority{}, false
	}
	a, ok := r.m[code]
	return a, ok
}

// Supported reports whether the market has a registered authority.
func (r *Registry) Supported(code string) bool { _, ok := r.Authority(code); return ok }

// SupportedCodes lists the registered markets, in Markets order.
func (r *Registry) SupportedCodes() []string {
	var out []string
	for _, c := range Codes() {
		if r.Supported(c) {
			out = append(out, c)
		}
	}
	return out
}

// Resolve sets a case's market: the caller's choice when given (it must be a
// market code), else the detector's reading of the text.
func Resolve(chosen, text string) (domain.Jurisdiction, error) {
	if chosen != "" {
		if !Valid(chosen) {
			return domain.Jurisdiction{}, fmt.Errorf("%w: jurisdiction %q is not one of %v", domain.ErrInvalid, chosen, Codes())
		}
		return domain.Jurisdiction{Code: chosen, Source: SourceChosen}, nil
	}
	return Detect(text), nil
}

// Of returns a stored case's market, United States when it has none (a case
// made before D680).
func Of(j *domain.Jurisdiction) domain.Jurisdiction {
	if j == nil || j.Code == "" {
		return domain.Jurisdiction{Code: US, Source: SourceDefault}
	}
	return *j
}

// Detail is the market as the API returns it.
type Detail struct {
	Code      string `json:"code"`
	Name      string `json:"name"`
	Source    string `json:"source"`
	Cue       string `json:"cue,omitempty"`
	Hint      string `json:"hint,omitempty"`
	Supported bool   `json:"supported"`
}

// DetailOf describes j for the API.
func (r *Registry) DetailOf(j *domain.Jurisdiction) Detail {
	v := Of(j)
	return Detail{Code: v.Code, Name: Name(v.Code), Source: v.Source, Cue: v.Cue, Hint: v.Hint, Supported: r.Supported(v.Code)}
}

// List is every market with whether it is supported, for GET /v1/jurisdictions.
func (r *Registry) List() []map[string]any {
	out := make([]map[string]any, 0, len(Markets))
	for _, m := range Markets {
		out = append(out, map[string]any{"code": m.Code, "name": m.Name, "supported": r.Supported(m.Code)})
	}
	return out
}
