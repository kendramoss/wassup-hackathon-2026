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

func TestUSMCA(t *testing.T) {
	for _, tt := range []struct {
		fast, conf float64
		want       domain.Route
		reason     string
	}{
		{0, 0.99, domain.RouteNeedsReview, "without a held-out result"},
		{0.9, 0.95, domain.RouteAuto, "≥ 0.90"},
		{0.9, 0.9, domain.RouteAuto, "≥ 0.90"},
		{0.9, 0.89, domain.RouteNeedsReview, "below 0.90"},
	} {
		got := USMCA(domain.Prediction{Origin: "Mexico", Confidence: tt.conf}, tt.fast)
		if got.Route != tt.want || !strings.HasPrefix(got.Reason, USMCAPrefix) || !strings.Contains(got.Reason, tt.reason) {
			t.Errorf("USMCA(conf %.2f, bar %.2f) = %+v", tt.conf, tt.fast, got)
		}
		if err := got.Validate(); err != nil {
			t.Error(err)
		}
	}
}

func TestPharma(t *testing.T) {
	tests := []struct {
		name   string
		conf   float64
		fast   float64
		want   domain.Route
		reason string
	}{
		{"no bar: always review", 0.99, 0, domain.RouteNeedsReview, "an expert reviews every pharmaceutical case"},
		{"at the bar", 0.95, 0.95, domain.RouteAuto, "≥ 0.95"},
		{"below the bar", 0.94, 0.95, domain.RouteNeedsReview, "below 0.95"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Pharma(domain.Prediction{Origin: "India", Confidence: tt.conf}, tt.fast)
			if got.Route != tt.want || !strings.HasPrefix(got.Reason, PharmaPrefix) || !strings.Contains(got.Reason, tt.reason) {
				t.Errorf("Pharma = %+v", got)
			}
			if err := got.Validate(); err != nil {
				t.Error(err)
			}
		})
	}
}

func TestFallbackAlwaysReviewsUnlessFastPathSet(t *testing.T) {
	p := domain.Prediction{Origin: "China", Confidence: 0.99, Trace: domain.Trace{ModelID: "tuned-flash"}}
	got := Fallback(p, 0, true, "")
	want := "answered by the fallback model (tuned-flash) because the primary engine was unavailable"
	if got.Route != domain.RouteNeedsReview || got.Reason != want {
		t.Errorf("default = %+v", got)
	}
	if auto := Fallback(p, 0.95, true, ""); auto.Route != domain.RouteAuto {
		t.Errorf("fast path = %+v", auto)
	}
	if below := Fallback(domain.Prediction{Origin: "China", Confidence: 0.90, Trace: p.Trace}, 0.95, true, ""); below.Route != domain.RouteNeedsReview || below.Reason != want {
		t.Errorf("below fallback bar = %+v", below)
	}
	if oos := Fallback(p, 0.95, false, "pharma"); oos.Route != domain.RouteNeedsReview || !strings.Contains(oos.Reason, "out of scope") {
		t.Errorf("out of scope = %+v", oos)
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
