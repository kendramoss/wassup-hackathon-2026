package nporigin

import (
	"fmt"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/tariffshift"
)

// General is how one market's law frames its list: the citations an answer
// gives and what happens to goods the list does not cover.
type General struct {
	Market string
	// ListName names the list in reasons ("Annex 22-01", "Part Four").
	ListName string
	// RowRef is prefixed to a row's code in a basis.
	RowRef string
	// Listed cites the provision that makes the list rules decide origin.
	Listed string
	// Residual, Mixture, Tolerance and ChapterAlt cite the chapter residual
	// rule, the residual rule for mixtures, the 10% tolerance and the
	// chapter primary rules note.
	Residual, Mixture, Tolerance, ChapterAlt string
	// General cites the general rule for goods the list does not cover, and
	// Judgement says, as one sentence, that it needs a person.
	General   string
	Judgement string
	// MinimalFallback: for a good the list does not cover whose last work
	// is only minimal operations, the origin is where the major portion of
	// the materials originated (Art. 34 DA, last paragraph). False for a
	// market whose text gives no such rule; such an answer needs a person.
	MinimalFallback bool
	MinimalRef      string
	// MinimalJudgement is the reason when MinimalFallback is false.
	MinimalJudgement string
	// MinimalOps and Uneconomic are the assumptions of every decided answer;
	// MinimalOps has %s for the country.
	MinimalOps, Uneconomic string
	// Neutral is the assumption that packing and neutral elements (energy,
	// plant, tools) are left out of the materials.
	Neutral string
	// MaxValueNonOriginating: the market's text defines the value in "the
	// value of the materials used does not exceed X%" as the value of the
	// non-originating materials (UK Part Four, value terms). When false the
	// text does not say, and the rule is left to an answer.
	MaxValueNonOriginating bool
	// Administration says who applies the rules, for the answer's note.
	Administration string
}

// Rules is a loaded table with the market's framing.
type Rules struct {
	t    Table
	g    General
	keys map[string]bool
}

// New returns rules over a table.
func New(t Table, g General) *Rules {
	r := &Rules{t: t, g: g, keys: map[string]bool{}}
	for _, ch := range t.Chapters {
		if len(ch.ChapterRules) > 0 {
			r.keys[chapterRuleKey(ch.Chapter)] = true
		}
		if ch.Mixture != "" {
			r.keys[mixtureKey(ch.Chapter)] = true
		}
		for _, row := range ch.Rows {
			for _, a := range row.Alts {
				if a.Kind == KindProcess || (a.Kind == KindMaxValue && !g.MaxValueNonOriginating) {
					r.keys[a.Key] = true
				}
				if a.Condition != "" {
					r.keys[a.Key+".c"] = true
				}
				for i, e := range a.Except {
					if e.Condition != "" {
						r.keys[exceptKey(a.Key, i)] = true
					}
				}
			}
		}
	}
	return r
}

// Table returns the committed table.
func (r *Rules) Table() Table { return r.t }

// General returns the market's framing.
func (r *Rules) General() General { return r.g }

// ValidAnswer reports whether key is a yes/no fact some rule reads.
func (r *Rules) ValidAnswer(key string) bool { return r.keys[key] }

// ValidRow reports whether key names a row, or is "none".
func (r *Rules) ValidRow(key string) bool {
	if key == RowNone {
		return true
	}
	for _, ch := range r.t.Chapters {
		for _, row := range ch.Rows {
			if row.Key == key {
				return true
			}
		}
	}
	return false
}

func chapterRuleKey(ch string) string { return "ch" + ch + ".r" }
func mixtureKey(ch string) string     { return "ch" + ch + ".mix" }
func exceptKey(alt string, i int) string {
	return alt + ".x" + strconv.Itoa(i+1)
}

// RowNone is the row answer for "the good is none of the listed goods".
const RowNone = "none"

// Material is one input to the good.
type Material struct {
	Name   string `json:"name,omitempty"`
	HTS    string `json:"hts,omitempty"`
	Origin string `json:"origin,omitempty"`
	// ValueShare is the material's value as a share (0 to 1) of the good's
	// ex-works price; WeightShare of its weight. Optional.
	ValueShare  *float64 `json:"value_share,omitempty"`
	WeightShare *float64 `json:"weight_share,omitempty"`
}

