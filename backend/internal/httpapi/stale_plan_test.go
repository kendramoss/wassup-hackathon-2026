package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/planner"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/store"
)

// heldPlanner blocks every plan search until release is closed, so a test
// can change the case while the search is in flight.
type heldPlanner struct {
	*fakePlanner
	entered chan struct{}
	release chan struct{}
	once    sync.Once
}

func newHeldPlanner() *heldPlanner {
	return &heldPlanner{fakePlanner: &fakePlanner{}, entered: make(chan struct{}), release: make(chan struct{})}
}

func (h *heldPlanner) Plan(ctx context.Context, r planner.Request) (planner.Result, error) {
	h.once.Do(func() { close(h.entered) })
	<-h.release
	return h.fakePlanner.Plan(ctx, r)
}

func (h *heldPlanner) PlanWithProgress(ctx context.Context, r planner.Request, progress func(planner.Event)) (planner.Result, error) {
	return h.Plan(ctx, r)
}

// hookCases runs before once, just before the first Update reaches the store.
type hookCases struct {
	store.Cases
	mu     sync.Mutex
	before func()
}

func (h *hookCases) Update(ctx context.Context, id string, fn func(*store.Case) error) (store.Case, error) {
	h.mu.Lock()
	b := h.before
	h.before = nil
	h.mu.Unlock()
	if b != nil {
		b()
	}
	return h.Cases.Update(ctx, id, fn)
}

func staleHarness(t *testing.T, hp *heldPlanner) (harness, string) {
	t.Helper()
	hs := bomHarness(t, func(d *Deps) {
		if hp != nil {
			d.Planner = hp
		}
	})
	_, c := hs.do(t, "POST", "/v1/cases/predict", heatGunBody(t))
	return hs, c["id"].(string)
}

// supersede applies a changed BOM while a search is held, then lets it finish.
func supersede(t *testing.T, hs harness, hp *heldPlanner, id string, run func() (int, string)) (int, string) {
	t.Helper()
	type res struct {
		code int
		body string
	}
	done := make(chan res, 1)
	go func() { code, body := run(); done <- res{code, body} }()
	<-hp.entered
	if code, out := hs.uploadBOM(t, id, motorInThailand(t), "true", nil); code != http.StatusOK || out["applied"] != true {
		t.Fatalf("apply BOM %d %v", code, out)
	}
	close(hp.release)
	r := <-done
	return r.code, r.body
}

func TestStalePlanJSONDiscardedAfterBOMApply(t *testing.T) {
	hp := newHeldPlanner()
	hs, id := staleHarness(t, hp)
	code, body := supersede(t, hs, hp, id, func() (int, string) {
		rec := httptest.NewRecorder()
		hs.h.ServeHTTP(rec, httptest.NewRequest("POST", "/v1/cases/"+id+"/plan", strings.NewReader(`{"target":"Thailand"}`)))
		return rec.Code, rec.Body.String()
	})
	if code != http.StatusConflict || !strings.Contains(body, `"conflict"`) {
		t.Errorf("stale plan: status %d body %s, want 409 conflict", code, body)
	}
	_, got := hs.do(t, "GET", "/v1/cases/"+id, "")
	if got["plan"] != nil {
		t.Errorf("superseded plan was attached: %v", got["plan"])
	}
}

func TestStalePlanSSEDiscardedAfterBOMApply(t *testing.T) {
	hp := newHeldPlanner()
	hs, id := staleHarness(t, hp)
	_, body := supersede(t, hs, hp, id, func() (int, string) {
		req := httptest.NewRequest("POST", "/v1/cases/"+id+"/plan", strings.NewReader(`{"target":"Thailand"}`))
		req.Header.Set("Accept", "text/event-stream")
		rec := httptest.NewRecorder()
		hs.h.ServeHTTP(rec, req)
		return rec.Code, rec.Body.String()
	})
	if !strings.Contains(body, "event: error") || strings.Contains(body, "event: done") || !strings.Contains(body, `"conflict"`) {
		t.Errorf("stale SSE plan not rejected:\n%s", body)
	}
	_, got := hs.do(t, "GET", "/v1/cases/"+id, "")
	if got["plan"] != nil {
		t.Errorf("superseded streamed plan was attached: %v", got["plan"])
	}
}

func TestStaleCompareDiscardedAfterBOMApply(t *testing.T) {
	hp := newHeldPlanner()
	hs, id := staleHarness(t, hp)
	code, body := supersede(t, hs, hp, id, func() (int, string) {
		rec := httptest.NewRecorder()
		hs.h.ServeHTTP(rec, httptest.NewRequest("POST", "/v1/cases/"+id+"/compare-targets", strings.NewReader(`{"targets":["Thailand","Vietnam"]}`)))
		return rec.Code, rec.Body.String()
	})
	if code != http.StatusConflict || !strings.Contains(body, `"conflict"`) {
		t.Errorf("stale compare: status %d body %s, want 409 conflict", code, body)
	}
	_, got := hs.do(t, "GET", "/v1/cases/"+id, "")
	if got["targets"] != nil {
		t.Errorf("superseded comparison was attached: %v", got["targets"])
	}
}

func TestStaleExportRejectedAfterBOMApply(t *testing.T) {
	hs, id := staleHarness(t, nil)
	if code, out := hs.do(t, "POST", "/v1/cases/"+id+"/plan", `{"target":"Thailand"}`); code != http.StatusOK {
		t.Fatalf("plan %d %v", code, out)
	}
	cs := &hookCases{Cases: store.NewMemory()}
	hs2 := bomHarness(t, func(d *Deps) { d.Cases = cs })
	_, c := hs2.do(t, "POST", "/v1/cases/predict", heatGunBody(t))
	id = c["id"].(string)
	hs2.do(t, "POST", "/v1/cases/"+id+"/plan", `{"target":"Thailand"}`)
	// The BOM lands after export loaded the case, before it records itself.
	cs.before = func() {
		if code, out := hs2.uploadBOM(t, id, motorInThailand(t), "true", nil); code != http.StatusOK {
			t.Errorf("apply BOM %d %v", code, out)
		}
	}
	code, out := hs2.do(t, "POST", "/v1/cases/"+id+"/export", `{"plan_index":0}`)
	if code != http.StatusConflict || out["code"] != "conflict" {
		t.Errorf("stale export: %d %v, want 409 conflict", code, out)
	}
	_, got := hs2.do(t, "GET", "/v1/cases/"+id, "")
	if got["exported_at"] != nil {
		t.Errorf("stale export was recorded: %v", got["exported_at"])
	}
}

func TestUnsupersededPlanStillAttaches(t *testing.T) {
	hs, id := staleHarness(t, nil)
	if code, out := hs.do(t, "POST", "/v1/cases/"+id+"/plan", `{"target":"Thailand"}`); code != http.StatusOK {
		t.Fatalf("plan %d %v", code, out)
	}
	if code, out := hs.do(t, "POST", "/v1/cases/"+id+"/compare-targets", `{"targets":["Thailand","Vietnam"]}`); code != http.StatusOK {
		t.Fatalf("compare %d %v", code, out)
	}
	_, got := hs.do(t, "GET", "/v1/cases/"+id, "")
	if got["plan"] == nil || got["targets"] == nil {
		t.Errorf("normal flows not stored: plan %v targets %v", got["plan"], got["targets"])
	}
	if code, out := hs.do(t, "POST", "/v1/cases/"+id+"/export", `{"plan_index":0}`); code != http.StatusOK {
		t.Errorf("export %d %v", code, out)
	}
}
