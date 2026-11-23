package usmca

import (
	"fmt"
	"math/big"
)

var bigHundred = big.NewRat(100, 1)

// RuleStatus is the selected-rule outcome. It is not a statement about
// originating status, certification or preferential entitlement.
type RuleStatus string

// Rule statuses.
const (
	RuleMet              RuleStatus = "met"
	RuleNotMet           RuleStatus = "not_met"
	RuleNeedsInformation RuleStatus = "needs_information"
	RuleNotSupported     RuleStatus = "not_supported"
)

// Outcome is the JSON form of a Tri.
type Outcome string

// Outcomes.
const (
	OutcomeSatisfied    Outcome = "satisfied"
	OutcomeNotSatisfied Outcome = "not_satisfied"
	OutcomeUnknown      Outcome = "unknown"
)

func outcome(t Tri) Outcome {
	switch t {
	case True:
		return OutcomeSatisfied
	case False:
		return OutcomeNotSatisfied
	}
	return OutcomeUnknown
}

// MissingFact names an input that would resolve an unknown.
type MissingFact struct {
	Code    string `json:"code"`
	Field   string `json:"field"`
	Message string `json:"message"`
}

// MaterialFinding reports a material that did not satisfy a condition.
type MaterialFinding struct {
	Index  int     `json:"index"`
	ID     string  `json:"id,omitempty"`
	Result Outcome `json:"result"`
	Note   string  `json:"note"`
}

// ConditionResult is the evaluation of one condition.
type ConditionResult struct {
	Kind         string            `json:"kind"`
	Result       Outcome           `json:"result"`
	Detail       string            `json:"detail"`
	Materials    []MaterialFinding `json:"materials,omitempty"`
	MissingFacts []MissingFact     `json:"missing_facts,omitempty"`
}

// AlternativeResult is the AND over an alternative's conditions.
type AlternativeResult struct {
	ID         string            `json:"id"`
	Label      string            `json:"label"`
	Note       string            `json:"note,omitempty"`
	Result     Outcome           `json:"result"`
	Conditions []ConditionResult `json:"conditions"`
}

// RVCCalc is the separate arithmetic. Exact is the exact percentage as an
// integer or fraction ("200/3"); Display is rounded for humans only.
type RVCCalc struct {
	Method      Method    `json:"method"`
	Status      Status    `json:"status"`
	Exact       string    `json:"exact,omitempty"`
	Display     string    `json:"display,omitempty"`
	DisplayNote string    `json:"display_note,omitempty"`
	Problems    []Problem `json:"problems,omitempty"`
}

// SourceInfo is the rule data version/snapshot the result was computed from.
type SourceInfo struct {
	DataVersion string   `json:"data_version"`
	Snapshot    Snapshot `json:"snapshot"`
	Row         *RowInfo `json:"row,omitempty"`
}

// RowInfo identifies the applied row and its source excerpt.
type RowInfo struct {
	ID          string    `json:"id"`
	Code        string    `json:"code"`
	Description string    `json:"description"`
	Source      RowSource `json:"source"`
}

// Output is the check result.
type Output struct {
	Status             RuleStatus          `json:"status"`
	Destination        string              `json:"destination"`
	Regime             string              `json:"regime"`
	GoodHS             string              `json:"good_hs"`
	RowID              string              `json:"row_id,omitempty"`
	Alternatives       []AlternativeResult `json:"alternatives,omitempty"`
	RVC                *RVCCalc            `json:"rvc,omitempty"`
	MissingFacts       []MissingFact       `json:"missing_facts,omitempty"`
	UnsupportedReasons []string            `json:"unsupported_reasons,omitempty"`
	Source             SourceInfo          `json:"source"`
	Limits             []string            `json:"limits"`
}

