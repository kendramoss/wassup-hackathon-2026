package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func round6(v float64) float64 { return math.Round(v*1e6) / 1e6 }

// Summary is summary.json: the shared evidence contract plus extra fields.
type Summary struct {
	SchemaVersion   int                   `json:"schema_version"`
	EvidenceKind    string                `json:"evidence_kind"`
	Features        []FeatureSummary      `json:"features"`
	Routing         *RoutingEntry         `json:"routing,omitempty"`
	Comparator      *ComparatorEntry      `json:"live_textile_comparator,omitempty"`
	JapanComparator *JapanComparatorEntry `json:"live_japan_comparator,omitempty"`
	Corpus          map[string]string     `json:"corpus"`
	Attempts        []AttemptTotal        `json:"attempts_by_feature"`
}

// RoutingEntry is the extra routing feature, with its own denominators.
type RoutingEntry struct {
	ID                string         `json:"id"`
	Label             string         `json:"label"`
	CandidateRevision string         `json:"candidate_revision"`
	DatasetSHA256     string         `json:"dataset_sha256"`
	Development       RoutingSummary `json:"development"`
	Test              RoutingSummary `json:"test"`
	CostUSD           float64        `json:"cost_usd"`
	LatencyP50MS      int64          `json:"latency_p50_ms"`
	LatencyP95MS      int64          `json:"latency_p95_ms"`
	Limitations       []string       `json:"limitations"`
}

// AttemptTotal counts every scored attempt of a feature, development and test.
type AttemptTotal struct {
	Feature  string  `json:"feature"`
	Attempts int     `json:"attempts"`
	CostUSD  float64 `json:"cost_usd"`
}

