package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/clarify"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/cross"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/demos"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/engine"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/evalsummary"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/llm"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/planner"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/router"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/store"
)

type fakeEngine struct {
	mu    sync.Mutex
	err   error
	conf  float64
	last  domain.Case
	later string // origin for cases dated after 2026-06-01 (watch re-checks), if set
}

func (f *fakeEngine) Predict(_ context.Context, c domain.Case) (engine.Result, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.last = c
	if f.err != nil {
		return engine.Result{}, f.err
	}
	origin := "China"
	if strings.Contains(c.Facts, "motor in Thailand") {
		origin = "Thailand"
	}
	if f.later != "" && c.Date.After(time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)) {
		origin = f.later
	}
	return engine.Result{Prediction: domain.Prediction{Origin: origin, Confidence: f.conf, KeyFactors: []string{"motor"}, CitedRulings: []string{"N350001"}, Precedents: []string{"N350001", "N350002"}}}, nil
}

type fakeRouter struct{ regime string }

func (f fakeRouter) Classify(context.Context, string) (router.Label, error) {
	return router.Label{Route: "assembly", Subfamily: "motorised", Regime: f.regime, Reason: "r"}, nil
}

type fakePlanner struct{ got planner.Request }

func (f *fakePlanner) PlanWithProgress(ctx context.Context, r planner.Request, progress func(planner.Event)) (planner.Result, error) {
	progress(planner.Event{Kind: "started", Total: 2})
	progress(planner.Event{Kind: "predicted", Move: "pcba@th", Origin: "Thailand", Total: 2})
	progress(planner.Event{Kind: "done", Done: 2, Total: 2})
	return f.Plan(ctx, r)
}

func (f *fakePlanner) Plan(_ context.Context, r planner.Request) (planner.Result, error) {
	f.got = r
	v := &domain.Verdict{PrecedentSupported: true, SupportingRuling: "N347421", CounterRuling: "N345859", Confidence: 0.96}
	return planner.Result{CaseID: r.CaseID, Target: r.Target, Plans: []domain.Plan{{Description: "Make Motor-fan assembly and PCBA (SMT) in Thailand",
		VariantFacts: "variant facts", Prediction: domain.Prediction{Origin: "Thailand", Confidence: 0.98}, Verdict: v, Shown: true}}}, nil
}

type fakeClarifier struct {
	onPoint bool
	facts   []string
}

func (f *fakeClarifier) Review(_ context.Context, facts string, _ domain.Prediction, ps []domain.Ruling) (clarify.Result, error) {
	f.facts = append(f.facts, facts)
	return clarify.Result{OnPoint: f.onPoint, Questions: []string{"Where is the EPS liner moulded?"},
		Conflict: &clarify.Conflict{RulingA: "N351809", RulingB: "N352485", DecidingDifference: "where the media is made"}}, nil
}

type fakePrecedents struct{}

func (fakePrecedents) Get(id string) (domain.Ruling, bool) {
	date := time.Date(2025, 3, 25, 0, 0, 0, 0, time.UTC)
	if id == "N350002" { // issued after the heat-gun fixture's case date
		date = time.Date(2026, 5, 4, 0, 0, 0, 0, time.UTC)
	}
	return domain.Ruling{ID: id, Date: date, Subject: "Air fryer", Origin: "Vietnam"}, true
}

type fakeRulings struct{ calls int }

func (f *fakeRulings) Ruling(_ context.Context, id string) (cross.Ruling, error) {
	f.calls++
	if id == "N000001" {
		return cross.Ruling{}, cross.ErrNotFound
	}
	return cross.Ruling{RulingNumber: id, Subject: "heat gun", RulingDate: "2026-01-29T00:00:00", Text: "Dear ..."}, nil
}

type harness struct {
	h       http.Handler
	clar    *fakeClarifier
	engine  *fakeEngine
	planner *fakePlanner
	rulings *fakeRulings
}

