package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/planner"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/store"
)

// countingPlanner counts every planner and estimator call.
type countingPlanner struct {
	*fakePlanner
	calls atomic.Int32
}

func (c *countingPlanner) Plan(ctx context.Context, r planner.Request) (planner.Result, error) {
	c.calls.Add(1)
	return c.fakePlanner.Plan(ctx, r)
}

func (c *countingPlanner) PlanWithProgress(ctx context.Context, r planner.Request, p func(planner.Event)) (planner.Result, error) {
	c.calls.Add(1)
	return c.fakePlanner.PlanWithProgress(ctx, r, p)
}

func (c *countingPlanner) Estimate(_ context.Context, r planner.Request) (planner.Estimate, error) {
	c.calls.Add(1)
	return planner.Estimate{Target: r.Target}, nil
}

// countingCases counts store mutations.
type countingCases struct {
	store.Cases
	writes atomic.Int32
}

func (c *countingCases) Put(ctx context.Context, cs store.Case) error {
	c.writes.Add(1)
	return c.Cases.Put(ctx, cs)
}

func (c *countingCases) Update(ctx context.Context, id string, fn func(*store.Case) error) (store.Case, error) {
	c.writes.Add(1)
	return c.Cases.Update(ctx, id, fn)
}

func (c *countingCases) Delete(ctx context.Context, id string) error {
	c.writes.Add(1)
	return c.Cases.Delete(ctx, id)
}

type econHarness struct {
	harness
	cp    *countingPlanner
	cases *countingCases
	id    string
}

func newEconHarness(t *testing.T) econHarness {
	t.Helper()
	eh := econHarness{cp: &countingPlanner{fakePlanner: &fakePlanner{}}, cases: &countingCases{Cases: store.NewMemory()}}
	eh.harness = newHarnessWith(t, "substantial_transformation", func(d *Deps) { d.Planner, d.Cases = eh.cp, eh.cases })
	_, c := eh.do(t, "POST", "/v1/cases/predict", heatGunBody(t))
	eh.id = c["id"].(string)
	return eh
}

// endpoints builds each direct-economics call with the given economics JSON.
var econEndpoints = []struct {
	name, path string
	body       func(econ string) string
}{
	{"plan", "/plan", func(e string) string { return `{"target":"Thailand","economics":` + e + `}` }},
	{"plan-stream", "/plan", func(e string) string { return `{"target":"Thailand","economics":` + e + `}` }},
	{"estimate", "/plan/estimate", func(e string) string { return `{"target":"Thailand","economics":` + e + `}` }},
	{"scenarios", "/scenarios", func(e string) string {
		return `{"scenarios":[{"label":"x","facts":"Heat gun with the motor in Thailand."}],"economics":` + e + `}`
	}},
	{"compare-targets", "/compare-targets", func(e string) string { return `{"targets":["Thailand","Vietnam"],"economics":` + e + `}` }},
}

// post sends a raw request; the plan-stream endpoint asks for server-sent events.
func (eh econHarness) post(t *testing.T, name, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest("POST", "/v1/cases/"+eh.id+path, strings.NewReader(body))
	if name == "plan-stream" {
		req.Header.Set("Accept", "text/event-stream")
	}
	rec := httptest.NewRecorder()
	eh.h.ServeHTTP(rec, req)
	return rec
}

func (eh econHarness) expect400(t *testing.T, name, path, body string) {
	t.Helper()
	engineBefore, writesBefore := eh.engine.calls, eh.cases.writes.Load()
	rec := eh.post(t, name, path, body)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), `"invalid_input"`) {
		t.Fatalf("%s: status %d body %s, want 400 invalid_input", name, rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); strings.HasPrefix(ct, "text/event-stream") {
		t.Errorf("%s: early 400 must be plain JSON, not an opened stream (%s)", name, ct)
	}
	if n := eh.cp.calls.Load(); n != 0 {
		t.Errorf("%s: %d planner calls on invalid economics", name, n)
	}
	if eh.engine.calls != engineBefore {
		t.Errorf("%s: engine called on invalid economics", name)
	}
	if w := eh.cases.writes.Load(); w != writesBefore {
		t.Errorf("%s: %d store mutations on invalid economics", name, w-writesBefore)
	}
}

