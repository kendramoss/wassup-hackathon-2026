package textilerules

import (
	"fmt"
	"sort"
	"strings"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
)

// Tri is a fact that may not be given.
type Tri int8

const (
	Unknown Tri = iota
	Yes
	No
)

func (t Tri) String() string {
	switch t {
	case Yes:
		return "yes"
	case No:
		return "no"
	}
	return "not_stated"
}

// MarshalText and UnmarshalText keep facts readable in committed files.
func (t Tri) MarshalText() ([]byte, error) { return []byte(t.String()), nil }

func (t *Tri) UnmarshalText(b []byte) error {
	switch string(b) {
	case "yes":
		*t = Yes
	case "no":
		*t = No
	default:
		*t = Unknown
	}
	return nil
}

// Stage names of the operations a good goes through, as the facts give them.
// "finishing" carries which operation in Op.
const (
	StageDyeing  = "dyeing"
	StagePrint   = "printing"
	StageFinish  = "finishing" // bleaching, shrinking, fulling, napping, decating, permanent stiffening, weighting, permanent embossing, moireing
	StageCutting = "cutting"
	StagePacking = "packing"
)

// Stage is one operation and the country where it took place.
type Stage struct {
	Process string `json:"process"`
	Op      string `json:"op,omitempty"`
	Country string `json:"country"`
	Quote   string `json:"quote,omitempty"`
}

// Material is a material that goes into the good, with its HTS code when the
// text gives one and the country it comes from when stated.
type Material struct {
	Name    string `json:"name"`
	HTS     string `json:"hts,omitempty"`
	Country string `json:"country,omitempty"`
}

// Facts is what the check reads about a good.
type Facts struct {
	GoodHTS             string  `json:"good_hts"`
	KnitToShape         Tri     `json:"knit_to_shape"`
	TwoOrMoreComponents Tri     `json:"two_or_more_components"`
	CottonWool16        Tri     `json:"cotton_wool_16"` // of cotton or wool, or a blend of 16 percent or more cotton
	FabricOfWool        Tri     `json:"fabric_of_wool"` // fabric (or lace, netting) of wool or fine animal hair
	Fibre               string  `json:"fibre_form,omitempty"`
	Stages              []Stage `json:"stages"`
	// Materials are the materials the text names with the code it gives; a
	// foreign material in a code the rule's tariff shift excludes means the
	// rule is not met in that country (102.21(c)(2): each foreign material
	// must change).
	Materials []Material `json:"materials,omitempty"`
	// Complete says the stages are every operation from the earliest
	// material the text names to the finished good.
	Complete bool `json:"complete"`
	// RawMaterialCountry is where the raw fibre or polymer was grown or
	// made, when stated (for wholly obtained goods).
	RawMaterialCountry string `json:"raw_material_country,omitempty"`
}

// Statuses of a result.
const (
	Decided      = "decided"
	Undetermined = "undetermined"
	OutOfScope   = "out_of_scope"
)

// Result is the check's answer.
type Result struct {
	Status string `json:"status"`
	Origin string `json:"origin,omitempty"`
	// Para is the paragraph of 102.21 that gave the answer.
	Para  string `json:"para,omitempty"`
	Row   string `json:"row,omitempty"`
	Alt   string `json:"alt,omitempty"`
	Basis string `json:"basis,omitempty"`
	// Reason says why the check did not decide; Class buckets it.
	Reason string `json:"reason,omitempty"`
	Class  string `json:"class,omitempty"`
	// Assumes lists what a decision rests on: the foreign materials whose
	// codes were not given are taken to make the change the rule needs.
	Assumes []string `json:"assumes,omitempty"`
	// Candidate is the country of the last important operation, the answer
	// (c)(5) would give if (c)(4) cannot name a country. It is shown, never
	// counted as a decision.
	Candidate string `json:"candidate,omitempty"`
}

// Reason classes.
const (
	ClassNoCode     = "no_code"
	ClassShortCode  = "code_too_short"
	ClassFact       = "fact_not_stated"
	ClassNoStage    = "operation_not_stated"
	ClassShift      = "needs_material_codes"
	ClassNotModeled = "not_read_by_code"
	ClassJudgment   = "judgment"
	ClassAmbiguous  = "ambiguous_row"
)

