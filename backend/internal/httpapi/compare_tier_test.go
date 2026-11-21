package httpapi

import (
	"sort"
	"strings"
	"testing"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/planner"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/store"
)

// #532, D371: Compare ranks targets by their best plan's tier first (clear
// margin before at the bar), then by net, as plans rank inside a target.
func TestRankTargetsByBestPlanTierThenNet(t *testing.T) {
	const clearFrom = 0.93 // bar 0.92 + DefaultClearMargin
	plan := func(label string, conf, net float64) domain.Plan {
		pl := domain.Plan{Description: label, NetYear: net, Verdict: &domain.Verdict{Confidence: conf, SupportingRuling: "N100001"}}
		pl.Tier = planner.TierOf(pl, clearFrom)
		return pl
	}
	ordered := func(ps ...domain.Plan) []domain.Plan { // planner.order: tier, then net
		sort.SliceStable(ps, func(i, j int) bool {
			if ps[i].Tier != ps[j].Tier {
				return ps[i].Tier == domain.TierClearMargin
			}
			return ps[i].NetYear > ps[j].NetYear
		})
		return ps
	}
	stats := planner.Stats{MovesSearched: 3}
	thailand := summarise("Thailand", planner.Result{Plans: ordered(plan("A big", 0.92, 200000), plan("B small", 0.96, 100000)), Stats: stats})
	vietnam := summarise("Vietnam", planner.Result{Plans: ordered(plan("B only", 0.92, 150000)), Stats: stats})
	malaysia := summarise("Malaysia", planner.Result{Plans: ordered(plan("C", 0.95, 90000)), Stats: stats})
	if thailand.Best.Description != "B small" || thailand.Best.Tier != domain.TierClearMargin || vietnam.Best.Tier != domain.TierAtBar {
		t.Fatalf("best plans: %+v / %+v", thailand.Best, vietnam.Best)
	}
	ts := []store.TargetSummary{vietnam, malaysia, thailand, {Target: "Laos", Status: TargetNoPlant}}
	rankTargets(ts)
	var got []string
	for _, x := range ts {
		got = append(got, x.Target)
	}
	// Thailand and Malaysia are clear margin (by net), then Vietnam at the bar.
	if strings.Join(got, ",") != "Thailand,Malaysia,Vietnam,Laos" || ts[0].Rank != 1 || ts[2].Rank != 3 {
		t.Errorf("ranking = %v", got)
	}
}
