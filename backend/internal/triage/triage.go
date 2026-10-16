// Package triage decides whether a prediction can be auto-accepted or needs
// expert review. The rule is fixed by docs/DECISIONS.md D7: auto only when the
// engine's confidence is at least the threshold and the case is in scope.
// It never changes the engine's answer.
package triage

import (
	"fmt"
	"strings"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
)

// Decide routes a prediction. inScope and scopeReason come from the router's
// regime label (true, "" when no label is available).
func Decide(p domain.Prediction, threshold float64, inScope bool, scopeReason string) domain.Triage {
	switch {
	case !inScope:
		return domain.Triage{Route: domain.RouteNeedsReview, Reason: "out of scope: " + scopeReason}
	case p.Confidence >= threshold:
		return domain.Triage{Route: domain.RouteAuto, Reason: fmt.Sprintf("confidence %.2f ≥ %.2f and in scope", p.Confidence, threshold)}
	default:
		return domain.Triage{Route: domain.RouteNeedsReview, Reason: fmt.Sprintf("confidence %.2f below %.2f", p.Confidence, threshold)}
	}
}

// WithClarify refines a needs_review decision with the clarify review: when
// no earlier ruling is on point and there are questions to ask, the case
// goes back to the requester first (needs_info). Auto decisions and
// out-of-scope cases are left alone.
func WithClarify(t domain.Triage, noPrecedent bool, questions int) domain.Triage {
	if t.Route != domain.RouteNeedsReview || strings.HasPrefix(t.Reason, "out of scope") {
		return t
	}
	if noPrecedent && questions > 0 {
		return domain.Triage{Route: domain.RouteNeedsInfo, Reason: t.Reason + "; no earlier ruling on point: answer the questions first"}
	}
	return t
}
