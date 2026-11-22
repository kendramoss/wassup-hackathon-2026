// Package planrule runs the 19 CFR 102.20 rule check (internal/tariffshift)
// on each move of a plan search: the product as it would be after the move,
// finished where its final-assembly group would be (D441). It never changes
// a plan's verdict, tier or whether it is shown; when the rule and the
// precedent check disagree it flags the move for a person's look. On a Part
// 102 case it also orders the shown plans within each tier (D500): moves the
// rule says shift first, then precedent only, then moves the rule gives to
// another country. On a substantial-transformation case it is display only.
package planrule

import (
	"fmt"
	"sort"
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
	if applies == AppliesPart102 && !out.Review {
		engineReview(out, pl, target)
	}
	return out
}

// engineReview flags, on a Part 102 case, where the rule decides and the
// engine's predicted origin says otherwise and no precedent check covers
// it (D590, safety change S): the current setup when the rule gives another
// country than the engine, and a move the rule says reaches the target
// while the engine predicts another country, which the search therefore
// never sent to the precedent check. It changes no answer, ranking or what
// is shown. Moves the engine sends to the target are covered by the
// precedent check's own note above.
func engineReview(out *domain.PlanRuleCheck, pl domain.Plan, target string) {
	pred := domain.NormalizeCountry(pl.Prediction.Origin)
	if pred == "" || (out.Status != Shifts && out.Status != NoShift && out.Status != OtherCountry) {
		return
	}
	if len(pl.Move) == 0 {
		if !sameCountry(pred, out.Origin) {
			out.Review = true
			out.ReviewNote = fmt.Sprintf("The engine predicts %s for the current setup, but the 102.20 rule gives %s: worth a broker's look.", pred, out.Origin)
		}
		return
	}
	if out.Status == Shifts && !sameCountry(pred, target) {
		out.Review = true
		out.ReviewNote = fmt.Sprintf("The 102.20 rule says this move reaches %s, but the engine predicts %s, so the move was not checked against precedent: worth a broker's look.", out.Origin, pred)
	}
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

// shortNeeds words tariffshift's needs for a chip.
var shortNeeds = map[string]string{
	"a person to read an alternative written by goods description":     "a rule sentence names goods in words",
	"whether the change is pursuant to GRI 2(a)":                       "needs the GRI 2(a) fact",
	"whether the work is a simple assembly":                            "needs whether it is a simple assembly",
	"whether the goods match a description in the rule":                "needs a goods description match",
	"whether the tubes are bent to shape":                              "needs whether tubes are bent",
	"a longer HTS code":                                                "needs a longer code",
	"the material's own HTS code (only a code it is outside is known)": "needs a part's own code",
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
				if w, ok := shortNeeds[m.Needs[0]]; ok {
					return w
				}
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
// The current setup (baseline) gets one too. On a Part 102 case the shown
// plans are then put in the D500 order (Rank).
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
	if part102 {
		res = Rank(res)
	}
	return res
}

// RankingRule names the D500 order of a Part 102 case's shown plans: tier
// (D341), then the move's rule check, then the planner's order.
const RankingRule = "tier_then_rule_then_net"

// Rule classes of a shown plan for the D500 order, best first.
const (
	ClassShifts       = 0 // precedent and the 102.20 rule both support the move
	ClassPrecedent    = 1 // precedent only: the rule cannot decide (or no check)
	ClassRuleDisagree = 2 // precedent supports, the rule gives another country
)

// Class is a plan's D500 rule class. A plan without a check counts as
// precedent only.
func Class(pl domain.Plan) int {
	if pl.RuleCheck == nil {
		return ClassPrecedent
	}
	switch pl.RuleCheck.Status {
	case Shifts:
		return ClassShifts
	case NoShift, OtherCountry:
		return ClassRuleDisagree
	}
	return ClassPrecedent
}

// Rank is res with its shown plans in the D500 order (D441's checks must be
// on them): tier, then rule class, then the order they came in, which is the
// planner's (net, or priced then net in operations mode). The smallest
// supported move is worked out again on the new order. A search stored
// before D341 (no ranking) is left as it is. Verdicts, tiers,
// what is shown and the rejected moves are not touched. res's own slice is
// not written; the caller decides that the case is Part 102.
func Rank(res planner.Result) planner.Result {
	// A search stored before D341 has no tiers and keeps its net order.
	if len(res.Plans) == 0 || res.Ranking == nil {
		return res
	}
	plans := append([]domain.Plan(nil), res.Plans...)
	sort.SliceStable(plans, func(i, j int) bool {
		a, b := plans[i], plans[j]
		if a.Tier != b.Tier {
			return a.Tier == domain.TierClearMargin
		}
		return Class(a) < Class(b)
	})
	for i := range plans {
		plans[i].SmallestSupported = false
	}
	if i := planner.SmallestSupported(plans); i >= 0 {
		plans[i].SmallestSupported = true
	}
	res.Plans = plans
	if res.Ranking != nil {
		r := *res.Ranking
		r.Rule = RankingRule
		res.Ranking = &r
	}
	return res
}
