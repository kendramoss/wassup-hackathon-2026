package main

// The scored Part 102 plan searches of D501, read offline (no model calls):
//
//	go run ./cmd/tariffshift plans102-eval [-series d501|d540] [-check]   (both in make reproduce)
//
// Each fixture (testdata/planner_part102, development rulings) has one
// plan search per series through the gateway
// (eval/tariffshift/plans102/runs/plan-p102-*-<series>-*.json): d501 with the
// general engine (D501), d540 with the Part 102 engine (D540).
// The 19 CFR 102.20 check runs on every move as on a Part 102 case (D441),
// and the shown plans are put in the D500 order.

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/hts"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/planner"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/planrule"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/tariffshift"
)

const (
	plans102Dir      = evalDir + "/plans102"
	plans102Fixtures = "testdata/planner_part102"
	plans102Runs     = plans102Dir + "/runs/plan-p102-%s-local-verifier-*%s-litellm.json"
)

// plans102Series are the scored series and their output names: d501 keeps
// the file names D502 published.
var plans102Series = map[string]string{"d501": "", "d540": "-d540"}

// plans102Later is the later CBP ruling on a changed setup of a picked
// product (D501): the rule is checked on its setup, not on a plan move.
var plans102Later = map[string]string{"N350133": "N354024"}

// Search102 is one scored search.
type Search102 struct {
	Ruling   string `json:"ruling"`
	Family   string `json:"family"`
	Target   string `json:"target"`
	CBP      string `json:"cbp_origin"`
	Moves    int    `json:"moves_searched"`
	Shown    int    `json:"shown"`
	Failed   bool   `json:"failed,omitempty"`
	Baseline struct {
		Status string `json:"status"`
		Origin string `json:"origin,omitempty"`
		Label  string `json:"label"`
		Agrees bool   `json:"agrees_with_cbp"`
	} `json:"current_setup"`
	ByStatus map[string]int `json:"moves_by_status"`
	// RuleShifts: moves the rule says reach the target; ShownShifts: of
	// those, shown (precedent-supported at the bar).
	RuleShifts  []string `json:"moves_rule_shifts"`
	ShownShifts []string `json:"shown_moves_rule_shifts"`
	// Flagged: moves precedent supports while the rule gives another
	// country (shown or below the bar).
	Flagged []string `json:"precedent_supports_rule_other_country"`
	// Order of shown plans: the planner's (tier, then net) and D500's.
	OrderBefore []string `json:"shown_order_planner"`
	OrderAfter  []string `json:"shown_order_d500"`
	// Engine against the rule on decided moves (target or not).
	Decided     int `json:"moves_decided"`
	EngineAgree int `json:"decided_engine_agrees_with_rule"`
	// Lean: moves the rule leaves undetermined only for de minimis while
	// naming a part that fails the shift (it does not shift unless de
	// minimis); the engine agrees when it does not predict the target (D540).
	Lean            int `json:"moves_rule_leans_no_shift"`
	LeanEngineAgree int `json:"lean_engine_agrees_with_rule"`
	// EnginePrompt is the prompt version of the engine that predicted the
	// moves (engine-v1: general; usmca-v1: Part 102); BaselineEngine its
	// origin for the current setup; Checked the moves sent to the precedent
	// check.
	EnginePrompt   string  `json:"engine_prompt"`
	BaselineEngine string  `json:"current_setup_engine_origin"`
	Checked        int     `json:"moves_precedent_checked"`
	GatewayUSD     float64 `json:"gateway_cost_usd"`
	WallSeconds    float64 `json:"wall_seconds"`
	Later          *Later  `json:"later_ruling,omitempty"`
}

// Later is the rule check on a later ruling's setup (CBP's good code, top
// D381 suggestion for every part).
type Later struct {
	Ruling string `json:"ruling"`
	CBP    string `json:"cbp_origin"`
	Status string `json:"status"`
	Origin string `json:"origin,omitempty"`
	Agrees bool   `json:"agrees_with_cbp"`
	Note   string `json:"note"`
}

// Summary102 is summary.json.
type Summary102 struct {
	Note       string      `json:"note"`
	Searches   []Search102 `json:"searches"`
	GatewayUSD float64     `json:"gateway_cost_usd"`
}

