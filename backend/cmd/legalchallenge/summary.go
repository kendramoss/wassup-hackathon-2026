package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

// USMCADetail keeps abstentions, refusals and errors in every denominator.
// Decisive means met or not_met; abstaining means needs_information or
// not_supported.
type USMCADetail struct {
	ExpectedDecisive        int     `json:"expected_decisive"`
	EngineDecisive          int     `json:"engine_decisive"`
	DecisiveCorrect         int     `json:"decisive_correct"`
	DecisivePrecision       float64 `json:"decisive_precision"`
	DecisiveRecall          float64 `json:"decisive_recall"`
	FalseDecisive           int     `json:"false_decisive"`
	UnexpectedAbstain       int     `json:"unexpected_abstain"`
	ExpectedAbstain         int     `json:"expected_abstain"`
	AbstainCorrect          int     `json:"abstain_correct"`
	InvalidExpected         int     `json:"invalid_expected"`
	InvalidRefusedCorrectly int     `json:"invalid_refused_correctly"`
	Errors                  int     `json:"errors"`
}

// TextileDetail is the same accounting for decided versus undetermined.
type TextileDetail struct {
	ExpectedDecided        int     `json:"expected_decided"`
	EngineDecided          int     `json:"engine_decided"`
	DecidedCorrect         int     `json:"decided_correct"`
	DecidedPrecision       float64 `json:"decided_precision"`
	DecidedRecall          float64 `json:"decided_recall"`
	FalseDecided           int     `json:"false_decided"`
	WrongOrigin            int     `json:"wrong_origin"`
	UnexpectedUndetermined int     `json:"unexpected_undetermined"`
	ExpectedUndetermined   int     `json:"expected_undetermined"`
	UndeterminedCorrect    int     `json:"undetermined_correct"`
	ReasonClassExpected    int     `json:"reason_class_expected"`
	ReasonClassMatched     int     `json:"reason_class_matched"`
	OutOfScopeExpected     int     `json:"out_of_scope_expected"`
	OutOfScopeCorrect      int     `json:"out_of_scope_correct"`
	Errors                 int     `json:"errors"`
}

// TariffDetail separates the kinds of tariff-watch check.
type TariffDetail struct {
	WatchCases       int `json:"watch_cases"`
	ReviseCases      int `json:"revise_cases"`
	RefusalExpected  int `json:"dataset_refusals_expected"`
	RefusalCorrect   int `json:"dataset_refusals_correct"`
	AffectedTP       int `json:"affected_true_positive"`
	AffectedFP       int `json:"affected_false_positive"`
	AffectedFN       int `json:"affected_false_negative"`
	AffectedTN       int `json:"affected_true_negative"`
	RateDeltaCases   int `json:"cases_with_rate_change_expected"`
	RateDeltaCorrect int `json:"cases_with_rate_change_fully_matched"`
	MoneyCases       int `json:"cases_with_dollar_figures_expected"`
	MoneyCorrect     int `json:"cases_with_dollar_figures_fully_matched"`
	ReviseCorrect    int `json:"revise_correct"`
	Errors           int `json:"errors"`
}

// SplitSummary is the count of one split of one feature.
type SplitSummary struct {
	Cases      int            `json:"cases"`
	Matched    int            `json:"matched"`
	Mismatched int            `json:"mismatched"`
	Errors     int            `json:"errors"`
	USMCA      *USMCADetail   `json:"usmca,omitempty"`
	Textile    *TextileDetail `json:"textile,omitempty"`
	Tariff     *TariffDetail  `json:"tariff,omitempty"`
}

func ratio(a, b int) float64 {
	if b == 0 {
		return 0
	}
	return float64(int64(float64(a)/float64(b)*1e6+0.5)) / 1e6
}

