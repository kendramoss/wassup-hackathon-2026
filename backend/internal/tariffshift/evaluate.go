package tariffshift

import (
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
)

// Material is one input to the good: a part or raw material, its HTS code
// and where it was made.
type Material struct {
	Name   string `json:"name"`
	HTS    string `json:"hts,omitempty"`
	Origin string `json:"origin"`
	// NotIn is a code the material is known to be classified outside of,
	// when its own code is not given (CBP often writes "the foreign
	// materials are classified outside heading 8473"). It decides only
	// tests that need no more than that.
	NotIn string `json:"not_in,omitempty"`
	// ValueShare is the material's value as a fraction of the good's value
	// (0 to 1), for the 102.13 de minimis rule. Optional.
	ValueShare *float64 `json:"value_share,omitempty"`
}

// Input is one question: a good of GoodHTS produced in Country from
// Materials. The optional facts settle the conditions some rules carry; nil
// means not known, and a rule that turns on an unknown fact is not decided.
type Input struct {
	GoodHTS   string     `json:"good_hts"`
	Country   string     `json:"country"`
	Materials []Material `json:"materials"`
	// SimpleAssembly: the production in Country is a simple assembly
	// (102.1(o): putting parts together with no more than minor processing).
	SimpleAssembly *bool `json:"simple_assembly,omitempty"`
	// GRI2a: the change comes from assembling parts into an unfinished good
	// classified as the finished good under GRI 2(a) (102.18(a)).
	GRI2a *bool `json:"gri_2a,omitempty"`
	// BendingToShape: the production bends the steel tubes to shape (the
	// heading 7303-7306 exceptions in chapter 84).
	BendingToShape *bool `json:"bending_to_shape,omitempty"`
}

// Statuses of a Result.
const (
	Decided      = "decided"
	Undetermined = "undetermined"
	OutOfScope   = "out_of_scope"
)

// Statuses of a MaterialResult.
const (
	MatShift      = "shift"       // undergoes the shift
	MatNoShift    = "no_shift"    // does not
	MatNeedsFact  = "needs_fact"  // depends on a fact the input does not give
	MatMissingHTS = "missing_hts" // no HTS code
)

// MaterialResult is the rule's answer for one material.
type MaterialResult struct {
	Name    string `json:"name"`
	HTS     string `json:"hts,omitempty"`
	Origin  string `json:"origin"`
	Foreign bool   `json:"foreign"`
	// Status is for the shift test of this material (also computed for
	// domestic materials, since 102.18(b) looks at every material).
	Status string `json:"status"`
	// Alternative is the 1-based alternative of the rule that the material
	// satisfies (Status shift).
	Alternative int `json:"alternative,omitempty"`
	// Needs lists what would decide a needs_fact material.
	Needs []string `json:"needs,omitempty"`
}

// RuleRef is the rule applied, for display.
type RuleRef struct {
	ID           string `json:"id"`
	HTS          string `json:"hts"`
	Text         string `json:"text"`
	Alternatives int    `json:"alternatives"`
	Encoded      int    `json:"encoded"`
}

// Result is the rule layer's answer. It never guesses: when a step needs a
// fact or a judgment the input does not give, Status is undetermined and
// Reason says what is missing.
type Result struct {
	Status string `json:"status"`
	// Origin is the country of origin when decided.
	Origin string `json:"origin,omitempty"`
	// Basis cites the paragraphs that decided it.
	Basis     string           `json:"basis,omitempty"`
	Reason    string           `json:"reason"`
	Rule      *RuleRef         `json:"rule,omitempty"`
	Materials []MaterialResult `json:"materials,omitempty"`
	// IfNotDeMinimis is the answer when the only open question is the
	// 102.13 de minimis value test: the origin if the materials that fail
	// the shift are worth more than 7% of the good.
	IfNotDeMinimis *Conditional `json:"if_not_de_minimis,omitempty"`
	// Assumes lists what a decided answer takes for granted.
	Assumes []string `json:"assumes,omitempty"`
}

// Conditional is an answer that holds under a stated condition.
type Conditional struct {
	Origin    string `json:"origin"`
	Basis     string `json:"basis"`
	Condition string `json:"condition"`
}

// Rules holds the loaded table.
type Rules struct{ t Table }

// New returns rules over a table.
func New(t Table) *Rules { return &Rules{t: t} }

// Table returns the loaded table.
func (r *Rules) Table() Table { return r.t }

// InScope says whether a chapter is encoded.
func (r *Rules) InScope(code string) bool {
	return len(code) >= 2 && slices.Contains(r.t.Chapters, code[:2])
}