func newHarness(t *testing.T, regime string) harness {
	t.Helper()
	tm, err := planner.LoadTemplates()
	if err != nil {
		t.Fatal(err)
	}
	hs := harness{engine: &fakeEngine{conf: 0.97}, planner: &fakePlanner{}, rulings: &fakeRulings{}, clar: &fakeClarifier{}}
	hs.h = NewHandler(slog.New(slog.NewJSONHandler(io.Discard, nil)), Info{Provider: "fake"}, Deps{
		Engine: hs.engine, Router: fakeRouter{regime}, Planner: hs.planner, Clarifier: hs.clar, Precedents: fakePrecedents{}, Rulings: hs.rulings, Cases: store.NewMemory(), Templates: tm,
		Summary: func() (evalsummary.Summary, error) {
			return evalsummary.Summary{Status: "proxy, development only", Items: []evalsummary.Item{{Key: "final_accuracy", Value: 0.899}}}, nil
		},
		Demos: mustDemos(t), FastPathConf: 0.95, Provider: "fake", EngineModel: "gemini-4-argon", RouterModel: "gemini-3.8-flash", VerifierModel: "gemini-4-argon",
	})
	return hs
}

func (hs harness) do(t *testing.T, method, path, body string) (int, map[string]any) {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	rec := httptest.NewRecorder()
	hs.h.ServeHTTP(rec, req)
	var out map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return rec.Code, out
}

func heatGunBody(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "testdata", "heatgun_2022.json"))
	if err != nil {
		t.Fatal(err)
	}
	var fx map[string]any
	_ = json.Unmarshal(b, &fx)
	body, _ := json.Marshal(map[string]any{"date": fx["date"], "product": fx["product"], "family": fx["family"],
		"exclude_rulings": fx["exclude_rulings"], "current_origin": fx["current_origin"]})
	return string(body)
}

func TestPredictFromProductThenPlanThenExport(t *testing.T) {
	hs := newHarness(t, "substantial_transformation")
	code, out := hs.do(t, "POST", "/v1/cases/predict", heatGunBody(t))
	if code != http.StatusCreated {
		t.Fatalf("predict status %d: %v", code, out)
	}
	for _, k := range []string{"model_ids", "prompt_versions", "llm_provider", "disclaimer"} {
		if out[k] == nil {
			t.Errorf("response missing %s", k)
		}
	}
	refs, _ := out["precedents_detail"].([]any)
	if len(refs) != len(out["prediction"].(map[string]any)["precedents"].([]any)) || len(refs) == 0 {
		t.Fatalf("precedents_detail = %v", out["precedents_detail"])
	}
	if ref := refs[0].(map[string]any); ref["origin"] != "Vietnam" || ref["date"] != "2025-03-25" || ref["subject"] != "Air fryer" || ref["url"] == "" {
		t.Errorf("precedent ref = %v", ref)
	}
	if out["disclaimer"] != domain.Disclaimer {
		t.Errorf("disclaimer = %v", out["disclaimer"])
	}
	if tr := out["triage"].(map[string]any); tr["route"] != "auto" {
		t.Errorf("triage = %v", tr)
	}
	if !strings.Contains(hs.engine.last.Facts, "assembled in Thailand") || len(hs.engine.last.Exclude) != 2 {
		t.Errorf("engine got facts %q exclude %v", hs.engine.last.Facts, hs.engine.last.Exclude)
	}
	id := out["id"].(string)

	code, got := hs.do(t, "GET", "/v1/cases/"+id, "")
	if code != http.StatusOK || got["id"] != id {
		t.Fatalf("get case %d: %v", code, got)
	}

	code, pl := hs.do(t, "POST", "/v1/cases/"+id+"/plan", `{"target":"Thailand","economics":{"illustrative":true,"annual_import_value_usd":2000000,"duty_rates":{"China":0.417,"Thailand":0.167}}}`)
	if code != http.StatusOK || pl["plan"] == nil {
		t.Fatalf("plan %d: %v", code, pl)
	}
	if hs.planner.got.CurrentOrigin == nil || hs.planner.got.CurrentOrigin.Ruling != "N328954" || hs.planner.got.Target != "Thailand" {
		t.Errorf("planner request = %+v", hs.planner.got)
	}

	code, ex := hs.do(t, "POST", "/v1/cases/"+id+"/export", `{"plan_index":0}`)
	if code != http.StatusOK {
		t.Fatalf("export %d: %v", code, ex)
	}
	md := ex["content"].(string)
	if !strings.Contains(md, "make Motor-fan assembly and PCBA (SMT) in Thailand") || !strings.Contains(md, "N347421") || !strings.Contains(md, "China (N328954)") {
		t.Errorf("export content:\n%s", md)
	}
	if code, _ := hs.do(t, "POST", "/v1/cases/"+id+"/export", `{"plan_index":5}`); code != http.StatusBadRequest {
		t.Errorf("bad plan index status %d", code)
	}
	if code, ex := hs.do(t, "POST", "/v1/cases/"+id+"/export", ""); code != http.StatusOK || !strings.Contains(ex["content"].(string), "Requester's position") {
		t.Errorf("export without body: %d", code)
	}
}

