package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func mkCase(id, split, feature string, input string) Case {
	c := Case{
		ID: id, Feature: feature, Split: split, SourceFamily: "fam-" + split, ScenarioFamily: "s", Template: "t",
		VariationTags: []string{"positive"}, Sources: []SourceRef{{ID: "src1", Locator: "x"}}, Rationale: "why",
		Synthetic: true, Input: json.RawMessage(input), Expected: json.RawMessage(`{"status":"met"}`),
	}
	return c
}

func usmcaInput(good string, mats string) string {
	return fmt.Sprintf(`{"ruleset":"pilot","input":{"destination":"US","regime":"usmca","good_hs":%q,"materials":%s,"profile":{"simple_goods_confirmed":true,"materials_list_complete":true,"sets":"not_applicable","accessories_packing":"not_applicable","fungibles":"not_applicable","intermediate_materials":"not_applicable","cost_averaging":"not_applicable","de_minimis":"not_applicable","outside_party_operations":"not_applicable","special_valuation":"not_applicable","parts_route":"not_applicable"}}}`, good, mats)
}

func srcs() []SourceRec {
	return []SourceRec{{ID: "src1", Title: "t", Path: "p", SHA256: "", Kind: "primary_text"}}
}

func TestLoadCasesRejectsUnknownFieldsAndDuplicateIDs(t *testing.T) {
	good := `{"id":"a","feature":"usmca_rules","split":"dev","source_family":"f","scenario_family":"s","template":"t","variation_tags":[],"sources":[],"rationale":"r","synthetic":true,"expect_invalid":false,"input":{},"expected":{}}`
	if _, err := LoadCases(strings.NewReader(good + "\n")); err != nil {
		t.Fatalf("valid row: %v", err)
	}
	if _, err := LoadCases(strings.NewReader(strings.Replace(good, `"synthetic":true`, `"synthetic":true,"surprise":1`, 1))); err == nil {
		t.Fatal("an unknown field must be refused")
	}
	if _, err := LoadCases(strings.NewReader(good + "\n" + good + "\n")); err == nil {
		t.Fatal("a repeated id must be refused")
	}
}

func TestNormalizeKeyIgnoresIdsOrderAndAmountFormatting(t *testing.T) {
	a := mkCase("a", "dev", "usmca_rules", `{"ruleset":"pilot","input":{"good_hs":"820231","vnm":"10.50","materials":[{"id":"x1","hs":"3901","originating":"non_originating"},{"id":"x2","hs":"7326","originating":"originating"}]}}`)
	b := mkCase("b", "test", "usmca_rules", `{"ruleset":"pilot","input":{"good_hs":"820231","vnm":"10.5","materials":[{"id":"zzz","hs":"7326","originating":"originating"},{"hs":"3901","originating":"non_originating"}]}}`)
	ka, _ := NormalizeKey(a)
	kb, _ := NormalizeKey(b)
	if ka != kb {
		t.Fatalf("keys differ:\n%s\n%s", ka, kb)
	}
	c := mkCase("c", "test", "usmca_rules", `{"ruleset":"pilot","input":{"good_hs":"820231","vnm":"10.51","materials":[]}}`)
	kc, _ := NormalizeKey(c)
	if kc == ka {
		t.Fatal("a different amount must give a different key")
	}
}

func TestIntegrityRejectsCrossSplitDuplicate(t *testing.T) {
	in := usmcaInput("820231", `[{"hs":"3901","originating":"non_originating"}]`)
	dev := []Case{mkCase("d1", "dev", "usmca_rules", in)}
	test := []Case{mkCase("t1", "test", "usmca_rules", usmcaInput("820231", `[{"id":"renamed","hs":"3901","originating":"non_originating"}]`))}
	probs := CheckIntegrity(dev, test, srcs(), IntegrityOptions{MinTest: 1})
	if !containsAny(probs, "cross-split") {
		t.Fatalf("expected a cross-split duplicate problem, got %v", probs)
	}
}