// SummarizeSplit recomputes a split's counts from its cases and raw rows. A
// missing, extra, repeated or unrecognised row is refused.
func SummarizeSplit(feature string, cases []Case, rows []Row) (SplitSummary, error) {
	var s SplitSummary
	byID := map[string]Row{}
	for _, r := range rows {
		if _, dup := byID[r.ID]; dup {
			return s, fmt.Errorf("result row %q appears twice", r.ID)
		}
		byID[r.ID] = r
	}
	known := map[string]bool{}
	for _, c := range cases {
		known[c.ID] = true
		if _, ok := byID[c.ID]; !ok {
			return s, fmt.Errorf("case %q has no result row", c.ID)
		}
	}
	for _, r := range rows {
		if !known[r.ID] {
			return s, fmt.Errorf("result row %q has no case", r.ID)
		}
		switch r.Status {
		case StatusMatch, StatusMismatch, StatusError:
		default:
			return s, fmt.Errorf("result row %q has status %q", r.ID, r.Status)
		}
	}
	var u USMCADetail
	var x TextileDetail
	var t TariffDetail
	for _, c := range cases {
		r := byID[c.ID]
		s.Cases++
		switch r.Status {
		case StatusMatch:
			s.Matched++
		case StatusMismatch:
			s.Mismatched++
		case StatusError:
			s.Errors++
		}
		switch feature {
		case "usmca_rules":
			tallyUSMCA(&u, r)
		case "textile_rules":
			tallyTextile(&x, r)
		case "tariff_watch_rates":
			tallyTariff(&t, r)
		}
	}
	switch feature {
	case "usmca_rules":
		u.DecisivePrecision, u.DecisiveRecall = ratio(u.DecisiveCorrect, u.EngineDecisive), ratio(u.DecisiveCorrect, u.ExpectedDecisive)
		s.USMCA = &u
	case "textile_rules":
		x.DecidedPrecision, x.DecidedRecall = ratio(x.DecidedCorrect, x.EngineDecided), ratio(x.DecidedCorrect, x.ExpectedDecided)
		s.Textile = &x
	case "tariff_watch_rates":
		s.Tariff = &t
	}
	return s, nil
}

func tallyUSMCA(u *USMCADetail, r Row) {
	var e usmcaExpected
	_ = json.Unmarshal(r.Expected, &e)
	if e.Invalid {
		u.InvalidExpected++
		if r.Status == StatusMatch {
			u.InvalidRefusedCorrectly++
		}
		if r.Status == StatusError {
			u.Errors++
		}
		return
	}
	decisive := func(s string) bool { return s == "met" || s == "not_met" }
	expDec := decisive(e.Status)
	if expDec {
		u.ExpectedDecisive++
	} else {
		u.ExpectedAbstain++
	}
	if r.Status == StatusError {
		u.Errors++
		return
	}
	var a usmcaActual
	_ = json.Unmarshal(r.Actual, &a)
	engDec := decisive(a.Status)
	if engDec {
		u.EngineDecisive++
	}
	switch {
	case expDec && engDec && a.Status == e.Status && r.Status == StatusMatch:
		u.DecisiveCorrect++
	case !expDec && engDec:
		u.FalseDecisive++
	case expDec && !engDec:
		u.UnexpectedAbstain++
	case !expDec && !engDec && a.Status == e.Status && r.Status == StatusMatch:
		u.AbstainCorrect++
	}
}

func tallyTextile(x *TextileDetail, r Row) {
	var e textileActual
	_ = json.Unmarshal(r.Expected, &e)
	switch e.Status {
	case "decided":
		x.ExpectedDecided++
	case "undetermined":
		x.ExpectedUndetermined++
	case "out_of_scope":
		x.OutOfScopeExpected++
	}
	if r.Status == StatusError {
		x.Errors++
		return
	}
	var a textileActual
	_ = json.Unmarshal(r.Actual, &a)
	if a.Status == "decided" {
		x.EngineDecided++
	}
	switch {
	case e.Status == "decided" && a.Status == "decided":
		if r.Status == StatusMatch {
			x.DecidedCorrect++
		} else if a.Origin != e.Origin {
			x.WrongOrigin++
		}
	case e.Status != "decided" && a.Status == "decided":
		x.FalseDecided++
	case e.Status == "decided" && a.Status == "undetermined":
		x.UnexpectedUndetermined++
	case e.Status == "undetermined" && a.Status == "undetermined":
		x.UndeterminedCorrect++
		if e.Class != "" {
			x.ReasonClassExpected++
			if a.Class == e.Class {
				x.ReasonClassMatched++
			}
		}
	case e.Status == "out_of_scope" && a.Status == "out_of_scope":
		x.OutOfScopeCorrect++
	}
}

func tallyTariff(t *TariffDetail, r Row) {
	var e map[string]any
	_ = json.Unmarshal(r.Expected, &e)
	var a map[string]any
	_ = json.Unmarshal(r.Actual, &a)
	if r.Status == StatusError {
		t.Errors++
	}
	switch {
	case e["dataset_refused"] == true:
		t.RefusalExpected++
		if r.Status == StatusMatch {
			t.RefusalCorrect++
		}
	case e["actions"] != nil || e["error"] == true:
		t.ReviseCases++
		if r.Status == StatusMatch {
			t.ReviseCorrect++
		}
	default:
		t.WatchCases++
		expAff, _ := e["affected"].(bool)
		actAff, _ := a["affected"].(bool)
		switch {
		case r.Status == StatusError:
		case expAff && actAff:
			t.AffectedTP++
		case !expAff && actAff:
			t.AffectedFP++
		case expAff && !actAff:
			t.AffectedFN++
		default:
			t.AffectedTN++
		}
		if l, _ := e["rates"].([]any); len(l) > 0 {
			t.RateDeltaCases++
			if sameField(e, a, "rates") && r.Status == StatusMatch {
				t.RateDeltaCorrect++
			}
		}
		if e["annual"] != nil {
			t.MoneyCases++
			if sameField(e, a, "annual") && sameField(e, a, "plan") && r.Status == StatusMatch {
				t.MoneyCorrect++
			}
		}
	}
}

