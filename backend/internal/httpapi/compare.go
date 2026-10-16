package httpapi

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/planner"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/store"
)

// MaxTargets caps the countries compared in one call.
const MaxTargets = 4

type compareRequest struct {
	Targets   []string          `json:"targets"`
	Economics planner.Economics `json:"economics"`
}

// compareTargets runs the plan search for 2–MaxTargets target countries in
// parallel and ranks them: countries with a shown plan first (best net per
// year), then countries whose best precedent-supported plan is below the bar
// (strongest precedent check, then net), then countries with no supported
// plan or no capable plant.
func (a *api) compareTargets(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	if a.d.Planner == nil {
		writeError(w, http.StatusServiceUnavailable, "internal", "planner is not configured", nil)
		return
	}
	c, err := a.loadCase(r)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	var in compareRequest
	if err := decode(w, r, &in); err != nil {
		a.fail(w, r, err)
		return
	}
	if c.Product == nil || c.Family == "" {
		a.fail(w, r, fmt.Errorf("%w: comparing countries needs a case created from a structured product with a family", domain.ErrInvalid))
		return
	}
	targets := []string{}
	seen := map[string]bool{}
	for _, t := range in.Targets {
		n := domain.NormalizeCountry(strings.TrimSpace(t))
		if n != "" && !seen[n] {
			seen[n] = true
			targets = append(targets, n)
		}
	}
	if len(targets) < 2 || len(targets) > MaxTargets {
		a.fail(w, r, fmt.Errorf("%w: give 2 to %d different target countries", domain.ErrInvalid, MaxTargets))
		return
	}
	date, _ := time.Parse(time.DateOnly, c.Date)
	econ := in.Economics
	if econ.AnnualImportValueUSD == 0 && c.Economics != nil {
		econ = *c.Economics
	}
	econ = a.priced(econ, c.Product)
	results := make([]planner.Result, len(targets))
	errs := make([]error, len(targets))
	var wg sync.WaitGroup
	for i, t := range targets {
		wg.Add(1)
		go func(i int, t string) {
			defer wg.Done()
			results[i], errs[i] = a.d.Planner.Plan(r.Context(), planner.Request{
				CaseID: c.ID, Date: planner.Date{Time: date}, Family: c.Family, Target: t,
				Product: *c.Product, Economics: econ, Exclude: c.Exclude, CurrentOrigin: c.CurrentOrigin,
			})
		}(i, t)
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			a.fail(w, r, fmt.Errorf("target %s: %w", targets[i], err))
			return
		}
	}
	cmp := &store.TargetComparison{CheckedAt: time.Now().UTC(), Results: map[string]planner.Result{}}
	for i, t := range targets {
		cmp.Results[t] = results[i]
		cmp.Ranked = append(cmp.Ranked, summarise(t, results[i]))
	}
	rankTargets(cmp.Ranked)
	var parts []string
	for _, t := range cmp.Ranked {
		parts = append(parts, fmt.Sprintf("#%d %s %s", t.Rank, t.Target, t.Status))
	}
	var u usage
	for _, res := range results {
		a.planUsage(&u, res)
	}
	ev := u.on(store.Event{Kind: "targets_compared", Summary: strings.Join(parts, ", ")}, start)
	if !a.demoReadOnly(r, c) {
		now := time.Now().UTC()
		if _, err := a.d.Cases.Update(r.Context(), c.ID, func(latest *store.Case) error {
			latest.Targets, latest.UpdatedAt = cmp, now
			latest.Economics = economicsFrom(in.Economics, latest.Economics)
			latest.Record(ev)
			return nil
		}); err != nil {
			a.fail(w, r, err)
			return
		}
	}
	a.respond(w, http.StatusOK, map[string]any{"case_id": c.ID, "targets": cmp.Ranked})
}

// Target statuses, best first.
const (
	TargetPlan     = "plan"      // a precedent-supported plan cleared the bar
	TargetBelowBar = "below_bar" // precedent-supported, but under the bar
	TargetNoPlan   = "no_plan"   // moves searched, none precedent-supported
	TargetNoPlant  = "no_plant"  // no plant in that country can take a group
)

func summarise(target string, res planner.Result) store.TargetSummary {
	s := store.TargetSummary{Target: target, MovesSearched: res.Stats.MovesSearched, ShownCount: len(res.Plans), Status: TargetNoPlan}
	switch {
	case res.Stats.MovesSearched == 0:
		s.Status = TargetNoPlant
	case len(res.Plans) > 0:
		best := res.Plans[0]
		s.Status, s.Best = TargetPlan, &best
	default:
		for _, p := range res.Rejected { // closest calls sort first
			if p.RejectionKind == domain.RejectBelowConfidence {
				closest := p
				s.Status, s.Best = TargetBelowBar, &closest
				break
			}
		}
	}
	return s
}

func rankTargets(ts []store.TargetSummary) {
	order := map[string]int{TargetPlan: 0, TargetBelowBar: 1, TargetNoPlan: 2, TargetNoPlant: 3}
	sort.SliceStable(ts, func(i, j int) bool {
		a, b := ts[i], ts[j]
		if order[a.Status] != order[b.Status] {
			return order[a.Status] < order[b.Status]
		}
		if a.Best == nil || b.Best == nil {
			return false
		}
		// Below the bar, a stronger precedent check beats more money.
		if a.Status == TargetBelowBar {
			if ca, cb := verdictConf(*a.Best), verdictConf(*b.Best); ca != cb {
				return ca > cb
			}
		}
		return a.Best.NetYear > b.Best.NetYear
	})
	for i := range ts {
		ts[i].Rank = i + 1
	}
}