func TestIntegrityRejectsDuplicateInsideSplit(t *testing.T) {
	in := usmcaInput("820231", `[]`)
	dev := []Case{mkCase("d1", "dev", "usmca_rules", in), mkCase("d2", "dev", "usmca_rules", in)}
	if p := CheckIntegrity(dev, nil, srcs(), IntegrityOptions{}); !containsAny(p, "duplicate") {
		t.Fatalf("got %v", p)
	}
}

func TestIntegrityMinimumTestCasesPerFeature(t *testing.T) {
	var test []Case
	for i := 0; i < 99; i++ {
		test = append(test, mkCase(fmt.Sprintf("t%d", i), "test", "usmca_rules", usmcaInput("820231", fmt.Sprintf(`[{"hs":"39%02d","originating":"non_originating"}]`, i))))
	}
	probs := CheckIntegrity(nil, test, srcs(), IntegrityOptions{MinTest: 100, Features: []string{"usmca_rules", "textile_rules"}})
	if !containsAny(probs, "usmca_rules", "99") || !containsAny(probs, "textile_rules", "0") {
		t.Fatalf("expected both features to be short, got %v", probs)
	}
}

func TestIntegrityFieldRules(t *testing.T) {
	base := mkCase("t1", "test", "usmca_rules", usmcaInput("820231", `[]`))
	cases := map[string]func(*Case){
		"no rationale":   func(c *Case) { c.Rationale = "" },
		"no sources":     func(c *Case) { c.Sources = nil },
		"unknown source": func(c *Case) { c.Sources = []SourceRef{{ID: "nope", Locator: "x"}} },
		"no locator":     func(c *Case) { c.Sources = []SourceRef{{ID: "src1"}} },
		"wrong split":    func(c *Case) { c.Split = "dev" },
		"empty expected": func(c *Case) { c.Expected = nil },
		"no family":      func(c *Case) { c.SourceFamily = "" },
		"no template":    func(c *Case) { c.Template = "" },
		"not synthetic":  func(c *Case) { c.Synthetic = false },
		"no tags":        func(c *Case) { c.VariationTags = nil },
		"missing parent": func(c *Case) { c.Parent = "ghost" },
		"bad feature":    func(c *Case) { c.Feature = "other" },
	}
	for name, mut := range cases {
		c := base
		mut(&c)
		if p := CheckIntegrity(nil, []Case{c}, srcs(), IntegrityOptions{}); len(p) == 0 {
			t.Errorf("%s: no problem reported", name)
		}
	}
	if p := CheckIntegrity(nil, []Case{base}, srcs(), IntegrityOptions{}); len(p) != 0 {
		t.Fatalf("a well-formed case reported %v", p)
	}
}

func TestIntegrityParentMustBeInSameSplit(t *testing.T) {
	d := mkCase("d1", "dev", "usmca_rules", usmcaInput("820231", `[]`))
	c := mkCase("t1", "test", "usmca_rules", usmcaInput("820713", `[]`))
	c.Parent = "d1"
	if p := CheckIntegrity([]Case{d}, []Case{c}, srcs(), IntegrityOptions{}); !containsAny(p, "parent") {
		t.Fatalf("got %v", p)
	}
}

func TestIntegrityInvalidCasesNeedAnErrorField(t *testing.T) {
	c := mkCase("t1", "test", "usmca_rules", `{"ruleset":"pilot","raw":"{}"}`)
	c.ExpectInvalid = true
	c.Expected = json.RawMessage(`{"invalid":true}`)
	if p := CheckIntegrity(nil, []Case{c}, srcs(), IntegrityOptions{}); !containsAny(p, "error_field") {
		t.Fatalf("got %v", p)
	}
	c.Expected = json.RawMessage(`{"invalid":true,"error_field":"body"}`)
	if p := CheckIntegrity(nil, []Case{c}, srcs(), IntegrityOptions{}); len(p) != 0 {
		t.Fatalf("got %v", p)
	}
	c.ExpectInvalid = false
	if p := CheckIntegrity(nil, []Case{c}, srcs(), IntegrityOptions{}); !containsAny(p, "expect_invalid") {
		t.Fatalf("an invalid expectation without the flag must be refused: %v", p)
	}
}