// Lookup returns the rule for a good's HTS code. ok is false with a reason
// when the code is out of scope, too short to pick one rule, or not found.
func (r *Rules) Lookup(hts string) (Rule, string, bool) {
	g := Digits(hts)
	switch {
	case g == "":
		return Rule{}, "the good's HTS code is missing or not a code", false
	case !r.InScope(g):
		return Rule{}, fmt.Sprintf("chapter %s is not in the encoded chapters (%s)", g[:2], strings.Join(r.t.Chapters, ", ")), false
	}
	var hit []Rule
	partial := false
	for _, rule := range r.t.Rules {
		switch rule.Applies.contains(g) {
		case yes:
			hit = append(hit, rule)
		case maybe:
			partial = true
		}
	}
	switch {
	case len(hit) == 1 && !partial:
		return hit[0], "", true
	case len(hit) > 1:
		return Rule{}, fmt.Sprintf("more than one rule covers %s", format(g)), false
	case partial:
		return Rule{}, fmt.Sprintf("%s is too short to pick one rule; give the 6-digit subheading", format(g)), false
	}
	return Rule{}, fmt.Sprintf("no 102.20 rule found for %s", format(g)), false
}

// deMinimisExcluded are the chapters 102.13(b) takes out of de minimis.
var deMinimisExcluded = []string{"01", "02", "03", "04", "07", "08", "11", "12", "15", "17", "20"}

