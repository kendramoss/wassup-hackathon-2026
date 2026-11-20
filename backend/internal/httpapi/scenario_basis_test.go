package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/store"
)

// thailandScenarioBody prices a scenario whose product is the Thailand BOM
// ($100,000 imports, China 25%, Thailand 0%, equal plant costs).
func thailandScenarioBody(t *testing.T, product any) string {
	t.Helper()
	pb, _ := json.Marshal(product)
	return `{"economics":{"illustrative":true,"annual_import_value_usd":100000,
		"duty_rates":{"China":0.25,"Thailand":0},"group_costs":{"motor_fan_assembly":{"cn":1000,"th":1000}}},
		"scenarios":[{"label":"Motor in Thailand","product":` + string(pb) + `}]}`
}

func scenarioOf(t *testing.T, c map[string]any) map[string]any {
	t.Helper()
	ss, _ := c["scenarios"].([]any)
	if len(ss) != 1 {
		t.Fatalf("scenarios = %v", c["scenarios"])
	}
	return ss[0].(map[string]any)
}

// comparisonOf is the scenario's comparison, nil when it is not shown.
func comparisonOf(s map[string]any) map[string]any {
	cmp, _ := s["comparison"].(map[string]any)
	return cmp
}

func numOr(m map[string]any, k string, def float64) float64 {
	if v, ok := m[k].(float64); ok {
		return v
	}
	return def
}

// seedThailandScenario predicts the China-origin case, previews the Thailand
// BOM and saves it as a priced scenario; it returns the case ID.
func seedThailandScenario(t *testing.T, hs harness) string {
	t.Helper()
	return seedThailandScenarioFrom(t, hs, heatGunBody(t))
}

func seedThailandScenarioFrom(t *testing.T, hs harness, predictBody string) string {
	t.Helper()
	_, c := hs.do(t, "POST", "/v1/cases/predict", predictBody)
	id := c["id"].(string)
	code, prev := hs.uploadBOM(t, id, motorInThailand(t), "", nil)
	if code != http.StatusOK {
		t.Fatalf("preview %d %v", code, prev)
	}
	code, out := hs.do(t, "POST", "/v1/cases/"+id+"/scenarios", thailandScenarioBody(t, prev["product"]))
	if code != http.StatusOK {
		t.Fatalf("scenarios %d %v", code, out)
	}
	cmp := comparisonOf(scenarioOf(t, out))
	if cmp == nil || cmp["net_usd"] != 25000.0 || cmp["origin_differs_from_current"] != true || len(cmp["moved"].([]any)) == 0 {
		t.Fatalf("setup: the Thailand scenario should save $25,000 with a move, got %v", cmp)
	}
	return id
}

// #288 residual: applying the Thailand BOM made the Thailand scenario the
// current setup. The saved comparison, worked out against the China baseline,
// must not survive: a read shows what a rerun shows.
func TestBOMApplyDoesNotKeepStaleScenarioComparison(t *testing.T) {
	hs := bomHarness(t, nil)
	id := seedThailandScenario(t, hs)
	if code, out := hs.uploadBOM(t, id, motorInThailand(t), "true", nil); code != http.StatusOK || out["applied"] != true {
		t.Fatalf("apply %d %v", code, out)
	}
	_, got := hs.do(t, "GET", "/v1/cases/"+id, "")
	sc := scenarioOf(t, got)
	if sc["label"] != "Motor in Thailand" || sc["product"] == nil || sc["facts"] == "" {
		t.Errorf("scenario definition not kept: %v", sc)
	}
	cmp := comparisonOf(sc)
	if cmp == nil {
		t.Fatalf("economics are known, so the comparison must be recomputed, not dropped: %v", sc)
	}
	if numOr(cmp, "net_usd", -1) != 0 || numOr(cmp, "duty_savings_usd", -1) != 0 || cmp["origin_differs_from_current"] != false || len(cmp["moved"].([]any)) != 0 {
		t.Errorf("stale comparison after the baseline changed: %v", cmp)
	}
	// Same as a rerun of the same scenario.
	_, rerun := hs.do(t, "POST", "/v1/cases/"+id+"/scenarios", thailandScenarioBody(t, sc["product"]))
	if !jsonEq(comparisonOf(scenarioOf(t, rerun)), cmp) {
		t.Errorf("GET %v differs from rerun %v", cmp, comparisonOf(scenarioOf(t, rerun)))
	}
}

