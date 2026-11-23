package tariffshift

import (
	"encoding/json"
	"reflect"
	"slices"
	"strings"
	"testing"
)

func rng(t *testing.T, s string) []Range {
	t.Helper()
	r, err := ParseRange(s)
	if err != nil {
		t.Fatal(err)
	}
	return []Range{r}
}

// toyRules is a one-row Canadian-regime table with rules the real Schedule
// III does not carry (it has no simple-assembly, GRI 2(a) or bending
// clauses), so the fact paths are tested against the evaluator itself.
func toyRules(t *testing.T, alts ...Alternative) *Rules {
	t.Helper()
	return New(Table{Regime: Canada.ID, Chapters: []string{"84"}, Rules: []Rule{{
		ID: "toy 8415.10", Paragraph: "84", HTS: "8415.10", Applies: rng(t, "841510")[0], Alternatives: alts,
	}}})
}

func guideOf(r *Rules, in Input) (Result, *Guidance) {
	res := r.Evaluate(in)
	return res, Guide(r, in, res)
}

func keys(g *Guidance) []string {
	var out []string
	for _, it := range g.Missing {
		out = append(out, it.Key)
	}
	return out
}

func TestGuideFactAnswerable(t *testing.T) {
	r := toyRules(t, Alternative{Encoded: true, To: rng(t, "841510"), From: 6,
		Except: []Exception{{From: rng(t, "841590"), Condition: CondSimpleAssembly}}})
	in := Input{GoodHTS: "8415.10", Country: "Mexico", Materials: []Material{{Name: "chassis", HTS: "8415.90", Origin: "China", ValueShare: ptrf(0.5)}}}
	res, g := guideOf(r, in)
	if res.Status != Undetermined || g == nil {
		t.Fatalf("%+v %v", res, g)
	}
	if g.State != GuideAnswerable || !slices.Equal(keys(g), []string{KeySimpleAssembly}) || g.Review != nil {
		t.Fatalf("%+v", g)
	}
	it := g.Missing[0]
	if it.Kind != KindFact || !it.Answerable || !it.Blocking || it.Material != nil || !strings.Contains(it.Detail, "chassis") {
		t.Fatalf("%+v", it)
	}
	// Answering settles it, either way; false is not the same as unknown.
	for v, origin := range map[bool]string{true: "China", false: "Mexico"} {
		q := in
		q.SimpleAssembly = &v
		got := r.Evaluate(q)
		if got.Status != Decided || got.Origin != origin {
			t.Fatalf("simple_assembly=%v: %+v", v, got)
		}
		if g2 := Guide(r, q, got); g2 != nil {
			t.Fatalf("settled but guided: %+v", g2)
		}
	}
	// JSON: a case-level item has no material key.
	b, _ := json.Marshal(g)
	if strings.Contains(string(b), `"material"`) {
		t.Fatalf("%s", b)
	}
}

// A fact asked only by a clause that cannot change the outcome is not
// reported, though the legacy needs list still carries it.
func TestGuideIrrelevantAlternativeFact(t *testing.T) {
	r := toyRules(t, Alternative{Encoded: true, To: rng(t, "841510"), From: 6,
		Also: []Source{{From: rng(t, "7318"), Condition: CondNotGRI2a}}})
	// A 4-digit code of 8415 cannot yet be told from the good's subheading;
	// the GRI 2(a) source is for 7318 only, so it is already no.
	in := Input{GoodHTS: "8415.10", Country: "Mexico", Materials: []Material{{Name: "part", HTS: "8415", Origin: "China"}}}
	res, g := guideOf(r, in)
	if res.Status != Undetermined || !slices.Contains(res.Materials[0].Needs, needGRI) {
		t.Fatalf("legacy needs should carry the eager fact: %+v", res.Materials[0])
	}
	if g == nil || g.State != GuideNeedsInput || !slices.Equal(keys(g), []string{KeyMaterialCode}) {
		t.Fatalf("%+v", g)
	}
	if g.Missing[0].Material == nil || *g.Missing[0].Material != 0 {
		t.Fatalf("%+v", g.Missing[0])
	}
	b, _ := json.Marshal(g)
	if !strings.Contains(string(b), `"material":0`) {
		t.Fatalf("index 0 must serialize: %s", b)
	}
}