// limitsText holds the scope statements attached to every result.
var limitsText = []string{
	"This checks one selected product-specific origin rule only. It is not a certification of origin, a preferential-treatment or tariff-rate determination, or a duty-saving claim.",
	"Originating-status claims, material codes, values and profile confirmations are supplied by the caller and are not verified.",
	"A not_met result concerns the selected rule as modeled; it is not a legal finding that the good fails to originate.",
	"Accumulation, remanufactured goods and automotive rules are not evaluated; they are covered only by the caller's explicit simple_goods_confirmed profile confirmation that none applies.",
	"The parts route (Art. 4.2(d) / GN 11(b)(iv)), de minimis, sets, accessories/packing, fungibles, intermediate materials, cost averaging, outside-party operations and special valuation are not evaluated; each must be confirmed not applicable, and an invoked one returns not_supported.",
	"RVC uses exact decimal arithmetic on the supplied figures, which must share one currency and valuation basis; the displayed percentage is rounded for readability only.",
}

// Limits returns a fresh copy of the scope statements attached to results.
func Limits() []string { return append([]string(nil), limitsText...) }

// Check evaluates in against the embedded rule data.
func Check(in Input) (Output, error) {
	rs, err := DefaultRules()
	if err != nil {
		return Output{}, err
	}
	return rs.Check(in)
}

type factSet struct {
	seen map[string]bool
	list []MissingFact
}

func (f *factSet) add(m MissingFact) {
	key := m.Code + "|" + m.Field
	if f.seen == nil {
		f.seen = map[string]bool{}
	}
	if !f.seen[key] {
		f.seen[key] = true
		f.list = append(f.list, m)
	}
}

// Check validates in and evaluates the matching row. Malformed input returns
// an error satisfying errors.Is(err, ErrInput).
func (rs *RuleSet) Check(in Input) (Output, error) {
	if err := in.Validate(); err != nil {
		return Output{}, err
	}
	out := Output{
		Destination: in.Destination, Regime: in.Regime, GoodHS: in.GoodHS,
		Source: SourceInfo{DataVersion: rs.data.DataVersion, Snapshot: rs.data.Snapshot},
		Limits: Limits(),
	}

	var rvc *Result
	if in.RVCMethod != "" {
		m := Method(in.RVCMethod)
		d := in.TransactionValue
		if m == MethodNetCost {
			d = in.NetCost
		}
		r, err := CalculateRVC(RVCInput{Method: m, Denominator: d, VNM: AggregateVNM{Value: in.VNM}})
		if err != nil {
			return Output{}, inputErr("rvc_method", "%v", err)
		}
		rvc = &r
		out.RVC = rvcCalc(r)
	}

	// Profile gate.
	var unsupported []string
	var profileFacts factSet
	if !in.Profile.SimpleGoods {
		profileFacts.add(MissingFact{"profile_unconfirmed", "profile.simple_goods_confirmed", "Confirm the goods fit the simple-goods profile assumed by this check."})
	}
	for _, p := range provisions {
		switch p.get(in.Profile) {
		case ProvisionApplies:
			unsupported = append(unsupported, p.name+" applies and is not supported by this check")
		case ProvisionNotApplicable:
		default:
			profileFacts.add(MissingFact{"profile_unconfirmed", "profile." + p.name, "Confirm whether " + p.name + " applies; it is not supported by this check."})
		}
	}

	row, shortGood := rs.matchRow(in.GoodHS)
	if row == nil && !shortGood {
		unsupported = append(unsupported, "the good is not covered by the rule data snapshot")
	}
	if len(unsupported) > 0 {
		out.Status = RuleNotSupported
		out.UnsupportedReasons = unsupported
		return out, nil
	}
	if row == nil { // shortGood
		out.Status = RuleNeedsInformation
		out.MissingFacts = []MissingFact{{"insufficient_precision", "good_hs", "Provide a more specific good code (more digits); a covered rule exists below the code given."}}
		return out, nil
	}

	out.RowID = row.ID
	out.Source.Row = &RowInfo{ID: row.ID, Code: row.Code, Description: row.Description, Source: row.Source}
	ev := &evaluator{in: in, rvc: rvc}
	rule := False
	var altFacts factSet
	for _, a := range row.Alternatives {
		ar := AlternativeResult{ID: a.ID, Label: a.Label, Note: a.Note}
		res := True
		for _, c := range a.Conditions {
			cr := ev.condition(c)
			res = res.And(triOf(cr.Result))
			ar.Conditions = append(ar.Conditions, cr)
		}
		ar.Result = outcome(res)
		out.Alternatives = append(out.Alternatives, ar)
		rule = rule.Or(res)
	}
	// Facts from alternatives only matter while the rule is unresolved.
	if rule == Unknown3 {
		for _, ar := range out.Alternatives {
			if ar.Result != OutcomeUnknown {
				continue
			}
			for _, cr := range ar.Conditions {
				for _, f := range cr.MissingFacts {
					altFacts.add(f)
				}
			}
		}
	}
	facts := append(append([]MissingFact(nil), profileFacts.list...), altFacts.list...)
	switch {
	case len(profileFacts.list) > 0:
		out.Status = RuleNeedsInformation
	case rule == True:
		out.Status = RuleMet
	case rule == False:
		out.Status = RuleNotMet
	default:
		out.Status = RuleNeedsInformation
	}
	if out.Status == RuleNeedsInformation {
		out.MissingFacts = facts
	}
	return out, nil
}