func norm(c string) string { return domain.NormalizeCountry(strings.TrimSpace(c)) }

func und(class, format string, a ...any) Result {
	return Result{Status: Undetermined, Class: class, Reason: fmt.Sprintf(format, a...)}
}

// InScope reports whether a code is a textile or apparel product under
// (b)(5): Chapters 50 through 63, or one of the listed codes. ok is false
// when the code is too short to tell.
func (r *Rules) InScope(code string) (in, ok bool) {
	d := Digits(code)
	if len(d) < 4 {
		return false, false
	}
	if lo, hi := "5000000000", "6399999999"; relate(d, lo, hi) == inside {
		return true, true
	}
	partialSeen := false
	for _, s := range r.T.Scope {
		lo, hi, ok := rangeOf(s)
		if !ok {
			continue
		}
		switch relate(d, lo, hi) {
		case inside:
			return true, true
		case partial:
			partialSeen = true
		}
	}
	// A heading that overlaps the scope list in part (4202, 9619) cannot be told from four digits.
	return false, !partialSeen
}

// Evaluate applies 102.21(c)(1) to (c)(5) to a good.
func (r *Rules) Evaluate(f Facts) Result {
	d := Digits(f.GoodHTS)
	if len(d) < 4 {
		return und(ClassNoCode, "the good's HTS code is missing, so the rule that applies is not known")
	}
	if in, ok := r.InScope(d); !ok {
		return und(ClassShortCode, "the good's code %s is too short to tell whether it is a textile or apparel product", f.GoodHTS)
	} else if !in {
		return Result{Status: OutOfScope, Reason: "not a textile or apparel product under 102.21(b)(5)"}
	}
	countries := f.manufacturingCountries()
	oneCountry := f.Complete && len(countries) == 1
	// (c)(1): wholly obtained or produced in one country. It comes before the
	// specific rule, so packing elsewhere does not take it away.
	if oneCountry && f.RawMaterialCountry != "" && countries[0] == norm(f.RawMaterialCountry) {
		return Result{Status: Decided, Origin: countries[0], Para: "(c)(1)",
			Basis: "19 CFR 102.21(c)(1): the raw material and every operation are in " + countries[0]}
	}
	// Every operation in one country: (c)(2), (c)(3) and (c)(4) all name it.
	if oneCountry && !r.packingDecides(d, f, countries[0]) {
		return Result{Status: Decided, Origin: countries[0], Para: "single country",
			Basis: "19 CFR 102.21(c): every operation took place in " + countries[0] + ", so (c)(2), (c)(3) and (c)(4) name the same country"}
	}
	// (c)(2): the specific rule of (e).
	res, skip3 := r.c2(d, f)
	if res != nil {
		return *res
	}
	if !skip3 {
		if res := r.c3(d, f); res != nil {
			return *res
		}
	}
	return r.c4(f, countries)
}

// packingDecides reports whether the good's rule is the packaging rule of
// headings 6309 and 6310 and the facts do not put the packing in c alone: the
// rule names the packing country, which is not a country of manufacture, so
// the single-country shortcut must not answer. With no packing stage stated
// the rule cannot name a country and the check asks.
func (r *Rules) packingDecides(d string, f Facts, c string) bool {
	for _, row := range r.T.Rows {
		if relate(d, row.Lo, row.Hi) != inside {
			continue
		}
		for _, a := range row.Alts {
			if a.Req.Process == ProcPackaging {
				pc := f.countriesOf(StagePacking)
				if len(pc) == 0 {
					return true
				}
				for _, p := range pc {
					if p != c {
						return true
					}
				}
			}
		}
	}
	return false
}

func (f Facts) manufacturingCountries() []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range f.Stages {
		if s.Process == StagePacking || strings.TrimSpace(s.Country) == "" {
			continue
		}
		c := norm(s.Country)
		if !seen[c] {
			seen[c] = true
			out = append(out, c)
		}
	}
	return out
}

func (f Facts) countriesOf(process string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range f.Stages {
		if s.Process == process && strings.TrimSpace(s.Country) != "" {
			if c := norm(s.Country); !seen[c] {
				seen[c] = true
				out = append(out, c)
			}
		}
	}
	return out
}

