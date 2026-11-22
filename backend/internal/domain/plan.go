package domain

import (
	"strings"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/planner/opcost"
)

// OperationGroup is a set of operations (and the components they produce)
// that move between plants together, e.g. motor_fan_assembly = stamp
// laminations, wind rotor, build stator, fit fan.
type OperationGroup struct {
	ID         string   `json:"id"`
	Name       string   `json:"name"`
	Operations []string `json:"operations"`
	Components []string `json:"components,omitempty"`
}

// Validate checks the group has an ID and at least one operation.
func (g OperationGroup) Validate() error {
	var errs problems
	if g.ID == "" {
		errs.add("operation group id is required")
	}
	if len(g.Operations) == 0 {
		errs.add("operation group %q has no operations", g.ID)
	}
	return errs.err()
}

// GroupMove moves one operation group to a plant.
type GroupMove struct {
	Group   string `json:"group"`
	ToPlant string `json:"to_plant"`
}

// Move is a single or double group move: the unit the planner searches over.
type Move []GroupMove

// MaxGroupsPerMove limits a move's size: the search lists up to three
// groups (D84) and a move followed from near-miss advice adds at most one
// more (D85).
const MaxGroupsPerMove = 4

// Validate checks the move has 1 to MaxGroupsPerMove distinct groups.
func (m Move) Validate() error {
	var errs problems
	if len(m) == 0 || len(m) > MaxGroupsPerMove {
		errs.add("a move must change 1 to %d operation groups, got %d", MaxGroupsPerMove, len(m))
	}
	seen := map[string]bool{}
	for _, gm := range m {
		if gm.Group == "" || gm.ToPlant == "" {
			errs.add("each group move needs group and to_plant")
		}
		if seen[gm.Group] {
			errs.add("group %q moved twice", gm.Group)
		}
		seen[gm.Group] = true
	}
	return errs.err()
}

// Key is a stable identifier for caching and display, e.g.
// "motor_fan_assembly@th+pcba@th".
func (m Move) Key() string {
	parts := make([]string, len(m))
	for i, gm := range m {
		parts[i] = gm.Group + "@" + gm.ToPlant
	}
	return strings.Join(parts, "+")
}

// Verdict is the precedent check on a proposed move. PrecedentSupported means
// an earlier ruling held a comparable change decisive; it is not a ruling.
type Verdict struct {
	PrecedentSupported bool    `json:"precedent_supported"`
	SupportingRuling   string  `json:"supporting_ruling,omitempty"`
	CounterRuling      string  `json:"counter_ruling,omitempty"`
	Confidence         float64 `json:"confidence"`
	Reasoning          string  `json:"reasoning"`
	// Agreement, when the check ran several times, is how many runs gave this
	// answer, e.g. "3/3".
	Agreement string `json:"agreement,omitempty"`
	// Trail is the check's search path: each search with the rulings it
	// returned, and each ruling read, in the order the agent made them (D80).
	// Verdicts cached before D80 have none.
	Trail []TrailStep `json:"trail,omitempty"`
	Trace Trace       `json:"trace"`
}

// TrailStep is one tool call in a precedent check.
type TrailStep struct {
	// Tool is "search" or "read".
	Tool string `json:"tool"`
	// Query is the search text, or the ruling number asked for.
	Query string `json:"query"`
	// Rulings are the ruling numbers a search returned, or the ruling read
	// (empty when it was not found or not dated before the case).
	Rulings []string `json:"rulings,omitempty"`
}

// Validate checks confidence is in range and that a supported verdict cites
// the ruling that supports it (the verifier's burden of proof).
func (v Verdict) Validate() error {
	var errs problems
	if v.Confidence < 0 || v.Confidence > 1 {
		errs.add("confidence %v is outside [0, 1]", v.Confidence)
	}
	if v.PrecedentSupported && !ValidRulingID(v.SupportingRuling) {
		errs.add("precedent_supported requires a supporting ruling, got %q", v.SupportingRuling)
	}
	if v.CounterRuling != "" && !ValidRulingID(v.CounterRuling) {
		errs.add("counter ruling %q is not a CBP ruling number", v.CounterRuling)
	}
	return errs.err()
}