func readAttempts() ([]attempt, error) {
	b, err := os.ReadFile(filepath.Join(dataRoot(), "attempts.jsonl"))
	if err != nil {
		return nil, err
	}
	var out []attempt
	for _, l := range bytes.Split(bytes.TrimSpace(b), []byte("\n")) {
		var a attempt
		if err := json.Unmarshal(l, &a); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, nil
}

// devRecord names the development run of record per feature (the final
// candidate's run), in dev-record.json.
func devRecord() (map[string]string, error) {
	b, err := os.ReadFile(filepath.Join(dataRoot(), "dev-record.json"))
	if err != nil {
		return nil, err
	}
	m := map[string]string{}
	return m, json.Unmarshal(b, &m)
}

func limitations(f Feature, tmplShared, famShared int) []string {
	l := []string{
		"Synthetic cases written by the implementer from cited law and rulings; labels come from a reference oracle that shares no code with the candidate. Not independent real-world customs decisions.",
		"Reserved cases were authored by the same hand and templates as development; they were not tuned on, but shared scenario templates are disclosed (templates shared with development: " + fmt.Sprint(tmplShared) + ", source families shared: " + fmt.Sprint(famShared) + ").",
		"No claim of 100 independent legal sources and no i.i.d. confidence interval; the cases are parameter variations of a small set of rules and rulings.",
		"The confident-decision bar is 0.92 stated confidence, a study convention. The deployed auto-accept bars are textile 0.95, pharmaceutical 0.85 and Part 102 0.95 (deploy workflow). The deployed textile engine is textile-v2 with precedent reasoning; the textile candidate measured here is built on textile-v1, so it describes that prompt only (see the separate live-textile comparator).",
	}
	if f.ID == "japan_engine" {
		l = append(l, "Only rows whose whole product-specific rule is CC, CTH or CTSH, in machinery, instruments and metal articles, were used; failing materials always exceed any tolerance or de minimis. Rules with alternatives, value tests or notes are not covered. Cases with a code outside the schedule's edition are answered before any model call (preflight) and are counted separately.")
	}
	if f.ID == "part102_specialist" {
		l = append(l, "Only 102.20 rows of the form 'A change to ... from any other chapter, heading or subheading' were used; sets, composite goods, textiles and rows with exceptions are not covered. Production countries are Canada and Mexico.")
	}
	if f.ID == "textile_specialist" {
		l = append(l, "Rows used: 5204-5207, 5401-5406, 5208-5212, 5407-5408, 5512-5516, 5508-5511, 6101-6117, 6201-6208, 6210-6211 and 6301-6306 (non-(e)(2)); knit fabrics, E2 headings and (c)(4)-(5) importance judgements are labelled uncertain or not used.")
	}
	if f.ID == "japan_engine" {
		l = append(l, "13 of the 112 reserved cases (3 of 48 development) carry a tariff code outside the HS edition of the schedule in force (6 against HS 2017 schedules, 7 against older editions, the latter checked only against the HS 2017 list): authoring defects, kept. Two of the 8 declined decided cases are among them. There is no production arm on the reserved Japan cases, so the study cannot say whether the Japan candidate helped; it is not wired on this evidence.")
	}
	if f.ID == "pharma_specialist" {
		l = append(l, "12 of the 113 reserved cases (6 of 48 development) are formulated pesticides; routing excludes them because router-v4 defines the pharmaceutical regime as drugs, vitamins and supplements, so 101 of 113 are route-eligible.", "Expected labels apply the pharmaceutical rules in the engine's own prompt (HQ 563207, N334870, N336575, N355022, N349588, H341715); this measures consistent application of them, not validation of the rules.")
	}
	return l
}

func buildSummary(candidate string) (Summary, error) {
	sum := Summary{SchemaVersion: 1, EvidenceKind: "source_grounded_synthetic"}
	dev, test, err := LoadAll(dataRoot())
	if err != nil {
		return sum, err
	}
	rec, err := devRecord()
	if err != nil {
		return sum, err
	}
	atts, err := readAttempts()
	if err != nil {
		return sum, err
	}
	res := filepath.Join(dataRoot(), "results")
	for _, f := range Features {
		fd, ft := byFeature(dev, f.ID), byFeature(test, f.ID)
		devRows, err := readRows(filepath.Join(res, rec[f.ID]))
		if err != nil {
			return sum, err
		}
		testRows, err := readRows(filepath.Join(res, f.File+"-test.jsonl"))
		if err != nil {
			return sum, err
		}
		if err := Gate(append(append([]Row{}, devRows...), testRows...)); err != nil {
			return sum, err
		}
		ds, err := SummarizeSplit(fd, devRows)
		if err != nil {
			return sum, err
		}
		ts, err := SummarizeSplit(ft, testRows)
		if err != nil {
			return sum, err
		}
		devSHA, err := fileSHA(filepath.Join(dataRoot(), f.File+"-dev.jsonl"))
		if err != nil {
			return sum, err
		}
		testSHA, err := fileSHA(filepath.Join(dataRoot(), f.File+"-test.jsonl"))
		if err != nil {
			return sum, err
		}
		fam, shared, _, tshared := familyStats(fd, ft)
		var lat []int64
		var testCost float64
		for _, r := range testRows {
			testCost += r.CostUSD
			if r.CalledModel {
				lat = append(lat, r.LatencyMS)
			}
		}
		sum.Features = append(sum.Features, FeatureSummary{ID: f.ID, Label: f.Label, CandidateRevision: candidate, DatasetSHA256: testSHA, DevDatasetSHA256: devSHA,
			SourceFamilies: fam, SharedFamilies: shared, Development: ds, Test: ts, CostUSD: round6(testCost), LatencyP50MS: Percentile(lat, .5), LatencyP95MS: Percentile(lat, .95),
			Limitations: limitations(f, tshared, shared)})
	}
	rdev, rtest, err := LoadRouting(dataRoot())
	if err != nil {
		return sum, err
	}
	drows, err := readRows(filepath.Join(res, rec[RoutingFeature]))
	if err != nil {
		return sum, err
	}
	trows, err := readRows(filepath.Join(res, "routing-test.jsonl"))
	if err != nil {
		return sum, err
	}
	if err := Gate(append(append([]Row{}, drows...), trows...)); err != nil {
		return sum, err
	}
	ds, err := SummarizeRouting(rdev, drows)
	if err != nil {
		return sum, err
	}
	ts, err := SummarizeRouting(rtest, trows)
	if err != nil {
		return sum, err
	}
	rsha, err := fileSHA(filepath.Join(dataRoot(), "routing-test.jsonl"))
	if err != nil {
		return sum, err
	}
	var lat []int64
	var rc float64
	for _, r := range trows {
		rc += r.CostUSD
		if r.CalledModel {
			lat = append(lat, r.LatencyMS)
		}
	}
	sum.Routing = &RoutingEntry{ID: RoutingFeature, Label: "Legal-regime routing (router-v4)", CandidateRevision: candidate, DatasetSHA256: rsha, Development: ds, Test: ts,
		CostUSD: round6(rc), LatencyP50MS: Percentile(lat, .5), LatencyP95MS: Percentile(lat, .95), Limitations: []string{
			"Appropriate cases reuse the facts of the textile, pharmaceutical and Part 102 specialist cases (parents are visible); they are not independent of those cases.",
			"Negative controls are real 2026 rulings whose recorded regime is substantial transformation; that label comes from the corpus labeller, not an expert.",
			"Japan cases are not routed: the router has no Japan regime.",
			"Pesticide cases (12 reserved, 6 development) are not routed, so routing covers 101 of the 113 pharmaceutical reserved cases.",
		}}
	byF := map[string]*AttemptTotal{}
	for _, a := range atts {
		t := byF[a.Feature]
		if t == nil {
			t = &AttemptTotal{Feature: a.Feature}
			byF[a.Feature] = t
		}
		t.Attempts++
		t.CostUSD += a.Cost
	}
	var keys []string
	for k := range byF {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		byF[k].CostUSD = round6(byF[k].CostUSD)
		sum.Attempts = append(sum.Attempts, *byF[k])
	}
	_, prov, err := loadPools()
	if err != nil {
		return sum, err
	}
	sum.Corpus = prov
	if sum.Comparator, err = buildComparator(); err != nil {
		return sum, err
	}
	if sum.JapanComparator, err = buildJapanComparator(); err != nil {
		return sum, err
	}
	return sum, nil
}

func marshal(s Summary) ([]byte, error) {
	b, err := json.MarshalIndent(s, "", " ")
	return append(b, '\n'), err
}

func cmdSummarize(args []string) error {
	fs := flag.NewFlagSet("summarize", flag.ExitOnError)
	cand := fs.String("candidate", "", "full sha of the frozen candidate commit")
	_ = fs.Parse(args)
	if !shaRE.MatchString(*cand) {
		return errors.New("-candidate must be the full 40-character commit sha")
	}
	s, err := buildSummary(*cand)
	if err != nil {
		return err
	}
	b, err := marshal(s)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dataRoot(), "summary.json"), b, 0o644)
}

