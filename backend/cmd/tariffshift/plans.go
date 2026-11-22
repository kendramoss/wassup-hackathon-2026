package main

// The 19 CFR 102.20 rule check on each move of the committed plan searches,
// counted as pre-registered in D440 (no model calls):
//
//	go run ./cmd/tariffshift plans-eval [-check]   (in make reproduce)
//
// Codes: only the 23 per-family plan fixtures can be checked, with the top
// suggestion for the good and every part from the D381 development run
// (suggest-dev-v1.jsonl, set family). The held-out suggestion file is never
// read here.

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/planner"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/planrule"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/tariffshift"
)

const (
	plansDir      = evalDir + "/plans"
	plansSuggest  = partsDir + "/suggest-dev-v1.jsonl"
	plansRunsGlob = "../eval/results/planner-families-%s-litellm.json"
)

// committedPlanProducts are every committed plan fixture, for the count of
// products that give every code as committed.
var committedPlanProducts = []string{"testdata/*.json", "testdata/families/*.json", "testdata/planner_families/*.json", "testdata/planner_post/*.json"}

// planFixture is the part of a plan fixture the count reads.
type planFixture struct {
	CaseID        string               `json:"case_id"`
	Target        string               `json:"target"`
	Product       *domain.Product      `json:"product"`
	CurrentOrigin *planner.KnownOrigin `json:"current_origin"`
}

// MoveRow is one checked move (or the current setup, Kind "baseline").
type MoveRow struct {
	Fixture   string   `json:"fixture"`
	Target    string   `json:"target"`
	Kind      string   `json:"kind"` // "move" or "baseline"
	Move      string   `json:"move,omitempty"`
	Shown     bool     `json:"shown"`
	Rejection string   `json:"rejection_kind,omitempty"`
	Precedent *bool    `json:"precedent_supported,omitempty"`
	Engine    string   `json:"engine_origin,omitempty"`
	CBP       string   `json:"cbp_origin,omitempty"`
	GoodHTS   string   `json:"good_hts"`
	Country   string   `json:"country"`
	Status    string   `json:"status"`
	Why       string   `json:"why,omitempty"`
	Origin    string   `json:"origin,omitempty"`
	Basis     string   `json:"basis,omitempty"`
	Label     string   `json:"label"`
	Failing   []string `json:"failing,omitempty"`
	Review    bool     `json:"review,omitempty"`
}

// PlansSummary is summary.json.
type PlansSummary struct {
	Note string `json:"note"`
	// CommittedProducts: plan fixtures in the repo; WithAllCodes: of those,
	// the ones whose committed product gives the good's and every part's code.
	CommittedProducts int `json:"committed_plan_products"`
	WithAllCodes      int `json:"committed_with_all_codes"`
	Part102Searches   int `json:"part102_plan_searches"`
	LaterRuling       int `json:"moves_with_a_later_ruling_and_codes"`
	Searches          int `json:"searches"`
	SearchesDecided   int `json:"searches_with_a_decided_move"`
	Moves             int `json:"moves"`
	// ByStatus and ByWhy count moves; Why only undetermined ones.
	ByStatus map[string]int `json:"moves_by_status"`
	ByWhy    map[string]int `json:"undetermined_by_reason"`
	Decided  int            `json:"moves_decided"`
	// Of decided moves with a precedent check.
	WithVerdict         int `json:"decided_with_precedent_check"`
	AgreePrecedent      int `json:"agree_with_precedent"`
	PrecedentRuleOther  int `json:"precedent_supports_rule_other_country"`
	RuleShiftsNoSupport int `json:"rule_shifts_precedent_not_supported"`
	AgreeEngine         int `json:"decided_agree_with_engine"`
	// Current setup.
	BaselineDecided  int `json:"baseline_decided"`
	BaselineAgreeCBP int `json:"baseline_agree_with_cbp"`
	BaselineAgreeEng int `json:"baseline_agree_with_engine"`
}

