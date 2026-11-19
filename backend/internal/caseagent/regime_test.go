package caseagent

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/engine"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/llm"
)

// classifyingEngine is a fake predictor that also classifies.
type classifyingEngine struct {
	fakeEngine
	k   Classification
	err error
	n   int
}

func (c *classifyingEngine) Classify(_ context.Context, cs domain.Case) (Classification, error) {
	c.n++
	c.seen = append(c.seen, cs)
	return c.k, c.err
}

// askOnce runs one question whose model calls what_if once, then answers.
func askOnce(t *testing.T, p Predictor) (Answer, []llm.Request) {
	t.Helper()
	step := 0
	f := &llm.Fake{Respond: func(llm.Request) (llm.Response, error) {
		step++
		if step == 1 {
			return llm.Response{ToolCalls: []llm.ToolCall{{ID: "1", Name: "what_if", Args: []byte(`{"change":"c","facts":"Revised facts."}`)}}}, nil
		}
		return llm.Response{Text: "done"}, nil
	}}
	ans, err := New(f, "m", fakeSearch{}, p).Ask(context.Background(), Input{Question: "What if?", Date: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)})
	if err != nil {
		t.Fatal(err)
	}
	return ans, f.Calls()
}

func toolText(t *testing.T, calls []llm.Request) string {
	t.Helper()
	for _, c := range calls {
		for _, m := range c.Messages {
			if m.Role == llm.RoleTool && m.Name == "what_if" {
				return m.Content
			}
		}
	}
	t.Fatalf("no what_if tool message in %d model calls", len(calls))
	return ""
}

func res(version string) engine.Result {
	return engine.Result{Prediction: domain.Prediction{Origin: "Vietnam", Confidence: 0.97, DecisiveStep: "knit", Trace: domain.Trace{PromptVersion: version}}}
}

func TestWhatIfUsesClassifyAndReportsRegimeAndRoute(t *testing.T) {
	e := &classifyingEngine{k: Classification{Result: res("textile-v1"), Regime: "textile_102_21", Selected: "textile_102_21", Scope: "confirmed", Specialist: "ok",
		Route: domain.RouteAuto, Reason: "textile rules (19 CFR 102.21): confidence 0.97"}}
	ans, calls := askOnce(t, e)
	w := ans.WhatIfs[0]
	if e.n != 1 || w.Regime != "textile_102_21" || w.Route != domain.RouteAuto || w.Provisional != "" || !strings.Contains(w.Reason, "102.21") {
		t.Errorf("classify calls %d what-if %+v", e.n, w)
	}
	txt := toolText(t, calls)
	if !strings.Contains(txt, "Regime that answered: textile_102_21") || !strings.Contains(txt, "Review route: auto") || strings.Contains(txt, "PROVISIONAL") {
		t.Errorf("tool text = %q", txt)
	}
}

func TestWhatIfProvisionalStates(t *testing.T) {
	for _, tt := range []struct {
		name string
		k    Classification
		want string
	}{
		{"specialist failed", Classification{Result: res("engine-v1"), Regime: "substantial_transformation", Selected: "pharma_api", Scope: "confirmed", Specialist: "failed", Route: domain.RouteNeedsReview, Reason: "specialist unavailable"}, "pharma_api engine failed"},
		{"specialist unconfigured", Classification{Result: res("engine-v1"), Regime: "substantial_transformation", Selected: "usmca_part102", Scope: "confirmed", Specialist: "unconfigured", Route: domain.RouteNeedsReview, Reason: "out of scope"}, "usmca_part102 engine is not enabled"},
		{"router failed", Classification{Result: res("engine-v1"), Regime: "substantial_transformation", Scope: "configured_failed", Route: domain.RouteNeedsReview, Reason: "scope unavailable"}, "scope router failed"},
		{"label missing", Classification{Result: res("engine-v1"), Regime: "substantial_transformation", Scope: "label_missing", Route: domain.RouteNeedsReview, Reason: "scope unavailable"}, "not confirmed"},
		{"unknown trace", Classification{Result: res("custom"), Regime: "", Scope: "confirmed", Route: domain.RouteNeedsReview, Reason: "x"}, "unknown"},
		{"unknown trace seam said auto", Classification{Result: res("custom"), Regime: "", Scope: "confirmed", Route: domain.RouteAuto, Reason: "x"}, "unknown"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ans, calls := askOnce(t, &classifyingEngine{k: tt.k})
			w := ans.WhatIfs[0]
			txt := toolText(t, calls)
			if !strings.Contains(w.Provisional, tt.want) || !strings.Contains(txt, "PROVISIONAL") || !strings.Contains(txt, "Review route: needs_review") {
				t.Errorf("what-if %+v text %q", w, txt)
			}
			if tt.k.Regime == "" && (w.Regime != "unknown" || strings.Contains(txt, "substantial_transformation")) {
				t.Errorf("unknown trace reported as %q: %q", w.Regime, txt)
			}
		})
	}
}

func TestWhatIfBareDispatchIsNeverRegimeCorrect(t *testing.T) {
	e := &fakeEngine{}
	ans, calls := askOnce(t, e)
	w := ans.WhatIfs[0]
	txt := toolText(t, calls)
	if w.Regime != "unclassified" || w.Route != "" || w.Provisional == "" || !strings.Contains(txt, "PROVISIONAL") || strings.Contains(txt, "Review route: auto") {
		t.Errorf("bare predictor what-if %+v text %q", w, txt)
	}
}

func TestWhatIfClassifyErrorRecordsNothing(t *testing.T) {
	step := 0
	f := &llm.Fake{Respond: func(llm.Request) (llm.Response, error) {
		step++
		if step == 1 {
			return llm.Response{ToolCalls: []llm.ToolCall{{ID: "1", Name: "what_if", Args: []byte(`{"change":"c","facts":"F."}`)}}}, nil
		}
		return llm.Response{Text: "no result"}, nil
	}}
	ans, err := New(f, "m", fakeSearch{}, &classifyingEngine{err: errors.New("boom")}).Ask(context.Background(), Input{Question: "q"})
	if err == nil && len(ans.WhatIfs) != 0 {
		t.Errorf("what-if recorded on a classify error: %+v", ans.WhatIfs)
	}
}

func TestSystemPromptGivesRegimeContext(t *testing.T) {
	for _, want := range []string{"102.21", "Part 102", "pharma", "Do not assume substantial transformation", "PROVISIONAL", "unknown"} {
		if !strings.Contains(System, want) {
			t.Errorf("System lacks %q", want)
		}
	}
	if strings.Contains(System, "under CBP's substantial-transformation test.") {
		t.Error("System still frames every case as substantial transformation")
	}
	if PromptVersion == "case-agent-v2" {
		t.Error("PromptVersion not bumped")
	}
}

func TestWhatIfJSONCarriesProvenance(t *testing.T) {
	b, _ := json.Marshal(WhatIf{Origin: "X", Regime: "unknown", Route: domain.RouteNeedsReview, Provisional: "p"})
	for _, k := range []string{`"regime":"unknown"`, `"route":"needs_review"`, `"provisional":"p"`} {
		if !strings.Contains(string(b), k) {
			t.Errorf("%s missing from %s", k, b)
		}
	}
}
