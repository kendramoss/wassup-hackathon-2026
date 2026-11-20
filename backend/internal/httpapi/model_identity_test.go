package httpapi

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"cloud.google.com/go/auth"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/blob"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/engine"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/llm"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/spend"
)

// #344: the engine's primary is the configured alias, but Vertex reports a
// resolved version (modelVersion gemini-4-argon-001). The alias must still be
// the primary for cache replay and fallback review; the resolved version stays
// a diagnostic. These tests use the real Vertex SDK against a localhost fake,
// the real WithFallback, the real engine cache and the real predict handler.

type staticTok struct{}

func (staticTok) Token(context.Context) (*auth.Token, error) {
	return &auth.Token{Value: "t", Type: "Bearer"}, nil
}

// vertexFake answers generateContent per model in the URL path. A model in
// down returns 429; version maps a model to the modelVersion it reports.
type vertexFake struct {
	mu      sync.Mutex
	calls   map[string]int
	down    map[string]bool
	version map[string]string
}

func (f *vertexFake) setDown(model string, v bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.down[model] = v
}

func (f *vertexFake) count(model string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls[model]
}

func (f *vertexFake) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	_, _ = io.Copy(io.Discard, r.Body)
	model := r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]
	model = strings.TrimSuffix(model, ":generateContent")
	f.mu.Lock()
	f.calls[model]++
	down, ver := f.down[model], f.version[model]
	f.mu.Unlock()
	if down {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = io.WriteString(w, `{"error":{"code":429,"message":"Resource exhausted","status":"RESOURCE_EXHAUSTED"}}`)
		return
	}
	_, _ = fmt.Fprintf(w, `{"modelVersion":%q,"candidates":[{"finishReason":"STOP","content":{"role":"model","parts":[{"text":"{\"origin_country\":\"China\",\"confidence\":0.97}"}]}}],"usageMetadata":{"promptTokenCount":1000,"candidatesTokenCount":100,"totalTokenCount":1100}}`, ver)
}

type identityRig struct {
	fake  *vertexFake
	hs    harness
	meter *spend.Meter
}

const (
	idPrimary  = "gemini-4-argon"
	idFallback = "gemini-3.8-flash"
)

func newIdentityRig(t *testing.T, primaryVersion string) identityRig {
	t.Helper()
	f := &vertexFake{calls: map[string]int{}, down: map[string]bool{},
		version: map[string]string{idPrimary: primaryVersion, idFallback: idFallback + "-002"}}
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)
	v, err := llm.NewVertex(context.Background(), llm.VertexOptions{
		Project: "p", Location: "global", Timeout: 5 * time.Second, BaseURL: srv.URL,
		Credentials: auth.NewCredentials(&auth.CredentialsOptions{TokenProvider: staticTok{}}),
	})
	if err != nil {
		t.Fatal(err)
	}
	meter := spend.New(spend.NewMemory(), 100)
	raw := meter.Attempts(v)
	model := llm.WithFallback(raw, raw, idFallback)
	eng := engine.NewCache(engine.New(model, nil, engine.Options{Model: idPrimary}), blob.Dir{Dir: t.TempDir()})
	hs := newHarnessWith(t, "substantial_transformation", func(d *Deps) { d.Engine = eng })
	return identityRig{fake: f, hs: hs, meter: meter}
}

func (r identityRig) predict(t *testing.T) (route, reason, modelID, requested string, cached bool) {
	t.Helper()
	code, out := r.hs.do(t, "POST", "/v1/cases/predict", heatGunBody(t))
	if code != http.StatusCreated {
		t.Fatalf("predict status %d: %v", code, out)
	}
	tr := out["triage"].(map[string]any)
	trace := out["prediction"].(map[string]any)["trace"].(map[string]any)
	s := func(m map[string]any, k string) string { v, _ := m[k].(string); return v }
	c, _ := trace["cached"].(bool)
	return s(tr, "route"), s(tr, "reason"), s(trace, "model_id"), s(trace, "requested_model"), c
}

