package tariffshift

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
)

// States of a Guidance.
const (
	// GuideAnswerable: at least one yes/no fact would move the answer.
	GuideAnswerable = "answerable"
	// GuideNeedsInput: no fact helps, but a code, a share or an origin is
	// missing from the input.
	GuideNeedsInput = "needs_input"
	// GuideNeedsReview: only a person can go further (a rule written by goods
	// description, or a judgment the codes cannot settle).
	GuideNeedsReview = "needs_review"
)

// Kinds of a GuidanceItem.
const (
	KindFact   = "fact"   // a yes/no fact the request can state
	KindCode   = "code"   // a code (or origin) to enter
	KindShare  = "share"  // a value or weight share to enter
	KindReview = "review" // a person's reading; never a control
)

// Keys of a GuidanceItem.
const (
	KeySimpleAssembly   = "simple_assembly"
	KeyGRI2a            = "gri_2a"
	KeyBendingToShape   = "bending_to_shape"
	KeyGoodCode         = "good_code"
	KeyCountry          = "country"
	KeyMaterialHTS      = "material_hts"
	KeyMaterialCode     = "material_code"
	KeyMaterialOrigin   = "material_origin"
	KeyValueShare       = "value_share"
	KeyWeightShare      = "weight_share"
	KeyGoodsDescription = "goods_description"
	KeyPersonReading    = "person_reading"
)

// GuidanceItem is one thing that would move an undetermined answer.
type GuidanceItem struct {
	Key  string `json:"key"`
	Kind string `json:"kind"`
	// Material is the 0-based index of the material the item is about; nil
	// for the whole case. A pointer, so index 0 is sent as 0, not left out.
	Material *int   `json:"material,omitempty"`
	Label    string `json:"label"`
	Detail   string `json:"detail,omitempty"`
	// Answerable: the request can carry the answer as a boolean field. False
	// for codes, shares and every review item.
	Answerable bool `json:"answerable"`
	// Blocking: resolving the item can change the answer.
	Blocking bool `json:"blocking"`
}

// GuidanceReview says why a person has to read the rule.
type GuidanceReview struct {
	Reason string `json:"reason"`
}

// Guidance says what would resolve an undetermined result.
type Guidance struct {
	State   string          `json:"state"`
	Missing []GuidanceItem  `json:"missing"`
	Review  *GuidanceReview `json:"review,omitempty"`
}

const reviewReason = "A person has to read part of this rule: it is written by goods description, or needs a judgment that codes cannot show."

