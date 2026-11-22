package main

// The scored Part 102 plan searches of D501, read offline (no model calls):
//
//	go run ./cmd/tariffshift plans102-eval [-series d501|d540|heldout|heldout2] [-check]   (all in make reproduce)
//
// Each fixture (testdata/planner_part102, development rulings) has one
// plan search per series through the gateway
// (eval/tariffshift/plans102/runs/plan-p102-*-<series>-*.json): d501 with the
// general engine (D501), d540 with the Part 102 engine (D540). heldout is
// the first held-out evaluation (D570): the rulings frozen in
// eval/tariffshift/plans102/heldout/frozen.json, fixtures in
// testdata/planner_part102_heldout built by fixed rules, the Part 102
// engine on every move. heldout2 is the second, pre-registered pass on the
// same frozen set (D630): the 20-family check of D581, fixtures in
// testdata/planner_part102_heldout2 (D571's where the family is unchanged),
// the production Part 102 engine (usmca-v5, the product's codes in the
// facts, D602).
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
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/evalkit"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/hts"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/planner"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/planrule"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/tariffshift"
)

const (
	plans102Dir      = evalDir + "/plans102"
	plans102Fixtures = "testdata/planner_part102"
	plans102Runs     = plans102Dir + "/runs/plan-p102-%s-local-verifier-*%s-litellm.json"
	// The held-out series (D570).
	plans102HeldoutFixtures = "testdata/planner_part102_heldout"
	plans102HeldoutRuns     = plans102Dir + "/runs/plan-p102h-%s-local-verifier-*%s-litellm.json"
	plans102Frozen          = plans102Dir + "/heldout/frozen.json"
	plans102FamilyCheck     = "../eval/results/families-coverage-final-all-litellm-p102-heldout.jsonl"
	// The second pass on the same set (D630).
	plans102Heldout2Fixtures = "testdata/planner_part102_heldout2"
	plans102FamilyCheck2     = "../eval/results/families-coverage-final-all-litellm-p102-heldout-f20.jsonl"
)

// plans102Series are the scored series and their output names: d501 keeps
// the file names D502 published.
var plans102Series = map[string]string{"d501": "", "d540": "-d540", "heldout": "-heldout", "heldout2": "-heldout2"}

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
	// Held-out series only (D570): each shown plan's rule status in the
	// D500 order, materials left out for having no country, whether the
	// engine's current-setup origin is CBP's holding, and whether the
	// suggester's input carried a code (D413).
	ShownStatus   []string `json:"shown_rule_status,omitempty"`
	PartsLeftOut  *int     `json:"parts_left_out,omitempty"`
	EngineCurrent *bool    `json:"current_setup_engine_agrees_with_cbp,omitempty"`
	LeakedInput   bool     `json:"suggester_input_carried_a_code,omitempty"`
	// Second pass only (D630): rows (current setup and moves) the rule
	// check marks for review (the D441 precedent note or the D591 engine
	// note), and whether the fixture is D571's, byte for byte.
	ReviewFlags *int  `json:"rows_flagged_for_review,omitempty"`
	SameFixture *bool `json:"fixture_same_as_d571,omitempty"`
}

// FrozenRow is one ruling of the frozen held-out set and what became of it.
type FrozenRow struct {
	Ruling     string  `json:"ruling"`
	CBP        string  `json:"cbp_origin"`
	Target     string  `json:"target"`
	Family     string  `json:"family"`
	Confidence float64 `json:"family_confidence"`
	Outcome    string  `json:"outcome"`
}

// Rate is k of n with its exact 95% Wilson interval (n >= 5 only).
type Rate struct {
	K  int       `json:"k"`
	N  int       `json:"n"`
	CI []float64 `json:"ci95,omitempty"`
}

func rate(k, n int) Rate {
	r := Rate{K: k, N: n}
	if n >= 5 {
		_, lo, hi := evalkit.Wilson(k, n)
		r.CI = []float64{round4(lo), round4(hi)}
	}
	return r
}

