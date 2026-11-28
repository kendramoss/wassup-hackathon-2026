package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/llm"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/retrieval"
)

func mk(feature, id, split, facts string, exp Expected) Case {
	return Case{ID: id, Feature: feature, Split: split, SourceFamily: "fam-" + id, ScenarioFamily: "s", Template: "t-" + id,
		Sources: []Source{{ID: "src", Locator: "19 CFR 102.21"}}, Rationale: "because " + id, Synthetic: true,
		Input: Input{Facts: facts, Date: "2026-11-28"}, Expected: exp}
}

func dec(origin string) Expected { return Expected{Kind: "decided", Origin: origin} }

func TestIntegrityCatchesBadDatasets(t *testing.T) {
	good := func() ([]Case, []Case) {
		var dev, test []Case
		for i := 0; i < 3; i++ {
			dev = append(dev, mk("textile_specialist", fmt.Sprintf("c0000000000%02d", i), "dev", fmt.Sprintf("dev facts number %d", i), dec("China")))
			test = append(test, mk("textile_specialist", fmt.Sprintf("c1000000000%02d", i), "test", fmt.Sprintf("test facts number %d", i), dec("India")))
		}
		return dev, test
	}
	dev, test := good()
	if p := CheckIntegrity(dev, test, IntegrityOptions{MinTest: 3}); len(p) != 0 {
		t.Fatalf("a well-formed dataset has problems: %v", p)
	}
	cases := map[string]func(dev, test []Case) ([]Case, []Case){
		"reserved cases below the minimum":      func(d, te []Case) ([]Case, []Case) { return d, te[:2] },
		"normalised input shared across splits": func(d, te []Case) ([]Case, []Case) { te[0].Input.Facts = "DEV  facts, number 0!"; return d, te },
		"duplicate id":                          func(d, te []Case) ([]Case, []Case) { te[1].ID = te[0].ID; return d, te },
		"id is not opaque":                      func(d, te []Case) ([]Case, []Case) { te[0].ID = "textile-test-001"; return d, te },
		"wrong split on a case":                 func(d, te []Case) ([]Case, []Case) { te[0].Split = "dev"; return d, te },
		"unknown feature":                       func(d, te []Case) ([]Case, []Case) { te[0].Feature = "invented"; return d, te },
		"no source":                             func(d, te []Case) ([]Case, []Case) { te[0].Sources = nil; return d, te },
		"no rationale":                          func(d, te []Case) ([]Case, []Case) { te[0].Rationale = ""; return d, te },
		"decided without an origin":             func(d, te []Case) ([]Case, []Case) { te[0].Expected = Expected{Kind: "decided"}; return d, te },
		"unknown expected kind":                 func(d, te []Case) ([]Case, []Case) { te[0].Expected.Kind = "maybe"; return d, te },
		"duplicate input inside the split":      func(d, te []Case) ([]Case, []Case) { te[1].Input.Facts = te[0].Input.Facts; return d, te },
		"empty facts":                           func(d, te []Case) ([]Case, []Case) { te[0].Input.Facts = ""; return d, te },
		"unlabelled synthetic flag":             func(d, te []Case) ([]Case, []Case) { te[0].Synthetic = false; return d, te },
	}
	for name, mut := range cases {
		d, te := good()
		d, te = mut(d, te)
		if p := CheckIntegrity(d, te, IntegrityOptions{MinTest: 3}); len(p) == 0 {
			t.Errorf("%s: not reported", name)
		}
	}
}

func TestLeaksFindsAnswerOrMetadataInTheModelVisibleInput(t *testing.T) {
	c := mk("textile_specialist", "c0123456789ab", "test", "A shirt is sewn in India.", dec("India"))
	c.Template, c.Rationale = "cut-and-sew-assembly", "wholly assembled in a single country"
	if l := Leaks(c); len(l) != 0 {
		t.Fatalf("clean input flagged: %v", l)
	}
	for name, facts := range map[string]string{
		"id":        "A shirt is sewn in India. Case c0123456789ab.",
		"template":  "A shirt is sewn in India. See cut-and-sew-assembly.",
		"rationale": "A shirt is sewn in India. wholly assembled in a single country",
		"label":     "A shirt is sewn in India. Expected origin: India.",
		"kind":      "A shirt is sewn in India. This case is decided.",
	} {
		d := c
		d.Input.Facts = facts
		if len(Leaks(d)) == 0 {
			t.Errorf("%s leak not found", name)
		}
	}
}

