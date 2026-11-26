package httpapi

import (
	"context"
	"strings"
	"testing"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/jurisdiction"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/tariffshift"
)

func mexicoEngineForTest(t *testing.T) mexicoEngine {
	t.Helper()
	r, err := tariffshift.MexicoRules()
	if err != nil {
		t.Fatal(err)
	}
	a := MexicoAuthority(r)
	if a.Market != jurisdiction.MX {
		t.Fatalf("market %q", a.Market)
	}
	return a.Engine.(mexicoEngine)
}

func TestMexicoEngineDecides(t *testing.T) {
	e := mexicoEngineForTest(t)
	c := domain.Case{Product: &domain.Product{HTS: "8483.20", Plants: []domain.Plant{{ID: "p1", Country: "Vietnam"}},
		Components: []domain.Component{{Name: "housing", HTS: "7325.99", MadeIn: "China"}}}}
	res, err := e.Predict(context.Background(), c)
	if err != nil {
		t.Fatal(err)
	}
	if res.Prediction.Origin != "Vietnam" || !strings.HasPrefix(res.Prediction.Reasoning, MexicoLabel) {
		t.Fatalf("got %+v", res.Prediction)
	}
}

func TestMexicoEngineNoRowNeedsPerson(t *testing.T) {
	e := mexicoEngineForTest(t)
	c := domain.Case{Product: &domain.Product{HTS: "2009.90", Plants: []domain.Plant{{ID: "p1", Country: "Vietnam"}},
		Components: []domain.Component{{Name: "oranges", HTS: "0805.10", MadeIn: "Brazil"}}}}
	res, err := e.Predict(context.Background(), c)
	if err != nil {
		t.Fatal(err)
	}
	if res.Prediction.Origin != "" || !strings.Contains(res.Prediction.Reasoning, "art. 3 II") {
		t.Fatalf("got %+v", res.Prediction)
	}
}

func TestMexicoEngineNeedsCodes(t *testing.T) {
	e := mexicoEngineForTest(t)
	res, _ := e.Predict(context.Background(), domain.Case{Product: &domain.Product{Components: []domain.Component{{Name: "frame", MadeIn: "China"}}}})
	if res.Prediction.Origin != "" || !strings.Contains(res.Prediction.Reasoning, "Add HS codes") {
		t.Fatalf("got %+v", res.Prediction)
	}
}