// Input is one question: a good of GoodHTS made in Country from Materials.
type Input struct {
	GoodHTS   string     `json:"good_hts"`
	Country   string     `json:"country"`
	Materials []Material `json:"materials"`
	// Row picks the list row when the good's code reaches more than one, or
	// an "ex" row that covers only some goods of the code; RowNone says the
	// good is none of them.
	Row string `json:"row,omitempty"`
	// Answers are yes/no facts the rules read, by the keys a result's needs
	// name. A missing key is unknown; false is an answer.
	Answers map[string]bool `json:"answers,omitempty"`
	// MinimalOnly: the last work, in Country, was only minimal operations.
	MinimalOnly *bool `json:"minimal_operations_only,omitempty"`
}

// Statuses of a Result.
const (
	Decided      = "decided"
	Undetermined = "undetermined"
	// NeedsJudgement: no list rule covers the good, so its origin turns on
	// the general test, which a person has to apply.
	NeedsJudgement = "needs_judgement"
)

// Kinds of a Need.
const (
	NeedFact   = "fact"   // a yes/no answer, sent in Input.Answers
	NeedRow    = "row"    // pick a row, sent in Input.Row
	NeedCode   = "code"   // a code, country or origin to enter
	NeedShare  = "share"  // a value or weight share to enter
	NeedReview = "review" // a person's reading; no field answers it
)

// Option is one row the caller can pick.
type Option struct {
	Key         string `json:"key"`
	Code        string `json:"code"`
	Description string `json:"description"`
}

// Need is one thing that would move an undetermined answer.
type Need struct {
	Key  string `json:"key"`
	Kind string `json:"kind"`
	// Material is the 0-based index of the material the need is about.
	Material *int     `json:"material,omitempty"`
	Label    string   `json:"label"`
	Detail   string   `json:"detail,omitempty"`
	Options  []Option `json:"options,omitempty"`
}

// AltRef is one alternative of the applied row and how it came out.
type AltRef struct {
	Key  string `json:"key"`
	Text string `json:"text"`
	Kind string `json:"kind"`
	// Outcome is "met", "not_met" or "open".
	Outcome string `json:"outcome"`
}

// RowRef is the row applied, for display.
type RowRef struct {
	Key          string   `json:"key"`
	Code         string   `json:"code"`
	Description  string   `json:"description"`
	Text         string   `json:"text"`
	Alternatives []AltRef `json:"alternatives"`
	// ChapterRules and Notes are the chapter's primary rules and notes,
	// verbatim; Residual its residual rule.
	ChapterRules []string `json:"chapter_rules,omitempty"`
	Notes        []string `json:"notes,omitempty"`
	Residual     string   `json:"residual,omitempty"`
}

// MaterialResult is one material as the change rule read it.
type MaterialResult struct {
	Name   string `json:"name,omitempty"`
	HTS    string `json:"hts,omitempty"`
	Origin string `json:"origin,omitempty"`
	// Status: "originating" (made in the country of production), "changes",
	// "no_change", "open" or "missing_hts".
	Status string `json:"status"`
}

// Result is the answer. It never guesses: when a step needs a fact the
// input does not give, Status is undetermined and Needs says what.
type Result struct {
	Status string `json:"status"`
	Origin string `json:"origin,omitempty"`
	// Basis cites what decided it.
	Basis  string `json:"basis,omitempty"`
	Reason string `json:"reason"`
	// Chapter is the good's chapter; Covered whether the list has rules for
	// that chapter.
	Chapter   string           `json:"chapter,omitempty"`
	Covered   bool             `json:"covered"`
	Rule      *RowRef          `json:"rule,omitempty"`
	Materials []MaterialResult `json:"materials,omitempty"`
	Needs     []Need           `json:"needs,omitempty"`
	Assumes   []string         `json:"assumes,omitempty"`
}

type tri int

const (
	no tri = iota
	open
	yes
)

func (t tri) String() string { return [...]string{"not_met", "open", "met"}[t] }

func or(a, b tri) tri  { return max(a, b) }
func and(a, b tri) tri { return min(a, b) }

type state struct {
	r       *Rules
	in      Input
	country string
	good    string
	needs   []Need
	seen    map[string]bool
}

func (s *state) need(n Need) {
	k := n.Kind + "|" + n.Key
	if n.Material != nil {
		k += "|" + strconv.Itoa(*n.Material)
	}
	if s.seen[k] {
		return
	}
	s.seen[k] = true
	s.needs = append(s.needs, n)
}

