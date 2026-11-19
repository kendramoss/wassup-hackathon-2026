package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/engine"
)

// watchGate passes predictions through to inner and, once armed, blocks the
// next one until release is closed, so a test can change the case while a
// watch re-check is in flight.
type watchGate struct {
	inner   Predictor
	mu      sync.Mutex
	armed   bool
	entered chan struct{}
	release chan struct{}
}

func (h *watchGate) arm() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.armed, h.entered, h.release = true, make(chan struct{}), make(chan struct{})
}

func (h *watchGate) Predict(ctx context.Context, c domain.Case) (engine.Result, error) {
	h.mu.Lock()
	armed, entered, release := h.armed, h.entered, h.release
	h.armed = false
	h.mu.Unlock()
	if armed {
		close(entered)
		<-release
	}
	return h.inner.Predict(ctx, c)
}

func watchHarness(t *testing.T) (harness, *watchGate, string) {
	t.Helper()
	he := &watchGate{}
	hs := bomHarness(t, func(d *Deps) { d.Engine = he })
	he.inner = hs.engine
	_, c := hs.do(t, "POST", "/v1/cases/predict", heatGunBody(t))
	return hs, he, c["id"].(string)
}

// A watch re-check that began on the old inputs must not attach its result
// after a new BOM or answers replaced them (D97: the watch result is cleared
// because it was worked out from the old facts).
func TestWatchRecheckDiscardedAfterBOMApply(t *testing.T) {
	hs, he, id := watchHarness(t)
	he.arm()
	done := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		rec := httptest.NewRecorder()
		hs.h.ServeHTTP(rec, httptest.NewRequest("POST", "/v1/cases/"+id+"/watch", strings.NewReader(`{}`)))
		done <- rec
	}()
	<-he.entered
	if code, out := hs.uploadBOM(t, id, motorInThailand(t), "true", nil); code != http.StatusOK || out["applied"] != true {
		t.Fatalf("apply BOM %d %v", code, out)
	}
	close(he.release)
	rec := <-done
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), `"conflict"`) {
		t.Errorf("stale watch: status %d body %s, want 409 conflict", rec.Code, rec.Body.String())
	}
	_, got := hs.do(t, "GET", "/v1/cases/"+id, "")
	if got["watch"] != nil {
		t.Errorf("a watch worked out from the old BOM was attached: %v", got["watch"])
	}
}

func TestWatchRecheckDiscardedAfterAnswers(t *testing.T) {
	hs, he, id := watchHarness(t)
	he.arm()
	done := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		rec := httptest.NewRecorder()
		hs.h.ServeHTTP(rec, httptest.NewRequest("POST", "/v1/cases/"+id+"/watch", strings.NewReader(`{}`)))
		done <- rec
	}()
	<-he.entered
	if code, out := hs.do(t, "POST", "/v1/cases/"+id+"/answers", answerBody); code != http.StatusOK {
		t.Fatalf("answers %d %v", code, out)
	}
	close(he.release)
	rec := <-done
	if rec.Code != http.StatusConflict {
		t.Errorf("stale watch after answers: status %d body %s, want 409", rec.Code, rec.Body.String())
	}
	_, got := hs.do(t, "GET", "/v1/cases/"+id, "")
	if got["watch"] != nil {
		t.Errorf("a watch worked out from the old facts was attached: %v", got["watch"])
	}
}

// Control: with nothing changed in between, the watch still attaches; money
// edits are not case inputs and do not supersede it.
func TestWatchRecheckStillAttachesWhenInputsUnchanged(t *testing.T) {
	hs, he, id := watchHarness(t)
	he.arm()
	done := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		rec := httptest.NewRecorder()
		hs.h.ServeHTTP(rec, httptest.NewRequest("POST", "/v1/cases/"+id+"/watch", strings.NewReader(`{}`)))
		done <- rec
	}()
	<-he.entered
	if code, out := hs.do(t, "PUT", "/v1/cases/"+id+"/economics", `{"annual_import_value_usd":1000}`); code != http.StatusOK {
		t.Fatalf("economics %d %v", code, out)
	}
	close(he.release)
	if rec := <-done; rec.Code != http.StatusOK {
		t.Fatalf("watch %d %s", rec.Code, rec.Body.String())
	}
	_, got := hs.do(t, "GET", "/v1/cases/"+id, "")
	if got["watch"] == nil {
		t.Errorf("watch not attached when the inputs were unchanged")
	}
}
