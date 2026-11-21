package httpapi

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/calibrate"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
)

// D160: with the calibration map on, a primary engine-v1 prediction carries
// the calibrated probability and is triaged on the calibrated bar; the
// stated confidence is kept as it was.
func TestCalibratedConfidenceInPredictAndTriage(t *testing.T) {
	m := calibrate.Must()
	tests := []struct {
		name     string
		conf     float64
		model    string
		cal      any
		route    string
		contains string
	}{
		{"stated 0.97 -> 0.94, auto", 0.97, "", 0.9355, "auto", "calibrated confidence 0.94 ≥ 0.90 (cal-v2; model stated 0.97)"},
		{"stated 0.92 -> 0.90, auto at the bar", 0.92, "", 0.9, "auto", "calibrated confidence 0.90 ≥ 0.90"},
		{"stated 0.90 -> 0.78, below the bar (clarify asks)", 0.90, "", 0.7778, "needs_info", "calibrated confidence 0.78 below 0.90"},
		{"fallback model: no calibrated value", 0.99, "gemini-fallback", nil, "needs_review", "fallback model"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hs := newHarnessWith(t, "substantial_transformation", func(d *Deps) {
				d.Engine = &fakeEngine{conf: tt.conf, modelID: tt.model}
				d.Calibration, d.CalibratedFastPath = &m, m.Bar
			})
			code, out := hs.do(t, "POST", "/v1/cases/predict", heatGunBody(t))
			if code != http.StatusCreated {
				t.Fatalf("predict status %d: %v", code, out)
			}
			pred := out["prediction"].(map[string]any)
			if pred["confidence"] != tt.conf {
				t.Errorf("stated confidence = %v, want %v", pred["confidence"], tt.conf)
			}
			if pred["calibrated_confidence"] != tt.cal {
				t.Errorf("calibrated_confidence = %v, want %v", pred["calibrated_confidence"], tt.cal)
			}
			if tt.cal != nil && pred["calibration"] != "cal-v2" {
				t.Errorf("calibration = %v", pred["calibration"])
			}
			tr := out["triage"].(map[string]any)
			if tr["route"] != tt.route || !strings.Contains(tr["reason"].(string), tt.contains) {
				t.Errorf("triage = %v, want %s containing %q", tr, tt.route, tt.contains)
			}
		})
	}
}

// With the map off (CALIBRATION=off) nothing changes from before D160.
func TestCalibrationOffKeepsStatedBar(t *testing.T) {
	var d Deps
	newHarnessWith(t, "substantial_transformation", func(dd *Deps) {
		dd.Engine = &fakeEngine{conf: 0.92}
		d = *dd
	})
	cl, err := NewCasePredictor(d).Classify(context.Background(), domain.Case{ID: "x", Facts: "heat gun"})
	if err != nil {
		t.Fatal(err)
	}
	if cl.Result.Prediction.CalibratedConfidence != nil || cl.Triage.Route != domain.RouteAuto || !strings.Contains(cl.Triage.Reason, "confidence 0.92 ≥ 0.92") {
		t.Errorf("got %+v / %+v", cl.Result.Prediction.CalibratedConfidence, cl.Triage)
	}
}
