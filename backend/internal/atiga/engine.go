package atiga

import (
	"errors"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
)

// Material statuses. "originating" means the supplier's declaration or
// proof of origin says the material is ATIGA-originating in its own right.
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
const Regime = "atiga_preferential"

// Note goes with every result.
const Note = "ATIGA preferential origin from the facts given. Not marking origin, not a ruling, not a Certificate of Origin (Form D). ITA goods (Annex 4), direct consignment, packaging, accessories and neutral elements are not checked. The rules have not been reviewed by a customs professional."

// Members are the ten ASEAN Member States party to ATIGA, by ISO 3166
// alpha-2 code. Timor-Leste joined ASEAN in October 2025 but is not yet a
// party to ATIGA.
var Members = map[string]string{
	"BN": "Brunei", "KH": "Cambodia", "ID": "Indonesia", "LA": "Laos",
	"MY": "Malaysia", "MM": "Myanmar", "PH": "Philippines", "SG": "Singapore",
	"TH": "Thailand", "VN": "Vietnam",
}

var memberAliases = map[string]string{
	"brunei darussalam": "BN", "lao pdr": "LA", "lao people's democratic republic": "LA",
	"viet nam": "VN", "burma": "MM",
}

// Member returns the ISO code of an ATIGA Member State from a name or code,
// or "" when s is not one.
func Member(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	if up := strings.ToUpper(s); len(up) == 2 {
		if _, ok := Members[up]; ok {
			return up
		}
		return ""
	}
	if c, ok := memberAliases[strings.ToLower(s)]; ok {
		return c
	}
	n := domain.NormalizeCountry(s)
	for code, name := range Members {
		if strings.EqualFold(n, name) {
			return code
		}
	}
	if c, ok := memberAliases[strings.ToLower(n)]; ok {
		return c
	}
	return ""
}

// MinimalOperations are the Art. 31 purposes. A good whose only processing
// of non-originating materials serves these is not originating.
var MinimalOperations = map[string]string{
	"preservation": "ensuring preservation of goods in good condition for transport or storage",
	"shipment":     "facilitating shipment or transportation",
	"packaging":    "packaging or presenting goods for sale",
}

// Processes are the conditions Annex 3 writes in words. The caller states
// each; the engine cannot test them from tariff codes.
var Processes = map[string]string{
	"textile_process":       "the working or processing of Attachment 1 (textiles) was carried out in a Member State",
	"cut_and_sewn":          "the good is both cut and sewn in the territory of any Member State",
	"refining":              "the good is produced by refining",
	"wet_to_dry":            "there is a change from the wet state to the dry state",
	"recording":             "sound or other phenomena were recorded onto blank or unrecorded media",
	"parts_same_subheading": "the machine, instrument or apparatus is made from parts produced solely for goods of the same subheading",
}

// Material is one input of the bill of materials.
type Material struct {
	Name string `json:"name"`
	// HS is the material's own HS code, as many digits as known.
	HS string `json:"hs"`
	// Value is the Art. 29.2 value: CIF at import, or the earliest price
	// paid in the Member State. Same currency as FOB.
	Value *float64 `json:"value,omitempty"`
	// Country is where the material originates (name or ISO code).
	Country string `json:"country,omitempty"`
	// Status is originating, non_originating or unknown (default unknown).
	Status string `json:"status,omitempty"`
	// RVCPct is the material's own ASEAN value content, for Art. 30.2
	// partial cumulation (counts only from 20 % to under 40 %). ATIGA only.
	RVCPct *float64 `json:"rvc_pct,omitempty"`
	// WhollyObtained says the material was wholly obtained or produced in a
	// Party. Some product rules need it (a material "wholly obtained in the
	// territory of any Party"); a material of unknown status leaves them
	// undetermined. AKFTA and ACFTA only.
	WhollyObtained *bool `json:"wholly_obtained,omitempty"`
	// Weight is the material's weight, for the textile de minimis (chapters
	// 50 to 63), same unit as the good's weight. AKFTA and ACFTA only.
	Weight *float64 `json:"weight,omitempty"`
}

// Costs are the direct method's other terms, Art. 29.1(a).
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
	Materials  []Material `json:"materials"`
	Costs      *Costs     `json:"costs,omitempty"`
	// Method is the RVC method the exporting Member State uses (Art. 29.3):
	// "direct", "indirect", or empty to try both.
	Method string `json:"method,omitempty"`
	// WhollyObtained is the producer's statement that the good is wholly
	// obtained or produced in the Member State (Art. 27).
	WhollyObtained bool `json:"wholly_obtained,omitempty"`
	// Operations is what is done to the non-originating materials, using
	// the MinimalOperations keys or any other word for real production.
	Operations []string `json:"operations,omitempty"`
	// Processes states the Annex 3 process conditions (Processes keys).
	// A key left out is not stated.
	Processes map[string]bool `json:"processes,omitempty"`
	// Variant picks the branch of a rule that depends on the product.
	Variant string `json:"variant,omitempty"`
	// Weight is the good's weight, for the textile de minimis (chapters 50
	// to 63). AKFTA and ACFTA only.
	Weight *float64 `json:"weight,omitempty"`
}