// cmdCheck recomputes the summary from the committed rows and compares bytes.
func cmdCheck() error {
	old, err := os.ReadFile(filepath.Join(dataRoot(), "summary.json"))
	if err != nil {
		return err
	}
	var prev Summary
	if err := json.Unmarshal(old, &prev); err != nil || len(prev.Features) == 0 {
		return errors.New("summary.json is unreadable")
	}
	s, err := buildSummary(prev.Features[0].CandidateRevision)
	if err != nil {
		return err
	}
	b, err := marshal(s)
	if err != nil {
		return err
	}
	if !bytes.Equal(b, old) {
		return errors.New("summary.json differs from the recomputed summary of the committed rows")
	}
	if p := checkTestNotRescored(); p != "" {
		return errors.New(p)
	}
	fmt.Println("committed rows reproduce summary.json exactly")
	return nil
}

// checkTestNotRescored reports a second reserved-test attempt for any scope.
func checkTestNotRescored() string {
	atts, err := readAttempts()
	if err != nil {
		return err.Error()
	}
	seen := map[string]int{}
	for _, a := range atts {
		if a.Split == "test" {
			seen[a.Feature]++
		}
	}
	var bad []string
	for f, n := range seen {
		if n > 1 {
			bad = append(bad, f)
		}
	}
	if len(bad) > 0 {
		sort.Strings(bad)
		return "more than one reserved-test attempt recorded for: " + strings.Join(bad, ", ")
	}
	return ""
}