func (r identityRig) spent(t *testing.T) float64 {
	t.Helper()
	_, total := r.meter.Over(context.Background())
	return total
}

func near(a, b float64) bool { d := a - b; return d < 1e-9 && d > -1e-9 }

// Vertex reports a version for the requested alias: still the primary. The
// repeat is a cache hit (one provider call), both are auto, and the provider's
// version stays visible in the trace.
func TestAliasResolvedToVersionIsPrimaryAndCached(t *testing.T) {
	r := newIdentityRig(t, idPrimary+"-001")
	route, reason, modelID, requested, cached := r.predict(t)
	if route != "auto" || cached || modelID != idPrimary+"-001" || requested != idPrimary {
		t.Errorf("first: route %s (%s) model_id %s requested %s cached %v", route, reason, modelID, requested, cached)
	}
	route, reason, modelID, requested, cached = r.predict(t)
	if route != "auto" || !cached || modelID != idPrimary+"-001" || requested != idPrimary {
		t.Errorf("repeat: route %s (%s) model_id %s requested %s cached %v", route, reason, modelID, requested, cached)
	}
	if p, f := r.fake.count(idPrimary), r.fake.count(idFallback); p != 1 || f != 0 {
		t.Errorf("provider calls primary %d fallback %d, want 1 and 0", p, f)
	}
	// 1000 in, 100 out at the primary's price, charged once: the cache hit is free.
	if got := r.spent(t); !near(got, 0.003) {
		t.Errorf("spend = %v, want 0.003", got)
	}
}

// Control: the provider echoes the exact configured name.
func TestExactNameIsPrimaryAndCached(t *testing.T) {
	r := newIdentityRig(t, idPrimary)
	for i := 0; i < 2; i++ {
		route, reason, modelID, _, cached := r.predict(t)
		if route != "auto" || modelID != idPrimary || cached != (i == 1) {
			t.Errorf("call %d: route %s (%s) model_id %s cached %v", i, route, reason, modelID, cached)
		}
	}
	if p, f := r.fake.count(idPrimary), r.fake.count(idFallback); p != 1 || f != 0 {
		t.Errorf("provider calls primary %d fallback %d, want 1 and 0", p, f)
	}
}

// A genuine fallback answer reviews, is not served for the primary, and does
// not block the primary once it recovers (D56).
func TestFallbackAnswerThenPrimaryRecovers(t *testing.T) {
	r := newIdentityRig(t, idPrimary+"-001")
	r.fake.setDown(idPrimary, true)
	route, reason, modelID, requested, _ := r.predict(t)
	if route != "needs_review" || !strings.Contains(reason, "fallback model") || requested != idFallback || modelID != idFallback+"-002" {
		t.Fatalf("fallback: route %s (%s) model_id %s requested %s", route, reason, modelID, requested)
	}
	if got := r.spent(t); !near(got, 0.001125) {
		t.Errorf("spend after fallback = %v, want 0.001125 (flash price, failed primary is free)", got)
	}
	r.fake.setDown(idPrimary, false)
	route, reason, modelID, requested, cached := r.predict(t)
	if route != "auto" || cached || modelID != idPrimary+"-001" || requested != idPrimary {
		t.Fatalf("recovery: route %s (%s) model_id %s requested %s cached %v", route, reason, modelID, requested, cached)
	}
	route, _, _, _, cached = r.predict(t)
	if route != "auto" || !cached {
		t.Errorf("after recovery: route %s cached %v, want the primary answer replayed", route, cached)
	}
	// primary: 1 failed + 1 recovered; no third call (the replay was cached).
	if p := r.fake.count(idPrimary); p != 2 {
		t.Errorf("primary calls = %d, want 2", p)
	}
	if got := r.spent(t); !near(got, 0.001125+0.003) {
		t.Errorf("total spend = %v, want 0.004125", got)
	}
}
