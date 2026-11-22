package main

// The recorded example on the site's /examples/plan-rule-check page (judge
// panel round 10, item 2a): one committed development plan search, N354220
// (a survival tool, target Vietnam), with the 19 CFR 102.20 check on each
// move as the API adds it (D441). An earlier ruling supports moving the
// machining to Vietnam, but the good is still finished in China, so the rule
// gives China and the move is flagged for a broker (D442). Nothing is called;
// the file is rebuilt from committed rows:
//
//	go run ./cmd/tariffshift plans-example [-check]   (in make reproduce)

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/planner"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/planrule"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/tariffshift"
)

const (
	planExampleFixture = "n354220"
	planExampleOut     = "../frontend/src/lib/plan-rule-example.json"
)

// planExamplePart is one part as the check reads it: the top suggested code
// from the committed D381 development run.
type planExamplePart struct {
	Name    string `json:"name"`
	MadeIn  string `json:"made_in"`
	Plant   string `json:"plant"`
	Group   string `json:"group"`
	HTS     string `json:"hts"`
	FromRun string `json:"code_source"`
}

// planExampleMove is one move of the recorded search, in the plan API's
// shape (the parts the page reads).
type planExampleMove struct {
	Move          domain.Move           `json:"move"`
	Description   string                `json:"description"`
	Shown         bool                  `json:"shown"`
	RejectionKind domain.RejectionKind  `json:"rejection_kind,omitempty"`
	Reason        string                `json:"rejection_reason,omitempty"`
	Verdict       *planExampleVerdict   `json:"verdict,omitempty"`
	EngineOrigin  string                `json:"engine_origin"`
	RuleCheck     *domain.PlanRuleCheck `json:"rule_check"`
}

type planExampleVerdict struct {
	PrecedentSupported bool    `json:"precedent_supported"`
	SupportingRuling   string  `json:"supporting_ruling,omitempty"`
	CounterRuling      string  `json:"counter_ruling,omitempty"`
	Confidence         float64 `json:"confidence"`
	Reasoning          string  `json:"reasoning"`
}

type planExampleFile struct {
	Note       string             `json:"note"`
	Ruling     string             `json:"ruling"`
	Date       string             `json:"date"`
	Split      string             `json:"split"`
	Run        string             `json:"run"`
	Suggest    string             `json:"suggestions"`
	Product    string             `json:"product"`
	Family     string             `json:"family"`
	Target     string             `json:"target"`
	GoodHTS    string             `json:"good_hts"`
	CBPOrigin  string             `json:"cbp_origin"`
	Plants     []domain.Plant     `json:"plants"`
	Parts      []planExamplePart  `json:"parts"`
	Operations []domain.Operation `json:"operations"`
	Baseline   planExampleMove    `json:"baseline"`
	Moves      []planExampleMove  `json:"moves"`
	Flagged    int                `json:"flagged"`
}