func TestGradeOriginCases(t *testing.T) {
	d := mk("textile_specialist", "c0000000000a1", "test", "f", dec("India"))
	u := mk("pharma_specialist", "c0000000000a2", "test", "g", Expected{Kind: "uncertain"})
	row := func(c Case, origin string, conf float64, route string) Row {
		return Row{ID: c.ID, Feature: c.Feature, Split: c.Split, CalledModel: true, Provider: "litellm", Origin: origin, Confidence: conf, Route: route}
	}
	tests := []struct {
		name         string
		c            Case
		r            Row
		verdict, cls string
	}{
		{"agree confident", d, row(d, "India", .95, "auto"), Match, "agree_confident"},
		{"agree but sent to review", d, row(d, "India", .6, "needs_review"), Match, "agree_review"},
		{"origin spelled differently", d, row(d, " INDIA ", .97, "auto"), Match, "agree_confident"},
		{"wrong and confident", d, row(d, "China", .97, "auto"), Mismatch, "wrong_confident"},
		{"wrong but reviewed", d, row(d, "China", .5, "needs_review"), Mismatch, "wrong_review"},
		{"uncertain case reviewed is correct", u, row(u, "Italy", .5, "needs_review"), Match, "abstain_correct"},
		{"uncertain case answered confidently is wrong", u, row(u, "Italy", .96, "auto"), Mismatch, "overconfident"},
		{"error is never a match", d, Row{ID: d.ID, Feature: d.Feature, Split: d.Split, Error: "timeout"}, ErrorV, "error"},
		{"error on uncertain is not an abstention", u, Row{ID: u.ID, Feature: u.Feature, Split: u.Split, Error: "timeout"}, ErrorV, "error"},
	}
	for _, tc := range tests {
		got := Grade(tc.c, tc.r)
		if got.Verdict != tc.verdict || got.Class != tc.cls {
			t.Errorf("%s: verdict %q class %q, want %q %q", tc.name, got.Verdict, got.Class, tc.verdict, tc.cls)
		}
	}
}

func TestGradeJapanAcceptsEveryValidCriterion(t *testing.T) {
	q := mk("japan_engine", "c0000000000b1", "test", "f", Expected{Kind: "decided", Outcome: "qualifies", Criteria: []string{"CTH", "CTSH", "PE"}})
	n := mk("japan_engine", "c0000000000b2", "test", "g", Expected{Kind: "decided", Outcome: "does_not_qualify"})
	u := mk("japan_engine", "c0000000000b3", "test", "h", Expected{Kind: "uncertain", Outcome: "undetermined"})
	pf := mk("japan_engine", "c0000000000b4", "test", "i", Expected{Kind: "uncertain", Outcome: "undetermined", Preflight: true})
	row := func(c Case, out, crit string, called bool) Row {
		return Row{ID: c.ID, Feature: c.Feature, Split: c.Split, CalledModel: called, Provider: "litellm", Outcome: out, Criterion: crit, Confidence: .9}
	}
	for _, tc := range []struct {
		name         string
		c            Case
		r            Row
		verdict, cls string
		critOK       *bool
	}{
		{"qualifies by the stricter valid criterion", q, row(q, "qualifies", "CTSH", true), Match, "agree", ptr(true)},
		{"qualifies by the rule's own criterion", q, row(q, "qualifies", "CTH", true), Match, "agree", ptr(true)},
		{"qualifies by a criterion the facts do not support", q, row(q, "qualifies", "RVC", true), Match, "agree", ptr(false)},
		{"declined when the facts decide", q, row(q, "undetermined", "NONE", true), Mismatch, "abstained_on_decided", nil},
		{"wrong decision", q, row(q, "does_not_qualify", "NONE", true), Mismatch, "wrong_decision", nil},
		{"correct negative", n, row(n, "does_not_qualify", "NONE", true), Match, "agree", nil},
		{"qualified a failing good", n, row(n, "qualifies", "CTH", true), Mismatch, "wrong_decision", nil},
		{"undetermined when facts are missing", u, row(u, "undetermined", "NONE", true), Match, "abstain_correct", nil},
		{"decided on missing facts", u, row(u, "qualifies", "CTH", true), Mismatch, "decided_on_uncertain", nil},
		{"preflight refusal without a model call", pf, row(pf, "undetermined", "NONE", false), Match, "abstain_correct", nil},
	} {
		got := Grade(tc.c, tc.r)
		if got.Verdict != tc.verdict || got.Class != tc.cls {
			t.Errorf("%s: verdict %q class %q, want %q %q", tc.name, got.Verdict, got.Class, tc.verdict, tc.cls)
		}
		if (tc.critOK == nil) != (got.CriterionOK == nil) || (tc.critOK != nil && got.CriterionOK != nil && *tc.critOK != *got.CriterionOK) {
			t.Errorf("%s: criterion validity %v, want %v", tc.name, got.CriterionOK, tc.critOK)
		}
	}
}

