package costest

import (
	"context"
	"errors"
	"testing"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/llm"
)

var dryer = domain.Product{Name: "Hair dryer",
	Plants: []domain.Plant{{ID: "vn", Name: "Binh Duong", Country: "Vietnam", Capabilities: []string{"housing_moulding", "pcba"}},
		{ID: "cn", Name: "Ningbo", Country: "China"}, {ID: "my", Name: "Penang", Country: "Malaysia", Capabilities: []string{"pcba"}}},
	Components: []domain.Component{{Name: "housing", MadeIn: "Vietnam", Plant: "vn", Group: "housing_moulding"},
		{Name: "PCBA", MadeIn: "China", Plant: "cn", Group: "pcba"}},
	Operations: []domain.Operation{{Name: "assemble", Plant: "vn", Group: "final_assembly"}}}

func TestBlankCells(t *testing.T) {
	got := Blank(dryer, map[string]map[string]float64{"pcba": {"cn": 70000}})
	want := [][2]string{{"housing_moulding", "vn"}, {"pcba", "vn"}, {"pcba", "my"}}
	if len(got) != len(want) {
		t.Fatalf("blank = %v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("blank[%d] = %v, want %v", i, got[i], want[i])
		}
	}
}

func TestEstimateKeepsOnlyRequestedCells(t *testing.T) {
	m := &llm.Fake{Respond: func(llm.Request) (llm.Response, error) {
		return llm.Response{Text: `{"estimates":[
			{"group":"pcba","plant":"my","low":80000,"usd":75000,"high":70000,"reason":"China cost plus Malaysian labour."},
			{"group":"pcba","plant":"my","low":1,"usd":2,"high":3,"reason":"repeat"},
			{"group":"pcba","plant":"cn","low":1,"usd":2,"high":3,"reason":"not requested"},
			{"group":"housing_moulding","plant":"vn","low":0,"usd":0,"high":0,"reason":"no answer"}]}`}, nil
	}}
	r, err := New(m, "flash").Estimate(context.Background(), Input{Product: dryer, GroupCosts: map[string]map[string]float64{"pcba": {"cn": 70000}}})
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Estimates) != 1 || r.Estimates[0].Plant != "my" || r.Estimates[0].Low != 75000 || r.Estimates[0].High != 75000 || r.Note == "" {
		t.Errorf("estimates = %+v", r)
	}
}

func TestEstimateNeedsAnAnchor(t *testing.T) {
	m := &llm.Fake{Respond: func(llm.Request) (llm.Response, error) { t.Fatal("model called"); return llm.Response{}, nil }}
	if _, err := New(m, "flash").Estimate(context.Background(), Input{Product: dryer}); !errors.Is(err, ErrNoAnchor) {
		t.Errorf("err = %v", err)
	}
	full := map[string]map[string]float64{"housing_moulding": {"vn": 1}, "pcba": {"vn": 1, "cn": 1, "my": 1}}
	if r, err := New(m, "flash").Estimate(context.Background(), Input{Product: dryer, GroupCosts: full}); err != nil || len(r.Estimates) != 0 {
		t.Errorf("full = %+v %v", r, err)
	}
}
