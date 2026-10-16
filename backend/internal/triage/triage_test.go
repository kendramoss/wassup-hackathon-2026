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