func cmdPlansExample(args []string) error {
	fl := flag.NewFlagSet("plans-example", flag.ExitOnError)
	check := fl.Bool("check", false, "rebuild and compare with the committed file; write nothing")
	_ = fl.Parse(args)
	rules, err := tariffshift.Default()
	if err != nil {
		return err
	}
	fxPath := filepath.Join(familyFixtures, planExampleFixture+".json")
	var fx struct {
		planFixture
		Date   string `json:"date"`
		Family string `json:"family"`
	}
	b, err := os.ReadFile(fxPath)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(b, &fx); err != nil || fx.Product == nil {
		return fmt.Errorf("%s: %v", fxPath, err)
	}
	sugg, err := readJSONL[SuggestRow](plansSuggest)
	if err != nil {
		return err
	}
	var s *SuggestRow
	for i := range sugg {
		if sugg[i].Set == "family" && sugg[i].Ruling == "N354220" {
			s = &sugg[i]
		}
	}
	if s == nil || s.Split != "dev" {
		return fmt.Errorf("%s: no development suggestion row", planExampleFixture)
	}
	p, err := withTopCodes(*fx.Product, *s)
	if err != nil {
		return err
	}
	runPath := fmt.Sprintf(plansRunsGlob, planExampleFixture)
	rb, err := os.ReadFile(runPath)
	if err != nil {
		return err
	}
	var res planner.Result
	if err := json.Unmarshal(rb, &res); err != nil {
		return err
	}
	target := domain.NormalizeCountry(res.Target)
	ex := planExampleFile{
		Note:   "Recorded example, rebuilt from committed files by `cd backend && go run ./cmd/tariffshift plans-example -check`. Plan search: the committed development run (precedent checks as recorded; no new model call). Codes: the top suggestion for the good and each part from the committed D381 development run, unverified. Rule check: the deterministic 19 CFR 102.20 layer on the product after each move, as the plan API adds it (D441), informational because the case is decided under substantial transformation.",
		Ruling: "N354220", Date: fx.Date, Split: s.Split,
		Run: "eval/results/" + filepath.Base(runPath), Suggest: "eval/tariffshift/parts/suggest-dev-v1.jsonl",
		Product: p.Name, Family: fx.Family, Target: target, GoodHTS: p.HTS, Plants: p.Plants, Operations: p.Operations,
	}
	if fx.CurrentOrigin != nil {
		ex.CBPOrigin = domain.NormalizeCountry(fx.CurrentOrigin.Country)
	}
	for _, c := range p.Components {
		ex.Parts = append(ex.Parts, planExamplePart{Name: c.Name, MadeIn: c.MadeIn, Plant: c.Plant, Group: c.Group, HTS: c.HTS, FromRun: "suggested"})
	}
	mk := func(pl domain.Plan) planExampleMove {
		m := planExampleMove{Move: pl.Move, Description: pl.Description, Shown: pl.Shown, RejectionKind: pl.RejectionKind, Reason: pl.RejectionReason,
			EngineOrigin: domain.NormalizeCountry(pl.Prediction.Origin), RuleCheck: planrule.Check(rules, p, pl, target, planrule.AppliesInformational)}
		if v := pl.Verdict; v != nil {
			m.Verdict = &planExampleVerdict{PrecedentSupported: v.PrecedentSupported, SupportingRuling: v.SupportingRuling, CounterRuling: v.CounterRuling, Confidence: v.Confidence, Reasoning: v.Reasoning}
		}
		return m
	}
	ex.Baseline = mk(domain.Plan{Description: "No change: keep the current setup"})
	ex.Baseline.EngineOrigin = domain.NormalizeCountry(res.Baseline.Prediction.Origin)
	for _, pl := range append(append([]domain.Plan(nil), res.Plans...), res.Rejected...) {
		if len(pl.Move) == 0 {
			continue
		}
		m := mk(pl)
		if m.RuleCheck.Review {
			ex.Flagged++
		}
		ex.Moves = append(ex.Moves, m)
	}
	// The page says an earlier ruling supports a move the rule does not.
	if ex.Flagged == 0 {
		return fmt.Errorf("%s: no move is flagged any more", planExampleFixture)
	}
	for _, m := range ex.Moves {
		if m.RuleCheck.Review && (m.Verdict == nil || !m.Verdict.PrecedentSupported || m.RuleCheck.Origin == target) {
			return fmt.Errorf("%s: a flagged move is not 'precedent supports, rule gives another country': %+v", planExampleFixture, m.RuleCheck)
		}
	}
	out, _ := json.MarshalIndent(ex, "", " ")
	if err := writeOrCheck(planExampleOut, append(out, '\n'), *check); err != nil {
		return fmt.Errorf("%w (plans-example)", err)
	}
	fmt.Printf("plans-example %s: %d moves, %d flagged\n", ex.Ruling, len(ex.Moves), ex.Flagged)
	return nil
}
