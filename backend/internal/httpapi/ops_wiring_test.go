package httpapi

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/audit"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/planner"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/planner/opcost"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/store"
)

// #281 step 3 wiring tests through the real handlers and the real planner
// (fake engine and verifier only; no model, provider or network).

// opsHandlerOps: the motor group, cn -> th, labour and logistics additional to
// the group cost. Hand-worked with the motor group cost cn 300,000, th 420,000
// and the 41.7% -> 16.7% duty on $2,000,000 (see the planner tests): duty
// 500,000, operating 115,000, one-time x over 4 years.
func opsHandlerOps(oneTime string, thExtra string) string {
	return fmt.Sprintf(`{"horizon_years":4,"groups":{"motor_fan_assembly":{"cost_basis":"additional","plants":{
	 "cn":{"ref":{"name":"China suppliers","country":"China"},"labour_usd_per_year":100000,"logistics_usd_per_year":10000},
	 "th":{"ref":{"name":"Rayong plant","country":"Thailand"},"labour_usd_per_year":60000,"logistics_usd_per_year":45000,"capex_usd":%s,"retooling_usd":0%s}}}}}`, oneTime, thExtra)
}

const opsLegacyMoney = `"illustrative":true,"annual_import_value_usd":2000000,"duty_rates":{"China":0.417,"Thailand":0.167},"group_costs":{"motor_fan_assembly":{"cn":300000,"th":420000}}`

func opsEcon(ops string) string { return `{` + opsLegacyMoney + `,"operations":` + ops + `}` }

func newOpsCase(t *testing.T, adjust func(*Deps)) (harness, string) {
	t.Helper()
	hs := realPlannerHarness(t, adjust)
	_, c := hs.do(t, "POST", "/v1/cases/predict", heatGunBody(t))
	return hs, c["id"].(string)
}

func doReq(hs harness, method, path, body string, hdr map[string]string) (int, string) {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	hs.h.ServeHTTP(rec, req)
	return rec.Code, rec.Body.String()
}

func planBody(econ string) string { return `{"target":"Thailand","economics":` + econ + `}` }

func rowOf(t *testing.T, list any, key string) map[string]any {
	t.Helper()
	for _, p := range list.([]any) {
		pm := p.(map[string]any)
		var parts []string
		for _, m := range pm["move"].([]any) {
			mm := m.(map[string]any)
			parts = append(parts, mm["group"].(string)+"@"+mm["to_plant"].(string))
		}
		if strings.Join(parts, "+") == key {
			return pm
		}
	}
	return nil
}

// 1. Inactive operations (null, {}, {"groups":{}}) through the real handlers
// are the legacy goldens, byte for byte, for every flow.
func TestInactiveOperationsAreTheLegacyGoldensThroughHandlers(t *testing.T) {
	for _, inject := range []string{`"operations":null`, `"operations":{}`, `"operations":{"groups":{}}`, `"operations":{"note":"only a note"}`} {
		for name, body := range legacyEconomics {
			t.Run(name+"/"+inject, func(t *testing.T) {
				goldenCheck(t, "flows_"+name, legacyFlows(t, body, inject))
			})
		}
	}
}

// 2. Validation: every slug through every route that takes economics; strict
// decode; nothing runs and nothing is stored on a refusal.
func TestOpsValidationSlugsAndStrictDecode(t *testing.T) {
	ref := func(p string) string {
		if p == "th" {
			return `"ref":{"name":"Rayong plant","country":"Thailand"}`
		}
		return `"ref":{"name":"China suppliers","country":"China"}`
	}
	plant := func(p, extra string) string { return `"` + p + `":{` + ref(p) + extra + `}` }
	group := func(extra string, plants ...string) string {
		return `{"groups":{"motor_fan_assembly":{` + extra + `"plants":{` + strings.Join(plants, ",") + `}}}}`
	}
	type bad struct {
		slug, ops string
		writeOnly bool // only where the product is known (PUT economics, create case)
	}
	cases := []bad{
		{"horizon_required", group(``, plant("th", `,"capex_usd":1000`)), false},
		{"horizon_out_of_range", `{"horizon_years":0}`, false},
		{"horizon_out_of_range", `{"horizon_years":31}`, false},
		{"lead_time_out_of_range", `{"max_lead_time_days":-1}`, false},
		{"cost_basis_required", group(``, plant("th", `,"labour_usd_per_year":5`)), false},
		{"cost_basis_invalid", group(`"cost_basis":"free",`, plant("th", `,"labour_usd_per_year":5`)), false},
		{"components_exceed_group_cost", group(`"cost_basis":"included",`, plant("th", `,"labour_usd_per_year":300000,"logistics_usd_per_year":120001`)), false},
		{"included_requires_group_cost", `{"groups":{"pcba":{"cost_basis":"included","plants":{"th":{"ref":{"name":"Rayong plant","country":"Thailand"},"labour_usd_per_year":1}}}}}`, false},
		{"unit_required", group(`"annual_demand_units":100,`, plant("th", `,"lead_time_days":5`)), false},
		{"moq_unit_required", group(``, plant("th", `,"moq_units":5`)), false},
		{"orders_per_year_out_of_range", group(`"orders_per_year":0,`, plant("th", `,"lead_time_days":5`)), false},
		{"value_out_of_range", group(``, plant("th", `,"capex_usd":-5`)), false},
		{"plant_ref_required", `{"groups":{"motor_fan_assembly":{"cost_basis":"additional","plants":{"th":{"labour_usd_per_year":5}}}}}`, false},
		{"unknown_source_key", group(``, plant("th", `,"lead_time_days":5,"sources":{"colour":"x"}`)), false},
		{"string_too_long", `{"horizon_years":1,"note":"` + strings.Repeat("n", 501) + `"}`, false},
		{"unknown_group", `{"horizon_years":1,"groups":{"nonexistent":{"unit":"pcs"}}}`, true},
		{"unknown_plant", group(``, `"zz":{"ref":{"name":"x","country":"y"},"lead_time_days":5}`), true},
		{"plant_ref_mismatch", group(``, `"th":{"ref":{"name":"Other works","country":"Thailand"},"lead_time_days":5}`), true},
		{"unknown_field", `{"horizon_years":1,"cost_bais":"x"}`, false},
		{"unknown_field", `{"groups":{"motor_fan_assembly":{"plants":{"th":{"labour_usd_per_yr":5}}}}}`, false},
		{"invalid_json", `{"horizon_years":"five"}`, false},
		{"invalid_json", `{"horizon_years":1e999}`, false},
		{"too_large", `{"horizon_years":1,"note":"` + strings.Repeat("n", 400) + `","groups":{` + bigGroups() + `}}`, false},
	}
	type route struct {
		name   string
		strict bool
		call   func(hs harness, id, econ string) (int, string)
	}
	routes := []route{
		{"PUT economics", true, func(hs harness, id, e string) (int, string) {
			return doReq(hs, "PUT", "/v1/cases/"+id+"/economics", e, nil)
		}},
		{"plan", false, func(hs harness, id, e string) (int, string) {
			return doReq(hs, "POST", "/v1/cases/"+id+"/plan", planBody(e), nil)
		}},
		{"plan stream", false, func(hs harness, id, e string) (int, string) {
			return doReq(hs, "POST", "/v1/cases/"+id+"/plan", planBody(e), map[string]string{"Accept": "text/event-stream"})
		}},
		{"compare-targets", false, func(hs harness, id, e string) (int, string) {
			return doReq(hs, "POST", "/v1/cases/"+id+"/compare-targets", `{"targets":["Thailand","Malaysia"],"economics":`+e+`}`, nil)
		}},
		{"scenarios", false, func(hs harness, id, e string) (int, string) {
			return doReq(hs, "POST", "/v1/cases/"+id+"/scenarios", `{"economics":`+e+`,"scenarios":[{"label":"x","facts":"The unit is assembled in Mexico."}]}`, nil)
		}},
		{"predict", true, func(hs harness, _, e string) (int, string) {
			var m map[string]any
			_ = json.Unmarshal([]byte(heatGunBody(t)), &m)
			m["economics"] = json.RawMessage(e)
			b, _ := json.Marshal(m)
			return doReq(hs, "POST", "/v1/cases/predict", string(b), nil)
		}},
	}
	for _, rt := range routes {
		for _, tc := range cases {
			if tc.writeOnly && !rt.strict {
				continue
			}
			t.Run(rt.name+"/"+tc.slug, func(t *testing.T) {
				mem := store.NewMemory()
				hs, id := newOpsCase(t, func(d *Deps) { d.Cases = mem })
				econ := `{"annual_import_value_usd":2000000,"group_costs":{"motor_fan_assembly":{"cn":300000,"th":420000}},"operations":` + tc.ops + `}`
				if tc.slug == "too_large" || strings.HasPrefix(tc.slug, "unknown_field") || tc.slug == "invalid_json" {
					econ = `{"annual_import_value_usd":2000000,"operations":` + tc.ops + `}`
				}
				before, _ := mem.Get(context.Background(), id)
				code, body := rt.call(hs, id, econ)
				if code != http.StatusBadRequest || !strings.Contains(body, `"invalid_input"`) {
					t.Fatalf("status %d %s, want 400 invalid_input", code, body)
				}
				if want := "operations." + tc.slug + ":"; !strings.Contains(body, want) || !strings.Contains(body, "economics: ") {
					t.Errorf("message lacks economics: ... %q: %.400s", want, body)
				}
				after, _ := mem.Get(context.Background(), id)
				if !reflect.DeepEqual(before, after) {
					t.Error("a refused request changed the stored case")
				}
			})
		}
	}
}

