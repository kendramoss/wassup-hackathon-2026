// Package triage decides whether a prediction can be auto-accepted or needs
// expert review. The rule is fixed by docs/DECISIONS.md D7: auto only when the
// engine's confidence is at least the threshold and the case is in scope.
// It never changes the engine's answer.
package triage

import (
	"fmt"

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
