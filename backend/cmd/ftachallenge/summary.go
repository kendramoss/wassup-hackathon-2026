package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"slices"
	"sort"
	"time"
)

type ResultsFile struct {
	SchemaVersion     int      `json:"schema_version"`
	EvidenceKind      string   `json:"evidence_kind"`
	Feature           string   `json:"feature"`
	Split             string   `json:"split"`
	Tag               string   `json:"tag,omitempty"`
	DatasetSHA256     string   `json:"dataset_sha256"`
	CandidateRevision string   `json:"candidate_revision"`
	Totals            Totals   `json:"totals"`
	Metrics           Metrics  `json:"metrics"`
	Results           []Result `json:"results"`
}

func NewResultsFile(ds *Dataset, other *Dataset, candidate, tag string, rs []Result) ResultsFile {
	return ResultsFile{SchemaVersion: schemaVersion, EvidenceKind: evidenceKind, Feature: ds.Feature, Split: ds.Split, Tag: tag,
		DatasetSHA256: ds.SHA256, CandidateRevision: candidate, Totals: Tally(rs), Metrics: ComputeMetrics(ds, rs, other), Results: rs}
}

func marshal(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

type SplitSummary struct {
	Cases      int `json:"cases"`
	Matched    int `json:"matched"`
	Mismatched int `json:"mismatched,omitempty"`
	Errors     int `json:"errors"`
}

type TestSummary struct {
	Cases      int `json:"cases"`
	Matched    int `json:"matched"`
	Mismatched int `json:"mismatched"`
	Errors     int `json:"errors"`
}

type SummaryFeature struct {
	ID                       string         `json:"id"`
	Label                    string         `json:"label"`
	CandidateRevision        string         `json:"candidate_revision"`
	DatasetSHA256            string         `json:"dataset_sha256"`
	SourceFamilies           int            `json:"source_families"`
	Development              SplitSummary   `json:"development"`
	Test                     TestSummary    `json:"test"`
	CostUSD                  float64        `json:"cost_usd"`
	LatencyP50Ms             float64        `json:"latency_p50_ms"`
	LatencyP95Ms             float64        `json:"latency_p95_ms"`
	Limitations              []string       `json:"limitations"`
	DevelopmentDatasetSHA256 string         `json:"development_dataset_sha256"`
	DevelopmentFirstRun      *SplitSummary  `json:"development_first_run,omitempty"`
	ScenarioFamilies         int            `json:"scenario_families"`
	ParentGroups             int            `json:"parent_groups"`
	CasesWithParent          int            `json:"cases_with_parent"`
	FormattingOnlyCases      int            `json:"formatting_only_cases"`
	SharedFamilies           int            `json:"families_shared_with_development"`
	TestOnlyFamilies         int            `json:"families_only_in_test"`
	CasesInSharedFamilies    int            `json:"cases_in_shared_families"`
	ExpectedOutcomes         map[string]int `json:"expected_outcomes"`
	Abstention               Abstention     `json:"abstention"`
	YesPrecision             Count          `json:"yes_precision"`
	YesRecall                Count          `json:"yes_recall"`
	DecidedOnlyAgreement     Count          `json:"decided_only_agreement"`
}

type Summary struct {
	SchemaVersion int              `json:"schema_version"`
	EvidenceKind  string           `json:"evidence_kind"`
	Features      []SummaryFeature `json:"features"`
}

type Latency struct{ P50, P95 float64 }

var commonLimitations = []string{
	"Synthetic cases written by the implementer from the cited rule rows and treaty clauses with knowledge of the engine interface: not blind authoring, not independent customs rulings.",
	"Development and reserved test cases share scenario families; only the test-only families are unseen in development.",
	"Near-duplicate and parent-linked cases are not independent observations; no population confidence interval is claimed.",
	"Case labels follow the supplied facts; where an input is silent the rule is not assumed met.",
}

var featureLimitations = map[string]string{
	"atiga": "ATIGA rules are read from the HS 2022 Annex 3 table in the engine; direct consignment, packaging, accessories and neutral elements are not scored.",
	"akfta": "AKFTA Appendix 2 rows with provisos on materials are scored only where the case states the facts the proviso needs.",
	"acfta": "ACFTA follows the 2019 Annex 1 and Attachment B (HS 2017); ACFTA 3.0 is not scored.",
	"rcep":  "RCEP Chapter 3 article numbers follow the engine's pinned citations and the ABF guide; the treaty text was not re-read in this study.",
}

func ptr(s SplitSummary) *SplitSummary { return &s }

func BuildSummary(dev, test map[string]*Dataset, devRes, testRes, firstRun map[string]ResultsFile, lat map[string]Latency) Summary {
	s := Summary{SchemaVersion: schemaVersion, EvidenceKind: evidenceKind}
	for _, f := range features {
		tr, dr := testRes[f], devRes[f]
		sf := SummaryFeature{
			ID: f, Label: labels[f], CandidateRevision: tr.CandidateRevision, DatasetSHA256: test[f].SHA256,
			SourceFamilies: tr.Metrics.SourceFamilies,
			Development:    SplitSummary{Cases: dr.Totals.Cases, Matched: dr.Totals.Matched, Errors: dr.Totals.Errors},
			Test:           TestSummary(tr.Totals.asTest()),
			CostUSD:        0, LatencyP50Ms: lat[f].P50, LatencyP95Ms: lat[f].P95,
			Limitations:              append(append([]string{}, commonLimitations...), featureLimitations[f]),
			DevelopmentDatasetSHA256: dev[f].SHA256,
			ScenarioFamilies:         tr.Metrics.ScenarioFamilies, ParentGroups: tr.Metrics.ParentGroups, CasesWithParent: tr.Metrics.CasesWithParent,
			FormattingOnlyCases: tr.Metrics.FormattingOnly, SharedFamilies: len(tr.Metrics.SharedFamilies), TestOnlyFamilies: len(tr.Metrics.TestOnlyFamilies),
			CasesInSharedFamilies: tr.Metrics.SharedFamilyCases, ExpectedOutcomes: tr.Metrics.Expected, Abstention: tr.Metrics.Abstention,
			YesPrecision: tr.Metrics.YesPrecision, YesRecall: tr.Metrics.YesRecall, DecidedOnlyAgreement: tr.Metrics.DecidedAgreement,
		}
		if fr, ok := firstRun[f]; ok {
			sf.DevelopmentFirstRun = ptr(SplitSummary{Cases: fr.Totals.Cases, Matched: fr.Totals.Matched, Mismatched: fr.Totals.Mismatched, Errors: fr.Totals.Errors})
		}
		s.Features = append(s.Features, sf)
	}
	return s
}

func (t Totals) asTest() TestSummary {
	return TestSummary{Cases: t.Cases, Matched: t.Matched, Mismatched: t.Mismatched, Errors: t.Errors}
}

// ValidateResults checks a results file against its dataset: every case
// scored once, in order, counts that add up, and a hash that matches.
func ValidateResults(ds *Dataset, rf ResultsFile) error {
	if rf.Feature != ds.Feature || rf.Split != ds.Split {
		return fmt.Errorf("results are for %s/%s, dataset is %s/%s", rf.Feature, rf.Split, ds.Feature, ds.Split)
	}
	if rf.DatasetSHA256 != ds.SHA256 {
		return fmt.Errorf("%s/%s: results were scored against a different dataset (hash mismatch)", rf.Feature, rf.Split)
	}
	if !sha40.MatchString(rf.CandidateRevision) {
		return fmt.Errorf("%s/%s: candidate_revision %q is not a full git SHA", rf.Feature, rf.Split, rf.CandidateRevision)
	}
	if len(rf.Results) != len(ds.Cases) {
		return fmt.Errorf("%s/%s: %d results for %d cases", rf.Feature, rf.Split, len(rf.Results), len(ds.Cases))
	}
	for i, r := range rf.Results {
		if r.ID != ds.Cases[i].ID {
			return fmt.Errorf("%s/%s: result %d is %s, case is %s (missing, duplicate or reordered)", rf.Feature, rf.Split, i, r.ID, ds.Cases[i].ID)
		}
		if r.Status != Matched && r.Status != Mismatched && r.Status != Errored {
			return fmt.Errorf("%s: unknown status %q", r.ID, r.Status)
		}
	}
	if got := Tally(rf.Results); got != rf.Totals {
		return fmt.Errorf("%s/%s: totals %+v do not match results %+v", rf.Feature, rf.Split, rf.Totals, got)
	}
	if rf.Totals.Matched+rf.Totals.Mismatched+rf.Totals.Errors != rf.Totals.Cases {
		return fmt.Errorf("%s/%s: matched, mismatched and errors do not add up to cases", rf.Feature, rf.Split)
	}
	return nil
}

// ValidateSummary checks a summary against the datasets and results it
// claims to describe.
func ValidateSummary(s Summary, dev, test map[string]*Dataset, devRes, testRes map[string]ResultsFile) error {
	if s.SchemaVersion != schemaVersion || s.EvidenceKind != evidenceKind {
		return fmt.Errorf("summary header: schema_version %d, evidence_kind %q", s.SchemaVersion, s.EvidenceKind)
	}
	got := make([]string, 0, len(s.Features))
	for _, f := range s.Features {
		got = append(got, f.ID)
	}
	if !slices.Equal(got, features) {
		return fmt.Errorf("summary features %v, want %v", got, features)
	}
	for _, f := range s.Features {
		id := f.ID
		tds, dds := test[id], dev[id]
		if tds == nil || dds == nil {
			return fmt.Errorf("%s: dataset missing", id)
		}
		if err := ValidateResults(tds, testRes[id]); err != nil {
			return err
		}
		if err := ValidateResults(dds, devRes[id]); err != nil {
			return err
		}
		if f.DatasetSHA256 != tds.SHA256 || f.DevelopmentDatasetSHA256 != dds.SHA256 {
			return fmt.Errorf("%s: dataset hash in summary differs from the dataset bytes", id)
		}
		if f.CandidateRevision != testRes[id].CandidateRevision {
			return fmt.Errorf("%s: candidate revision in summary differs from the results", id)
		}
		t := testRes[id].Totals
		if f.Test.Cases != len(tds.Cases) || f.Test.Cases != t.Cases || f.Test.Matched != t.Matched || f.Test.Mismatched != t.Mismatched || f.Test.Errors != t.Errors {
			return fmt.Errorf("%s: test counts in summary %+v differ from results %+v", id, f.Test, t)
		}
		if f.Test.Matched+f.Test.Mismatched+f.Test.Errors != f.Test.Cases {
			return fmt.Errorf("%s: test errors or abstentions have left the denominator", id)
		}
		if f.Test.Cases < MinTestCases {
			return fmt.Errorf("%s: only %d reserved test cases", id, f.Test.Cases)
		}
		d := devRes[id].Totals
		if f.Development.Cases != d.Cases || f.Development.Matched != d.Matched || f.Development.Errors != d.Errors {
			return fmt.Errorf("%s: development counts in summary differ from results", id)
		}
		if f.SourceFamilies != distinct(tds.Cases, func(c Case) string { return c.SourceFamilyID }) {
			return fmt.Errorf("%s: source_families in summary differs from the dataset", id)
		}
		if f.CostUSD < 0 || f.LatencyP50Ms < 0 || f.LatencyP95Ms < f.LatencyP50Ms || math.IsNaN(f.LatencyP95Ms) {
			return fmt.Errorf("%s: cost or latency out of range", id)
		}
		if len(f.Limitations) == 0 {
			return fmt.Errorf("%s: no limitations stated", id)
		}
		if f.Abstention.Expected != testRes[id].Metrics.Abstention.Expected || f.YesPrecision != testRes[id].Metrics.YesPrecision {
			return fmt.Errorf("%s: metrics in summary differ from the results", id)
		}
	}
	return nil
}

// MeasureLatency times each test case reps times and reports nearest-rank
// percentiles in milliseconds.
func MeasureLatency(ds *Dataset, reps int) Latency {
	var ds_ []float64
	for _, c := range ds.Cases {
		for i := 0; i < reps; i++ {
			start := time.Now()
			_, _ = callEngine(c.Feature, c.Input)
			ds_ = append(ds_, float64(time.Since(start).Microseconds())/1000)
		}
	}
	sort.Float64s(ds_)
	rank := func(p float64) float64 {
		if len(ds_) == 0 {
			return 0
		}
		i := int(math.Ceil(p/100*float64(len(ds_)))) - 1
		return math.Round(ds_[max(i, 0)]*1000) / 1000
	}
	return Latency{P50: rank(50), P95: rank(95)}
}
