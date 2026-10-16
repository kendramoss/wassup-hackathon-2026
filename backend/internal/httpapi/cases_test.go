package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/caseagent"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/clarify"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/cross"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/demos"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/duty"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/engine"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/evalsummary"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/extract"
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
	// byProduct, if set, gives the origin from the structured product (BOM
	// versions); "" falls through to the rules below.
	byProduct func(domain.Product) string
	// byFacts, if set, gives the origin from the facts' wording (rendered or
	// CBP's), after byProduct; "" falls through.
	byFacts func(string) string
	calls   int
	modelID string // trace.model_id; empty means the primary engine
}

func (f *fakeEngine) Predict(_ context.Context, c domain.Case) (engine.Result, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.last = c
	f.calls++
	if f.err != nil {
		return engine.Result{}, f.err
	}
	origin := "China"
	if strings.Contains(c.Facts, "motor in Thailand") {
		origin = "Thailand"
	}
	if f.byProduct != nil && c.Product != nil {
		if o := f.byProduct(*c.Product); o != "" {
			origin = o
		}
	}
	if f.byFacts != nil {
		if o := f.byFacts(c.Facts); o != "" {
			origin = o
		}
	}
	if f.later != "" && c.Date.After(time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)) {
		origin = f.later
	}
	modelID := f.modelID
	if modelID == "" {
		modelID = "gemini-4-argon"
	}
	return engine.Result{Prediction: domain.Prediction{Origin: origin, Confidence: f.conf, KeyFactors: []string{"motor"}, CitedRulings: []string{"N350001"}, Precedents: []string{"N350001", "N350002"},
		Trace: domain.Trace{Provider: "fake", ModelID: modelID, PromptVersion: "engine-v1", InputTokens: 3000, OutputTokens: 200}}}, nil
}

type fakeRouter struct{ regime string }

func (f fakeRouter) Classify(context.Context, string) (router.Label, error) {
	return router.Label{Route: "assembly", Subfamily: "motorised", Regime: f.regime, Reason: "r", ModelID: "gemini-3.8-flash", InputTokens: 1000, OutputTokens: 100}, nil
}

type fakePlanner struct {
	mu  sync.Mutex
	got planner.Request
}

func (f *fakePlanner) PlanWithProgress(ctx context.Context, r planner.Request, progress func(planner.Event)) (planner.Result, error) {
	progress(planner.Event{Kind: planner.EventStarted, Target: r.Target, Moves: []planner.MoveRef{{Key: "pcba@th"}}, Total: 1})
	progress(planner.Event{Kind: planner.EventMoveStarted, Move: "pcba@th", Total: 1})
	time.Sleep(30 * time.Millisecond) // lets a keep-alive through
	progress(planner.Event{Kind: planner.EventMoveDone, Move: "pcba@th", Plan: &domain.Plan{Description: "PCBA in Thailand"}, Done: 1, Total: 1})
	res, err := f.Plan(ctx, r)
	progress(planner.Event{Kind: planner.EventDone, Result: &res, Done: 1, Total: 1})
	return res, err
}