// A fact that matters to one alternative while another already holds is not
// blocking; the result is decided, so there is no guidance at all.
func TestGuideAnotherAlternativeSatisfied(t *testing.T) {
	r := toyRules(t,
		Alternative{Encoded: true, To: rng(t, "841510"), From: 6},
		Alternative{Encoded: true, To: rng(t, "841510"), From: 6, NotGRI2a: true})
	res, g := guideOf(r, Input{GoodHTS: "8415.10", Country: "Mexico", Materials: []Material{{Name: "p", HTS: "7318.15", Origin: "China"}}})
	if res.Status != Decided || g != nil {
		t.Fatalf("%+v %+v", res, g)
	}
}

func TestGuideDomesticOnlyWhereRead(t *testing.T) {
	r := toyRules(t, Alternative{Encoded: true, To: rng(t, "841510"), From: 6, Group: "outside",
		Except: []Exception{{From: rng(t, "841590"), Condition: CondSimpleAssembly}}})
	foreignFail := Material{Name: "a", HTS: "8415.10", Origin: "China", ValueShare: ptrf(0.5)}
	domestic := Material{Name: "b", HTS: "8415.90", Origin: "Mexico"}

	// Foreign A fails the shift, so the essential-character test reads B: its
	// simple-assembly fact matters.
	_, g := guideOf(r, Input{GoodHTS: "8415.10", Country: "Mexico", Materials: []Material{foreignFail, domestic}})
	if g == nil || !slices.Contains(keys(g), KeySimpleAssembly) {
		t.Fatalf("domestic fact should count: %+v", g)
	}

	// Foreign A has no code: the answer stops there and never reads B.
	open := Material{Name: "a", Origin: "China"}
	_, g = guideOf(r, Input{GoodHTS: "8415.10", Country: "Mexico", Materials: []Material{open, domestic}})
	if g == nil || !slices.Equal(keys(g), []string{KeyMaterialHTS}) || g.State != GuideNeedsInput {
		t.Fatalf("%+v", g)
	}
}

func TestGuideShares(t *testing.T) {
	r := toyRules(t, Alternative{Encoded: true, To: rng(t, "841510"), From: 6, Group: "outside"})
	in := Input{GoodHTS: "8415.10", Country: "Mexico", Materials: []Material{
		{Name: "ok", HTS: "7318.15", Origin: "China"},
		{Name: "bad", HTS: "8415.10", Origin: "China"}}}
	res, g := guideOf(r, in)
	if res.Status != Undetermined || g == nil || g.State != GuideNeedsInput || !slices.Equal(keys(g), []string{KeyValueShare}) {
		t.Fatalf("%+v %+v", res, g)
	}
	it := g.Missing[0]
	if it.Material == nil || *it.Material != 1 || it.Answerable || it.Kind != KindShare {
		t.Fatalf("%+v", it)
	}
}

func TestGuideUnmodeledRealRows(t *testing.T) {
	r := mustCanada(t)
	// 0210.91 has two alternatives written by goods description.
	in := Input{GoodHTS: "0210.91", Country: "Mexico", Materials: []Material{{Name: "meat", HTS: "0201.10", Origin: "Brazil"}}}
	res, g := guideOf(r, in)
	if res.Status != Undetermined || g == nil {
		t.Fatalf("%+v %+v", res, g)
	}
	if g.State != GuideNeedsReview || g.Review == nil || g.Review.Reason == "" || !slices.Equal(keys(g), []string{KeyPersonReading}) {
		t.Fatalf("%+v", g)
	}
	for _, it := range g.Missing {
		if it.Answerable || it.Kind != KindReview {
			t.Fatalf("review item offered as a control: %+v", it)
		}
	}
	// The booleans cannot settle a rule the evaluator does not model.
	yes := true
	in.SimpleAssembly, in.GRI2a, in.BendingToShape = &yes, &yes, &yes
	if got, g2 := guideOf(r, in); got.Status != Undetermined || g2 == nil || g2.State != GuideNeedsReview {
		t.Fatalf("%+v %+v", got, g2)
	}
}