// RuleRef identifies the rule used.
type RuleRef struct {
	Code     string    `json:"code"`
	Listed   bool      `json:"listed"`
	Concise  string    `json:"concise"`
	Full     string    `json:"full"`
	Version  string    `json:"version"`
	Variants []Variant `json:"variants,omitempty"`
	Note     string    `json:"note,omitempty"`
}

// RVC is the regional value content worked both ways, in per cent.
type RVC struct {
	Threshold int      `json:"threshold"`
	Direct    *float64 `json:"direct,omitempty"`
	Indirect  *float64 `json:"indirect,omitempty"`
	VNM       float64  `json:"vnm"`
	VOM       float64  `json:"vom"`
	// Partial is the ASEAN content counted from partially cumulated
	// materials (Art. 30.2).
	Partial float64 `json:"partial,omitempty"`
}

// DeMinimis reports an Art. 33 test that was needed.
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
	// Basis is WO (wholly obtained), PE (only originating materials) or PSR
	// (the table rule or the general rule). Empty when no.
	Basis             string      `json:"basis,omitempty"`
	Applied           string      `json:"applied,omitempty"`
	Rule              *RuleRef    `json:"rule,omitempty"`
	RVC               *RVC        `json:"rvc,omitempty"`
	DeMinimis         *DeMinimis  `json:"de_minimis,omitempty"`
	Accumulation      bool        `json:"accumulation"`
	PartialCumulation bool        `json:"partial_cumulation"`
	Failing           []Issue     `json:"failing"`
	Alternatives      []AltResult `json:"alternatives"`
	Reasons           []string    `json:"reasons"`
	Regime            string      `json:"regime"`
	Note              string      `json:"note"`
}

// ErrInput marks a request that is malformed rather than incomplete.
var ErrInput = errors.New("atiga: bad input")

func bad(format string, a ...any) error {
	return fmt.Errorf("%w: %s", ErrInput, fmt.Sprintf(format, a...))
}

func finite(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }

// Validate rejects malformed ATIGA input. Missing facts are not errors: they
// make the answer undetermined.
func Validate(in Input) error { return ATIGAProfile.Validate(in) }

// Validate rejects malformed input for the agreement of p.
func (p *Profile) Validate(in Input) error {
	if n := len(digits(in.HS)); n < 4 || n > 10 {
		return bad("hs must have 4 to 10 digits")
	}
	if in.FOB != nil && (*in.FOB <= 0 || !finite(*in.FOB)) {
		return bad("fob must be positive")
	}
	if in.Weight != nil && (*in.Weight <= 0 || !finite(*in.Weight)) {
		return bad("weight must be positive")
	}
	if in.Costs != nil {
		for _, v := range []float64{in.Costs.Labour, in.Costs.Overhead, in.Costs.Profit, in.Costs.Other} {
			if v < 0 || !finite(v) {
				return bad("costs must be zero or more")
			}
		}
	}
	switch in.Method {
	case "", "indirect":
	case "direct":
		if p.BuildDownOnly {
			return bad("method must be indirect: %s has only the build-down formula", p.Name)
		}
	default:
		if p.BuildDownOnly {
			return bad("method must be indirect")
		}
		return bad("method must be direct or indirect")
	}
	for k := range in.Processes {
		if _, ok := p.Processes[k]; !ok {
			return bad("processes.%s is not one of %s", k, strings.Join(p.ProcessKeys(), ", "))
		}
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
		if m.Value != nil && (*m.Value < 0 || !finite(*m.Value)) {
			return bad("materials[%d].value must be zero or more", i)
		}
		if m.Weight != nil && (*m.Weight < 0 || !finite(*m.Weight)) {
			return bad("materials[%d].weight must be zero or more", i)
		}
		if p := m.RVCPct; p != nil && (*p < 0 || *p > 100 || !finite(*p)) {
			return bad("materials[%d].rvc_pct must be 0 to 100", i)
		}
	}
	return nil
}

func processKeys() []string { return ATIGAProfile.ProcessKeys() }

// mat is a material after its status is settled.
type mat struct {
	Material
	idx         int
	originating bool
	// party is the ISO code of the Party the material comes from, "" when it
	// is not one or no country was given.
	party string
	// partial is the share (0-1) of its value counted as ASEAN content
	// under Art. 30.2; zero when it does not apply.
	partial float64
}