// c2 is paragraph (c)(2). It returns a result when the paragraph decides or
// the answer is undetermined; nil when (c)(2) names no country and the
// hierarchy goes on. skip3 is set when the row itself sends the good to (c)(4).
func (r *Rules) c2(d string, f Facts) (res *Result, skip3 bool) {
	var inside_, partial_ []int
	for i, row := range r.T.Rows {
		switch relate(d, row.Lo, row.Hi) {
		case inside:
			inside_ = append(inside_, i)
		case partial:
			partial_ = append(partial_, i)
		}
	}
	// A row that sits inside another (9619.00.31-33 inside 9619) is the more
	// specific rule and is the one that applies.
	inside_ = narrowest(r.T.Rows, inside_)
	switch {
	case len(inside_) > 1:
		x := und(ClassAmbiguous, "more than one 102.21(e) row covers %s", f.GoodHTS)
		return &x, false
	case len(inside_) == 0 && len(partial_) > 0:
		x := und(ClassShortCode, "the code %s is too short to pick one 102.21(e) row (%s)", f.GoodHTS, r.T.Rows[partial_[0]].Code)
		return &x, false
	case len(inside_) == 0:
		return nil, false // no specific rule: (c)(2) gives no country
	}
	row := r.T.Rows[inside_[0]]
	alts, label := row.Alts, row.Code
	switch r.e2Applies(d, f) {
	case Unknown:
		x := und(ClassFact, "102.21(e)(2) takes this code over unless the good is of cotton or wool or a blend of 16 percent or more cotton, which is not stated, or the code is too short")
		return &x, false
	case Yes:
		alts, label = r.T.E2.Alts, "(e)(2)"
	}
	out := r.evalAlts(d, f, row, alts)
	switch out.kind {
	case outCountry:
		x := Result{Status: Decided, Origin: out.country, Para: "(c)(2)", Row: label, Alt: out.alt,
			Basis: fmt.Sprintf("19 CFR 102.21(c)(2), (e) row %s, alternative %s: %s", label, out.alt, out.why)}
		if missing := f.foreignWithoutCode(out.country); missing {
			x.Assumes = []string{"foreign materials whose HTS codes are not given are taken to change as the rule requires"}
		}
		return &x, false
	case outUnknown:
		x := und(out.class, "%s", out.why)
		x.Row, x.Alt = label, out.alt
		return &x, false
	case outToC4:
		return nil, true
	}
	return nil, false
}

// e2Applies is Yes when paragraph (e)(2) takes the good over from table 1.
func (r *Rules) e2Applies(d string, f Facts) Tri {
	in := No
	for _, c := range r.T.E2.Codes {
		lo, hi, ok := rangeOf(c)
		if !ok {
			continue
		}
		switch relate(d, lo, hi) {
		case inside:
			in = Yes
		case partial:
			if in != Yes {
				in = Unknown
			}
		}
	}
	switch in {
	case No:
		return No
	case Unknown:
		return Unknown
	}
	switch f.CottonWool16 {
	case No:
		return Yes
	case Yes:
		return No
	}
	return Unknown
}

type outKind int

const (
	outNone outKind = iota // no alternative applies or none gives a country
	outCountry
	outUnknown
	outToC4
)

type altOutcome struct {
	kind    outKind
	country string
	alt     string
	why     string
	class   string
}

type altState struct {
	applies Tri // whether the conditions hold
	country string
	failed  bool // applies, but the process does not give one country
	toC4    bool
	why     string
	class   string
}

func (r *Rules) evalAlts(d string, f Facts, row Row, alts []Alt) altOutcome {
	st := map[string]altState{}
	var firstUnknown *altOutcome
	var toC4 bool
	for _, a := range alts {
		if a.After != "" {
			if prev, ok := st[a.After]; ok {
				if prev.country != "" {
					st[a.ID] = altState{applies: No}
					continue
				}
				if prev.applies == Unknown {
					s := altState{applies: Unknown, why: prev.why, class: prev.class}
					st[a.ID] = s
					if firstUnknown == nil {
						firstUnknown = &altOutcome{kind: outUnknown, alt: a.ID, why: s.why, class: s.class}
					}
					continue
				}
			}
		}
		s := r.evalAlt(d, f, row, a)
		st[a.ID] = s
		switch {
		case s.applies == Yes && s.country != "":
			return altOutcome{kind: outCountry, country: s.country, alt: a.ID, why: s.why}
		case s.applies == Unknown:
			if firstUnknown == nil {
				firstUnknown = &altOutcome{kind: outUnknown, alt: a.ID, why: s.why, class: s.class}
			}
		case s.applies == Yes && s.toC4:
			toC4 = true
		}
	}
	if firstUnknown != nil {
		return *firstUnknown
	}
	if toC4 {
		return altOutcome{kind: outToC4}
	}
	return altOutcome{kind: outNone}
}