func (s *state) answer(key, label, detail string) tri {
	if v, ok := s.in.Answers[key]; ok {
		if v {
			return yes
		}
		return no
	}
	s.need(Need{Key: key, Kind: NeedFact, Label: label, Detail: detail})
	return open
}

func idx(i int) *int { return &i }

// Evaluate answers one question. No model, no I/O.
func (r *Rules) Evaluate(in Input) Result {
	s := &state{r: r, in: in, country: domain.NormalizeCountry(in.Country), good: tariffshift.Digits(in.GoodHTS), seen: map[string]bool{}}
	res := s.evaluate()
	res.Needs = s.needs
	if res.Status != Undetermined {
		res.Needs = nil
	}
	return res
}

func (s *state) evaluate() Result {
	g := s.r.g
	if s.country == "" {
		s.need(Need{Key: "country", Kind: NeedCode, Label: "Country where the good is made"})
	}
	if len(s.good) < 4 {
		s.need(Need{Key: "good_code", Kind: NeedCode, Label: "HS code of the good (at least 4 digits)"})
	}
	if len(s.needs) > 0 {
		return Result{Status: Undetermined, Reason: "the country of production and the good's HS code are needed"}
	}
	res := Result{Chapter: s.good[:2]}
	ch, ok := s.chapter(res.Chapter)
	res.Covered = ok
	if !ok {
		return s.notListed(res, fmt.Sprintf("%s has no rules for chapter %s", g.ListName, res.Chapter))
	}
	row, ok, stop := s.pickRow(ch, &res)
	if stop {
		return res
	}
	if !ok {
		return s.notListed(res, fmt.Sprintf("%s has no rule for this good (chapter %s rules cover: %s)", g.ListName, ch.Chapter, rowCodes(ch)))
	}
	ref := &RowRef{Key: row.Key, Code: row.Code, Description: row.Description, Text: row.Text,
		ChapterRules: ch.ChapterRules, Notes: append(slicesClone(ch.Notes), nonEmpty(ch.Fallback)...), Residual: ch.ResidualText}
	res.Rule = ref
	if s.in.MinimalOnly != nil && *s.in.MinimalOnly {
		if !g.MinimalFallback {
			res.Status, res.Reason, res.Basis = NeedsJudgement, g.MinimalJudgement, g.MinimalRef
			return res
		}
		// Art. 34 DA: for goods covered by the list, the chapter residual
		// rules apply.
		return s.residual(res, ch, "the work was only minimal operations ("+g.MinimalRef+"), so the chapter residual rule applies")
	}
	if len(s.in.Materials) == 0 {
		s.need(Need{Key: "materials", Kind: NeedCode, Label: "The materials, each with its HS code and origin"})
		res.Status, res.Reason = Undetermined, "the rule reads the materials; none were given"
		return res
	}
	// The row's alternatives, then the chapter primary rules (note 2.6).
	overall := no
	var met *Alternative
	var mats []MaterialResult
	for i := range row.Alts {
		a := &row.Alts[i]
		t, ms := s.alt(*a)
		if mats == nil && ms != nil {
			mats = ms
		}
		ref.Alternatives = append(ref.Alternatives, AltRef{Key: a.Key, Text: a.Text, Kind: a.Kind, Outcome: t.String()})
		if t == yes && met == nil {
			met = a
		}
		overall = or(overall, t)
	}
	if len(ch.ChapterRules) > 0 && overall == no {
		t := s.answer(chapterRuleKey(ch.Chapter), "Does the chapter rule for goods made from blanks apply, and was it met in "+s.country+"?", strings.Join(ch.ChapterRules, "\n"))
		ref.Alternatives = append(ref.Alternatives, AltRef{Key: chapterRuleKey(ch.Chapter), Text: strings.Join(ch.ChapterRules, "\n"), Kind: KindProcess, Outcome: t.String()})
		if t == yes {
			met = &Alternative{Text: "chapter primary rule (" + g.ChapterAlt + ")"}
		}
		overall = or(overall, t)
	}
	res.Materials = mats
	switch overall {
	case yes:
		res.Status, res.Origin = Decided, s.country
		res.Basis = g.Listed + "; " + g.RowRef + row.Code
		res.Reason = fmt.Sprintf("the primary rule for %s is met in %s: %s", row.Code, s.country, met.Text)
		res.Assumes = s.assumes(ch)
		return res
	case open:
		res.Status = Undetermined
		res.Reason = fmt.Sprintf("the primary rule for %s needs facts the codes cannot show", row.Code)
		return res
	}
	// Not met where the good was made.
	if ch.Fallback != "" {
		s.need(Need{Key: "review", Kind: NeedReview, Label: "A chapter note names the origin", Detail: ch.Fallback})
		res.Status, res.Reason = Undetermined, "the primary rule is not met, and a chapter note then names the origin by facts the codes cannot show"
		return res
	}
	for _, a := range row.Alts {
		if a.Identifies {
			s.need(Need{Key: "review", Kind: NeedReview, Label: "The rule names another country", Detail: a.Text})
			res.Status, res.Reason = Undetermined, "the rule names the country of origin by where the work was done, and that was not "+s.country
			return res
		}
	}
	return s.residual(res, ch, fmt.Sprintf("the primary rule for %s is not met in %s", row.Code, s.country))
}

