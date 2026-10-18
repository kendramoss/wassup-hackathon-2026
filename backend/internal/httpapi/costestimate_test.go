package httpapi

import (
	"context"
	"net/http"
	"testing"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/costest"
)

type fakeCost struct{ in costest.Input }

func (f *fakeCost) Estimate(_ context.Context, in costest.Input) (costest.Result, error) {
	f.in = in
	if in.AnnualImportValueUSD <= 0 && len(in.GroupCosts) == 0 {
		return costest.Result{}, costest.ErrNoAnchor
	}
	return costest.Result{Estimates: []costest.Estimate{{Group: "pcba", Plant: "my", Low: 60000, USD: 75000, High: 90000, Reason: "x"}}, Note: "n"}, nil
}

func TestCostEstimate(t *testing.T) {
	f := &fakeCost{}
	hs := newHarnessWith(t, "substantial_transformation", func(d *Deps) { d.CostEstimator = f })
	product := `{"name":"Dryer","family":"motorised_tools_small_appliances","plants":[{"id":"cn","country":"China"},{"id":"my","country":"Malaysia","capabilities":["pcba"]}],"components":[{"name":"PCBA","made_in":"China","plant":"cn","group":"pcba"}],"operations":[]}`
	code, out := hs.do(t, "POST", "/v1/products/cost-estimate", `{"product":`+product+`,"economics":{"annual_import_value_usd":900000,"group_costs":{"pcba":{"cn":70000}}}}`)
	if code != http.StatusOK || len(out["estimates"].([]any)) != 1 || out["illustrative"] != true || f.in.GroupNames["pcba"] == "" || f.in.GroupCosts["pcba"]["cn"] != 70000 {
		t.Fatalf("estimate = %d %v (in %+v)", code, out, f.in)
	}
	if code, _ := hs.do(t, "POST", "/v1/products/cost-estimate", `{"product":`+product+`}`); code != http.StatusBadRequest {
		t.Errorf("no anchor = %d", code)
	}
	if code, _ := newHarness(t, "substantial_transformation").do(t, "POST", "/v1/products/cost-estimate", `{}`); code != http.StatusNotFound {
		t.Errorf("off = %d", code)
	}
}