// Evaluate applies 102.11(a) to (b) with the 102.20 rule for the good.
func (r *Rules) Evaluate(in Input) Result {
	country := domain.NormalizeCountry(in.Country)
	if country == "" {
		return Result{Status: Undetermined, Reason: "the country where the good is produced is not given"}
	}
	rule, why, ok := r.Lookup(in.GoodHTS)
	if !ok {
		status := Undetermined
		if g := Digits(in.GoodHTS); g != "" && !r.InScope(g) {
			status = OutOfScope
		}
		return Result{Status: status, Reason: why}
	}
	res := Result{Rule: ref(rule)}
	if len(in.Materials) == 0 {
		res.Status, res.Reason = Undetermined, "no materials given; the tariff-shift rule tests each foreign material"
		return res
	}
	g := Digits(in.GoodHTS)
	foreign := 0
	for _, m := range in.Materials {
		o := domain.NormalizeCountry(m.Origin)
		// A material of unknown origin is treated as foreign: if it shifts,
		// its origin does not matter; if it does not, the answer stays open.
		mr := MaterialResult{Name: m.Name, HTS: m.HTS, Origin: o, Foreign: o != country}
		if mr.Foreign {
			foreign++
		}
		mr.Status, mr.Alternative, mr.Needs = r.material(rule, g, mcode{code: Digits(m.HTS), notIn: Digits(m.NotIn)}, in)
		res.Materials = append(res.Materials, mr)
	}
	if foreign == 0 {
		res.Status, res.Origin = Decided, country
		res.Basis = "19 CFR 102.11(a)(2)"
		res.Reason = fmt.Sprintf("every material is from %s, so the good is produced exclusively from domestic materials", country)
		return res
	}

	var fail, open, noOrigin []int
	for i, mr := range res.Materials {
		if !mr.Foreign {
			continue
		}
		switch mr.Status {
		case MatNoShift:
			fail = append(fail, i)
			if mr.Origin == "" {
				noOrigin = append(noOrigin, i)
			}
		case MatNeedsFact, MatMissingHTS:
			open = append(open, i)
		}
	}
	a3 := []string{"the work in " + country + " is more than the operations 102.17 says do not count (for example simple packing, dilution or disassembly)"}
	if len(fail) == 0 && len(open) == 0 {
		res.Status, res.Origin = Decided, country
		res.Basis = "19 CFR 102.11(a)(3); " + rule.ID
		res.Reason = fmt.Sprintf("each foreign material undergoes the change in tariff classification the rule requires, in %s", country)
		res.Assumes = a3
		return res
	}
	if len(open) > 0 {
		res.Status = Undetermined
		res.Reason = "the shift cannot be decided for " + names(res.Materials, open) + "; see what each needs"
		return res
	}
	if len(noOrigin) > 0 {
		res.Status = Undetermined
		res.Reason = names(res.Materials, noOrigin) + " do not shift and their origin is not given"
		return res
	}

	// Some foreign materials fail the shift. 102.13: they are disregarded
	// if worth no more than 7% of the good (10% in chapter 22).
	chapter := g[:2]
	deMinimisOpen := false
	if !slices.Contains(deMinimisExcluded, chapter) {
		limit := 0.07
		if chapter == "22" {
			limit = 0.10
		}
		sum, known := 0.0, true
		for _, i := range fail {
			v := in.Materials[i].ValueShare
			if v == nil {
				known = false
				break
			}
			sum += *v
		}
		switch {
		case known && sum <= limit:
			res.Status, res.Origin = Decided, country
			res.Basis = "19 CFR 102.13(a); 19 CFR 102.11(a)(3); " + rule.ID
			res.Reason = fmt.Sprintf("the materials that do not shift (%s) are %.1f%% of the good's value, within the %.0f%% de minimis limit", names(res.Materials, fail), sum*100, limit*100)
			res.Assumes = a3
			return res
		case !known:
			deMinimisOpen = true
		}
	}

	if rule.FallbackNote {
		res.Status = Undetermined
		res.Reason = names(res.Materials, fail) + " do not shift, and a chapter note gives its own origin test for this rule; a person applies it"
		return res
	}

	// 102.11(b) with 102.18(b)(1): only materials classified where the rule
	// allows no change can impart the essential character. That set is
	// known only if every material, domestic too, has a decided status.
	var cand, unknown []int
	for i, mr := range res.Materials {
		switch mr.Status {
		case MatNoShift:
			cand = append(cand, i)
		case MatNeedsFact, MatMissingHTS:
			unknown = append(unknown, i)
		}
	}
	var b *Conditional
	bReason := ""
	switch {
	case len(unknown) > 0:
		bReason = "the materials that may impart the essential character (102.18(b)(1)) are not all known: " + names(res.Materials, unknown) + " cannot be placed"
	case len(cand) == 1:
		m := res.Materials[cand[0]]
		b = &Conditional{Origin: m.Origin, Basis: "19 CFR 102.11(b)(1); 19 CFR 102.18(b)(1)(iii); " + rule.ID}
		bReason = fmt.Sprintf("%s is the only material classified where the rule allows no change, so it imparts the essential character (102.18(b)(1)(iii))", m.Name)
	default:
		origins := map[string]bool{}
		for _, i := range cand {
			origins[res.Materials[i].Origin] = true
		}
		if len(origins) == 1 {
			o := res.Materials[cand[0]].Origin
			b = &Conditional{Origin: o, Basis: "19 CFR 102.11(b)(1); 19 CFR 102.18(b)(1); " + rule.ID}
			bReason = fmt.Sprintf("every material that can impart the essential character (%s) is from %s", names(res.Materials, cand), o)
		} else {
			bReason = "the essential character (102.11(b)) must be judged between " + names(res.Materials, cand) + ", which come from different countries"
		}
	}
	bAssumes := []string{"the good is not a set or mixture (102.11(b) does not apply to sets)"}
	if deMinimisOpen {
		res.Status = Undetermined
		res.Reason = fmt.Sprintf("%s do not shift; if they are worth no more than 7%% of the good, the origin is %s (102.13); their value is not given", names(res.Materials, fail), country)
		if b != nil {
			b.Condition = "if the materials that do not shift are worth more than 7% of the good: " + bReason
			res.IfNotDeMinimis = b
		} else {
			res.Reason += "; otherwise " + bReason
		}
		return res
	}
	if b == nil {
		res.Status, res.Reason = Undetermined, names(res.Materials, fail)+" do not shift, so 102.11(a) does not decide; "+bReason
		return res
	}
	res.Status, res.Origin, res.Basis = Decided, b.Origin, b.Basis
	res.Reason = names(res.Materials, fail) + " do not shift, so 102.11(a) does not decide; " + bReason
	res.Assumes = bAssumes
	return res
}

func ref(r Rule) *RuleRef {
	enc := 0
	for _, a := range r.Alternatives {
		if a.Encoded {
			enc++
		}
	}
	return &RuleRef{ID: r.ID, HTS: r.HTS, Text: r.Text, Alternatives: len(r.Alternatives), Encoded: enc}
}

func names(ms []MaterialResult, idx []int) string {
	var out []string
	for _, i := range idx {
		out = append(out, ms[i].Name)
	}
	return strings.Join(out, ", ")
}

// material tests one material against every alternative of the rule.
func (r *Rules) material(rule Rule, g string, m mcode, in Input) (string, int, []string) {
	if m.code == "" && m.notIn == "" {
		return MatMissingHTS, 0, []string{"the material's HTS code"}
	}
	best, alt := no, 0
	needs := map[string]bool{}
	for i, a := range rule.Alternatives {
		n := map[string]bool{}
		t := alternative(a, g, m, in, n)
		if t == yes && best != yes {
			best, alt = yes, i+1
		}
		if t == maybe {
			best = or(best, maybe)
			for k := range n {
				needs[k] = true
			}
		}
	}
	switch best {
	case yes:
		return MatShift, alt, nil
	case no:
		return MatNoShift, 0, nil
	}
	var list []string
	for k := range needs {
		list = append(list, k)
	}
	sort.Strings(list)
	return MatNeedsFact, 0, list
}

