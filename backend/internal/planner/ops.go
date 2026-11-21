package planner

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/planner/opcost"
)

// This file is the planner's adapter to the opcost calculator (#281). Every
// function here is reached only when Economics.Operations is active; the
// legacy economics() and knownAddedCost paths never call it.

// Objective states what an operations-mode ranking means (contract 8.2).
type Objective struct {
	Mode      string `json:"mode"`
	Scope     string `json:"scope"`
	Statement string `json:"statement"`
}

const (
	objectiveMode  = "operations_v1"
	objectiveScope = "searched_moves_only"
	// objectiveStatement is bounded on purpose: it never says optimal, cheapest
	// or total cost.
	objectiveStatement = "Ranked by modelled yearly duty saved minus modelled yearly operating cost difference minus one-time costs amortized over horizon_years, for the moves searched and the inputs you entered. Not a global optimum: terms listed in not_modelled are in neither side."
)

func newObjective() *Objective {
	return &Objective{Mode: objectiveMode, Scope: objectiveScope, Statement: objectiveStatement}
}

// CanonicalOps is the operations block as the calculator reads it: a deep copy
// (the caller's block is never changed) whose plant-ref countries are
// canonical, because opcost compares countries exactly and holds no alias map.
func CanonicalOps(o *opcost.Operations) *opcost.Operations {
	c := opcost.Normalize(o)
	if c == nil {
		return nil
	}
	for gid, g := range c.Groups {
		for pid, p := range g.Plants {
			if p.Ref != nil {
				p.Ref.Country = domain.NormalizeCountry(p.Ref.Country)
				g.Plants[pid] = p
			}
		}
		c.Groups[gid] = g
	}
	return c
}

// ProductPlants are the product's plants by ID, with canonical countries.
func ProductPlants(p domain.Product) map[string]opcost.Ref {
	out := make(map[string]opcost.Ref, len(p.Plants))
	for _, pl := range p.Plants {
		out[pl.ID] = PlantRef(pl)
	}
	return out
}

// PlantRef is the identity an operations entry binds to: the trimmed plant name,
// else the plant ID when the product leaves the plant unnamed (the UI labels an
// unnamed plant by its ID, and the calculator needs a non-empty name), with the
// canonical country. It is the one helper behind validation and evaluation, for
// source (current product) and destination (variant) alike. An unnamed plant is
// identified by its ID and country only, so a replacement under the same ID and
// country is the same plant here (the #372 limit; not a stable identity).
func PlantRef(pl domain.Plant) opcost.Ref {
	name := strings.TrimSpace(pl.Name)
	if name == "" {
		name = pl.ID
	}
	return opcost.Ref{Name: name, Country: domain.NormalizeCountry(pl.Country)}
}

// groupPlants is every plant a group is performed at, by its operations and
// its components.
func groupPlants(p domain.Product, group string) map[string]bool {
	ids := map[string]bool{}
	for _, o := range p.Operations {
		if o.Group == group {
			ids[o.Plant] = true
		}
	}
	for _, c := range p.Components {
		if c.Group == group && c.Plant != "" {
			ids[c.Plant] = true
		}
	}
	return ids
}

// CurrentPlant is the one plant a group is performed at now. A group spread
// over several plants, or at a plant the product does not list, has an
// ambiguous baseline: "" (opcost then makes the move's costs unknown rather
// than pick a source plant).
func CurrentPlant(p domain.Product, group string) string {
	ids := groupPlants(p, group)
	if len(ids) != 1 {
		return ""
	}
	for id := range ids {
		if _, ok := p.PlantByID(id); ok {
			return id
		}
	}
	return ""
}

