package httpapi

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/engine"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/planner"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/router"
)

// factsRecorder records every facts string the Part 102 engine reads.
type factsRecorder struct {
	*fakeEngine
	mu    sync.Mutex
	facts []string
}

func (f *factsRecorder) Predict(ctx context.Context, c domain.Case) (engine.Result, error) {
	f.mu.Lock()
	f.facts = append(f.facts, c.Facts)
	f.mu.Unlock()
	return f.fakeEngine.Predict(ctx, c)
}

// D640 (#642): on a Part 102 plan search the product's codes reach the
// engine, but the drafted ruling request's facts (VariantFacts) and the
// stored plan do not carry the "not verified by CBP" paragraph.
func TestPart102PlanDraftHasNoCodesParagraph(t *testing.T) {
	const note = "Tariff classifications given with the product"
	usmca := &factsRecorder{fakeEngine: &fakeEngine{conf: 0.97, byFacts: goldenEngine}}
	hs := newHarnessWith(t, router.RegimeUSMCA, func(d *Deps) {
		d.ExportCFR177 = true
		d.USMCA = usmca
		d.Planner = planner.New(d.Templates, d.Engine, goldenVerifier{}, planner.Options{PlanConfidence: 0.9, Concurrency: 1,
			MaxMoves: planner.DefaultMaxMoves, Engines: map[string]planner.Predictor{router.RegimeUSMCA: usmca},
			CodesInFacts: map[string]bool{router.RegimeUSMCA: true}})
	})
	var body map[string]any
	_ = json.Unmarshal([]byte(heatGunBody(t)), &body)
	prod := body["product"].(map[string]any)
	prod["hts"] = "8516.79"
	prod["components"].([]any)[0].(map[string]any)["hts"] = "8501.10"
	b, _ := json.Marshal(body)
	code, c := hs.do(t, "POST", "/v1/cases/predict", string(b))
	if code != 201 {
		t.Fatalf("predict %d %v", code, c)
	}
	id := c["id"].(string)
	if code, out := hs.do(t, "POST", "/v1/cases/"+id+"/plan", `{"target":"Thailand"}`); code != 200 {
		t.Fatalf("plan %d %v", code, out)
	}
	usmca.mu.Lock()
	sawNote := false
	for _, f := range usmca.facts {
		sawNote = sawNote || strings.Contains(f, note)
	}
	usmca.mu.Unlock()
	if !sawNote {
		t.Fatal("the Part 102 engine never read the codes paragraph: the test does not exercise D602")
	}
	_, full := hs.do(t, "GET", "/v1/cases/"+id, "")
	if raw, _ := json.Marshal(full["plan"]); strings.Contains(string(raw), note) {
		t.Error("the stored plan carries the codes paragraph")
	}
	for _, format := range []string{"markdown", "cfr177"} {
		code, prev := hs.do(t, "POST", "/v1/cases/"+id+"/export", `{"plan_index":0,"format":"`+format+`","preview":true}`)
		if code != 200 {
			t.Fatalf("%s preview %d %v", format, code, prev)
		}
		if letter, _ := prev["content"].(string); strings.Contains(letter, note) || strings.Contains(letter, "not verified by CBP") {
			t.Errorf("%s draft carries the codes paragraph:\n%s", format, letter)
		}
	}
}
