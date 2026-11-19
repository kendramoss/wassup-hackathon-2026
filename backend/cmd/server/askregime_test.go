package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/engine"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/httpapi"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/llm"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/planner"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/router"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/store"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/verifier"
)

type noSearch struct{}

func (noSearch) Search(context.Context, string, time.Time, map[string]bool) ([]verifier.Hit, error) {
	return nil, nil
}
func (noSearch) Read(context.Context, string, time.Time, map[string]bool) (string, error) {
	return "", nil
}

type labelRouter struct{ regime string }

func (r labelRouter) Classify(context.Context, string) (router.Label, error) {
	return router.Label{Route: "assembly", Subfamily: "s", Regime: r.regime, Reason: "r", ModelID: "m"}, nil
}

// regimeEngine is a fake engine that records the facts it was given.
type regimeEngine struct {
	mu      sync.Mutex
	version string
	origin  string
	conf    float64
	err     error
	facts   []string
}

func (e *regimeEngine) Predict(_ context.Context, c domain.Case) (engine.Result, error) {
	e.mu.Lock()
	e.facts = append(e.facts, c.Facts)
	e.mu.Unlock()
	if e.err != nil {
		return engine.Result{}, e.err
	}
	return engine.Result{Prediction: domain.Prediction{Origin: e.origin, Confidence: e.conf, DecisiveStep: "step", Precedents: []string{"N350001"},
		Trace: domain.Trace{ModelID: "primary", PromptVersion: e.version}}}, nil
}

func (e *regimeEngine) last() string {
	e.mu.Lock()
	defer e.mu.Unlock()
	if len(e.facts) == 0 {
		return ""
	}
	return e.facts[len(e.facts)-1]
}

func call(t *testing.T, h http.Handler, method, path, body string) map[string]any {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(method, path, strings.NewReader(body)))
	var out map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	if rec.Code >= 300 {
		t.Fatalf("%s %s = %d %s", method, path, rec.Code, rec.Body.String())
	}
	return out
}

const revised = "Knit polyester neck gaiters, knit in Vietnam from Chinese yarn; the yarn is now spun in Vietnam."

type rig struct {
	h       http.Handler
	general *regimeEngine
	spec    *regimeEngine
	model   *llm.Fake
}

// build wires the REAL withAgent path (predictor from the same Deps before the
// Agent) behind the real HTTP handler. The model first calls what_if with
// revised, then answers.
func build(t *testing.T, regime, version, origin string, specErr error, mutate func(*httpapi.Deps)) rig {
	t.Helper()
	tm, err := planner.LoadTemplates()
	if err != nil {
		t.Fatal(err)
	}
	gen := &regimeEngine{version: engine.PromptVersion, origin: "China", conf: 0.97}
	spec := &regimeEngine{version: version, origin: origin, conf: 0.97, err: specErr}
	d := httpapi.Deps{Engine: gen, Router: labelRouter{regime}, Cases: store.NewMemory(), Templates: tm,
		FastPathConf: 0.92, EngineModel: "primary", Logger: slog.New(slog.NewJSONHandler(io.Discard, nil))}
	switch regime {
	case router.RegimeTextile:
		d.Textile, d.TextileFastPath = spec, 0.9
	case router.RegimeUSMCA:
		d.USMCA, d.USMCAFastPath = spec, 0.9
	case router.RegimePharma:
		d.Pharma, d.PharmaFastPath = spec, 0.9
	}
	if mutate != nil {
		mutate(&d)
	}
	args, _ := json.Marshal(map[string]string{"change": "yarn spun in Vietnam", "facts": revised})
	step := 0
	var mu sync.Mutex
	m := &llm.Fake{Respond: func(llm.Request) (llm.Response, error) {
		mu.Lock()
		defer mu.Unlock()
		step++
		if step == 1 {
			return llm.Response{ToolCalls: []llm.ToolCall{{ID: "1", Name: "what_if", Args: args}}}, nil
		}
		return llm.Response{Text: "ok"}, nil
	}}
	d = withAgent(d, m, "m", noSearch{})
	return rig{h: httpapi.NewHandler(slog.New(slog.NewJSONHandler(io.Discard, nil)), httpapi.Info{Provider: "fake"}, d), general: gen, spec: spec, model: m}
}

