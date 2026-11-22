package rcep

import (
	"errors"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
)

// Material statuses. "originating" means the supplier's declaration or
// proof of origin says the material is RCEP-originating in its own right.
const (
	StatusOriginating    = "originating"
	StatusNonOriginating = "non_originating"
	StatusUnknown        = "unknown"
)

// Outcomes.
const (
	Yes          = "yes"
	No           = "no"
	Undetermined = "undetermined"
)

// Regime labels every result so it cannot be read as marking origin.
const Regime = "rcep_preferential"

// Note goes with every result.
const Note = "RCEP preferential origin from the facts given. Not marking origin, not a ruling, not a certificate of origin. The rules have not been reviewed by a customs professional."

// MinimalOperations are the Art. 3.6 operations. A good whose only
// processing on non-originating materials is one or more of these is not
// originating, whatever its rule says.
var MinimalOperations = map[string]string{
	"preserving":                "preserving for transport or storage",
	"packaging":                 "packaging or presenting for transport or sale",
	"simple_processes":          "sifting, sorting, cutting, grinding, bending or similar simple processes",
	"marking":                   "affixing marks, labels or logos",
	"dilution":                  "mere dilution",
	"disassembly":               "disassembly into parts",
	"slaughtering":              "slaughtering",
	"simple_painting_polishing": "simple painting or polishing",
	"simple_peeling":            "simple peeling, stoning or shelling",
	"simple_mixing":             "simple mixing",
}

// Material is one input of the bill of materials.
type Material struct {
	Name string `json:"name"`
	// HS is the material's own HS code, as many digits as known.
	HS string `json:"hs"`
	// Value is the Art. 3.5.3 value: CIF at import for imported materials,
	// the earliest price paid in the Party otherwise. Same currency as FOB.
	Value *float64 `json:"value,omitempty"`
	// Weight is in the same unit as the good's weight (textiles de minimis).
	Weight *float64 `json:"weight,omitempty"`
	// Country is where the material originates (name or ISO code).
	Country string `json:"country,omitempty"`
	// Status is originating, non_originating or unknown (default unknown).
	Status string `json:"status,omitempty"`
}

// Costs are the build-up (direct) formula's other terms, Art. 3.5.1(b).
type Costs struct {
	Labour   float64 `json:"labour"`
	Overhead float64 `json:"overhead"`
	Profit   float64 `json:"profit"`
	Other    float64 `json:"other"`
}

// Input describes the good and how it is made.
type Input struct {
	HS         string     `json:"hs"`
	ProducedIn string     `json:"produced_in"`
	FOB        *float64   `json:"fob,omitempty"`
	Weight     *float64   `json:"weight,omitempty"`
	Materials  []Material `json:"materials"`
	Costs      *Costs     `json:"costs,omitempty"`
	// WhollyObtained is the producer's statement that the good is wholly
	// obtained or produced in the Party (Art. 3.3).
	WhollyObtained bool `json:"wholly_obtained,omitempty"`
	// Operations is what is done to the non-originating materials, using
	// the MinimalOperations keys or any other word for real production.
	Operations []string `json:"operations,omitempty"`
	// ChemicalReaction says whether a chemical reaction (Annex 3A "CR")
	// took place in a Party. Unset means not stated.
	ChemicalReaction *bool `json:"chemical_reaction,omitempty"`
	// Variant picks the branch of a rule that depends on the product.
	Variant string `json:"variant,omitempty"`
	// ActiveIngredientOriginatingPct is the share by weight of the active
	// ingredients that is originating (heading 38.08 only).
	ActiveIngredientOriginatingPct *float64 `json:"active_ingredient_originating_pct,omitempty"`
}

// RuleRef identifies the Annex 3A row used.
type RuleRef struct {
	Code        string    `json:"code"`
	Level       string    `json:"level"`
	Description string    `json:"description"`
	Text        string    `json:"text"`
	Version     string    `json:"version"`
	Variants    []Variant `json:"variants,omitempty"`
}

// RVC is the regional value content worked both ways, in per cent.
type RVC struct {
	Threshold int      `json:"threshold"`
	BuildDown *float64 `json:"build_down,omitempty"`
	BuildUp   *float64 `json:"build_up,omitempty"`
	VNM       float64  `json:"vnm"`
	VOM       float64  `json:"vom"`
}

// DeMinimis reports an Art. 3.7 test that was needed.
type DeMinimis struct {
	Used        bool     `json:"used"`
	ValueShare  *float64 `json:"value_share,omitempty"`
	WeightShare *float64 `json:"weight_share,omitempty"`
}

