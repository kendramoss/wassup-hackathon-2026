package httpapi

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/store"
)

// The batch watch re-checks a pinned demo and reports it, but never writes
// the result onto the demo: its shown answer stays the pinned one (#954).
func TestWatchRunDoesNotWritePinnedDemo(t *testing.T) {
	mem := store.NewMemory()
	hs := newHarnessWith(t, "substantial_transformation", func(d *Deps) { d.Cases = mem })
	_, c := hs.do(t, "POST", "/v1/cases/predict", heatGunBody(t))
	id := c["id"].(string)
	if _, err := mem.Update(context.Background(), id, func(x *store.Case) error { x.Pinned = true; return nil }); err != nil {
		t.Fatal(err)
	}
	hs.engine.later = "Thailand"
	code, run := hs.doAs(t, "POST", "/v1/watch/run", "", map[string]string{"X-Admin-Token": testAdmin})
	if code != http.StatusOK || run["checked"] != 1.0 {
		t.Fatalf("watch run %d %v", code, run)
	}
	if ch := run["changed"].([]any); len(ch) != 1 || ch[0] != id {
		t.Errorf("changed = %v, want the demo reported", ch)
	}
	got, err := mem.Get(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	if got.Watch != nil {
		t.Errorf("the batch watch wrote onto a pinned demo: %+v", got.Watch)
	}
}

// A re-check answered by the fallback model (the primary engine was down)
// is refused and not saved, so an outage never shows as "Changed" (#954).
func TestWatchFallbackAnswerIsNotSaved(t *testing.T) {
	hs := newHarness(t, "substantial_transformation")
	_, c := hs.do(t, "POST", "/v1/cases/predict", heatGunBody(t))
	id := c["id"].(string)
	hs.engine.later = "Thailand"
	hs.engine.modelID = "gemini-3.8-flash"

	code, out := hs.do(t, "POST", "/v1/cases/"+id+"/watch", "")
	if code != http.StatusBadGateway || out["code"] != "model_unavailable" {
		t.Errorf("fallback re-check = %d %v, want 502 model_unavailable", code, out)
	}
	_, got := hs.do(t, "GET", "/v1/cases/"+id, "")
	if got["watch"] != nil {
		t.Errorf("a fallback re-check was saved: %v", got["watch"])
	}

	_, run := hs.doAs(t, "POST", "/v1/watch/run", "", map[string]string{"X-Admin-Token": testAdmin})
	if f, _ := run["failed"].([]any); len(f) != 1 || f[0] != id || len(run["changed"].([]any)) != 0 {
		t.Errorf("watch run = %v, want the case under failed, none changed", run)
	}
	if _, got := hs.do(t, "GET", "/v1/cases/"+id, ""); got["watch"] != nil && strings.Contains(got["watch"].(map[string]any)["status"].(string), "changed") {
		t.Errorf("the batch watch saved a fallback change: %v", got["watch"])
	}
}
