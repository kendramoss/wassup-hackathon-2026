package main

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func fixtureInput(feature string, i int) json.RawMessage {
	hs := "9403.60"
	if feature == "rcep" {
		hs = "8471.30"
	}
	return json.RawMessage(fmt.Sprintf(`{"hs":%q,"produced_in":"Malaysia","fob":%d,"materials":[{"name":"Imported input","hs":"4410.90","value":10,"country":"Belgium","status":"non_originating"}]}`, hs, 100+i))
}

func fixtureCase(feature, split string, i int) Case {
	c := Case{
		ID: fmt.Sprintf("%s-%s-%03d", feature, split, i), Split: split, Feature: feature,
		SourceFamilyID: "fam-" + fmt.Sprint(i%5), ScenarioFamily: "scn-" + fmt.Sprint(i%3),
		VariationTags: []string{"t"}, Sources: []SourceRef{{Source: "s", Locator: "Art. 1"}},
		Rationale: "because", Input: fixtureInput(feature, i),
	}
	act, _ := callEngine(feature, c.Input)
	c.Expect = Expect{Outcome: act.Outcome}
	c.ExpectedAbstention = act.Outcome == "undetermined"
	return c
}

func fixtureDataset(t *testing.T, feature, split string, n int) *Dataset {
	t.Helper()
	ds := &Dataset{SchemaVersion: schemaVersion, DatasetVer: "t", EvidenceKind: evidenceKind, Feature: feature, Split: split,
		Sources: map[string]json.RawMessage{"s": json.RawMessage(`{"title":"x"}`)}, CaseCount: n}
	for i := 0; i < n; i++ {
		ds.Cases = append(ds.Cases, fixtureCase(feature, split, i))
	}
	b, err := json.Marshal(ds)
	if err != nil {
		t.Fatal(err)
	}
	out, err := parseDataset(b)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestFixtureIsValid(t *testing.T) {
	for _, f := range features {
		if err := fixtureDataset(t, f, "test", MinTestCases).Validate(); err != nil {
			t.Fatalf("%s: %v", f, err)
		}
	}
}

func TestValidateRejectsDuplicateIDs(t *testing.T) {
	ds := fixtureDataset(t, "atiga", "dev", 5)
	ds.Cases[3].ID = ds.Cases[1].ID
	if err := ds.Validate(); err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("duplicate id accepted: %v", err)
	}
}

func TestValidateRejectsMissingCases(t *testing.T) {
	ds := fixtureDataset(t, "atiga", "dev", 5)
	ds.Cases = ds.Cases[:4]
	if err := ds.Validate(); err == nil || !strings.Contains(err.Error(), "case_count") {
		t.Fatalf("a dataset with a case removed was accepted: %v", err)
	}
}

func TestValidateRequiresMinimumTestCases(t *testing.T) {
	ds := fixtureDataset(t, "rcep", "test", MinTestCases-1)
	if err := ds.Validate(); err == nil || !strings.Contains(err.Error(), "at least") {
		t.Fatalf("a short reserved test set was accepted: %v", err)
	}
}