func triOf(o Outcome) Tri {
	switch o {
	case OutcomeSatisfied:
		return True
	case OutcomeNotSatisfied:
		return False
	}
	return Unknown3
}

// matchRow picks the most specific row whose code prefixes the good. shortGood
// is true when no row matches but some row lies below the (shorter) good code.
func (rs *RuleSet) matchRow(good string) (row *Row, shortGood bool) {
	for i := range rs.data.Rows {
		r := &rs.data.Rows[i]
		if len(r.Code) <= len(good) && good[:len(r.Code)] == r.Code {
			if row == nil || len(r.Code) > len(row.Code) {
				row = r
			}
		} else if len(r.Code) > len(good) && r.Code[:len(good)] == good {
			shortGood = true
		}
	}
	if row != nil {
		return row, false
	}
	return nil, shortGood
}

func rvcCalc(r Result) *RVCCalc {
	c := &RVCCalc{Method: r.Method, Status: r.Status, Display: r.Display, DisplayNote: r.DisplayNote, Problems: r.Problems}
	if r.Ratio != nil {
		c.Exact = r.Ratio.RatString()
	}
	return c
}

// prefixSame compares the first n digits of a and b. It is Unknown3 when the
// shorter code ends before n digits while everything it has still matches.
func prefixSame(a, b string, n int) Tri {
	k := min(len(a), len(b), n)
	if a[:k] != b[:k] {
		return False
	}
	if k == n {
		return True
	}
	return Unknown3
}

// hasPrefix reports whether code lies under prefix p; a code shorter than p is
// Unknown3 when it matches so far, False when it already differs.
func hasPrefix(code, p string) Tri {
	if code == "" {
		return Unknown3
	}
	k := min(len(code), len(p))
	if code[:k] != p[:k] {
		return False
	}
	if len(code) >= len(p) {
		return True
	}
	return Unknown3
}

func hasAnyPrefix(code string, ps []string) Tri {
	t := False
	for _, p := range ps {
		t = t.Or(hasPrefix(code, p))
	}
	return t
}

type evaluator struct {
	in  Input
	rvc *Result
}

func (e *evaluator) claim(m Material) OriginClaim {
	if m.Originating == "" {
		return ClaimUnknown
	}
	return m.Originating
}

// satisfy folds a material's claim into okIfNonOriginating (whether the
// material satisfies a condition if it is non-originating): originating
// materials always satisfy, unknown-origin ones only when they satisfy anyway.
func satisfy(c OriginClaim, ok Tri) Tri {
	switch c {
	case ClaimOriginating:
		return True
	case ClaimNonOriginating:
		return ok
	}
	if ok == True {
		return True
	}
	return Unknown3
}

// counts folds a claim into whether a material is a non-originating member of
// a set (used for existence checks).
func counts(c OriginClaim, member Tri) Tri {
	switch c {
	case ClaimOriginating:
		return False
	case ClaimNonOriginating:
		return member
	}
	if member == False {
		return False
	}
	return Unknown3
}