// outcome of one path: what the saved scenario and the Ask what-if each report.
type pair struct{ origin, route, reason, regime string }

func (r rig) both(t *testing.T) (scenario, ask pair, askW map[string]any) {
	t.Helper()
	c := call(t, r.h, "POST", "/v1/cases/predict", `{"date":"2026-03-01","facts":"Knit polyester neck gaiters, knit in Vietnam from Chinese yarn."}`)
	id := c["id"].(string)
	sj, _ := json.Marshal(map[string]any{"scenarios": []map[string]string{{"label": "s", "facts": revised}}})
	sc := call(t, r.h, "POST", "/v1/cases/"+id+"/scenarios", string(sj))
	s := sc["scenarios"].([]any)[0].(map[string]any)
	sp, st := s["prediction"].(map[string]any), s["triage"].(map[string]any)
	tr := sp["trace"].(map[string]any)
	scenario = pair{sp["origin_country"].(string), st["route"].(string), st["reason"].(string), httpapiRegime(tr["prompt_version"].(string))}
	scenFacts := r.last()
	rep := call(t, r.h, "POST", "/v1/cases/"+id+"/ask", `{"question":"what if the yarn were spun in Vietnam?"}`)
	w := rep["reply"].(map[string]any)["what_ifs"].([]any)[0].(map[string]any)
	askW = w
	ask = pair{w["origin"].(string), asString(w["route"]), asString(w["triage_reason"]), asString(w["regime"])}
	if r.last() != scenFacts {
		t.Errorf("engine saw different facts: scenario %q, ask %q", scenFacts, r.last())
	}
	return
}

func (r rig) last() string {
	if f := r.spec.last(); f != "" {
		return f
	}
	return r.general.last()
}

func httpapiRegime(version string) string {
	return httpapi.PredictionRegime(domain.Prediction{Trace: domain.Trace{PromptVersion: version}})
}

func asString(v any) string { s, _ := v.(string); return s }

// Byte-identical revised facts get the same origin, regime and triage through
// the saved-scenario route and the Ask what-if, for every regime.
func TestAskWhatIfMatchesSavedScenario(t *testing.T) {
	for _, tc := range []struct{ name, regime, version, origin string }{
		{"usmca_part102", router.RegimeUSMCA, engine.USMCAVersion, "Mexico"},
		{"textile_102_21", router.RegimeTextile, engine.TextileVersion, "Vietnam"},
		{"pharma_api", router.RegimePharma, engine.PharmaVersion, "India"},
		{"general control", "substantial_transformation", engine.PromptVersion, "China"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := build(t, tc.regime, tc.version, tc.origin, nil, nil)
			s, a, w := r.both(t)
			if s != a {
				t.Errorf("scenario %+v != ask %+v", s, a)
			}
			if tc.regime == "substantial_transformation" {
				if a.regime != router.RegimeTextile && a.regime != "substantial_transformation" {
					t.Errorf("regime %q", a.regime)
				}
			} else if a.regime != tc.regime {
				t.Errorf("ask regime %q want %q", a.regime, tc.regime)
			}
			if w["provisional"] != nil {
				t.Errorf("healthy run marked provisional: %v", w["provisional"])
			}
		})
	}
}

// Specialist failure: Ask says provisional and needs_review, same as the scenario.
func TestAskWhatIfSpecialistFailureIsProvisional(t *testing.T) {
	for _, tc := range []struct{ name, regime, version string }{
		{"usmca_part102", router.RegimeUSMCA, engine.USMCAVersion},
		{"textile_102_21", router.RegimeTextile, engine.TextileVersion},
		{"pharma_api", router.RegimePharma, engine.PharmaVersion},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := build(t, tc.regime, tc.version, "X", errors.New("down"), nil)
			s, a, w := r.both(t)
			if s != a || a.route != "needs_review" || !strings.HasPrefix(a.reason, "specialist unavailable") {
				t.Errorf("scenario %+v ask %+v", s, a)
			}
			if p := asString(w["provisional"]); !strings.Contains(p, tc.regime) || w["specialist"] != "failed" {
				t.Errorf("provisional %q what-if %v", p, w)
			}
			if a.regime != "substantial_transformation" {
				t.Errorf("actual regime after failure = %q, want the general engine's", a.regime)
			}
		})
	}
}

