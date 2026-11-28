package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// PublicFeature is one current-path study for the public evidence screen. It
// follows the shared contract (candidate and data hashes, source-family count,
// development and test counts, cost, latency, limits) and adds a short public
// note and display metrics, every number derived from recorded rows and
// datasets.
type PublicFeature struct {
	ID                string          `json:"id"`
	Label             string          `json:"label"`
	EvidenceKind      string          `json:"evidence_kind"`
	PostHoc           bool            `json:"post_hoc"`
	CandidateRevision string          `json:"candidate_revision"`
	DatasetSHA256     string          `json:"dataset_sha256"`
	DevDatasetSHA256  string          `json:"dev_dataset_sha256"`
	SourceFamilies    int             `json:"source_families"`
	SharedFamilies    int             `json:"source_families_shared_with_dev"`
	Development       PublicDev       `json:"development"`
	Test              PublicTest      `json:"test"`
	CostUSD           float64         `json:"cost_usd"`
	LatencyP50MS      int64           `json:"latency_p50_ms"`
	LatencyP95MS      int64           `json:"latency_p95_ms"`
	Limitations       []string        `json:"limitations"`
	PublicNote        string          `json:"public_note"`
	Source            PublicSource    `json:"source"`
	DisplayMetrics    []DisplayMetric `json:"display_metrics"`
}

// PublicDev and PublicTest are the contract's count objects.
type PublicDev struct {
	Cases   int `json:"cases"`
	Matched int `json:"matched"`
	Errors  int `json:"errors"`
}

type PublicTest struct {
	Cases      int `json:"cases"`
	Matched    int `json:"matched"`
	Mismatched int `json:"mismatched"`
	Errors     int `json:"errors"`
}

// PublicSource links a public note to the decision text and the committed rows.
type PublicSource struct {
	Decision string   `json:"decision"`
	Results  []string `json:"results"`
	Dataset  string   `json:"dataset"`
}

// DisplayMetric is a numerator and denominator taken from recorded counts.
type DisplayMetric struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Num   int    `json:"num"`
	Den   int    `json:"den"`
}

func famCount(dev, test []Case) (int, int) {
	f, s, _, _ := familyStats(dev, test)
	return f, s
}

func editionDefects() (dev, test int, err error) {
	b, err := os.ReadFile(filepath.Join(dataRoot(), "edition-defects.json"))
	if err != nil {
		return 0, 0, err
	}
	var m map[string][]string
	if err := json.Unmarshal(b, &m); err != nil {
		return 0, 0, err
	}
	return len(m["dev"]), len(m["test"]), nil
}

func countWhere(cases []Case, f func(Case) bool) int {
	n := 0
	for _, c := range cases {
		if f(c) {
			n++
		}
	}
	return n
}

// autoAccepted counts reserved rows of a feature that the deployed bar would
// have accepted although they were wrong (decided) or should have been reviewed.
func autoAccepted(rows []Row, bar float64) (decidedWrong, uncertainConfident int) {
	for _, r := range rows {
		if r.Confidence < bar {
			continue
		}
		switch r.Class {
		case "wrong_confident", "wrong_review":
			decidedWrong++
		case "overconfident":
			uncertainConfident++
		}
	}
	return
}

// Part102DeployedBar is the deployed Part 102 auto-accept bar (deploy workflow).
const Part102DeployedBar = 0.95

func decidedMetrics(prefix, bar string, s SplitSummary) []DisplayMetric {
	return []DisplayMetric{
		{prefix + "_decided_confident_correct", "Decided cases answered confidently and correctly (" + bar + ")", s.Decided.ConfidentCorrect, s.Decided.Cases},
		{prefix + "_uncertain_reviewed", "Cases with a missing fact sent to review", s.Uncertain.AbstainedCorrectly, s.Uncertain.Cases},
	}
}

