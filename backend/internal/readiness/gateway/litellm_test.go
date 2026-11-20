package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/llm"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/readiness/budget"
)

const key = "sk-SENTINEL-GATEWAY-KEY"

var testNow = time.Date(2026, 11, 21, 12, 0, 0, 0, time.UTC)

const testPrices = `{"schema":2,"entries":[{"alias":"alias","resolved_model_ids":["m"],"list_usd_per_mtok_in":4,"list_usd_per_mtok_out":20,"gateway_usd_per_mtok_in":4,"gateway_usd_per_mtok_out":20,"source":"t","verified_at":"2026-11-21T00:00:00Z","valid_until":"2026-11-30T00:00:00Z","max_output_enforced":true,"output_includes_reasoning":true}]}`

// guarded wraps m in a real guard over a temp ledger; the only legitimate way
// to obtain a permit. It returns a context-free call function.
func guarded(t *testing.T, m llm.Model) (*budget.Guard, *budget.Ledger) {
	t.Helper()
	d := t.TempDir()
	shr, att := filepath.Join(d, "SHAREDBUDGET.json"), filepath.Join(d, "attempts.jsonl")
	os.WriteFile(shr, []byte(`{"cap_usd":20,"spent_usd":0,"reserved_usd":0,"attempts_raw":0,"failures_raw":0,"calls_authorized":true}`), 0o600)
	tbl, err := budget.ParsePriceTable([]byte(testPrices), 0)
	if err != nil {
		t.Fatal(err)
	}
	l, err := budget.Open(att, shr)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { l.Close() })
	return budget.NewGuard(m, l, tbl, budget.Opts{AllowModelCalls: true, Now: func() time.Time { return testNow }, RunEndsBy: testNow.Add(time.Hour)}), l
}

// twice sends two wire attempts with one context, i.e. one permit.
type twice struct{ c *Client }

func (twice) Provider() string { return "x" }
func (w twice) Generate(ctx context.Context, r llm.Request) (llm.Response, error) {
	if _, err := w.c.Generate(ctx, r); err != nil {
		return llm.Response{}, err
	}
	return w.c.Generate(ctx, r)
}

// capture records the raw adapter response before the guard judges it.
type capture struct {
	c    *Client
	resp *llm.Response
}

func (capture) Provider() string { return "x" }
func (w capture) Generate(ctx context.Context, r llm.Request) (llm.Response, error) {
	resp, err := w.c.Generate(ctx, r)
	*w.resp = resp
	return resp, err
}

func ctxCase() context.Context { return budget.WithCase(context.Background(), "c") }

func client(t *testing.T, url string) *Client {
	t.Helper()
	c, err := New(Config{BaseURL: url, APIKey: key, Timeout: 5 * time.Second, AllowLoopbackHTTP: true})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func rq() llm.Request {
	return llm.Request{Model: "alias", System: "s", Messages: []llm.Message{{Role: llm.RoleUser, Content: "hi"}}, MaxOutputTokens: 64}
}

const okBody = `{"model":"m","choices":[{"message":{"role":"assistant","content":"hello"},"finish_reason":"stop"}],"usage":{"prompt_tokens":11,"completion_tokens":3,"total_tokens":14}}`

func TestNoPermitMeansNoNetwork(t *testing.T) {
	var hits atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hits.Add(1) }))
	defer srv.Close()
	if _, err := client(t, srv.URL).Generate(context.Background(), rq()); !errors.Is(err, budget.ErrNoPermit) || hits.Load() != 0 {
		t.Fatalf("err=%v hits=%d", err, hits.Load())
	}
}