func (f *fakePlanner) Plan(_ context.Context, r planner.Request) (planner.Result, error) {
	f.mu.Lock()
	f.got = r
	f.mu.Unlock()
	v := &domain.Verdict{PrecedentSupported: true, SupportingRuling: "N347421", CounterRuling: "N345859", Confidence: 0.96}
	below := &domain.Verdict{PrecedentSupported: true, SupportingRuling: "N347421", CounterRuling: "N345859", Confidence: 0.88}
	switch r.Target {
	case "Malaysia": // no capable plant there
		return planner.Result{CaseID: r.CaseID, Target: r.Target, Plans: []domain.Plan{}, Rejected: []domain.Plan{}}, nil
	case "Vietnam": // supported only below the bar
		return planner.Result{CaseID: r.CaseID, Target: r.Target, Plans: []domain.Plan{}, Stats: planner.Stats{MovesSearched: 3},
			Rejected: []domain.Plan{{Move: domain.Move{{Group: "pcba", ToPlant: "vn"}}, Description: "Make PCBA in Vietnam", NetYear: 900000,
				Prediction: domain.Prediction{Origin: "Vietnam"}, Verdict: below, RejectionKind: domain.RejectBelowConfidence}}}, nil
	}
	return planner.Result{CaseID: r.CaseID, Target: r.Target, Stats: planner.Stats{MovesSearched: 6, EngineInTok: 30000, EngineOutTok: 2000,
		VerifierInTok: 10000, VerifierOutTok: 1000, WallSeconds: 12.5}, Plans: []domain.Plan{{Description: "Make Motor-fan assembly and PCBA (SMT) in Thailand",
		VariantFacts: "variant facts", Prediction: domain.Prediction{Origin: "Thailand", Confidence: 0.98}, Verdict: v, Shown: true, NetYear: 320000,
		Move: domain.Move{{Group: "motor_fan_assembly", ToPlant: "th"}, {Group: "pcba", ToPlant: "th"}}}},
		Rejected: []domain.Plan{
			{Move: domain.Move{{Group: "pcba", ToPlant: "th"}}, Description: "Make PCBA in Thailand", VariantFacts: "pcba facts",
				Prediction: domain.Prediction{Origin: "Thailand", Confidence: 0.96}, Verdict: below, RejectionKind: domain.RejectBelowConfidence},
			{Move: domain.Move{{Group: "switch", ToPlant: "th"}}, Description: "Make switch in Thailand", RejectionKind: domain.RejectNoPrecedent},
		}}, nil
}

type fakeAgent struct {
	mu    sync.Mutex
	last  caseagent.Input
	hold  chan struct{} // Ask signals entry, then waits for release
	reply *caseagent.Answer
}