func buildPublic(sum Summary, dev, test, rdev, rtest []Case, pubRec map[string]string) ([]PublicFeature, error) {
	res := filepath.Join(dataRoot(), "results")
	var out []PublicFeature
	feat := func(id string) *FeatureSummary {
		for i := range sum.Features {
			if sum.Features[i].ID == id {
				return &sum.Features[i]
			}
		}
		return nil
	}
	src := "eval/challenges/specialists/"
	tx, ph, p102, jp := feat("textile_specialist"), feat("pharma_specialist"), feat("part102_specialist"), feat("japan_engine")
	if tx == nil || ph == nil || p102 == nil || jp == nil || sum.Comparator == nil || sum.Comparator.Test == nil || sum.JapanComparator == nil || sum.Routing == nil {
		return nil, fmt.Errorf("public features need the study, both comparators and routing")
	}
	// 1. Deployed textile engine (post hoc).
	c := sum.Comparator
	fam, shared := famCount(byFeature(dev, "textile_specialist"), byFeature(test, "textile_specialist"))
	out = append(out, PublicFeature{ID: "textile_live_v2", Label: "Textile origin, deployed engine (textile-v2)", EvidenceKind: sum.EvidenceKind, PostHoc: true,
		CandidateRevision: c.CandidateRevision, DatasetSHA256: c.DatasetSHA256, DevDatasetSHA256: tx.DevDatasetSHA256, SourceFamilies: fam, SharedFamilies: shared,
		Development: PublicDev{c.DevBase.Cases, c.DevBase.Matched, c.DevBase.Errors}, Test: PublicTest{c.Test.Cases, c.Test.Matched, c.Test.Mismatched, c.Test.Errors},
		CostUSD: c.CostUSD, LatencyP50MS: c.LatencyP50MS, LatencyP95MS: c.LatencyP95MS, Limitations: c.Limitations,
		PublicNote:     fmt.Sprintf("Post hoc: the deployed textile prompt scored once on %d cases the author had already scored and seen, at the deployed 0.95 bar. Not a new holdout.", c.Test.Cases),
		Source:         PublicSource{"docs/DECISIONS.md, D971 amendments 2 and 3", []string{src + "results/textile-live-test.jsonl", src + "results/textile-dev-livebase.jsonl"}, src + "textile-test.jsonl"},
		DisplayMetrics: decidedMetrics("textile", "deployed bar 0.95", *c.Test)})
	// 2. Pharmaceutical, original cand-1.
	pest := countWhere(byFeature(test, "pharma_specialist"), func(x Case) bool { return x.ScenarioFamily == "pesticide" })
	fam, shared = famCount(byFeature(dev, "pharma_specialist"), byFeature(test, "pharma_specialist"))
	out = append(out, PublicFeature{ID: "pharma_specialist", Label: "Pharmaceutical origin", EvidenceKind: sum.EvidenceKind, CandidateRevision: ph.CandidateRevision,
		DatasetSHA256: ph.DatasetSHA256, DevDatasetSHA256: ph.DevDatasetSHA256, SourceFamilies: fam, SharedFamilies: shared,
		Development: PublicDev{ph.Development.Cases, ph.Development.Matched, ph.Development.Errors}, Test: PublicTest{ph.Test.Cases, ph.Test.Matched, ph.Test.Mismatched, ph.Test.Errors},
		CostUSD: ph.CostUSD, LatencyP50MS: ph.LatencyP50MS, LatencyP95MS: ph.LatencyP95MS, Limitations: dropTextile(ph.Limitations),
		PublicNote:     fmt.Sprintf("Frozen candidate scored once. The %d cases include %d formulated pesticides outside the current router's pharmaceutical scope; labels apply the engine's own rules to new facts.", ph.Test.Cases, pest),
		Source:         PublicSource{"docs/DECISIONS.md, D970 amendment 4 and D971", []string{src + "results/pharma-test.jsonl", src + "results/pharma-dev-cand1.jsonl"}, src + "pharma-test.jsonl"},
		DisplayMetrics: append(decidedMetrics("pharma", "study bar 0.92; deployed bar differs", ph.Test), DisplayMetric{"pharma_route_eligible", "Cases the current router sends to this engine", ph.Test.Cases - pest, ph.Test.Cases})})
	// 3. Part 102, original cand-1.
	prow, err := readRows(filepath.Join(res, "part102-test.jsonl"))
	if err != nil {
		return nil, err
	}
	dw, uc := autoAccepted(prow, Part102DeployedBar)
	fam, shared = famCount(byFeature(dev, "part102_specialist"), byFeature(test, "part102_specialist"))
	out = append(out, PublicFeature{ID: "part102_specialist", Label: "Part 102 marking origin (usmca-v5)", EvidenceKind: sum.EvidenceKind, CandidateRevision: p102.CandidateRevision,
		DatasetSHA256: p102.DatasetSHA256, DevDatasetSHA256: p102.DevDatasetSHA256, SourceFamilies: fam, SharedFamilies: shared,
		Development: PublicDev{p102.Development.Cases, p102.Development.Matched, p102.Development.Errors}, Test: PublicTest{p102.Test.Cases, p102.Test.Matched, p102.Test.Mismatched, p102.Test.Errors},
		CostUSD: p102.CostUSD, LatencyP50MS: p102.LatencyP50MS, LatencyP95MS: p102.LatencyP95MS, Limitations: dropTextile(p102.Limitations),
		PublicNote:     fmt.Sprintf("Frozen candidate scored once. %d of %d cases would still be auto-accepted wrongly at the deployed 0.95 bar (%d decided, %d with a missing fact).", dw+uc, p102.Test.Cases, dw, uc),
		Source:         PublicSource{"docs/DECISIONS.md, D970 amendment 4 and D971", []string{src + "results/part102-test.jsonl", src + "results/part102-dev-cand1.jsonl"}, src + "part102-test.jsonl"},
		DisplayMetrics: append(decidedMetrics("part102", "study bar 0.92; deployed bar differs", p102.Test), DisplayMetric{"part102_auto_accepted_wrong", "Cases auto-accepted wrongly at the deployed bar", dw + uc, p102.Test.Cases})})
	// 4. Japan, unmodified engine (post hoc), development baseline base3.
	j := sum.JapanComparator
	devRows, err := readRows(filepath.Join(res, pubRec["japan_dev_base"]))
	if err != nil {
		return nil, err
	}
	jdev, err := SummarizeSplit(byFeature(dev, "japan_engine"), devRows)
	if err != nil {
		return nil, err
	}
	_, edTest, err := editionDefects()
	if err != nil {
		return nil, err
	}
	fam, shared = famCount(byFeature(dev, "japan_engine"), byFeature(test, "japan_engine"))
	modelUnc := j.Test.Uncertain.Cases - j.Test.Preflight
	out = append(out, PublicFeature{ID: "japan_live_unmodified", Label: "Japan preference engine, as deployed", EvidenceKind: sum.EvidenceKind, PostHoc: true,
		CandidateRevision: j.CandidateRevision, DatasetSHA256: j.DatasetSHA256, DevDatasetSHA256: jp.DevDatasetSHA256, SourceFamilies: fam, SharedFamilies: shared,
		Development: PublicDev{jdev.Cases, jdev.Matched, jdev.Errors}, Test: PublicTest{j.Test.Cases, j.Test.Matched, j.Test.Mismatched, j.Test.Errors},
		CostUSD: j.CostUSD, LatencyP50MS: j.LatencyP50MS, LatencyP95MS: j.LatencyP95MS, Limitations: japanLimits(j.Limitations),
		PublicNote: fmt.Sprintf("Post hoc: the unmodified engine scored once on %d cases the author had already scored and seen. Includes %d potential HS-edition issues (approximate check against HS 2017 and 2022 code lists) and %d cases refused before any model call.", j.Test.Cases, edTest, j.Test.Preflight),
		Source:     PublicSource{"docs/DECISIONS.md, D971 amendments 4 and 5", []string{src + "results/japan-live-test.jsonl", src + "results/" + pubRec["japan_dev_base"]}, src + "japan-test.jsonl"},
		DisplayMetrics: []DisplayMetric{
			{"japan_decided_agreed", "Decided cases answered correctly", j.Test.Decided.Agreed, j.Test.Decided.Cases},
			{"japan_model_uncertain_undetermined", "Missing-fact cases the model left undetermined", j.Test.Uncertain.AbstainedCorrectly - j.Test.Preflight, modelUnc},
			{"japan_criterion_valid", "Qualifying answers naming a valid criterion", j.Test.CriterionValid, j.Test.CriterionValid + j.Test.CriterionInvalid}}})
	// 5. Routing.
	r := sum.Routing
	rf, rs, _, _ := familyStats(rdev, rtest)
	rdSHA, err := fileSHA(filepath.Join(dataRoot(), "routing-dev.jsonl"))
	if err != nil {
		return nil, err
	}
	rm := []DisplayMetric{{"routing_appropriate", "Specialist cases routed to their regime", r.Test.AppropriateAll.Matched, r.Test.AppropriateAll.Cases},
		{"routing_negative_controls", "General-origin controls kept general", r.Test.NegativeControl.Matched, r.Test.NegativeControl.Cases}}
	if g, ok := r.Test.Appropriate["pharma"]; ok {
		rm = append(rm, DisplayMetric{"routing_pharma", "Pharmaceutical cases routed to their regime", g.Matched, g.Cases})
	}
	out = append(out, PublicFeature{ID: "legal_routing", Label: "Legal-regime routing (router-v4)", EvidenceKind: sum.EvidenceKind, CandidateRevision: r.CandidateRevision,
		DatasetSHA256: r.DatasetSHA256, DevDatasetSHA256: rdSHA, SourceFamilies: rf, SharedFamilies: rs,
		Development: PublicDev{r.Development.Cases, r.Development.Matched, r.Development.Errors}, Test: PublicTest{r.Test.Cases, r.Test.Matched, r.Test.Mismatched, r.Test.Errors},
		CostUSD: r.CostUSD, LatencyP50MS: r.LatencyP50MS, LatencyP95MS: r.LatencyP95MS, Limitations: r.Limitations,
		PublicNote:     fmt.Sprintf("Scored once. %d specialist cases and %d general-origin controls are counted separately; pesticide cases and Japan are not routed.", r.Test.AppropriateAll.Cases, r.Test.NegativeControl.Cases),
		Source:         PublicSource{"docs/DECISIONS.md, D970 amendment 4 and D971", []string{src + "results/routing-test.jsonl", src + "results/routing-dev-base2.jsonl"}, src + "routing-test.jsonl"},
		DisplayMetrics: rm})
	return out, nil
}

// dropTextile removes the sentence about the textile candidate's prompt, which
// does not apply to the pharmaceutical and Part 102 cards.
func dropTextile(in []string) []string {
	var out []string
	for _, l := range in {
		if !strings.Contains(l, "textile-v1") {
			out = append(out, l)
		}
	}
	return out
}

// japanLimits words the HS-edition cases as potential issues: the check of
// schedules of HS 2012 or older uses the HS 2017 list and is approximate.
func japanLimits(in []string) []string {
	var out []string
	for _, l := range in {
		l = strings.Replace(l, "the 13 cases whose tariff codes fall outside the schedule's HS edition stay in the denominator",
			"the 13 potential HS-edition issues (cases with a code outside the schedule's edition by an approximate check against the HS 2017 and 2022 code lists; older editions are checked against HS 2017 only) stay in the denominator", 1)
		out = append(out, l)
	}
	return out
}