func TestSourceFilesAreVerifiedByHash(t *testing.T) {
	dir := t.TempDir()
	body := []byte("clause text")
	if err := os.WriteFile(filepath.Join(dir, "s.txt"), body, 0o644); err != nil {
		t.Fatal(err)
	}
	h := sha256.Sum256(body)
	ok := []SourceRec{{ID: "src1", Path: "s.txt", SHA256: hex.EncodeToString(h[:]), Kind: "primary_text"}}
	if p := CheckSources(ok, dir); len(p) != 0 {
		t.Fatalf("got %v", p)
	}
	bad := []SourceRec{{ID: "src1", Path: "s.txt", SHA256: "00", Kind: "primary_text"}}
	if p := CheckSources(bad, dir); len(p) == 0 {
		t.Fatal("a wrong hash must be reported")
	}
	missing := []SourceRec{{ID: "src1", Path: "gone.txt", SHA256: "00", Kind: "primary_text"}}
	if p := CheckSources(missing, dir); len(p) == 0 {
		t.Fatal("a missing file must be reported")
	}
}

func TestScoreUSMCAMatchAndMismatchAreRetained(t *testing.T) {
	c := mkCase("t1", "test", "usmca_rules", usmcaInput("820231", `[{"hs":"390110","originating":"non_originating"}]`))
	c.Expected = json.RawMessage(`{"status":"met","alternatives":{"A":"satisfied"}}`)
	row := Score(c)
	if row.Status != StatusMatch {
		t.Fatalf("row = %+v", row)
	}
	c.Expected = json.RawMessage(`{"status":"not_met"}`)
	row = Score(c)
	if row.Status != StatusMismatch || row.Why == "" || len(row.Actual) == 0 {
		t.Fatalf("a mismatch must keep its actual output and reason: %+v", row)
	}
}

func TestScoreUSMCAInvalidInputsAreExpectedRefusals(t *testing.T) {
	c := mkCase("t1", "test", "usmca_rules", usmcaInput("82", `[]`))
	c.ExpectInvalid = true
	c.Expected = json.RawMessage(`{"invalid":true,"error_field":"good_hs"}`)
	if row := Score(c); row.Status != StatusMatch {
		t.Fatalf("row = %+v", row)
	}
	c.Expected = json.RawMessage(`{"invalid":true,"error_field":"destination"}`)
	if row := Score(c); row.Status != StatusMismatch {
		t.Fatalf("the wrong refusal field is a mismatch: %+v", row)
	}
	// a valid input expected to be refused is a mismatch, not an authoring pass
	v := mkCase("t2", "test", "usmca_rules", usmcaInput("820231", `[]`))
	v.ExpectInvalid = true
	v.Expected = json.RawMessage(`{"invalid":true,"error_field":"good_hs"}`)
	if row := Score(v); row.Status != StatusMismatch {
		t.Fatalf("row = %+v", row)
	}
}

func TestScoreRecoversPanicsAsErrors(t *testing.T) {
	c := mkCase("t1", "test", "usmca_rules", `{"ruleset":"no-such-ruleset","input":{}}`)
	row := Score(c)
	if row.Status != StatusError || row.Why == "" {
		t.Fatalf("an unrunnable case is an error row, not a skipped one: %+v", row)
	}
}

