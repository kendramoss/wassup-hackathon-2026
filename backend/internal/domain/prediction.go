package domain

import "fmt"

// Disclaimer is shown with every prediction and plan.
const Disclaimer = "Not a ruling. Confirm with a licensed customs broker or a CBP binding ruling."

// Trace records which model and prompt produced an output, for every API
// response and eval row.
type Trace struct {
	Provider      string `json:"llm_provider"`
	ModelID       string `json:"model_id"`
	PromptVersion string `json:"prompt_version"`
	InputTokens   int    `json:"input_tokens"`
	OutputTokens  int    `json:"output_tokens"`
	LatencyMS     int64  `json:"latency_ms"`
	// Cached is true when the prediction was replayed from the engine cache
	// (D54): no model call, no tokens.
	Cached bool `json:"cached,omitempty"`
}

// Prediction is the origin the engine expects CBP to assign.
type Prediction struct {
	Origin       string   `json:"origin_country"`
	KeyFactors   []string `json:"key_factors"`
	DecisiveStep string   `json:"decisive_step"`
	Confidence   float64  `json:"confidence"`
	Reasoning    string   `json:"reasoning"`
	// CitedRulings are the precedents the model named in its reasoning, in
	// retrieval order; Precedents are all rulings shown to it.
	CitedRulings []string `json:"cited_rulings"`
	Precedents   []string `json:"precedents"`
	// VoteAgreement, when the engine voted, is how many calls gave this
	// origin, e.g. "4/5".
	VoteAgreement string `json:"vote_agreement,omitempty"`
	Trace         Trace  `json:"trace"`
}

// Validate checks the model returned an origin and a confidence in [0, 1].
func (p Prediction) Validate() error {
	var errs problems
	if p.Origin == "" {
		errs.add("origin_country is empty")
	}
	if p.Confidence < 0 || p.Confidence > 1 {
		errs.add("confidence %v is outside [0, 1]", p.Confidence)
	}
	for _, id := range p.CitedRulings {
		if !ValidRulingID(id) {
			errs.add("cited ruling %q is not a CBP ruling number", id)
		}
	}
	return errs.err()
}

// Route is the triage outcome for a prediction.
type Route string

const (
	RouteAuto        Route = "auto"
	RouteNeedsReview Route = "needs_review"
	// RouteNeedsInfo: no earlier ruling is on point and the requester can
	// answer questions that would settle it.
	RouteNeedsInfo Route = "needs_info"
)

// Triage is the routing decision plus why.
type Triage struct {
	Route  Route  `json:"route"`
	Reason string `json:"reason"`
}

// Validate checks the route is known and a reason is given.
func (t Triage) Validate() error {
	var errs problems
	if t.Route != RouteAuto && t.Route != RouteNeedsReview && t.Route != RouteNeedsInfo {
		errs.add("unknown route %q", t.Route)
	}
	if t.Reason == "" {
		errs.add("triage reason is required")
	}
	return errs.err()
}

// ConfidencePct formats a confidence for display, e.g. "95%".
func ConfidencePct(c float64) string { return fmt.Sprintf("%.0f%%", c*100) }