// Guide says what would resolve an undetermined result of rules.Evaluate(in).
// It is pure: no model, no I/O, and it never changes res. It returns nil when
// res is not undetermined, when the input is malformed (shares out of range),
// or when nothing in the input can be improved (for example a good outside
// the encoded chapters).
//
// Triage. Evaluate records the facts an alternative would need even when
// that alternative can no longer change the outcome (it is already no, or
// another alternative already holds), so the recorded Needs strings are not
// used for facts. A yes/no fact is reported only if, with the fact set to
// true or to false on a copy of the input, the full Evaluate gives a different
// overall status, origin, reason or conditional answer, or a different status
// for a material the answer actually uses (foreign materials; domestic ones
// show up through the overall reason, where 102.11(b) or s. 5 reads them).
// Codes, shares and unmodeled readings are taken from the materials the
// answer uses, using the same order of tests as evaluate.
func Guide(rules *Rules, in Input, res Result) *Guidance {
	if rules == nil || res.Status != Undetermined || ValidateShares(in.Materials) != nil {
		return nil
	}
	b := &guideBuilder{}
	country := domain.NormalizeCountry(in.Country)
	if country == "" {
		b.add(GuidanceItem{Key: KeyCountry, Kind: KindCode, Label: "Where is the good made?", Detail: "The country of production is not given."})
		return b.done()
	}
	rule, why, ok := rules.Lookup(in.GoodHTS)
	g := Digits(in.GoodHTS)
	switch {
	case !ok && g != "" && !rules.InScope(g):
		return nil
	case !ok:
		b.add(GuidanceItem{Key: KeyGoodCode, Kind: KindCode, Label: "Give the good's full HS code", Detail: capital(why)})
		return b.done()
	case anyContains(rule.Textile, g) == maybe:
		b.add(GuidanceItem{Key: KeyGoodCode, Kind: KindCode, Label: "Give the good's full HS code", Detail: "Some codes under it follow other rules."})
		return b.done()
	}
	if len(in.Materials) == 0 {
		b.add(GuidanceItem{Key: KeyMaterialHTS, Kind: KindCode, Label: "Add the materials of the good with their HS codes", Detail: "The rule tests each foreign material."})
		return b.done()
	}

	// Which materials the answer reads, in the order evaluate reads them.
	var fail, open, noOrigin []int
	for i, m := range res.Materials {
		if !m.Foreign {
			continue
		}
		switch m.Status {
		case MatNoShift:
			fail = append(fail, i)
			if m.Origin == "" {
				noOrigin = append(noOrigin, i)
			}
		case MatNeedsFact, MatMissingHTS:
			open = append(open, i)
		}
	}
	// Domestic materials are read only by the essential-character test,
	// which evaluate reaches when every foreign material is decided, one
	// fails, its origin is known and no chapter note replaces that test.
	reached := len(open) == 0 && len(noOrigin) == 0 && len(fail) > 0 && !rule.FallbackNote
	live := func(i int) bool { return res.Materials[i].Foreign || reached }

	review := map[string][]string{}
	goodShort := len(g) < 6
	for _, a := range rule.Alternatives {
		if anyContains(a.To, g) == maybe && len(a.To) > 0 {
			goodShort = true
		}
	}
	for i, m := range res.Materials {
		if !live(i) {
			continue
		}
		switch m.Status {
		case MatMissingHTS:
			b.add(GuidanceItem{Key: KeyMaterialHTS, Kind: KindCode, Material: ptr(i), Label: "HS code for " + matName(m, i)})
		case MatNeedsFact:
			if slices.Contains(m.Needs, needOwnCode) {
				b.add(GuidanceItem{Key: KeyMaterialHTS, Kind: KindCode, Material: ptr(i), Label: "HS code for " + matName(m, i), Detail: "Only a code it is outside of is known."})
			}
			if slices.Contains(m.Needs, needCode) {
				if goodShort {
					b.add(GuidanceItem{Key: KeyGoodCode, Kind: KindCode, Label: "Give the good's full HS code", Detail: "A longer code is needed to apply the rule."})
				}
				if len(Digits(m.HTS)) < 6 || !goodShort {
					b.add(GuidanceItem{Key: KeyMaterialCode, Kind: KindCode, Material: ptr(i), Label: "Longer HS code for " + matName(m, i), Detail: "A code of 6 digits or more is needed to apply the rule."})
				}
			}
			if slices.Contains(m.Needs, needDescribe) {
				review[KeyGoodsDescription] = append(review[KeyGoodsDescription], matName(m, i))
			}
			if slices.Contains(m.Needs, needPerson) {
				review[KeyPersonReading] = append(review[KeyPersonReading], matName(m, i))
			}
		}
	}
	for _, i := range noOrigin {
		b.add(GuidanceItem{Key: KeyMaterialOrigin, Kind: KindCode, Material: ptr(i), Label: "Where is " + matName(res.Materials[i], i) + " made?", Detail: "It does not shift, so its origin matters."})
	}

	if len(open) == 0 && len(noOrigin) == 0 && len(fail) > 0 {
		g0 := rules.g
		chapter := g[:2]
		byWeight := slices.Contains(g0.ByWeight, chapter)
		if !slices.Contains(g0.DeMinimisExcluded, chapter) {
			for _, i := range fail {
				m := in.Materials[i]
				switch {
				case byWeight && m.WeightShare == nil:
					b.add(GuidanceItem{Key: KeyWeightShare, Kind: KindShare, Material: ptr(i), Label: "Weight share of " + matName(res.Materials[i], i), Detail: "Its share of the good's weight. A small enough share is ignored (" + g0.DeMinimisRef + ")."})
				case !byWeight && m.ValueShare == nil:
					b.add(GuidanceItem{Key: KeyValueShare, Kind: KindShare, Material: ptr(i), Label: "Value share of " + matName(res.Materials[i], i), Detail: "Its share of the good's value. A small enough share is ignored (" + g0.DeMinimisRef + ")."})
				}
			}
		}
		switch {
		case rule.FallbackNote:
			review[KeyPersonReading] = append(review[KeyPersonReading], "a chapter note gives its own origin test")
		default:
			// The essential-character pick between materials of different
			// countries is a judgment, not a fact.
			origins := map[string]bool{}
			unknown := false
			for _, m := range res.Materials {
				switch m.Status {
				case MatNoShift:
					origins[m.Origin] = true
				case MatNeedsFact, MatMissingHTS:
					unknown = true
				}
			}
			if !unknown && len(origins) > 1 {
				review[KeyPersonReading] = append(review[KeyPersonReading], "the essential character is judged between materials of different countries")
			}
		}
	}

	// Facts, by counterfactual.
	base := guideSig(res)
	for _, f := range []struct {
		key, label, detail string
		get                func(*Input) **bool
	}{
		{KeySimpleAssembly, "Is this a simple assembly?", "The parts are put together with no more than minor processing.", func(i *Input) **bool { return &i.SimpleAssembly }},
		{KeyGRI2a, "Are the parts only gathered together?", "GRI 2(a): the parts are collected into an unfinished good that is classified as the finished good.", func(i *Input) **bool { return &i.GRI2a }},
		{KeyBendingToShape, "Are the tubes bent to shape?", "The production bends steel tubes to shape.", func(i *Input) **bool { return &i.BendingToShape }},
	} {
		if *f.get(&in) != nil {
			continue
		}
		var moved []string
		changed := false
		for _, v := range []bool{true, false} {
			q := in
			val := v
			*f.get(&q) = &val
			alt := rules.Evaluate(q)
			if guideSig(alt) != base {
				changed = true
			}
			for i, m := range alt.Materials {
				if i < len(res.Materials) && res.Materials[i].Foreign && m.Status != res.Materials[i].Status {
					if n := matName(res.Materials[i], i); !slices.Contains(moved, n) {
						moved = append(moved, n)
					}
				}
			}
		}
		if changed {
			d := f.detail
			if len(moved) > 0 {
				d += " Decides: " + strings.Join(moved, ", ") + "."
			}
			b.add(GuidanceItem{Key: f.key, Kind: KindFact, Label: f.label, Detail: d, Answerable: true})
		}
	}
	// Facts first among the answerable ones, then codes in material order.
	slices.SortStableFunc(b.items, func(x, y GuidanceItem) int { return rank(x) - rank(y) })

	for _, k := range []string{KeyGoodsDescription, KeyPersonReading} {
		if who := review[k]; len(who) > 0 {
			b.add(GuidanceItem{Key: k, Kind: KindReview, Label: reviewLabel(k), Detail: "Affects: " + strings.Join(who, ", ") + "."})
			b.review = true
		}
	}
	return b.done()
}