func TestEconomicsInvalidRefusedBeforeWork(t *testing.T) {
	classes := map[string]string{
		"negative annual":     `{"annual_import_value_usd":-5,"duty_rates":{"China":0.1}}`,
		"negative group cost": `{"annual_import_value_usd":1000000,"group_costs":{"pcba":{"th":-1}}}`,
		"negative cost, zero annual (ignored field still refused)": `{"annual_import_value_usd":0,"group_costs":{"pcba":{"th":-1}}}`,
		"rate above one":   `{"annual_import_value_usd":1000000,"duty_rates":{"China":25}}`,
		"rate below zero":  `{"annual_import_value_usd":1000000,"duty_rates":{"China":-0.1}}`,
		"zero annual rate": `{"annual_import_value_usd":0,"duty_rates":{"China":1.5}}`,
	}
	for _, ep := range econEndpoints {
		for cls, econ := range classes {
			t.Run(ep.name+"/"+cls, func(t *testing.T) {
				eh := newEconHarness(t)
				eh.expect400(t, ep.name, ep.path, ep.body(econ))
			})
		}
	}
}

func TestEconomicsNonFiniteAndOversizedJSONRefused(t *testing.T) {
	// Go's JSON parser already refuses NaN/Infinity tokens and out-of-range
	// numbers; the refusal must precede any work, as a 400.
	for _, econ := range []string{
		`{"annual_import_value_usd":NaN}`, `{"annual_import_value_usd":Infinity}`,
		`{"annual_import_value_usd":1e999}`, `{"duty_rates":{"China":-1e999}}`,
		`{"annual_import_value_usd":"1000"}`, `{"group_costs":{"pcba":{"th":1e999}}}`,
	} {
		for _, ep := range econEndpoints {
			t.Run(ep.name+"/"+econ, func(t *testing.T) {
				eh := newEconHarness(t)
				eh.expect400(t, ep.name, ep.path, ep.body(econ))
			})
		}
	}
}

func TestValidateEconomicsNonFinite(t *testing.T) {
	zero := 0.0
	inf, nan := 1/zero, zero/zero
	for name, e := range map[string]planner.Economics{
		"annual NaN":  {AnnualImportValueUSD: nan},
		"annual +Inf": {AnnualImportValueUSD: inf},
		"rate NaN":    {DutyRates: map[string]float64{"China": nan}},
		"rate Inf":    {DutyRates: map[string]float64{"China": inf}},
		"cost NaN":    {GroupCosts: map[string]map[string]float64{"g": {"p": nan}}},
		"cost +Inf":   {GroupCosts: map[string]map[string]float64{"g": {"p": inf}}},
	} {
		if validateEconomics(e) == nil {
			t.Errorf("%s accepted", name)
		}
	}
	if err := validateEconomics(planner.Economics{AnnualImportValueUSD: 1, DutyRates: map[string]float64{"China": 1, "Vietnam": 0}, GroupCosts: map[string]map[string]float64{"g": {"p": 0}}}); err != nil {
		t.Errorf("boundary values refused: %v", err)
	}
}