// ValidateEnv describes a product to opcost.Validate. strict also makes group
// and plant IDs and refs checked against the product (a write); without it only
// the current plants are known, which decides which one-time costs are sunk.
func ValidateEnv(p *domain.Product, strict bool, e Economics) opcost.ValidateEnv {
	env := opcost.ValidateEnv{GroupCosts: e.GroupCosts}
	if p == nil {
		return env
	}
	env.HasProduct = strict
	env.Groups = map[string]bool{}
	env.CurrentPlant = map[string]string{}
	env.Plants = ProductPlants(*p)
	add := func(g string) {
		if g != "" && !env.Groups[g] {
			env.Groups[g] = true
			env.CurrentPlant[g] = CurrentPlant(*p, g)
		}
	}
	for _, c := range p.Components {
		add(c.Group)
	}
	for _, o := range p.Operations {
		add(o.Group)
	}
	return env
}

// plantUnion is the plants a comparison can refer to: the current product's
// (the source of a move) plus those only the variant has (a destination the
// user adds in a scenario). An id both products have with a different name or
// country cannot be represented on both sides, so it is dropped: entries for it
// read as unknown, never as the other plant's. No id is inherited across.
func plantUnion(cur, variant map[string]opcost.Ref) map[string]opcost.Ref {
	out := make(map[string]opcost.Ref, len(cur)+len(variant))
	for id, r := range cur {
		out[id] = r
	}
	for id, r := range variant {
		if c, ok := cur[id]; ok {
			if c != r {
				delete(out, id)
			}
			continue
		}
		out[id] = r
	}
	return out
}

// opsEvaluate prices and checks the moves of one plan or comparison.
func opsEvaluate(e Economics, p domain.Product, variant *domain.Product, m domain.Move, fromOrigin, toOrigin string) opcost.Result {
	plants := ProductPlants(p)
	if variant != nil {
		plants = plantUnion(plants, ProductPlants(*variant))
	}
	in := opcost.Input{AnnualImportValueUSD: e.AnnualImportValueUSD, GroupCosts: e.GroupCosts,
		Plants: plants, Ops: CanonicalOps(e.Operations)}
	if r, ok := e.DutyRate(fromOrigin); ok {
		in.FromRate = &r
	}
	if r, ok := e.DutyRate(toOrigin); ok {
		in.ToRate = &r
	}
	for _, gm := range m {
		in.Moves = append(in.Moves, opcost.Move{Group: gm.Group, FromPlant: CurrentPlant(p, gm.Group), ToPlant: gm.ToPlant})
	}
	res, _ := opcost.Evaluate(in)
	return res
}

// applyMoney fills a checked plan's money and, in operations mode, runs the
// cost demotion. It is the one place the search, the followed-advice move and
// the SSE event all get their numbers from, so the stream never shows a plan
// as shown that the final result rejects.
func applyMoney(req Request, tm Template, pl *domain.Plan, m domain.Move, baseOrigin string) {
	if req.Economics.Operations.Active() {
		res := opsEvaluate(req.Economics, req.Product, nil, m, baseOrigin, pl.Prediction.Origin)
		pl.Operations = &res
		pl.DutyEffectYear, pl.AddedCostYear, pl.NetYear = res.DutyEffectYear, res.AddedCostYear, res.NetYear
		priced := res.Priced
		pl.NetPriced = &priced
		demote(pl, tm)
		return
	}
	var priced bool
	pl.DutyEffectYear, pl.AddedCostYear, priced = economics(req, m, baseOrigin, pl.Prediction.Origin)
	pl.NetPriced = &priced
	pl.NetYear = pl.DutyEffectYear - pl.AddedCostYear
}

// demote moves a plan that passed the legal and precedent gate out of the
// shown list when its operations result says it is known infeasible or fully
// priced and net-costlier. Cost never promotes: a plan the gate rejected keeps
// its legal reason and kind, and an unknown never demotes.
func demote(pl *domain.Plan, tm Template) {
	if pl.Operations == nil || !pl.Shown {
		return
	}
	switch pl.Operations.Decision {
	case opcost.DecisionInfeasible:
		pl.Shown, pl.RejectionKind = false, domain.RejectOperationallyInfeasible
		pl.RejectionReason = infeasibleReason(pl.Operations, tm)
	case opcost.DecisionCostlier:
		pl.Shown, pl.RejectionKind = false, domain.RejectNetCostlier
		pl.RejectionReason = costlierReason(pl.Operations)
	}
}