func bigGroups() string {
	var pl []string
	for i := range 30 {
		pl = append(pl, fmt.Sprintf(`"p%d":{"ref":{"name":"%s","country":"%s"},"lead_time_days":1}`, i, strings.Repeat("n", 100), strings.Repeat("c", 100)))
	}
	var parts []string
	for i := range 10 { // ten groups of about 8 KB: over 64 KiB, under the 256 KiB body limit
		parts = append(parts, fmt.Sprintf(`"big%d":{"plants":{%s}}`, i, strings.Join(pl, ",")))
	}
	return strings.Join(parts, ",")
}

// A body without operations keeps its old error text byte for byte.
func TestNoOperationsKeepsTheOldDecodeError(t *testing.T) {
	hs, id := newOpsCase(t, nil)
	_, body := doReq(hs, "POST", "/v1/cases/"+id+"/plan", `{"target":"Thailand","economics":{"annual_import_value_usd":1,"typo_field":3}}`, nil)
	if !strings.Contains(body, `request body: json: unknown field \"typo_field\"`) || strings.Contains(body, "operations.") {
		t.Errorf("legacy decode error changed: %s", body)
	}
}

// 3. Persistence: the case stores the RAW economics, explicit zeros survive,
// null and an inactive block are not stored, derived rates are not stored.
func TestOpsPersistenceRoundTripRawEconomics(t *testing.T) {
	for _, kind := range []string{"memory", "file"} {
		t.Run(kind, func(t *testing.T) {
			var cs store.Cases = store.NewMemory()
			if kind == "file" {
				f, err := store.NewFile(t.TempDir())
				if err != nil {
					t.Fatal(err)
				}
				cs = f
			}
			hs, id := newOpsCase(t, func(d *Deps) { d.Cases = cs })
			ops := opsHandlerOps(`900000`, `,"lead_time_days":0,"moq_units":0,"moq_unit":"pcs","sources":{"capex_usd":"quote Q-1 (synthetic)"}`)
			if code, out := hs.do(t, "PUT", "/v1/cases/"+id+"/economics", opsEcon(ops)); code != http.StatusOK {
				t.Fatalf("PUT %d %v", code, out)
			}
			_, got := hs.do(t, "GET", "/v1/cases/"+id, "")
			stored := got["economics"].(map[string]any)["operations"].(map[string]any)
			th := stored["groups"].(map[string]any)["motor_fan_assembly"].(map[string]any)["plants"].(map[string]any)["th"].(map[string]any)
			for _, k := range []string{"retooling_usd", "lead_time_days", "moq_units"} {
				if v, ok := th[k]; !ok || v != 0.0 {
					t.Errorf("explicit zero %s lost: %v", k, th)
				}
			}
			if th["sources"].(map[string]any)["capex_usd"] != "quote Q-1 (synthetic)" {
				t.Errorf("sources lost: %v", th["sources"])
			}
			if _, ok := stored["groups"].(map[string]any)["motor_fan_assembly"].(map[string]any)["unit"]; ok {
				t.Error("an unentered field was stored")
			}
			// A plan with operations stores RAW economics: no table or live rate.
			if code, out := hs.do(t, "POST", "/v1/cases/"+id+"/plan", `{"target":"Thailand","economics":{"annual_import_value_usd":2000000,"operations":`+ops+`}}`); code != http.StatusOK {
				t.Fatalf("plan %d %v", code, out)
			}
			c, _ := cs.Get(context.Background(), id)
			if len(c.Economics.DutyRates) != 0 || len(c.Economics.DutyRatesFromTable) != 0 || c.Economics.Operations == nil {
				t.Errorf("stored economics after plan = %+v", c.Economics)
			}
			_, got = hs.do(t, "GET", "/v1/cases/"+id, "")
			if r, _ := got["economics_priced"].(map[string]any)["duty_rates"].(map[string]any); r["China"] == nil {
				t.Errorf("display still shows the derived rates: %v", got["economics_priced"])
			}
			// An inactive block is dropped on save.
			if code, _ := hs.do(t, "PUT", "/v1/cases/"+id+"/economics", opsEcon(`{"groups":{}}`)); code != http.StatusOK {
				t.Fatal("PUT inactive")
			}
			c, _ = cs.Get(context.Background(), id)
			if c.Economics.Operations != nil {
				t.Errorf("inactive block stored: %+v", c.Economics.Operations)
			}
			// Firestore keeps the case as a gzipped JSON blob (store.gzipJSON); the
			// stored value is the same JSON, so inspect it: nothing to migrate.
			if code, _ := hs.do(t, "PUT", "/v1/cases/"+id+"/economics", opsEcon(ops)); code != http.StatusOK {
				t.Fatal("PUT")
			}
			c, _ = cs.Get(context.Background(), id)
			blob, _ := json.Marshal(c)
			var buf bytes.Buffer
			zw := gzip.NewWriter(&buf)
			_, _ = zw.Write(blob)
			_ = zw.Close()
			zr, _ := gzip.NewReader(&buf)
			raw, _ := io.ReadAll(zr)
			var back store.Case
			if err := json.Unmarshal(raw, &back); err != nil || !reflect.DeepEqual(back.Economics.Operations, c.Economics.Operations) ||
				!strings.Contains(string(raw), `"retooling_usd":0`) {
				t.Errorf("blob round trip lost operations: %v", err)
			}
		})
	}
}

// A case stored before this field loads and plans exactly as before.
func TestOldStoredEconomicsJSONLoadsWithoutOperations(t *testing.T) {
	var e planner.Economics
	if err := json.Unmarshal([]byte(`{"illustrative":true,"annual_import_value_usd":5,"duty_rates":{"China":0.1},"group_costs":{"g":{"p":1}}}`), &e); err != nil || e.Operations != nil {
		t.Fatalf("decode: %v %+v", err, e)
	}
	b, _ := json.Marshal(e)
	if strings.Contains(string(b), "operations") {
		t.Errorf("re-encoded legacy economics gained operations: %s", b)
	}
}