// ComparatorEntry is the post-hoc live-textile comparison (D971 amendment 2),
// published apart from the frozen study: the deployed textile-v2 prompt with
// precedent reasoning against the same prompt plus the missing-fact rule and
// single-country guard, graded at the deployed 0.95 bar.
type ComparatorEntry struct {
	ID                string        `json:"id"`
	Label             string        `json:"label"`
	CandidateRevision string        `json:"candidate_revision"`
	DeployedBar       float64       `json:"deployed_bar"`
	DevBase           SplitSummary  `json:"development_live_v2"`
	DevCandidate      SplitSummary  `json:"development_live_v2_plus_option"`
	Selected          string        `json:"selected_on_development"`
	Test              *SplitSummary `json:"test,omitempty"`
	DatasetSHA256     string        `json:"dataset_sha256"`
	CostUSD           float64       `json:"test_cost_usd"`
	LatencyP50MS      int64         `json:"latency_p50_ms"`
	LatencyP95MS      int64         `json:"latency_p95_ms"`
	Limitations       []string      `json:"limitations"`
}

type comparatorRecord struct {
	DevBase string `json:"dev_base"`
	DevCand string `json:"dev_candidate"`
}

// buildComparator returns nil when no comparator has been recorded.
func buildComparator() (*ComparatorEntry, error) {
	b, err := os.ReadFile(filepath.Join(dataRoot(), "comparator.json"))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	} else if err != nil {
		return nil, err
	}
	var rec comparatorRecord
	if err := json.Unmarshal(b, &rec); err != nil {
		return nil, err
	}
	dev, _, err := LoadAll(dataRoot())
	if err != nil {
		return nil, err
	}
	_, test, _ := LoadAll(dataRoot())
	fd, ft := byFeature(dev, "textile_specialist"), byFeature(test, "textile_specialist")
	res := filepath.Join(dataRoot(), "results")
	sum := func(name string, cases []Case) (SplitSummary, []Row, error) {
		rows, err := readRows(filepath.Join(res, name))
		if err != nil {
			return SplitSummary{}, nil, err
		}
		if err := Gate(rows); err != nil {
			return SplitSummary{}, nil, err
		}
		s, err := SummarizeSplit(cases, rows)
		return s, rows, err
	}
	e := &ComparatorEntry{ID: ComparatorFeature, Label: "Deployed textile engine (textile-v2 with precedent reasoning), with and without the missing-fact option", DeployedBar: DeployedTextileBar}
	if e.DevBase, _, err = sum(rec.DevBase, fd); err != nil {
		return nil, err
	}
	if e.DevCandidate, _, err = sum(rec.DevCand, fd); err != nil {
		return nil, err
	}
	e.Selected = SelectLive(e.DevBase, e.DevCandidate)
	if atts, err := readAttempts(); err == nil {
		for _, a := range atts {
			if a.Feature == ComparatorFeature && a.Split == "test" {
				e.CandidateRevision = a.Head
			}
		}
	}
	if e.DatasetSHA256, err = fileSHA(filepath.Join(dataRoot(), "textile-test.jsonl")); err != nil {
		return nil, err
	}
	if _, serr := os.Stat(filepath.Join(res, comparatorTestFile)); serr == nil {
		if frozenLive != e.Selected {
			return nil, fmt.Errorf("frozenLive %q is not the variant the development rule selects (%q)", frozenLive, e.Selected)
		}
		ts, rows, err := sum(comparatorTestFile, ft)
		if err != nil {
			return nil, err
		}
		e.Test = &ts
		var lat []int64
		for _, r := range rows {
			e.CostUSD += r.CostUSD
			if r.CalledModel {
				lat = append(lat, r.LatencyMS)
			}
		}
		e.CostUSD, e.LatencyP50MS, e.LatencyP95MS = round6(e.CostUSD), Percentile(lat, .5), Percentile(lat, .95)
	}
	e.Limitations = []string{
		"Post-hoc and current-configuration only: the original synthetic reserved textile cases had already been scored and seen by the author before this comparison, so it is not a new blind holdout and does not replace the original 114-case result.",
		"Selected on the original development cases only, by a rule registered before any comparator call; one comparator score on the unchanged 114 reserved cases, no relabelling.",
		"Gateway calls without the deployed engine cache, retry or fallback model; the deployed provider is Vertex. Graded at the deployed 0.95 textile bar.",
		"Same synthetic-case limits as the study: oracle labels by the case author, shared templates with development.",
	}
	return e, nil
}