func (s *state) chapter(c string) (Chapter, bool) {
	for _, ch := range s.r.t.Chapters {
		if ch.Chapter == c {
			return ch, true
		}
	}
	return Chapter{}, false
}

func rowCodes(ch Chapter) string {
	var out []string
	for _, r := range ch.Rows {
		out = append(out, r.Code)
	}
	if len(out) > 8 {
		return strings.Join(out[:8], ", ") + fmt.Sprintf(" and %d more", len(out)-8)
	}
	return strings.Join(out, ", ")
}

func slicesClone(s []string) []string { return append([]string(nil), s...) }

func nonEmpty(s string) []string {
	if s == "" {
		return nil
	}
	return []string{s}
}

// contains says whether the good's code is in the range: yes, no, or open
// when the code is shorter than the range and overlaps it.
func contains(rg Range, code string) tri {
	n := len(rg.Lo)
	if len(code) >= n {
		if c := code[:n]; c >= rg.Lo && c <= rg.Hi {
			return yes
		}
		return no
	}
	lo := code + strings.Repeat("0", n-len(code))
	hi := code + strings.Repeat("9", n-len(code))
	switch {
	case hi < rg.Lo || lo > rg.Hi:
		return no
	case lo >= rg.Lo && hi <= rg.Hi:
		return yes
	}
	return open
}

func anyContains(rs []Range, code string) tri {
	t := no
	for _, rg := range rs {
		t = or(t, contains(rg, code))
	}
	return t
}

// pickRow finds the row for the good. ok is false when no row covers it;
// stop is true when res is final (a choice or a longer code is needed).
func (s *state) pickRow(ch Chapter, res *Result) (Row, bool, bool) {
	var sure, maybe []Row
	for _, row := range ch.Rows {
		switch anyContains(row.Applies, s.good) {
		case yes:
			sure = append(sure, row)
		case open:
			maybe = append(maybe, row)
		}
	}
	if len(maybe) > 0 {
		s.need(Need{Key: "good_code", Kind: NeedCode, Label: "The good's 6-digit HS subheading", Detail: "The list has rules by subheading here: " + codesOf(maybe)})
		res.Status, res.Reason = Undetermined, "the list has rules by subheading for this heading; give the 6-digit code"
		return Row{}, false, true
	}
	if len(sure) == 0 {
		return Row{}, false, false
	}
	if len(sure) == 1 && !sure[0].Ex && sure[0].Split == "" {
		return sure[0], true, false
	}
	if s.in.Row == RowNone {
		return Row{}, false, false
	}
	for _, row := range sure {
		if row.Key == s.in.Row {
			return row, true, false
		}
	}
	n := Need{Key: "row", Kind: NeedRow, Label: "Which of these is the good?", Detail: "The rule depends on the goods' description, not the code alone."}
	for _, row := range sure {
		n.Options = append(n.Options, Option{Key: row.Key, Code: row.Code, Description: oneLine(row.Description)})
	}
	// Split rows that end in "Other" cover the whole code: no "none".
	if !coversAll(sure) {
		n.Options = append(n.Options, Option{Key: RowNone, Code: "", Description: "None of these"})
	}
	s.need(n)
	res.Status, res.Reason = Undetermined, "the list covers only some goods of this code; say which the good is"
	return Row{}, false, true
}