// 4. Plan JSON and SSE with operations through the real planner.
func TestOpsPlanJSONAndSSEThroughHandlers(t *testing.T) {
	mem := store.NewMemory()
	hs, id := newOpsCase(t, func(d *Deps) { d.Cases = mem })
	ops := opsHandlerOps(`1900000`, ``) // capex 1.9M + retooling 0 over 4 years = 475,000: net 500,000 - 115,000 - 475,000 = -90,000
	code, out := hs.do(t, "POST", "/v1/cases/"+id+"/plan", planBody(opsEcon(ops)))
	if code != http.StatusOK {
		t.Fatalf("plan %d %v", code, out)
	}
	plan := out["plan"].(map[string]any)
	obj := plan["objective"].(map[string]any)
	if obj["mode"] != "operations_v1" || obj["scope"] != "searched_moves_only" {
		t.Errorf("objective = %v", obj)
	}
	c, _ := mem.Get(context.Background(), id)
	if plan["economics_basis"] != economicsHash(c.Economics) || plan["economics_basis"] == "" {
		t.Errorf("economics_basis %v is not the hash of the stored raw economics %s", plan["economics_basis"], economicsHash(c.Economics))
	}
	row := rowOf(t, plan["rejected"], "motor_fan_assembly@th")
	if row == nil || row["shown"] != false || row["rejection_kind"] != "net_costlier" || row["net_per_year"] != -90000.0 {
		t.Fatalf("motor row = %v", row)
	}
	ops2 := row["operations"].(map[string]any)
	if ops2["decision"] != "net_costlier" || ops2["net_modelled_per_year"] != -90000.0 || ops2["priced"] != true {
		t.Errorf("operations = %v", ops2)
	}
	// Not stale right after a normal request (the stamp hashes the raw money).
	_, got := hs.do(t, "GET", "/v1/cases/"+id, "")
	if _, bad := got["plan_economics_stale"]; bad {
		t.Error("a fresh plan is reported stale")
	}

	// The stream: the same search, the same stamp; move_done already carries the
	// demotion; event names and their order are unchanged.
	hs2, id2 := newOpsCase(t, nil)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/v1/cases/"+id2+"/plan", strings.NewReader(planBody(opsEcon(ops))))
	req.Header.Set("Accept", "text/event-stream")
	hs2.h.ServeHTTP(rec, req)
	ids := map[string]bool{id2: true}
	s := sseGolden(t, rec.Body.String(), ids)
	done := s["done"].(map[string]any)["plan"].(map[string]any)
	if done["economics_basis"] != plan["economics_basis"] {
		t.Errorf("streamed stamp %v != json stamp %v", done["economics_basis"], plan["economics_basis"])
	}
	md := s["move_done"].(map[string]any)["motor_fan_assembly@th"].(map[string]any)["plan"].(map[string]any)
	if md["shown"] != false || md["rejection_kind"] != "net_costlier" || md["operations"] == nil {
		t.Errorf("move_done is not already demoted: %v", md)
	}
	for k, v := range s["move_done"].(map[string]any) {
		final := rowOf(t, done["plans"], k)
		if final == nil {
			final = rowOf(t, done["rejected"], k)
		}
		p := v.(map[string]any)["plan"].(map[string]any)
		if final == nil || final["shown"] != p["shown"] || final["rejection_kind"] != p["rejection_kind"] {
			t.Errorf("move %s: move_done (%v/%v) disagrees with the final result", k, p["shown"], p["rejection_kind"])
		}
	}
}

// 5. Staleness after a cost PUT, and policy/provenance-only edits.
func TestOpsStalenessAfterCostPUT(t *testing.T) {
	mem := store.NewMemory()
	hs, id := newOpsCase(t, func(d *Deps) { d.Cases = mem })
	ops := opsHandlerOps(`900000`, `,"sources":{"capex_usd":"quote Q-1"}`)
	if code, out := hs.do(t, "POST", "/v1/cases/"+id+"/plan", planBody(opsEcon(ops))); code != http.StatusOK {
		t.Fatalf("plan %d %v", code, out)
	}
	// compare-targets and a scenario are stamped/priced under the same money
	if code, out := hs.do(t, "POST", "/v1/cases/"+id+"/compare-targets", `{"targets":["Thailand","Malaysia"]}`); code != http.StatusOK {
		t.Fatalf("compare %d %v", code, out)
	}
	_, prev := hs.uploadBOM(t, id, motorInThailand(t), "", nil)
	if code, out := hs.do(t, "POST", "/v1/cases/"+id+"/scenarios", `{"scenarios":[{"label":"Motor in Thailand","product":`+mustJSON(prev["product"])+`}]}`); code != http.StatusOK {
		t.Fatalf("scenarios %d %v", code, out)
	}
	cur, _ := mem.Get(context.Background(), id)
	if cur.Plan == nil || cur.Plan.EconomicsBasis == "" || planMoneyStale(cur) || targetsMoneyStale(cur) {
		t.Fatalf("setup: stamp %q stale plan %v targets %v", cur.Plan.EconomicsBasis, planMoneyStale(cur), targetsMoneyStale(cur))
	}
	stale := func(c map[string]any, key string) bool { v, _ := c[key].(bool); return v }

	// Provenance-only edit: note and sources change, the money does not. Nothing
	// reprices, nothing goes stale, and the stored result bytes are untouched.
	planBefore, _ := json.Marshal(cur.Plan)
	scBefore, _ := json.Marshal(cur.Scenarios)
	prov := strings.Replace(strings.Replace(ops, `"horizon_years":4,`, `"horizon_years":4,"note":"edited note",`, 1), "quote Q-1", "quote Q-2 (renewed)", 1)
	out := putEcon(t, hs, id, opsEcon(prov))
	if stale(out, "plan_economics_stale") || stale(out, "targets_economics_stale") {
		t.Errorf("a provenance-only edit marked the plan stale: %v", out)
	}
	after, _ := mem.Get(context.Background(), id)
	planAfter, _ := json.Marshal(after.Plan)
	scAfter, _ := json.Marshal(after.Scenarios)
	if string(planBefore) != string(planAfter) || string(scBefore) != string(scAfter) {
		t.Error("a provenance-only edit rewrote the stored plan or scenarios")
	}
	if after.Economics.Operations.Note != "edited note" {
		t.Error("the edited provenance was not stored")
	}
	if a, b := scenarioBasis(cur), scenarioBasis(after); a != b {
		t.Error("note/sources changed the scenario basis")
	}

	// A cost edit: the stored plan is stale, flagged on the PUT response and on
	// the case view; scenarios are repriced; Ask says the net is not current.
	costed := strings.Replace(ops, `"labour_usd_per_year":60000`, `"labour_usd_per_year":61000`, 1)
	out = putEcon(t, hs, id, opsEcon(costed))
	if !stale(out, "plan_economics_stale") || !stale(out, "targets_economics_stale") {
		t.Errorf("a cost edit did not mark results stale: %v", out)
	}
	_, got := hs.do(t, "GET", "/v1/cases/"+id, "")
	if !stale(got, "plan_economics_stale") || !stale(got, "targets_economics_stale") {
		t.Errorf("GET does not flag the stale plan: plan=%v targets=%v", got["plan_economics_stale"], got["targets_economics_stale"])
	}
	if got["plan"] == nil {
		t.Error("the stored plan was removed; it must stay as history")
	}
	now, _ := mem.Get(context.Background(), id)
	if txt := agentCaseText(now); !strings.Contains(txt, "net not current") || strings.Contains(txt, "modelled net $") {
		t.Errorf("Ask context presents old money as current:\n%s", txt)
	}
	// Scenario comparisons follow the money (no stale flag needed).
	if a, b := scenarioBasis(cur), scenarioBasis(now); a == b {
		t.Error("a labour edit left the scenario basis unchanged")
	}
	if cmp := comparisonOf(scenarioOf(t, got)); cmp == nil || cmp["operations"] == nil {
		t.Errorf("scenario comparison not repriced with operations: %v", cmp)
	}
	// Editing the money back makes the stored stamp current again (same hash).
	out = putEcon(t, hs, id, opsEcon(ops))
	if stale(out, "plan_economics_stale") {
		t.Errorf("restoring the money left the plan stale: %v", out)
	}
	// Removing the operations (legacy money) is a change too.
	out = putEcon(t, hs, id, `{`+opsLegacyMoney+`}`)
	if !stale(out, "plan_economics_stale") {
		t.Errorf("dropping operations did not mark the operations plan stale: %v", out)
	}
}

