package httpapi

import (
	"context"
	"strings"
	"testing"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/jurisdiction"
)

func TestRCEPAuthorityMarkets(t *testing.T) {
	for _, m := range []string{jurisdiction.KR, jurisdiction.AU} {
		a := RCEPAuthority(m, "agency")
		if a.Market != m || a.Engine == nil {
			t.Fatalf("%s: %+v", m, a)
		}
	}
}

func TestRCEPEngineQualifies(t *testing.T) {
	e := RCEPAuthority(jurisdiction.KR, "Korea Customs Service").Engine
	c := domain.Case{Product: &domain.Product{HTS: "2009.90", Plants: []domain.Plant{{ID: "p1", Country: "Vietnam"}}, Components: []domain.Component{
		{Name: "oranges", HTS: "0805.10", MadeIn: "United States"},
		{Name: "limes", HTS: "0805.50", MadeIn: "United States"},
	}}}
	res, err := e.Predict(context.Background(), c)
	if err != nil {
		t.Fatal(err)
	}
	p := res.Prediction
	if p.Origin != "Vietnam" || !strings.Contains(p.DecisiveStep, "RCEP Art. 3.2") || !strings.HasPrefix(p.Reasoning, RCEPMarketLabel) {
		t.Fatalf("got origin %q step %q: %s", p.Origin, p.DecisiveStep, p.Reasoning)
	}
}

func TestRCEPEngineNeedsCodes(t *testing.T) {
	e := RCEPAuthority(jurisdiction.AU, "Australian Border Force").Engine
	res, err := e.Predict(context.Background(), domain.Case{Product: &domain.Product{Components: []domain.Component{{Name: "frame", MadeIn: "China"}}}})
	if err != nil {
		t.Fatal(err)
	}
	if res.Prediction.Origin != "" || !strings.Contains(res.Prediction.Reasoning, "Add HS codes") {
		t.Fatalf("got %+v", res.Prediction)
	}
}