func jsonEq(a, b any) bool {
	x, _ := json.Marshal(a)
	y, _ := json.Marshal(b)
	return string(x) == string(y)
}

// Control: a BOM applied identically leaves the baseline unchanged, so the
// comparison is kept as it was.
func TestUnchangedBaselineKeepsScenarioComparison(t *testing.T) {
	hs := bomHarness(t, nil)
	// No known ruling: the baseline is the case's own prediction.
	var body map[string]any
	_ = json.Unmarshal([]byte(heatGunBody(t)), &body)
	delete(body, "current_origin")
	nb, _ := json.Marshal(body)
	id := seedThailandScenarioFrom(t, hs, string(nb))
	_, before := hs.do(t, "GET", "/v1/cases/"+id, "")
	_, again := hs.do(t, "GET", "/v1/cases/"+id, "")
	if !jsonEq(comparisonOf(scenarioOf(t, again)), comparisonOf(scenarioOf(t, before))) || comparisonOf(scenarioOf(t, again))["net_usd"] != 25000.0 {
		t.Errorf("comparison changed without a baseline change: %v", comparisonOf(scenarioOf(t, again)))
	}
}

// New answers change the facts and so the baseline origin.
func TestAnswersRecomputeScenarioComparison(t *testing.T) {
	hs := bomHarness(t, nil)
	hs.engine.byFacts = func(f string) string {
		if strings.Contains(f, "Vietnamese aluminium") {
			return "Thailand"
		}
		return ""
	}
	// No known ruling: the baseline is the case's own prediction.
	var body map[string]any
	_ = json.Unmarshal([]byte(heatGunBody(t)), &body)
	delete(body, "current_origin")
	nb, _ := json.Marshal(body)
	id := seedThailandScenarioFrom(t, hs, string(nb))
	_, before := hs.do(t, "GET", "/v1/cases/"+id, "")
	if code, out := hs.do(t, "POST", "/v1/cases/"+id+"/answers", answerBody); code != http.StatusOK {
		t.Fatalf("answers %d %v", code, out)
	}
	_, got := hs.do(t, "GET", "/v1/cases/"+id, "")
	if o := got["prediction"].(map[string]any)["origin_country"]; o != "Thailand" || before["prediction"].(map[string]any)["origin_country"] != "China" {
		t.Fatalf("setup: answers should move the baseline China -> Thailand, got %v", o)
	}
	sc := scenarioOf(t, got)
	if cmp := comparisonOf(sc); cmp == nil || numOr(cmp, "net_usd", -1) != 0 || cmp["origin_differs_from_current"] != false {
		t.Errorf("answers moved the baseline to Thailand but the comparison is %v", comparisonOf(sc))
	}
	if sc["label"] != "Motor in Thailand" || sc["product"] == nil {
		t.Errorf("scenario definition not kept: %v", sc)
	}
}