func codesOf(rows []Row) string {
	var out []string
	for _, r := range rows {
		out = append(out, r.Code)
	}
	return strings.Join(out, ", ")
}

// notListed answers a good the list does not cover: the general rule, a
// judgement, unless the work was only minimal operations and the market's
// text then names the country of the major portion of the materials.
func (s *state) notListed(res Result, why string) Result {
	g := s.r.g
	if s.in.MinimalOnly != nil && *s.in.MinimalOnly {
		if !g.MinimalFallback {
			res.Status, res.Reason, res.Basis = NeedsJudgement, g.MinimalJudgement, g.MinimalRef
			return res
		}
		basis := "value"
		if c, _ := strconv.Atoi(res.Chapter); (c >= 1 && c <= 29) || (c >= 31 && c <= 40) {
			basis = "weight"
		}
		return s.majority(res, basis, g.MinimalRef+", last paragraph",
			why+"; the work was only minimal operations ("+g.MinimalRef+"), so the origin is where the major portion of the materials originated")
	}
	res.Status = NeedsJudgement
	res.Basis = g.General
	res.Reason = why + ". " + g.Judgement
	return res
}

// residual applies the chapter residual rules: the mixture rule when the
// chapter has one and the good is a mixture, else the major portion of the
// materials.
func (s *state) residual(res Result, ch Chapter, why string) Result {
	g := s.r.g
	if ch.Mixture != "" {
		t := s.answer(mixtureKey(ch.Chapter), "Is the good a mixture (fungible materials mixed in controlled proportions)?", ch.Mixture)
		switch t {
		case open:
			res.Status, res.Reason = Undetermined, why+"; the chapter residual rule depends on whether the good is a mixture"
			return res
		case yes:
			if strings.Contains(ch.Mixture, "dry matter") {
				s.need(Need{Key: "review", Kind: NeedReview, Label: "Some mixtures here are weighed by dry matter", Detail: ch.Mixture})
				res.Status, res.Reason = Undetermined, why+"; the mixture rule here weighs some mixtures by dry matter, which a weight share does not show"
				return res
			}
			return s.mixture(res, why)
		}
	}
	if ch.Residual == "" {
		s.need(Need{Key: "review", Kind: NeedReview, Label: "No residual rule was read for this chapter"})
		res.Status, res.Reason = Undetermined, why
		return res
	}
	return s.majority(res, ch.Residual, g.Residual, why+"; under the chapter residual rule the origin is where the major portion of the materials originated, by "+ch.Residual)
}

// mixture: the origin of the materials over 50% by weight, else where the
// mixing was done.
func (s *state) mixture(res Result, why string) Result {
	by, ok := s.shares("weight")
	if !ok {
		res.Status, res.Reason = Undetermined, why+"; the mixture rule needs each material's weight share"
		return res
	}
	res.Status, res.Basis = Decided, s.r.g.Mixture
	for _, c := range sortedKeys(by) {
		if by[c] > 0.5 {
			res.Origin = c
			res.Reason = fmt.Sprintf("%s; the good is a mixture and materials from %s are %s of its weight", why, c, pct(by[c]))
			return res
		}
	}
	res.Origin = s.country
	res.Reason = why + "; the good is a mixture, no origin has over 50% of its weight, so it is where the mixing was done"
	return res
}

// majority decides by the country with the largest share of the materials.
func (s *state) majority(res Result, basis, cite, why string) Result {
	by, ok := s.shares(basis)
	if !ok {
		res.Status, res.Reason = Undetermined, why+"; each material's "+basis+" share is needed"
		return res
	}
	keys := sortedKeys(by)
	sort.SliceStable(keys, func(i, j int) bool { return by[keys[i]] > by[keys[j]] })
	if len(keys) > 1 && math.Abs(by[keys[0]]-by[keys[1]]) < 1e-9 {
		s.need(Need{Key: "review", Kind: NeedReview, Label: "Two origins have the same share"})
		res.Status, res.Reason = Undetermined, why+"; two origins have the same share"
		return res
	}
	res.Status, res.Origin, res.Basis = Decided, keys[0], cite
	res.Reason = fmt.Sprintf("%s; materials from %s are the largest part (%s by %s)", why, keys[0], pct(by[keys[0]]), basis)
	if by[keys[0]] <= 0.5 {
		res.Assumes = append(res.Assumes, "\"major portion\" is read as the largest share by origin; no origin has more than half")
	}
	return res
}