// Issue is a material that stops (or may stop) the good from qualifying.
type Issue struct {
	Index  int    `json:"index"`
	Name   string `json:"name"`
	HS     string `json:"hs"`
	Reason string `json:"reason"`
}

// AltResult is the outcome of one alternative of the rule.
type AltResult struct {
	Rule    string   `json:"rule"`
	Outcome string   `json:"outcome"`
	Reasons []string `json:"reasons"`
}

// Result is the answer.
type Result struct {
	Outcome string `json:"outcome"`
	// Basis is the Art. 3.2 limb: WO (wholly obtained), PE (only
	// originating materials) or PSR (the Annex 3A rule). Empty when no.
	Basis string `json:"basis,omitempty"`
	// Applied is the alternative that decided a yes ("CTH", "RVC40").
	Applied      string      `json:"applied,omitempty"`
	Rule         *RuleRef    `json:"rule,omitempty"`
	RVC          *RVC        `json:"rvc,omitempty"`
	DeMinimis    *DeMinimis  `json:"de_minimis,omitempty"`
	Accumulation bool        `json:"accumulation"`
	Failing      []Issue     `json:"failing"`
	Alternatives []AltResult `json:"alternatives"`
	Reasons      []string    `json:"reasons"`
	Regime       string      `json:"regime"`
	Note         string      `json:"note"`
}

// ErrInput marks a request that is malformed rather than incomplete.
var ErrInput = errors.New("rcep: bad input")

func bad(format string, a ...any) error {
	return fmt.Errorf("%w: %s", ErrInput, fmt.Sprintf(format, a...))
}

// Validate rejects malformed input. Missing facts are not errors: they make
// the answer undetermined.
func Validate(in Input) error {
	if len(digits(in.HS)) < 4 || len(digits(in.HS)) > 10 {
		return bad("hs must have 4 to 10 digits")
	}
	if in.FOB != nil && (*in.FOB <= 0 || math.IsNaN(*in.FOB) || math.IsInf(*in.FOB, 0)) {
		return bad("fob must be positive")
	}
	if in.Weight != nil && (*in.Weight <= 0 || math.IsNaN(*in.Weight)) {
		return bad("weight must be positive")
	}
	if in.Costs != nil {
		for _, v := range []float64{in.Costs.Labour, in.Costs.Overhead, in.Costs.Profit, in.Costs.Other} {
			if v < 0 || math.IsNaN(v) || math.IsInf(v, 0) {
				return bad("costs must be zero or more")
			}
		}
	}
	if p := in.ActiveIngredientOriginatingPct; p != nil && (*p < 0 || *p > 100) {
		return bad("active_ingredient_originating_pct must be 0 to 100")
	}
	for i, m := range in.Materials {
		switch m.Status {
		case "", StatusOriginating, StatusNonOriginating, StatusUnknown:
		default:
			return bad("materials[%d].status must be originating, non_originating or unknown", i)
		}
		if d := digits(m.HS); len(d) > 10 || (len(d) > 0 && len(d) < 2) {
			return bad("materials[%d].hs must have 2 to 10 digits", i)
		}
		if m.Value != nil && (*m.Value < 0 || math.IsNaN(*m.Value) || math.IsInf(*m.Value, 0)) {
			return bad("materials[%d].value must be zero or more", i)
		}
		if m.Weight != nil && (*m.Weight < 0 || math.IsNaN(*m.Weight)) {
			return bad("materials[%d].weight must be zero or more", i)
		}
	}
	return nil
}

// mat is a material after its status is settled.
type mat struct {
	Material
	idx         int
	originating bool
}

// Check decides RCEP origin with the default table.
func Check(in Input) (Result, error) { return Default().Check(in) }