func TestValidityBeforeFreezeSeparatesAuthoringErrorsFromRefusals(t *testing.T) {
	good := mkCase("g", "test", "usmca_rules", usmcaInput("820231", `[]`))
	bad := mkCase("b", "test", "usmca_rules", usmcaInput("82", `[]`))
	bad.ExpectInvalid = true
	bad.Expected = json.RawMessage(`{"invalid":true,"error_field":"good_hs"}`)
	accident := mkCase("a", "test", "usmca_rules", usmcaInput("82", `[]`)) // invalid input, but not declared invalid
	if p := CheckInputValidity([]Case{good, bad}); len(p) != 0 {
		t.Fatalf("got %v", p)
	}
	if p := CheckInputValidity([]Case{accident}); len(p) != 1 || !strings.Contains(p[0], "authoring") {
		t.Fatalf("an accidentally invalid input must be an authoring error: %v", p)
	}
	declaredButValid := mkCase("v", "test", "usmca_rules", usmcaInput("820231", `[]`))
	declaredButValid.ExpectInvalid = true
	declaredButValid.Expected = json.RawMessage(`{"invalid":true,"error_field":"good_hs"}`)
	if p := CheckInputValidity([]Case{declaredButValid}); len(p) != 1 {
		t.Fatalf("a declared-invalid case that validates is an authoring error: %v", p)
	}
}

func TestSummaryKeepsErrorsAndAbstentionsInEveryDenominator(t *testing.T) {
	cases := []Case{
		mkCase("t1", "test", "usmca_rules", usmcaInput("820231", `[]`)),
		mkCase("t2", "test", "usmca_rules", usmcaInput("820713", `[]`)),
		mkCase("t3", "test", "usmca_rules", usmcaInput("950300", `[]`)),
		mkCase("t4", "test", "usmca_rules", usmcaInput("820750", `[]`)),
		mkCase("t5", "test", "usmca_rules", usmcaInput("82", `[]`)),
	}
	exp := []string{`{"status":"met"}`, `{"status":"needs_information"}`, `{"status":"not_met"}`, `{"status":"not_supported"}`, `{"invalid":true,"error_field":"good_hs"}`}
	for i := range cases {
		cases[i].Expected = json.RawMessage(exp[i])
	}
	cases[4].ExpectInvalid = true
	rows := []Row{
		{ID: "t1", Feature: "usmca_rules", Split: "test", Status: StatusMatch, Expected: cases[0].Expected, Actual: json.RawMessage(`{"status":"met"}`)},
		{ID: "t2", Feature: "usmca_rules", Split: "test", Status: StatusMismatch, Expected: cases[1].Expected, Actual: json.RawMessage(`{"status":"met"}`)},
		{ID: "t3", Feature: "usmca_rules", Split: "test", Status: StatusMismatch, Expected: cases[2].Expected, Actual: json.RawMessage(`{"status":"needs_information"}`)},
		{ID: "t4", Feature: "usmca_rules", Split: "test", Status: StatusError, Expected: cases[3].Expected, Why: "boom"},
		{ID: "t5", Feature: "usmca_rules", Split: "test", Status: StatusMatch, Expected: cases[4].Expected, Actual: json.RawMessage(`{"invalid":true,"error_field":"good_hs"}`)},
	}
	sum, err := SummarizeSplit("usmca_rules", cases, rows)
	if err != nil {
		t.Fatal(err)
	}
	if sum.Cases != 5 || sum.Matched != 2 || sum.Mismatched != 2 || sum.Errors != 1 {
		t.Fatalf("counts = %+v", sum)
	}
	if sum.Cases != sum.Matched+sum.Mismatched+sum.Errors {
		t.Fatal("matched + mismatched + errors must equal the cases")
	}
	x := sum.USMCA
	if x == nil || x.ExpectedDecisive != 2 || x.EngineDecisive != 2 || x.DecisiveCorrect != 1 || x.FalseDecisive != 1 || x.UnexpectedAbstain != 1 || x.ExpectedAbstain != 2 || x.InvalidExpected != 1 || x.InvalidRefusedCorrectly != 1 {
		t.Fatalf("usmca detail = %+v", x)
	}
	if x.Errors != 1 {
		t.Fatalf("an error row stays in its own count: %+v", x)
	}
}