func (r *Rules) evalAlt(d string, f Facts, row Row, a Alt) altState {
	applies := Yes
	var why string
	for _, c := range a.Conds {
		m, w := r.cond(d, f, c)
		switch m {
		case No:
			return altState{applies: No}
		case Unknown:
			applies = Unknown
			if why == "" {
				why = w
			}
		}
	}
	if applies == Unknown {
		return altState{applies: Unknown, why: why, class: classOf(a.Conds, f)}
	}
	switch a.Req.Kind {
	case ReqToC4:
		return altState{applies: Yes, toC4: true}
	case ReqShift:
		return altState{applies: Unknown, class: ClassShift, why: "the rule is a tariff shift (" + a.Text + ") and needs the HTS codes of the foreign materials"}
	case ReqOther:
		return altState{applies: Unknown, class: ClassNotModeled, why: "the rule is not read by code: " + a.Text}
	}
	c, status, why := f.process(a.Req.Process, a.ID)
	switch status {
	case Yes:
		if a.Req.Last {
			if v := f.laterElsewhere(a.Req.Process, c); v != "" {
				return altState{applies: Yes, failed: true, why: v}
			}
		}
		if v := f.vetoShift(row, a, d, c); v != "" {
			return altState{applies: Yes, failed: true, why: v}
		}
		return altState{applies: Yes, country: c, why: why}
	case No:
		return altState{applies: Yes, failed: true, why: why}
	}
	return altState{applies: Unknown, class: ClassNoStage, why: why}
}

func classOf(cs []Cond, f Facts) string {
	for _, c := range cs {
		if c.Unmodeled != "" {
			return ClassNotModeled
		}
	}
	return ClassFact
}

// cond checks one condition: Yes when it holds, No when it does not,
// Unknown when a fact is not given.
func (r *Rules) cond(d string, f Facts, c Cond) (Tri, string) {
	if c.Unmodeled != "" {
		return Unknown, "the rule turns on a description the facts do not give (" + c.Unmodeled + ")"
	}
	want := func(have Tri, what string) (Tri, string) {
		if have == Unknown {
			return Unknown, what + " is not stated"
		}
		if (c.Want == "yes") == (have == Yes) {
			return Yes, ""
		}
		return No, ""
	}
	switch c.Fact {
	case FactKnitToShape:
		return want(f.KnitToShape, "whether the good is knit to shape")
	case FactComponents:
		return want(f.TwoOrMoreComponents, "whether the good consists of two or more component parts")
	case FactFabricOfWool:
		return want(f.FabricOfWool, "whether the fabric is of wool or fine animal hair")
	case FactFibre:
		if f.Fibre == "" {
			return Unknown, "whether the good is of filaments or staple fibres is not stated"
		}
		if f.Fibre == c.Want {
			return Yes, ""
		}
		return No, ""
	case FactNotE2:
		switch r.e2Applies(d, f) {
		case Yes:
			return No, ""
		case No:
			return Yes, ""
		}
		return Unknown, "whether (e)(2) applies is not known"
	case Fact6117_10:
		lo, hi, _ := rangeOf("6117.10")
		switch relate(d, lo, hi) {
		case inside:
			return Yes, ""
		case disjoint:
			return No, ""
		}
		return Unknown, "the subheading is too short to say whether it is 6117.10"
	case FactNot6117Other:
		lo, hi, _ := rangeOf("6117.10")
		switch relate(d, lo, hi) {
		case disjoint:
			return Yes, ""
		case partial:
			return Unknown, "the subheading is too short to say whether it is 6117.10"
		}
		switch {
		case f.KnitToShape == Yes || f.TwoOrMoreComponents == Yes:
			return No, ""
		case f.KnitToShape == No && f.TwoOrMoreComponents == No:
			return Yes, ""
		}
		return Unknown, "whether the 6117.10 good is knit to shape or has two or more parts is not stated"
	}
	return Unknown, "unknown condition " + c.Fact
}