func sameField(e, a map[string]any, k string) bool {
	x, _ := marshal(e[k])
	y, _ := marshal(a[k])
	if k == "rates" {
		nx, _ := normTariff([]byte(fmt.Sprintf(`{"rates":%s}`, x)))
		ny, _ := normTariff([]byte(fmt.Sprintf(`{"rates":%s}`, y)))
		return bytes.Equal(nx, ny)
	}
	return bytes.Equal(x, y)
}

// FeatureSummary is one entry of summary.json.
type FeatureSummary struct {
	ID               string        `json:"id"`
	Label            string        `json:"label"`
	CandidateRev     string        `json:"candidate_revision"`
	DatasetSHA256    string        `json:"dataset_sha256"`
	DevDatasetSHA256 string        `json:"development_dataset_sha256"`
	SourceFamilies   int           `json:"source_families"`
	Families         FamilyAccount `json:"families"`
	Development      SplitSummary  `json:"development"`
	Test             SplitSummary  `json:"test"`
	CostUSD          float64       `json:"cost_usd"`
	LatencyP50Ms     float64       `json:"latency_p50_ms"`
	LatencyP95Ms     float64       `json:"latency_p95_ms"`
	Limitations      []string      `json:"limitations"`
}

// Summary is summary.json.
type Summary struct {
	SchemaVersion int              `json:"schema_version"`
	EvidenceKind  string           `json:"evidence_kind"`
	Features      []FeatureSummary `json:"features"`
}

// Latency is the measured per-case wall time of one split of one feature.
type Latency struct {
	P50Ms float64 `json:"p50_ms"`
	P95Ms float64 `json:"p95_ms"`
	N     int     `json:"n"`
}

var limitations = map[string][]string{
	"usmca_rules": {
		"Cases are constructed from the General Note 11(o), 11(c), 11(e), 11(n) and 11(p) text by one author; they are not independent customs decisions or an i.i.d. sample.",
		"The expected labels come from a separate reference oracle written from the same authority text; it does not independently validate the embedded rule data's transcription.",
		"Cases share rule rows across inputs; source families and structural templates shared with development are counted, not hidden.",
		"The check models simple goods; accumulation, remanufacture, automotive rules and the rayon exceptions of textile chapters are outside its scope and outside these cases.",
	},
	"textile_rules": {
		"Cases are constructed facts for rows of 19 CFR 102.21(e); they are not CBP rulings and carry no real-world confidence interval.",
		"The reference oracle re-encodes the used rows by hand from the eCFR text; it does not independently validate the full embedded table.",
		"Pure tariff-shift rows and the (c)(4) most-important-process judgment are documented abstentions: expected undetermined means the check declines, not that the law has no answer.",
		"Free-text operation names are not parsed by these cases; the facts are given as structured stages.",
	},
	"tariff_watch_rates": {
		"Every revised data set is SYNTHETIC: a constructed next revision exercising one kind of change, not a government tariff action. Tariff lines, heading numbers and baseline rates are real.",
		"Expected figures come from arithmetic on the case's own numbers by a reference oracle that shares no code with the candidate.",
		"Rates are compared as whole units of 0.0001 and dollars as whole cents so platform float differences cannot change a label.",
		"Revision-text cases render synthetic heading text in the HTS PDF-extraction layout; they test the reader, not the accuracy of any real notice.",
	},
}

func fileSHA(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:]), nil
}

func loadRows(path string) ([]Row, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return ReadRows(f)
}

func rowsFor(rows []Row, feature string) []Row {
	var out []Row
	for _, r := range rows {
		if r.Feature == feature {
			out = append(out, r)
		}
	}
	return out
}

