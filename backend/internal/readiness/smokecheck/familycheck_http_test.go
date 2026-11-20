package smokecheck

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/config"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/engine"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/familycheck"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/httpapi"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/llm"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/planner"
)

const familyPath = "/v1/products/family-check"

// stubEngine only satisfies Deps.Engine (the handler serves no routes without
// it). The family-check route never calls it.
type stubEngine struct{ calls int }

func (s *stubEngine) Predict(context.Context, domain.Case) (engine.Result, error) {
	s.calls++
	return engine.Result{}, errors.New("smokecheck: the engine must not be called")
}

type rig struct {
	url       string
	model     *llm.Fake
	ids       []string
	client    *http.Client
	engine    *stubEngine
	templates map[string]planner.Template
}

// newRig serves the real handler with the real familycheck.Checker over a fake
// model, on 127.0.0.1 only. The client has no proxy and refuses any host that
// is not the loopback listener, so the test cannot reach another machine.
func newRig(t *testing.T, respond func(llm.Request) (llm.Response, error), withFamily bool) *rig {
	t.Helper()
	tm, err := planner.LoadTemplates()
	if err != nil {
		t.Fatal(err)
	}
	fake := &llm.Fake{Respond: respond}
	eng := &stubEngine{}
	d := httpapi.Deps{Engine: eng, Templates: tm, Provider: "fake", Logger: slog.New(slog.NewJSONHandler(io.Discard, nil))}
	if withFamily {
		d.Family = familycheck.New(fake, "smoke-synthetic-model", tm)
	}
	srv := httptest.NewServer(httpapi.NewHandler(d.Logger, httpapi.Info{Provider: "fake"}, d))
	t.Cleanup(srv.Close)
	host := strings.TrimPrefix(srv.URL, "http://")
	if h, _, err := net.SplitHostPort(host); err != nil || net.ParseIP(h) == nil || !net.ParseIP(h).IsLoopback() {
		t.Fatalf("test server is not on loopback: %q", srv.URL)
	}
	tr := &http.Transport{
		Proxy: nil,
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			if addr != host {
				return nil, errors.New("smokecheck: refused non-harness address " + addr)
			}
			return (&net.Dialer{}).DialContext(ctx, network, addr)
		},
		DisableKeepAlives: true,
	}
	t.Cleanup(tr.CloseIdleConnections)
	var ids []string
	for id := range tm {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	return &rig{url: srv.URL, model: fake, ids: ids, engine: eng, templates: tm,
		client: &http.Client{Transport: tr, Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
}

func (r *rig) post(t *testing.T, body string) (int, map[string]any, string) {
	t.Helper()
	resp, err := r.client.Post(r.url+familyPath, "application/json", bytes.NewBufferString(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("status %d body is not JSON: %q", resp.StatusCode, raw)
	}
	return resp.StatusCode, out, string(raw)
}

func say(text string) func(llm.Request) (llm.Response, error) {
	return func(llm.Request) (llm.Response, error) { return llm.Response{Text: text}, nil }
}

const synthetic = `{"text":"synthetic solar garden lamp, smoke test","product":{"name":"Synthetic lamp","components":[{"name":"LED board","made_in":"China"}],"plants":[],"operations":[]}}`

// assertFailClosed: a non-200 answer is the documented error body (code and
// message, nothing else), carries no family result, and never echoes an
// invented family.
func assertFailClosed(t *testing.T, code int, out map[string]any, raw, invented string) {
	t.Helper()
	if code == http.StatusOK {
		t.Fatalf("status 200, want a failure: %s", raw)
	}
	if _, ok := out["result"]; ok {
		t.Errorf("failure carries a result: %s", raw)
	}
	for k := range out {
		if k != "code" && k != "message" && k != "details" {
			t.Errorf("unexpected key %q in error body %s", k, raw)
		}
	}
	if c, _ := out["code"].(string); c == "" {
		t.Errorf("error body has no code: %s", raw)
	}
	if m, _ := out["message"].(string); m == "" {
		t.Errorf("error body has no message: %s", raw)
	}
	if invented != "" && strings.Contains(raw, invented) {
		t.Errorf("error body echoes the invented value %q: %s", invented, raw)
	}
}

func TestSupportedFamily(t *testing.T) {
	r := newRig(t, nil, true)
	for _, id := range r.ids {
		id := id
		r.model.Respond = say(`{"family":"` + id + `","confidence":0.93,"reason":"synthetic","product_name":"Synthetic lamp","typed_unclear":false}`)
		code, out, raw := r.post(t, synthetic)
		res, _ := out["result"].(map[string]any)
		if code != 200 || res["family"] != id || res["name"] != r.templates[id].Name || out["prompt_version"] != familycheck.PromptVersion || out["llm_provider"] != "fake" {
			t.Fatalf("family %s: %d %s", id, code, raw)
		}
	}
	if len(r.ids) == 0 {
		t.Fatal("no templates loaded")
	}
	// Confidence outside [0,1] is clamped, not passed through.
	r.model.Respond = say(`{"family":"` + r.ids[0] + `","confidence":7,"reason":"x"}`)
	_, out, raw := r.post(t, synthetic)
	if c := out["result"].(map[string]any)["confidence"]; c != 1.0 {
		t.Errorf("confidence not clamped: %s", raw)
	}
}

func TestUnsupportedIsNone(t *testing.T) {
	r := newRig(t, say(`{"family":"none","confidence":0.8,"reason":"no template fits","typed_unclear":false}`), true)
	code, out, raw := r.post(t, `{"text":"synthetic sailing yacht hull"}`)
	res, _ := out["result"].(map[string]any)
	if code != 200 || res["family"] != familycheck.None || res["name"] != nil {
		t.Fatalf("none: %d %s", code, raw)
	}
}

func TestSchemaSentToModelListsOnlyKnownFamiliesAndNone(t *testing.T) {
	r := newRig(t, say(`{"family":"none","confidence":0.1,"reason":"x"}`), true)
	if code, _, raw := r.post(t, synthetic); code != 200 {
		t.Fatal(raw)
	}
	calls := r.model.Calls()
	if len(calls) != 1 {
		t.Fatalf("model calls = %d, want 1", len(calls))
	}
	req := calls[0]
	if req.Model != "smoke-synthetic-model" || req.MaxOutputTokens <= 0 || req.SchemaName != "family" {
		t.Errorf("request = model %q cap %d schema %q", req.Model, req.MaxOutputTokens, req.SchemaName)
	}
	props, _ := req.JSONSchema["properties"].(map[string]any)
	fam, _ := props["family"].(map[string]any)
	enum, _ := fam["enum"].([]string)
	want := append(slices.Clone(r.ids), familycheck.None)
	if !slices.Equal(enum, want) {
		t.Errorf("schema enum = %v, want %v", enum, want)
	}
	if req := req.JSONSchema["required"]; !slices.Equal(req.([]string), []string{"family", "confidence", "reason"}) {
		t.Errorf("required = %v", req)
	}
}

func TestInventedFamilyAndInvalidEnumAreRejected(t *testing.T) {
	for _, bad := range []string{"quantum_toaster", "", "None", "NONE", "led_lights ", "LED_LIGHTS", "../etc"} {
		r := newRig(t, say(`{"family":`+jsonString(bad)+`,"confidence":0.99,"reason":"invented"}`), true)
		code, out, raw := r.post(t, synthetic)
		if code != http.StatusBadGateway || out["code"] != "model_bad_response" {
			t.Errorf("family %q: %d %s", bad, code, raw)
		}
		assertFailClosed(t, code, out, raw, strings.TrimSpace(bad))
		if n := len(r.model.Calls()); n != 1 {
			t.Errorf("family %q: model calls = %d, want 1 (no retry)", bad, n)
		}
	}
}

func TestMalformedModelOutputFailsClosed(t *testing.T) {
	for name, text := range map[string]string{
		"empty":          ``,
		"prose":          `I think it is a lamp`,
		"truncated":      `{"family":"led_lig`,
		"wrong type":     `{"family":42,"confidence":0.5,"reason":"x"}`,
		"confidence str": `{"family":"none","confidence":"high","reason":"x"}`,
		"array":          `["none"]`,
		"empty object":   `{}`,
	} {
		r := newRig(t, say(text), true)
		code, out, raw := r.post(t, synthetic)
		if code != http.StatusBadGateway || out["code"] != "model_bad_response" {
			t.Errorf("%s: %d %s", name, code, raw)
		}
		assertFailClosed(t, code, out, raw, "")
		if n := len(r.model.Calls()); n != 1 {
			t.Errorf("%s: model calls = %d, want 1", name, n)
		}
	}
}

func TestProviderErrorsFailClosedWithoutRetry(t *testing.T) {
	for name, perr := range map[string]error{
		"http 500":    &llm.APIError{Provider: "fake", StatusCode: 500, Body: "SECRET-LOOKING-UPSTREAM-BODY"},
		"http 429":    &llm.APIError{Provider: "fake", StatusCode: 429, Body: "quota"},
		"unavailable": llm.ErrUnavailable,
		"deadline":    context.DeadlineExceeded,
	} {
		r := newRig(t, func(llm.Request) (llm.Response, error) { return llm.Response{}, perr }, true)
		code, out, raw := r.post(t, synthetic)
		if code != http.StatusBadGateway || out["code"] != "model_unavailable" {
			t.Errorf("%s: %d %s", name, code, raw)
		}
		assertFailClosed(t, code, out, raw, "SECRET-LOOKING-UPSTREAM-BODY")
		if n := len(r.model.Calls()); n != 1 {
			t.Errorf("%s: model calls = %d, want exactly 1", name, n)
		}
	}
	// An unknown error is an opaque 500, not a family.
	r := newRig(t, func(llm.Request) (llm.Response, error) { return llm.Response{}, errors.New("boom internal detail") }, true)
	code, out, raw := r.post(t, synthetic)
	assertFailClosed(t, code, out, raw, "boom internal detail")
	if code != http.StatusInternalServerError {
		t.Errorf("unknown error = %d", code)
	}
}

func TestMalformedRequestsNeverReachTheModel(t *testing.T) {
	r := newRig(t, say(`{"family":"none","confidence":1,"reason":"x"}`), true)
	for name, body := range map[string]string{
		"not json":       `{`,
		"empty body":     ``,
		"unknown field":  `{"text":"x","admin":true}`,
		"empty text":     `{"text":"   "}`,
		"missing text":   `{}`,
		"wrong type":     `{"text":5}`,
		"too long":       `{"text":"` + strings.Repeat("a", 2001) + `"}`,
		"trailing array": `[]`,
	} {
		code, out, raw := r.post(t, body)
		if code != http.StatusBadRequest || out["code"] != "invalid_input" {
			t.Errorf("%s: %d %s", name, code, raw)
		}
		assertFailClosed(t, code, out, raw, "")
	}
	if n := len(r.model.Calls()); n != 0 {
		t.Errorf("model called %d times for invalid requests", n)
	}
	if r.engine.calls != 0 {
		t.Error("engine was called")
	}
}

func TestRouteAbsentWhenCheckerNotConfigured(t *testing.T) {
	r := newRig(t, say(`{}`), false)
	code, out, raw := r.post(t, synthetic)
	if code != http.StatusNotFound {
		t.Fatalf("%d %s", code, raw)
	}
	assertFailClosed(t, code, out, raw, "")
	if len(r.model.Calls()) != 0 {
		t.Error("model called")
	}
}

func TestHealthReportsFakeProviderAndHarnessIsLoopbackOnly(t *testing.T) {
	r := newRig(t, say(`{}`), true)
	resp, err := r.client.Get(r.url + "/health")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var h map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&h)
	if resp.StatusCode != 200 || h["llm_provider"] != "fake" {
		t.Errorf("health = %d %v", resp.StatusCode, h)
	}
	if _, err := r.client.Get("http://example.invalid/health"); err == nil || !strings.Contains(err.Error(), "refused non-harness address") {
		t.Errorf("client reached outside the harness: %v", err)
	}
}

// The production selector still rejects the gateway (D49): the offline smoke
// and the eval-only adapter must not widen it.
func TestProductionSelectorStillRejectsLiteLLM(t *testing.T) {
	env := map[string]string{"GOOGLE_CLOUD_PROJECT": "wassup-hackathon-2026", "LLM_PROVIDER": "litellm"}
	_, err := config.FromEnv(func(k string) string { return env[k] })
	if err == nil || !strings.Contains(err.Error(), "LiteLLM proxy was removed") {
		t.Fatalf("LLM_PROVIDER=litellm: err = %v", err)
	}
	env["LLM_PROVIDER"] = "litellm-eval"
	if _, err := config.FromEnv(func(k string) string { return env[k] }); err == nil {
		t.Fatal("LLM_PROVIDER=litellm-eval accepted by the server config")
	}
}

func jsonString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}
