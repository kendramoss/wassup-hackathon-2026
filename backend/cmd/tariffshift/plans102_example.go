package main

// The recorded example on the site's /examples/part102-plan page (judge
// panel round 11, item 3; round 12, item 4; round 13, item 3): the D630 plan
// search for N348465, an electrical actuator finished in Canada from Chinese
// parts (CBP held China), in which the production Part 102 engine
// (usmca-v5, the product's codes in the facts, D602) predicted each move.
// The predictions for the same move in the D541 run (usmca-v1) and the D501
// run (the general engine) are shown beside it. Each move carries the 19 CFR 102.20 check as on a Part 102 case
// (D441), the shown plans are in the D500 order, and the broker draft's
// tariff-shift paragraph, Part 102 question and regime note (D502, D520) are
// built from the first shown plan by the export package, with the regime
// read from the plan's own prediction as the API does. The run files are
// read as committed and never re-run; the product's name comes from the
// fixture, cleaned in D532 after the D501 and D541 runs (their recorded facts
// still carry the ruling subject as the name, so the page does not show them). Nothing
// is called:
//
//	go run ./cmd/tariffshift plans102-example [-check]   (in make reproduce)

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/export"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/httpapi"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/planner"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/planrule"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/router"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/tariffshift"
)

const (
	p102ExampleFixture = "n348465"
	p102ExampleOut     = "../frontend/src/lib/part102-plan-example.json"
)

type p102ExampleMove struct {
	Key           string               `json:"key"`
	Description   string               `json:"description"`
	PartsMoved    []string             `json:"parts_moved"`
	MovesMotor    bool                 `json:"moves_motor"`
	Shown         bool                 `json:"shown"`
	RejectionKind domain.RejectionKind `json:"rejection_kind,omitempty"`
	Verdict       *planExampleVerdict  `json:"verdict,omitempty"`
	EngineOrigin  string               `json:"engine_origin,omitempty"`
	// EngineV1 is usmca-v1's prediction for the same move in the D541 run;
	// EngineBefore the general engine's in the D501 run (before D540).
	EngineV1     string                `json:"engine_origin_d541,omitempty"`
	EngineBefore string                `json:"engine_origin_d501,omitempty"`
	RuleCheck    *domain.PlanRuleCheck `json:"rule_check"`
}

type p102ExampleDraft struct {
	// Plan is the shown plan the draft asks about (the first in the D500
	// order).
	Plan string `json:"plan"`
	// Question is the draft's Issue section; TariffShift its "Tariff-shift
	// check (19 C.F.R. § 102.20)" paragraph; Notes the reviewer notes
	// that name Part 102.
	Question    string   `json:"question"`
	TariffShift string   `json:"tariff_shift"`
	Notes       []string `json:"notes"`
}

type p102ExampleFile struct {
	Note       string            `json:"note"`
	Ruling     string            `json:"ruling"`
	Subject    string            `json:"subject"`
	Date       string            `json:"date"`
	Split      string            `json:"split"`
	Run        string            `json:"run"`
	RunV1      string            `json:"run_d541"`
	RunBefore  string            `json:"run_d501"`
	Engine     string            `json:"engine"`
	Fixture    string            `json:"fixture"`
	Product    string            `json:"product"`
	Family     string            `json:"family"`
	Target     string            `json:"target"`
	GoodHTS    string            `json:"good_hts"`
	CBPOrigin  string            `json:"cbp_origin"`
	Ranking    string            `json:"ranking"`
	Parts      []planExamplePart `json:"parts"`
	Baseline   p102ExampleMove   `json:"baseline"`
	Shown      []p102ExampleMove `json:"shown"`
	Others     []p102ExampleMove `json:"others"`
	RuleShifts int               `json:"rule_shifts"`
	MotorMoves int               `json:"motor_moves"`
	// Moves counts the moves shown and not shown; EngineAgrees those whose
	// engine prediction is the target exactly when the rule says the move
	// shifts, in this run, in the D541 run and in the D501 run.
	Moves              int                `json:"moves"`
	EngineAgrees       int                `json:"engine_agrees_with_rule"`
	EngineAgreesV1     int                `json:"engine_agrees_with_rule_d541"`
	EngineAgreesBefore int                `json:"engine_agrees_with_rule_d501"`
	Draft              p102ExampleDraft   `json:"draft"`
	GatewayUSD         float64            `json:"gateway_cost_usd"`
	Plants             []domain.Plant     `json:"plants"`
	Operations         []domain.Operation `json:"operations"`
}

