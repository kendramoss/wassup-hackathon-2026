package main

// The recorded example on the site's /examples/part102-plan page (judge
// panel round 11, item 3): the scored D501 plan search for N348465, an
// electrical actuator finished in Canada from Chinese parts (CBP held
// China). Each move carries the 19 CFR 102.20 check as on a Part 102 case
// (D441), the shown plans are in the D500 order, and the broker draft's
// tariff-shift paragraph and Part 102 question (D502, D520) are built from
// the shown plan by the export package. The run file is read as committed
// and never re-run; the product's name comes from the fixture, cleaned in
// D532 after the run (the recorded run's facts still carry the ruling
// subject as the name, so the page does not show them). Nothing is called:
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
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/planner"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/planrule"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/tariffshift"
)

const (
	p102ExampleFixture = "n348465"
	p102ExampleOut     = "../frontend/src/lib/part102-plan-example.json"
)

type p102ExampleMove struct {
	Key           string                `json:"key"`
	Description   string                `json:"description"`
	PartsMoved    []string              `json:"parts_moved"`
	MovesMotor    bool                  `json:"moves_motor"`
	Shown         bool                  `json:"shown"`
	RejectionKind domain.RejectionKind  `json:"rejection_kind,omitempty"`
	Verdict       *planExampleVerdict   `json:"verdict,omitempty"`
	EngineOrigin  string                `json:"engine_origin,omitempty"`
	RuleCheck     *domain.PlanRuleCheck `json:"rule_check"`
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
	Note       string             `json:"note"`
	Ruling     string             `json:"ruling"`
	Subject    string             `json:"subject"`
	Date       string             `json:"date"`
	Split      string             `json:"split"`
	Run        string             `json:"run"`
	Fixture    string             `json:"fixture"`
	Product    string             `json:"product"`
	Family     string             `json:"family"`
	Target     string             `json:"target"`
	GoodHTS    string             `json:"good_hts"`
	CBPOrigin  string             `json:"cbp_origin"`
	Ranking    string             `json:"ranking"`
	Parts      []planExamplePart  `json:"parts"`
	Baseline   p102ExampleMove    `json:"baseline"`
	Shown      []p102ExampleMove  `json:"shown"`
	Others     []p102ExampleMove  `json:"others"`
	RuleShifts int                `json:"rule_shifts"`
	MotorMoves int                `json:"motor_moves"`
	Draft      p102ExampleDraft   `json:"draft"`
	GatewayUSD float64            `json:"gateway_cost_usd"`
	Plants     []domain.Plant     `json:"plants"`
	Operations []domain.Operation `json:"operations"`
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
	runs, _ := filepath.Glob(fmt.Sprintf(plans102Runs, p102ExampleFixture, "d501"))
	if len(runs) != 1 {
		return fmt.Errorf("%s: want one D501 run, found %d", p102ExampleFixture, len(runs))
	}
	rb, err := os.ReadFile(runs[0])
	if err != nil {
		return err
	}
	var run struct {
		planner.Result
		GatewayUSD float64 `json:"gateway_cost_usd"`
	}
	if err := json.Unmarshal(rb, &run); err != nil {
		return err
	}
	labels, err := usmcaLabels()
	if err != nil {
		return err
	}
	p := *fx.Product
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
			EngineOrigin: domain.NormalizeCountry(pl.Prediction.Origin), RuleCheck: pl.RuleCheck}
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
		Note:   "Recorded example, rebuilt from committed files by `cd backend && go run ./cmd/tariffshift plans102-example -check`. Plan search: the D501 development run, as committed (precedent checks and the general engine's predictions as recorded; no new model call). Codes: the top D381 development suggestion for each part, unverified; the good's code is CBP's. Rule check and order: the 19 CFR 102.20 layer on the product after each move, as on a Part 102 case (D441), and the D500 order. Draft: the export package's Part 102 letter for the shown plan (D502, D520). The product's name is the fixture's since D532; the recorded run's own facts still use the ruling subject as the name and are not shown.",
		Ruling: fx.CurrentOrigin.Ruling, Date: fx.Date, Split: "dev", Run: "eval/tariffshift/plans102/runs/" + filepath.Base(runs[0]),
		Fixture: "backend/" + fxPath, Product: p.Name, Family: fx.Family, Target: target, GoodHTS: p.HTS,
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
	for _, m := range append(append([]p102ExampleMove(nil), ex.Shown...), ex.Others...) {
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

	// The draft for the first shown plan, as the API builds it for a case
	// routed to Part 102 whose plan came from the general engine (D520).
	top := ann.Plans[0]
	moved := planner.Apply(p, top.Move)
	in := export.Input{ProductName: p.Name, Product: &moved, Facts: "[FACTS OF THE PROPOSED SETUP]", Requested: top.Prediction.Origin, Prediction: top.Prediction,
		PlanSummary: top.Description, RuleCheck: top.RuleCheck, Part102: true}
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
	fmt.Printf("plans102-example %s: %d shown, %d other moves, %d shift (all %d motor moves)\n", ex.Ruling, len(ex.Shown), len(ex.Others), ex.RuleShifts, ex.MotorMoves)
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