// Ask context for a fresh operations plan states the scope even when priced.
func TestOpsAskContextStatesScopeWhenPriced(t *testing.T) {
	mem := store.NewMemory()
	hs, id := newOpsCase(t, func(d *Deps) { d.Cases = mem })
	if code, out := hs.do(t, "POST", "/v1/cases/"+id+"/plan", planBody(opsEcon(opsHandlerOps(`100000`, ``)))); code != http.StatusOK {
		t.Fatalf("plan %d %v", code, out)
	}
	c, _ := mem.Get(context.Background(), id)
	txt := agentCaseText(c)
	if !strings.Contains(txt, "modelled net $") || !strings.Contains(txt, "modelled terms only; not modelled:") || !strings.Contains(txt, "subsidies_incentives") ||
		!strings.Contains(txt, "feasibility not assessed") || strings.Contains(strings.ToLower(txt), "optimal") {
		t.Errorf("priced operations plan lacks its scope:\n%s", txt)
	}
	if !strings.Contains(txt, "supported by precedent but demoted") && strings.Contains(txt, "net_costlier") {
		t.Errorf("a cost demotion is not explained:\n%s", txt)
	}
}

// 6. Money edited while a plan priced from the STORED economics is held: 409,
// nothing stored. A plan that carries its own money does not self-conflict.
func TestOpsPlanHeldWhileMoneyEditedConflicts(t *testing.T) {
	ops := opsHandlerOps(`900000`, ``)
	changed := opsEcon(strings.Replace(ops, `"labour_usd_per_year":60000`, `"labour_usd_per_year":99000`, 1))
	run := func(t *testing.T, stream bool, body string, withStored bool) (int, string, store.Case, harness, string) {
		cs := &hookCases{Cases: store.NewMemory()}
		hs, id := newOpsCase(t, func(d *Deps) { d.Cases = cs })
		if withStored {
			if code, _ := hs.do(t, "PUT", "/v1/cases/"+id+"/economics", opsEcon(ops)); code != http.StatusOK {
				t.Fatal("seed economics")
			}
		}
		cs.before = func() {
			if code, out := hs.do(t, "PUT", "/v1/cases/"+id+"/economics", changed); code != http.StatusOK {
				t.Errorf("overlapping PUT %d %v", code, out)
			}
		}
		hdr := map[string]string{}
		if stream {
			hdr["Accept"] = "text/event-stream"
		}
		code, out := doReq(hs, "POST", "/v1/cases/"+id+"/plan", body, hdr)
		c, _ := cs.Get(context.Background(), id)
		return code, out, c, hs, id
	}
	for _, stream := range []bool{false, true} {
		t.Run(fmt.Sprintf("stored money reused, stream=%v", stream), func(t *testing.T) {
			code, out, c, _, _ := run(t, stream, `{"target":"Thailand"}`, true)
			conflict := code == http.StatusConflict && strings.Contains(out, `"conflict"`)
			if stream {
				conflict = strings.Contains(out, "event: error") && strings.Contains(out, `"conflict"`) && !strings.Contains(out, "event: done")
			}
			if !conflict {
				t.Errorf("status %d: %s", code, out)
			}
			if c.Plan != nil || c.Economics.Operations.Groups["motor_fan_assembly"].Plants["th"].Labour == nil || *c.Economics.Operations.Groups["motor_fan_assembly"].Plants["th"].Labour != 99000 {
				t.Errorf("stale commit or lost PUT: plan=%v", c.Plan != nil)
			}
		})
		t.Run(fmt.Sprintf("own money in the request, stream=%v", stream), func(t *testing.T) {
			code, out, c, _, _ := run(t, stream, planBody(opsEcon(ops)), false)
			if (!stream && code != http.StatusOK) || strings.Contains(out, "event: error") {
				t.Fatalf("a request with its own money conflicted with itself: %d %.300s", code, out)
			}
			if c.Plan == nil || c.Plan.EconomicsBasis != economicsHash(c.Economics) {
				t.Errorf("stored plan %v is not stamped with the stored money", c.Plan != nil)
			}
		})
	}
	t.Run("compare-targets", func(t *testing.T) {
		cs := &hookCases{Cases: store.NewMemory()}
		hs, id := newOpsCase(t, func(d *Deps) { d.Cases = cs })
		hs.do(t, "PUT", "/v1/cases/"+id+"/economics", opsEcon(ops))
		cs.before = func() { hs.do(t, "PUT", "/v1/cases/"+id+"/economics", changed) }
		code, out := doReq(hs, "POST", "/v1/cases/"+id+"/compare-targets", `{"targets":["Thailand","Malaysia"]}`, nil)
		if code != http.StatusConflict || !strings.Contains(out, `"conflict"`) {
			t.Errorf("status %d %s, want 409", code, out)
		}
		if c, _ := cs.Get(context.Background(), id); c.Targets != nil {
			t.Error("stale comparison stored")
		}
	})
}

// A scenario save held while an operations input changes is discarded (D97).
func TestOpsPutDuringScenarioSaveConflicts(t *testing.T) {
	cs := &hookCases{Cases: store.NewMemory()}
	hs := bomHarness(t, func(d *Deps) { d.Cases = cs })
	_, c := hs.do(t, "POST", "/v1/cases/predict", heatGunBody(t))
	id := c["id"].(string)
	_, prev := hs.uploadBOM(t, id, motorInThailand(t), "", nil)
	ops := opsHandlerOps(`900000`, ``)
	if code, _ := hs.do(t, "PUT", "/v1/cases/"+id+"/economics", opsEcon(ops)); code != http.StatusOK {
		t.Fatal("seed economics")
	}
	cs.before = func() {
		hs.do(t, "PUT", "/v1/cases/"+id+"/economics", opsEcon(strings.Replace(ops, `"logistics_usd_per_year":45000`, `"logistics_usd_per_year":46000`, 1)))
	}
	code, out := hs.do(t, "POST", "/v1/cases/"+id+"/scenarios", `{"scenarios":[{"label":"Motor in Thailand","product":`+mustJSON(prev["product"])+`}]}`)
	if code != http.StatusConflict || out["code"] != "conflict" {
		t.Errorf("scenario save over a changed operations input: %d %v, want 409", code, out["code"])
	}
	// And a save that carries the new operations is the change itself.
	hs2 := bomHarness(t, nil)
	_, c2 := hs2.do(t, "POST", "/v1/cases/predict", heatGunBody(t))
	id2 := c2["id"].(string)
	_, prev2 := hs2.uploadBOM(t, id2, motorInThailand(t), "", nil)
	body := `{"economics":` + opsEcon(ops) + `,"scenarios":[{"label":"Motor in Thailand","product":` + mustJSON(prev2["product"]) + `}]}`
	for i := range 2 {
		code, out := hs2.do(t, "POST", "/v1/cases/"+id2+"/scenarios", body)
		if code != http.StatusOK {
			t.Fatalf("save %d: %d %v", i, code, out)
		}
		cmp := comparisonOf(scenarioOf(t, out))
		// net: duty 500,000 - operating 115,000 - amortized 225,000 = 160,000
		if cmp["net_usd"] != 160000.0 || cmp["operations"] == nil || cmp["amortized_one_time_usd"] != 225000.0 {
			t.Errorf("save %d comparison = %v", i, cmp)
		}
	}
}