// laterElsewhere says whether a manufacturing operation after the last stage
// of process p took place outside c: then the filaments or fibres of p did
// not become the good by p alone.
func (f Facts) laterElsewhere(p, c string) string {
	last := -1
	for i, s := range f.Stages {
		if s.Process == p {
			last = i
		}
	}
	for _, s := range f.Stages[last+1:] {
		if s.Process == StagePacking || s.Process == StageCutting || strings.TrimSpace(s.Country) == "" {
			continue
		}
		if norm(s.Country) != c {
			return fmt.Sprintf("a later operation (%s) took place in %s, so the change of those filaments or fibres to the good is not the result of %s alone", s.Process, norm(s.Country), label(p))
		}
	}
	return ""
}

// vetoShift applies the alternative's tariff shift to the foreign materials
// whose codes the text gives: one in a heading the clause excepts (or inside
// the group the good is in) did not change, so the rule is not met in country
// c. A material is foreign when it does not come from c or its origin is not
// stated. "" when none is vetoed.
func (f Facts) vetoShift(row Row, a Alt, good, c string) string {
	if a.Req.Shift == nil {
		return ""
	}
	for _, m := range f.Materials {
		if strings.TrimSpace(m.HTS) == "" {
			continue
		}
		if m.Country != "" && norm(m.Country) == c {
			continue
		}
		if ShiftAllows(row, a, good, m.HTS) == No {
			return fmt.Sprintf("the foreign material %s (%s) does not change under the tariff shift of the rule, so it is not met in %s", m.Name, m.HTS, c)
		}
	}
	return ""
}

// foreignWithoutCode reports whether the facts leave a foreign material
// without a code (or name none), so the tariff shift was not tested for it.
func (f Facts) foreignWithoutCode(c string) bool {
	if len(f.Materials) == 0 {
		return true
	}
	for _, m := range f.Materials {
		if (m.Country == "" || norm(m.Country) != c) && strings.TrimSpace(m.HTS) == "" {
			return true
		}
	}
	return false
}

var finishOps = map[string]bool{"bleaching": true, "shrinking": true, "fulling": true, "napping": true, "decating": true,
	"permanent stiffening": true, "weighting": true, "permanent embossing": true, "moireing": true}

// process finds the country of an operation. status is Yes with the
// country, No when the operation is stated and does not give one country,
// Unknown when the facts do not say.
func (f Facts) process(p, alt string) (country string, status Tri, why string) {
	switch p {
	case ProcAssembly:
		if f.TwoOrMoreComponents == No {
			return "", No, "wholly assembled needs two or more components"
		}
		if f.TwoOrMoreComponents == Unknown {
			return "", Unknown, "whether the good consists of two or more component parts is not stated"
		}
		cs := f.countriesOf(ProcAssembly)
		return single(cs, "wholly assembled", "was wholly assembled in", "the assembly is not stated")
	case ProcDyePrint:
		return f.dyePrint()
	default:
		// The facts name the packing stage StagePacking; the rule's requirement is ProcPackaging.
		stage := p
		if p == ProcPackaging {
			stage = StagePacking
		}
		cs := f.countriesOf(stage)
		return single(cs, label(p), "took place in", "where "+label(p)+" took place is not stated")
	}
}

func label(p string) string {
	return map[string]string{ProcSpinning: "spinning", ProcExtrusion: "extrusion", ProcFabricMaking: "fabric-making", ProcGarnetting: "garnetting",
		ProcKnitToShape: "knitting to shape", ProcEmbroidery: "embroidery", ProcPackaging: "packaging"}[p]
}

func single(cs []string, what, verb, missing string) (string, Tri, string) {
	switch len(cs) {
	case 0:
		return "", Unknown, missing
	case 1:
		return cs[0], Yes, fmt.Sprintf("%s %s %s", what, verb, cs[0])
	}
	sort.Strings(cs)
	return "", No, fmt.Sprintf("%s took place in more than one country (%s)", what, strings.Join(cs, ", "))
}