func ptr(b bool) *bool { return &b }

func TestCompleteRequiresExactlyOneRowPerCase(t *testing.T) {
	a, b := mk("textile_specialist", "c0000000000c1", "test", "x", dec("India")), mk("textile_specialist", "c0000000000c2", "test", "y", dec("India"))
	r := func(c Case) Row { return Row{ID: c.ID, Feature: c.Feature, Split: c.Split} }
	if err := Complete([]Case{a, b}, []Row{r(a), r(b)}); err != nil {
		t.Fatalf("complete set refused: %v", err)
	}
	for name, rows := range map[string][]Row{"missing": {r(a)}, "duplicate": {r(a), r(a), r(b)}, "extra": {r(a), r(b), {ID: "c9999999999zz"}}, "wrong split": {r(a), {ID: b.ID, Feature: b.Feature, Split: "dev"}}} {
		if Complete([]Case{a, b}, rows) == nil {
			t.Errorf("%s accepted", name)
		}
	}
}

func TestSummaryKeepsErrorsAbstentionsAndSeparateDenominators(t *testing.T) {
	var cases []Case
	var rows []Row
	add := func(c Case, r Row) {
		cases = append(cases, c)
		r.ID, r.Feature, r.Split = c.ID, c.Feature, c.Split
		rows = append(rows, Grade(c, r))
	}
	f := "textile_specialist"
	add(mk(f, "c0000000000d1", "test", "1", dec("India")), Row{CalledModel: true, Provider: "litellm", Origin: "India", Confidence: .95, Route: "auto"})
	add(mk(f, "c0000000000d2", "test", "2", dec("India")), Row{CalledModel: true, Provider: "litellm", Origin: "India", Confidence: .5, Route: "needs_review"})
	add(mk(f, "c0000000000d3", "test", "3", dec("India")), Row{CalledModel: true, Provider: "litellm", Origin: "China", Confidence: .95, Route: "auto"})
	add(mk(f, "c0000000000d4", "test", "4", dec("India")), Row{Error: "timeout"})
	add(mk(f, "c0000000000d5", "test", "5", Expected{Kind: "uncertain"}), Row{CalledModel: true, Provider: "litellm", Origin: "India", Confidence: .4, Route: "needs_review"})
	add(mk(f, "c0000000000d6", "test", "6", Expected{Kind: "uncertain"}), Row{CalledModel: true, Provider: "litellm", Origin: "India", Confidence: .99, Route: "auto"})
	s, err := SummarizeSplit(cases, rows)
	if err != nil {
		t.Fatal(err)
	}
	want := SplitSummary{Counts: Counts{Cases: 6, Matched: 3, Mismatched: 2, Errors: 1},
		Decided:    DecidedStats{Cases: 4, Agreed: 2, Wrong: 1, Errors: 1, Confident: 2, ConfidentCorrect: 1, ConfidentWrong: 1},
		Uncertain:  UncertainStats{Cases: 2, AbstainedCorrectly: 1, Overconfident: 1},
		ModelCalls: 5}
	if s != want {
		t.Errorf("summary\n got %+v\nwant %+v", s, want)
	}
	if s.Matched+s.Mismatched+s.Errors != s.Cases || s.Decided.Cases+s.Uncertain.Cases != s.Cases {
		t.Error("denominators do not add up")
	}
	if _, err := SummarizeSplit(cases, rows[:5]); err == nil {
		t.Error("an incomplete result set was summarised")
	}
}

func TestGateRefusesFakeAndVertexRows(t *testing.T) {
	ok := Row{ID: "a", CalledModel: true, Provider: "litellm", ModelID: "gemini-4-argon", CostUSD: .01}
	if err := Gate([]Row{ok, {ID: "pre", CalledModel: false}}); err != nil {
		t.Fatalf("a litellm row and a preflight row were refused: %v", err)
	}
	for name, r := range map[string]Row{
		"fake provider":   {ID: "a", CalledModel: true, Provider: "fake", ModelID: "x"},
		"vertex provider": {ID: "a", CalledModel: true, Provider: "vertex", ModelID: "gemini-4-argon"},
		"no provider":     {ID: "a", CalledModel: true, ModelID: "x"},
		"negative cost":   {ID: "a", CalledModel: true, Provider: "litellm", CostUSD: -1},
		"vertex model id": {ID: "a", CalledModel: true, Provider: "litellm", ModelID: "vertex_ai/gemini"},
		"a public score":  {ID: "a", CalledModel: false, Provider: "litellm", Origin: "India"},
	} {
		if Gate([]Row{r}) == nil {
			t.Errorf("%s accepted", name)
		}
	}
}