// run is one check: the agreement, the input and the materials settled.
type run struct {
	p       *Profile
	in      Input
	good    string
	member  string
	mats    []mat
	nonOrig int
}

// Check decides ATIGA origin with the default table.
func Check(in Input) (Result, error) { return Default().Check(in) }

// onlyMinimal reports whether every operation named is one the profile
// treats as minimal, with their descriptions.
func (p *Profile) onlyMinimal(ops []string) (bool, []string) {
	if len(ops) == 0 {
		return false, nil
	}
	var names []string
	for _, op := range ops {
		d, ok := p.MinimalOps[strings.ToLower(strings.TrimSpace(op))]
		if !ok {
			return false, nil
		}
		names = append(names, d)
	}
	return true, names
}

// onlyPackaging reports whether the only operations named are packaging or
// presenting goods for sale.
func (p *Profile) onlyPackaging(ops []string) bool {
	if len(ops) == 0 {
		return false
	}
	for _, op := range ops {
		k := strings.ToLower(strings.TrimSpace(op))
		found := false
		for _, pk := range p.PackagingKeys {
			if k == pk {
				found = true
			}
		}
		if !found {
			return false
		}
	}
	return true
}

// Check decides origin for in under the agreement of the table's profile.
func (t *Table) Check(in Input) (Result, error) {
	p := t.Profile()
	if err := p.Validate(in); err != nil {
		return Result{}, err
	}
	res := Result{Regime: p.Regime, Note: p.Note, Failing: []Issue{}, Alternatives: []AltResult{}}
	say := func(format string, a ...any) { res.Reasons = append(res.Reasons, fmt.Sprintf(format, a...)) }
	good := digits(in.HS)
	cite := p.Cites

	member := p.member(in.ProducedIn)
	switch {
	case strings.TrimSpace(in.ProducedIn) == "":
		res.Outcome = Undetermined
		say("Say where the good is made.")
		return res, nil
	case member == "":
		res.Outcome = No
		say("%s is not %s. Only goods made in %s can qualify (%s).", strings.TrimSpace(in.ProducedIn), cite.NotParty, cite.OnlyIn, cite.Origin)
		return res, nil
	}

	r, listed, found, need := t.Lookup(good)
	if found {
		res.Rule = &RuleRef{Code: r.Code, Listed: listed, Concise: r.Concise, Full: r.Full, Version: t.Version, Variants: r.Variants, Note: r.Note}
	}

	minimal, minimalNames := p.onlyMinimal(in.Operations)
	if in.WhollyObtained {
		if p.MinimalOnlyForWO && minimal {
			say("Only minimal operations are done (%s). They do not count in determining whether a good is wholly obtained (%s), so the wholly obtained statement is not relied on.", strings.Join(minimalNames, "; "), cite.Minimal)
		} else {
			res.Outcome, res.Basis, res.Applied = Yes, "WO", "WO"
			say("You state the good is wholly obtained or produced in %s (%s), so it qualifies (%s).", p.Parties[member], cite.WO, cite.OriginWO)
			return res, nil
		}
	}

	// Settle each material's status.
	x := &run{p: p, in: in, good: good, member: member}
	mats := make([]mat, len(in.Materials))
	nonOrig := 0
	for i, m := range in.Materials {
		mm := mat{Material: m, idx: i}
		c := p.member(m.Country)
		mm.party = c
		if m.Status == StatusOriginating {
			if strings.TrimSpace(m.Country) != "" && c == "" {
				say("%s is declared originating but comes from %s, which is not a %s; it counts as non-originating (%s).", label(m, i), strings.TrimSpace(m.Country), cite.PartyWord, cite.Cumulation)
			} else {
				mm.originating = true
				if c != "" && c != member {
					res.Accumulation = true
				}
			}
		}
		if !mm.originating {
			nonOrig++
			if p.PartialCumulation {
				if pc := m.RVCPct; pc != nil && c != "" && *pc >= 20 && *pc < 40 {
					mm.partial = *pc / 100
				}
			}
		}
		mats[i] = mm
	}
	x.mats, x.nonOrig = mats, nonOrig
	if unk := countStatus(in.Materials, StatusUnknown, ""); unk > 0 {
		say("%s of undetermined origin %s as non-originating.", plural(unk, "material", "materials"), isAre(unk, "counts", "count"))
	}
	if res.Accumulation {
		say("Originating materials from other %ss count as originating (%s).", cite.PartyWord, cite.Cumulate)
	}

	if len(mats) == 0 {
		res.Outcome = Undetermined
		say("List the materials, or state that the good is wholly obtained.")
		return res, nil
	}
	if nonOrig == 0 && p.PE {
		res.Outcome, res.Basis, res.Applied = Yes, "PE", "PE"
		say("Every material is originating, so the good qualifies (%s).", cite.PE)
		return res, nil
	}

	// Minimal operations confer nothing.
	if minimal {
		if p.MinimalOnlyForWO {
			say("Only minimal operations are done (%s). They do not count in determining whether a good is wholly obtained (%s); the product-specific rule is still tested.", strings.Join(minimalNames, "; "), cite.Minimal)
		} else {
			res.Outcome = No
			say("Only minimal operations are done on non-originating materials (%s), which do not confer origin (%s).", strings.Join(minimalNames, "; "), cite.Minimal)
			return res, nil
		}
	}

	if !found {
		res.Outcome = Undetermined
		say("No rule applied: %s.", need)
		return res, nil
	}
	if listed {
		say("%s rule for %s: %s.", cite.Annex, r.Code, r.Concise)
	} else {
		say("%s is not listed in %s, so the general rule applies: %s (%s).", dotted(good[:min(6, len(good))]), cite.Annex, r.Concise, cite.General)
	}
	if r.Note != "" {
		say("Note in the rule: %s", r.Note)
	}

	alts := r.Alternatives
	if len(r.Variants) > 0 {
		v, ok := pickVariant(r.Variants, in.Variant)
		switch {
		case ok:
			alts = v.Alternatives
			say("Using the branch for %s.", v.Label)
		case in.Variant != "":
			return Result{}, bad("variant %q is not one of %s", in.Variant, variantIDs(r.Variants))
		default:
			out := x.eachVariant(res, r)
			if err := finiteResult(out); err != nil {
				return Result{}, err
			}
			return out, nil
		}
	}
	x.applyAlternatives(alts, &res)
	if err := finiteResult(res); err != nil {
		return Result{}, err
	}
	if len(in.Operations) == 0 && res.Outcome == Yes {
		say("This assumes the work done is more than the minimal operations of %s.", cite.Minimal)
	}
	return res, nil
}

