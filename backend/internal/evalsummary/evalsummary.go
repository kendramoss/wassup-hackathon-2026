// Package evalsummary serves the evaluation numbers shown on the results
// page. summary.json is generated from eval/results by `eval summary`
// (make eval-summary) and embedded, so the API needs no eval files at runtime.
package evalsummary

import (
	_ "embed"
	"encoding/json"
	"fmt"
)

//go:embed summary.json
var raw []byte

// Item is one reported number with its provenance.
type Item struct {
	Key             string  `json:"key"`
	Label           string  `json:"label"`
	Value           float64 `json:"value"`
	CILow           float64 `json:"ci_low,omitempty"`
	CIHigh          float64 `json:"ci_high,omitempty"`
	N               int     `json:"n,omitempty"`
	Split           string  `json:"split,omitempty"`
	DevelopmentOnly bool    `json:"development_only"`
	Source          string  `json:"source"`
}

// Summary is the results-page payload.
type Summary struct {
	GeneratedAt string `json:"generated_at"`
	Status      string `json:"status"`
	Items       []Item `json:"items"`
	// VerifierCalibration is how often the precedent check was right at each
	// confidence, from the scored flip pairs (D86).
	VerifierCalibration *VerifierCalibration `json:"verifier_calibration,omitempty"`
}

// VerifierCalibration is the precedent check's calibration, by split.
type VerifierCalibration struct {
	// Bar is the plan bar (PLAN_CONFIDENCE): a plan is shown at or above it.
	Bar float64 `json:"bar"`
	// Prompt is the verifier prompt scored (verifier-v2, D74).
	Prompt string `json:"prompt"`
	// Note says how the runs were made (LiteLLM proxy, same model as Vertex).
	Note   string          `json:"note"`
	Splits []VerifierSplit `json:"splits"`
}

// VerifierSplit is one set of flip pairs: the development pairs, or the 2026
// pairs held out from development.
type VerifierSplit struct {
	Split           string `json:"split"`
	Label           string `json:"label"`
	DevelopmentOnly bool   `json:"development_only"`
	// Caveat qualifies the split's numbers (the held-out contamination, D74).
	Caveat string        `json:"caveat,omitempty"`
	Runs   []VerifierRun `json:"runs"`
	// Pairs is the number of distinct flip pairs; Rows the scored verdicts
	// pooled over every run (a pair counts once per run); Errors the rows
	// with no verdict (provider failures), left out of every count.
	Pairs  int `json:"pairs"`
	Rows   int `json:"rows"`
	Errors int `json:"errors"`
	// Bands are precedent-supported verdicts by confidence, pooled over runs.
	Bands []VerifierBand `json:"bands"`
	// Shown is the plan bar, pooled over runs: verdicts at or above it.
	Shown VerifierShown `json:"shown"`
	// PairLevel counts each pair once, so repeated runs do not inflate n.
	PairLevel VerifierPairs `json:"pair_level"`
}

// VerifierRun is one scored result file.
type VerifierRun struct {
	Source string `json:"source"`
	// Mode is known-a (CBP's origin for the current setup given) or unknown-a.
	Mode string `json:"mode"`
	// Variant names the run: base, run2, seed-run1, seed-vote3-run1, panel-run1…
	Variant string `json:"variant"`
	Rows    int    `json:"rows"`
	Errors  int    `json:"errors"`
}

// Rate is K of N with a Wilson 95% interval.
type Rate struct {
	K      int     `json:"k"`
	N      int     `json:"n"`
	Value  float64 `json:"value"`
	CILow  float64 `json:"ci_low"`
	CIHigh float64 `json:"ci_high"`
}

// VerifierBand is the precedent-supported verdicts with confidence in
// [Min, Max) (the top band includes 1.0). Correct is how many of them were
// real origin changes.
type VerifierBand struct {
	Label   string  `json:"label"`
	Min     float64 `json:"min"`
	Max     float64 `json:"max"`
	Correct Rate    `json:"correct"`
}

// VerifierShown is what the plan bar would show, pooled over runs: a verdict
// is shown when it is precedent-supported, says origin changes, and is at or
// above the bar.
type VerifierShown struct {
	Real  int `json:"real"`
	Wrong int `json:"wrong"`
	// Precision is real of shown.
	Precision Rate `json:"precision"`
	// Recall is real changes shown of the real-change verdicts scored.
	Recall Rate `json:"recall"`
}

// VerifierPairs counts pairs, not runs. A real-change pair is shown-correct
// only if it was shown in every run that scored it; a no-change pair is
// shown-wrong if any run showed it.
type VerifierPairs struct {
	Pairs           int `json:"pairs"`
	RealChangePairs int `json:"real_change_pairs"`
	ShownEveryRun   int `json:"shown_every_run"`
	ShownSomeRuns   int `json:"shown_some_runs"`
	WrongAnyRun     int `json:"wrong_any_run"`
	// Precision is ShownEveryRun of ShownEveryRun + WrongAnyRun.
	Precision Rate `json:"precision"`
	// Recall is ShownEveryRun of RealChangePairs.
	Recall Rate `json:"recall"`
}

// Load decodes the embedded summary.
func Load() (Summary, error) {
	var s Summary
	if err := json.Unmarshal(raw, &s); err != nil {
		return Summary{}, fmt.Errorf("evalsummary: %w", err)
	}
	return s, nil
}
