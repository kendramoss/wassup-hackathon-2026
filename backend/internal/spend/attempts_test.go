package spend

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"cloud.google.com/go/auth"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/evalkit"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/llm"
)

// Prices (PromoPrices): gemini-4-argon $2 in / $10 out per 1M, gemini-3.8-flash $0.75 / $3.75.
const (
	argon = "gemini-4-argon"
	flash = "gemini-3.8-flash"
)

type staticToken struct{}

func (staticToken) Token(context.Context) (*auth.Token, error) {
	return &auth.Token{Value: "t", Type: "Bearer"}, nil
}

// vertexServer is a localhost fake Vertex endpoint. script maps a model name
// (found in the URL path) to the bodies/statuses it answers with, in order.
type reply struct {
	status int
	body   string
}

type vertexServer struct {
	mu     sync.Mutex
	script map[string][]reply
	hits   map[string]int
	url    string
}

func newVertexServer(t *testing.T, script map[string][]reply) *vertexServer {
	t.Helper()
	vs := &vertexServer{script: script, hits: map[string]int{}}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		model := ""
		for m := range vs.script {
			if strings.Contains(r.URL.Path, "/models/"+m+":") {
				model = m
			}
		}
		vs.mu.Lock()
		i := vs.hits[model]
		vs.hits[model]++
		rs := vs.script[model]
		vs.mu.Unlock()
		if i >= len(rs) {
			i = len(rs) - 1
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(rs[i].status)
		_, _ = io.WriteString(w, rs[i].body)
	}))
	t.Cleanup(srv.Close)
	vs.url = srv.URL
	return vs
}