func fact(b *bool, name string, needs map[string]bool) tri {
	if b == nil {
		needs[name] = true
		return maybe
	}
	if *b {
		return yes
	}
	return no
}

const (
	needSimple   = "whether the work is a simple assembly"
	needGRI      = "whether the change is pursuant to GRI 2(a)"
	needBending  = "whether the tubes are bent to shape"
	needDescribe = "whether the goods match a description in the rule"
	needCode     = "a longer HTS code"
	needOwnCode  = "the material's own HTS code (only a code it is outside is known)"
	needPerson   = "a person to read an alternative written by goods description"
)

// alternative evaluates one alternative for good g and material m.
func alternative(a Alternative, g string, m mcode, in Input, needs map[string]bool) tri {
	applies := yes
	if len(a.To) > 0 {
		applies = anyContains(a.To, g)
		if applies == maybe {
			needs[needCode] = true
		}
	}
	if applies == no {
		return no
	}
	if !a.Encoded {
		needs[needPerson] = true
		return maybe
	}
	pass := no
	if a.From > 0 {
		pass = fromClause(a, g, m)
	}
	for _, s := range a.Also {
		c := m.in(s.From)
		if s.Described {
			c = and(c, describe(needs))
		}
		switch s.Condition {
		case CondNotSimpleAssembly:
			c = and(c, fact(in.SimpleAssembly, needSimple, needs).not())
		case CondNotGRI2a:
			c = and(c, fact(in.GRI2a, needGRI, needs).not())
		}
		pass = or(pass, c)
	}
	exc := no
	for _, e := range a.Except {
		c := m.in(e.From)
		if len(e.To) > 0 {
			c = and(c, anyContains(e.To, g))
		}
		if c == no {
			continue
		}
		if e.Described {
			c = and(c, describe(needs))
		}
		switch e.Condition {
		case CondSimpleAssembly:
			c = and(c, fact(in.SimpleAssembly, needSimple, needs))
		case CondUnlessBending:
			c = and(c, fact(in.BendingToShape, needBending, needs).not())
		case CondGRI2a:
			c = and(c, fact(in.GRI2a, needGRI, needs))
		}
		exc = or(exc, c)
	}
	t := and(pass, exc.not())
	if t != no && a.NotSimpleAssembly {
		t = and(t, fact(in.SimpleAssembly, needSimple, needs).not())
	}
	if t != no && a.NotGRI2a {
		t = and(t, fact(in.GRI2a, needGRI, needs).not())
	}
	t = and(t, applies)
	if t == maybe {
		switch {
		case m.code == "":
			needs[needOwnCode] = true
		case len(m.code) < 6 || len(g) < 6:
			needs[needCode] = true
		}
	}
	return t
}

func describe(needs map[string]bool) tri {
	needs[needDescribe] = true
	return maybe
}

// fromClause is the "from any other chapter/heading/subheading" test.
func fromClause(a Alternative, g string, m mcode) tri {
	L := a.From
	var to Range
	if len(a.To) > 0 {
		to = a.To[0].truncate(L)
	} else {
		to = Range{Lo: g[:min(L, len(g))], Hi: g[:min(L, len(g))]}
	}
	switch a.Group {
	case "outside":
		return m.in([]Range{to}).not()
	case "within":
		return and(m.in([]Range{to}), m.differ(g, L))
	}
	return m.differ(g, L)
}

// mcode is a material's code, or only a code it is known to be outside.
type mcode struct{ code, notIn string }

// in says whether the material is in any of rs.
func (m mcode) in(rs []Range) tri {
	if m.code != "" {
		return anyContains(rs, m.code)
	}
	ex := Range{Lo: m.notIn, Hi: m.notIn}
	t := no
	for _, r := range rs {
		// A range wholly inside the excluded code cannot hold the material.
		if r.Level() >= ex.Level() && ex.contains(r.Lo) == yes && ex.contains(r.Hi) == yes {
			continue
		}
		t = maybe
	}
	return t
}

// differ says whether the material's code differs from g in the first n
// digits.
func (m mcode) differ(g string, n int) tri {
	if m.code != "" {
		return differ(m.code, g, n)
	}
	if len(m.notIn) <= n && len(g) >= len(m.notIn) && g[:len(m.notIn)] == m.notIn {
		return yes
	}
	return maybe
}
