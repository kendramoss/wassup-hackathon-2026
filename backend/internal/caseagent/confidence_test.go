package caseagent

import "testing"

// Issue #402 / D161: the what_if tool text must not quote an unexplained
// stated number beside a calibrated card in Ask.
func TestWhatIfConfidencePhrase(t *testing.T) {
	cal := 0.6
	cases := []struct {
		name string
		w    WhatIf
		want string
	}{
		{"calibrated", WhatIf{Confidence: 0.85, CalibratedConfidence: &cal}, "calibrated 0.60; model stated 0.85"},
		{"stated only", WhatIf{Confidence: 0.85}, "confidence 0.85 as stated by the model, not calibrated"},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.w.confidencePhrase(); got != tt.want {
				t.Errorf("confidencePhrase() = %q, want %q", got, tt.want)
			}
		})
	}
}
