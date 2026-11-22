// Package planrule runs the 19 CFR 102.20 rule check (internal/tariffshift)
// on each move of a plan search: the product as it would be after the move,
// finished where its final-assembly group would be (D441). It is display
// only. It never changes a plan's verdict, whether it is shown, or its rank;
// when the rule and the precedent check disagree it flags the move for a
// person's look.
package planrule

import (
	"fmt"
	"strings"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/planner"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/tariffshift"
)

// Where the check applies (the same words as the case's rule check, D363).
const (
	AppliesPart102       = "part102"
	AppliesInformational = "informational"
)

// Statuses of a move's check.
const (
	Shifts       = "shifts"
	NoShift      = "no_shift"
	OtherCountry = "other_country"
	Undetermined = "undetermined"
)

// Why classes of an undetermined check.
const (
	WhyOutOfScope  = "out_of_scope" // the good's chapter is not encoded
	WhyGoodCode    = "good_code"    // the good's code is missing or picks no one rule
	WhyNoCountry   = "no_country"   // where the good is finished is not known
	WhyMissingCode = "missing_code" // a foreign part has no code
	WhyNeedsFact   = "needs_fact"   // a fact a code cannot give
	WhyDeMinimis   = "de_minimis"   // the value of the parts that do not shift
	WhyEssential   = "essential"    // 102.11(b) must be judged between countries
	WhyChapterNote = "chapter_note" // a chapter note gives its own test
	WhyNoOrigin    = "no_origin"    // a part that fails has no country
	WhyNoMaterials = "no_materials" // no parts listed
	WhyOther       = "other"
)

// Eligible says whether a plan search on this product carries the check. A
// Part 102 case always does (an undetermined check says which codes to
// add); an informational one only when the good and every part have a code,
// so a substantial-transformation case without codes shows nothing.
func Eligible(p *domain.Product, part102 bool) bool {
	if p == nil {
		return false
	}
	if part102 {
		return true
	}
	if strings.TrimSpace(p.HTS) == "" || len(p.Components) == 0 {
		return false
	}
	for _, c := range p.Components {
		if strings.TrimSpace(c.HTS) == "" {
			return false
		}
	}
	return true
}

// ProductionCountry is where the good is produced: the plant of the
// final-assembly group, else the plant of the last operation, else the only
// plant. "" when none of those is known.
func ProductionCountry(p domain.Product) (string, string) {
	for i := len(p.Operations) - 1; i >= 0; i-- {
		o := p.Operations[i]
		if o.Group == "final_assembly" {
			if pl, ok := p.PlantByID(o.Plant); ok && pl.Country != "" {
				return pl.Country, "final assembly plant"
			}
		}
	}
	if n := len(p.Operations); n > 0 {
		if pl, ok := p.PlantByID(p.Operations[n-1].Plant); ok && pl.Country != "" {
			return pl.Country, "plant of the last operation"
		}
	}
	if len(p.Plants) == 1 && p.Plants[0].Country != "" {
		return p.Plants[0].Country, "the only plant"
	}
	return "", "not known: add a plant and its operations"
}

// Input is the rule check's question for the product after move m.
func Input(p domain.Product, m domain.Move) tariffshift.Input {
	v := planner.Apply(p, m)
	country, _ := ProductionCountry(v)
	in := tariffshift.Input{GoodHTS: v.HTS, Country: country}
	for _, c := range v.Components {
		in.Materials = append(in.Materials, tariffshift.Material{Name: c.Name, HTS: c.HTS, Origin: c.MadeIn})
	}
	return in
}

// Check is the rule check of one plan of a search for target. applies is
// AppliesPart102 or AppliesInformational.
func Check(rules *tariffshift.Rules, p domain.Product, pl domain.Plan, target, applies string) *domain.PlanRuleCheck {
	in := Input(p, pl.Move)
	res := rules.Evaluate(in)
	out := &domain.PlanRuleCheck{Applies: applies, Country: domain.NormalizeCountry(in.Country), Origin: res.Origin, Basis: res.Basis, Reason: res.Reason}
	if res.Rule != nil {
		out.Rule = res.Rule.ID
	}
	out.Status, out.Why, out.Failing = Classify(res, target)
	out.Label = label(out, res, len(pl.Move) == 0)
	if pl.Verdict != nil && (out.Status == Shifts || out.Status == NoShift || out.Status == OtherCountry) {
		supported := pl.Verdict.PrecedentSupported
		agree := supported == (out.Status == Shifts)
		out.AgreesWithPrecedent = &agree
		switch {
		case supported && !agree:
			out.Review = true
			out.ReviewNote = fmt.Sprintf("An earlier ruling supports this move, but the 102.20 rule gives %s: worth a broker's look.", out.Origin)
		case !supported && !agree && applies == AppliesPart102:
			out.Review = true
			out.ReviewNote = fmt.Sprintf("The 102.20 rule gives %s, but no earlier ruling supports this move: worth a broker's look.", out.Origin)
		}
	}
	return out
}

