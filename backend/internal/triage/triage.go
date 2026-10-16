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

// TextilePrefix starts the reason of every textile-regime decision (D51).
const TextilePrefix = "textile rules (19 CFR 102.21)"

// Textile routes a prediction made under the textile rules. With fastPath 0
// (the default until the textile eval sets a threshold) it always goes to
// review: the regime is new and has no held-out result yet.
func Textile(p domain.Prediction, fastPath float64) domain.Triage {
	if fastPath > 0 && p.Confidence >= fastPath {
		return domain.Triage{Route: domain.RouteAuto, Reason: fmt.Sprintf("%s: confidence %.2f ≥ %.2f", TextilePrefix, p.Confidence, fastPath)}
	}
	if fastPath > 0 {
		return domain.Triage{Route: domain.RouteNeedsReview, Reason: fmt.Sprintf("%s: confidence %.2f below %.2f", TextilePrefix, p.Confidence, fastPath)}
	}
	return domain.Triage{Route: domain.RouteNeedsReview, Reason: TextilePrefix + ": a new regime without a held-out result yet; an expert reviews every textile case"}
}

// USMCAPrefix starts the reason of every USMCA-regime decision (D61).
const USMCAPrefix = "USMCA / Part 102 rules (19 CFR 102.11)"

// USMCA routes a prediction made under the Part 102 rules. With fastPath 0
// (the default until a held-out run sets a threshold) it always goes to
// review.
func USMCA(p domain.Prediction, fastPath float64) domain.Triage {
	if fastPath > 0 && p.Confidence >= fastPath {
		return domain.Triage{Route: domain.RouteAuto, Reason: fmt.Sprintf("%s: confidence %.2f ≥ %.2f", USMCAPrefix, p.Confidence, fastPath)}
	}
	if fastPath > 0 {
		return domain.Triage{Route: domain.RouteNeedsReview, Reason: fmt.Sprintf("%s: confidence %.2f below %.2f", USMCAPrefix, p.Confidence, fastPath)}
	}
	return domain.Triage{Route: domain.RouteNeedsReview, Reason: USMCAPrefix + ": a new regime without a held-out result yet; an expert reviews every USMCA case"}
}

// PharmaPrefix starts the reason of every pharmaceutical-regime decision (D62).
const PharmaPrefix = "pharmaceutical rules (active ingredient)"

// Pharma routes a prediction made under the pharmaceutical rules, as Textile
// does: with fastPath 0 (the default) every case goes to review.
func Pharma(p domain.Prediction, fastPath float64) domain.Triage {
	if fastPath > 0 && p.Confidence >= fastPath {
		return domain.Triage{Route: domain.RouteAuto, Reason: fmt.Sprintf("%s: confidence %.2f ≥ %.2f", PharmaPrefix, p.Confidence, fastPath)}
	}
	if fastPath > 0 {
		return domain.Triage{Route: domain.RouteNeedsReview, Reason: fmt.Sprintf("%s: confidence %.2f below %.2f", PharmaPrefix, p.Confidence, fastPath)}
	}
	return domain.Triage{Route: domain.RouteNeedsReview, Reason: PharmaPrefix + ": a new regime without a held-out result yet; an expert reviews every pharmaceutical case"}
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
