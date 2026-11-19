package httpapi

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/engine"
)

const answerBody = `{"answers":[{"question":"Where is the gearbox cast?","answer":"In Vietnam, from Vietnamese aluminium."}]}`

func eventKinds(c map[string]any) []string {
	var out []string
	evs, _ := c["history"].([]any)
	for _, e := range evs {
		out = append(out, e.(map[string]any)["kind"].(string))
	}
	return out
}

// An answer changes the facts, so a plan, comparison, watch and export mark
// worked out from the old facts must go; history and scenarios stay, and the
// next export draws on the new facts.
func TestAnswersInvalidateFactDependentState(t *testing.T) {
	hs, id := staleHarness(t, nil)
	if code, out := hs.do(t, "POST", "/v1/cases/"+id+"/plan", `{"target":"Thailand"}`); code != http.StatusOK {
		t.Fatalf("plan %d %v", code, out)
	}
	if code, out := hs.do(t, "POST", "/v1/cases/"+id+"/compare-targets", `{"targets":["Thailand","Vietnam"]}`); code != http.StatusOK {
		t.Fatalf("compare %d %v", code, out)
	}
	if code, out := hs.do(t, "POST", "/v1/cases/"+id+"/scenarios", `{"scenarios":[{"label":"My setup","facts":"The unit is assembled in Mexico."}]}`); code != http.StatusOK {
		t.Fatalf("scenarios %d %v", code, out)
	}
	if code, out := hs.do(t, "POST", "/v1/cases/"+id+"/watch", `{}`); code != http.StatusOK {
		t.Fatalf("watch %d %v", code, out)
	}
	if code, out := hs.do(t, "POST", "/v1/cases/"+id+"/export", `{"plan_index":0}`); code != http.StatusOK {
		t.Fatalf("export %d %v", code, out)
	}
	_, before := hs.do(t, "GET", "/v1/cases/"+id, "")
	if before["plan"] == nil || before["targets"] == nil || before["watch"] == nil || before["step"] != "export" {
		t.Fatalf("setup incomplete: %v", before)
	}

	if code, out := hs.do(t, "POST", "/v1/cases/"+id+"/answers", answerBody); code != http.StatusOK {
		t.Fatalf("answers %d %v", code, out)
	}
	_, after := hs.do(t, "GET", "/v1/cases/"+id, "")
	// exported_at is not in the response; the case step shows it.
	if after["step"] == "export" {
		t.Errorf("the case is still at the export step after new facts")
	}
	for _, k := range []string{"plan", "targets", "watch"} {
		if after[k] != nil {
			t.Errorf("%s survived an answer that changed the facts: %v", k, after[k])
		}
	}
	if len(after["scenarios"].([]any)) != 1 {
		t.Errorf("user scenarios were dropped: %v", after["scenarios"])
	}
	if after["prediction"] == nil {
		t.Errorf("prediction missing after re-analysis")
	}
	if got, want := len(eventKinds(after)), len(eventKinds(before))+1; got != want {
		t.Errorf("history %v, want the earlier events kept plus one", eventKinds(after))
	}
	if k := eventKinds(after); k[len(k)-1] != "answered" {
		t.Errorf("last event %v", k)
	}

	// The old plan is gone, so its VariantFacts cannot be exported.
	if code, out := hs.do(t, "POST", "/v1/cases/"+id+"/export", `{"plan_index":0}`); code == http.StatusOK {
		t.Errorf("export of a cleared plan succeeded: %v", out)
	}
	code, out := hs.do(t, "POST", "/v1/cases/"+id+"/export", `{}`)
	if code != http.StatusOK {
		t.Fatalf("export %d %v", code, out)
	}
	md, _ := out["content"].(string)
	if !strings.Contains(md, "Vietnamese aluminium") || strings.Contains(md, "variant facts") {
		t.Errorf("export is not from the current answer:\n%s", md)
	}
}

// heldEngine blocks its first Predict after arm, so a test can change the
// case while a re-analysis is in flight.
type heldEngine struct {
	Predictor
	mu      sync.Mutex
	armed   bool
	entered chan struct{}
	release chan struct{}
}

func (h *heldEngine) Predict(ctx context.Context, c domain.Case) (engine.Result, error) {
	h.mu.Lock()
	hold := h.armed
	h.armed = false
	h.mu.Unlock()
	if hold {
		close(h.entered)
		<-h.release
	}
	return h.Predictor.Predict(ctx, c)
}

func TestAnswersReanalysisSupersededByBOMApply(t *testing.T) {
	he := &heldEngine{entered: make(chan struct{}), release: make(chan struct{})}
	hs := bomHarness(t, func(d *Deps) { he.Predictor = d.Engine; d.Engine = he })
	_, c := hs.do(t, "POST", "/v1/cases/predict", heatGunBody(t))
	id := c["id"].(string)
	he.mu.Lock()
	he.armed = true
	he.mu.Unlock()
	type res struct {
		code int
		out  map[string]any
	}
	done := make(chan res, 1)
	go func() {
		code, out := hs.do(t, "POST", "/v1/cases/"+id+"/answers", answerBody)
		done <- res{code, out}
	}()
	<-he.entered
	if code, out := hs.uploadBOM(t, id, motorInThailand(t), "true", nil); code != http.StatusOK || out["applied"] != true {
		t.Fatalf("apply BOM %d %v", code, out)
	}
	_, mid := hs.do(t, "GET", "/v1/cases/"+id, "")
	close(he.release)
	r := <-done
	if r.code != http.StatusConflict || r.out["code"] != "conflict" {
		t.Errorf("superseded answers: %d %v, want 409 conflict", r.code, r.out)
	}
	_, got := hs.do(t, "GET", "/v1/cases/"+id, "")
	if got["facts"] != mid["facts"] || got["answers"] != nil {
		t.Errorf("late answers overwrote the BOM facts: answers=%v", got["answers"])
	}
	if got["prediction"].(map[string]any)["origin"] != mid["prediction"].(map[string]any)["origin"] {
		t.Errorf("late answers overwrote the BOM prediction")
	}
	for _, k := range eventKinds(got) {
		if k == "answered" {
			t.Errorf("superseded answers were recorded in history")
		}
	}
}