// Scenario basis: any money input changes it, note and sources do not.
func TestOpsScenarioBasisFollowsInputsNotProvenance(t *testing.T) {
	hs, id := newOpsCase(t, nil)
	_ = hs
	basisFor := func(ops string) string {
		var e planner.Economics
		if err := json.Unmarshal([]byte(opsEcon(ops)), &e); err != nil {
			t.Fatal(err)
		}
		c := store.Case{Economics: &e, Prediction: store.Case{}.Prediction}
		c.ID = id
		return scenarioBasis(c)
	}
	base := opsHandlerOps(`900000`, `,"lead_time_days":90`)
	b0 := basisFor(base)
	must := map[string]string{
		"labour":    strings.Replace(base, `"labour_usd_per_year":60000`, `"labour_usd_per_year":60001`, 1),
		"logistics": strings.Replace(base, `"logistics_usd_per_year":45000`, `"logistics_usd_per_year":45001`, 1),
		"capex":     strings.Replace(base, `"capex_usd":900000`, `"capex_usd":900001`, 1),
		"horizon":   strings.Replace(base, `"horizon_years":4`, `"horizon_years":5`, 1),
		"lead":      strings.Replace(base, `"lead_time_days":90`, `"lead_time_days":91`, 1),
		"basis":     strings.Replace(base, `"additional"`, `"included"`, 1),
		"ref":       strings.Replace(base, `Rayong plant`, `Rayong works`, 1),
		"zero":      strings.Replace(base, `"retooling_usd":0`, `"retooling_usd":1`, 1),
		"limit":     strings.Replace(base, `"horizon_years":4`, `"horizon_years":4,"max_lead_time_days":80`, 1),
	}
	for name, v := range must {
		if basisFor(v) == b0 {
			t.Errorf("%s edit left the basis unchanged", name)
		}
	}
	same := strings.Replace(strings.Replace(base, `"horizon_years":4`, `"horizon_years":4,"note":"x"`, 1), `"lead_time_days":90`, `"lead_time_days":90,"sources":{"lead_time_days":"call"}`, 1)
	if basisFor(same) != b0 {
		t.Error("note/sources changed the basis")
	}
	// Absent differs from an entered zero.
	if basisFor(strings.Replace(base, `,"retooling_usd":0`, ``, 1)) == b0 {
		t.Error("an entered zero and an absent field hash the same")
	}
}

// 7. Roles and audit are untouched: a viewer still gets 403 and changes
// nothing; an editor's operations save is audited like any economics save.
func TestOpsRolesAndAuditUntouched(t *testing.T) {
	e := newAuditEnv(t)
	ten := e.tenant()
	ops := opsEcon(opsHandlerOps(`900000`, ``))
	id := e.wsCase()
	before := e.snapshot()
	vera := user("vera")
	for _, rt := range []struct{ method, path, body string }{
		{"PUT", "/v1/cases/" + id + "/economics", ops},
		{"POST", "/v1/cases/" + id + "/plan", planBody(ops)},
		{"POST", "/v1/cases/" + id + "/compare-targets", `{"targets":["Thailand","Vietnam"],"economics":` + ops + `}`},
		{"POST", "/v1/cases/" + id + "/scenarios", `{"economics":` + ops + `,"scenarios":[{"label":"x","facts":"The unit is assembled in Mexico."}]}`},
	} {
		code, out := e.hs.doAs(t, rt.method, rt.path, rt.body, vera)
		if code != http.StatusForbidden || out["code"] != "forbidden" {
			t.Errorf("viewer %s %s = %d %v, want 403", rt.method, rt.path, code, out)
		}
	}
	if e.snapshot() != before {
		t.Error("a viewer's request changed a stored case")
	}
	n := len(e.events(ten))
	dave := user("dave")
	code, _ := e.hs.doAs(t, "PUT", "/v1/cases/"+id+"/economics", ops, dave)
	if code != http.StatusOK {
		t.Fatalf("editor PUT = %d", code)
	}
	in, done := pair(t, "economics with operations", e.since(ten, n), audit.CaseUpdate, "economics")
	if in.Actor != e.actor("dave@example.com") || done.Object != "case:"+id {
		t.Errorf("audit events = %+v %+v", in, done)
	}
	// An invalid operations body is refused before the intent is recorded as allowed work.
	n = len(e.events(ten))
	if code, _ := e.hs.doAs(t, "PUT", "/v1/cases/"+id+"/economics", opsEcon(`{"horizon_years":0}`), dave); code != http.StatusBadRequest {
		t.Fatalf("invalid ops = %d", code)
	}
	if got := e.since(ten, n); len(got) != 0 {
		t.Errorf("a refused body was audited as work: %s", brief(got))
	}
}

// The case's own-history summary names operations, nothing else changes in it.
func TestOpsPutHistorySummary(t *testing.T) {
	mem := store.NewMemory()
	hs, id := newOpsCase(t, func(d *Deps) { d.Cases = mem })
	hs.do(t, "PUT", "/v1/cases/"+id+"/economics", opsEcon(opsHandlerOps(`1`, ``)))
	c, _ := mem.Get(context.Background(), id)
	last := c.History[len(c.History)-1]
	if last.Kind != "economics" || !strings.HasSuffix(last.Summary, ", 1 groups costed, operations: 1 groups") {
		t.Errorf("summary = %q", last.Summary)
	}
	hs.do(t, "PUT", "/v1/cases/"+id+"/economics", `{`+opsLegacyMoney+`}`)
	c, _ = mem.Get(context.Background(), id)
	if s := c.History[len(c.History)-1].Summary; strings.Contains(s, "operations") {
		t.Errorf("legacy summary changed: %q", s)
	}
}

// 8. Compare-targets and the plan search price the same move identically.
func TestOpsCompareTargetsStampsAndPricesEachTarget(t *testing.T) {
	mem := store.NewMemory()
	hs, id := newOpsCase(t, func(d *Deps) { d.Cases = mem })
	ops := opsEcon(opsHandlerOps(`900000`, ``))
	code, out := hs.do(t, "POST", "/v1/cases/"+id+"/compare-targets", `{"targets":["Thailand","Malaysia"],"economics":`+ops+`}`)
	if code != http.StatusOK {
		t.Fatalf("compare %d %v", code, out)
	}
	c, _ := mem.Get(context.Background(), id)
	for tgt, res := range c.Targets.Results {
		if res.Objective == nil || res.EconomicsBasis != economicsHash(c.Economics) {
			t.Errorf("target %s result not stamped: %+v %q", tgt, res.Objective, res.EconomicsBasis)
		}
	}
	best := c.Targets.Ranked[0].Best
	if best == nil || best.Operations == nil || !best.Operations.Priced {
		t.Fatalf("best = %+v", best)
	}
	// A plan search for the same target with the same money gives the same move numbers.
	_, pl := hs.do(t, "POST", "/v1/cases/"+id+"/plan", planBody(ops))
	row := rowOf(t, pl["plan"].(map[string]any)["plans"], best.Move.Key())
	var fromBest any
	bb, _ := json.Marshal(best.Operations)
	_ = json.Unmarshal(bb, &fromBest)
	if row == nil || !reflect.DeepEqual(row["operations"], fromBest) {
		t.Errorf("target comparison and plan differ for %s\n%v\n%v", best.Move.Key(), row["operations"], fromBest)
	}
}