// shares sums the materials' value or weight shares by origin. ok is false
// (and needs are recorded) when a share or an origin is missing.
func (s *state) shares(basis string) (map[string]float64, bool) {
	if len(s.in.Materials) == 0 {
		s.need(Need{Key: "materials", Kind: NeedCode, Label: "The materials, each with its origin and " + basis + " share"})
		return nil, false
	}
	by := map[string]float64{}
	ok := true
	for i, m := range s.in.Materials {
		o := domain.NormalizeCountry(m.Origin)
		if o == "" {
			s.need(Need{Key: "material_origin", Kind: NeedCode, Material: idx(i), Label: "Origin of " + matName(m, i)})
			ok = false
		}
		v := m.ValueShare
		key, label := "value_share", "Value share of "
		if basis == "weight" {
			v, key, label = m.WeightShare, "weight_share", "Weight share of "
		}
		if v == nil {
			s.need(Need{Key: key, Kind: NeedShare, Material: idx(i), Label: label + matName(m, i)})
			ok = false
			continue
		}
		by[o] += *v
	}
	return by, ok
}

func matName(m Material, i int) string {
	if n := strings.TrimSpace(m.Name); n != "" {
		return n
	}
	return fmt.Sprintf("material %d", i+1)
}

func pct(v float64) string { return strconv.FormatFloat(math.Round(v*1000)/10, 'f', -1, 64) + "%" }

func sortedKeys(m map[string]float64) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func (s *state) assumes(ch Chapter) []string {
	g := s.r.g
	var out []string
	for _, a := range []string{fmt.Sprintf(g.MinimalOps, s.country), g.Uneconomic, g.Neutral} {
		if a != "" {
			out = append(out, a)
		}
	}
	if len(ch.Notes) > 0 {
		out = append(out, "no chapter note excludes the change")
	}
	return out
}

// alt decides one alternative. For change rules it also returns how each
// material read.
func (s *state) alt(a Alternative) (tri, []MaterialResult) {
	switch {
	case !a.Encoded || a.Kind == KindProcess:
		return s.answer(a.Key, "In "+s.country+": "+brief(a.Text)+"?", a.Text), nil
	case a.Kind == KindMaxValue:
		if !s.r.g.MaxValueNonOriginating {
			return s.answer(a.Key, "Met in "+s.country+": "+brief(a.Text)+"?", a.Text), nil
		}
		return s.maxValue(a), nil
	}
	t, ms := s.change(a)
	if a.Condition != "" {
		t = and(t, s.answer(a.Key+".c", "In "+s.country+": "+brief(a.Condition)+"?", a.Text))
	}
	return t, ms
}

// maxValue: the non-originating materials' value shares sum to no more than
// MaxShare.
func (s *state) maxValue(a Alternative) tri {
	sum, t := 0.0, yes
	for i, m := range s.in.Materials {
		o := domain.NormalizeCountry(m.Origin)
		if o == "" {
			s.need(Need{Key: "material_origin", Kind: NeedCode, Material: idx(i), Label: "Origin of " + matName(m, i)})
			t = open
			continue
		}
		if o == s.country {
			continue
		}
		if m.ValueShare == nil {
			s.need(Need{Key: "value_share", Kind: NeedShare, Material: idx(i), Label: "Value share of " + matName(m, i)})
			t = open
			continue
		}
		sum += *m.ValueShare
	}
	if sum > a.MaxShare+1e-9 {
		return no
	}
	return t
}

