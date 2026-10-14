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