// Check decides RCEP origin for in.
func (t *Table) Check(in Input) (Result, error) {
	if err := Validate(in); err != nil {
		return Result{}, err
	}
	res := Result{Regime: Regime, Note: Note, Failing: []Issue{}, Alternatives: []AltResult{}}
	say := func(format string, a ...any) { res.Reasons = append(res.Reasons, fmt.Sprintf(format, a...)) }
	good := digits(in.HS)

	// Art. 3.2: the good must be produced in a Party.
	party := Party(in.ProducedIn)
	switch {
	case strings.TrimSpace(in.ProducedIn) == "":
		res.Outcome = Undetermined
		say("Say where the good is made.")
		return res, nil
	case party == "":
		res.Outcome = No
		say("%s is not an RCEP Party. Only goods made in a Party can qualify (Art. 3.2).", strings.TrimSpace(in.ProducedIn))
		return res, nil
	}

	if r, ok, _ := t.Lookup(good); ok {
		res.Rule = &RuleRef{Code: r.Code, Level: r.Level, Description: r.Description, Text: r.Text, Version: t.Version, Variants: r.Variants}
	}

	// Art. 3.2(a): wholly obtained.
	if in.WhollyObtained {
		res.Outcome, res.Basis, res.Applied = Yes, "WO", "WO"
		say("You state the good is wholly obtained in %s (Art. 3.3), so it qualifies (Art. 3.2(a)).", Parties[party])
		return res, nil
	}

	// Settle each material's status (Art. 3.4 cumulation, Art. 3.5.4).
	mats := make([]mat, len(in.Materials))
	nonOrig := 0
	for i, m := range in.Materials {
		mm := mat{Material: m, idx: i}
		c := Party(m.Country)
		switch m.Status {
		case StatusOriginating:
			if strings.TrimSpace(m.Country) != "" && c == "" {
				say("%s is declared originating but comes from %s, which is not an RCEP Party; it counts as non-originating (Art. 3.4).", label(m, i), strings.TrimSpace(m.Country))
			} else {
				mm.originating = true
				if c != "" && c != party {
					res.Accumulation = true
				}
			}
		case StatusUnknown, "":
			// counted below
		}
		if !mm.originating {
			nonOrig++
		}
		mats[i] = mm
	}
	if unk := countStatus(in.Materials, StatusUnknown, ""); unk > 0 {
		say("%s of undetermined origin %s as non-originating (Art. 3.5.4).", plural(unk, "material", "materials"), isAre(unk, "counts", "count"))
	}
	if res.Accumulation {
		say("Originating materials from other Parties count as originating (Art. 3.4).")
	}

	// Art. 3.2(b): produced only from originating materials.
	if len(mats) > 0 && nonOrig == 0 {
		res.Outcome, res.Basis, res.Applied = Yes, "PE", "PE"
		say("Every material is originating, so the good qualifies (Art. 3.2(b)).")
		return res, nil
	}
	if len(mats) == 0 {
		res.Outcome = Undetermined
		say("List the materials, or state that the good is wholly obtained.")
		return res, nil
	}

	// Art. 3.6: minimal operations confer nothing.
	if len(in.Operations) > 0 {
		minimal := true
		var names []string
		for _, op := range in.Operations {
			d, ok := MinimalOperations[strings.ToLower(strings.TrimSpace(op))]
			if !ok {
				minimal = false
				break
			}
			names = append(names, d)
		}
		if minimal {
			res.Outcome = No
			say("Only minimal operations are done on non-originating materials (%s), which do not confer origin (Art. 3.6).", strings.Join(names, "; "))
			return res, nil
		}
	}

	// Art. 3.2(c): the Annex 3A rule.
	r, ok, need := t.Lookup(good)
	if !ok {
		res.Outcome = Undetermined
		say("No Annex 3A rule applied: %s.", need)
		return res, nil
	}
	say("Rule for %s: %s.", r.Code, r.Text)

	alts := r.Alternatives
	if len(r.Variants) > 0 {
		v, found := pickVariant(r.Variants, in.Variant)
		switch {
		case found:
			alts = v.Alternatives
			say("Using the branch for %s.", v.Label)
		case in.Variant != "":
			return Result{}, bad("variant %q is not one of %s", in.Variant, variantIDs(r.Variants))
		default:
			return t.eachVariant(in, mats, res, r), nil
		}
	}
	t.applyAlternatives(in, good, mats, alts, &res)
	if len(in.Operations) == 0 && res.Outcome == Yes {
		say("This assumes the work done is more than the minimal operations of Art. 3.6.")
	}
	return res, nil
}

// eachVariant decides a rule that depends on the product when the caller
// did not say which branch: if every branch gives the same answer, that is
// the answer; otherwise it is undetermined.
func (t *Table) eachVariant(in Input, mats []mat, base Result, r *Rule) Result {
	good := digits(in.HS)
	var outs []Result
	for _, v := range r.Variants {
		res := base
		res.Reasons = append([]string(nil), base.Reasons...)
		t.applyAlternatives(in, good, mats, v.Alternatives, &res)
		outs = append(outs, res)
	}
	same := true
	for _, o := range outs[1:] {
		if o.Outcome != outs[0].Outcome {
			same = false
		}
	}
	if same {
		res := outs[0]
		res.Reasons = append(res.Reasons, "The rule depends on the product, but every branch gives the same answer.")
		return res
	}
	res := base
	res.Outcome = Undetermined
	var opts []string
	for _, v := range r.Variants {
		opts = append(opts, fmt.Sprintf("%q (%s)", v.ID, v.Label))
	}
	res.Reasons = append(res.Reasons, "The rule depends on the product and the branches disagree. Pick a variant: "+strings.Join(opts, ", ")+".")
	return res
}

