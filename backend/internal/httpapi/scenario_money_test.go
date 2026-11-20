package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/store"
)

// econBody is a money body for the Thailand motor move: China pays cnRate,
// Thailand thRate, and moving the motor group costs thCost - cnCost a year.
func econBody(value float64, cnRate, thRate, cnCost, thCost float64) string {
	return fmt.Sprintf(`{"illustrative":true,"annual_import_value_usd":%v,"duty_rates":{"China":%v,"Thailand":%v},
		"group_costs":{"motor_fan_assembly":{"cn":%v,"th":%v}}}`, value, cnRate, thRate, cnCost, thCost)
}

// $2M, China 25%, Thailand 0%, move costs $120,000: saves 500,000 - 120,000.
const econ2M = `{"illustrative":true,"annual_import_value_usd":2000000,"duty_rates":{"China":0.25,"Thailand":0},"group_costs":{"motor_fan_assembly":{"cn":100000,"th":220000}}}`

func scenarioBodyWith(t *testing.T, econ string, product any) string {
	t.Helper()
	pb, _ := json.Marshal(product)
	return `{"economics":` + econ + `,"scenarios":[{"label":"Motor in Thailand","product":` + string(pb) + `}]}`
}

// moneySeed saves the Thailand scenario priced with econ2M ($380,000).
func moneySeed(t *testing.T, hs harness) (id string, product any) {
	t.Helper()
	_, c := hs.do(t, "POST", "/v1/cases/predict", heatGunBody(t))
	id = c["id"].(string)
	code, prev := hs.uploadBOM(t, id, motorInThailand(t), "", nil)
	if code != http.StatusOK {
		t.Fatalf("preview %d %v", code, prev)
	}
	code, out := hs.do(t, "POST", "/v1/cases/"+id+"/scenarios", scenarioBodyWith(t, econ2M, prev["product"]))
	if code != http.StatusOK {
		t.Fatalf("scenarios %d %v", code, out)
	}
	if n := numOr(comparisonOf(scenarioOf(t, out)), "net_usd", -1); n != 380000 {
		t.Fatalf("setup: net = %v, want 380000", n)
	}
	return id, prev["product"]
}

func netOf(t *testing.T, c map[string]any) float64 {
	t.Helper()
	cmp := comparisonOf(scenarioOf(t, c))
	if cmp == nil || cmp["net_usd"] == nil {
		t.Fatalf("no priced comparison: %v", cmp)
	}
	return cmp["net_usd"].(float64)
}

func putEcon(t *testing.T, hs harness, id, body string) map[string]any {
	t.Helper()
	code, out := hs.do(t, "PUT", "/v1/cases/"+id+"/economics", body)
	if code != http.StatusOK {
		t.Fatalf("PUT economics %d %v", code, out)
	}
	return out
}

// storedCurrent asserts the stored scenario carries a comparison stamped with
// the stored case's own basis (what a reader would trust without recomputing).
func storedCurrent(t *testing.T, mem store.Cases, id string, wantNet float64) {
	t.Helper()
	c, err := mem.Get(t.Context(), id)
	if err != nil {
		t.Fatal(err)
	}
	s := c.Scenarios[0]
	if s.Basis != scenarioBasis(c) || s.Comparison == nil || s.Comparison.NetUSD == nil || *s.Comparison.NetUSD != wantNet {
		t.Errorf("stored scenario not current: basis ok=%v cmp=%+v want net %v", s.Basis == scenarioBasis(c), s.Comparison, wantNet)
	}
}

func TestEconomicsSaveReprices(t *testing.T) {
	cases := map[string]struct {
		body string
		want float64
	}{
		"annual value 2M to 4M": {econBody(4000000, 0.25, 0, 100000, 220000), 880000},
		"duty rate":             {econBody(2000000, 0.25, 0.05, 100000, 220000), 280000},
		"group cost":            {econBody(2000000, 0.25, 0, 100000, 320000), 280000},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			mem := store.NewMemory()
			hs := bomHarness(t, func(d *Deps) { d.Cases = mem })
			id, _ := moneySeed(t, hs)
			out := putEcon(t, hs, id, tc.body)
			// The save response itself is current.
			if ss, _ := out["scenarios"].([]any); len(ss) != 1 || numOr(ss[0].(map[string]any)["comparison"].(map[string]any), "net_usd", -1) != tc.want {
				t.Errorf("PUT response scenarios = %v, want net %v", out["scenarios"], tc.want)
			}
			for i := range 2 { // GET, then a reload
				_, got := hs.do(t, "GET", "/v1/cases/"+id, "")
				if n := netOf(t, got); n != tc.want {
					t.Errorf("GET %d: net %v, want %v", i, n, tc.want)
				}
				if sc := scenarioOf(t, got); sc["product"] == nil || sc["label"] != "Motor in Thailand" {
					t.Errorf("definition lost: %v", sc)
				}
			}
			storedCurrent(t, mem, id, tc.want)
		})
	}
}