// JapanComparatorEntry is the post-hoc measurement of the unmodified Japan
// engine on the original reserved cases (D971 amendment 4).
type JapanComparatorEntry struct {
	ID                string       `json:"id"`
	Label             string       `json:"label"`
	CandidateRevision string       `json:"candidate_revision"`
	DatasetSHA256     string       `json:"dataset_sha256"`
	Test              SplitSummary `json:"test"`
	CostUSD           float64      `json:"test_cost_usd"`
	LatencyP50MS      int64        `json:"latency_p50_ms"`
	LatencyP95MS      int64        `json:"latency_p95_ms"`
	Limitations       []string     `json:"limitations"`
}

func buildJapanComparator() (*JapanComparatorEntry, error) {
	path := filepath.Join(dataRoot(), "results", japanComparatorTestFile)
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	_, test, err := LoadAll(dataRoot())
	if err != nil {
		return nil, err
	}
	cases := byFeature(test, "japan_engine")
	rows, err := readRows(path)
	if err != nil {
		return nil, err
	}
	if err := Gate(rows); err != nil {
		return nil, err
	}
	ts, err := SummarizeSplit(cases, rows)
	if err != nil {
		return nil, err
	}
	e := &JapanComparatorEntry{ID: JapanComparatorFeature, Label: "Japan preference engine as deployed (jp-engine-v1, no unknown-status sentence)", Test: ts}
	if e.DatasetSHA256, err = fileSHA(filepath.Join(dataRoot(), "japan-test.jsonl")); err != nil {
		return nil, err
	}
	if atts, err := readAttempts(); err == nil {
		for _, a := range atts {
			if a.Feature == JapanComparatorFeature && a.Split == "test" {
				e.CandidateRevision = a.Head
			}
		}
	}
	var lat []int64
	for _, r := range rows {
		e.CostUSD += r.CostUSD
		if r.CalledModel {
			lat = append(lat, r.LatencyMS)
		}
	}
	e.CostUSD, e.LatencyP50MS, e.LatencyP95MS = round6(e.CostUSD), Percentile(lat, .5), Percentile(lat, .95)
	e.Limitations = []string{
		"Post-hoc and current-configuration only: the original reserved Japan cases had already been scored and seen by the author; this is not a new blind holdout and does not replace the original 112-case result of the frozen cand-1 variant, which is unchanged.",
		"The unmodified engine, one score, no tuning and no label or input change; the 13 cases whose tariff codes fall outside the schedule's HS edition stay in the denominator.",
		"Gateway calls without the deployed engine cache, retry or fallback; the deployed provider is Vertex.",
		"Same synthetic-case limits as the study: oracle labels by the case author, shared templates with development.",
	}
	return e, nil
}