func TestPredictOutOfScopeGoesToReview(t *testing.T) {
	hs := newHarness(t, "textile_102_21")
	code, out := hs.do(t, "POST", "/v1/cases/predict", `{"date":"2026-02-01","facts":"A knit sweater is knit in China and sewn in Vietnam."}`)
	if code != http.StatusCreated {
		t.Fatalf("status %d: %v", code, out)
	}
	tr := out["triage"].(map[string]any)
	if tr["route"] != "needs_review" || !strings.Contains(tr["reason"].(string), "102.21") {
		t.Errorf("triage = %v", tr)
	}
}

func TestErrors(t *testing.T) {
	hs := newHarness(t, "substantial_transformation")
	tests := []struct {
		name, method, path, body string
		status                   int
		code                     string
	}{
		{"no date", "POST", "/v1/cases/predict", `{"facts":"x"}`, 400, "invalid_input"},
		{"bad date", "POST", "/v1/cases/predict", `{"date":"15/10/2026","facts":"x"}`, 400, "invalid_input"},
		{"unknown field", "POST", "/v1/cases/predict", `{"date":"2026-01-01","facts":"x","bogus":1}`, 400, "invalid_input"},
		{"facts too long", "POST", "/v1/cases/predict", `{"date":"2026-01-01","facts":"` + strings.Repeat("a", 12001) + `"}`, 400, "invalid_input"},
		{"unknown family", "POST", "/v1/cases/predict", `{"date":"2026-01-01","family":"spaceships","product":{"name":"x","components":[{"name":"a","made_in":"China"}]}}`, 400, "invalid_input"},
		{"bad exclude id", "POST", "/v1/cases/predict", `{"date":"2026-01-01","facts":"x","exclude_rulings":["abc"]}`, 400, "invalid_input"},
		{"missing case", "GET", "/v1/cases/nope", "", 404, "not_found"},
		{"plan missing case", "POST", "/v1/cases/nope/plan", `{"target":"Thailand"}`, 404, "not_found"},
		{"bad ruling id", "GET", "/v1/rulings/abc", "", 400, "invalid_input"},
		{"unknown ruling", "GET", "/v1/rulings/N000001", "", 404, "not_found"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, out := hs.do(t, tt.method, tt.path, tt.body)
			if code != tt.status || out["code"] != tt.code {
				t.Errorf("got %d %v, want %d %s", code, out, tt.status, tt.code)
			}
		})
	}
	// Plan on a facts-only case is rejected.
	_, out := hs.do(t, "POST", "/v1/cases/predict", `{"date":"2026-01-01","facts":"x"}`)
	if code, e := hs.do(t, "POST", "/v1/cases/"+out["id"].(string)+"/plan", `{"target":"Thailand"}`); code != 400 || !strings.Contains(e["message"].(string), "structured product") {
		t.Errorf("plan on facts-only case: %d %v", code, e)
	}
}

func TestModelErrorsMapTo502(t *testing.T) {
	for _, tt := range []struct {
		err  error
		code string
	}{{llm.ErrUnavailable, "model_unavailable"}, {&llm.APIError{StatusCode: 503}, "model_unavailable"}, {llm.ErrBadResponse, "model_bad_response"}} {
		hs := newHarness(t, "substantial_transformation")
		hs.engine.err = tt.err
		code, out := hs.do(t, "POST", "/v1/cases/predict", `{"date":"2026-01-01","facts":"x"}`)
		if code != http.StatusBadGateway || out["code"] != tt.code {
			t.Errorf("%v: got %d %v", tt.err, code, out)
		}
	}
}

