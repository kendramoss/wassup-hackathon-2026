package httpapi

import (
	"net/http"
	"strings"
	"testing"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/triage"
)

// FAST_PATH_BY_FAMILY (D63): with family bars the router's subfamily
// (fakeRouter: motorised) picks the bar and the reason names it; without
// them the global bar decides as before.
func TestTriageByFamilyBar(t *testing.T) {
	bars := &triage.FamilyBars{Bars: map[string]float64{"motorised": 0.80}}
	tests := []struct {
		name   string
		bars   *triage.FamilyBars
		conf   float64
		route  string
		reason string
	}{
		{"off: global bar reviews 0.85", nil, 0.85, "needs_info", "confidence 0.85 below 0.92"},
		{"on: motorised bar accepts 0.85", bars, 0.85, "auto", "confidence 0.85 ≥ 0.80 (motorised family bar) and in scope"},
		{"on: below the motorised bar", bars, 0.75, "needs_info", "confidence 0.75 below 0.80 (motorised family bar)"},
		{"on: no motorised bar falls back", &triage.FamilyBars{Bars: map[string]float64{"other": 0.96}}, 0.93, "auto", "≥ 0.92 (global bar; no motorised family bar)"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hs := newHarnessWith(t, "substantial_transformation", func(d *Deps) { d.FamilyBars = tt.bars })
			hs.engine.conf = tt.conf
			code, out := hs.do(t, "POST", "/v1/cases/predict", heatGunBody(t))
			if code != http.StatusCreated {
				t.Fatalf("predict status %d: %v", code, out)
			}
			tr := out["triage"].(map[string]any)
			if tr["route"] != tt.route || !strings.Contains(tr["reason"].(string), tt.reason) {
				t.Errorf("triage = %v, want %s %q", tr, tt.route, tt.reason)
			}
		})
	}
	// Out of scope stays out of scope whatever the bar.
	hs := newHarnessWith(t, "usmca_part102", func(d *Deps) { d.FamilyBars = bars })
	hs.engine.conf = 0.99
	if _, out := hs.do(t, "POST", "/v1/cases/predict", heatGunBody(t)); !strings.HasPrefix(out["triage"].(map[string]any)["reason"].(string), "out of scope") {
		t.Errorf("out of scope triage = %v", out["triage"])
	}
}