func rank(it GuidanceItem) int {
	switch it.Kind {
	case KindFact:
		return 1
	case KindReview:
		return 3
	}
	if it.Material == nil {
		return 0
	}
	return 2
}

func reviewLabel(key string) string {
	if key == KeyGoodsDescription {
		return "The rule turns on a goods description"
	}
	return "A person has to read this rule"
}

type guideBuilder struct {
	items  []GuidanceItem
	review bool
}

func (b *guideBuilder) add(it GuidanceItem) {
	it.Blocking = true
	for _, x := range b.items {
		if x.Key == it.Key && ((x.Material == nil) == (it.Material == nil)) && (x.Material == nil || *x.Material == *it.Material) {
			return
		}
	}
	b.items = append(b.items, it)
}

func (b *guideBuilder) done() *Guidance {
	g := finishGuidance(b.items, b.review)
	return g
}

// finishGuidance sets the state of a list of items; nil when it is empty.
func finishGuidance(items []GuidanceItem, review bool) *Guidance {
	if len(items) == 0 {
		return nil
	}
	g := &Guidance{Missing: items}
	hasInput := false
	g.State = GuideNeedsReview
	for _, it := range items {
		switch {
		case it.Answerable:
			g.State = GuideAnswerable
		case it.Kind != KindReview:
			hasInput = true
		}
	}
	if g.State != GuideAnswerable && hasInput {
		g.State = GuideNeedsInput
	}
	if review {
		g.Review = &GuidanceReview{Reason: reviewReason}
	}
	return g
}

// guideSig is what a counterfactual must change to count: the answer, and the
// status of every foreign material (not which alternative satisfied it). Needs strings are left out on purpose:
// they include facts of clauses that are already no.
func guideSig(r Result) string {
	s := struct {
		Status, Origin, Reason string
		IfNot                  *Conditional
		Mats                   []string
	}{r.Status, r.Origin, r.Reason, r.IfNotDeMinimis, nil}
	for _, x := range r.Materials {
		if x.Foreign {
			s.Mats = append(s.Mats, x.Status)
		}
	}
	b, _ := json.Marshal(s)
	return string(b)
}

func matName(m MaterialResult, i int) string {
	if strings.TrimSpace(m.Name) != "" {
		return m.Name
	}
	return fmt.Sprintf("material %d", i+1)
}

func capital(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:] + "."
}

func ptr[T any](v T) *T { return &v }