func TestRulingIsProxiedAndCached(t *testing.T) {
	hs := newHarness(t, "substantial_transformation")
	for i := 0; i < 2; i++ {
		code, out := hs.do(t, "GET", "/v1/rulings/N357771", "")
		if code != 200 || out["url"] != "https://rulings.cbp.gov/ruling/N357771" || out["date"] != "2026-01-29" || out["origin"] != "Vietnam" {
			t.Fatalf("ruling %d: %v", code, out)
		}
	}
	if hs.rulings.calls != 1 {
		t.Errorf("CROSS calls = %d, want 1 (cached)", hs.rulings.calls)
	}
}

func TestEvalSummary(t *testing.T) {
	hs := newHarness(t, "substantial_transformation")
	code, out := hs.do(t, "GET", "/v1/eval/summary", "")
	if code != 200 || out["status"] != "proxy, development only" {
		t.Fatalf("summary %d: %v", code, out)
	}
}

func TestBodyLimit(t *testing.T) {
	hs := newHarness(t, "substantial_transformation")
	big := bytes.Repeat([]byte("a"), maxBody+10)
	code, _ := hs.do(t, "POST", "/v1/cases/predict", `{"date":"2026-01-01","facts":"`+string(big)+`"}`)
	if code != 400 {
		t.Errorf("oversized body status %d", code)
	}
}

func mustDemos(t *testing.T) []demos.Demo {
	t.Helper()
	ds, err := demos.Load()
	if err != nil {
		t.Fatal(err)
	}
	return ds
}

func TestDemoFlowThroughTheAPI(t *testing.T) {
	hs := newHarness(t, "substantial_transformation")
	code, list := hs.do(t, "GET", "/v1/demos", "")
	if code != 200 || len(list["demos"].([]any)) != 4 {
		t.Fatalf("list demos %d: %v", code, list)
	}
	req := httptest.NewRequest("GET", "/v1/demos/heatgun-2022", nil)
	rec := httptest.NewRecorder()
	hs.h.ServeHTTP(rec, req)
	var d demos.Demo
	if err := json.Unmarshal(rec.Body.Bytes(), &d); err != nil || d.ID != "heatgun-2022" {
		t.Fatalf("get demo: %v %s", err, rec.Body.String())
	}
	code, out := hs.do(t, "POST", "/v1/cases/predict", string(d.Predict))
	if code != 201 {
		t.Fatalf("predict from demo: %d %v", code, out)
	}
	if !strings.HasSuffix(hs.engine.last.Facts, "of the heat gun,\n") || strings.HasSuffix(hs.engine.last.Facts, "\n\n") {
		t.Errorf("pasted facts should end with one newline like the benchmark corpus: %q", hs.engine.last.Facts[len(hs.engine.last.Facts)-20:])
	}
	if !strings.HasPrefix(hs.engine.last.Facts, "In your letter dated October 25, 2022") {
		t.Errorf("engine should read CBP's facts text, got %.60q", hs.engine.last.Facts)
	}
	code, pl := hs.do(t, "POST", "/v1/cases/"+out["id"].(string)+"/plan", string(d.Plan))
	if code != 200 || hs.planner.got.ActualChange == nil || hs.planner.got.Product.Name == "" {
		t.Errorf("plan from demo: %d %v %+v", code, pl, hs.planner.got.ActualChange)
	}
	if code, _ := hs.do(t, "GET", "/v1/demos/nope", ""); code != 404 {
		t.Errorf("missing demo status %d", code)
	}
}