// BuildSummary recomputes summary.json from the datasets and the raw rows.
func BuildSummary(root, candidate string, lat map[string]map[string]Latency) (Summary, error) {
	dev, test, _, err := LoadAll(root)
	if err != nil {
		return Summary{}, err
	}
	devRows, err := loadRows(filepath.Join(root, "results-dev.jsonl"))
	if err != nil {
		return Summary{}, err
	}
	testRows, err := loadRows(filepath.Join(root, "results-test.jsonl"))
	if err != nil {
		return Summary{}, err
	}
	sum := Summary{SchemaVersion: 1, EvidenceKind: "source_grounded_synthetic"}
	for _, f := range Features {
		fs := FeatureSummary{ID: f.ID, Label: f.Label, CandidateRev: candidate, Limitations: limitations[f.ID]}
		if fs.DatasetSHA256, err = fileSHA(filepath.Join(root, f.Prefix+"-test.jsonl")); err != nil {
			return sum, err
		}
		if fs.DevDatasetSHA256, err = fileSHA(filepath.Join(root, f.Prefix+"-dev.jsonl")); err != nil {
			return sum, err
		}
		fd, ft := byFeature(dev, f.ID), byFeature(test, f.ID)
		fa := Families(fd, ft)
		fs.Families, fs.SourceFamilies = fa, fa.TestSourceFamilies
		if fs.Development, err = SummarizeSplit(f.ID, fd, rowsFor(devRows, f.ID)); err != nil {
			return sum, fmt.Errorf("%s dev: %w", f.ID, err)
		}
		if fs.Test, err = SummarizeSplit(f.ID, ft, rowsFor(testRows, f.ID)); err != nil {
			return sum, fmt.Errorf("%s test: %w", f.ID, err)
		}
		if l, ok := lat["test"][f.ID]; ok {
			fs.LatencyP50Ms, fs.LatencyP95Ms = l.P50Ms, l.P95Ms
		}
		sum.Features = append(sum.Features, fs)
	}
	return sum, nil
}

// RunSplit scores every case of a split once and returns the rows in dataset order.
func RunSplit(cases []Case) ([]Row, map[string]Latency) {
	times := map[string][]float64{}
	var rows []Row
	for _, c := range cases {
		start := nowNs()
		r := Score(c)
		times[c.Feature] = append(times[c.Feature], float64(nowNs()-start)/1e6)
		rows = append(rows, r)
	}
	lat := map[string]Latency{}
	for f, ts := range times {
		sort.Float64s(ts)
		pick := func(q float64) float64 {
			i := int(q*float64(len(ts))+0.999999) - 1
			if i < 0 {
				i = 0
			}
			if i >= len(ts) {
				i = len(ts) - 1
			}
			return float64(int64(ts[i]*1000+0.5)) / 1000
		}
		lat[f] = Latency{P50Ms: pick(0.5), P95Ms: pick(0.95), N: len(ts)}
	}
	return rows, lat
}

// CheckCommitted re-scores both splits through the current candidate and
// compares every row byte for byte with the committed results; it then
// recomputes summary.json from the raw rows (ignoring only the measured
// latency and the recorded candidate revision).
func CheckCommitted(root string) error {
	dev, test, srcs, err := LoadAll(root)
	if err != nil {
		return err
	}
	if p := CheckSources(srcs, repoRoot()); len(p) > 0 {
		return fmt.Errorf("sources: %v", p)
	}
	for _, sp := range []struct {
		name  string
		cases []Case
	}{{"dev", dev}, {"test", test}} {
		want, _ := RunSplit(sp.cases)
		var b bytes.Buffer
		if err := WriteRows(&b, want); err != nil {
			return err
		}
		got, err := os.ReadFile(filepath.Join(root, "results-"+sp.name+".jsonl"))
		if err != nil {
			return err
		}
		if !bytes.Equal(got, b.Bytes()) {
			return fmt.Errorf("results-%s.jsonl differs from a fresh scoring run of the committed datasets", sp.name)
		}
	}
	raw, err := os.ReadFile(filepath.Join(root, "summary.json"))
	if err != nil {
		return err
	}
	var have Summary
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&have); err != nil {
		return err
	}
	if len(have.Features) == 0 {
		return fmt.Errorf("summary.json has no features")
	}
	rev := have.Features[0].CandidateRev
	if !shaRE.MatchString(rev) {
		return fmt.Errorf("summary.json candidate_revision %q is not a full commit sha", rev)
	}
	want, err := BuildSummary(root, rev, nil)
	if err != nil {
		return err
	}
	for i := range have.Features {
		have.Features[i].LatencyP50Ms, have.Features[i].LatencyP95Ms = 0, 0
	}
	a, _ := marshal(have)
	b, _ := marshal(want)
	if !bytes.Equal(a, b) {
		return fmt.Errorf("summary.json differs from the summary recomputed from the raw rows")
	}
	return nil
}