func pickVariant(vs []Variant, id string) (Variant, bool) {
	for _, v := range vs {
		if v.ID == id {
			return v, true
		}
	}
	return Variant{}, false
}

func variantIDs(vs []Variant) string {
	var ids []string
	for _, v := range vs {
		ids = append(ids, v.ID)
	}
	return strings.Join(ids, ", ")
}

// applyAlternatives evaluates each alternative and combines them: yes if any
// is yes, no if all are no, else undetermined.
func (t *Table) applyAlternatives(in Input, good string, mats []mat, alts []Alternative, res *Result) {
	outcome := No
	var failing []Issue
	var failingDM *DeMinimis
	for _, a := range alts {
		ar, issues, rvc, dm := evalAlt(in, good, mats, a)
		res.Alternatives = append(res.Alternatives, ar)
		if rvc != nil {
			res.RVC = rvc
		}
		switch ar.Outcome {
		case Yes:
			if outcome != Yes {
				outcome = Yes
				res.Applied = ar.Rule
				res.DeMinimis = dm
			}
		case Undetermined:
			if outcome == No {
				outcome = Undetermined
			}
		}
		if issues != nil && failing == nil {
			failing, failingDM = issues, dm
		}
	}
	if outcome != Yes {
		res.DeMinimis = failingDM
	}
	res.Outcome = outcome
	if outcome == Yes {
		res.Basis = "PSR"
		res.Reasons = append(res.Reasons, fmt.Sprintf("Qualifies under %s.", res.Applied))
		for _, ar := range res.Alternatives {
			if ar.Rule == res.Applied {
				res.Reasons = append(res.Reasons, ar.Reasons...)
			}
		}
		return
	}
	if failing != nil {
		res.Failing = failing
	}
	for _, ar := range res.Alternatives {
		res.Reasons = append(res.Reasons, ar.Rule+": "+strings.Join(ar.Reasons, " "))
	}
}

// evalAlt evaluates one alternative.
func evalAlt(in Input, good string, mats []mat, a Alternative) (AltResult, []Issue, *RVC, *DeMinimis) {
	ar := AltResult{Rule: AltName(a)}
	switch a.Criterion {
	case "WO":
		ar.Outcome = No
		ar.Reasons = []string{"The good uses non-originating materials, so it is not wholly obtained."}
		return ar, nil, nil, nil
	case "CR":
		switch {
		case in.ChemicalReaction == nil:
			ar.Outcome = Undetermined
			ar.Reasons = []string{"Not stated whether a chemical reaction took place in a Party."}
		case *in.ChemicalReaction:
			ar.Outcome = Yes
			ar.Reasons = []string{"You state a chemical reaction took place in a Party."}
		default:
			ar.Outcome = No
			ar.Reasons = []string{"No chemical reaction in a Party."}
		}
		return ar, nil, nil, nil
	case "RVC":
		rvc, out, why := regionalValue(in, mats, a.Threshold)
		ar.Outcome, ar.Reasons = out, why
		return ar, nil, rvc, nil
	case "CC", "CTH", "CTSH":
		out, why, issues, dm := tariffShift(in, good, mats, a)
		if a.Condition == "active_ingredient_50pct" && out != No {
			p := in.ActiveIngredientOriginatingPct
			switch {
			case p == nil:
				out = Undetermined
				why = append(why, "The rule also needs at least 50 % by weight of the active ingredients to be originating; not stated.")
			case *p < 50:
				out = No
				why = append(why, fmt.Sprintf("Only %s %% of the active ingredients by weight are originating; at least 50 %% is needed.", pct(*p)))
			default:
				why = append(why, fmt.Sprintf("%s %% of the active ingredients by weight are originating (50 %% needed).", pct(*p)))
			}
		}
		ar.Outcome, ar.Reasons = out, why
		return ar, issues, nil, dm
	}
	ar.Outcome = Undetermined
	ar.Reasons = []string{"Unknown criterion " + a.Criterion + "."}
	return ar, nil, nil, nil
}