func groupName(tm Template, id string) string {
	if g, ok := tm.Group(id); ok && g.Name != "" {
		return g.Name
	}
	return id
}

func infeasibleReason(r *opcost.Result, tm Template) string {
	var parts []string
	for _, ch := range r.Checks {
		if ch.Status != opcost.Infeasible {
			continue
		}
		switch {
		case ch.Kind == "lead_time" && ch.Value != nil && ch.Limit != nil:
			parts = append(parts, fmt.Sprintf("lead time %s days exceeds your %s-day limit", num(*ch.Value), num(*ch.Limit)))
		case ch.Kind == "moq" && ch.Value != nil && ch.Limit != nil:
			parts = append(parts, fmt.Sprintf("%s: order size %s %s is below the %s %s minimum order", groupName(tm, ch.Group), num(*ch.Value), ch.Unit, num(*ch.Limit), ch.Unit))
		default:
			parts = append(parts, ch.Kind+" check failed")
		}
	}
	return "not operationally feasible on the inputs you entered: " + strings.Join(parts, "; ")
}

func costlierReason(r *opcost.Result) string {
	c := r.Components
	return fmt.Sprintf("modelled net is negative: duty effect %s a year, operating cost difference %s, one-time costs %s a year, net %s a year (modelled terms only; see not_modelled)",
		usd(c.DutyEffectPerYear), usd(c.OperatingDelta), usd(c.AmortizedOneTimePerYear), usd(r.NetModelledPerYear))
}

// usd formats a nullable amount; nil is "not modelled", never $0.
func usd(p *float64) string {
	if p == nil {
		return "not modelled"
	}
	v := *p
	s := strconv.FormatFloat(math.Abs(v), 'f', 2, 64)
	i := strings.IndexByte(s, '.')
	for j := i - 3; j > 0; j -= 3 {
		s = s[:j] + "," + s[j:]
	}
	if v < 0 {
		return "-$" + s
	}
	return "$" + s
}

// num prints a check value without trailing zeros.
func num(f float64) string { return strconv.FormatFloat(f, 'f', -1, 64) }

// orderOps is the operations-mode ranking (contract 7): shown plans sort by
// tier (D341), then fully priced before not priced, then modelled net, best
// first; rejected
// plans keep the current setup first, then the cost demotions, then the
// existing kinds in their existing order.
func orderOps(res *Result) {
	pricedOf := func(p domain.Plan) bool { return p.NetPriced != nil && *p.NetPriced }
	sort.SliceStable(res.Plans, func(i, j int) bool {
		a, b := res.Plans[i], res.Plans[j]
		if a.Tier != b.Tier {
			return a.Tier == domain.TierClearMargin
		}
		if pa, pb := pricedOf(a), pricedOf(b); pa != pb {
			return pa
		}
		return a.NetYear > b.NetYear
	})
	rank := map[domain.RejectionKind]int{domain.RejectCurrentSetup: -1, domain.RejectOperationallyInfeasible: 0, domain.RejectNetCostlier: 1,
		domain.RejectBelowConfidence: 2, domain.RejectNoPrecedent: 3, domain.RejectCheckFailed: 4, domain.RejectOtherOrigin: 5}
	sort.SliceStable(res.Rejected, func(i, j int) bool {
		a, b := res.Rejected[i], res.Rejected[j]
		if rank[a.RejectionKind] != rank[b.RejectionKind] {
			return rank[a.RejectionKind] < rank[b.RejectionKind]
		}
		switch a.RejectionKind {
		case domain.RejectOperationallyInfeasible, domain.RejectNetCostlier, domain.RejectBelowConfidence:
			return a.NetYear > b.NetYear
		}
		return false
	})
}