func cmdPlans102Example(args []string) error {
	fl := flag.NewFlagSet("plans102-example", flag.ExitOnError)
	check := fl.Bool("check", false, "rebuild and compare with the committed file; write nothing")
	_ = fl.Parse(args)
	rules, err := tariffshift.Default()
	if err != nil {
		return err
	}
	fxPath := filepath.Join(plans102Fixtures, p102ExampleFixture+".json")
	var fx struct {
		planFixture
		Date   string `json:"date"`
		Family string `json:"family"`
	}
	b, err := os.ReadFile(fxPath)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(b, &fx); err != nil || fx.Product == nil || fx.CurrentOrigin == nil {
		return fmt.Errorf("%s: %v", fxPath, err)
	}
	type runFile struct {
		planner.Result
		GatewayUSD float64 `json:"gateway_cost_usd"`
	}
	load := func(series string) (runFile, string, error) {
		var run runFile
		runs, _ := filepath.Glob(fmt.Sprintf(plans102Runs, p102ExampleFixture, series))
		if len(runs) != 1 {
			return run, "", fmt.Errorf("%s: want one %s run, found %d", p102ExampleFixture, series, len(runs))
		}
		rb, err := os.ReadFile(runs[0])
		if err != nil {
			return run, "", err
		}
		return run, runs[0], json.Unmarshal(rb, &run)
	}
	run, runPath, err := load("d630")
	if err != nil {
		return err
	}
	v1, v1Path, err := load("d540")
	if err != nil {
		return err
	}
	before, beforePath, err := load("d501")
	if err != nil {
		return err
	}
	engineV1, engineBefore := map[string]string{}, map[string]string{}
	for _, pl := range append(append([]domain.Plan(nil), v1.Plans...), v1.Rejected...) {
		engineV1[pl.Move.Key()] = domain.NormalizeCountry(pl.Prediction.Origin)
	}
	for _, pl := range append(append([]domain.Plan(nil), before.Plans...), before.Rejected...) {
		engineBefore[pl.Move.Key()] = domain.NormalizeCountry(pl.Prediction.Origin)
	}
	labels, err := usmcaLabels()
	if err != nil {
		return err
	}
	p := *fx.Product
	// Plan sentences are rebuilt from the moves with the current wording
	// (planner.Redescribe, as the server does for a stored search): the
	// recorded run's titles predate D641. Moves, predictions and verdicts are
	// as recorded.
	tmpls, err := planner.LoadTemplates()
	if err != nil {
		return err
	}
	planner.Redescribe(&run.Result, &p, tmpls)
	ann := planrule.Annotate(rules, p, run.Result, true)
	target := domain.NormalizeCountry(ann.Target)

	motor := map[string]bool{} // the groups holding the good's own-heading part
	for _, c := range p.Components {
		if strings.HasPrefix(c.HTS, "8501") {
			motor[c.Group] = true
		}
	}
	mk := func(pl domain.Plan) p102ExampleMove {
		m := p102ExampleMove{Key: pl.Move.Key(), Description: pl.Description, Shown: pl.Shown, RejectionKind: pl.RejectionKind,
			EngineOrigin: domain.NormalizeCountry(pl.Prediction.Origin), EngineV1: engineV1[pl.Move.Key()], EngineBefore: engineBefore[pl.Move.Key()], RuleCheck: pl.RuleCheck}
		for _, gm := range pl.Move {
			if motor[gm.Group] {
				m.MovesMotor = true
			}
			for _, c := range p.Components {
				if c.Group == gm.Group {
					m.PartsMoved = append(m.PartsMoved, c.Name)
				}
			}
		}
		if v := pl.Verdict; v != nil {
			m.Verdict = &planExampleVerdict{PrecedentSupported: v.PrecedentSupported, SupportingRuling: v.SupportingRuling, CounterRuling: v.CounterRuling, Confidence: v.Confidence, Reasoning: v.Reasoning}
		}
		return m
	}
	l := labels[fx.CurrentOrigin.Ruling]
	ex := p102ExampleFile{
		Note:   "Recorded example, rebuilt from committed files by `cd backend && go run ./cmd/tariffshift plans102-example -check`. Plan search: the D630 development run, as committed, in which the production Part 102 engine (usmca-v5, the product's codes in the facts, D602) predicted each move; precedent checks and predictions as recorded, no new model call; plan sentences rebuilt from the recorded moves with the current wording (D641). Beside each move, the predictions for the same move in the D541 run (usmca-v1) and the D501 run (the general engine). Codes: the top D381 development suggestion for each part, unverified; the good's code is CBP's. Rule check and order: the 19 CFR 102.20 layer on the product after each move, as on a Part 102 case (D441), and the D500 order. Draft: the export package's Part 102 letter for the first shown plan, under the regime of that plan's prediction as the API reads it (D502, D520). The product's name is the fixture's since D532, after the D501 and D541 runs; their recorded facts still use the ruling subject as the name and are not shown.",
		Ruling: fx.CurrentOrigin.Ruling, Date: fx.Date, Split: "dev", Run: "eval/tariffshift/plans102/runs/" + filepath.Base(runPath),
		RunV1:     "eval/tariffshift/plans102/runs/" + filepath.Base(v1Path),
		RunBefore: "eval/tariffshift/plans102/runs/" + filepath.Base(beforePath),
		Fixture:   "backend/" + fxPath, Product: p.Name, Family: fx.Family, Target: target, GoodHTS: p.HTS,
		CBPOrigin: domain.NormalizeCountry(fx.CurrentOrigin.Country), Ranking: planrule.RankingRule, GatewayUSD: round4(run.GatewayUSD),
		Plants: p.Plants, Operations: p.Operations,
	}
	if l.Subject != "" {
		ex.Subject = l.Subject
	}
	for _, c := range p.Components {
		ex.Parts = append(ex.Parts, planExamplePart{Name: c.Name, MadeIn: c.MadeIn, Plant: c.Plant, Group: c.Group, HTS: c.HTS, FromRun: "suggested"})
	}
	ex.Baseline = mk(ann.Baseline)
	ex.Baseline.Description = "No change: keep the current setup"
	for _, pl := range ann.Plans {
		ex.Shown = append(ex.Shown, mk(pl))
	}
	for _, pl := range ann.Rejected {
		if pl.RejectionKind == domain.RejectCurrentSetup || len(pl.Move) == 0 {
			continue
		}
		ex.Others = append(ex.Others, mk(pl))
	}
	agrees := func(origin string, m p102ExampleMove) bool {
		shifts := m.RuleCheck != nil && m.RuleCheck.Status == planrule.Shifts
		return origin != "" && (origin == target) == shifts
	}
	for _, m := range append(append([]p102ExampleMove(nil), ex.Shown...), ex.Others...) {
		ex.Moves++
		if agrees(m.EngineOrigin, m) {
			ex.EngineAgrees++
		}
		if agrees(m.EngineV1, m) {
			ex.EngineAgreesV1++
		}
		if agrees(m.EngineBefore, m) {
			ex.EngineAgreesBefore++
		}
		if m.EngineV1 == "" {
			return fmt.Errorf("%s: move %s is not in the D541 run", p102ExampleFixture, m.Key)
		}
		if m.EngineBefore == "" {
			return fmt.Errorf("%s: move %s is not in the D501 run", p102ExampleFixture, m.Key)
		}
		if m.RuleCheck != nil && m.RuleCheck.Status == planrule.Shifts {
			ex.RuleShifts++
			if !m.MovesMotor {
				return fmt.Errorf("%s: a move without the motor shifts: %s", p102ExampleFixture, m.Key)
			}
		}
		if m.MovesMotor {
			ex.MotorMoves++
			if m.RuleCheck == nil || m.RuleCheck.Status != planrule.Shifts {
				return fmt.Errorf("%s: a motor move does not shift: %s", p102ExampleFixture, m.Key)
			}
		}
	}
	if len(ex.Shown) == 0 {
		return fmt.Errorf("%s: no shown plan", p102ExampleFixture)
	}

	// The draft for the first shown plan, as the API builds it: the regime
	// is the one that made the plan's prediction, read from its trace (the
	// Part 102 engine here, D540); a general-engine plan on a Part 102 case
	// would be asked under Part 102 with D520's note instead.
	top := ann.Plans[0]
	moved := planner.Apply(p, top.Move)
	regime := httpapi.PredictionRegime(top.Prediction)
	ex.Engine = top.Prediction.Trace.PromptVersion
	if regime != router.RegimeUSMCA {
		return fmt.Errorf("%s: the shown plan's prediction is not the Part 102 engine's (%q)", p102ExampleFixture, top.Prediction.Trace.PromptVersion)
	}
	in := export.Input{ProductName: p.Name, Product: &moved, Facts: "[FACTS OF THE PROPOSED SETUP]", Requested: top.Prediction.Origin, Prediction: top.Prediction,
		PlanSummary: top.Description, RuleCheck: top.RuleCheck, Regime: regime}
	if v := top.Verdict; v != nil {
		in.Supporting, in.Counter = v.SupportingRuling, v.CounterRuling
	}
	in.CurrentRuled = fx.CurrentOrigin.Country + " (CBP ruling " + fx.CurrentOrigin.Ruling + ")"
	if t, err := time.Parse(time.DateOnly, fx.Date); err == nil {
		in.CurrentRuled = fx.CurrentOrigin.Country + " (CBP ruling " + fx.CurrentOrigin.Ruling + ", " + t.Format("2 January 2006") + ")"
	}
	md := export.Markdown(in)
	ex.Draft = p102ExampleDraft{Plan: top.Description, Question: section(md, "## Issue"), TariffShift: export.TariffShiftLine(top.RuleCheck, "[CONFIRM WITH COUNSEL]")}
	for _, n := range export.Notes(in, false) {
		if strings.Contains(n, "Part 102") {
			ex.Draft.Notes = append(ex.Draft.Notes, n)
		}
	}
	if ex.Draft.Question == "" || ex.Draft.TariffShift == "" || !strings.Contains(md, ex.Draft.TariffShift) || !strings.Contains(ex.Draft.Question, "102") {
		return fmt.Errorf("%s: the draft has no Part 102 question or 102.20 paragraph", p102ExampleFixture)
	}
	if strings.Contains(strings.ToLower(md), "country of origin of the country of origin") || strings.Contains(md, "a The ") {
		return fmt.Errorf("%s: the draft doubles the product name", p102ExampleFixture)
	}

	out, _ := json.MarshalIndent(ex, "", " ")
	if err := writeOrCheck(p102ExampleOut, append(out, '\n'), *check); err != nil {
		return fmt.Errorf("%w (plans102-example)", err)
	}
	fmt.Printf("plans102-example %s: %d shown, %d other moves, %d shift (all %d motor moves); engine (%s) agrees with the rule on %d of %d (D541 usmca-v1: %d; D501 general engine: %d)\n", ex.Ruling, len(ex.Shown), len(ex.Others), ex.RuleShifts, ex.MotorMoves, ex.Engine, ex.EngineAgrees, ex.Moves, ex.EngineAgreesV1, ex.EngineAgreesBefore)
	return nil
}

// section is the body of a Markdown section, up to the next heading.
func section(md, heading string) string {
	i := strings.Index(md, heading+"\n\n")
	if i < 0 {
		return ""
	}
	rest := md[i+len(heading)+2:]
	if j := strings.Index(rest, "\n## "); j >= 0 {
		rest = rest[:j]
	}
	return strings.TrimSpace(rest)
}