func TestRequestShape(t *testing.T) {
	var body []byte
	var auth, path string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ = io.ReadAll(r.Body)
		auth, path = r.Header.Get("Authorization"), r.URL.Path
		io.WriteString(w, okBody)
	}))
	defer srv.Close()
	g, _ := guarded(t, client(t, srv.URL))
	resp, err := g.Generate(ctxCase(), rq())
	if err != nil || resp.Text != "hello" || resp.ModelID != "m" || resp.Usage.InputTokens != 11 || resp.Usage.OutputTokens != 3 {
		t.Fatalf("%+v %v", resp, err)
	}
	var m map[string]any
	if err := json.Unmarshal(body, &m); err != nil {
		t.Fatal(err)
	}
	if m["max_tokens"] != float64(64) || m["seed"] != float64(llm.Seed) || m["temperature"] != float64(0) {
		t.Fatalf("body %v", m)
	}
	if auth != "Bearer "+key || path != "/v1/chat/completions" || strings.Contains(string(body), key) {
		t.Fatal("auth or path wrong, or key in body")
	}
}

func TestExactlyOneWireRequestOnFailureNoRetry(t *testing.T) {
	for _, code := range []int{429, 500, 502, 503, 504} {
		var hits atomic.Int64
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			hits.Add(1)
			http.Error(w, "echo "+key, code)
		}))
		g, _ := guarded(t, client(t, srv.URL))
		_, err := g.Generate(ctxCase(), rq())
		srv.Close()
		var api *llm.APIError
		if !errors.As(err, &api) || api.StatusCode != code || hits.Load() != 1 {
			t.Fatalf("%d: err=%v hits=%d", code, err, hits.Load())
		}
		if strings.Contains(err.Error(), key) {
			t.Fatalf("%d: key echoed in error", code)
		}
	}
}

func TestRedirectsAreNeverFollowed(t *testing.T) {
	for _, code := range []int{301, 302, 307, 308} {
		var targetHits, srcHits atomic.Int64
		target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { targetHits.Add(1) }))
		src := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			srcHits.Add(1)
			http.Redirect(w, r, target.URL+"/v1/chat/completions", code)
		}))
		g, _ := guarded(t, client(t, src.URL))
		_, err := g.Generate(ctxCase(), rq())
		src.Close()
		target.Close()
		if !errors.Is(err, ErrRedirect) || srcHits.Load() != 1 || targetHits.Load() != 0 {
			t.Fatalf("%d: err=%v src=%d target=%d", code, err, srcHits.Load(), targetHits.Load())
		}
		var api *llm.APIError
		if !errors.As(err, &api) || api.StatusCode != code {
			t.Fatalf("%d: not classified as an API failure: %v", code, err)
		}
	}
}

func TestPermitCannotBeReplayed(t *testing.T) {
	var hits atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hits.Add(1); io.WriteString(w, okBody) }))
	defer srv.Close()
	g, l := guarded(t, twice{client(t, srv.URL)})
	_, err := g.Generate(ctxCase(), rq())
	if !errors.Is(err, budget.ErrPermitUsed) || hits.Load() != 1 {
		t.Fatalf("err=%v hits=%d", err, hits.Load())
	}
	// One request reached the wire, so the reservation is charged in full
	// whatever the replay error says (R1): charged > 0 and equals the reservation.
	rqq := rq()
	inUp, _ := budget.InputTokensUpper(rqq)
	base := budget.CostNanos(inUp, 4_000_000_000) + budget.CostNanos(int64(rqq.MaxOutputTokens), 20_000_000_000)
	margin := base / 50
	if margin < 1_000_000 {
		margin = 1_000_000
	}
	if st := l.Snapshot(); st.AttemptsRaw != 1 || st.SpentNanos != base+margin || st.SpentNanos <= 0 || st.FailuresRaw != 1 || st.OpenNanos != 0 {
		t.Fatalf("replay after a wire call was not charged in full: %+v want %d", st, base+margin)
	}
}

// nopermit calls the client without the guard's context: nothing can be sent.
type nopermit struct{ c *Client }

func (nopermit) Provider() string { return "x" }
func (w nopermit) Generate(_ context.Context, r llm.Request) (llm.Response, error) {
	return w.c.Generate(context.Background(), r)
}