// Pooled102 are the pooled held-out figures pre-registered in D570.
type Pooled102 struct {
	Frozen               int  `json:"frozen"`
	Template             Rate `json:"template_found"`
	Planned              Rate `json:"planned_end_to_end"`
	SearchesWithMoves    int  `json:"searches_with_a_move"`
	Moves                int  `json:"moves"`
	Decided              int  `json:"moves_decided"`
	EngineAgreesDecided  Rate `json:"decided_moves_engine_agrees_with_rule"`
	EngineAgreesLean     Rate `json:"lean_moves_engine_agrees_with_rule"`
	Shown                int  `json:"shown_plans"`
	ShownShift           Rate `json:"shown_plans_rule_shifts"`
	Flagged              int  `json:"moves_flagged"`
	CurrentRuleDecided   Rate `json:"current_setup_rule_decided"`
	CurrentRuleAgrees    Rate `json:"current_setup_rule_decided_agrees_with_cbp"`
	CurrentEngineAgrees  Rate `json:"current_setup_engine_agrees_with_cbp"`
	Backtests            int  `json:"backtests"`
	WithoutLeakedPlanned int  `json:"planned_without_leaked_inputs"`
	ReviewFlags          *int `json:"rows_flagged_for_review,omitempty"`
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
	Frozen     []FrozenRow `json:"frozen_set,omitempty"`
	Pooled     *Pooled102  `json:"pooled,omitempty"`
	// Second pass only (D630): the pooled figures on the searches whose
	// fixture is D571's, so only the engine (and the run) differs.
	PooledSameFixture *Pooled102 `json:"pooled_same_fixture_as_d571,omitempty"`
}