func shiftDigits(criterion string) int {
	switch criterion {
	case "CC":
		return 2
	case "CTH":
		return 4
	}
	return 6
}

var levelDigits = map[string]int{"chapter": 2, "heading": 4, "subheading": 6}
var levelWord = map[int]string{2: "chapter", 4: "heading", 6: "subheading"}

// tariffShift checks a CC, CTH or CTSH alternative and, when materials fail
// it, the Art. 3.7 de minimis.
func tariffShift(in Input, good string, mats []mat, a Alternative) (string, []string, []Issue, *DeMinimis) {
	n := shiftDigits(a.Criterion)
	// #671: Lookup can return a heading-level rule whose CTSH alternative
	// needs six good digits even when the caller gave only four or five.
	// Missing digits make the answer undetermined, never a slice panic.
	if len(good) < n {
		return Undetermined,
			[]string{fmt.Sprintf("%s needs the good's %d-digit %s; give at least %d digits.", a.Criterion, n, levelWord[n], n)},
			nil, nil
	}
	var fail, unknown []Issue
	for _, m := range mats {
		if m.originating {
			continue
		}
		md := digits(m.HS)
		iss := Issue{Index: m.idx, Name: m.Name, HS: m.HS}
		if len(md) < n {
			iss.Reason = fmt.Sprintf("needs a %d-digit HS code to test this rule", n)
			unknown = append(unknown, iss)
			continue
		}
		if md[:n] == good[:n] {
			iss.Reason = fmt.Sprintf("same %s as the good (%s), so no change", levelWord[n], dottedN(good, n))
			fail = append(fail, iss)
			continue
		}
		excluded, short := false, false
		for _, x := range a.Except {
			k := levelDigits[x.Level]
			if len(md) < k {
				short = true
				continue
			}
			if code := md[:k]; code >= digits(x.From) && code <= digits(x.To) {
				excluded = true
				iss.Reason = fmt.Sprintf("comes from %s %s, which the rule excludes", x.Level, dottedN(md, k))
				break
			}
		}
		switch {
		case excluded:
			fail = append(fail, iss)
		case short:
			iss.Reason = "needs more HS digits to test the rule's exclusions"
			unknown = append(unknown, iss)
		}
	}
	word := map[int]string{2: "chapter", 4: "heading", 6: "subheading"}[n]
	if len(fail)+len(unknown) == 0 {
		return Yes, []string{fmt.Sprintf("Every non-originating material changes %s.", word)}, nil, nil
	}
	issues := append(append([]Issue{}, fail...), unknown...)
	sort.Slice(issues, func(i, j int) bool { return issues[i].Index < issues[j].Index })
	var why []string
	for _, f := range issues {
		code := ""
		if strings.TrimSpace(f.HS) != "" {
			code = " (" + f.HS + ")"
		}
		why = append(why, fmt.Sprintf("%s%s %s.", label(mats[f.Index].Material, f.Index), code, f.Reason))
	}

	// Art. 3.7 de minimis, counting materials we cannot test as failing.
	dm := &DeMinimis{}
	valueKnown := in.FOB != nil
	var v float64
	for _, f := range issues {
		m := mats[f.Index]
		if m.Value == nil {
			valueKnown = false
			break
		}
		v += *m.Value
	}
	if valueKnown {
		s := v / *in.FOB * 100
		dm.ValueShare = &s
		if s <= 10+1e-9 {
			dm.Used = true
			return Yes, append(why, fmt.Sprintf("De minimis: those materials are %s %% of the FOB value, within 10 %% (Art. 3.7).", pct(s))), issues, dm
		}
		why = append(why, fmt.Sprintf("Those materials are %s %% of the FOB value, over the 10 %% de minimis (Art. 3.7).", pct(s)))
	}
	if ch, _ := strconv.Atoi(good[:2]); ch >= 50 && ch <= 63 {
		weightKnown := in.Weight != nil
		var w float64
		for _, f := range issues {
			m := mats[f.Index]
			if m.Weight == nil {
				weightKnown = false
				break
			}
			w += *m.Weight
		}
		if weightKnown {
			s := w / *in.Weight * 100
			dm.WeightShare = &s
			if s <= 10+1e-9 {
				dm.Used = true
				return Yes, append(why, fmt.Sprintf("De minimis: those materials are %s %% of the good's weight, within 10 %% (Art. 3.7(b)).", pct(s))), issues, dm
			}
			why = append(why, fmt.Sprintf("Those materials are %s %% of the good's weight, over 10 %%.", pct(s)))
		} else if !valueKnown {
			why = append(why, "Give values and FOB, or weights, to test de minimis (Art. 3.7).")
			return Undetermined, why, issues, dm
		}
	} else if !valueKnown {
		why = append(why, "Give their values and the FOB value to test de minimis (Art. 3.7).")
		return Undetermined, why, issues, dm
	}
	if len(unknown) > 0 {
		return Undetermined, why, issues, dm
	}
	return No, why, issues, dm
}