func TestEconomicsSaveUnchangedKeepsComparison(t *testing.T) {
	mem := store.NewMemory()
	hs := bomHarness(t, func(d *Deps) { d.Cases = mem })
	id, _ := moneySeed(t, hs)
	before, _ := mem.Get(t.Context(), id)
	// Same money, plus a derived note that is not priced.
	putEcon(t, hs, id, econ2M[:len(econ2M)-1]+`,"duty_rates_note":"noted"}`)
	after, _ := mem.Get(t.Context(), id)
	if !jsonEq(before.Scenarios, after.Scenarios) {
		t.Errorf("unchanged money rewrote the scenarios: %+v -> %+v", before.Scenarios, after.Scenarios)
	}
	_, got := hs.do(t, "GET", "/v1/cases/"+id, "")
	if n := netOf(t, got); n != 380000 {
		t.Errorf("net %v, want 380000", n)
	}
}

// A GET on a case whose stored comparison is stale (a legacy or pre-fix save)
// shows the repriced value and leaves the store as it was.
func TestStaleMoneyBasisRecomputedOnReadWithoutWrite(t *testing.T) {
	mem := store.NewMemory()
	hs := bomHarness(t, func(d *Deps) { d.Cases = mem })
	id, _ := moneySeed(t, hs)
	if _, err := mem.Update(t.Context(), id, func(c *store.Case) error {
		ec := *c.Economics
		ec.AnnualImportValueUSD = 4000000 // economics changed, comparison untouched
		c.Economics = &ec
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	before, _ := mem.Get(t.Context(), id)
	_, got := hs.do(t, "GET", "/v1/cases/"+id, "")
	if n := netOf(t, got); n != 880000 {
		t.Errorf("net %v, want 880000 (stale 380000 shown)", n)
	}
	after, _ := mem.Get(t.Context(), id)
	if !jsonEq(before, after) {
		t.Error("GET mutated the stored case")
	}
}

// An economics save landing while a scenario save is held discards the
// scenario save (409) rather than committing comparisons priced with the old
// money.
func TestEconomicsPutDuringScenarioSaveConflicts(t *testing.T) {
	cs := &hookCases{Cases: store.NewMemory()}
	hs := bomHarness(t, func(d *Deps) { d.Cases = cs })
	_, c := hs.do(t, "POST", "/v1/cases/predict", heatGunBody(t))
	id := c["id"].(string)
	_, prev := hs.uploadBOM(t, id, motorInThailand(t), "", nil)
	if code, _ := hs.do(t, "PUT", "/v1/cases/"+id+"/economics", econ2M); code != http.StatusOK {
		t.Fatal("seed economics")
	}
	cs.before = func() {
		if code, out := hs.do(t, "PUT", "/v1/cases/"+id+"/economics", econBody(4000000, 0.25, 0, 100000, 220000)); code != http.StatusOK {
			t.Errorf("overlapping PUT %d %v", code, out)
		}
	}
	// The scenario call carries no economics, so it reuses the stored $2M.
	code, out := hs.do(t, "POST", "/v1/cases/"+id+"/scenarios", `{"scenarios":[{"label":"Motor in Thailand","product":`+mustJSON(prev["product"])+`}]}`)
	if code != http.StatusConflict || out["code"] != "conflict" {
		t.Errorf("scenario save over a changed economics: %d code=%v, want 409", code, out["code"])
	}
	got, _ := cs.Get(t.Context(), id)
	if len(got.Scenarios) != 0 || got.Economics.AnnualImportValueUSD != 4000000 {
		t.Errorf("stale commit or lost PUT: scenarios=%d econ=%+v", len(got.Scenarios), got.Economics)
	}
}

func mustJSON(v any) string { b, _ := json.Marshal(v); return string(b) }

// The scenario save carrying new economics is the change itself: it must not
// conflict with itself, its stamp matches the resulting case, and saving the
// same thing again is fine.
func TestScenarioSaveWithChangedEconomicsDoesNotSelfConflict(t *testing.T) {
	mem := store.NewMemory()
	hs := bomHarness(t, func(d *Deps) { d.Cases = mem })
	id, product := moneySeed(t, hs)
	body := scenarioBodyWith(t, econBody(4000000, 0.25, 0, 100000, 220000), product)
	for i := range 3 {
		code, out := hs.do(t, "POST", "/v1/cases/"+id+"/scenarios", body)
		if code != http.StatusOK {
			t.Fatalf("save %d: %d %v", i, code, out)
		}
		if n := netOf(t, out); n != 880000 {
			t.Errorf("save %d response net %v, want 880000", i, n)
		}
		storedCurrent(t, mem, id, 880000)
	}
	_, got := hs.do(t, "GET", "/v1/cases/"+id, "")
	if n := netOf(t, got); n != 880000 {
		t.Errorf("GET net %v", n)
	}
}