func (vs *vertexServer) client(t *testing.T) *llm.Vertex {
	t.Helper()
	v, err := llm.NewVertex(context.Background(), llm.VertexOptions{
		Project: "p", Location: "global", Timeout: 5 * time.Second,
		BaseURL: vs.url, Credentials: auth.NewCredentials(&auth.CredentialsOptions{TokenProvider: staticToken{}}),
	})
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func usage(in, thoughts int) string {
	return fmt.Sprintf(`"usageMetadata":{"promptTokenCount":%d,"candidatesTokenCount":0,"thoughtsTokenCount":%d,"totalTokenCount":%d}`, in, thoughts, in+thoughts)
}

func okBody(in, out int) string {
	return fmt.Sprintf(`{"candidates":[{"finishReason":"STOP","content":{"role":"model","parts":[{"text":"{}"}]}}],"usageMetadata":{"promptTokenCount":%d,"candidatesTokenCount":%d,"totalTokenCount":%d}}`, in, out, in+out)
}

func cost(model string, in, out int) float64 {
	return evalkit.Cost(evalkit.PromoPrices, model, in, out)
}

func near(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

func total(t *testing.T, m *Meter) float64 {
	t.Helper()
	_, tot := m.Over(context.Background())
	return tot
}

var noPaceRetry = func(m llm.Model, n int) llm.Model { return llm.WithRetry(m, n, time.Millisecond) }

// 1: 200 with usage and no candidates.
func TestEmptyCandidatesUsageIsCharged(t *testing.T) {
	vs := newVertexServer(t, map[string][]reply{argon: {{200, `{` + usage(10000, 1000) + `}`}}})
	m := New(NewMemory(), 0.01)
	_, err := m.Attempts(vs.client(t)).Generate(context.Background(), llm.Request{Model: argon})
	if !errors.Is(err, llm.ErrBadResponse) {
		t.Fatalf("err = %v, want ErrBadResponse", err)
	}
	over, tot := m.Over(context.Background())
	if !near(tot, cost(argon, 10000, 1000)) || !near(tot, 0.03) || !over {
		t.Fatalf("total %v over %v, want 0.03 and over", tot, over)
	}
}

// Vertex returns the usage-bearing partial response together with the error.
func TestVertexReturnsUsageWithBadResponse(t *testing.T) {
	vs := newVertexServer(t, map[string][]reply{argon: {{200, `{"modelVersion":"gemini-4-argon-001",` + usage(700, 50) + `}`}}})
	resp, err := vs.client(t).Generate(context.Background(), llm.Request{Model: argon})
	if !errors.Is(err, llm.ErrBadResponse) {
		t.Fatalf("err = %v", err)
	}
	if resp.Usage.InputTokens != 700 || resp.Usage.OutputTokens != 50 || resp.ModelID != "gemini-4-argon-001" || resp.Provider != "vertex" {
		t.Fatalf("partial response = %+v", resp)
	}
}

// 2: SAFETY finish, usage, no content.
func TestSafetyFinishUsageIsCharged(t *testing.T) {
	vs := newVertexServer(t, map[string][]reply{argon: {{200, `{"candidates":[{"finishReason":"SAFETY"}],` + usage(2000, 0) + `}`}}})
	m := New(NewMemory(), 0)
	resp, err := m.Attempts(vs.client(t)).Generate(context.Background(), llm.Request{Model: argon})
	if !errors.Is(err, llm.ErrBadResponse) {
		t.Fatalf("err = %v", err)
	}
	_ = resp
	if got, _ := m.store.Total(context.Background(), m.today()); !near(got, cost(argon, 2000, 0)) {
		t.Fatalf("charged %v, want %v", got, cost(argon, 2000, 0))
	}
}

// 3: retry.
func TestRetryChargesEachAttemptOnce(t *testing.T) {
	t.Run("503 without usage then success", func(t *testing.T) {
		vs := newVertexServer(t, map[string][]reply{argon: {{503, `{"error":{"code":503,"message":"busy","status":"UNAVAILABLE"}}`}, {200, okBody(100, 10)}}})
		m := New(NewMemory(), 1)
		resp, err := noPaceRetry(m.Attempts(vs.client(t)), 2).Generate(context.Background(), llm.Request{Model: argon})
		if err != nil || resp.Usage.InputTokens != 100 {
			t.Fatalf("resp %+v err %v", resp, err)
		}
		if got := total(t, m); !near(got, cost(argon, 100, 10)) {
			t.Fatalf("total %v want %v", got, cost(argon, 100, 10))
		}
	})
	t.Run("bad response with usage then success", func(t *testing.T) {
		vs := newVertexServer(t, map[string][]reply{argon: {{200, `{"promptFeedback":{"blockReason":"SAFETY"},` + usage(500, 0) + `}`}, {200, okBody(100, 10)}}})
		m := New(NewMemory(), 1)
		// Bad responses are not retried, so the caller retries by hand.
		mod := noPaceRetry(m.Attempts(vs.client(t)), 2)
		if _, err := mod.Generate(context.Background(), llm.Request{Model: argon}); !errors.Is(err, llm.ErrBadResponse) {
			t.Fatalf("err = %v", err)
		}
		if _, err := mod.Generate(context.Background(), llm.Request{Model: argon}); err != nil {
			t.Fatal(err)
		}
		if got, want := total(t, m), cost(argon, 500, 0)+cost(argon, 100, 10); !near(got, want) {
			t.Fatalf("total %v want %v", got, want)
		}
	})
	t.Run("retryable failure carrying usage then success", func(t *testing.T) {
		// A raw client whose first attempt fails retryably after reporting usage.
		n := 0
		raw := &llm.Fake{Respond: func(llm.Request) (llm.Response, error) {
			n++
			if n == 1 {
				return llm.Response{Usage: llm.Usage{InputTokens: 1000, OutputTokens: 100}}, &llm.APIError{Provider: "vertex", StatusCode: 503}
			}
			return llm.Response{Text: "x", Usage: llm.Usage{InputTokens: 100, OutputTokens: 10}}, nil
		}}
		m := New(NewMemory(), 1)
		resp, err := noPaceRetry(m.Attempts(raw), 2).Generate(context.Background(), llm.Request{Model: argon})
		if err != nil || resp.Usage.InputTokens != 100 {
			t.Fatalf("resp %+v err %v", resp, err)
		}
		if got, want := total(t, m), cost(argon, 1000, 100)+cost(argon, 100, 10); !near(got, want) {
			t.Fatalf("total %v want %v", got, want)
		}
	})
}

// 4: primary exhausted, fallback answers at its own price.
func TestPrimaryAndFallbackPricedSeparately(t *testing.T) {
	m := New(NewMemory(), 1)
	pn := 0
	primary := &llm.Fake{Respond: func(llm.Request) (llm.Response, error) {
		pn++
		return llm.Response{Usage: llm.Usage{InputTokens: 4000, OutputTokens: 400}}, &llm.APIError{Provider: "vertex", StatusCode: 429}
	}}
	fb := &llm.Fake{Respond: func(llm.Request) (llm.Response, error) {
		return llm.Response{Text: "ok", Usage: llm.Usage{InputTokens: 3000, OutputTokens: 300}}, nil
	}}
	model := llm.WithFallback(noPaceRetry(m.Attempts(primary), 1), m.Attempts(fb), flash)
	resp, err := model.Generate(context.Background(), llm.Request{Model: argon})
	if err != nil || resp.ModelID != flash {
		t.Fatalf("resp %+v err %v", resp, err)
	}
	want := 2*cost(argon, 4000, 400) + cost(flash, 3000, 300)
	if got := total(t, m); !near(got, want) || pn != 2 {
		t.Fatalf("total %v want %v (primary attempts %d)", got, want, pn)
	}
	if near(want, 3*cost(argon, 3000, 300)) || near(want, 3*cost(flash, 3000, 300)) {
		t.Fatal("test prices do not distinguish models")
	}
}

// 5: fallback fails with usage metadata.
func TestFailingFallbackUsageChargedAtFallbackPrice(t *testing.T) {
	vs := newVertexServer(t, map[string][]reply{
		argon: {{429, `{"error":{"code":429,"message":"quota","status":"RESOURCE_EXHAUSTED"}}`}},
		flash: {{200, `{` + usage(8000, 500) + `}`}},
	})
	m := New(NewMemory(), 1)
	c := vs.client(t)
	model := llm.WithFallback(noPaceRetry(m.Attempts(c), 0), m.Attempts(c), flash)
	_, err := model.Generate(context.Background(), llm.Request{Model: argon})
	if !errors.Is(err, llm.ErrBadResponse) {
		t.Fatalf("err = %v", err)
	}
	if got, want := total(t, m), cost(flash, 8000, 500); !near(got, want) || want == 0 {
		t.Fatalf("total %v want %v", got, want)
	}
}

// 6: no metadata or zero usage on error charges nothing.
func TestNoUsageChargesNothing(t *testing.T) {
	vs := newVertexServer(t, map[string][]reply{argon: {
		{200, `{"candidates":[]}`},
		{200, `{"candidates":[],"usageMetadata":{}}`},
		{500, `{"error":{"code":500,"message":"boom"}}`},
	}})
	m := New(NewMemory(), 1)
	mod := m.Attempts(vs.client(t))
	for i := 0; i < 3; i++ {
		if _, err := mod.Generate(context.Background(), llm.Request{Model: argon}); err == nil {
			t.Fatal("want error")
		}
	}
	if got := total(t, m); got != 0 {
		t.Fatalf("total %v want 0", got)
	}
	// A transport error (connection closed) also charges nothing.
	cl := &llm.Fake{Respond: func(llm.Request) (llm.Response, error) {
		return llm.Response{}, fmt.Errorf("%w: reset", llm.ErrUnavailable)
	}}
	_, _ = m.Attempts(cl).Generate(context.Background(), llm.Request{Model: argon})
	if got := total(t, m); got != 0 {
		t.Fatalf("total %v want 0", got)
	}
}

// 7: store outage then recovery; bad-response usage still counts and the cap cannot reopen.
func TestBadResponseUsageThroughStoreOutageKeepsCap(t *testing.T) {
	vs := newVertexServer(t, map[string][]reply{argon: {{200, `{` + usage(10000, 1000) + `}`}}}) // $0.03 each
	s := &scriptedStore{Memory: NewMemory(), errs: []error{errors.New("store down")}}
	m := New(s, 0.05)
	mod := m.Attempts(vs.client(t))
	ctx := context.Background()
	mod.Generate(ctx, llm.Request{Model: argon}) // write fails: debt 0.03
	if over, tot := m.Over(ctx); over || !near(tot, 0.03) {
		t.Fatalf("while down: over %v total %v", over, tot)
	}
	mod.Generate(ctx, llm.Request{Model: argon}) // store back: knows 0.03, debt 0.03 on top
	if over, tot := m.Over(ctx); !over || !near(tot, 0.06) {
		t.Fatalf("after recovery: over %v total %v, want over at 0.06", over, tot)
	}
	mod.Generate(ctx, llm.Request{Model: argon})
	if _, tot := m.Over(ctx); tot+1e-9 < 0.09 {
		t.Fatalf("total fell to %v", tot)
	}
}

// A canceled request still charges the attempt it made.
func TestCanceledContextStillCharges(t *testing.T) {
	m := New(NewMemory(), 1)
	raw := &llm.Fake{Respond: func(llm.Request) (llm.Response, error) {
		return llm.Response{Usage: llm.Usage{InputTokens: 1000, OutputTokens: 100}}, llm.ErrBadResponse
	}}
	ctx, cancel := context.WithCancel(context.Background())
	cl := &cancelAfter{Model: raw, cancel: cancel}
	_, _ = m.Attempts(cl).Generate(ctx, llm.Request{Model: argon})
	if got := total(t, m); !near(got, cost(argon, 1000, 100)) {
		t.Fatalf("total %v", got)
	}
}

type cancelAfter struct {
	llm.Model
	cancel func()
}

func (c *cancelAfter) Generate(ctx context.Context, r llm.Request) (llm.Response, error) {
	resp, err := c.Model.Generate(ctx, r)
	c.cancel()
	return resp, err
}

// Attempts is idempotent: wrapping an already metered client again does not double charge.
func TestAttemptsIsIdempotent(t *testing.T) {
	m := New(NewMemory(), 1)
	raw := &llm.Fake{Respond: func(llm.Request) (llm.Response, error) {
		return llm.Response{Text: "x", Usage: llm.Usage{InputTokens: 1000, OutputTokens: 100}}, nil
	}}
	twice := m.Attempts(m.Attempts(raw))
	_, _ = twice.Generate(context.Background(), llm.Request{Model: argon})
	if got := total(t, m); !near(got, cost(argon, 1000, 100)) {
		t.Fatalf("double charged: %v", got)
	}
	var nilMeter *Meter
	if nilMeter.Attempts(raw) != llm.Model(raw) {
		t.Fatal("nil meter must leave the client unchanged")
	}
}

// 9: displayed usage is the raw response's usage, unchanged by metering.
func TestSuccessfulResponseUnchangedByMetering(t *testing.T) {
	vs := newVertexServer(t, map[string][]reply{argon: {{200, okBody(123, 45)}}})
	raw := vs.client(t)
	want, err := raw.Generate(context.Background(), llm.Request{Model: argon})
	if err != nil {
		t.Fatal(err)
	}
	got, err := New(NewMemory(), 1).Attempts(raw).Generate(context.Background(), llm.Request{Model: argon})
	if err != nil {
		t.Fatal(err)
	}
	got.Latency, want.Latency = 0, 0
	if got.Usage != want.Usage || got.Text != want.Text || got.ModelID != want.ModelID || got.Usage.InputTokens != 123 || got.Usage.OutputTokens != 45 {
		t.Fatalf("got %+v want %+v", got, want)
	}
}