// dyePrint: both dyeing and printing, with two or more finishing operations,
// in one country.
func (f Facts) dyePrint() (string, Tri, string) {
	dye, pr := f.countriesOf(StageDyeing), f.countriesOf(StagePrint)
	if len(dye) == 0 || len(pr) == 0 {
		if f.Complete {
			return "", No, "the fabric was not both dyed and printed"
		}
		return "", Unknown, "whether the fabric was both dyed and printed is not stated"
	}
	for _, c := range dye {
		for _, p := range pr {
			if c != p {
				continue
			}
			ops := map[string]bool{}
			for _, s := range f.Stages {
				if s.Process == StageFinish && norm(s.Country) == c && finishOps[strings.ToLower(s.Op)] {
					ops[strings.ToLower(s.Op)] = true
				}
			}
			if len(ops) >= 2 {
				return c, Yes, fmt.Sprintf("the fabric was dyed and printed in %s with %d finishing operations", c, len(ops))
			}
			if f.Complete {
				return "", No, "dyeing and printing were not accompanied by two finishing operations"
			}
			return "", Unknown, "the finishing operations that accompany dyeing and printing are not stated"
		}
	}
	return "", No, "dyeing and printing were not in the same country"
}

// c3 is paragraph (c)(3): goods knit to shape, then goods wholly assembled.
func (r *Rules) c3(d string, f Facts) *Result {
	switch f.KnitToShape {
	case Unknown:
		x := und(ClassFact, "no (e) rule gives a country and 102.21(c)(3) turns on whether the good is knit to shape, which is not stated")
		return &x
	case Yes:
		cs := f.countriesOf(ProcKnitToShape)
		if len(cs) == 1 {
			return &Result{Status: Decided, Origin: cs[0], Para: "(c)(3)(i)", Basis: "19 CFR 102.21(c)(3)(i): the good was knit to shape in " + cs[0]}
		}
		return nil
	}
	// not knit to shape: (c)(3)(ii), unless the good is one of those it excludes.
	for _, e := range r.T.Excl3ii {
		lo, hi, ok := rangeOf(e)
		if ok && relate(d, lo, hi) == inside {
			return nil
		}
		if ok && relate(d, lo, hi) == partial {
			x := und(ClassShortCode, "the code is too short to tell whether 102.21(c)(3)(ii) excludes the good (%s)", e)
			return &x
		}
	}
	switch f.TwoOrMoreComponents {
	case Unknown:
		x := und(ClassFact, "102.21(c)(3)(ii) needs two or more components, which is not stated")
		return &x
	case No:
		return nil
	}
	cs := f.countriesOf(ProcAssembly)
	switch len(cs) {
	case 1:
		return &Result{Status: Decided, Origin: cs[0], Para: "(c)(3)(ii)", Basis: "19 CFR 102.21(c)(3)(ii): the good was wholly assembled in " + cs[0]}
	case 0:
		if !f.Complete {
			x := und(ClassNoStage, "where the good was assembled is not stated")
			return &x
		}
	}
	return nil
}

var important = map[string]bool{ProcSpinning: true, ProcExtrusion: true, ProcFabricMaking: true, ProcKnitToShape: true, ProcAssembly: true, ProcEmbroidery: true}

// c4 is paragraph (c)(4). Its "most important" process is a judgment, so the
// check does not decide it; Candidate is what (c)(5) would give.
func (r *Rules) c4(f Facts, countries []string) Result {
	x := und(ClassJudgment, "102.21(c)(4) asks for the country of the most important assembly or manufacturing process, which is a judgment")
	for i := len(f.Stages) - 1; i >= 0; i-- {
		if important[f.Stages[i].Process] && strings.TrimSpace(f.Stages[i].Country) != "" {
			x.Candidate = norm(f.Stages[i].Country)
			break
		}
	}
	return x
}

// narrowest keeps the rows that no other of them lies inside.
func narrowest(rows []Row, idx []int) []int {
	var out []int
	for _, i := range idx {
		nested := false
		for _, j := range idx {
			if i != j && rows[i].Lo <= rows[j].Lo && rows[j].Hi <= rows[i].Hi && (rows[i].Lo != rows[j].Lo || rows[i].Hi != rows[j].Hi) {
				nested = true // row j lies inside row i
			}
		}
		if !nested {
			out = append(out, i)
		}
	}
	return out
}