// Classify maps a rule-check result to a move status for target, with the
// undetermined reason class and the foreign parts that do not shift.
func Classify(res tariffshift.Result, target string) (status, why string, failing []string) {
	for _, m := range res.Materials {
		if m.Foreign && m.Status == tariffshift.MatNoShift {
			failing = append(failing, m.Name)
		}
	}
	switch res.Status {
	case tariffshift.Decided:
		if sameCountry(res.Origin, target) {
			return Shifts, "", failing
		}
		if len(failing) > 0 {
			return NoShift, "", failing
		}
		return OtherCountry, "", nil
	case tariffshift.OutOfScope:
		return Undetermined, WhyOutOfScope, failing
	}
	foreign := func(st string) bool {
		for _, m := range res.Materials {
			if m.Foreign && m.Status == st {
				return true
			}
		}
		return false
	}
	r := res.Reason
	switch {
	case res.Rule == nil && strings.Contains(r, "country where the good is produced"):
		why = WhyNoCountry
	case res.Rule == nil:
		why = WhyGoodCode
	case len(res.Materials) == 0:
		why = WhyNoMaterials
	case foreign(tariffshift.MatMissingHTS):
		why = WhyMissingCode
	case foreign(tariffshift.MatNeedsFact):
		why = WhyNeedsFact
	case strings.Contains(r, "their origin is not given"):
		why = WhyNoOrigin
	case strings.Contains(r, "their value is not given"):
		why = WhyDeMinimis
	case strings.Contains(r, "chapter note"):
		why = WhyChapterNote
	case strings.Contains(r, "essential character"):
		why = WhyEssential
	default:
		why = WhyOther
	}
	return Undetermined, why, failing
}

func sameCountry(a, b string) bool {
	a, b = domain.NormalizeCountry(a), domain.NormalizeCountry(b)
	return a != "" && a == b
}

// label is the status in a few words. baseline: the current setup, which
// has no target to reach.
func label(c *domain.PlanRuleCheck, res tariffshift.Result, baseline bool) string {
	ruleHTS := ""
	if res.Rule != nil {
		ruleHTS = res.Rule.HTS
	}
	switch c.Status {
	case Shifts, NoShift, OtherCountry:
		if baseline {
			return "102.20 gives " + c.Origin
		}
	}
	switch c.Status {
	case Shifts:
		switch {
		case strings.Contains(c.Basis, "102.11(a)(2)"):
			return fmt.Sprintf("Every part from %s (102.11(a)(2))", c.Origin)
		case strings.Contains(c.Basis, "102.13"):
			return fmt.Sprintf("Shifts under 102.20 (%s), with de minimis", ruleHTS)
		case strings.Contains(c.Basis, "102.11(b)"):
			return fmt.Sprintf("%s by essential character (102.11(b))", c.Origin)
		}
		return fmt.Sprintf("Shifts under 102.20 (%s)", ruleHTS)
	case NoShift:
		return fmt.Sprintf("Does not shift under 102.20 (%s %s)", shortList(c.Failing), fails(len(c.Failing)))
	case OtherCountry:
		return fmt.Sprintf("102.20 gives %s: finished there", c.Origin)
	}
	if c.Why == WhyDeMinimis && len(c.Failing) > 0 {
		// The parts fail the shift; only their value (102.13) could still
		// let the good reach the country where it is finished.
		return fmt.Sprintf("Does not shift under 102.20 (%s %s) unless de minimis", shortList(c.Failing), fails(len(c.Failing)))
	}
	return "102.20 undetermined (" + whyWords(c, res) + ")"
}

func fails(n int) string {
	if n == 1 {
		return "fails"
	}
	return "fail"
}

// shortList names up to two items and counts the rest.
func shortList(xs []string) string {
	switch {
	case len(xs) <= 2:
		return strings.Join(xs, " and ")
	default:
		return fmt.Sprintf("%s, %s and %d more", xs[0], xs[1], len(xs)-2)
	}
}

func whyWords(c *domain.PlanRuleCheck, res tariffshift.Result) string {
	switch c.Why {
	case WhyOutOfScope:
		if f := strings.Fields(res.Reason); len(f) >= 2 && f[0] == "chapter" {
			return "chapter " + f[1] + " not encoded"
		}
		return "the good's chapter is not encoded"
	case WhyGoodCode:
		return "no rule for the good's code"
	case WhyNoCountry:
		return "where it is finished is not known"
	case WhyNoMaterials:
		return "no parts listed"
	case WhyMissingCode:
		n := 0
		for _, m := range res.Materials {
			if m.Foreign && m.Status == tariffshift.MatMissingHTS {
				n++
			}
		}
		if n == 1 {
			return "no code for 1 part"
		}
		return fmt.Sprintf("no code for %d parts", n)
	case WhyNeedsFact:
		for _, m := range res.Materials {
			if m.Foreign && m.Status == tariffshift.MatNeedsFact && len(m.Needs) > 0 {
				return "needs " + m.Needs[0]
			}
		}
		return "needs a fact the codes cannot give"
	case WhyDeMinimis:
		return "value of " + shortList(c.Failing) + " not given"
	case WhyEssential:
		return "essential character to judge"
	case WhyChapterNote:
		return "a chapter note applies"
	case WhyNoOrigin:
		return "a part's country is not given"
	}
	return "see the reason"
}

// Annotate returns res with each plan's rule check filled in, for a product
// that is Eligible. The plans are copies: res's own slices are not written.
// The current setup (baseline) gets one too.
func Annotate(rules *tariffshift.Rules, p domain.Product, res planner.Result, part102 bool) planner.Result {
	applies := AppliesInformational
	if part102 {
		applies = AppliesPart102
	}
	one := func(pl domain.Plan) domain.Plan {
		pl.RuleCheck = Check(rules, p, pl, res.Target, applies)
		return pl
	}
	all := func(ps []domain.Plan) []domain.Plan {
		if ps == nil {
			return nil
		}
		out := make([]domain.Plan, len(ps))
		for i, pl := range ps {
			out[i] = one(pl)
		}
		return out
	}
	res.Baseline = one(res.Baseline)
	res.Plans, res.Rejected = all(res.Plans), all(res.Rejected)
	return res
}
