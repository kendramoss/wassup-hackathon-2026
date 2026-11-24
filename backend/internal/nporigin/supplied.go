package nporigin

import (
	"fmt"
	"sort"
	"strings"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/tariffshift"
)

// Applied says how supplied facts were read into an Input. Facts that no
// longer fit the product or the rules are left out and named here, never
// an error: a parts change must not break the check (D806).
type Applied struct {
	// Stale lists the supplied materials not read because the part at that
	// position is gone or changed (its name, its code, or it now has a code
	// or an origin the fact no longer fits).
	Stale []int `json:"stale,omitempty"`
	// Dropped names the other facts not read: "good_hts" (it no longer
	// lengthens the product's code), "row", or "answers.<key>".
	Dropped []string `json:"dropped,omitempty"`
	// AllStale: the whole set was saved before the parts list was replaced.
	AllStale bool `json:"all_stale,omitempty"`
}

// Fit keeps the supplied facts that still fit the product and these rules,
// and says what it left out. It never fails.
func (r *Rules) Fit(p domain.Product, s *domain.SuppliedOriginFacts) (*domain.SuppliedOriginFacts, Applied) {
	var ap Applied
	if s == nil {
		return nil, ap
	}
	if s.Stale {
		ap.AllStale = true
		return nil, ap
	}
	out := &domain.SuppliedOriginFacts{Source: s.Source, UpdatedAt: s.UpdatedAt, Actor: s.Actor}
	if s.GoodHTS != "" {
		if extends(p.HTS, s.GoodHTS) {
			out.GoodHTS = s.GoodHTS
		} else {
			ap.Dropped = append(ap.Dropped, "good_hts")
		}
	}
	if s.Row != "" {
		if r.ValidRow(s.Row) {
			out.Row = s.Row
		} else {
			ap.Dropped = append(ap.Dropped, "row")
		}
	}
	for _, k := range sortedBoolKeys(s.Answers) {
		if !r.ValidAnswer(k) {
			ap.Dropped = append(ap.Dropped, "answers."+k)
			continue
		}
		if out.Answers == nil {
			out.Answers = map[string]bool{}
		}
		out.Answers[k] = s.Answers[k]
	}
	seen := map[int]bool{}
	for _, m := range s.Materials {
		if !materialFits(p, m) || seen[m.Index] {
			ap.Stale = append(ap.Stale, m.Index)
			continue
		}
		seen[m.Index] = true
		out.Materials = append(out.Materials, m)
	}
	return out, ap
}

// materialFits: the part is still there as it was, and each fact still
// fills or lengthens what the part lacks.
func materialFits(p domain.Product, m domain.SuppliedMaterial) bool {
	if m.Index < 0 || m.Index >= len(p.Components) {
		return false
	}
	c := p.Components[m.Index]
	if strings.TrimSpace(c.Name) != strings.TrimSpace(m.Name) || tariffshift.Digits(c.HTS) != tariffshift.Digits(m.PartHTS) {
		return false
	}
	if m.HTS != "" {
		if d := tariffshift.Digits(m.HTS); len(d) < 4 || len(d) > 10 || (strings.TrimSpace(c.HTS) != "" && !extends(c.HTS, m.HTS)) {
			return false
		}
	}
	if m.Origin != "" && strings.TrimSpace(c.MadeIn) != "" {
		return false
	}
	for _, v := range []*float64{m.ValueShare, m.WeightShare} {
		if v != nil && (*v < 0 || *v > 1 || *v != *v) {
			return false
		}
	}
	return true
}

func sortedBoolKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// FromProduct reads a case's product into an Input the way the case's U.S.
// rule check does (its code, its parts' codes and origins, country the
// production country), then adds the supplied facts (#770, D801). Pass facts
// through Rules.Fit first; a material that does not fit is skipped here too.
// The product is not changed.
func FromProduct(p domain.Product, country string, s *domain.SuppliedOriginFacts) Input {
	in := Input{GoodHTS: p.HTS, Country: country}
	for _, c := range p.Components {
		in.Materials = append(in.Materials, Material{Name: c.Name, HTS: c.HTS, Origin: c.MadeIn})
	}
	if s == nil {
		return in
	}
	if extends(p.HTS, s.GoodHTS) {
		in.GoodHTS = s.GoodHTS
	}
	in.Row = s.Row
	if len(s.Answers) > 0 {
		in.Answers = make(map[string]bool, len(s.Answers))
		for k, v := range s.Answers {
			in.Answers[k] = v
		}
	}
	for _, m := range s.Materials {
		if !materialFits(p, m) {
			continue
		}
		c := p.Components[m.Index]
		t := &in.Materials[m.Index]
		if m.HTS != "" {
			t.HTS = m.HTS
		}
		if m.Origin != "" && strings.TrimSpace(c.MadeIn) == "" {
			t.Origin = m.Origin
		}
		t.ValueShare, t.WeightShare = m.ValueShare, m.WeightShare
	}
	return in
}

// extends: code is longer than base and starts with it, digit for digit.
func extends(base, code string) bool {
	b, c := tariffshift.Digits(base), tariffshift.Digits(code)
	return b != "" && len(c) > len(b) && strings.HasPrefix(c, b)
}

// ValidateSupplied checks supplied facts against a product and these rules:
// keys the rules ask, a row they have, parts that exist, codes that only fill
// or lengthen, origins only where the part has none. Shares are checked on
// the merged Input by the caller.
func (r *Rules) ValidateSupplied(p domain.Product, s domain.SuppliedOriginFacts) error {
	if s.GoodHTS != "" && !extends(p.HTS, s.GoodHTS) {
		return fmt.Errorf("good_hts must be a longer code that starts with the product's code %q", p.HTS)
	}
	if s.Row != "" && !r.ValidRow(s.Row) {
		return fmt.Errorf("row %q is not a row of these rules", s.Row)
	}
	for k := range s.Answers {
		if !r.ValidAnswer(k) {
			return fmt.Errorf("answers: %q is not a question these rules ask", k)
		}
	}
	seen := map[int]bool{}
	for i, m := range s.Materials {
		if m.Index < 0 || m.Index >= len(p.Components) {
			return fmt.Errorf("materials[%d].index %d is not a part of the product", i, m.Index)
		}
		if seen[m.Index] {
			return fmt.Errorf("materials[%d].index %d is given twice", i, m.Index)
		}
		seen[m.Index] = true
		for _, v := range []*float64{m.ValueShare, m.WeightShare} {
			if v != nil && (*v < 0 || *v > 1 || *v != *v) {
				return fmt.Errorf("materials[%d]: shares must be from 0 to 1", i)
			}
		}
		c := p.Components[m.Index]
		if m.HTS != "" {
			d := tariffshift.Digits(m.HTS)
			if len(d) < 4 || len(d) > 10 {
				return fmt.Errorf("materials[%d].hts must be an HS code of 4 to 10 digits", i)
			}
			if strings.TrimSpace(c.HTS) != "" && !extends(c.HTS, m.HTS) {
				return fmt.Errorf("materials[%d].hts must start with the part's code %q", i, c.HTS)
			}
		}
		if m.Origin != "" {
			if strings.TrimSpace(c.MadeIn) != "" {
				return fmt.Errorf("materials[%d].origin: the part already has an origin; change it on the product", i)
			}
			if domain.NormalizeCountry(m.Origin) == "" {
				return fmt.Errorf("materials[%d].origin is not a country", i)
			}
		}
	}
	return nil
}