// The outer strict decode keeps its say: an invalid root field is a 400 whether
// or not operations is valid, and an invalid operations block never replaces
// the outer error. Trailing data after the first JSON value is ignored exactly
// as before for every route: this change does not make the whole body strict.
func TestOpsDecodeNeverErasesTheOuterError(t *testing.T) {
	hs, id := newOpsCase(t, nil)
	ops := opsHandlerOps(`1`, ``)
	_, body := doReq(hs, "POST", "/v1/cases/"+id+"/plan", `{"target":"Thailand","bogus_root":1,"economics":`+opsEcon(ops)+`}`, nil)
	if !strings.Contains(body, `unknown field \"bogus_root\"`) || strings.Contains(body, "operations.") {
		t.Errorf("valid operations masked or replaced the root decode error: %s", body)
	}
	code, body := doReq(hs, "POST", "/v1/cases/"+id+"/plan", `{"target":"Thailand","bogus_root":1,"economics":{"operations":{"horizon_years":"x"}}}`, nil)
	if code != http.StatusBadRequest || !strings.Contains(body, "operations.invalid_json") || !strings.Contains(body, `unknown field \"bogus_root\"`) {
		t.Errorf("both problems must be reported: %d %s", code, body)
	}
	// Legacy policy, unchanged: trailing data is not an error on the HTTP body.
	if code, _ := doReq(hs, "POST", "/v1/cases/"+id+"/plan", `{"target":"Thailand"} {"junk":1}`, nil); code != http.StatusOK {
		t.Errorf("trailing data after the body = %d; legacy decode ignores it", code)
	}
}

// Every full-case response carries the stale flags (not only GET), and a
// missing flag means fresh.
func TestOpsStaleFlagOnEveryFullCaseResponse(t *testing.T) {
	mem := store.NewMemory()
	hs := bomHarness(t, func(d *Deps) {
		eng := d.Engine.(*fakeEngine)
		d.Planner = planner.New(d.Templates, eng, goldenVerifier{}, planner.Options{PlanConfidence: 0.9, Concurrency: 1, MaxMoves: planner.DefaultMaxMoves})
		d.Cases = mem
	})
	_, c := hs.do(t, "POST", "/v1/cases/predict", heatGunBody(t))
	id := c["id"].(string)
	ops := opsHandlerOps(`900000`, ``)
	if code, out := hs.do(t, "POST", "/v1/cases/"+id+"/plan", planBody(opsEcon(ops))); code != http.StatusOK {
		t.Fatalf("plan %d %v", code, out)
	}
	_, fresh := hs.do(t, "GET", "/v1/cases/"+id, "")
	if _, ok := fresh["plan_economics_stale"]; ok {
		t.Fatal("fresh plan flagged")
	}
	putEcon(t, hs, id, opsEcon(strings.Replace(ops, `"logistics_usd_per_year":45000`, `"logistics_usd_per_year":45500`, 1)))
	flag := func(what string, m map[string]any) {
		t.Helper()
		if m["plan_economics_stale"] != true {
			t.Errorf("%s dropped plan_economics_stale", what)
		}
	}
	_, prev := hs.uploadBOM(t, id, motorInThailand(t), "", nil)
	code, out := hs.do(t, "POST", "/v1/cases/"+id+"/scenarios", `{"scenarios":[{"label":"Motor in Thailand","product":`+mustJSON(prev["product"])+`}]}`)
	if code != http.StatusOK {
		t.Fatalf("scenarios %d %v", code, out)
	}
	flag("scenarios response", out)
	code, out = hs.do(t, "PUT", "/v1/cases/"+id+"/economics", opsEcon(strings.Replace(ops, `"logistics_usd_per_year":45000`, `"logistics_usd_per_year":45600`, 1)))
	if code != http.StatusOK {
		t.Fatal("PUT")
	}
	flag("PUT economics response", out)
	// Back to the original money: fresh, and the flag is simply absent.
	code, out = hs.do(t, "PUT", "/v1/cases/"+id+"/economics", opsEcon(ops))
	if _, ok := out["plan_economics_stale"]; code != http.StatusOK || ok {
		t.Errorf("restoring the money still flags the plan: %v", out)
	}
	// A BOM change clears the plan itself; no stale flag without a plan.
	code, out = hs.uploadBOM(t, id, motorInThailand(t), "true", nil)
	if code != http.StatusOK {
		t.Fatalf("apply %d %v", code, out)
	}
	if cs := out["case"].(map[string]any); cs["plan"] != nil || cs["plan_economics_stale"] != nil {
		t.Errorf("applied BOM left a plan or a flag: plan=%v flag=%v", cs["plan"], cs["plan_economics_stale"])
	}
}