func TestPercentile(t *testing.T) {
	v := []int64{10, 20, 30, 40, 50, 60, 70, 80, 90, 100}
	if got := Percentile(v, .5); got != 50 {
		t.Errorf("p50 = %d", got)
	}
	if got := Percentile(v, .95); got != 100 {
		t.Errorf("p95 = %d", got)
	}
	if Percentile(nil, .5) != 0 {
		t.Error("empty")
	}
}

func TestFrozenGuard(t *testing.T) {
	sha := strings.Repeat("a", 40)
	git := func(head, status string) func(args ...string) (string, error) {
		return func(args ...string) (string, error) {
			if args[0] == "rev-parse" {
				return head + "\n", nil
			}
			return status, nil
		}
	}
	dir := t.TempDir()
	if err := FrozenGuard(git(sha, ""), sha, dir+"/none.jsonl"); err != nil {
		t.Fatalf("clean frozen tree refused: %v", err)
	}
	for name, tc := range map[string]struct {
		g   func(args ...string) (string, error)
		sha string
		res string
	}{
		"short sha":     {git(sha, ""), "abc", dir + "/n"},
		"wrong head":    {git(strings.Repeat("b", 40), ""), sha, dir + "/n"},
		"dirty tree":    {git(sha, " M backend/internal/engine/textile.go\n"), sha, dir + "/n"},
		"result exists": {git(sha, ""), sha, "main_test.go"},
	} {
		if FrozenGuard(tc.g, tc.sha, tc.res) == nil {
			t.Errorf("%s accepted", name)
		}
	}
	if FrozenGuard(func(...string) (string, error) { return "", errors.New("no git") }, sha, dir+"/n") == nil {
		t.Error("git failure accepted")
	}
}

func rulings() []domain.Ruling {
	d := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	return []domain.Ruling{
		{ID: "N000001", Date: d, Subject: "shirts", Facts: "cotton shirts sewn in India from fabric woven in China", Holding: "India", Origin: "India"},
		{ID: "N000002", Date: d, Subject: "shirts", Facts: "cotton shirts sewn in Vietnam from fabric woven in China", Holding: "Vietnam", Origin: "Vietnam"},
		{ID: "N000003", Date: d, Subject: "yarn", Facts: "cotton yarn spun in Turkey", Holding: "Turkey", Origin: "Turkey"},
	}
}

const answerJSON = `{"origin_country":"India","essential_character":"(c)(2) assembly","decisive_step":"sewing in India","flip":null,"confidence":0.95,"reasoning":"sewn in India"}`

func TestRunCaseExcludesSourceRulingsAndHidesTheLabel(t *testing.T) {
	m := llm.FakeText(answerJSON)
	cand, err := NewCandidate(m, "gemini-4-argon", Pools{Textile: rulings(), Pharma: rulings(), Part102: rulings()})
	if err != nil {
		t.Fatal(err)
	}
	c := mk("textile_specialist", "c0000000000e1", "test", "Cotton shirts are sewn in India from fabric woven in China.", dec("India"))
	c.ExcludeRulings = []string{"N000001"}
	c.Template, c.Rationale = "cut-and-sew-assembly", "wholly assembled in a single country"
	row := RunCase(context.Background(), cand, c)
	if row.Error != "" {
		t.Fatalf("error: %s", row.Error)
	}
	for _, id := range row.Retrieved {
		if id == "N000001" {
			t.Fatal("a source-answer ruling was retrieved")
		}
	}
	if len(row.Retrieved) == 0 {
		t.Error("no precedent retrieved, so the exclusion was not exercised")
	}
	req := m.Calls()[0]
	text := req.System
	for _, msg := range req.Messages {
		text += msg.Content
	}
	for _, bad := range []string{"N000001", c.ID, c.Template, c.Rationale, c.SourceFamily, "decided", "Expected"} {
		if strings.Contains(text, bad) {
			t.Errorf("the model request contains %q", bad)
		}
	}
	if row.Verdict != Match || !row.CalledModel || row.Provider != "fake" {
		t.Errorf("row = %+v", row)
	}
}