func (e *evaluator) materialFacts(cr *ConditionResult, i int, m Material, hsTri, final Tri) {
	label := fmt.Sprintf("materials[%d]", i)
	f := MaterialFinding{Index: i, ID: m.ID, Result: outcome(final)}
	if final == False {
		f.Note = "does not satisfy this condition"
	} else {
		f.Note = "cannot be determined from the facts given"
		if e.claim(m) == ClaimUnknown {
			cr.MissingFacts = append(cr.MissingFacts, MissingFact{"origin_status_unknown", label + ".originating", "State whether this material is originating or non-originating."})
		}
		if hsTri == Unknown3 && e.claim(m) != ClaimOriginating {
			if m.HS == "" {
				cr.MissingFacts = append(cr.MissingFacts, MissingFact{"material_hs_unknown", label + ".hs", "Provide the classification code of this material."})
			} else {
				cr.MissingFacts = append(cr.MissingFacts, MissingFact{"material_hs_imprecise", label + ".hs", "Provide a more specific classification code for this material."})
			}
		}
	}
	cr.Materials = append(cr.Materials, f)
}

// completeness returns the extra conjunct/disjunct for an unconfirmed
// materials list, and records the missing fact.
func (e *evaluator) completeness(cr *ConditionResult) Tri {
	if e.in.Profile.MaterialsComplete {
		return True
	}
	cr.MissingFacts = append(cr.MissingFacts, MissingFact{"materials_incomplete", "profile.materials_list_complete", "Confirm that every non-originating material is listed."})
	return Unknown3
}

// allMaterials evaluates ok(m) over every material (AND), under claims.
func (e *evaluator) allMaterials(cr *ConditionResult, ok func(Material) Tri) Tri {
	res := True
	for i, m := range e.in.Materials {
		hsTri := ok(m)
		final := satisfy(e.claim(m), hsTri)
		if final != True {
			e.materialFacts(cr, i, m, hsTri, final)
		}
		res = res.And(final)
	}
	return res.And(e.completeness(cr))
}

func (e *evaluator) condition(c Condition) ConditionResult {
	cr := ConditionResult{Kind: c.Kind}
	good := e.in.GoodHS
	var t Tri
	switch c.Kind {
	case KindChangeFromChapter, KindChangeFromHeading, KindChangeFromSubheading, KindChangeFromTariffItem:
		n := shiftDigits(c.Kind)
		t = e.allMaterials(&cr, func(m Material) Tri {
			shifted := prefixSame(good, m.HS, n).Not()
			if m.HS == "" {
				shifted = Unknown3
			}
			return shifted.And(hasAnyPrefix(m.HS, c.ExcludePrefixes).Not())
		})
		cr.Detail = fmt.Sprintf("every non-originating material must differ from the good in its first %d digits", n)
		if len(c.ExcludePrefixes) > 0 {
			cr.Detail += fmt.Sprintf(" and not be under %v", c.ExcludePrefixes)
		}
	case KindForbiddenInputs:
		t = e.allMaterials(&cr, func(m Material) Tri {
			if m.HS == "" {
				return Unknown3
			}
			hit := hasAnyPrefix(m.HS, c.Prefixes)
			if c.SameAsGoodLevel != "" {
				hit = hit.Or(prefixSame(good, m.HS, levelDigits(c.SameAsGoodLevel)))
			}
			return hit.Not()
		})
		cr.Detail = "no non-originating material may be a forbidden input"
		if c.SameAsGoodLevel != "" {
			cr.Detail += " (same " + c.SameAsGoodLevel + " as the good)"
		}
		if len(c.Prefixes) > 0 {
			cr.Detail += fmt.Sprintf(" (under %v)", c.Prefixes)
		}
	case KindAllInputsFrom:
		t = e.allInputsFrom(&cr, c)
		cr.Detail = fmt.Sprintf("every non-originating material must be under %v OR outside chapter %q", c.OthersFrom, c.OthersOutsideChapter)
	case KindSiblingParts:
		t = e.sibling(&cr, c)
		cr.Detail = fmt.Sprintf("at least one non-originating material under %v AND every other non-originating material under %v OR outside chapter %q", c.AtLeastOneFrom, c.OthersFrom, c.OthersOutsideChapter)
	case KindNoShiftRequired:
		t = True
		cr.Detail = "no tariff classification change is required for this alternative"
	case KindRVC:
		t = e.rvcCondition(&cr, c)
	}
	cr.Result = outcome(t)
	return cr
}