// Plan is one evaluated move: the variant's predicted origin, its precedent
// verdict and its economics. Costs are user inputs and illustrative.
type Plan struct {
	Move           Move       `json:"move"`
	Description    string     `json:"description"`
	VariantFacts   string     `json:"variant_facts"`
	Prediction     Prediction `json:"prediction"`
	Verdict        *Verdict   `json:"verdict,omitempty"`
	DutyEffectYear float64    `json:"duty_effect_per_year"`
	AddedCostYear  float64    `json:"added_cost_per_year"`
	// NetYear is duty effect minus added cost, the ranking key.
	NetYear float64 `json:"net_per_year"`
	// NetPriced says whether the search could work this plan's money out. True:
	// the figures above are real even when 0 (an annual import value, a duty
	// rate for both origins, and a cost at both plants of every moved group
	// whenever any group costs were entered). False: the search ran and could
	// not, so a 0 is unknown. Absent: the plan was stored before this field
	// and its pricing can't be proven either way.
	NetPriced *bool `json:"net_priced,omitempty"`
	// Operations is the modelled operational cost and feasibility of this move
	// (#281). Present only when the economics carried an operations block;
	// absent otherwise, so a plan without one serialises exactly as before.
	Operations      *opcost.Result `json:"operations,omitempty"`
	Shown           bool           `json:"shown"`
	RejectionReason string         `json:"rejection_reason,omitempty"`
	RejectionKind   RejectionKind  `json:"rejection_kind,omitempty"`
	// NearMiss, for the closest moves just under the plan bar, says what
	// differs from the supporting ruling and what would match it (D81).
	// Advice only: it does not change the verdict or Shown.
	NearMiss *NearMiss `json:"near_miss,omitempty"`
	// FromAdvice marks the one move per search added by following a near
	// miss's advice (D85): checked once like any other move.
	FromAdvice bool `json:"from_advice,omitempty"`
	// Tier is a shown plan's ranking tier (D340, D341): "clear_margin" when
	// its check is at least the bar plus the clear margin and names no
	// counter-ruling, "at_bar" otherwise. Shown plans sort by tier, then net.
	// Empty on rejected moves and on plans stored before D341.
	Tier PlanTier `json:"tier,omitempty"`
	// PartsMoved counts the components whose country the move changes.
	PartsMoved int `json:"parts_moved,omitempty"`
	// SmallestSupported marks the shown plan that moves the fewest parts when
	// two or more are shown (D340): a label, not a sort key.
	SmallestSupported bool `json:"smallest_supported,omitempty"`
	// RuleCheck is the 19 CFR 102.20 rule check of the product as it would
	// be after this move (D441). It is computed when the plan is read and
	// never stored; it does not change Shown, the verdict or the ranking.
	// Absent when the product does not give the codes the check needs.
	RuleCheck *PlanRuleCheck `json:"rule_check,omitempty"`
}

// PlanRuleCheck is the 102.20 rule check of one move (D441).
type PlanRuleCheck struct {
	// Applies is "part102" when the case was routed to the Part 102 rules,
	// else "informational" (decided under substantial transformation).
	Applies string `json:"applies"`
	// Country is where the good would be finished after the move.
	Country string `json:"country"`
	// Status: "shifts" (the rule gives the target), "no_shift" (it gives
	// another country because a foreign part fails the change), "other_country"
	// (it gives another country although no part fails: the good is still
	// finished there) or "undetermined".
	Status string `json:"status"`
	// Label is the status in a few words, for a chip or a list line.
	Label  string `json:"label"`
	Origin string `json:"origin,omitempty"`
	Basis  string `json:"basis,omitempty"`
	Rule   string `json:"rule,omitempty"`
	Reason string `json:"reason"`
	// Why classifies an undetermined answer (see planrule).
	Why string `json:"why,omitempty"`
	// Failing names the foreign parts that do not make the change.
	Failing []string `json:"failing,omitempty"`
	// AgreesWithPrecedent is set when the rule decided and the move has a
	// precedent check.
	AgreesWithPrecedent *bool `json:"agrees_with_precedent,omitempty"`
	// Review is true when the two disagree in a way worth a broker's look;
	// ReviewNote says how. It does not change the plan.
	Review     bool   `json:"review"`
	ReviewNote string `json:"review_note,omitempty"`
}

// PlanTier is a shown plan's ranking tier (D341).
type PlanTier string

// Ranking tiers.
const (
	TierClearMargin PlanTier = "clear_margin"
	TierAtBar       PlanTier = "at_bar"
)

// NearMiss explains a move whose precedent check is just under the bar.
type NearMiss struct {
	Difference string `json:"difference"`
	WouldMatch string `json:"would_match"`
	Ruling     string `json:"ruling"`
	Trace      Trace  `json:"trace"`
}

// RejectionKind classifies why a plan is not shown, so the UI can style it
// (e.g. a greyed "below confidence bar" row) without parsing the reason.
type RejectionKind string

const (
	// RejectCurrentSetup is the current setup, listed for comparison.
	RejectCurrentSetup RejectionKind = "current_setup"
	// RejectBelowConfidence is precedent-supported, but the verifier's
	// confidence is below the display bar (PLAN_CONFIDENCE, 0.92).
	RejectBelowConfidence RejectionKind = "below_confidence_bar"
	// RejectNoPrecedent means no earlier ruling held this kind of change decisive.
	RejectNoPrecedent RejectionKind = "no_supporting_precedent"
	// RejectOtherOrigin means the engine predicts the move does not reach the target.
	RejectOtherOrigin RejectionKind = "engine_other_origin"
	// RejectCheckFailed means the precedent check could not complete.
	RejectCheckFailed RejectionKind = "check_failed"
	// RejectOperationallyInfeasible means the move passed the legal gate but a
	// constraint the user entered (lead time, minimum order) is known to fail.
	// Only produced when the economics carry an operations block (#281).
	RejectOperationallyInfeasible RejectionKind = "operationally_infeasible"
	// RejectNetCostlier means the move passed the legal gate and is fully
	// priced, but its modelled net per year is below zero. Only produced when
	// the economics carry an operations block (#281).
	RejectNetCostlier RejectionKind = "net_costlier"
)