func TestSummaryRefusesMissingOrExtraRows(t *testing.T) {
	cases := []Case{mkCase("t1", "test", "usmca_rules", usmcaInput("820231", `[]`)), mkCase("t2", "test", "usmca_rules", usmcaInput("820713", `[]`))}
	rows := []Row{{ID: "t1", Feature: "usmca_rules", Split: "test", Status: StatusMatch}}
	if _, err := SummarizeSplit("usmca_rules", cases, rows); err == nil {
		t.Fatal("a case without a result row must be refused")
	}
	rows = append(rows, Row{ID: "t2", Feature: "usmca_rules", Split: "test", Status: StatusMatch}, Row{ID: "ghost", Feature: "usmca_rules", Split: "test", Status: StatusMatch})
	if _, err := SummarizeSplit("usmca_rules", cases, rows); err == nil {
		t.Fatal("a result row without a case must be refused")
	}
	rows = []Row{{ID: "t1", Status: StatusMatch}, {ID: "t1", Status: StatusMatch}, {ID: "t2", Status: StatusMatch}}
	if _, err := SummarizeSplit("usmca_rules", cases, rows); err == nil {
		t.Fatal("a repeated result row must be refused")
	}
	rows = []Row{{ID: "t1", Status: "weird"}, {ID: "t2", Status: StatusMatch}}
	if _, err := SummarizeSplit("usmca_rules", cases, rows); err == nil {
		t.Fatal("an unknown row status must be refused")
	}
}

func TestFamilyAccountingShowsSharedFamiliesWithDev(t *testing.T) {
	d1 := mkCase("d1", "dev", "usmca_rules", usmcaInput("820231", `[]`))
	d1.SourceFamily, d1.Template = "A", "T1"
	t1 := mkCase("t1", "test", "usmca_rules", usmcaInput("820713", `[]`))
	t1.SourceFamily, t1.Template = "B", "T1"
	t2 := mkCase("t2", "test", "usmca_rules", usmcaInput("950300", `[]`))
	t2.SourceFamily, t2.Template, t2.Parent = "A", "T2", "t1"
	fa := Families([]Case{d1}, []Case{t1, t2})
	if fa.TestSourceFamilies != 2 || fa.SourceFamiliesSharedWithDev != 1 || fa.TestTemplates != 2 || fa.TemplatesSharedWithDev != 1 || fa.CasesWithParent != 1 {
		t.Fatalf("families = %+v", fa)
	}
}

func TestTariffCanonicalisationIsExactIntegers(t *testing.T) {
	if got := rate1e4(0.0725); got != 725 {
		t.Fatalf("rate1e4 = %d", got)
	}
	if got := rate1e4(0.07250000000001); got != 725 {
		t.Fatalf("a float tail must not change the canonical rate: %d", got)
	}
	if got := rate1e4(-0.15); got != -1500 {
		t.Fatalf("got %d", got)
	}
	if got := toCents(72500.00000000001); got != 7250000 {
		t.Fatalf("toCents = %d", got)
	}
	if got := toCents(-150000.004); got != -15000000 {
		t.Fatalf("toCents = %d", got)
	}
}