func (x *run) eachVariant(base Result, r *Rule) Result {
	var outs []Result
	for _, v := range r.Variants {
		res := base
		res.Reasons = append([]string(nil), base.Reasons...)
		res.Alternatives = []AltResult{}
		x.applyAlternatives(v.Alternatives, &res)
		outs = append(outs, res)
	}
	for _, o := range outs[1:] {
		if o.Outcome != outs[0].Outcome {
			res := base
			res.Outcome = Undetermined
			var opts []string
			for _, v := range r.Variants {
				opts = append(opts, fmt.Sprintf("%q (%s)", v.ID, v.Label))
			}
			res.Reasons = append(res.Reasons, "The rule depends on the product and the branches disagree. Pick a variant: "+strings.Join(opts, ", ")+".")
			return res
		}
	}
	res := outs[0]
	res.Reasons = append(res.Reasons, "The rule depends on the product, but every branch gives the same answer.")
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
// is yes (the exporter chooses), no if all are no, else undetermined.
func (x *run) applyAlternatives(alts []Alternative, res *Result) {
	outcome := No
	var failing []Issue
	var failingDM *DeMinimis
	for _, a := range alts {
		ar, issues, rvc, dm := x.evalAlt(a)
		res.Alternatives = append(res.Alternatives, ar)
		if rvc != nil && (res.RVC == nil || rvc.Threshold == 40) {
			res.RVC = rvc
			if rvc.Partial > 0 {
				res.PartialCumulation = true
			}
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
		if x.p.FinalProcess != "" {
			res.Reasons = append(res.Reasons, x.p.FinalProcess)
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

// evalAlt evaluates one alternative: every requirement must hold.
func (x *run) evalAlt(a Alternative) (AltResult, []Issue, *RVC, *DeMinimis) {
	ar := AltResult{Rule: a.Name, Outcome: Yes}
	var issues []Issue
	var rvcOut *RVC
	var dmOut *DeMinimis
	for _, q := range a.Requires {
		var out string
		var why []string
		switch q.Kind {
		case "wo":
			if x.nonOrig > 0 {
				out, why = No, []string{"The good uses non-originating materials, so it is not wholly obtained."}
			} else {
				out, why = Undetermined, []string{"The rule needs the good to be wholly obtained or produced in the exporting " + x.p.Cites.PartyWord + "; state wholly_obtained if it is."}
			}
		case "wo_any":
			out, why = x.woAny()
		case "rvc":
			var rvc *RVC
			rvc, out, why = x.regionalValue(q.Threshold)
			rvcOut = rvc
		case "ctc":
			var iss []Issue
			var dm *DeMinimis
			out, why, iss, dm = x.tariffShift(q)
			issues, dmOut = iss, dm
		case "materials":
			var iss []Issue
			out, why, iss = x.materialScope(q)
			if iss != nil && issues == nil {
				issues = iss
			}
		case "value_cap":
			out, why = x.valueCap(q)
		case "process":
			desc := x.p.Processes[q.Process]
			v, stated := x.in.Processes[q.Process]
			switch {
			case !stated:
				out, why = Undetermined, []string{fmt.Sprintf("Not stated whether %s (processes.%s).", desc, q.Process)}
			case v:
				out, why = Yes, []string{fmt.Sprintf("You state %s.", desc)}
				if q.Process == "textile_process" {
					why = append(why, "Attachment 1 paragraph 2 operations alone (combining, labelling, pressing, cleaning, packaging, simple cutting and hemming, finishing, dyeing or printing) do not count.")
				}
			default:
				out, why = No, []string{fmt.Sprintf("You state it is not the case that %s.", desc)}
			}
		default:
			out, why = Undetermined, []string{"Unknown requirement " + q.Kind + "."}
		}
		ar.Reasons = append(ar.Reasons, why...)
		switch {
		case out == No:
			ar.Outcome = No
		case out == Undetermined && ar.Outcome == Yes:
			ar.Outcome = Undetermined
		}
	}
	return ar, issues, rvcOut, dmOut
}

// woAny tests a rule that needs the good wholly obtained or produced in the
// territory of any Party: every material must itself be wholly obtained in a
// Party.
func (x *run) woAny() (string, []string) {
	var bad, unknown []string
	for _, m := range x.mats {
		switch {
		case !m.originating && m.party == "" && m.Status == StatusOriginating:
			bad = append(bad, label(m.Material, m.idx))
		case !m.originating:
			bad = append(bad, label(m.Material, m.idx))
		case m.WhollyObtained == nil:
			unknown = append(unknown, label(m.Material, m.idx))
		case !*m.WhollyObtained:
			bad = append(bad, label(m.Material, m.idx))
		}
	}
	switch {
	case len(bad) > 0:
		return No, []string{fmt.Sprintf("The rule needs the good wholly obtained or produced in the territory of any Party; %s %s not wholly obtained in a Party.", strings.Join(bad, ", "), isAre(len(bad), "is", "are"))}
	case len(unknown) > 0:
		return Undetermined, []string{fmt.Sprintf("The rule needs the good wholly obtained or produced in the territory of any Party; say whether %s %s wholly obtained in a Party (materials[].wholly_obtained), or state wholly_obtained for the good.", strings.Join(unknown, ", "), isAre(len(unknown), "is", "are"))}
	}
	return Yes, []string{"Every material is wholly obtained or produced in a Party, so the good is wholly obtained in the territory of any Party."}
}

var levelDigits = map[string]int{"chapter": 2, "heading": 4, "subheading": 6}

// inRange tells whether a material's code falls in a range: yes, no or, when
// the code is too short to say, unknown.
func inRange(md string, g Range) (in, unknown bool) {
	k := levelDigits[g.Level]
	from, to := digits(g.From), digits(g.To)
	if len(md) < k {
		lo := md + strings.Repeat("0", k-len(md))
		hi := md + strings.Repeat("9", k-len(md))
		return false, hi >= from && lo <= to
	}
	code := md[:k]
	return code >= from && code <= to, false
}

func describe(g Range) string {
	if g.From == g.To {
		return g.Level + " " + dotted(digits(g.From))
	}
	return g.Level + "s " + dotted(digits(g.From)) + " to " + dotted(digits(g.To))
}

// materialScope tests a condition on the materials classified in a set of
// chapters, headings or subheadings: each must be wholly obtained or
// originating in a Party (any Party, or the exporting Party).
func (x *run) materialScope(q Requirement) (string, []string, []Issue) {
	var fail, unknown []Issue
	inScope := 0
	exporting := strings.HasSuffix(q.Origin, "exporting")
	wo := strings.HasPrefix(q.Origin, "wo_")
	for _, m := range x.mats {
		md := digits(m.HS)
		covered, short := false, false
		for _, g := range q.Scope {
			in, unk := inRange(md, g)
			covered = covered || in
			short = short || unk
		}
		if !covered && !short {
			continue
		}
		iss := Issue{Index: m.idx, Name: m.Name, HS: m.HS}
		if !covered {
			iss.Reason = "needs more HS digits to tell whether the rule's condition on materials applies"
			unknown = append(unknown, iss)
			continue
		}
		inScope++
		where := "any Party"
		if exporting {
			where = "the exporting " + x.p.Cites.PartyWord
		}
		okOrigin := m.originating && (!exporting || m.party == x.member || m.party == "")
		switch {
		case wo && !m.originating:
			iss.Reason = "is not wholly obtained in " + where + " (it is not originating)"
			fail = append(fail, iss)
		case wo && m.WhollyObtained == nil:
			iss.Reason = "needs a statement whether it is wholly obtained in " + where + " (wholly_obtained)"
			unknown = append(unknown, iss)
		case wo && (!*m.WhollyObtained || !okOrigin):
			iss.Reason = "is not wholly obtained in " + where
			fail = append(fail, iss)
		case !wo && !okOrigin:
			iss.Reason = "is not originating in " + where
			fail = append(fail, iss)
		}
	}
	scope := make([]string, 0, len(q.Scope))
	for _, g := range q.Scope {
		scope = append(scope, describe(g))
	}
	what := "originating in"
	if wo {
		what = "wholly obtained in"
	}
	where := "any Party"
	if exporting {
		where = "the exporting " + x.p.Cites.PartyWord
	}
	cond := fmt.Sprintf("materials of %s must be %s %s", strings.Join(scope, ", "), what, where)
	issues := append(append([]Issue{}, fail...), unknown...)
	sort.Slice(issues, func(i, j int) bool { return issues[i].Index < issues[j].Index })
	var why []string
	for _, f := range issues {
		code := ""
		if strings.TrimSpace(f.HS) != "" {
			code = " (" + f.HS + ")"
		}
		why = append(why, fmt.Sprintf("%s%s %s.", label(x.mats[f.Index].Material, f.Index), code, f.Reason))
	}
	switch {
	case len(fail) > 0:
		return No, append([]string{"The rule's condition fails: " + cond + "."}, why...), issues
	case len(unknown) > 0:
		return Undetermined, append([]string{"The rule's condition: " + cond + "."}, why...), issues
	case inScope == 0:
		return Yes, []string{"No material is classified in " + strings.Join(scope, ", ") + ", so the condition on those materials is met."}, nil
	}
	return Yes, []string{"The rule's condition is met: " + cond + "."}, nil
}

// valueCap tests that the non-originating materials in a set of headings
// stay within a share of the FOB value.
func (x *run) valueCap(q Requirement) (string, []string) {
	scope := make([]string, 0, len(q.Scope))
	for _, g := range q.Scope {
		scope = append(scope, describe(g))
	}
	where := strings.Join(scope, ", ")
	var v float64
	unknown := false
	for _, m := range x.mats {
		if m.originating {
			continue
		}
		md := digits(m.HS)
		covered, short := false, false
		for _, g := range q.Scope {
			in, unk := inRange(md, g)
			covered = covered || in
			short = short || unk
		}
		switch {
		case covered && m.Value == nil, short:
			unknown = true
		case covered:
			v += *m.Value
		}
	}
	if x.in.FOB == nil || unknown {
		return Undetermined, []string{fmt.Sprintf("Give the FOB value and the value and HS code of every non-originating material to test the cap on materials of %s (%s %% of the FOB value).", where, pct(q.Percent))}
	}
	s := v / *x.in.FOB * 100
	if s <= q.Percent+1e-9 {
		return Yes, []string{fmt.Sprintf("Non-originating materials of %s are %s %% of the FOB value, within the %s %% cap.", where, pct(s), pct(q.Percent))}
	}
	return No, []string{fmt.Sprintf("Non-originating materials of %s are %s %% of the FOB value, over the %s %% cap.", where, pct(s), pct(q.Percent))}
}

// tariffShift checks a change of chapter, heading or subheading and, when
// materials fail it, the de minimis rule.
func (x *run) tariffShift(q Requirement) (string, []string, []Issue, *DeMinimis) {
	in, good, mats, p := x.in, x.good, x.mats, x.p
	n := levelDigits[q.Level]
	if len(good) < n {
		return Undetermined, []string{fmt.Sprintf("A change of %s needs the good's %d-digit code.", q.Level, n)}, nil, nil
	}
	if q.NotOnlyPackaging && p.onlyPackaging(in.Operations) {
		return No, []string{"The change of tariff classification comes only from packaging or presenting goods for sale, which does not count."}, nil, nil
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
			if q.Within && q.Level == "subheading" {
				iss.Reason = fmt.Sprintf("is in the same subheading as the good (%s); the rule also accepts a change within the subheading, which needs the national tariff line", dotted(good[:n]))
				unknown = append(unknown, iss)
				continue
			}
			iss.Reason = fmt.Sprintf("same %s as the good (%s), so no change", q.Level, dotted(good[:n]))
			fail = append(fail, iss)
			continue
		}
		excluded, short, partial := false, false, ""
		for _, g := range q.Except {
			k := levelDigits[g.Level]
			from, to := digits(g.From), digits(g.To)
			if len(md) < k {
				lo := md + strings.Repeat("0", k-len(md))
				hi := md + strings.Repeat("9", k-len(md))
				if hi >= from && lo <= to {
					short = true
				}
				continue
			}
			if code := md[:k]; code >= from && code <= to {
				if g.Partial != "" {
					partial = g.Partial
					iss.Reason = fmt.Sprintf("comes from %s %s, of which the rule excludes only %s; say whether it is", g.Level, dotted(code), g.Partial)
					continue
				}
				excluded = true
				iss.Reason = fmt.Sprintf("comes from %s %s, which the rule excludes", g.Level, dotted(code))
				break
			}
		}
		switch {
		case excluded:
			fail = append(fail, iss)
		case partial != "":
			unknown = append(unknown, iss)
		case short:
			iss.Reason = "needs more HS digits to test the rule's exclusions"
			unknown = append(unknown, iss)
		}
	}
	if len(fail)+len(unknown) == 0 {
		return Yes, []string{fmt.Sprintf("Every non-originating material changes %s.", q.Level)}, nil, nil
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
	out, dwhy, dm := x.deMinimis(q, issues, len(unknown) > 0)
	return out, append(why, dwhy...), issues, dm
}

// isTextile reports whether the good is in chapters 50 to 63.
func isTextile(good string) bool {
	if len(good) < 2 {
		return false
	}
	return good[:2] >= "50" && good[:2] <= "63"
}

// deMinimis tests the de minimis rule for the materials that fail a
// tariff-shift requirement, counting materials it cannot test as failing.
func (x *run) deMinimis(q Requirement, issues []Issue, hasUnknown bool) (string, []string, *DeMinimis) {
	p, in, mats := x.p, x.in, x.mats
	cite := p.Cites
	pc := p.DeMinimis.Percent
	dm := &DeMinimis{}
	var why []string
	if q.NoDeMinimisSameSubheading {
		for _, f := range issues {
			m := mats[f.Index]
			if md := digits(m.HS); len(md) >= 6 && len(x.good) >= 6 && md[:6] == x.good[:6] && m.party == "" {
				why = append(why, fmt.Sprintf("De minimis cannot cover %s: for this good it does not apply to a non-originating material imported from a non-Party that is in the good's own subheading.", label(m.Material, m.idx)))
				return No, why, dm
			}
		}
	}
	textile := isTextile(x.good) && p.DeMinimis.Textiles != ""
	useValue := !textile || p.DeMinimis.Textiles == "weight_or_value"
	useWeight := textile

	type path struct {
		out   string
		share *float64
		why   string
	}
	valuePath := func() path {
		known := in.FOB != nil
		var v float64
		for _, f := range issues {
			m := mats[f.Index]
			if m.Value == nil {
				known = false
				break
			}
			v += *m.Value
		}
		if !known {
			return path{Undetermined, nil, fmt.Sprintf("Give their values and the FOB value to test de minimis (%s).", cite.DeMinimis)}
		}
		s := v / *in.FOB * 100
		if s <= pc+1e-9 {
			return path{Yes, &s, fmt.Sprintf("De minimis: those materials are %s %% of the FOB value, within %s %% (%s).", pct(s), pct(pc), cite.DeMinimis)}
		}
		return path{No, &s, fmt.Sprintf("Those materials are %s %% of the FOB value, over the %s %% de minimis (%s).", pct(s), pct(pc), cite.DeMinimis)}
	}
	weightPath := func() path {
		known := in.Weight != nil
		var v float64
		for _, f := range issues {
			m := mats[f.Index]
			if m.Weight == nil {
				known = false
				break
			}
			v += *m.Weight
		}
		if !known {
			return path{Undetermined, nil, fmt.Sprintf("Give their weights and the weight of the good to test de minimis by weight (%s).", cite.DeMinimis)}
		}
		s := v / *in.Weight * 100
		if s <= pc+1e-9 {
			return path{Yes, &s, fmt.Sprintf("De minimis: those materials are %s %% of the weight of the good, within %s %% (%s).", pct(s), pct(pc), cite.DeMinimis)}
		}
		return path{No, &s, fmt.Sprintf("Those materials are %s %% of the weight of the good, over the %s %% de minimis (%s).", pct(s), pct(pc), cite.DeMinimis)}
	}
	var paths []path
	if useValue {
		vp := valuePath()
		dm.ValueShare = vp.share
		paths = append(paths, vp)
	}
	if useWeight {
		wp := weightPath()
		dm.WeightShare = wp.share
		paths = append(paths, wp)
	}
	outcome := No
	for _, pa := range paths {
		why = append(why, pa.why)
		switch pa.out {
		case Yes:
			outcome = Yes
		case Undetermined:
			if outcome == No {
				outcome = Undetermined
			}
		}
	}
	switch {
	case outcome == Yes:
		dm.Used = true
		return Yes, why, dm
	case hasUnknown && outcome == No:
		return Undetermined, why, dm
	}
	return outcome, why, dm
}

// regionalValue works out RVC both ways, with Art. 30.2 partial cumulation
// where the agreement has it.
func (x *run) regionalValue(threshold int) (*RVC, string, []string) {
	in, mats, p := x.in, x.mats, x.p
	cite := p.Cites
	rvc := &RVC{Threshold: threshold}
	if x.nonOrig == 0 {
		full := 100.0
		rvc.Indirect = &full
		return rvc, Yes, []string{"Every material is originating, so the regional value content is 100 %."}
	}
	if in.FOB == nil {
		return rvc, Undetermined, []string{"Give the FOB value to work out RVC."}
	}
	fob := *in.FOB
	vnmKnown, vomKnown := true, true
	var partialNames []string
	for _, m := range mats {
		switch {
		case m.Value == nil && m.originating:
			vomKnown = false
		case m.Value == nil:
			vnmKnown = false
		case m.originating:
			rvc.VOM += *m.Value
		default:
			asean := *m.Value * m.partial
			rvc.Partial += asean
			rvc.VNM += *m.Value - asean
			if m.partial > 0 {
				partialNames = append(partialNames, fmt.Sprintf("%s (%s %%)", label(m.Material, m.idx), pct(m.partial*100)))
			}
		}
	}
	var why []string
	if len(partialNames) > 0 {
		why = append(why, fmt.Sprintf("Partial cumulation: the ASEAN content of %s counts, %s in all (%s).", strings.Join(partialNames, ", "), money(rvc.Partial), cite.Partial))
	}
	t := float64(threshold)
	useInd := in.Method != "direct"
	useDir := in.Method != "indirect" && !p.BuildDownOnly
	pass := false
	indFail, dirFail := !useInd, !useDir
	if useInd {
		if vnmKnown {
			ind := (fob - rvc.VNM) / fob * 100
			rvc.Indirect = &ind
			why = append(why, fmt.Sprintf("Indirect method: (FOB %s - non-originating %s) / FOB = %s %%.", money(fob), money(rvc.VNM), pct(ind)))
			pass = pass || ind >= t-1e-9
			indFail = ind < t-1e-9
		} else {
			why = append(why, "The indirect method needs a value for every non-originating material.")
		}
	}
	if useDir {
		switch c := in.Costs; {
		case c == nil:
			if in.Method == "direct" {
				why = append(why, "The direct method needs costs (labour, overhead, profit, other).")
			}
		case !vomKnown:
			why = append(why, "The direct method needs a value for every originating material.")
		default:
			dir := (rvc.VOM + rvc.Partial + c.Labour + c.Overhead + c.Profit + c.Other) / fob * 100
			rvc.Direct = &dir
			why = append(why, fmt.Sprintf("Direct method: (ASEAN materials %s + labour %s + overhead %s + profit %s + other %s) / FOB = %s %%.",
				money(rvc.VOM+rvc.Partial), money(c.Labour), money(c.Overhead), money(c.Profit), money(c.Other), pct(dir)))
			pass = pass || dir >= t-1e-9
			dirFail = dir < t-1e-9
		}
	}
	switch {
	case pass:
		why = append(why, fmt.Sprintf("At least %d %% needed: met.", threshold))
		if in.Method == "" && !p.BuildDownOnly {
			why = append(why, fmt.Sprintf("Each %s uses one method (%s); give method to check only yours.", cite.PartyWord, cite.Method))
		}
		return rvc, Yes, why
	case indFail && dirFail:
		why = append(why, fmt.Sprintf("At least %d %% needed: not met.", threshold))
		return rvc, No, why
	case indFail && in.Costs == nil && in.Method == "":
		why = append(why, fmt.Sprintf("At least %d %% needed: not met by the indirect method; the direct method needs costs.", threshold))
		return rvc, No, why
	}
	why = append(why, fmt.Sprintf("At least %d %% needed: not enough data to decide.", threshold))
	return rvc, Undetermined, why
}

// finiteResult rejects a result whose computed shares are not finite, so the
// route answers 400 rather than failing to encode.
func finiteResult(res Result) error {
	check := func(name string, p *float64) error {
		if p != nil && !finite(*p) {
			return bad("computed %s share is not finite; reduce the values", name)
		}
		return nil
	}
	if r := res.RVC; r != nil {
		for _, c := range []struct {
			n string
			p *float64
		}{{"direct", r.Direct}, {"indirect", r.Indirect}, {"VNM", &r.VNM}, {"VOM", &r.VOM}, {"partial", &r.Partial}} {
			if err := check(c.n, c.p); err != nil {
				return err
			}
		}
	}
	if d := res.DeMinimis; d != nil {
		if err := check("de minimis", d.ValueShare); err != nil {
			return err
		}
		return check("de minimis weight", d.WeightShare)
	}
	return nil
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