// change decides a change rule over the non-originating materials, with
// the 10% value tolerance for those that fail.
func (s *state) change(a Alternative) (tri, []MaterialResult) {
	var ms []MaterialResult
	t := yes
	var failing []int
	for i, m := range s.in.Materials {
		mr := MaterialResult{Name: m.Name, HTS: m.HTS, Origin: domain.NormalizeCountry(m.Origin)}
		if mr.Origin == "" {
			s.need(Need{Key: "material_origin", Kind: NeedCode, Material: idx(i), Label: "Origin of " + matName(m, i)})
			mr.Status = "open"
			t = and(t, open)
			ms = append(ms, mr)
			continue
		}
		if mr.Origin == s.country {
			mr.Status = "originating"
			ms = append(ms, mr)
			continue
		}
		code := tariffshift.Digits(m.HTS)
		if len(code) < 4 {
			s.need(Need{Key: "material_hts", Kind: NeedCode, Material: idx(i), Label: "HS code of " + matName(m, i)})
			mr.Status = "missing_hts"
			t = and(t, open)
			ms = append(ms, mr)
			continue
		}
		var mt tri
		if a.Kind == KindChangeFrom {
			mt = anyContains(a.From, code)
			if mt == open {
				s.need(Need{Key: "material_hts", Kind: NeedCode, Material: idx(i), Label: "Longer HS code of " + matName(m, i)})
			}
		} else {
			mt = s.shifts(a, code, i, m)
		}
		mr.Status = map[tri]string{yes: "changes", no: "no_change", open: "open"}[mt]
		ms = append(ms, mr)
		switch mt {
		case no:
			failing = append(failing, i)
		case open:
			t = and(t, open)
		}
	}
	if len(failing) == 0 {
		return t, ms
	}
	// Note 2.5: failing materials are disregarded when their value is no more
	// than 10% of the ex-works price.
	sum := 0.0
	for _, i := range failing {
		v := s.in.Materials[i].ValueShare
		if v == nil {
			s.need(Need{Key: "value_share", Kind: NeedShare, Material: idx(i), Label: "Value share of " + matName(s.in.Materials[i], i),
				Detail: "Materials that do not meet the rule are disregarded if together they are worth no more than 10% of the good (" + s.r.g.Tolerance + ")."})
			return and(t, open), ms
		}
		sum += *v
	}
	if sum <= 0.10+1e-9 {
		return t, ms
	}
	return no, ms
}

// shifts says whether a non-originating material of code meets a change
// rule.
func (s *state) shifts(a Alternative, code string, i int, m Material) tri {
	n := a.Level
	if len(s.good) < n {
		s.need(Need{Key: "good_code", Kind: NeedCode, Label: "The good's 6-digit HS subheading", Detail: a.Text + " compares subheadings."})
		return open
	}
	var t tri
	switch {
	case code[:min(len(code), n)] != s.good[:min(len(code), n)]:
		t = yes
	case len(code) >= n:
		t = no
		if a.Split {
			s.need(Need{Key: "review", Kind: NeedReview, Label: matName(m, i) + " is in the good's own heading or subheading", Detail: a.Text + ": a change from another split also counts, which the code alone does not show."})
			t = open
		}
	default:
		s.need(Need{Key: "material_hts", Kind: NeedCode, Material: idx(i), Label: "Longer HS code of " + matName(m, i)})
		t = open
	}
	if t == no {
		return no
	}
	for j, e := range a.Except {
		in := anyContains(e.From, code)
		switch in {
		case no:
			continue
		case open:
			s.need(Need{Key: "material_hts", Kind: NeedCode, Material: idx(i), Label: "Longer HS code of " + matName(m, i)})
			t = and(t, open)
			continue
		}
		// In the excepted range.
		hit := yes
		if e.Described {
			s.need(Need{Key: "review", Kind: NeedReview, Label: "Is " + matName(m, i) + " one of the excepted goods?", Detail: e.Text})
			hit = open
		}
		if e.Condition != "" {
			hit = and(hit, s.answer(exceptKey(a.Key, j), "Does the change of "+matName(m, i)+" result from "+e.Condition+"?", e.Text))
		}
		switch hit {
		case yes:
			return no
		case open:
			t = and(t, open)
		}
	}
	return t
}

// brief is the first line of a rule's text, cut to about 120 characters,
// for a question's label; the full text is the need's detail.
func brief(s string) string {
	s = strings.TrimSpace(strings.SplitN(s, "\n", 2)[0])
	s = strings.TrimSuffix(strings.TrimSuffix(s, ":"), ".")
	if r := []rune(s); len(r) > 120 {
		s = strings.TrimSpace(string(r[:117])) + "…"
	}
	return s
}

// reOther is a split row's catch-all description ("- other", "Other").
var reOther = regexp.MustCompile(`(?i)^[-–\s]*other\s*$`)

// coversAll: the rows are splits of one code and one of them is "Other",
// so every good of the code is one of them.
func coversAll(rows []Row) bool {
	for _, r := range rows {
		if r.Split == "" {
			return false
		}
	}
	for _, r := range rows {
		if reOther.MatchString(oneLine(r.Description)) {
			return true
		}
	}
	return false
}
