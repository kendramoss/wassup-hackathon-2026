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

// DecideCalibrated routes a prediction that carries a calibrated probability
// (D160): auto only when it is at least bar and the case is in scope. The
// reason gives the calibrated value, the bar and the stated confidence.
// Without a calibrated value it falls back to Decide on stated confidence
// with statedBar.
func DecideCalibrated(p domain.Prediction, bar, statedBar float64, inScope bool, scopeReason string) domain.Triage {
	c := p.CalibratedConfidence
	if c == nil {
		return Decide(p, statedBar, inScope, scopeReason)
	}
	switch {
	case !inScope:
		return domain.Triage{Route: domain.RouteNeedsReview, Reason: "out of scope: " + scopeReason}
	case *c >= bar:
		return domain.Triage{Route: domain.RouteAuto, Reason: fmt.Sprintf("calibrated confidence %.2f ≥ %.2f (%s; model stated %.2f) and in scope", *c, bar, p.Calibration, p.Confidence)}
	default:
		return domain.Triage{Route: domain.RouteNeedsReview, Reason: fmt.Sprintf("calibrated confidence %.2f below %.2f (%s; model stated %.2f)", *c, bar, p.Calibration, p.Confidence)}
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

// Fallback routes a prediction the fallback model made, because the primary
// engine was unavailable (D56). fastPath 0 always reviews, even when
// confidence would clear the ordinary bar. A positive fastPath auto-accepts
// only at or above that bar, and only in scope.
func Fallback(p domain.Prediction, fastPath float64, inScope bool, scopeReason string) domain.Triage {
	if !inScope {
		return domain.Triage{Route: domain.RouteNeedsReview, Reason: "out of scope: " + scopeReason}
	}
	if fastPath > 0 && p.Confidence >= fastPath {
		return domain.Triage{Route: domain.RouteAuto, Reason: fmt.Sprintf("answered by the fallback model (%s); confidence %.2f ≥ %.2f", p.Trace.Identity(), p.Confidence, fastPath)}
	}
	return domain.Triage{Route: domain.RouteNeedsReview, Reason: FallbackReason(p.Trace.Identity())}
}

// FallbackReason is the review reason when the fallback model answered and
// FALLBACK_FAST_PATH does not auto-accept.
func FallbackReason(modelID string) string {
	return fmt.Sprintf("answered by the fallback model (%s) because the primary engine was unavailable", modelID)
}

// AnsweredByFallback reports whether the trace's configured model is other
// than the primary engine. It compares the requested identity, never the
// provider's resolved version, so the primary alias answered as
// "<alias>-001" is still the primary (#344). A trace recorded before
// requested_model existed falls back to model_id. An empty trace is not a
// fallback answer.
func AnsweredByFallback(t domain.Trace, primary string) bool {
	id := t.Identity()
	return id != "" && id != primary
}

// AllGates composes independent mandatory-review gates (#300). The case is
// auto only when every gate is auto; no gate overrides another. The reason
// joins, in order, the reasons of the gates that sent it to review (all
// gates' reasons when every gate passed), so simultaneous reasons stay
// visible. With no gates it reviews. A single gate is returned unchanged.
func AllGates(gates ...domain.Triage) domain.Triage {
	switch len(gates) {
	case 0:
		return domain.Triage{Route: domain.RouteNeedsReview, Reason: "no triage gate applied"}
	case 1:
		return gates[0]
	}
	var review, all []string
	for _, g := range gates {
		all = append(all, g.Reason)
		if g.Route != domain.RouteAuto {
			review = append(review, g.Reason)
		}
	}
	if len(review) == 0 {
		return domain.Triage{Route: domain.RouteAuto, Reason: strings.Join(all, "; ")}
	}
	return domain.Triage{Route: domain.RouteNeedsReview, Reason: strings.Join(review, "; ")}
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