func TestNeedsInfoThenAnswers(t *testing.T) {
	hs := newHarness(t, "substantial_transformation")
	hs.engine.conf = 0.8
	code, out := hs.do(t, "POST", "/v1/cases/predict", `{"date":"2026-01-01","facts":"A bicycle helmet: ABS shell moulded in Taiwan, assembled in China."}`)
	if code != 201 {
		t.Fatalf("predict %d %v", code, out)
	}
	if tr := out["triage"].(map[string]any); tr["route"] != "needs_info" {
		t.Errorf("triage = %v", tr)
	}
	cl := out["clarify"].(map[string]any)
	if cl["questions"].([]any)[0] != "Where is the EPS liner moulded?" || cl["conflict"] == nil {
		t.Errorf("clarify = %v", cl)
	}
	id := out["id"].(string)
	code, out = hs.do(t, "POST", "/v1/cases/"+id+"/answers", `{"answers":[{"question":"Where is the EPS liner moulded?","answer":"In Taiwan, from Taiwanese EPS beads."}]}`)
	if code != 200 {
		t.Fatalf("answers %d %v", code, out)
	}
	if !strings.Contains(hs.engine.last.Facts, "ADDITIONAL FACTS FROM THE REQUESTER") || !strings.Contains(hs.engine.last.Facts, "A: In Taiwan, from Taiwanese EPS beads.") ||
		!strings.HasPrefix(hs.engine.last.Facts, "A bicycle helmet") {
		t.Errorf("re-predicted facts = %q", hs.engine.last.Facts)
	}
	if len(out["answers"].([]any)) != 1 || out["id"] != id {
		t.Errorf("answers response %v", out)
	}
	// A second round appends, it does not duplicate the first.
	hs.do(t, "POST", "/v1/cases/"+id+"/answers", `{"answers":[{"question":"Who makes the straps?","answer":"China"}]}`)
	if strings.Count(hs.engine.last.Facts, "ADDITIONAL FACTS") != 1 || strings.Count(hs.engine.last.Facts, "Q: ") != 2 {
		t.Errorf("second round facts = %q", hs.engine.last.Facts)
	}
	if code, _ := hs.do(t, "POST", "/v1/cases/"+id+"/answers", `{"answers":[{"question":"Resin source?","answer":"Unknown"}]}`); code != 400 {
		t.Errorf("all-unknown answers status %d", code)
	}
	if code, _ := hs.do(t, "POST", "/v1/cases/"+id+"/answers", `{"answers":[]}`); code != 400 {
		t.Errorf("empty answers status %d", code)
	}
}

func TestClarifySkippedOnFastPathAndOutOfScope(t *testing.T) {
	hs := newHarness(t, "substantial_transformation")
	_, out := hs.do(t, "POST", "/v1/cases/predict", `{"date":"2026-01-01","facts":"x"}`)
	if out["clarify"] != nil || len(hs.clar.facts) != 0 {
		t.Errorf("clarify ran on an auto case: %v", out["clarify"])
	}
	hs = newHarness(t, "textile_102_21")
	hs.engine.conf = 0.5
	_, out = hs.do(t, "POST", "/v1/cases/predict", `{"date":"2026-01-01","facts":"x"}`)
	if out["clarify"] != nil || out["triage"].(map[string]any)["route"] != "needs_review" {
		t.Errorf("out-of-scope case: %v %v", out["clarify"], out["triage"])
	}
}

func TestListCases(t *testing.T) {
	hs := newHarness(t, "substantial_transformation")
	if code, out := hs.do(t, "GET", "/v1/cases", ""); code != 200 || len(out["cases"].([]any)) != 0 {
		t.Fatalf("empty list %d %v", code, out)
	}
	_, a := hs.do(t, "POST", "/v1/cases/predict", heatGunBody(t))
	time.Sleep(2 * time.Millisecond)
	_, b := hs.do(t, "POST", "/v1/cases/predict", `{"date":"2026-01-01","facts":"A pump body is cast in China and assembled in Vietnam with a Vietnamese motor."}`)
	hs.do(t, "POST", "/v1/cases/"+a["id"].(string)+"/export", "")
	_, out := hs.do(t, "GET", "/v1/cases", "")
	rows := out["cases"].([]any)
	first, second := rows[0].(map[string]any), rows[1].(map[string]any)
	if first["id"] != a["id"] || first["step"] != "export" || second["id"] != b["id"] || second["step"] != "origin" {
		t.Errorf("order/steps = %v / %v", first, second)
	}
	if first["product_name"] != "Kobalt 12.5-amp heat gun, model 5208257" || !strings.HasPrefix(second["product_name"].(string), "A pump body is cast") {
		t.Errorf("names = %v / %v", first["product_name"], second["product_name"])
	}
	if first["predicted_origin"] != "China" || first["triage_route"] != "auto" || first["confidence"] != 0.97 || first["updated_at"] == "" {
		t.Errorf("row = %v", first)
	}
}