// A ruling the case excludes must never reach the model even when the
// retriever ignores the filter: the run records an error instead.
type ignoreFilter struct{ r retrieval.Retriever }

func (i ignoreFilter) Search(ctx context.Context, q string, k int, _ func(domain.Ruling) bool) ([]retrieval.Hit, error) {
	return i.r.Search(ctx, q, k, func(domain.Ruling) bool { return true })
}

func TestRunCaseRefusesAnExcludedRulingFromALeakyRetriever(t *testing.T) {
	m := llm.FakeText(answerJSON)
	cand, err := NewCandidate(m, "gemini-4-argon", Pools{Textile: rulings()})
	if err != nil {
		t.Fatal(err)
	}
	cand.WrapRetrievers(func(r retrieval.Retriever) retrieval.Retriever { return ignoreFilter{r} })
	c := mk("textile_specialist", "c0000000000e2", "test", "Cotton shirts are sewn in India from fabric woven in China.", dec("India"))
	c.ExcludeRulings = []string{"N000001"}
	row := RunCase(context.Background(), cand, c)
	if row.Error == "" || row.Verdict != ErrorV || len(m.Calls()) != 0 {
		t.Errorf("leak not stopped: %+v, %d model calls", row, len(m.Calls()))
	}
}

func TestRunCaseErrorsAreRowsNotPanics(t *testing.T) {
	m := &llm.Fake{Respond: func(llm.Request) (llm.Response, error) { return llm.Response{}, errors.New("gateway 500") }}
	cand, _ := NewCandidate(m, "gemini-4-argon", Pools{Textile: rulings()})
	c := mk("textile_specialist", "c0000000000e3", "test", "Cotton shirts are sewn in India.", dec("India"))
	row := RunCase(context.Background(), cand, c)
	if row.Verdict != ErrorV || row.Error == "" {
		t.Errorf("row = %+v", row)
	}
}

func TestCommittedDatasetsAreWellFormed(t *testing.T) {
	dev, test, err := LoadAll("../../../eval/challenges/specialists")
	if err != nil {
		t.Fatal(err)
	}
	if p := CheckIntegrity(dev, test, IntegrityOptions{MinTest: 100}); len(p) != 0 {
		t.Fatalf("%d problems, first: %s", len(p), p[0])
	}
	for _, c := range append(append([]Case{}, dev...), test...) {
		if l := Leaks(c); len(l) != 0 {
			t.Fatalf("%s leaks: %v", c.ID, l)
		}
	}
	for _, f := range Features {
		n := 0
		for _, c := range test {
			if c.Feature == f.ID {
				n++
			}
		}
		if n < 100 {
			t.Errorf("%s has %d reserved cases", f.ID, n)
		}
	}
}

func TestRunnerNeverUsesVertex(t *testing.T) {
	files, _ := filepath.Glob("*.go")
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for _, bad := range []string{"NewVertex", "VertexOptions", "cloud.google.com", "ProviderVertex"} {
			if strings.Contains(string(b), bad) {
				t.Errorf("%s mentions %s", f, bad)
			}
		}
	}
}

func TestGradeRoutingSeparatesSpecialistsFromNegativeControls(t *testing.T) {
	tx := mk(RoutingFeature, "c0000000000f1", "test", "t", dec("textile_102_21"))
	tx.ScenarioFamily = "textile"
	ng := mk(RoutingFeature, "c0000000000f2", "test", "n", dec("substantial_transformation"))
	ng.ScenarioFamily = "negative-control"
	row := func(c Case, regime string) Row {
		return Row{ID: c.ID, Feature: c.Feature, Split: c.Split, CalledModel: true, Provider: "litellm", Origin: regime}
	}
	for _, tc := range []struct {
		c            Case
		got          string
		verdict, cls string
	}{
		{tx, "textile_102_21", Match, "routed_correct"},
		{tx, "substantial_transformation", Mismatch, "routed_wrong"},
		{ng, "substantial_transformation", Match, "kept_general"},
		{ng, "pharma_api", Mismatch, "false_specialist"},
		{ng, "unclear", Mismatch, "unclear_label"},
	} {
		g := GradeRouting(tc.c, row(tc.c, tc.got))
		if g.Verdict != tc.verdict || g.Class != tc.cls {
			t.Errorf("%s -> %s: %s/%s, want %s/%s", tc.c.Expected.Origin, tc.got, g.Verdict, g.Class, tc.verdict, tc.cls)
		}
	}
	cases := []Case{tx, ng}
	rows := []Row{GradeRouting(tx, row(tx, "textile_102_21")), GradeRouting(ng, row(ng, "textile_102_21"))}
	s, err := SummarizeRouting(cases, rows)
	if err != nil {
		t.Fatal(err)
	}
	if s.AppropriateAll != (Counts{Cases: 1, Matched: 1}) || s.NegativeControl != (Counts{Cases: 1, Mismatched: 1}) || s.FalseSpecialist != 1 || s.Cases != 2 {
		t.Errorf("summary %+v", s)
	}
	if _, err := SummarizeRouting(cases, rows[:1]); err == nil {
		t.Error("an incomplete routing result was summarised")
	}
}