// regionalValue works out RVC both ways (Art. 3.5).
func regionalValue(in Input, mats []mat, threshold int) (*RVC, string, []string) {
	rvc := &RVC{Threshold: threshold}
	if in.FOB == nil {
		return rvc, Undetermined, []string{"Give the FOB value to work out RVC."}
	}
	fob := *in.FOB
	vnmKnown, vomKnown := true, true
	for _, m := range mats {
		switch {
		case m.Value == nil && m.originating:
			vomKnown = false
		case m.Value == nil:
			vnmKnown = false
		case m.originating:
			rvc.VOM += *m.Value
		default:
			rvc.VNM += *m.Value
		}
	}
	var why []string
	t := float64(threshold)
	pass, bdFail, buFail := false, false, false
	if vnmKnown {
		bd := (fob - rvc.VNM) / fob * 100
		rvc.BuildDown = &bd
		why = append(why, fmt.Sprintf("Build-down: (FOB %s - non-originating %s) / FOB = %s %%.", money(fob), money(rvc.VNM), pct(bd)))
		pass = pass || bd >= t-1e-9
		bdFail = bd < t-1e-9
	} else {
		why = append(why, "Build-down needs a value for every non-originating material.")
	}
	if c := in.Costs; c != nil {
		bu := (rvc.VOM + c.Labour + c.Overhead + c.Profit + c.Other) / fob * 100
		rvc.BuildUp = &bu
		why = append(why, fmt.Sprintf("Build-up: (originating %s + labour %s + overhead %s + profit %s + other %s) / FOB = %s %%.",
			money(rvc.VOM), money(c.Labour), money(c.Overhead), money(c.Profit), money(c.Other), pct(bu)))
		pass = pass || bu >= t-1e-9
		buFail = bu < t-1e-9 && vomKnown
	}
	switch {
	case pass:
		why = append(why, fmt.Sprintf("At least %d %% needed: met.", threshold))
		return rvc, Yes, why
	case bdFail && (in.Costs == nil || buFail):
		why = append(why, fmt.Sprintf("At least %d %% needed: not met.", threshold))
		return rvc, No, why
	}
	why = append(why, fmt.Sprintf("At least %d %% needed: not enough data to decide.", threshold))
	return rvc, Undetermined, why
}

// AltName writes an alternative the way the Annex does.
func AltName(a Alternative) string {
	switch a.Criterion {
	case "RVC":
		return fmt.Sprintf("RVC%d", a.Threshold)
	case "CC", "CTH", "CTSH":
		s := a.Criterion
		if len(a.Except) > 0 {
			var parts []string
			for _, x := range a.Except {
				if x.From == x.To {
					parts = append(parts, x.From)
				} else {
					parts = append(parts, x.From+" to "+x.To)
				}
			}
			s += " except from " + a.Except[0].Level + " " + strings.Join(parts, ", ")
		}
		if a.Condition == "active_ingredient_50pct" {
			s += " (50 % originating active ingredient)"
		}
		return s
	}
	return a.Criterion
}

func dottedN(d string, n int) string {
	switch n {
	case 2:
		return d[:2]
	case 4:
		return d[:2] + "." + d[2:4]
	}
	return d[:4] + "." + d[4:6]
}

func label(m Material, i int) string {
	if strings.TrimSpace(m.Name) != "" {
		return m.Name
	}
	return fmt.Sprintf("Material %d", i+1)
}

func countStatus(ms []Material, statuses ...string) int {
	n := 0
	for _, m := range ms {
		for _, s := range statuses {
			if m.Status == s {
				n++
				break
			}
		}
	}
	return n
}

func plural(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return fmt.Sprintf("%d %s", n, many)
}

func isAre(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

func pct(v float64) string { return strconv.FormatFloat(math.Round(v*10)/10, 'f', 1, 64) }

func money(v float64) string { return strconv.FormatFloat(v, 'f', -1, 64) }