func TestScenariosSideBySide(t *testing.T) {
	hs := newHarness(t, "substantial_transformation")
	_, c := hs.do(t, "POST", "/v1/cases/predict", heatGunBody(t))
	id := c["id"].(string)
	code, out := hs.do(t, "POST", "/v1/cases/"+id+"/scenarios", `{"scenarios":[
		{"label":"As today","facts":"Heat gun with the motor in China."},
		{"label":"Move motor","facts":"Heat gun with the motor in Thailand."}]}`)
	if code != 200 {
		t.Fatalf("scenarios %d %v", code, out)
	}
	ss := out["scenarios"].([]any)
	first, second := ss[0].(map[string]any), ss[1].(map[string]any)
	if first["label"] != "As today" || first["prediction"].(map[string]any)["origin_country"] != "China" ||
		second["prediction"].(map[string]any)["origin_country"] != "Thailand" || len(second["precedents_detail"].([]any)) != 2 {
		t.Errorf("scenarios = %v", ss)
	}
	_, list := hs.do(t, "GET", "/v1/cases", "")
	if n := list["cases"].([]any)[0].(map[string]any)["scenario_count"]; n != 2.0 {
		t.Errorf("scenario_count = %v", n)
	}
	for _, body := range []string{`{"scenarios":[]}`, `{"scenarios":[{"facts":"a"},{"facts":"b"},{"facts":"c"},{"facts":"d"}]}`,
		`{"scenarios":[{"label":"x"}]}`, `{"scenarios":[{"facts":"a","product":{"name":"p"}}]}`} {
		if code, _ := hs.do(t, "POST", "/v1/cases/"+id+"/scenarios", body); code != 400 {
			t.Errorf("bad scenarios %s: status %d", body, code)
		}
	}
}

func TestWatchRechecksWithNewerRulings(t *testing.T) {
	hs := newHarness(t, "substantial_transformation")
	_, c := hs.do(t, "POST", "/v1/cases/predict", heatGunBody(t))
	id := c["id"].(string)
	before := c["updated_at"]
	hs.engine.later = "Thailand"
	code, out := hs.do(t, "POST", "/v1/cases/"+id+"/watch", "")
	if code != 200 {
		t.Fatalf("watch %d %v", code, out)
	}
	w := out["watch"].(map[string]any)
	newer := w["newer_rulings"].([]any)
	if w["status"] != "changed" || len(newer) != 1 || newer[0].(map[string]any)["id"] != "N350002" ||
		w["prediction"].(map[string]any)["origin_country"] != "Thailand" || out["prediction"].(map[string]any)["origin_country"] != "China" {
		t.Errorf("watch = %v", w)
	}
	if out["updated_at"] != before {
		t.Errorf("watch moved updated_at: %v -> %v", before, out["updated_at"])
	}
	hs.engine.later = ""
	_, run := hs.do(t, "POST", "/v1/watch/run", "")
	if run["checked"] != 1.0 || len(run["changed"].([]any)) != 0 {
		t.Errorf("watch run = %v", run)
	}
	_, list := hs.do(t, "GET", "/v1/cases", "")
	if st := list["cases"].([]any)[0].(map[string]any)["watch_status"]; st != "unchanged" {
		t.Errorf("watch_status = %v", st)
	}
}

func TestPlanStreamsProgressThenResult(t *testing.T) {
	hs := newHarness(t, "substantial_transformation")
	_, c := hs.do(t, "POST", "/v1/cases/predict", heatGunBody(t))
	req := httptest.NewRequest("POST", "/v1/cases/"+c["id"].(string)+"/plan", strings.NewReader(`{"target":"Thailand"}`))
	req.Header.Set("Accept", "text/event-stream")
	rec := httptest.NewRecorder()
	hs.h.ServeHTTP(rec, req)
	body := rec.Body.String()
	if rec.Code != 200 || rec.Header().Get("Content-Type") != "text/event-stream" {
		t.Fatalf("status %d type %q", rec.Code, rec.Header().Get("Content-Type"))
	}
	if strings.Count(body, "event: progress\n") != 3 || !strings.Contains(body, "event: result\ndata: {") ||
		strings.Index(body, "event: result") < strings.LastIndex(body, "event: progress") {
		t.Errorf("stream = %s", body)
	}
	if !strings.Contains(body, `"plans":[`) || !strings.Contains(body, `"case_id"`) {
		t.Errorf("result event lacks the plan: %s", body)
	}
	_, got := hs.do(t, "GET", "/v1/cases/"+c["id"].(string), "")
	if got["step"] != "plan" {
		t.Errorf("streamed plan not stored: step %v", got["step"])
	}
}