func cmdPlans102Eval(args []string) error {
	fl := flag.NewFlagSet("plans102-eval", flag.ExitOnError)
	check := fl.Bool("check", false, "recompute and fail if the committed files differ")
	series := fl.String("series", "d501", "run series: d501 (general engine) or d540 (Part 102 engine)")
	_ = fl.Parse(args)
	suffix, ok := plans102Series[*series]
	if !ok {
		return fmt.Errorf("-series must be d501 or d540")
	}
	rules, err := tariffshift.Default()
	if err != nil {
		return err
	}
	files, err := filepath.Glob(plans102Fixtures + "/*.json")
	if err != nil {
		return err
	}
	sort.Strings(files)
	note := "The scored Part 102 plan searches pre-registered in D501 (development rulings, one run each through an OpenAI-compatible LiteLLM gateway to Gemini; the general engine predicts each move). Each move carries the 19 CFR 102.20 check as on a Part 102 case (D441), and shown plans are put in the D500 order. Part codes are unverified suggestions (D382). Recompute: cd backend && go run ./cmd/tariffshift plans102-eval -check"
	if *series == "d540" {
		note = "The D540 re-run of the three D501 searches with the Part 102 engine predicting each move (seen development products, one run each through an OpenAI-compatible LiteLLM gateway to Gemini; not new evidence of accuracy). Scored as D501. Part codes are unverified suggestions (D382). Recompute: cd backend && go run ./cmd/tariffshift plans102-eval -series d540 -check"
	}
	sum := Summary102{Note: note}
	var rows []MoveRow
	for _, f := range files {
		id := strings.TrimSuffix(filepath.Base(f), ".json")
		var fx planFixture
		b, err := os.ReadFile(f)
		if err != nil {
			return err
		}
		if err := json.Unmarshal(b, &fx); err != nil || fx.Product == nil || fx.CurrentOrigin == nil {
			return fmt.Errorf("%s: %v", f, err)
		}
		var fam struct {
			Family string `json:"family"`
		}
		_ = json.Unmarshal(b, &fam)
		s := Search102{Ruling: fx.CurrentOrigin.Ruling, Family: fam.Family, Target: domain.NormalizeCountry(fx.Target),
			CBP: domain.NormalizeCountry(fx.CurrentOrigin.Country), ByStatus: map[string]int{}}
		runs, _ := filepath.Glob(fmt.Sprintf(plans102Runs, id, *series))
		if len(runs) != 1 {
			s.Failed = true // no result file (the search failed or did not run)
			sum.Searches = append(sum.Searches, s)
			continue
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
		res := run.Result
		s.Moves, s.Shown = res.Stats.MovesSearched, len(res.Plans)
		s.GatewayUSD, s.WallSeconds = round4(run.GatewayUSD), res.Stats.WallSeconds
		s.EnginePrompt = res.Baseline.Prediction.Trace.PromptVersion
		s.BaselineEngine = domain.NormalizeCountry(res.Baseline.Prediction.Origin)
		s.Checked = res.Stats.VerifierRuns
		sum.GatewayUSD += run.GatewayUSD
		for _, pl := range res.Plans {
			s.OrderBefore = append(s.OrderBefore, pl.Move.Key())
		}
		ann := planrule.Annotate(rules, *fx.Product, res, true)
		for _, pl := range ann.Plans {
			s.OrderAfter = append(s.OrderAfter, pl.Move.Key())
		}
		bc := ann.Baseline.RuleCheck
		s.Baseline.Status, s.Baseline.Origin, s.Baseline.Label = bc.Status, bc.Origin, bc.Label
		s.Baseline.Agrees = decided(bc.Status) && domain.NormalizeCountry(bc.Origin) == s.CBP
		base := moveRow(id, s.Target, *fx.Product, ann.Baseline, bc)
		base.Kind, base.Move, base.CBP = "baseline", "", s.CBP
		rows = append(rows, base)
		for _, pl := range append(append([]domain.Plan(nil), ann.Plans...), ann.Rejected...) {
			if pl.RejectionKind == domain.RejectCurrentSetup {
				continue
			}
			c := pl.RuleCheck
			rows = append(rows, moveRow(id, s.Target, *fx.Product, pl, c))
			s.ByStatus[c.Status]++
			key := pl.Move.Key()
			if c.Status == planrule.Shifts {
				s.RuleShifts = append(s.RuleShifts, key)
				if pl.Shown {
					s.ShownShifts = append(s.ShownShifts, key)
				}
			}
			if pl.Verdict != nil && pl.Verdict.PrecedentSupported && (c.Status == planrule.NoShift || c.Status == planrule.OtherCountry) {
				s.Flagged = append(s.Flagged, key)
			}
			predictsTarget := domain.NormalizeCountry(pl.Prediction.Origin) == s.Target
			if decided(c.Status) {
				s.Decided++
				if predictsTarget == (c.Status == planrule.Shifts) {
					s.EngineAgree++
				}
			}
			if c.Status == planrule.Undetermined && c.Why == planrule.WhyDeMinimis && len(c.Failing) > 0 {
				s.Lean++
				if !predictsTarget {
					s.LeanEngineAgree++
				}
			}
		}
		if later, ok := plans102Later[s.Ruling]; ok {
			l, err := laterCheck(rules, later)
			if err != nil {
				return err
			}
			s.Later = l
		}
		sum.Searches = append(sum.Searches, s)
	}
	sum.GatewayUSD = round4(sum.GatewayUSD)
	sb, _ := json.MarshalIndent(sum, "", "  ")
	if !*check {
		if err := os.MkdirAll(plans102Dir, 0o755); err != nil {
			return err
		}
	}
	if err := writeOrCheckPlans(plans102Dir+"/moves-dev"+suffix+".jsonl", jsonlOf(rows), *check); err != nil {
		return err
	}
	if err := writeOrCheckPlans(plans102Dir+"/summary"+suffix+".json", append(sb, '\n'), *check); err != nil {
		return err
	}
	for _, s := range sum.Searches {
		fmt.Printf("plans102-eval %s: %s → %s: %d moves, %d shown, rule shifts %d (shown %d), flagged %d, engine agrees %d/%d decided, %d/%d lean, current setup %s %s\n",
			*series, s.Ruling, s.Target, s.Moves, s.Shown, len(s.RuleShifts), len(s.ShownShifts), len(s.Flagged), s.EngineAgree, s.Decided, s.LeanEngineAgree, s.Lean, s.Baseline.Status, s.Baseline.Origin)
	}
	return nil
}

// laterCheck is the rule check on a later ruling's setup: CBP's good code
// and countries from its D362 full-text extraction, the top D381 dev
// suggestion for every part (the D382 input "CBP's code for the good, top
// suggestion for every part").
func laterCheck(rules *tariffshift.Rules, ruling string) (*Later, error) {
	inputs, err := readInputs(filepath.Join(evalDir, "inputs-dev-full_text.jsonl"))
	if err != nil {
		return nil, err
	}
	sugg, err := readJSONL[SuggestRow](plansSuggest)
	if err != nil {
		return nil, err
	}
	labels, err := usmcaLabels()
	if err != nil {
		return nil, err
	}
	var in *InputRow
	for i := range inputs {
		if inputs[i].Ruling == ruling {
			in = &inputs[i]
		}
	}
	if in == nil {
		return nil, fmt.Errorf("no D362 input for %s", ruling)
	}
	for _, s := range sugg {
		if s.Ruling != ruling || s.Set != "usmca" {
			continue
		}
		if s.Split != "dev" {
			return nil, fmt.Errorf("%s is not a development row", ruling)
		}
		top := map[int][]hts.PartCandidate{}
		for _, p := range s.Parts {
			top[p.Index] = p.Candidates
		}
		res := rules.Evaluate(withParts(in.Input, in.Input.GoodHTS, top))
		cbp := ""
		if o := labels[ruling].Origin; o != nil {
			cbp = domain.NormalizeCountry(*o)
		}
		l := &Later{Ruling: ruling, CBP: cbp, Status: res.Status, Origin: res.Origin,
			Note: "The rule on the later ruling's own setup, not on a plan move: no later ruling is a move the planner makes from the picked ruling (D501)."}
		l.Agrees = res.Status == tariffshift.Decided && domain.NormalizeCountry(res.Origin) == cbp
		return l, nil
	}
	return nil, fmt.Errorf("no D381 suggestion row for %s", ruling)
}