func (e *evaluator) allInputsFrom(cr *ConditionResult, c Condition) Tri {
	return e.allMaterials(cr, func(m Material) Tri {
		if m.HS == "" {
			return Unknown3
		}
		ok := hasAnyPrefix(m.HS, c.OthersFrom)
		if c.OthersOutsideChapter != "" {
			ok = ok.Or(hasPrefix(m.HS, c.OthersOutsideChapter).Not())
		}
		return ok
	})
}

func (e *evaluator) sibling(cr *ConditionResult, c Condition) Tri {
	// every other (non-X) material must be in Y or outside chapter C.
	others := e.allMaterials(cr, func(m Material) Tri {
		if m.HS == "" {
			return Unknown3
		}
		ok := hasAnyPrefix(m.HS, c.AtLeastOneFrom).Or(hasAnyPrefix(m.HS, c.OthersFrom))
		if c.OthersOutsideChapter != "" {
			ok = ok.Or(hasPrefix(m.HS, c.OthersOutsideChapter).Not())
		}
		return ok
	})
	exists := False
	for _, m := range e.in.Materials {
		var member Tri
		if m.HS == "" {
			member = Unknown3
		} else {
			member = hasAnyPrefix(m.HS, c.AtLeastOneFrom)
		}
		exists = exists.Or(counts(e.claim(m), member))
	}
	if !e.in.Profile.MaterialsComplete && exists != True {
		exists = exists.Or(Unknown3)
	}
	if exists == Unknown3 && len(cr.MissingFacts) == 0 {
		cr.MissingFacts = append(cr.MissingFacts, MissingFact{"sibling_part_unconfirmed", "materials", "Confirm whether a non-originating material from the required sibling code is present."})
	}
	if exists == False {
		cr.Materials = append(cr.Materials, MaterialFinding{Index: -1, Result: OutcomeNotSatisfied, Note: "no non-originating material from the required sibling code"})
	}
	return exists.And(others)
}

func (e *evaluator) rvcCondition(cr *ConditionResult, c Condition) Tri {
	cr.Detail = "regional value content must meet the threshold for the elected method"
	if e.rvc == nil {
		cr.MissingFacts = append(cr.MissingFacts, MissingFact{"rvc_method_required", "rvc_method", "Elect an RVC method (transaction_value or net_cost); none is chosen for you."})
		return Unknown3
	}
	var th *string
	if e.rvc.Method == MethodTransactionValue {
		th = c.Thresholds.TransactionValue
	} else {
		th = c.Thresholds.NetCost
	}
	if th == nil {
		cr.Detail = "this alternative has no threshold for the elected method " + string(e.rvc.Method)
		return False
	}
	cr.Detail = "RVC (" + string(e.rvc.Method) + ") must be at least " + *th + "%"
	switch e.rvc.Status {
	case StatusUndefinedDenominator:
		cr.MissingFacts = append(cr.MissingFacts, MissingFact{"rvc_undefined", e.rvc.Problems[0].Field, e.rvc.Problems[0].Message})
		return Unknown3
	case StatusNeedsInformation:
		for _, p := range e.rvc.Problems {
			cr.MissingFacts = append(cr.MissingFacts, MissingFact{p.Code, p.Field, p.Message})
		}
		return Unknown3
	}
	return e.rvc.Meets(*th, true)
}

func shiftDigits(kind string) int {
	switch kind {
	case KindChangeFromChapter:
		return levelDigits(LevelChapter)
	case KindChangeFromHeading:
		return levelDigits(LevelHeading)
	case KindChangeFromSubheading:
		return levelDigits(LevelSubheading)
	}
	return levelDigits(LevelTariffItem)
}