func TestFreshLocalRefusalStillCostsNothing(t *testing.T) {
	var hits atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hits.Add(1) }))
	defer srv.Close()
	g, l := guarded(t, nopermit{client(t, srv.URL)})
	_, err := g.Generate(ctxCase(), rq())
	if !errors.Is(err, budget.ErrNotSent) || hits.Load() != 0 {
		t.Fatalf("err=%v hits=%d", err, hits.Load())
	}
	if st := l.Snapshot(); st.SpentNanos != 0 || st.FailuresRaw != 0 || st.AttemptsRaw != 1 || st.OpenNanos != 0 {
		t.Fatalf("%+v", st)
	}
}

func TestNewValidation(t *testing.T) {
	bad := []Config{
		{BaseURL: "http://gw.example.com", APIKey: key},
		{BaseURL: "http://127.0.0.1:9", APIKey: key}, // loopback http only with the test option
		{BaseURL: "https://user:pw@gw.example.com", APIKey: key},
		{BaseURL: "https://gw.example.com?x=1", APIKey: key},
		{BaseURL: "https://gw.example.com#f", APIKey: key},
		{BaseURL: "gw.example.com", APIKey: key},
		{BaseURL: "https://gw.example.com", APIKey: ""},
		{BaseURL: "https://gw.example.com", APIKey: "a\nb"},
		{BaseURL: "https://gw.example.com", APIKey: "a b"},
	}
	for i, c := range bad {
		if _, err := New(c); err == nil {
			t.Errorf("case %d accepted", i)
		} else if strings.Contains(err.Error(), key) {
			t.Errorf("case %d leaks key", i)
		}
	}
	if _, err := New(Config{BaseURL: "https://gw.example.com/litellm", APIKey: key}); err != nil {
		t.Error(err)
	}
	if _, err := New(Config{BaseURL: "http://evil.example.com", APIKey: key, AllowLoopbackHTTP: true}); err == nil {
		t.Error("test option must not allow non-loopback http")
	}
}

func TestProxyEnvironmentIsRefusedNotCleared(t *testing.T) {
	t.Setenv("HTTPS_PROXY", "http://proxy.invalid:3128")
	if _, err := New(Config{BaseURL: "https://gw.example.com", APIKey: key}); err == nil {
		t.Fatal("proxy configured for the gateway host was ignored")
	}
	if got := os.Getenv("HTTPS_PROXY"); got == "" {
		t.Fatal("the tool must not clear the administrator's proxy setting")
	}
	t.Setenv("NO_PROXY", "internal.example,gw.example.com")
	if _, err := New(Config{BaseURL: "https://gw.example.com", APIKey: key}); err != nil {
		t.Fatalf("NO_PROXY coverage should allow a direct route: %v", err)
	}
	if err := checkProxyEnv("other.example.com", func(k string) string {
		return map[string]string{"https_proxy": "x", "NO_PROXY": "gw.example.com"}[k]
	}); err == nil {
		t.Fatal("lowercase variable or other host ignored")
	}
	if err := checkProxyEnv("127.0.0.1", func(k string) string { return map[string]string{"HTTP_PROXY": "x"}[k] }); err != nil {
		t.Fatal("loopback is never proxied")
	}
}

func TestMaxTokensRequired(t *testing.T) {
	r := rq()
	r.MaxOutputTokens = 0
	if _, err := client(t, "http://127.0.0.1:1").Generate(context.Background(), r); !errors.Is(err, budget.ErrNotSent) {
		t.Fatalf("accepted a request without max tokens: %v", err)
	}
}

func TestBilledOutputTakesLargestFigure(t *testing.T) {
	cases := []struct{ p, c, tot, r, want int }{
		{10, 5, 15, 0, 5}, {10, 5, 40, 0, 30}, {10, 5, 15, 25, 25}, {10, 0, 0, 0, 0},
	}
	for _, c := range cases {
		if got := BilledOutput(c.p, c.c, c.tot, c.r); got != c.want {
			t.Errorf("%+v got %d", c, got)
		}
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"model":"m","choices":[{"message":{"content":"x"}}],"usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":400,"completion_tokens_details":{"reasoning_tokens":7}}}`)
	}))
	defer srv.Close()
	var resp llm.Response
	g, _ := guarded(t, capture{client(t, srv.URL), &resp})
	r := rq()
	r.MaxOutputTokens = 1000
	if _, err := g.Generate(ctxCase(), r); err != nil || resp.Usage.OutputTokens != 390 {
		t.Fatalf("%+v %v", resp.Usage, err)
	}
}

func TestMissingModelIDStaysEmpty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"choices":[{"message":{"content":"x"}}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`)
	}))
	defer srv.Close()
	var resp llm.Response
	g, l := guarded(t, capture{client(t, srv.URL), &resp})
	_, err := g.Generate(ctxCase(), rq())
	if resp.ModelID != "" || !errors.Is(err, budget.ErrModelNotAllowed) || !l.Snapshot().Halted {
		t.Fatalf("%q %v", resp.ModelID, err)
	}
}