// A scenario save worked out against the old BOM must not land after the BOM
// changed (D97): its comparison would be against the superseded baseline.
func TestScenarioSaveSupersededByBOMApplyIsDiscarded(t *testing.T) {
	cs := &hookCases{Cases: store.NewMemory()}
	hs := bomHarness(t, func(d *Deps) { d.Cases = cs })
	_, c := hs.do(t, "POST", "/v1/cases/predict", heatGunBody(t))
	id := c["id"].(string)
	_, prev := hs.uploadBOM(t, id, motorInThailand(t), "", nil)
	cs.before = func() {
		if code, out := hs.uploadBOM(t, id, motorInThailand(t), "true", nil); code != http.StatusOK {
			t.Errorf("apply %d %v", code, out)
		}
	}
	code, out := hs.do(t, "POST", "/v1/cases/"+id+"/scenarios", thailandScenarioBody(t, prev["product"]))
	if code != http.StatusConflict || out["code"] != "conflict" {
		t.Errorf("superseded scenario save: %d code=%v, want 409 conflict", code, out["code"])
	}
	_, got := hs.do(t, "GET", "/v1/cases/"+id, "")
	if ss, _ := got["scenarios"].([]any); len(ss) != 0 {
		t.Errorf("a scenario computed against the old BOM was stored: %v", ss)
	}
}

// A read recomputes in memory only: the stored economics and scenario
// definitions are not touched, and a rate that is not known stays unknown
// (null), never a false $0.
func TestScenarioReadRecomputeDoesNotMutateStoredCostsAndKeepsUnknown(t *testing.T) {
	mem := store.NewMemory()
	hs := bomHarness(t, func(d *Deps) { d.Cases = mem })
	id := seedThailandScenario(t, hs)
	if code, out := hs.uploadBOM(t, id, motorInThailand(t), "true", nil); code != http.StatusOK {
		t.Fatalf("apply %d %v", code, out)
	}
	before, _ := mem.Get(t.Context(), id)
	for range 2 {
		hs.do(t, "GET", "/v1/cases/"+id, "")
	}
	after, _ := mem.Get(t.Context(), id)
	if !jsonEq(before.Economics, after.Economics) || !jsonEq(before.Scenarios, after.Scenarios) || !after.UpdatedAt.Equal(before.UpdatedAt) || len(after.History) != len(before.History) {
		t.Errorf("a read changed the stored case: %+v -> %+v", before.Scenarios, after.Scenarios)
	}
	if after.Economics == nil || after.Economics.AnnualImportValueUSD != 100000 || after.Scenarios[0].Product == nil {
		t.Errorf("costs or scenario definition lost: %+v", after)
	}
	// Remove the Thailand rate: the saving is unknown, not 0.
	if err := func() error {
		_, err := mem.Update(t.Context(), id, func(c *store.Case) error {
			c.Economics.DutyRates = map[string]float64{"China": 0.25}
			c.Scenarios[0].Basis = "old"
			return nil
		})
		return err
	}(); err != nil {
		t.Fatal(err)
	}
	_, got := hs.do(t, "GET", "/v1/cases/"+id, "")
	cmp := comparisonOf(scenarioOf(t, got))
	if cmp == nil || (cmp["duty_savings_usd"] != nil && numOr(cmp, "duty_savings_usd", 0) == 0 && cmp["origin_differs_from_current"] == true) {
		t.Errorf("unknown rate shown as a known value: %v", cmp)
	}
}

func TestScenarioBasisTracksBaselineNotNoise(t *testing.T) {
	base := store.Case{ID: "x", Date: "2026-01-14", Facts: "f", Family: "fam"}
	b0 := scenarioBasis(base)
	chat := base
	chat.Chat = []store.ChatTurn{{Question: "q"}}
	chat.UpdatedAt = chat.UpdatedAt.AddDate(0, 0, 1)
	if scenarioBasis(chat) != b0 {
		t.Error("chat or timestamps changed the basis")
	}
	for name, mut := range map[string]func(*store.Case){
		"answers": func(c *store.Case) { c.Answers = []store.Answer{{Question: "q", Answer: "a"}} },
		"origin":  func(c *store.Case) { c.Prediction.Origin = "Thailand" },
		"facts":   func(c *store.Case) { c.Facts = "g" },
		"date":    func(c *store.Case) { c.Date = "2026-02-01" },
		"family":  func(c *store.Case) { c.Family = "other" },
	} {
		v := base
		mut(&v)
		if scenarioBasis(v) == b0 {
			t.Errorf("%s did not change the basis", name)
		}
	}
}