// A prediction whose trace names no known engine is "unknown" in Ask; it is
// never relabelled as the router's regime or as general.
func TestAskWhatIfUnknownTraceIsUnknown(t *testing.T) {
	r := build(t, router.RegimeTextile, "mystery-v9", "Vietnam", nil, nil)
	_, _, w := r.both(t)
	if w["regime"] != "unknown" || asString(w["provisional"]) == "" {
		t.Errorf("what-if = %v", w)
	}
	for _, c := range r.model.Calls() {
		for _, m := range c.Messages {
			if m.Role == llm.RoleTool && m.Name == "what_if" && (strings.Contains(m.Content, "regime that answered: textile") || strings.Contains(m.Content, "Regime that answered: textile") || !strings.Contains(m.Content, "Regime that answered: unknown")) {
				t.Errorf("tool result %q", m.Content)
			}
		}
	}
}

// main.go builds the predictor from the Deps before Agent is set; the
// predictor therefore carries the engines, and cannot see a later Agent.
func TestWithAgentUsesDepsEnginesNotBarePredict(t *testing.T) {
	r := build(t, router.RegimePharma, engine.PharmaVersion, "India", nil, nil)
	_, _, w := r.both(t)
	if w["regime"] != router.RegimePharma || r.spec.last() == "" {
		t.Errorf("agent did not use the specialist from Deps: %v", w)
	}
}

// Provenance survives the saved chat: real HTTP Ask -> stored case -> GET.
func TestAskProvenanceSurvivesReload(t *testing.T) {
	for _, tc := range []struct {
		name, regime, version string
		specErr               error
		wantRegime, wantSpec  string
		provisional           bool
	}{
		{"healthy pharma", router.RegimePharma, engine.PharmaVersion, nil, router.RegimePharma, "ok", false},
		{"specialist failure", router.RegimeTextile, engine.TextileVersion, errors.New("down"), "substantial_transformation", "failed", true},
		{"unknown trace", router.RegimeUSMCA, "mystery-v9", nil, "unknown", "ok", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := build(t, tc.regime, tc.version, "X", tc.specErr, nil)
			c := call(t, r.h, "POST", "/v1/cases/predict", `{"date":"2026-03-01","facts":"Knit polyester neck gaiters, knit in Vietnam from Chinese yarn."}`)
			id := c["id"].(string)
			live := call(t, r.h, "POST", "/v1/cases/"+id+"/ask", `{"question":"what if?"}`)
			lw := live["reply"].(map[string]any)["what_ifs"].([]any)[0].(map[string]any)
			got := call(t, r.h, "GET", "/v1/cases/"+id, "")
			chat := got["chat"].([]any)
			if len(chat) != 1 {
				t.Fatalf("chat = %v", chat)
			}
			w := chat[0].(map[string]any)["what_ifs"].([]any)[0].(map[string]any)
			if w["regime"] != tc.wantRegime || w["specialist"] != lw["specialist"] || w["scope"] != "confirmed" || w["route"] != lw["route"] || w["triage_reason"] != lw["triage_reason"] || w["provisional"] != lw["provisional"] {
				t.Errorf("stored %v live %v", w, lw)
			}
			if tc.wantSpec != "ok" && w["specialist"] != tc.wantSpec {
				t.Errorf("specialist %v", w["specialist"])
			}
			if (w["provisional"] != nil) != tc.provisional || (tc.provisional && w["route"] != "needs_review") {
				t.Errorf("provisional/route wrong: %v", w)
			}
		})
	}
}