func (f *fakeAgent) Ask(_ context.Context, in caseagent.Input) (caseagent.Answer, error) {
	if f.hold != nil {
		f.hold <- struct{}{}
		<-f.hold
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.last = in
	if f.reply != nil {
		return *f.reply, nil
	}
	return caseagent.Answer{Answer: "Because the motor is Chinese, as in N350001.", Cited: []string{"N350001"}, WhatIfs: []caseagent.WhatIf{}, Steps: []caseagent.Step{{Tool: "get_case"}}}, nil
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
	agent   *fakeAgent
	extract *fakeExtractor
}

func newHarness(t *testing.T, regime string) harness {
	t.Helper()
	return newHarnessWith(t, regime, nil)
}

// newHarnessWith lets a test adjust the deps (limits, admin token).
func newHarnessWith(t *testing.T, regime string, adjust func(*Deps)) harness {
	t.Helper()
	tm, err := planner.LoadTemplates()
	if err != nil {
		t.Fatal(err)
	}
	hs := harness{engine: &fakeEngine{conf: 0.97}, planner: &fakePlanner{}, rulings: &fakeRulings{}, clar: &fakeClarifier{}, agent: &fakeAgent{}, extract: &fakeExtractor{}}
	d := Deps{
		Engine: hs.engine, Router: fakeRouter{regime}, Planner: hs.planner, Clarifier: hs.clar, Agent: hs.agent, Extractor: hs.extract, Precedents: fakePrecedents{}, Rulings: hs.rulings, Cases: store.NewMemory(), Templates: tm,
		Summary: func() (evalsummary.Summary, error) {
			return evalsummary.Summary{Status: "proxy, development only", Items: []evalsummary.Item{{Key: "final_accuracy", Value: 0.899}}}, nil
		},
		Demos: mustDemos(t), FastPathConf: 0.92, PlanConfidence: 0.92, Provider: "fake", EngineModel: "gemini-4-argon", RouterModel: "gemini-3.8-flash", VerifierModel: "gemini-4-argon",
		AdminToken: testAdmin, Duty: mustDuty(t),
	}
	if adjust != nil {
		adjust(&d)
	}
	hs.h = NewHandler(slog.New(slog.NewJSONHandler(io.Discard, nil)), Info{Provider: "fake"}, d)
	return hs
}

func (hs harness) do(t *testing.T, method, path, body string) (int, map[string]any) {
	t.Helper()
	return hs.doAs(t, method, path, body, nil)
}

const testAdmin = "test-admin-token-0123456789"

// doAs sends a request with extra headers (admin token, client IP).
func (hs harness) doAs(t *testing.T, method, path, body string, headers map[string]string) (int, map[string]any) {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	for k, v := range headers {
		req.Header.Set(k, v)
	}
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

func TestFallbackPredictionNeedsReview(t *testing.T) {
	const id = "projects/p/locations/us/endpoints/9"
	eng := &fakeEngine{conf: 0.99, modelID: id}
	hs := newHarnessWith(t, "substantial_transformation", func(d *Deps) { d.Engine = eng })
	code, out := hs.do(t, "POST", "/v1/cases/predict", heatGunBody(t))
	if code != http.StatusCreated {
		t.Fatalf("predict status %d: %v", code, out)
	}
	tr := out["triage"].(map[string]any)
	want := "answered by the fallback model (" + id + ") because the primary engine was unavailable"
	if tr["route"] != "needs_review" || tr["reason"] != want {
		t.Errorf("triage = %v", tr)
	}
	pred := out["prediction"].(map[string]any)
	trace := pred["trace"].(map[string]any)
	if trace["model_id"] != id {
		t.Errorf("trace.model_id = %v", trace["model_id"])
	}
	if out["clarify"] != nil {
		t.Errorf("clarify ran on a fallback answer: %v", out["clarify"])
	}
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
	// A preview renders the draft but records nothing.
	_, before := hs.do(t, "GET", "/v1/cases/"+id, "")
	if code, ex := hs.do(t, "POST", "/v1/cases/"+id+"/export", `{"plan_index":0,"preview":true}`); code != http.StatusOK || !strings.Contains(ex["content"].(string), "Requester's position") {
		t.Errorf("preview: %d", code)
	}
	_, after := hs.do(t, "GET", "/v1/cases/"+id, "")
	if len(after["history"].([]any)) != len(before["history"].([]any)) || after["updated_at"] != before["updated_at"] {
		t.Errorf("preview changed the case: history %d -> %d", len(before["history"].([]any)), len(after["history"].([]any)))
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
	// The list sorts on updated_at. Windows' wall clock ticks every ~15.6 ms,
	// so two steps closer than that get the same time and fall back to the
	// ID tiebreak (random): wait past a tick between them.
	tick := func() { time.Sleep(20 * time.Millisecond) }
	_, a := hs.do(t, "POST", "/v1/cases/predict", heatGunBody(t))
	tick()
	_, b := hs.do(t, "POST", "/v1/cases/predict", `{"date":"2026-01-01","facts":"A pump body is cast in China and assembled in Vietnam with a Vietnamese motor."}`)
	tick()
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
	if code, _ := hs.do(t, "POST", "/v1/watch/run", ""); code != http.StatusForbidden {
		t.Errorf("watch run without the admin token = %d", code)
	}
	_, run := hs.doAs(t, "POST", "/v1/watch/run", "", map[string]string{"X-Admin-Token": testAdmin})
	if run["checked"] != 1.0 || len(run["changed"].([]any)) != 0 {
		t.Errorf("watch run = %v", run)
	}
	_, list := hs.do(t, "GET", "/v1/cases", "")
	if st := list["cases"].([]any)[0].(map[string]any)["watch_status"]; st != "unchanged" {
		t.Errorf("watch_status = %v", st)
	}
}

func TestPlanStreamsProgressThenResult(t *testing.T) {
	keepAlive = 10 * time.Millisecond
	t.Cleanup(func() { keepAlive = 10 * time.Second })
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
	order := []string{"event: start\n", "event: move_started\n", ": keep-alive\n", "event: move_done\n", "event: done\n"}
	at := 0
	for _, o := range order {
		i := strings.Index(body[at:], o)
		if i < 0 {
			t.Fatalf("stream missing %q after offset %d:\n%s", o, at, body)
		}
		at += i
	}
	if !strings.Contains(body, `"moves":[{"key":"pcba@th"`) || !strings.Contains(body, `"plan":{"move"`) {
		t.Errorf("stream payloads = %s", body)
	}
	if !strings.Contains(body, `"plans":[`) || !strings.Contains(body, `"case_id"`) {
		t.Errorf("result event lacks the plan: %s", body)
	}
	_, got := hs.do(t, "GET", "/v1/cases/"+c["id"].(string), "")
	if got["step"] != "plan" {
		t.Errorf("streamed plan not stored: step %v", got["step"])
	}
}

func TestExportBelowBarPlanByMoveKey(t *testing.T) {
	hs := newHarness(t, "substantial_transformation")
	_, c := hs.do(t, "POST", "/v1/cases/predict", heatGunBody(t))
	id := c["id"].(string)
	if code, _ := hs.do(t, "POST", "/v1/cases/"+id+"/export", `{"move_key":"pcba@th"}`); code != 400 {
		t.Errorf("export before plan search: %d", code)
	}
	hs.do(t, "POST", "/v1/cases/"+id+"/plan", `{"target":"Thailand"}`)
	code, ex := hs.do(t, "POST", "/v1/cases/"+id+"/export", `{"move_key":"pcba@th"}`)
	md, _ := ex["content"].(string)
	if code != 200 || !strings.Contains(md, "rated this change 0.88, under its 0.92 bar") || !strings.Contains(md, "make PCBA in Thailand") || !strings.Contains(md, "N347421") {
		t.Fatalf("below-bar export %d:\n%s", code, md)
	}
	for body, want := range map[string]int{
		`{"move_key":"switch@th"}`:              400, // no supporting precedent
		`{"move_key":"pcba@cn"}`:                400, // not searched
		`{"move_key":"pcba@th","plan_index":0}`: 400,
	} {
		if code, _ := hs.do(t, "POST", "/v1/cases/"+id+"/export", body); code != want {
			t.Errorf("export %s: %d, want %d", body, code, want)
		}
	}
	if _, ex := hs.do(t, "POST", "/v1/cases/"+id+"/export", `{"plan_index":0}`); strings.Contains(ex["content"].(string), "Below the confidence bar") {
		t.Error("shown plan draft carries the below-bar notice")
	}
}

func TestScenariosPricedWithEconomics(t *testing.T) {
	hs := newHarness(t, "substantial_transformation")
	_, c := hs.do(t, "POST", "/v1/cases/predict", heatGunBody(t))
	id := c["id"].(string)
	body := `{"economics":{"illustrative":true,"annual_import_value_usd":2000000,"duty_rates":{"China":0.417,"Thailand":0.167}},
		"scenarios":[{"label":"As today","facts":"Heat gun with the motor in China."},{"label":"Move motor","facts":"Heat gun with the motor in Thailand."}]}`
	code, out := hs.do(t, "POST", "/v1/cases/"+id+"/scenarios", body)
	if code != 200 {
		t.Fatalf("scenarios %d %v", code, out)
	}
	ss := out["scenarios"].([]any)
	today := ss[0].(map[string]any)["comparison"].(map[string]any)
	moved := ss[1].(map[string]any)["comparison"].(map[string]any)
	if today["duty_savings_usd"] != 0.0 || moved["duty_savings_usd"] != 500000.0 || moved["annual_duty_usd"] != 334000.0 ||
		moved["added_cost_usd"] != nil || moved["illustrative"] != true {
		t.Errorf("comparisons = %v / %v", today, moved)
	}
	// Economics are kept on the case and reused when a later call omits them.
	_, again := hs.do(t, "POST", "/v1/cases/"+id+"/scenarios", `{"scenarios":[{"label":"Move motor","facts":"Heat gun with the motor in Thailand."}]}`)
	if cmp := again["scenarios"].([]any)[0].(map[string]any)["comparison"].(map[string]any); cmp["duty_savings_usd"] != 500000.0 {
		t.Errorf("stored economics not reused: %v", cmp)
	}
}

func TestImportProductFromBOM(t *testing.T) {
	hs := newHarness(t, "substantial_transformation")
	csv := "Part,Made in,Factory,Operation group,Plant capabilities\n" +
		"main housing,Thailand,Rayong plant,housing_moulding,motor_fan_assembly\n" +
		"electric motor,China,China suppliers,motor_fan_assembly,\n"
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, _ := mw.CreateFormFile("file", "bom.csv")
	_, _ = fw.Write([]byte(csv))
	_ = mw.WriteField("name", "Heat gun")
	_ = mw.WriteField("family", "motorised_tools_small_appliances")
	_ = mw.Close()
	req := httptest.NewRequest("POST", "/v1/products/import", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	rec := httptest.NewRecorder()
	hs.h.ServeHTTP(rec, req)
	var out map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	if rec.Code != 200 {
		t.Fatalf("import %d %v", rec.Code, out)
	}
	p := out["product"].(map[string]any)
	if p["name"] != "Heat gun" || len(p["components"].([]any)) != 2 || len(p["plants"].([]any)) != 2 || out["warnings"] == nil {
		t.Errorf("import = %v", out)
	}
	// The imported product is accepted by predict as is.
	body, _ := json.Marshal(map[string]any{"date": "2026-01-14", "product": p, "family": "motorised_tools_small_appliances"})
	if code, c := hs.do(t, "POST", "/v1/cases/predict", string(body)); code != 201 {
		t.Errorf("predict with imported product: %d %v", code, c)
	}
	for _, bad := range []string{`{"name":"x","csv":"Part,Factory\nmotor,A\n"}`, `{"name":"x","family":"nope","csv":"Part,Country\nm,China\n"}`} {
		if code, _ := hs.do(t, "POST", "/v1/products/import", bad); code != 400 {
			t.Errorf("bad import %s: %d", bad, code)
		}
	}
	if code, out := hs.do(t, "POST", "/v1/products/import", `{"name":"Fan","csv":"Part,Country\nmotor,China\n"}`); code != 200 || len(out["warnings"].([]any)) == 0 {
		t.Errorf("json import %d %v", code, out)
	}
}

func TestCompareTargetsRanksCountries(t *testing.T) {
	hs := newHarness(t, "substantial_transformation")
	_, c := hs.do(t, "POST", "/v1/cases/predict", heatGunBody(t))
	id := c["id"].(string)
	code, out := hs.do(t, "POST", "/v1/cases/"+id+"/compare-targets", `{"targets":["Malaysia","vietnam","Thailand"]}`)
	if code != 200 {
		t.Fatalf("compare %d %v", code, out)
	}
	ranked := out["targets"].([]any)
	var got []string
	for _, x := range ranked {
		m := x.(map[string]any)
		got = append(got, fmt.Sprintf("%v:%v:%v", m["rank"], m["target"], m["status"]))
	}
	// A shown plan beats a bigger below-bar one; no plant comes last.
	if strings.Join(got, " ") != "1:Thailand:plan 2:Vietnam:below_bar 3:Malaysia:no_plant" {
		t.Errorf("ranking = %v", got)
	}
	code, ex := hs.do(t, "POST", "/v1/cases/"+id+"/export", `{"move_key":"pcba@vn","target":"Vietnam"}`)
	if code != 200 || !strings.Contains(ex["content"].(string), "under its 0.92 bar") {
		t.Errorf("draft from comparison %d %v", code, ex)
	}
	if code, _ := hs.do(t, "POST", "/v1/cases/"+id+"/export", `{"move_key":"pcba@vn","target":"Laos"}`); code != 400 {
		t.Errorf("unknown target draft %d", code)
	}
	for _, bad := range []string{`{"targets":["Thailand"]}`, `{"targets":["Thailand","thailand"]}`, `{"targets":["A","B","C","D","E"]}`} {
		if code, _ := hs.do(t, "POST", "/v1/cases/"+id+"/compare-targets", bad); code != 400 {
			t.Errorf("bad compare %s: %d", bad, code)
		}
	}
	_, got2 := hs.do(t, "GET", "/v1/cases/"+id, "")
	if tg, _ := got2["targets"].(map[string]any); tg == nil || len(tg["ranked"].([]any)) != 3 || tg["results"] != nil {
		t.Errorf("case targets field = %v", got2["targets"])
	}
}

func TestAskKeepsConversation(t *testing.T) {
	hs := newHarness(t, "substantial_transformation")
	_, c := hs.do(t, "POST", "/v1/cases/predict", heatGunBody(t))
	id := c["id"].(string)
	code, out := hs.do(t, "POST", "/v1/cases/"+id+"/ask", `{"question":"Why China?"}`)
	if code != 200 || out["reply"].(map[string]any)["answer"] == "" || len(out["chat"].([]any)) != 1 || len(out["cited_detail"].([]any)) != 1 {
		t.Fatalf("ask %d %v", code, out)
	}
	if !strings.Contains(hs.agent.last.CaseSummary, "PREDICTION: origin China") || len(hs.agent.last.History) != 0 {
		t.Errorf("first ask input = %+v", hs.agent.last)
	}
	hs.do(t, "POST", "/v1/cases/"+id+"/ask", `{"question":"And Thailand?"}`)
	if len(hs.agent.last.History) != 1 || hs.agent.last.History[0].Question != "Why China?" {
		t.Errorf("history = %+v", hs.agent.last.History)
	}
	if _, got := hs.do(t, "GET", "/v1/cases/"+id, ""); len(got["chat"].([]any)) != 2 {
		t.Errorf("chat on case = %v", got["chat"])
	}
	if code, out := hs.do(t, "POST", "/v1/cases/"+id+"/ask", `{"reset":true}`); code != 200 || len(out["chat"].([]any)) != 0 {
		t.Errorf("reset %d %v", code, out)
	}
	if code, _ := hs.do(t, "POST", "/v1/cases/"+id+"/ask", `{"question":""}`); code != 400 {
		t.Errorf("empty question %d", code)
	}
}

// An export that finishes while Ask is still in the model must still be on
// the case when the answer is saved (#111).
func TestAskDoesNotClobberExport(t *testing.T) {
	hs := newHarness(t, "substantial_transformation")
	_, c := hs.do(t, "POST", "/v1/cases/predict", heatGunBody(t))
	id := c["id"].(string)
	hs.agent.hold = make(chan struct{})
	type result struct {
		code int
		out  map[string]any
	}
	ch := make(chan result, 1)
	go func() {
		code, out := hs.do(t, "POST", "/v1/cases/"+id+"/ask", `{"question":"Why China?"}`)
		ch <- result{code, out}
	}()
	<-hs.agent.hold
	if code, ex := hs.do(t, "POST", "/v1/cases/"+id+"/export", ""); code != 200 {
		t.Fatalf("export during ask %d %v", code, ex)
	}
	hs.agent.hold <- struct{}{}
	got := <-ch
	if got.code != 200 || len(got.out["chat"].([]any)) != 1 {
		t.Fatalf("ask %d %v", got.code, got.out)
	}
	_, after := hs.do(t, "GET", "/v1/cases/"+id, "")
	chat, _ := after["chat"].([]any)
	if after["step"] != "export" || len(chat) != 1 {
		t.Fatalf("case after both: step %v chat %v", after["step"], after["chat"])
	}
}

func TestAskStoresWhatIfsStepsAndCitedDetail(t *testing.T) {
	hs := newHarness(t, "substantial_transformation")
	_, c := hs.do(t, "POST", "/v1/cases/predict", heatGunBody(t))
	id := c["id"].(string)
	hs.agent.reply = &caseagent.Answer{
		Answer: "Moving the motor would make it Thailand.",
		Cited:  []string{"N350009"},
		WhatIfs: []caseagent.WhatIf{{
			Change: "motor in Thailand", Origin: "Thailand", Confidence: 0.91,
			Decisive: "The motor is made in Thailand.", Facts: "Heat gun; motor made in Thailand.",
		}},
		Steps: []caseagent.Step{{Tool: "what_if", Arg: "motor in Thailand"}},
	}
	code, out := hs.do(t, "POST", "/v1/cases/"+id+"/ask", `{"question":"What if the motor were made in Thailand?"}`)
	if code != 200 {
		t.Fatalf("ask %d %v", code, out)
	}
	turn := out["chat"].([]any)[0].(map[string]any)
	w := turn["what_ifs"].([]any)[0].(map[string]any)
	step := turn["steps"].([]any)[0].(map[string]any)
	if w["facts"] != "Heat gun; motor made in Thailand." || w["origin"] != "Thailand" || step["tool"] != "what_if" {
		t.Fatalf("stored turn = %v", turn)
	}
	_, got := hs.do(t, "GET", "/v1/cases/"+id, "")
	again := got["chat"].([]any)[0].(map[string]any)
	if again["what_ifs"].([]any)[0].(map[string]any)["facts"] != "Heat gun; motor made in Thailand." {
		t.Fatalf("reloaded turn = %v", again)
	}
	var cited bool
	for _, d := range got["cited_detail"].([]any) {
		m := d.(map[string]any)
		if m["id"] == "N350009" && m["subject"] == "Air fryer" {
			cited = true
		}
	}
	if !cited {
		t.Errorf("cited_detail = %v", got["cited_detail"])
	}
	if got["updated_at"] != c["updated_at"] {
		t.Errorf("ask moved updated_at from %v to %v", c["updated_at"], got["updated_at"])
	}
}

func TestRankTargetsBelowBarByConfidenceThenNet(t *testing.T) {
	plan := func(conf, net float64) *domain.Plan {
		return &domain.Plan{NetYear: net, Verdict: &domain.Verdict{Confidence: conf}}
	}
	ts := []store.TargetSummary{
		{Target: "Malaysia", Status: TargetBelowBar, Best: plan(0.65, 455000)},
		{Target: "Vietnam", Status: TargetBelowBar, Best: plan(0.85, 300000)},
		{Target: "Laos", Status: TargetNoPlant},
		{Target: "Thailand", Status: TargetBelowBar, Best: plan(0.65, 440000)},
	}
	rankTargets(ts)
	var got []string
	for _, x := range ts {
		got = append(got, x.Target)
	}
	if strings.Join(got, ",") != "Vietnam,Malaysia,Thailand,Laos" || ts[0].Rank != 1 || ts[3].Rank != 4 {
		t.Errorf("ranking = %v", got)
	}
}

func TestCaseHistoryRecordsEachStep(t *testing.T) {
	hs := newHarness(t, "substantial_transformation")
	_, c := hs.do(t, "POST", "/v1/cases/predict", heatGunBody(t))
	id := c["id"].(string)
	hs.do(t, "POST", "/v1/cases/"+id+"/plan", `{"target":"Thailand"}`)
	hs.do(t, "POST", "/v1/cases/"+id+"/scenarios", `{"scenarios":[{"label":"A","facts":"motor in China"}]}`)
	hs.do(t, "POST", "/v1/cases/"+id+"/watch", "")
	hs.do(t, "POST", "/v1/cases/"+id+"/ask", `{"question":"Why?"}`)
	hs.do(t, "POST", "/v1/cases/"+id+"/export", `{"move_key":"pcba@th"}`)
	_, got := hs.do(t, "GET", "/v1/cases/"+id, "")
	var kinds []string
	for _, e := range got["history"].([]any) {
		kinds = append(kinds, e.(map[string]any)["kind"].(string))
	}
	if strings.Join(kinds, ",") != "predicted,plan_searched,scenarios,watched,asked,exported" {
		t.Errorf("history kinds = %v", kinds)
	}
	last := got["history"].([]any)[5].(map[string]any)
	if !strings.Contains(last["summary"].(string), "below the bar, check 0.88") || last["at"] == "" {
		t.Errorf("export event = %v", last)
	}
}

type fakeExtractor struct{ tm *planner.Template }

func (f *fakeExtractor) Extract(_ context.Context, facts, name string, tm *planner.Template) (extract.Draft, error) {
	f.tm = tm
	return extract.Draft{Product: domain.Product{Name: "Heat gun"}, Warnings: []string{"The text says 24 components; it names 15."}}, nil
}

func TestExtractProductEndpoint(t *testing.T) {
	hs := newHarness(t, "substantial_transformation")
	fx := hs.extract
	code, out := hs.do(t, "POST", "/v1/products/extract", `{"facts":"Each heat gun is assembled in Thailand.","family":"motorised_tools_small_appliances"}`)
	if code != 200 || out["product"].(map[string]any)["name"] != "Heat gun" || len(out["warnings"].([]any)) != 1 || fx.tm == nil {
		t.Fatalf("extract %d %v", code, out)
	}
	for _, bad := range []string{`{"facts":" "}`, `{"facts":"x","family":"nope"}`} {
		if code, _ := hs.do(t, "POST", "/v1/products/extract", bad); code != 400 {
			t.Errorf("bad extract %s: %d", bad, code)
		}
	}
}

func TestEconomicsOnPredictAndPut(t *testing.T) {
	hs := newHarness(t, "substantial_transformation")
	b, _ := os.ReadFile(filepath.Join("..", "..", "testdata", "heatgun_2022.json"))
	var fx map[string]json.RawMessage
	_ = json.Unmarshal(b, &fx)
	var body map[string]any
	_ = json.Unmarshal([]byte(heatGunBody(t)), &body)
	body["economics"] = fx["economics"]
	raw, _ := json.Marshal(body)
	code, c := hs.do(t, "POST", "/v1/cases/predict", string(raw))
	if code != 201 {
		t.Fatalf("predict %d %v", code, c)
	}
	id := c["id"].(string)
	var want, got any
	_ = json.Unmarshal(fx["economics"], &want)
	got = c["economics"]
	if fmt.Sprint(want) != fmt.Sprint(got) {
		t.Errorf("economics did not round-trip:\n want %v\n got  %v", want, got)
	}
	// A plan call without economics uses the case's.
	hs.do(t, "POST", "/v1/cases/"+id+"/plan", `{"target":"Thailand"}`)
	if e := hs.planner.got.Economics; e.AnnualImportValueUSD != 2000000 || e.DutyRates["China"] != 0.417 {
		t.Errorf("plan got economics %+v", e)
	}
	code, out := hs.do(t, "PUT", "/v1/cases/"+id+"/economics", `{"illustrative":true,"annual_import_value_usd":1000000,"duty_rates":{"China":0.417,"Thailand":0.167}}`)
	if code != 200 || out["economics"].(map[string]any)["annual_import_value_usd"] != 1000000.0 {
		t.Fatalf("put %d %v", code, out)
	}
	_, got2 := hs.do(t, "GET", "/v1/cases/"+id, "")
	h := got2["history"].([]any)
	if got2["economics"].(map[string]any)["annual_import_value_usd"] != 1000000.0 || h[len(h)-1].(map[string]any)["kind"] != "economics" {
		t.Errorf("case after put: %v / %v", got2["economics"], h[len(h)-1])
	}
	for _, bad := range []string{`{"duty_rates":{"China":41.7}}`, `{"annual_import_value_usd":-1}`, `{"group_costs":{"pcba":{"th":-5}}}`} {
		if code, _ := hs.do(t, "PUT", "/v1/cases/"+id+"/economics", bad); code != 400 {
			t.Errorf("bad economics %s: %d", bad, code)
		}
	}
	bad := strings.Replace(string(raw), `"China":0.417`, `"China":41.7`, 1)
	if code, _ := hs.do(t, "POST", "/v1/cases/predict", bad); code != 400 {
		t.Errorf("predict with a percent rate: %d", code)
	}
}

func mustDuty(t *testing.T) *duty.Table {
	t.Helper()
	tb, err := duty.Load()
	if err != nil {
		t.Fatal(err)
	}
	return tb
}
