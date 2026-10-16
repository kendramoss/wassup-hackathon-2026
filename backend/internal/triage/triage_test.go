package triage

import (
	"strings"
	"testing"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
)

func TestDecide(t *testing.T) {
	tests := []struct {
		name    string
		conf    float64
		inScope bool
		want    domain.Route
		reason  string
	}{
		{"confident and in scope", 0.97, true, domain.RouteAuto, "≥ 0.95"},
		{"exactly at threshold", 0.95, true, domain.RouteAuto, "≥ 0.95"},
		{"below threshold", 0.94, true, domain.RouteNeedsReview, "below 0.95"},
		{"confident but out of scope", 0.99, false, domain.RouteNeedsReview, "out of scope: textile"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Decide(domain.Prediction{Origin: "China", Confidence: tt.conf}, 0.95, tt.inScope, "textile rules")
			if got.Route != tt.want || !strings.Contains(got.Reason, tt.reason) {
				t.Errorf("Decide = %+v", got)
			}
			if err := got.Validate(); err != nil {
				t.Error(err)
			}
		})
	}
}

func TestWithClarify(t *testing.T) {
	review := domain.Triage{Route: domain.RouteNeedsReview, Reason: "confidence 0.80 below 0.95"}
	if got := WithClarify(review, true, 2); got.Route != domain.RouteNeedsInfo || got.Validate() != nil {
		t.Errorf("no precedent + questions = %+v", got)
	}
	if got := WithClarify(review, true, 0); got.Route != domain.RouteNeedsReview {
		t.Errorf("no questions should stay needs_review: %+v", got)
	}
	if got := WithClarify(review, false, 3); got.Route != domain.RouteNeedsReview {
		t.Errorf("on point should stay needs_review: %+v", got)
	}
	oos := domain.Triage{Route: domain.RouteNeedsReview, Reason: "out of scope: textile"}
	if got := WithClarify(oos, true, 2); got.Route != domain.RouteNeedsReview {
		t.Errorf("out of scope must stay: %+v", got)
	}
	auto := domain.Triage{Route: domain.RouteAuto, Reason: "x"}
	if WithClarify(auto, true, 2) != auto {
		t.Error("auto must not change")
	}
}

func TestDecideByFamily(t *testing.T) {
	bars := FamilyBars{Bars: map[string]float64{"electronics": 0.78, "other": 0.96}}
	tests := []struct {
		name      string
		subfamily string
		conf      float64
		inScope   bool
		want      domain.Route
		reason    string
	}{
		{"family bar below global accepts", "electronics", 0.80, true, domain.RouteAuto, "confidence 0.80 ≥ 0.78 (electronics family bar) and in scope"},
		{"family bar above global reviews", "other", 0.94, true, domain.RouteNeedsReview, "confidence 0.94 below 0.96 (other family bar)"},
		{"no fitted bar uses global", "medical", 0.92, true, domain.RouteAuto, "≥ 0.92 (global bar; no medical family bar)"},
		{"no router label uses global", "", 0.90, true, domain.RouteNeedsReview, "below 0.92 (global bar; no router subfamily)"},
		{"out of scope wins", "electronics", 0.99, false, domain.RouteNeedsReview, "out of scope: textile"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DecideByFamily(domain.Prediction{Origin: "China", Confidence: tt.conf}, bars, tt.subfamily, 0.92, tt.inScope, "textile")
			if got.Route != tt.want || !strings.Contains(got.Reason, tt.reason) {
				t.Errorf("DecideByFamily = %+v", got)
			}
			if err := got.Validate(); err != nil {
				t.Error(err)
			}
		})
	}
	// The global rule's reason is unchanged when the flag is off.
	if got := Decide(domain.Prediction{Confidence: 0.93}, 0.92, true, ""); got.Reason != "confidence 0.93 ≥ 0.92 and in scope" {
		t.Errorf("Decide reason changed: %q", got.Reason)
	}
}

func TestEmbeddedFamilyBars(t *testing.T) {
	b, err := LoadFamilyBars()
	if err != nil {
		t.Fatal(err)
	}
	if b.Rule == "" || b.Source == "" {
		t.Errorf("embedded bars lack provenance: %+v", b)
	}
	for fam, v := range b.Bars {
		if v <= 0 || v > 1 {
			t.Errorf("%s bar %v", fam, v)
		}
	}
	if bar, fitted := b.Bar("no-such-family", 0.92); bar != 0.92 || fitted {
		t.Errorf("unknown family = %v %v", bar, fitted)
	}
}