func TestGuideGoodsDescriptionRow(t *testing.T) {
	r := mustCanada(t)
	in := Input{GoodHTS: "6401.10", Country: "Mexico", Materials: []Material{{Name: "upper", HTS: "6406.10", Origin: "China"}}}
	res, g := guideOf(r, in)
	if res.Status != Undetermined {
		t.Skipf("row decides now: %+v", res)
	}
	if g == nil || !slices.Contains(keys(g), KeyGoodsDescription) || g.State != GuideNeedsReview {
		t.Fatalf("%+v %+v", res, g)
	}
}

func TestGuideMissingCodes(t *testing.T) {
	r := mustCanada(t)
	cases := []struct {
		name string
		in   Input
		key  string
		mat  *int
	}{
		{"short good", Input{GoodHTS: "84", Country: "Mexico", Materials: []Material{{Name: "a", HTS: "7318.15", Origin: "China"}}}, KeyGoodCode, nil},
		{"material code, index 1", Input{GoodHTS: "8473.30", Country: "Mexico", Materials: []Material{{Name: "a", HTS: "7318.15", Origin: "Mexico"}, {Name: "b", Origin: "China"}}}, KeyMaterialHTS, ptr(1)},
		{"no materials", Input{GoodHTS: "8473.30", Country: "Mexico"}, KeyMaterialHTS, nil},
		{"no country", Input{GoodHTS: "8473.30", Materials: []Material{{Name: "a", HTS: "7318.15", Origin: "China"}}}, KeyCountry, nil},
	}
	for _, c := range cases {
		res, g := guideOf(r, c.in)
		if res.Status != Undetermined || g == nil || g.State != GuideNeedsInput || g.Review != nil {
			t.Errorf("%s: %+v %+v", c.name, res, g)
			continue
		}
		it := g.Missing[0]
		if it.Key != c.key || it.Kind != KindCode || it.Answerable || !reflect.DeepEqual(it.Material, c.mat) {
			t.Errorf("%s: %+v", c.name, it)
		}
	}
}

func TestGuideNone(t *testing.T) {
	r := mustCanada(t)
	// Decided, out of scope and malformed input carry no guidance.
	for name, in := range map[string]Input{
		"decided":      {GoodHTS: "6301.30", Country: "Mexico", Materials: []Material{{Name: "y", HTS: "5205.11", Origin: "China"}}},
		"out of scope": {GoodHTS: "7701", Country: "Mexico", Materials: []Material{{Name: "y", HTS: "5205.11", Origin: "China"}}},
		"bad share":    {GoodHTS: "6301.30", Country: "Mexico", Materials: []Material{{Name: "y", HTS: "5205.11", Origin: "China", ValueShare: ptrf(1.5)}}},
	} {
		res := r.Evaluate(in)
		if g := Guide(r, in, res); g != nil {
			t.Errorf("%s: %s %+v", name, res.Status, g)
		}
	}
	if Guide(nil, Input{}, Result{Status: Undetermined}) != nil {
		t.Error("nil rules")
	}
}

// Guide never changes what Evaluate says, and the U.S. result type has no
// guidance field, so a case's rule_check JSON is unchanged.
func TestResultHasNoGuidanceField(t *testing.T) {
	typ := reflect.TypeOf(Result{})
	for i := 0; i < typ.NumField(); i++ {
		if strings.Contains(strings.ToLower(typ.Field(i).Tag.Get("json")), "guidance") {
			t.Fatalf("Result carries guidance: %s", typ.Field(i).Name)
		}
	}
	r := mustCanada(t)
	in := Input{GoodHTS: "0210.91", Country: "Mexico", Materials: []Material{{Name: "meat", HTS: "0201.10", Origin: "Brazil"}}}
	before, _ := json.Marshal(r.Evaluate(in))
	Guide(r, in, r.Evaluate(in))
	after, _ := json.Marshal(r.Evaluate(in))
	if string(before) != string(after) || strings.Contains(string(after), "guidance") {
		t.Fatalf("%s\n%s", before, after)
	}
}