// A product edited after the operations were saved (a plant renamed) does not
// brick plan runs: the entry is read as unknown, shown as a stale ref. Saving
// operations that point at a plant the product does not have is a 400.
func TestOpsStaleRefAtPlanTimeIsUnknownNotAnError(t *testing.T) {
	mem := store.NewMemory()
	hs, id := newOpsCase(t, func(d *Deps) { d.Cases = mem })
	if code, out := hs.do(t, "PUT", "/v1/cases/"+id+"/economics", opsEcon(opsHandlerOps(`900000`, ``))); code != http.StatusOK {
		t.Fatalf("PUT %d %v", code, out)
	}
	if _, err := mem.Update(context.Background(), id, func(c *store.Case) error {
		for i := range c.Product.Plants {
			if c.Product.Plants[i].ID == "th" {
				c.Product.Plants[i].Name = "Songkhla works" // the plant at id th is now another plant
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	code, out := hs.do(t, "POST", "/v1/cases/"+id+"/plan", `{"target":"Thailand"}`)
	if code != http.StatusOK {
		t.Fatalf("plan after a product edit = %d %v; a stale ref must not brick plan runs", code, out)
	}
	row := rowOf(t, out["plan"].(map[string]any)["plans"], "motor_fan_assembly@th")
	if row == nil {
		row = rowOf(t, out["plan"].(map[string]any)["rejected"], "motor_fan_assembly@th")
	}
	o := row["operations"].(map[string]any)
	if o["priced"] != false || len(o["stale_refs"].([]any)) == 0 || row["net_priced"] != false || row["shown"] != true {
		t.Errorf("stale entry was used or the plan was demoted on it: %v", row)
	}
	// Saving the same operations again against the edited product is refused.
	if code, out := hs.do(t, "PUT", "/v1/cases/"+id+"/economics", opsEcon(opsHandlerOps(`900000`, ``))); code != http.StatusBadRequest || !strings.Contains(fmt.Sprint(out["message"]), "operations.plant_ref_mismatch") {
		t.Errorf("PUT with a stale ref = %d %v, want 400 plant_ref_mismatch", code, out["message"])
	}
}

// The stamp hashes the RAW money: a request that leaves rates to the tariff
// table is stamped from what the user entered, not from the table-filled copy
// the search ran on, so the stored case matches it and nothing is stale.
func TestOpsStampIsTakenBeforeRatesAreFilled(t *testing.T) {
	mem := store.NewMemory()
	hs, id := newOpsCase(t, func(d *Deps) { d.Cases = mem })
	econ := `{"annual_import_value_usd":2000000,"group_costs":{"motor_fan_assembly":{"cn":300000,"th":420000}},"operations":` + opsHandlerOps(`900000`, ``) + `}`
	code, out := hs.do(t, "POST", "/v1/cases/"+id+"/plan", planBody(econ))
	if code != http.StatusOK {
		t.Fatalf("plan %d %v", code, out)
	}
	c, _ := mem.Get(context.Background(), id)
	if len(c.Economics.DutyRates) != 0 {
		t.Fatalf("setup: the stored economics hold derived rates: %v", c.Economics.DutyRates)
	}
	if !jsonEq(out["plan"].(map[string]any)["economics_basis"], economicsHash(c.Economics)) {
		t.Errorf("stamp %v != hash of the stored raw economics %s", out["plan"].(map[string]any)["economics_basis"], economicsHash(c.Economics))
	}
	if planMoneyStale(c) {
		t.Error("a normal request is stale the moment it is stored")
	}
	// The search itself ran on the table-filled rates (priced, not raw).
	row := rowOf(t, out["plan"].(map[string]any)["plans"], "motor_fan_assembly@th")
	if row == nil || row["net_priced"] != true {
		t.Errorf("the table rates were not used for pricing: %v", row)
	}
	// The same through the stream.
	hs2, id2 := newOpsCase(t, func(d *Deps) { d.Cases = mem })
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/v1/cases/"+id2+"/plan", strings.NewReader(planBody(econ)))
	req.Header.Set("Accept", "text/event-stream")
	hs2.h.ServeHTTP(rec, req)
	c2, _ := mem.Get(context.Background(), id2)
	if c2.Plan == nil || planMoneyStale(c2) {
		t.Errorf("streamed plan stale or missing: %v", c2.Plan != nil)
	}
}

// F1 (financial review): operations with no annual value and no stored
// economics. The request owns its money: no spurious 409, the body is stored
// whole, the result is unpriced but its feasibility is kept, and a fresh read
// is not stale.
func TestOpsOnlyBodyWithZeroAnnualValue(t *testing.T) {
	ops := `{"max_lead_time_days":90,"groups":{"motor_fan_assembly":{"plants":{"th":{"ref":{"name":"Rayong plant","country":"Thailand"},"lead_time_days":120}}}}}`
	body := `{"target":"Thailand","economics":{"annual_import_value_usd":0,"operations":` + ops + `}}`
	for _, stream := range []bool{false, true} {
		t.Run(fmt.Sprintf("plan stream=%v", stream), func(t *testing.T) {
			mem := store.NewMemory()
			hs, id := newOpsCase(t, func(d *Deps) { d.Cases = mem })
			hdr := map[string]string{}
			if stream {
				hdr["Accept"] = "text/event-stream"
			}
			code, out := doReq(hs, "POST", "/v1/cases/"+id+"/plan", body, hdr)
			if code != http.StatusOK || strings.Contains(out, "event: error") || strings.Contains(out, `"conflict"`) {
				t.Fatalf("spurious failure: %d %.300s", code, out)
			}
			c, _ := mem.Get(context.Background(), id)
			if c.Plan == nil || c.Economics == nil || c.Economics.Operations == nil || c.Plan.EconomicsBasis != economicsHash(c.Economics) {
				t.Fatalf("not stored whole or not stamped: plan=%v econ=%+v", c.Plan != nil, c.Economics)
			}
			_, got := hs.do(t, "GET", "/v1/cases/"+id, "")
			if _, bad := got["plan_economics_stale"]; bad {
				t.Error("fresh GET flags the plan stale")
			}
			row := rowOf(t, c2any(t, c.Plan.Rejected), "motor_fan_assembly@th")
			if row == nil || row["rejection_kind"] != "operationally_infeasible" {
				t.Errorf("feasibility of an unpriced operations body was dropped: %v", row)
			}
			if o := row["operations"].(map[string]any); o["priced"] != false {
				t.Errorf("priced without an annual value: %v", o["priced"])
			}
		})
	}
	t.Run("compare-targets", func(t *testing.T) {
		mem := store.NewMemory()
		hs, id := newOpsCase(t, func(d *Deps) { d.Cases = mem })
		code, out := hs.do(t, "POST", "/v1/cases/"+id+"/compare-targets", `{"targets":["Thailand","Malaysia"],"economics":{"annual_import_value_usd":0,"operations":`+ops+`}}`)
		if code != http.StatusOK {
			t.Fatalf("%d %v", code, out)
		}
		c, _ := mem.Get(context.Background(), id)
		if c.Targets == nil || c.Economics == nil || c.Economics.Operations == nil || targetsMoneyStale(c) {
			t.Errorf("targets=%v econ=%+v", c.Targets != nil, c.Economics)
		}
	})
}

func c2any(t *testing.T, v any) any {
	t.Helper()
	b, _ := json.Marshal(v)
	var out any
	_ = json.Unmarshal(b, &out)
	return out
}

// Source-aware guard: stored money reused (value 0, no operations in the body)
// conflicts; an explicit operations body owns its money even at value 0 and
// never conflicts with itself, and replaces the stored economics whole.
func TestOpsMoneySourceGuard(t *testing.T) {
	ops := opsHandlerOps(`900000`, ``)
	opsZero := `{"annual_import_value_usd":0,"operations":{"max_lead_time_days":200,"groups":{"motor_fan_assembly":{"plants":{"th":{"ref":{"name":"Rayong plant","country":"Thailand"},"lead_time_days":30}}}}}}`
	t.Run("stored source conflicts", func(t *testing.T) {
		cs := &hookCases{Cases: store.NewMemory()}
		hs, id := newOpsCase(t, func(d *Deps) { d.Cases = cs })
		hs.do(t, "PUT", "/v1/cases/"+id+"/economics", opsEcon(ops))
		cs.before = func() {
			hs.do(t, "PUT", "/v1/cases/"+id+"/economics", opsEcon(strings.Replace(ops, `"labour_usd_per_year":60000`, `"labour_usd_per_year":61000`, 1)))
		}
		code, out := doReq(hs, "POST", "/v1/cases/"+id+"/plan", `{"target":"Thailand","economics":{"annual_import_value_usd":0}}`, nil)
		if code != http.StatusConflict {
			t.Errorf("%d %.200s", code, out)
		}
	})
	t.Run("own value-0 operations never self-conflict", func(t *testing.T) {
		for _, stream := range []bool{false, true} {
			cs := &hookCases{Cases: store.NewMemory()}
			hs, id := newOpsCase(t, func(d *Deps) { d.Cases = cs })
			hs.do(t, "PUT", "/v1/cases/"+id+"/economics", opsEcon(ops))
			cs.before = func() {
				hs.do(t, "PUT", "/v1/cases/"+id+"/economics", opsEcon(strings.Replace(ops, `"labour_usd_per_year":60000`, `"labour_usd_per_year":61000`, 1)))
			}
			hdr := map[string]string{}
			if stream {
				hdr["Accept"] = "text/event-stream"
			}
			code, out := doReq(hs, "POST", "/v1/cases/"+id+"/plan", `{"target":"Thailand","economics":`+opsZero+`}`, hdr)
			if code != http.StatusOK || strings.Contains(out, "event: error") {
				t.Fatalf("stream=%v: %d %.200s", stream, code, out)
			}
			c, _ := cs.Get(context.Background(), id)
			if c.Economics.AnnualImportValueUSD != 0 || c.Economics.Operations == nil || *c.Economics.Operations.MaxLeadTimeDays != 200 || len(c.Economics.GroupCosts) != 0 {
				t.Errorf("the request's operations did not replace the stored economics whole: %+v", c.Economics)
			}
			if planMoneyStale(c) {
				t.Errorf("stream=%v: result is immediately stale", stream)
			}
		}
	})
	t.Run("legacy value-0 body still keeps stored economics", func(t *testing.T) {
		mem := store.NewMemory()
		hs, id := newOpsCase(t, func(d *Deps) { d.Cases = mem })
		hs.do(t, "PUT", "/v1/cases/"+id+"/economics", opsEcon(ops))
		hs.do(t, "POST", "/v1/cases/"+id+"/plan", `{"target":"Thailand","economics":{"annual_import_value_usd":0,"operations":{}}}`)
		c, _ := mem.Get(context.Background(), id)
		if c.Economics.AnnualImportValueUSD != 2000000 || c.Economics.Operations == nil {
			t.Errorf("an inactive value-0 body replaced the stored economics: %+v", c.Economics)
		}
	})
}

// F3: operations-mode target ranking puts fully priced before unpriced.
func TestRankTargetsOperationsKnownBeforeUnknown(t *testing.T) {
	yes, no := true, false
	mk := func(name string, net float64, priced *bool, ops bool) store.TargetSummary {
		p := domain.Plan{NetYear: net, NetPriced: priced, Shown: true}
		if ops {
			p.Operations = &opcost.Result{}
		}
		return store.TargetSummary{Target: name, Status: TargetPlan, Best: &p}
	}
	rank := func(ts ...store.TargetSummary) []string {
		rankTargets(ts)
		var out []string
		for _, x := range ts {
			out = append(out, x.Target)
		}
		return out
	}
	if got := rank(mk("unknownHigh", 900000, &no, true), mk("knownLow", 10, &yes, true)); got[0] != "knownLow" {
		t.Errorf("unpriced fallback outranked a priced target: %v", got)
	}
	if got := rank(mk("knownZero", 0, &yes, true), mk("unknownHigh", 5, &no, true), mk("knownNeg", -5, &yes, true)); got[0] != "knownZero" || got[1] != "knownNeg" {
		t.Errorf("known zero/negative must precede unknown: %v", got)
	}
	// legacy ordering is unchanged: by net, priced or not
	if got := rank(mk("unknownHigh", 900000, &no, false), mk("knownLow", 10, &yes, false)); got[0] != "unknownHigh" {
		t.Errorf("legacy ordering changed: %v", got)
	}
}

// The body decode still reads the first JSON value only. A valid first value
// followed by more than the body limit of ignored trailing bytes is accepted,
// as on the base; a bad operations block is explained from the bytes the
// decoder actually read; the body limit still protects the first value.
func TestDecodeKeepsFirstValueStreaming(t *testing.T) {
	hs, id := newOpsCase(t, nil)
	trail := strings.Repeat(" ", maxBody+4096) // literal 256 KiB + 4 KiB of ignored whitespace, in memory only
	for _, body := range []string{
		`{"target":"Thailand"}` + trail + `{"junk":1}`,
		planBody(opsEcon(opsHandlerOps(`1`, ``))) + trail,
	} {
		if code, out := doReq(hs, "POST", "/v1/cases/"+id+"/plan", body, nil); code != http.StatusOK {
			t.Errorf("valid first value + large ignored trail = %d %.200s, want 200", code, out)
		}
	}
	// active operations with an invalid outer field: the outer error stands
	code, out := doReq(hs, "POST", "/v1/cases/"+id+"/plan", `{"target":"Thailand","bogus":1,"economics":`+opsEcon(opsHandlerOps(`1`, ``))+`}`+trail, nil)
	if code != http.StatusBadRequest || !strings.Contains(out, `unknown field \"bogus\"`) {
		t.Errorf("outer error lost: %d %.200s", code, out)
	}
	// a bad nested block next to a large trail is still explained
	code, out = doReq(hs, "POST", "/v1/cases/"+id+"/plan", `{"target":"Thailand","economics":{"operations":{"cost_bais":1}}}`+trail, nil)
	if code != http.StatusBadRequest || !strings.Contains(out, "operations.unknown_field") {
		t.Errorf("nested error not explained: %d %.200s", code, out)
	}
	// the limit still bounds the first value itself
	big := `{"target":"Thailand","economics":{"operations":{"note":"` + strings.Repeat("n", maxBody) + `"}}}`
	if code, _ := doReq(hs, "POST", "/v1/cases/"+id+"/plan", big, nil); code != http.StatusBadRequest {
		t.Errorf("an oversized first value = %d, want 400", code)
	}
}

// Scenario saves follow the same money-source rule (ownsMoney): an explicit
// active operations body at an annual value of 0 owns its money, is stored
// whole and is compared (unpriced, feasibility kept); a fresh GET keeps it.
// An inactive value-0 body still reuses the stored economics.
func TestOpsScenarioSaveFollowsMoneySource(t *testing.T) {
	opsZero := `{"annual_import_value_usd":0,"operations":{"max_lead_time_days":90,"groups":{"motor_fan_assembly":{"plants":{"th":{"ref":{"name":"Rayong plant","country":"Thailand"},"lead_time_days":120}}}}}}`
	save := func(hs harness, id string, product any, econ string) (int, map[string]any) {
		return hs.do(t, "POST", "/v1/cases/"+id+"/scenarios", `{"economics":`+econ+`,"scenarios":[{"label":"Motor in Thailand","product":`+mustJSON(product)+`}]}`)
	}
	check := func(t *testing.T, where string, c map[string]any) {
		t.Helper()
		cmp := comparisonOf(scenarioOf(t, c))
		if cmp == nil || cmp["net_usd"] != nil || cmp["feasibility"] != "infeasible" || cmp["operations"] == nil || cmp["operations"].(map[string]any)["priced"] != false {
			t.Errorf("%s: comparison = %v, want unpriced with feasibility kept", where, cmp)
		}
	}
	for _, withStored := range []bool{false, true} {
		t.Run(fmt.Sprintf("stored economics=%v", withStored), func(t *testing.T) {
			mem := store.NewMemory()
			hs := bomHarness(t, func(d *Deps) { d.Cases = mem })
			_, c := hs.do(t, "POST", "/v1/cases/predict", heatGunBody(t))
			id := c["id"].(string)
			_, prev := hs.uploadBOM(t, id, motorInThailand(t), "", nil)
			if withStored {
				putEcon(t, hs, id, econ2M)
			}
			code, out := save(hs, id, prev["product"], opsZero)
			if code != http.StatusOK {
				t.Fatalf("save %d %v", code, out)
			}
			check(t, "save response", out)
			_, got := hs.do(t, "GET", "/v1/cases/"+id, "")
			check(t, "fresh GET", got)
			stored, _ := mem.Get(context.Background(), id)
			if stored.Economics == nil || stored.Economics.Operations == nil || stored.Economics.AnnualImportValueUSD != 0 || len(stored.Economics.GroupCosts) != 0 {
				t.Errorf("the request's operations did not replace the economics whole: %+v", stored.Economics)
			}
			if s := stored.Scenarios[0]; s.Basis != scenarioBasis(stored) || s.Comparison == nil {
				t.Error("the stored comparison does not carry the stored case's basis")
			}
		})
	}
	t.Run("inactive value-0 body reuses the stored economics", func(t *testing.T) {
		mem := store.NewMemory()
		hs := bomHarness(t, func(d *Deps) { d.Cases = mem })
		id, product := moneySeed(t, hs)
		code, out := save(hs, id, product, `{"annual_import_value_usd":0,"operations":{}}`)
		if code != http.StatusOK || netOf(t, out) != 380000 {
			t.Fatalf("%d net %v", code, out)
		}
		if s, _ := mem.Get(context.Background(), id); s.Economics.AnnualImportValueUSD != 2000000 {
			t.Errorf("stored economics replaced: %+v", s.Economics)
		}
	})
}