func cmdPlansEval(args []string) error {
	fl := flag.NewFlagSet("plans-eval", flag.ExitOnError)
	check := fl.Bool("check", false, "recompute and fail if the committed files differ")
	_ = fl.Parse(args)
	rules, err := tariffshift.Default()
	if err != nil {
		return err
	}
	sum := PlansSummary{ByStatus: map[string]int{}, ByWhy: map[string]int{},
		Note: "19 CFR 102.20 rule check on each move of the committed plan searches, counted as pre-registered in D440 (results D442). Codes: the 23 per-family fixtures (D171, development and train rulings) with the top suggestion for the good and every part from the D381 development run; no committed plan product gives every code. Every search here is decided under substantial transformation, so the check is informational and agreement is not accuracy. Recompute: cd backend && go run ./cmd/tariffshift plans-eval -check"}

	for _, g := range committedPlanProducts {
		files, _ := filepath.Glob(g)
		for _, f := range files {
			var fx planFixture
			b, err := os.ReadFile(f)
			if err != nil || json.Unmarshal(b, &fx) != nil || fx.Product == nil || len(fx.Product.Components) == 0 {
				continue
			}
			sum.CommittedProducts++
			if planrule.Eligible(fx.Product, false) {
				sum.WithAllCodes++
			}
		}
	}

	sugg, err := readJSONL[SuggestRow](plansSuggest)
	if err != nil {
		return err
	}
	byRuling := map[string]SuggestRow{}
	for _, s := range sugg {
		if s.Set != "family" {
			continue
		}
		if s.Split != "dev" {
			return fmt.Errorf("%s: %s is not a development row", plansSuggest, s.Ruling)
		}
		byRuling[strings.ToLower(s.Ruling)] = s
	}

	files, err := filepath.Glob(familyFixtures + "/*.json")
	if err != nil {
		return err
	}
	sort.Strings(files)
	var rows []MoveRow
	for _, f := range files {
		id := strings.TrimSuffix(filepath.Base(f), ".json")
		var fx planFixture
		b, err := os.ReadFile(f)
		if err != nil {
			return err
		}
		if err := json.Unmarshal(b, &fx); err != nil || fx.Product == nil {
			return fmt.Errorf("%s: %v", f, err)
		}
		s, ok := byRuling[id]
		if !ok {
			return fmt.Errorf("no suggestion row for %s", id)
		}
		p, err := withTopCodes(*fx.Product, s)
		if err != nil {
			return fmt.Errorf("%s: %w", id, err)
		}
		var res planner.Result
		rb, err := os.ReadFile(fmt.Sprintf(plansRunsGlob, id))
		if err != nil {
			return err
		}
		if err := json.Unmarshal(rb, &res); err != nil {
			return err
		}
		target := domain.NormalizeCountry(res.Target)
		if target == "Canada" || target == "Mexico" {
			sum.Part102Searches++
		}
		sum.Searches++
		decidedHere := false

		base := planrule.Check(rules, p, domain.Plan{}, target, planrule.AppliesInformational)
		br := moveRow(id, target, p, domain.Plan{}, base)
		br.Kind, br.Engine = "baseline", domain.NormalizeCountry(res.Baseline.Prediction.Origin)
		if fx.CurrentOrigin != nil {
			br.CBP = domain.NormalizeCountry(fx.CurrentOrigin.Country)
		}
		if decided(base.Status) {
			sum.BaselineDecided++
			if base.Origin == br.CBP {
				sum.BaselineAgreeCBP++
			}
			if base.Origin == br.Engine {
				sum.BaselineAgreeEng++
			}
		}
		rows = append(rows, br)

		for _, pl := range append(append([]domain.Plan(nil), res.Plans...), res.Rejected...) {
			if len(pl.Move) == 0 { // the current-setup row, counted above
				continue
			}
			c := planrule.Check(rules, p, pl, target, planrule.AppliesInformational)
			r := moveRow(id, target, p, pl, c)
			rows = append(rows, r)
			sum.Moves++
			sum.ByStatus[c.Status]++
			if c.Status == planrule.Undetermined {
				sum.ByWhy[c.Why]++
				continue
			}
			sum.Decided++
			decidedHere = true
			if (domain.NormalizeCountry(pl.Prediction.Origin) == target) == (c.Status == planrule.Shifts) {
				sum.AgreeEngine++
			}
			if pl.Verdict == nil {
				continue
			}
			sum.WithVerdict++
			switch {
			case *c.AgreesWithPrecedent:
				sum.AgreePrecedent++
			case pl.Verdict.PrecedentSupported:
				sum.PrecedentRuleOther++
			default:
				sum.RuleShiftsNoSupport++
			}
		}
		if decidedHere {
			sum.SearchesDecided++
		}
	}

	sb, _ := json.MarshalIndent(sum, "", "  ")
	if !*check {
		if err := os.MkdirAll(plansDir, 0o755); err != nil {
			return err
		}
	}
	if err := writeOrCheckPlans(plansDir+"/moves-dev.jsonl", jsonlOf(rows), *check); err != nil {
		return err
	}
	if err := writeOrCheckPlans(plansDir+"/summary.json", append(sb, '\n'), *check); err != nil {
		return err
	}
	fmt.Printf("plans-eval: %d searches, %d moves, %d decided (%d with a precedent check, %d agree), %d searches with a decided move\n",
		sum.Searches, sum.Moves, sum.Decided, sum.WithVerdict, sum.AgreePrecedent, sum.SearchesDecided)
	return nil
}

func writeOrCheckPlans(path string, b []byte, check bool) error {
	if err := writeOrCheck(path, b, check); err != nil {
		return fmt.Errorf("%w (go run ./cmd/tariffshift plans-eval)", err)
	}
	return nil
}

func decided(status string) bool { return status != planrule.Undetermined }

// withTopCodes is p with the top suggested code on the good and each part.
// A part with no suggestion keeps no code.
func withTopCodes(p domain.Product, s SuggestRow) (domain.Product, error) {
	if len(s.Parts) != len(p.Components) {
		return p, fmt.Errorf("%d suggested parts for %d components", len(s.Parts), len(p.Components))
	}
	p.Components = append([]domain.Component(nil), p.Components...)
	for i, sp := range s.Parts {
		if sp.Index != i || sp.Name != p.Components[i].Name {
			return p, fmt.Errorf("part %d: suggestion %q does not match %q", i, sp.Name, p.Components[i].Name)
		}
		p.Components[i].HTS = ""
		if len(sp.Candidates) > 0 {
			p.Components[i].HTS = sp.Candidates[0].HTS
		}
	}
	if s.Good != nil && len(s.Good.Candidates) > 0 {
		p.HTS = s.Good.Candidates[0].HTS
	}
	return p, nil
}

func moveRow(id, target string, p domain.Product, pl domain.Plan, c *domain.PlanRuleCheck) MoveRow {
	r := MoveRow{Fixture: id, Target: target, Kind: "move", Move: pl.Move.Key(), Shown: pl.Shown, Rejection: string(pl.RejectionKind),
		Engine: domain.NormalizeCountry(pl.Prediction.Origin), GoodHTS: p.HTS, Country: c.Country, Status: c.Status, Why: c.Why,
		Origin: c.Origin, Basis: c.Basis, Label: c.Label, Failing: c.Failing, Review: c.Review}
	if pl.Verdict != nil {
		v := pl.Verdict.PrecedentSupported
		r.Precedent = &v
	}
	return r
}