func TestEconomicsUnsupportedCountryKeyIsNotAnEconomicsError(t *testing.T) {
	// The contract validates values, not country names: an unrecognised key
	// with a valid rate keeps working (no loosening, no new rule).
	eh := newEconHarness(t)
	rec := eh.post(t, "estimate", "/plan/estimate", `{"target":"Thailand","economics":{"annual_import_value_usd":1000,"duty_rates":{"Atlantis":0.1}}}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d %s", rec.Code, rec.Body.String())
	}
}

// legacyInvalid stores economics that putEconomics would now refuse.
func (eh econHarness) legacyInvalid(t *testing.T, e planner.Economics) {
	t.Helper()
	if _, err := eh.cases.Cases.Update(context.Background(), eh.id, func(c *store.Case) error { c.Economics = &e; return nil }); err != nil {
		t.Fatal(err)
	}
}

func TestEconomicsLegacyInvalidStoredRefusedWhenReused(t *testing.T) {
	legacy := []planner.Economics{
		{AnnualImportValueUSD: -10},
		{AnnualImportValueUSD: 1000, DutyRates: map[string]float64{"China": 7}},
		{AnnualImportValueUSD: 1000, GroupCosts: map[string]map[string]float64{"pcba": {"th": -3}}},
	}
	for _, ep := range econEndpoints {
		for i, e := range legacy {
			t.Run(ep.name+"/"+string(rune('a'+i)), func(t *testing.T) {
				eh := newEconHarness(t)
				eh.legacyInvalid(t, e)
				// omitted economics: the stored ones are the effective ones
				body := map[string]string{
					"/plan": `{"target":"Thailand"}`, "/plan/estimate": `{"target":"Thailand"}`,
					"/scenarios":       `{"scenarios":[{"label":"x","facts":"Heat gun with the motor in Thailand."}]}`,
					"/compare-targets": `{"targets":["Thailand","Vietnam"]}`,
				}[ep.path]
				writes := eh.cases.writes.Load()
				rec := eh.post(t, ep.name, ep.path, body)
				if rec.Code != http.StatusBadRequest || strings.HasPrefix(rec.Header().Get("Content-Type"), "text/event-stream") {
					t.Fatalf("status %d %s", rec.Code, rec.Body.String())
				}
				if eh.cp.calls.Load() != 0 || eh.cases.writes.Load() != writes {
					t.Errorf("work or mutation on invalid stored economics")
				}
			})
		}
	}
}

func TestEconomicsLegacyInvalidStoredIgnoredWhenReplacedByValid(t *testing.T) {
	// A valid supplied annual value replaces the stored economics, so the
	// invalid legacy ones are not effective and the call proceeds.
	for _, ep := range econEndpoints {
		t.Run(ep.name, func(t *testing.T) {
			eh := newEconHarness(t)
			eh.legacyInvalid(t, planner.Economics{AnnualImportValueUSD: -10})
			rec := eh.post(t, ep.name, ep.path, ep.body(`{"annual_import_value_usd":500000,"duty_rates":{"China":0.2}}`))
			if rec.Code != http.StatusOK {
				t.Fatalf("status %d %s", rec.Code, rec.Body.String())
			}
		})
	}
}

func TestEconomicsValidControlsUnchanged(t *testing.T) {
	for _, ep := range econEndpoints {
		t.Run(ep.name+"/supplied", func(t *testing.T) {
			eh := newEconHarness(t)
			rec := eh.post(t, ep.name, ep.path, ep.body(`{"annual_import_value_usd":500000,"duty_rates":{"China":0.25,"Thailand":0},"group_costs":{"pcba":{"th":0}}}`))
			ran := eh.cp.calls.Load() > 0 || (ep.name == "scenarios" && eh.engine.calls > 1) // scenarios use the engine, not the planner
			if rec.Code != http.StatusOK || !ran {
				t.Fatalf("status %d ran %v %s", rec.Code, ran, rec.Body.String())
			}
		})
		t.Run(ep.name+"/omitted-reuses-stored", func(t *testing.T) {
			eh := newEconHarness(t)
			if code, out := eh.do(t, "PUT", "/v1/cases/"+eh.id+"/economics", `{"annual_import_value_usd":900000,"duty_rates":{"China":0.3}}`); code != 200 {
				t.Fatalf("put %d %v", code, out)
			}
			rec := eh.post(t, ep.name, ep.path, ep.body(`{}`))
			if rec.Code != http.StatusOK {
				t.Fatalf("status %d %s", rec.Code, rec.Body.String())
			}
		})
	}
	// the reused value really is what the planner receives
	eh := newEconHarness(t)
	eh.do(t, "PUT", "/v1/cases/"+eh.id+"/economics", `{"annual_import_value_usd":900000,"duty_rates":{"China":0.3}}`)
	if rec := eh.post(t, "plan", "/plan", `{"target":"Thailand"}`); rec.Code != 200 {
		t.Fatalf("plan %d", rec.Code)
	}
	if got := eh.cp.got.Economics.AnnualImportValueUSD; got != 900000 {
		t.Errorf("planner saw annual %v, want the stored 900000", got)
	}
}
