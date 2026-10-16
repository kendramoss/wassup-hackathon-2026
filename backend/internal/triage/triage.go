// Package triage decides whether a prediction can be auto-accepted or needs
// expert review. The rule is fixed by docs/DECISIONS.md D7: auto only when the
// engine's confidence is at least the threshold and the case is in scope.
// It never changes the engine's answer.
package triage

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
)

// Decide routes a prediction. inScope and scopeReason come from the router's
// regime label (true, "" when no label is available).
func Decide(p domain.Prediction, threshold float64, inScope bool, scopeReason string) domain.Triage {
	return decide(p, threshold, "", inScope, scopeReason)
}

// decide is Decide with an optional note naming the bar used.
func decide(p domain.Prediction, threshold float64, bar string, inScope bool, scopeReason string) domain.Triage {
	if bar != "" {
		bar = " (" + bar + ")"
	}
	switch {
	case !inScope:
		return domain.Triage{Route: domain.RouteNeedsReview, Reason: "out of scope: " + scopeReason}
	case p.Confidence >= threshold:
		return domain.Triage{Route: domain.RouteAuto, Reason: fmt.Sprintf("confidence %.2f ≥ %.2f%s and in scope", p.Confidence, threshold, bar)}
	default:
		return domain.Triage{Route: domain.RouteNeedsReview, Reason: fmt.Sprintf("confidence %.2f below %.2f%s", p.Confidence, threshold, bar)}
	}
}

// familyBarsJSON is written by `eval calibrate` (D63): the auto-accept bar
// per router subfamily, fitted on the dev split only.
//
//go:embed family_bars.json
var familyBarsJSON []byte

// FamilyBars are auto-accept bars per router subfamily (D63). A subfamily
// without a fitted bar (too few dev cases, or never 95% accurate) uses the
// global bar.
type FamilyBars struct {
	Rule   string             `json:"rule"`
	Source string             `json:"source"`
	Bars   map[string]float64 `json:"bars"`
}

// LoadFamilyBars decodes the embedded bars.
func LoadFamilyBars() (FamilyBars, error) {
	var b FamilyBars
	if err := json.Unmarshal(familyBarsJSON, &b); err != nil {
		return FamilyBars{}, fmt.Errorf("triage: family bars: %w", err)
	}
	for fam, v := range b.Bars {
		if v <= 0 || v > 1 {
			return FamilyBars{}, fmt.Errorf("triage: family bar %s = %v, want (0, 1]", fam, v)
		}
	}
	return b, nil
}

// Bar returns subfamily's bar, or global when it has none.
func (b FamilyBars) Bar(subfamily string, global float64) (bar float64, fitted bool) {
	if v, ok := b.Bars[subfamily]; ok {
		return v, true
	}
	return global, false
}

// DecideByFamily is Decide with the router subfamily's bar instead of the
// global one; the reason names which bar applied. subfamily is "" when the
// router gave no label.
func DecideByFamily(p domain.Prediction, bars FamilyBars, subfamily string, global float64, inScope bool, scopeReason string) domain.Triage {
	bar, fitted := bars.Bar(subfamily, global)
	note := subfamily + " family bar"
	switch {
	case subfamily == "":
		note = "global bar; no router subfamily"
	case !fitted:
		note = "global bar; no " + subfamily + " family bar"
	}
	return decide(p, bar, note, inScope, scopeReason)
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
