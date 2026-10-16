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
}

// Load decodes the embedded summary.
func Load() (Summary, error) {
	var s Summary
	if err := json.Unmarshal(raw, &s); err != nil {
		return Summary{}, fmt.Errorf("evalsummary: %w", err)
	}
	return s, nil
}