// End to end with the guard: the real adapter behind the real guard and ledger.
func TestGuardedGatewayEndToEnd(t *testing.T) {
	var hits atomic.Int64
	mode := "ok"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		switch mode {
		case "504":
			http.Error(w, "echo "+key, 504)
		case "reasoning":
			io.WriteString(w, `{"model":"m","choices":[{"message":{"content":"x"}}],"usage":{"prompt_tokens":10,"completion_tokens":2,"total_tokens":9000}}`)
		default:
			io.WriteString(w, okBody)
		}
	}))
	defer srv.Close()
	d := t.TempDir()
	shr, att := filepath.Join(d, "SHAREDBUDGET.json"), filepath.Join(d, "attempts.jsonl")
	os.WriteFile(shr, []byte(`{"cap_usd":20,"spent_usd":0,"reserved_usd":0,"attempts_raw":0,"failures_raw":0,"calls_authorized":true}`), 0o600)
	tbl, err := budget.ParsePriceTable([]byte(`{"schema":2,"entries":[{"alias":"alias","resolved_model_ids":["m"],"list_usd_per_mtok_in":4,"list_usd_per_mtok_out":20,"gateway_usd_per_mtok_in":4,"gateway_usd_per_mtok_out":20,"source":"t","verified_at":"2026-11-21T00:00:00Z","valid_until":"2026-11-30T00:00:00Z","max_output_enforced":true,"output_includes_reasoning":true}]}`), 0)
	if err != nil {
		t.Fatal(err)
	}
	l, err := budget.Open(att, shr)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	now := time.Date(2026, 11, 21, 12, 0, 0, 0, time.UTC)
	g := budget.NewGuard(client(t, srv.URL), l, tbl, budget.Opts{AllowModelCalls: true, Now: func() time.Time { return now }, RunEndsBy: now.Add(time.Hour)})
	r := rq()
	r.Messages[0].Content = "PRIVATE-PROMPT-TEXT"
	if _, err := g.Generate(budget.WithCase(context.Background(), "c"), r); err != nil {
		t.Fatal(err)
	}
	if st := l.Snapshot(); st.SpentNanos != 11*4000+3*20000 || hits.Load() != 1 {
		t.Fatalf("%+v hits=%d", st, hits.Load())
	}
	mode = "504"
	_, err = g.Generate(budget.WithCase(context.Background(), "c2"), r)
	var api *llm.APIError
	if !errors.As(err, &api) || api.StatusCode != 504 || hits.Load() != 2 {
		t.Fatalf("%v hits=%d", err, hits.Load())
	}
	mode = "reasoning" // hidden reasoning visible only through total_tokens: billed, then halt (cap breached)
	if _, err := g.Generate(budget.WithCase(context.Background(), "c3"), r); !errors.Is(err, budget.ErrHalted) {
		t.Fatalf("%v", err)
	}
	if hits.Load() != 3 {
		t.Fatalf("hits=%d", hits.Load())
	}
	if _, err := g.Generate(budget.WithCase(context.Background(), "c4"), r); err == nil || hits.Load() != 3 {
		t.Fatal("call after halt")
	}
	for _, p := range []string{att, shr} {
		b, _ := os.ReadFile(p)
		if strings.Contains(string(b), key) || strings.Contains(string(b), "PRIVATE-PROMPT-TEXT") {
			t.Fatalf("%s leaks", p)
		}
	}
}