func TestFrozenGuard(t *testing.T) {
	sha := strings.Repeat("a", 40)
	run := func(head, status string) func(args ...string) (string, error) {
		return func(args ...string) (string, error) {
			if args[0] == "rev-parse" {
				return head + "\n", nil
			}
			return status, nil
		}
	}
	if err := FrozenGuard(run(sha, ""), sha, "/nonexistent/results-test.jsonl"); err != nil {
		t.Fatalf("clean tree at the frozen commit: %v", err)
	}
	if err := FrozenGuard(run(strings.Repeat("b", 40), ""), sha, "/nonexistent/x"); err == nil {
		t.Fatal("HEAD that is not the frozen commit must be refused")
	}
	if err := FrozenGuard(run(sha, " M eval/challenges/legal-rules/usmca-test.jsonl\n"), sha, "/nonexistent/x"); err == nil {
		t.Fatal("a dirty candidate or dataset tree must be refused")
	}
	if err := FrozenGuard(run(sha, ""), "abc", "/nonexistent/x"); err == nil {
		t.Fatal("a short sha must be refused")
	}
	existing := filepath.Join(t.TempDir(), "results-test.jsonl")
	if err := os.WriteFile(existing, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := FrozenGuard(run(sha, ""), sha, existing); err == nil {
		t.Fatal("a second scoring run must not overwrite the first")
	}
}

func TestRowsRoundTripAsDeterministicBytes(t *testing.T) {
	rows := []Row{
		{ID: "a", Feature: "usmca_rules", Split: "dev", Status: StatusMatch, Expected: json.RawMessage(`{"status":"met"}`), Actual: json.RawMessage(`{"status":"met"}`)},
		{ID: "b", Feature: "usmca_rules", Split: "dev", Status: StatusMismatch, Expected: json.RawMessage(`{"status":"met"}`), Actual: json.RawMessage(`{"status":"not_met"}`), Why: "status"},
	}
	var b1, b2 bytes.Buffer
	if err := WriteRows(&b1, rows); err != nil {
		t.Fatal(err)
	}
	if err := WriteRows(&b2, rows); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(b1.Bytes(), b2.Bytes()) || strings.Count(b1.String(), "\n") != 2 {
		t.Fatal("rows must serialise identically, one line each")
	}
	got, err := ReadRows(bytes.NewReader(b1.Bytes()))
	if err != nil || len(got) != 2 || got[1].Why != "status" {
		t.Fatalf("read back %v %v", got, err)
	}
}

func TestDevelopmentDataIsFullyMatchedOrDocumented(t *testing.T) {
	// Every committed development case is scored through the candidate; the
	// test fails on a mismatch that is not listed in the committed defects file.
	root := dataRoot()
	if _, err := os.Stat(filepath.Join(root, "usmca-dev.jsonl")); err != nil {
		t.Skip("datasets not generated")
	}
	for _, f := range Features {
		cases := mustLoad(t, filepath.Join(root, f.Prefix+"-dev.jsonl"))
		for _, c := range cases {
			row := Score(c)
			if row.Status != StatusMatch {
				t.Errorf("%s: %s: %s", c.ID, row.Status, row.Why)
			}
		}
	}
}

func TestCommittedDatasetsPassIntegrity(t *testing.T) {
	root := dataRoot()
	if _, err := os.Stat(filepath.Join(root, "usmca-test.jsonl")); err != nil {
		t.Skip("datasets not generated")
	}
	dev, test, srcs, err := LoadAll(root)
	if err != nil {
		t.Fatal(err)
	}
	probs := CheckIntegrity(dev, test, srcs, IntegrityOptions{MinTest: 100, Features: featureIDs()})
	probs = append(probs, CheckSources(srcs, repoRoot())...)
	if len(probs) > 0 {
		t.Fatal(strings.Join(probs, "\n"))
	}
	if probs := CheckInputValidity(append(append([]Case{}, dev...), test...)); len(probs) > 0 {
		t.Fatal(strings.Join(probs, "\n"))
	}
}

func TestCommittedResultsMatchTheCandidateAndSummary(t *testing.T) {
	root := dataRoot()
	if _, err := os.Stat(filepath.Join(root, "results-test.jsonl")); err != nil {
		t.Skip("no scored results committed yet")
	}
	if err := CheckCommitted(root); err != nil {
		t.Fatal(err)
	}
}

func containsAny(probs []string, parts ...string) bool {
	for _, p := range probs {
		ok := true
		for _, part := range parts {
			if !strings.Contains(p, part) {
				ok = false
			}
		}
		if ok {
			return true
		}
	}
	return false
}

func mustLoad(t *testing.T, path string) []Case {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	cs, err := LoadCases(f)
	if err != nil {
		t.Fatal(err)
	}
	return cs
}
