package domain

import "strings"

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

// MaxGroupsPerMove limits the search to single and double moves.
const MaxGroupsPerMove = 2

// Validate checks the move has 1–2 distinct groups.
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
	Trace     Trace  `json:"trace"`
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
	NetYear         float64       `json:"net_per_year"`
	Shown           bool          `json:"shown"`
	RejectionReason string        `json:"rejection_reason,omitempty"`
	RejectionKind   RejectionKind `json:"rejection_kind,omitempty"`
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
)
