package httpapi

import (
	"strings"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/planner"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/store"
)

// redescribed is c with every stored move description rewritten by
// planner.Describe (D220), so a plan search saved before the wording changed
// reads like a new one. Only descriptions change; c's own slices and maps
// are not written to.
func (a *api) redescribed(c store.Case) store.Case {
	if c.Product == nil {
		return c
	}
	if c.Plan != nil {
		r := *c.Plan
		planner.Redescribe(&r, c.Product, a.d.Templates)
		c.Plan = &r
	}
	if c.Targets != nil {
		t := *c.Targets
		if c.Targets.Results != nil {
			t.Results = make(map[string]planner.Result, len(c.Targets.Results))
			for k, r := range c.Targets.Results {
				planner.Redescribe(&r, c.Product, a.d.Templates)
				t.Results[k] = r
			}
		}
		t.Ranked = append([]store.TargetSummary(nil), c.Targets.Ranked...)
		for i := range t.Ranked {
			b := t.Ranked[i].Best
			if b == nil {
				continue
			}
			fam := c.Family
			if r, ok := c.Targets.Results[t.Ranked[i].Target]; ok && r.Family != "" {
				fam = r.Family
			}
			if tm, ok := a.d.Templates[fam]; ok {
				cp := *b
				cp.Description = planner.Describe(*c.Product, tm, cp.Move)
				t.Ranked[i].Best = &cp
			}
		}
		c.Targets = &t
	}
	return c
}

// planDescription is the description of a stored plan as the case reads now.
func (a *api) planDescription(c store.Case, p domain.Plan, family string) string {
	if family == "" {
		family = c.Family
	}
	if tm, ok := a.d.Templates[family]; ok && c.Product != nil {
		return planner.Describe(*c.Product, tm, p.Move)
	}
	return p.Description
}

// scenarioPlanCheck is what the latest plan search found for the same move as
// a scenario (D220): a scenario and a plan row that move the same groups to
// the same plants are one move, so they must carry one verdict. Nil when the
// scenario moves nothing, has no structured product, or the plan search did
// not try that move.
type scenarioPlanCheck struct {
	Shown            bool                 `json:"shown"`
	Confidence       *float64             `json:"confidence"`
	Bar              float64              `json:"bar"`
	SupportingRuling string               `json:"supporting_ruling,omitempty"`
	RejectionKind    domain.RejectionKind `json:"rejection_kind,omitempty"`
	RejectionReason  string               `json:"rejection_reason,omitempty"`
	Description      string               `json:"description"`
	NetPerYear       float64              `json:"net_per_year"`
	Target           string               `json:"target"`
}

func (a *api) scenarioCheck(c store.Case, s store.Scenario) *scenarioPlanCheck {
	if c.Plan == nil || c.Product == nil || s.Product == nil {
		return nil
	}
	p := planner.MatchMove(c.Plan, *c.Product, *s.Product)
	if p == nil {
		return nil
	}
	out := &scenarioPlanCheck{Shown: p.Shown, Bar: a.d.PlanConfidence, RejectionKind: p.RejectionKind, RejectionReason: p.RejectionReason,
		Description: a.planDescription(c, *p, c.Plan.Family), NetPerYear: p.NetYear, Target: c.Plan.Target}
	if p.Verdict != nil {
		v := p.Verdict.Confidence
		out.Confidence = &v
		out.SupportingRuling = p.Verdict.SupportingRuling
	}
	return out
}

// factsSource says what the case's own prediction read: "parts_list" when its
// facts are the structured product rendered with the family template (as
// scenarios and plan moves are), "pasted" when they are text the user gave,
// such as CBP's own facts. Empty without a product or a template. The two can
// differ for the same setup, so the screens say which one they show.
func (a *api) factsSource(c store.Case) string {
	if c.Product == nil {
		return ""
	}
	tm, ok := a.d.Templates[c.Family]
	if !ok {
		return ""
	}
	facts := c.OriginalFacts
	if strings.TrimSpace(facts) == "" {
		facts = c.Facts
	}
	if strings.TrimSpace(planner.Render(*c.Product, tm)) == strings.TrimSpace(facts) {
		return "parts_list"
	}
	return "pasted"
}