func TestValidateRejectsMissingProvenance(t *testing.T) {
	cases := map[string]func(*Case){
		"source":    func(c *Case) { c.Sources = nil },
		"unknown":   func(c *Case) { c.Sources = []SourceRef{{Source: "nope", Locator: "x"}} },
		"locator":   func(c *Case) { c.Sources = []SourceRef{{Source: "s"}} },
		"rationale": func(c *Case) { c.Rationale = " " },
		"tags":      func(c *Case) { c.VariationTags = nil },
		"family":    func(c *Case) { c.SourceFamilyID = "" },
		"scenario":  func(c *Case) { c.ScenarioFamily = "" },
		"outcome":   func(c *Case) { c.Expect.Outcome = "maybe" },
		"abstain":   func(c *Case) { c.ExpectedAbstention = !c.ExpectedAbstention },
	}
	for name, mutate := range cases {
		ds := fixtureDataset(t, "akfta", "dev", 3)
		mutate(&ds.Cases[1])
		if err := ds.Validate(); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestValidateRejectsFabricatedAgreementMembership(t *testing.T) {
	yes := true
	mat := func(country, status string) json.RawMessage {
		return json.RawMessage(fmt.Sprintf(`{"hs":"9403.60","produced_in":"Malaysia","fob":100,"materials":[{"hs":"4410.90","value":10,"country":%q,"status":%q}]}`, country, status))
	}
	tests := []struct {
		name, feature, country string
		tags                   []string
		acc                    *bool
		producer               string
		wantErr                bool
	}{
		{"korea is not an ATIGA party", "atiga", "South Korea", []string{"member-origin"}, &yes, "", true},
		{"korea cumulates under AKFTA", "akfta", "South Korea", []string{"member-origin"}, &yes, "", false},
		{"japan is not an AKFTA party", "akfta", "Japan", []string{"member-origin"}, nil, "", true},
		{"china is not an ACFTA... Korea party", "acfta", "South Korea", nil, &yes, "", true},
		{"japan is an RCEP party", "rcep", "Japan", []string{"member-origin"}, &yes, "", false},
		{"india is no RCEP party", "rcep", "India", []string{"member-origin"}, &yes, "", true},
		{"claimed outsider is a member", "atiga", "Thailand", []string{"non-member-origin"}, nil, "", true},
		{"claimed non-member producer is a member", "atiga", "Belgium", []string{"non-member-producer"}, nil, "", true},
	}
	for _, tc := range tests {
		ds := fixtureDataset(t, tc.feature, "dev", 1)
		c := &ds.Cases[0]
		c.Input = mat(tc.country, "originating")
		c.VariationTags = append([]string{"t"}, tc.tags...)
		c.Expect.Accumulation = tc.acc
		err := validateMembership(*c)
		if (err != nil) != tc.wantErr {
			t.Errorf("%s: err=%v wantErr=%v", tc.name, err, tc.wantErr)
		}
	}
}

func TestValidateRejectsRepeatedNormalizedInput(t *testing.T) {
	ds := fixtureDataset(t, "atiga", "dev", 3)
	ds.Cases[2].Input = json.RawMessage(strings.ReplaceAll(string(ds.Cases[1].Input), "Imported input", "Another name for it"))
	ds.Cases[2].Input = json.RawMessage(strings.ReplaceAll(string(ds.Cases[2].Input), `"9403.60"`, `"940360"`))
	if err := ds.Validate(); err == nil || !strings.Contains(err.Error(), "normalized input") {
		t.Fatalf("a renamed copy of a case was accepted: %v", err)
	}
}

func TestValidateSplitsRejectsCrossSplitDuplicates(t *testing.T) {
	dev := fixtureDataset(t, "acfta", "dev", 3)
	test := fixtureDataset(t, "acfta", "test", MinTestCases)
	test.Cases[0].Input = dev.Cases[1].Input
	if err := ValidateSplits(dev, test); err == nil || !strings.Contains(err.Error(), "same normalized input") {
		t.Fatalf("development input reused in the test set: %v", err)
	}
	test = fixtureDataset(t, "acfta", "test", MinTestCases)
	test.Cases[0].ID = dev.Cases[0].ID
	if err := ValidateSplits(dev, test); err == nil || !strings.Contains(err.Error(), "both splits") {
		t.Fatalf("shared case id accepted: %v", err)
	}
}

func TestWrongExpectedResultIsAMismatch(t *testing.T) {
	c := fixtureCase("atiga", "test", 1)
	if r := ScoreCase(c); r.Status != Matched {
		t.Fatalf("control case should match: %+v", r)
	}
	c.Expect.Outcome = map[string]string{"yes": "no", "no": "yes", "undetermined": "yes"}[c.Expect.Outcome]
	c.ExpectedAbstention = false
	r := ScoreCase(c)
	if r.Status != Mismatched || len(r.Failures) == 0 {
		t.Fatalf("a wrong label must mismatch and say why: %+v", r)
	}
	c = fixtureCase("atiga", "test", 1)
	c.Expect.Alternative = map[string]string{"RVC40": "banana"}
	if r := ScoreCase(c); r.Status != Mismatched {
		t.Fatalf("a wrong alternative must mismatch: %+v", r)
	}
	c = fixtureCase("atiga", "test", 1)
	yes := true
	c.Expect.Accumulation = &yes
	if r := ScoreCase(c); r.Status != Mismatched {
		t.Fatalf("a wrong accumulation flag must mismatch: %+v", r)
	}
}

func TestEngineErrorsStayInTheDenominator(t *testing.T) {
	good := fixtureCase("rcep", "test", 1)
	bad := fixtureCase("rcep", "test", 2)
	bad.Input = json.RawMessage(`{"hs":"84","produced_in":"Japan"}`)
	wrongShape := fixtureCase("rcep", "test", 3)
	wrongShape.Input = json.RawMessage(`{"hs":"8471.30","produced_in":"Japan","unknown_field":1}`)
	rs := []Result{ScoreCase(good), ScoreCase(bad), ScoreCase(wrongShape)}
	got := Tally(rs)
	if got.Cases != 3 || got.Matched != 1 || got.Errors != 2 || got.Mismatched != 0 {
		t.Fatalf("tally %+v: errors must be counted, not dropped (%+v)", got, rs)
	}
}

func TestRefusalExpectations(t *testing.T) {
	c := fixtureCase("atiga", "test", 1)
	c.Input = json.RawMessage(`{"hs":"9403.60","produced_in":"Malaysia","fob":-5,"materials":[]}`)
	c.Expect = Expect{Outcome: "refused"}
	if r := ScoreCase(c); r.Status != Matched {
		t.Fatalf("an invalid input expected to be refused should match: %+v", r)
	}
	c = fixtureCase("atiga", "test", 1)
	c.Expect = Expect{Outcome: "refused"}
	if r := ScoreCase(c); r.Status != Mismatched {
		t.Fatalf("a valid input answered when a refusal was expected must mismatch: %+v", r)
	}
	c = fixtureCase("acfta", "test", 1)
	c.Input = json.RawMessage(`{"hs":"9403.60","produced_in":"Thailand","fob":100,"method":"direct","materials":[]}`)
	c.Expect = Expect{Outcome: "refused"}
	if r := ScoreCase(c); r.Status != Matched {
		t.Fatalf("ACFTA refuses the direct method: %+v", r)
	}
}

type fixtureSet struct {
	dev, test       map[string]*Dataset
	devRes, testRes map[string]ResultsFile
}

func newFixtureSet(t *testing.T) fixtureSet {
	t.Helper()
	s := fixtureSet{dev: map[string]*Dataset{}, test: map[string]*Dataset{}, devRes: map[string]ResultsFile{}, testRes: map[string]ResultsFile{}}
	cand := strings.Repeat("a", 40)
	for _, f := range features {
		s.dev[f] = fixtureDataset(t, f, "dev", 4)
		s.test[f] = fixtureDataset(t, f, "test", MinTestCases)
		for i := range s.test[f].Cases {
			s.test[f].Cases[i].Input = fixtureInput(f, 1000+i)
		}
		b, _ := json.Marshal(s.test[f])
		ds, err := parseDataset(b)
		if err != nil {
			t.Fatal(err)
		}
		s.test[f] = ds
		s.devRes[f] = NewResultsFile(s.dev[f], nil, cand, "", ScoreDataset(s.dev[f]))
		s.testRes[f] = NewResultsFile(s.test[f], s.dev[f], cand, "", ScoreDataset(s.test[f]))
	}
	return s
}

func (s fixtureSet) summary() Summary {
	lat := map[string]Latency{}
	for _, f := range features {
		lat[f] = Latency{0.1, 0.2}
	}
	return BuildSummary(s.dev, s.test, s.devRes, s.testRes, nil, lat)
}

func TestSummaryAcceptsConsistentResults(t *testing.T) {
	s := newFixtureSet(t)
	sum := s.summary()
	if err := ValidateSummary(sum, s.dev, s.test, s.devRes, s.testRes); err != nil {
		t.Fatal(err)
	}
	if len(sum.Features) != 4 || sum.Features[0].ID != "atiga" || sum.Features[3].ID != "rcep" {
		t.Fatalf("features %+v", sum.Features)
	}
}

func TestSummaryRejectsMismatchedNumbers(t *testing.T) {
	mutations := map[string]func(*Summary, *fixtureSet){
		"matched inflated":     func(s *Summary, _ *fixtureSet) { s.Features[0].Test.Matched++ },
		"errors dropped":       func(s *Summary, _ *fixtureSet) { s.Features[1].Test.Cases--; s.Features[1].Test.Matched-- },
		"dataset hash":         func(s *Summary, _ *fixtureSet) { s.Features[2].DatasetSHA256 = strings.Repeat("0", 64) },
		"candidate not sha":    func(s *Summary, _ *fixtureSet) { s.Features[3].CandidateRevision = "main" },
		"source families":      func(s *Summary, _ *fixtureSet) { s.Features[0].SourceFamilies += 3 },
		"development matched":  func(s *Summary, _ *fixtureSet) { s.Features[1].Development.Matched++ },
		"feature removed":      func(s *Summary, _ *fixtureSet) { s.Features = s.Features[:3] },
		"feature order":        func(s *Summary, _ *fixtureSet) { s.Features[0], s.Features[1] = s.Features[1], s.Features[0] },
		"negative cost":        func(s *Summary, _ *fixtureSet) { s.Features[0].CostUSD = -1 },
		"no limitations":       func(s *Summary, _ *fixtureSet) { s.Features[2].Limitations = nil },
		"abstention rewritten": func(s *Summary, _ *fixtureSet) { s.Features[0].Abstention.Expected += 2 },
		"wrong evidence kind":  func(s *Summary, _ *fixtureSet) { s.EvidenceKind = "independent_real_world" },
		"p95 below p50":        func(s *Summary, _ *fixtureSet) { s.Features[0].LatencyP95Ms = 0 },
		"mismatches hidden":    func(*Summary, *fixtureSet) {},
	}
	for name, mutate := range mutations {
		s := newFixtureSet(t)
		sum := s.summary()
		mutate(&sum, &s)
		if name == "mismatches hidden" {
			s.testRes["atiga"].Results[0].Status = Mismatched
			r := s.testRes["atiga"]
			r.Totals = Tally(r.Results)
			s.testRes["atiga"] = r
		}
		if err := ValidateSummary(sum, s.dev, s.test, s.devRes, s.testRes); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestResultsMustCoverEveryCaseOnce(t *testing.T) {
	s := newFixtureSet(t)
	short := s.testRes["atiga"]
	short.Results = short.Results[:len(short.Results)-1]
	short.Totals = Tally(short.Results)
	if err := ValidateResults(s.test["atiga"], short); err == nil {
		t.Fatal("a results file missing a case was accepted")
	}
	dup := s.testRes["akfta"]
	dup.Results = append([]Result{}, dup.Results...)
	dup.Results[1] = dup.Results[0]
	dup.Totals = Tally(dup.Results)
	if err := ValidateResults(s.test["akfta"], dup); err == nil {
		t.Fatal("a results file with a duplicated case was accepted")
	}
	stale := s.testRes["acfta"]
	stale.DatasetSHA256 = strings.Repeat("1", 64)
	if err := ValidateResults(s.test["acfta"], stale); err == nil {
		t.Fatal("results scored against another dataset were accepted")
	}
	forged := s.testRes["rcep"]
	forged.Totals.Matched--
	forged.Totals.Errors++
	if err := ValidateResults(s.test["rcep"], forged); err == nil {
		t.Fatal("totals that disagree with the results were accepted")
	}
}

func TestMetricsKeepAbstentionAndErrorsVisible(t *testing.T) {
	ds := fixtureDataset(t, "atiga", "test", 4)
	ds.Cases[0].Expect = Expect{Outcome: "undetermined"}
	ds.Cases[0].ExpectedAbstention = true
	ds.Cases[0].Input = json.RawMessage(`{"hs":"9403.60","produced_in":"","fob":100,"materials":[]}`)
	ds.Cases[1].Expect = Expect{Outcome: "yes"}
	ds.Cases[1].ExpectedAbstention = false
	rs := ScoreDataset(ds)
	m := ComputeMetrics(ds, rs, nil)
	if m.Abstention.Expected != 1 || m.Abstention.CorrectlyAbstains != 1 {
		t.Fatalf("abstention %+v", m.Abstention)
	}
	if m.YesRecall.Den != 3 || m.YesRecall.Num != 3 {
		t.Fatalf("recall denominator %+v", m.YesRecall)
	}
	if m.Expected["undetermined"] != 1 {
		t.Fatalf("expected outcomes %+v", m.Expected)
	}
}