func cmdPlans102Eval(args []string) error {
	fl := flag.NewFlagSet("plans102-eval", flag.ExitOnError)
	check := fl.Bool("check", false, "recompute and fail if the committed files differ")
	series := fl.String("series", "d501", "run series: d501 (general engine), d540 (Part 102 engine), heldout (D570) or heldout2 (D630)")
	_ = fl.Parse(args)
	suffix, ok := plans102Series[*series]
	if !ok {
		return fmt.Errorf("-series must be d501, d540, heldout or heldout2")
	}
	second := *series == "heldout2"
	heldout := *series == "heldout" || second
	fixtures, runsPattern, movesName := plans102Fixtures, plans102Runs, "/moves-dev"
	familyCheck := plans102FamilyCheck
	if heldout {
		fixtures, runsPattern, movesName = plans102HeldoutFixtures, plans102HeldoutRuns, "/moves"
	}
	if second {
		fixtures, familyCheck = plans102Heldout2Fixtures, plans102FamilyCheck2
	}
	rules, err := tariffshift.Default()
	if err != nil {
		return err
	}
	files, err := filepath.Glob(fixtures + "/*.json")
	if err != nil {
		return err
	}
	sort.Strings(files)
	note := "The scored Part 102 plan searches pre-registered in D501 (development rulings, one run each through an OpenAI-compatible LiteLLM gateway to Gemini; the general engine predicts each move). Each move carries the 19 CFR 102.20 check as on a Part 102 case (D441), and shown plans are put in the D500 order. Part codes are unverified suggestions (D382). Recompute: cd backend && go run ./cmd/tariffshift plans102-eval -check"
	if *series == "d540" {
		note = "The D540 re-run of the three D501 searches with the Part 102 engine predicting each move (seen development products, one run each through an OpenAI-compatible LiteLLM gateway to Gemini; not new evidence of accuracy). Scored as D501. Part codes are unverified suggestions (D382). Recompute: cd backend && go run ./cmd/tariffshift plans102-eval -series d540 -check"
	}
	if heldout {
		note = "The first held-out evaluation of the Part 102 plan pipeline (D570): held-out Part 102 rulings frozen by a fixed rule before any run (heldout/frozen.json), the production family check, fixtures built by fixed rules, one plan search each with the Part 102 engine predicting every move, through an OpenAI-compatible LiteLLM gateway to Gemini. Each move carries the 19 CFR 102.20 check as on a Part 102 case (D441); shown plans are put in the D500 order. Part codes are unverified held-out suggestions (D382). Recompute: cd backend && go run ./cmd/tariffshift plans102-eval -series heldout -check"
	}
	if second {
		note = "The second, pre-registered pass on the D570 held-out Part 102 plan set (D630): the same 18 frozen rulings, the second scoring of this set. Templates from the 20-family check committed in D581 (not re-run); D571's group rows and fixtures where the family is unchanged, otherwise one group call and a fixture by the same fixed rules; one plan search each with the production Part 102 engine (usmca-v5, the product's codes in the facts, D602) predicting every move, through an OpenAI-compatible LiteLLM gateway to Gemini. Scored as D571. Part codes are unverified held-out suggestions (D382). D670: every planned ruling is a window-1 ruling on which usmca-v5 was scored in D601 before it was adopted (D602), so this pass is a candidate comparison, not new held-out evidence; the current-setup result is D601's. The precedent verdicts were made with the product's codes also in the check's facts (D640). Recompute: cd backend && go run ./cmd/tariffshift plans102-eval -series heldout2 -check"
	}
	sum := Summary102{Note: note}
	frozenMats := map[string]int{}
	leaked := map[string]bool{"H351116": true, "N357063": true} // D413: the two frozen rulings whose suggester input carried a code
	if heldout {
		fr, err := heldoutFrozen(familyCheck)
		if err != nil {
			return err
		}
		sum.Frozen = fr.rows
		frozenMats = fr.withCountry
	}
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
		runs, _ := filepath.Glob(fmt.Sprintf(runsPattern, id, *series))
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
		if heldout {
			left := frozenMats[s.Ruling] - len(fx.Product.Components)
			agrees := s.BaselineEngine == s.CBP
			s.PartsLeftOut, s.EngineCurrent, s.LeakedInput = &left, &agrees, leaked[s.Ruling]
			if second {
				old, err := os.ReadFile(filepath.Join(plans102HeldoutFixtures, filepath.Base(f)))
				same := err == nil && string(old) == string(b)
				s.SameFixture = &same
				n := 0
				if bc.Review {
					n++
				}
				for _, pl := range append(append([]domain.Plan(nil), ann.Plans...), ann.Rejected...) {
					if pl.RejectionKind != domain.RejectCurrentSetup && pl.RuleCheck != nil && pl.RuleCheck.Review {
						n++
					}
				}
				s.ReviewFlags = &n
			}
			for _, pl := range ann.Plans {
				st := "undetermined"
				if pl.RuleCheck != nil {
					st = pl.RuleCheck.Status
				}
				s.ShownStatus = append(s.ShownStatus, st)
			}
		}
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
	if heldout {
		sum.Pooled = pool102(sum)
	}
	if second {
		same := sum
		same.Searches, same.Frozen = nil, nil
		for _, s := range sum.Searches {
			if s.SameFixture != nil && *s.SameFixture {
				same.Searches = append(same.Searches, s)
			}
		}
		sum.PooledSameFixture = pool102(same)
		sum.PooledSameFixture.Frozen, sum.PooledSameFixture.Template, sum.PooledSameFixture.Planned = 0, Rate{}, Rate{}
	}
	sb, _ := json.MarshalIndent(sum, "", "  ")
	if !*check {
		if err := os.MkdirAll(plans102Dir, 0o755); err != nil {
			return err
		}
	}
	if err := writeOrCheckPlans(plans102Dir+movesName+suffix+".jsonl", jsonlOf(rows), *check); err != nil {
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

type frozenSet struct {
	rows        []FrozenRow
	withCountry map[string]int // materials in the extraction, per frozen ruling
}

// heldoutFrozen reads the frozen set (D570) and the family check run on it:
// a ruling whose answer is "none" or below the bar has no plan template.
func heldoutFrozen(familyCheck string) (frozenSet, error) {
	var fz struct {
		Final   []string `json:"final"`
		Rulings []struct {
			Ruling string `json:"ruling"`
			CBP    string `json:"cbp_origin"`
			Target string `json:"target"`
			Mats   int    `json:"materials"`
		} `json:"rulings"`
	}
	b, err := os.ReadFile(plans102Frozen)
	if err != nil {
		return frozenSet{}, err
	}
	if err := json.Unmarshal(b, &fz); err != nil {
		return frozenSet{}, err
	}
	type famRow struct {
		Ruling     string  `json:"ruling"`
		Family     string  `json:"family"`
		Confidence float64 `json:"confidence"`
		Covered    bool    `json:"covered"`
		Error      string  `json:"error"`
	}
	fams, err := readJSONL[famRow](familyCheck)
	if err != nil {
		return frozenSet{}, err
	}
	fam := map[string]famRow{}
	for _, f := range fams {
		fam[f.Ruling] = f
	}
	out := frozenSet{withCountry: map[string]int{}}
	in := map[string]bool{}
	for _, id := range fz.Final {
		in[id] = true
	}
	for _, r := range fz.Rulings {
		if !in[r.Ruling] {
			continue
		}
		out.withCountry[r.Ruling] = r.Mats
		f, ok := fam[r.Ruling]
		row := FrozenRow{Ruling: r.Ruling, CBP: r.CBP, Target: r.Target, Family: f.Family, Confidence: f.Confidence}
		switch {
		case !ok || f.Error != "":
			row.Outcome = "family check failed"
		case !f.Covered:
			row.Outcome = "no plan template"
		default:
			row.Outcome = "fixture built"
		}
		out.rows = append(out.rows, row)
	}
	if len(out.rows) != len(fz.Final) {
		return frozenSet{}, fmt.Errorf("frozen.json: %d rulings listed, %d described", len(fz.Final), len(out.rows))
	}
	return out, nil
}

// pool102 is the pooled D570 summary.
func pool102(sum Summary102) *Pooled102 {
	p := &Pooled102{Frozen: len(sum.Frozen)}
	tmpl := 0
	for _, f := range sum.Frozen {
		if f.Outcome == "fixture built" {
			tmpl++
		}
	}
	planned, agree, lean, leanAgree, shownShift, curDecided, curAgree, curEngine, ran, plannedClean := 0, 0, 0, 0, 0, 0, 0, 0, 0, 0
	for _, s := range sum.Searches {
		if s.Failed {
			continue
		}
		ran++
		planned++
		if !s.LeakedInput {
			plannedClean++
		}
		if s.Moves > 0 {
			p.SearchesWithMoves++
		}
		p.Moves += s.Moves
		p.Decided += s.Decided
		agree += s.EngineAgree
		lean += s.Lean
		leanAgree += s.LeanEngineAgree
		p.Shown += s.Shown
		for _, st := range s.ShownStatus {
			if st == planrule.Shifts {
				shownShift++
			}
		}
		p.Flagged += len(s.Flagged)
		if decided(s.Baseline.Status) {
			curDecided++
			if s.Baseline.Agrees {
				curAgree++
			}
		}
		if s.EngineCurrent != nil && *s.EngineCurrent {
			curEngine++
		}
	}
	p.Template = rate(tmpl, p.Frozen)
	p.Planned = rate(planned, p.Frozen)
	p.EngineAgreesDecided = rate(agree, p.Decided)
	p.EngineAgreesLean = rate(leanAgree, lean)
	p.ShownShift = rate(shownShift, p.Shown)
	p.CurrentRuleDecided = rate(curDecided, ran)
	p.CurrentRuleAgrees = rate(curAgree, curDecided)
	p.CurrentEngineAgrees = rate(curEngine, ran)
	p.WithoutLeakedPlanned = plannedClean
	flags, seen := 0, false
	for _, s := range sum.Searches {
		if s.ReviewFlags != nil {
			flags += *s.ReviewFlags
			seen = true
		}
	}
	if seen {
		p.ReviewFlags = &flags
	}
	return p
}