func TestCommittedRoutingDatasetsAreWellFormed(t *testing.T) {
	dev, test, err := LoadRouting("../../../eval/challenges/specialists")
	if err != nil {
		t.Fatal(err)
	}
	if p := CheckIntegrity(dev, test, IntegrityOptions{MinTest: 100}); len(p) != 0 {
		t.Fatalf("%d problems, first: %s", len(p), p[0])
	}
	var neg, app int
	for _, c := range test {
		if c.ScenarioFamily == "negative-control" {
			neg++
		} else {
			app++
		}
		if len(c.Parents) != 1 {
			t.Fatalf("%s has no visible parent", c.ID)
		}
	}
	if neg < 100 || app < 100 {
		t.Errorf("reserved routing: %d appropriate, %d negative controls", app, neg)
	}
}

func TestJapanPreflightCasesNeverCallTheModel(t *testing.T) {
	_, test, err := LoadAll("../../../eval/challenges/specialists")
	if err != nil {
		t.Fatal(err)
	}
	var pf *Case
	for i := range test {
		if test[i].Feature == "japan_engine" && test[i].Expected.Preflight {
			pf = &test[i]
			break
		}
	}
	if pf == nil {
		t.Fatal("no preflight Japan case in the reserved set")
	}
	m := llm.FakeText("{}")
	cand, err := NewCandidate(m, "gemini-4-argon", Pools{})
	if err != nil {
		t.Fatal(err)
	}
	row := RunCase(context.Background(), cand, *pf)
	if row.CalledModel || len(m.Calls()) != 0 || row.Error != "" || row.Outcome != "undetermined" || row.Verdict != Match {
		t.Errorf("preflight row = %+v, %d calls", row, len(m.Calls()))
	}
}

func TestCandidateVariantsChangeOnlyWhatDevelopmentSelected(t *testing.T) {
	m := llm.FakeText(answerJSON)
	base, _ := NewCandidate(m, "gemini-4-argon", Pools{Textile: rulings(), Pharma: rulings(), Part102: rulings()})
	c1, _ := NewCandidate(m, "gemini-4-argon", Pools{Textile: rulings(), Pharma: rulings(), Part102: rulings()})
	if err := c1.SetVariant("cand-1"); err != nil {
		t.Fatal(err)
	}
	if err := c1.SetVariant("nonsense"); err == nil {
		t.Error("unknown variant accepted")
	}
	fv := frozenVariants
	frozenVariants = map[string]string{"pharma_specialist": "cand-1"}
	defer func() { frozenVariants = fv }()
	fr, _ := NewCandidate(m, "gemini-4-argon", Pools{Textile: rulings(), Pharma: rulings(), Part102: rulings()})
	if err := fr.SetVariant("frozen"); err != nil {
		t.Fatal(err)
	}
	c := mk("pharma_specialist", "c0000000000g1", "dev", "A tablet is made from an API of India.", dec("India"))
	ver := func(cd *Candidate) string { return RunCase(context.Background(), cd, c).PromptVersion }
	if v := ver(base); strings.Contains(v, "missing") {
		t.Errorf("production run used %q", v)
	}
	if v := ver(c1); !strings.Contains(v, "missing-v1") {
		t.Errorf("cand-1 run used %q", v)
	}
	if v := ver(fr); !strings.Contains(v, "missing-v1") {
		t.Errorf("frozen pharma run used %q", v)
	}
	tx := mk("textile_specialist", "c0000000000g2", "dev", "A shirt is sewn in India.", dec("India"))
	if v := RunCase(context.Background(), fr, tx).PromptVersion; strings.Contains(v, "missing") || v != "textile-v1" {
		t.Errorf("frozen textile run used %q", v)
	}
}
