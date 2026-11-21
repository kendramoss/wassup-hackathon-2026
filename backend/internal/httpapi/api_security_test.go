package httpapi

import (
	"bufio"
	"context"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/apikey"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/llm"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/ratelimit"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/spend"
)

// Tests for the public API's limits and security headers (D137).

// routeWrappers parses server.go and returns, for every registered pattern,
// the name of the api method its handler is wrapped in ("" when the
// handler is passed bare).
func routeWrappers(t *testing.T) map[string]string {
	t.Helper()
	f, err := parser.ParseFile(token.NewFileSet(), "server.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]string{}
	ast.Inspect(f, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok || len(call.Args) < 2 {
			return true
		}
		if sel, ok := call.Fun.(*ast.SelectorExpr); !ok || sel.Sel.Name != "HandleFunc" {
			return true
		}
		lit, ok := call.Args[0].(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			return true
		}
		p, err := strconv.Unquote(lit.Value)
		if err != nil {
			return true
		}
		wrapper := ""
		if inner, ok := call.Args[1].(*ast.CallExpr); ok {
			if sel, ok := inner.Fun.(*ast.SelectorExpr); ok {
				wrapper = sel.Sel.Name
			}
		}
		if _, seen := out[p]; !seen || wrapper != "" {
			out[p] = wrapper
		}
		return true
	})
	return out
}

// modelRoutes call the model, so they must go through limit: the spend
// caps, the per-client spend share and the in-flight cap apply to them.
var modelRoutes = map[string]bool{
	"POST /v1/cases/predict": true, "POST /v1/cases/batch": true, "POST /v1/products/import": true,
	"POST /v1/products/extract": true, "POST /v1/products/extract-media": true, "POST /v1/products/money-prefill": true,
	"POST /v1/products/hts-suggest": true, "POST /v1/products/parts-hts-suggest": true, "POST /v1/products/cost-estimate": true, "POST /v1/products/family-check": true,
	"POST /v1/cases/{id}/bom-version": true, "POST /v1/cases/{id}/scenarios": true, "POST /v1/cases/{id}/compare-targets": true,
	"POST /v1/cases/{id}/ask": true, "POST /v1/cases/{id}/watch": true, "POST /v1/cases/{id}/plan": true, "POST /v1/cases/{id}/answers": true,
}

// Every /v1 route has a rate class: model-calling routes go through limit,
// the rest through rateOnly or cheap. Only admin routes may skip it (they
// refuse anyone without the admin token, who is never limited).
func TestEveryV1RouteHasALimitClass(t *testing.T) {
	routes := routeWrappers(t)
	for p := range modelRoutes {
		if _, ok := routes[p]; !ok {
			t.Errorf("modelRoutes lists %q, which server.go does not register", p)
		}
	}
	if len(routes) < 35 {
		t.Fatalf("found only %d routes; the parse is broken", len(routes))
	}
	allowed := map[string]bool{"limit": true, "rateOnly": true, "cheap": true, "adminOnly": true}
	for p, w := range routes {
		if p == "POST /mcp" {
			if w != "rateOnly" {
				t.Errorf("POST /mcp is wrapped in %q, want a.rateOnly(basic, ...): its tool calls take their own route's limits in-process, and cheap's deadline would cut them off", w)
			}
			continue
		}
		if !strings.Contains(p, " /v1/") {
			continue
		}
		if !allowed[w] {
			t.Errorf("route %q is registered without a rate class (wrapper %q): wrap it in a.cheap, a.rateOnly or a.limit", p, w)
		}
		if modelRoutes[p] && w != "limit" {
			t.Errorf("route %q calls the model but is wrapped in %q: it needs a.limit (spend caps and the in-flight cap)", p, w)
		}
		if w == "adminOnly" && routeTable[p] != adminRoute {
			t.Errorf("route %q skips the limits as adminOnly but is not an admin route", p)
		}
	}
}

// openAPIOperations reads docs/api/openapi.yaml and returns "METHOD /path"
// for every operation under paths.
func openAPIOperations(t *testing.T) map[string]bool {
	t.Helper()
	f, err := os.Open(filepath.Join("..", "..", "..", "docs", "api", "openapi.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	ops := map[string]bool{}
	inPaths, path := false, ""
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		switch {
		case line == "paths:":
			inPaths = true
		case inPaths && line != "" && !strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "#"):
			inPaths = false
		case inPaths && strings.HasPrefix(line, "  /") && strings.HasSuffix(line, ":"):
			path = strings.TrimSuffix(strings.TrimSpace(line), ":")
		case inPaths && path != "" && strings.HasPrefix(line, "    ") && !strings.HasPrefix(line, "     "):
			m := strings.TrimSuffix(strings.TrimSpace(line), ":")
			switch m {
			case "get", "post", "put", "patch", "delete":
				ops[strings.ToUpper(m)+" "+path] = true
			}
		}
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	return ops
}

// Every registered route is documented in openapi.yaml, and every
// documented operation is registered, so the two cannot drift.
func TestEveryRouteIsInOpenAPI(t *testing.T) {
	ops := openAPIOperations(t)
	if len(ops) < 35 {
		t.Fatalf("found only %d operations in openapi.yaml; the parse is broken", len(ops))
	}
	reg := map[string]bool{}
	for p := range routeWrappers(t) {
		if p == "/" { // the catch-all 404
			continue
		}
		reg[p] = true
		if !ops[p] {
			t.Errorf("route %q is registered but not documented in docs/api/openapi.yaml", p)
		}
	}
	for op := range ops {
		if !reg[op] {
			t.Errorf("openapi.yaml documents %q, which server.go does not register", op)
		}
	}
}

// Each cheap route is limited per client with the basic class, reports
// the RateLimit-* headers, and answers 429 with Retry-After past it.
func TestCheapRoutesAreLimited(t *testing.T) {
	routes := []struct{ method, path, body string }{
		{"GET", "/v1/cases", ""},
		{"GET", "/v1/cases/nope", ""},
		{"PATCH", "/v1/cases/nope", `{"title":"x"}`},
		{"DELETE", "/v1/cases/nope", ""},
		{"PUT", "/v1/cases/nope/economics", `{}`},
		{"POST", "/v1/cases/nope/export", `{}`},
		{"GET", "/v1/eval/summary", ""},
		{"GET", "/v1/demos", ""},
		{"GET", "/v1/demos/nope", ""},
		{"GET", "/v1/duty-rates?hts=8419.89.95", ""},
		{"GET", "/v1/rulings/N123456", ""},
	}
	for _, rt := range routes {
		t.Run(rt.method+" "+rt.path, func(t *testing.T) {
			hs := newHarnessWith(t, "substantial_transformation", func(d *Deps) {
				d.Limits = Limits{BasicPerMin: 2, LightPerMin: 100, HeavyPerMin: 100, GlobalPerMin: 1000, LookupPerMin: 100}
			})
			client := map[string]string{"X-Client-IP": "203.0.113.5"}
			for i, wantRemaining := range []string{"1", "0"} {
				rec := serve(hs, rt.method, rt.path, rt.body, client)
				if rec.Code == http.StatusTooManyRequests {
					t.Fatalf("request %d = 429 within the limit", i+1)
				}
				if rec.Header().Get("RateLimit-Limit") != "2" || rec.Header().Get("RateLimit-Remaining") != wantRemaining || rec.Header().Get("RateLimit-Reset") == "" {
					t.Errorf("request %d headers = %v", i+1, rec.Header())
				}
			}
			rec := serve(hs, rt.method, rt.path, rt.body, client)
			if rec.Code != http.StatusTooManyRequests || rec.Header().Get("Retry-After") == "" || rec.Header().Get("RateLimit-Remaining") != "0" ||
				!strings.Contains(rec.Body.String(), `"rate_limited"`) {
				t.Errorf("past the limit = %d %s %v", rec.Code, rec.Body, rec.Header())
			}
			if rec := serve(hs, rt.method, rt.path, rt.body, map[string]string{"X-Client-IP": "203.0.113.6"}); rec.Code == http.StatusTooManyRequests {
				t.Errorf("another client limited = %d", rec.Code)
			}
			if rec := serve(hs, rt.method, rt.path, rt.body, map[string]string{"X-Client-IP": "203.0.113.5", "X-Admin-Token": testAdmin}); rec.Code == http.StatusTooManyRequests {
				t.Errorf("admin limited = %d", rec.Code)
			}
		})
	}
}

func serve(hs harness, method, path, body string, headers map[string]string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	hs.h.ServeHTTP(rec, req)
	return rec
}

// Model-calling routes report their bucket too, locally and with the
// shared counters.
func TestRateLimitHeadersOnModelRoutes(t *testing.T) {
	for _, shared := range []bool{false, true} {
		t.Run("shared="+strconv.FormatBool(shared), func(t *testing.T) {
			hs := newHarnessWith(t, "substantial_transformation", func(d *Deps) {
				d.Limits = Limits{LightPerMin: 3, HeavyPerMin: 1, GlobalPerMin: 100}
				if shared {
					d.RateStore = ratelimit.NewMemory()
				}
			})
			client := map[string]string{"X-Client-IP": "203.0.113.7"}
			body := heatGunBody(t)
			for i, want := range []string{"2", "1", "0"} {
				rec := serve(hs, "POST", "/v1/cases/predict", body, client)
				if rec.Code != http.StatusCreated || rec.Header().Get("RateLimit-Limit") != "3" || rec.Header().Get("RateLimit-Remaining") != want {
					t.Errorf("predict %d = %d %v", i+1, rec.Code, rec.Header())
				}
				reset, err := strconv.Atoi(rec.Header().Get("RateLimit-Reset"))
				if err != nil || reset < 0 || reset > 60 {
					t.Errorf("RateLimit-Reset = %q", rec.Header().Get("RateLimit-Reset"))
				}
			}
			rec := serve(hs, "POST", "/v1/cases/predict", body, client)
			if rec.Code != http.StatusTooManyRequests || rec.Header().Get("Retry-After") == "" {
				t.Errorf("fourth = %d %v", rec.Code, rec.Header())
			}
		})
	}
}

// A live MFN fetch takes from the shared lookup counter, as a ruling fetch
// does: two instances on one store share one lookup limit.
func TestDutyLookupUsesTheSharedLimiter(t *testing.T) {
	live, _ := fakeHTS(t)
	store := ratelimit.NewMemory()
	adjust := func(d *Deps) {
		d.LiveMFN = live
		d.Limits = Limits{LightPerMin: 100, HeavyPerMin: 100, GlobalPerMin: 1000, LookupPerMin: 1}
		d.RateStore = store
	}
	one := newHarnessWith(t, "substantial_transformation", adjust)
	two := newHarnessWith(t, "substantial_transformation", adjust)
	client := map[string]string{"X-Client-IP": "203.0.113.8"}
	if code, _ := one.doAs(t, "GET", "/v1/duty-rates?hts=8516.79.00", "", client); code != http.StatusOK {
		t.Fatalf("first fetch = %d", code)
	}
	if code, _ := two.doAs(t, "GET", "/v1/duty-rates?hts=8516.21.00", "", client); code != http.StatusTooManyRequests {
		t.Errorf("second fetch on another instance = %d, want 429 (one shared lookup limit)", code)
	}
}

// The in-flight cap: a client gets at most max model-calling requests at
// once; a slot is released when the handler returns, including after a
// stream; admin and other clients are not held back.
func TestInFlightCap(t *testing.T) {
	a := &api{d: Deps{Logger: slog.New(slog.DiscardHandler)}, limiter: newLimiter(Limits{LightPerMin: 100, HeavyPerMin: 100}), inflight: newInflight(2)}
	a.d.AdminToken = testAdmin
	a.d.TrustedProxyCIDRs = testProxyCIDRs(t)
	release := make(chan struct{})
	started := make(chan struct{}, 10)
	stream := a.limit(light, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		started <- struct{}{}
		<-release
		_, _ = w.Write([]byte("event: done\ndata: {}\n\n"))
	})
	call := func(headers map[string]string) *httptest.ResponseRecorder {
		req := httptest.NewRequest("POST", "/v1/cases/predict", nil)
		for k, v := range headers {
			req.Header.Set(k, v)
		}
		rec := httptest.NewRecorder()
		stream(rec, req)
		return rec
	}
	alice := map[string]string{"X-Client-IP": "203.0.113.10"}
	var wg sync.WaitGroup
	for range 2 {
		wg.Add(1)
		go func() { defer wg.Done(); call(alice) }()
	}
	<-started
	<-started
	tests := []struct {
		name    string
		headers map[string]string
		want    int
	}{
		{"third from the same client", alice, http.StatusTooManyRequests},
	}
	for _, tt := range tests {
		if rec := call(tt.headers); rec.Code != tt.want || rec.Header().Get("Retry-After") == "" || !strings.Contains(rec.Body.String(), `"too_many_in_flight"`) {
			t.Errorf("%s = %d %s", tt.name, rec.Code, rec.Body)
		}
	}
	// Another client and the admin are not held back (they block too, so run them in the background).
	for _, h := range []map[string]string{{"X-Client-IP": "203.0.113.11"}, {"X-Client-IP": "203.0.113.10", "X-Admin-Token": testAdmin}} {
		wg.Add(1)
		go func() { defer wg.Done(); call(h) }()
		select {
		case <-started:
		case <-time.After(2 * time.Second):
			t.Fatalf("%v was held back", h)
		}
	}
	close(release)
	wg.Wait()
	if rec := call(alice); rec.Code != http.StatusOK {
		t.Errorf("after the streams ended = %d, want the slots back", rec.Code)
	}
	if len(a.inflight.n) != 0 {
		t.Errorf("slots still held: %v", a.inflight.n)
	}
}

// A refused rate check gives its in-flight slot back.
func TestInFlightSlotReturnedWhenRateLimited(t *testing.T) {
	a := &api{d: Deps{Logger: slog.New(slog.DiscardHandler)}, limiter: newLimiter(Limits{LightPerMin: 1}), inflight: newInflight(1)}
	h := a.limit(light, func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	codes := []int{}
	for range 3 {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest("POST", "/v1/cases/predict", nil)
		req.RemoteAddr = "203.0.113.12:1"
		h(rec, req)
		codes = append(codes, rec.Code)
	}
	if codes[0] != http.StatusOK || codes[1] != http.StatusTooManyRequests || codes[2] != http.StatusTooManyRequests || len(a.inflight.n) != 0 {
		t.Errorf("codes %v, slots %v", codes, a.inflight.n)
	}
}

// One client may spend its share of the daily cap; others and the admin
// keep going. Model calls are charged to the client through the context.
func TestClientSpendShare(t *testing.T) {
	meter := spend.New(spend.NewMemory(), 1.0).WithClientShare(0.1)
	// gemini-4-argon at $10 per 1M output tokens: 6,000 tokens is $0.06.
	fake := &llm.Fake{Respond: func(llm.Request) (llm.Response, error) {
		return llm.Response{Text: "{}", Usage: llm.Usage{OutputTokens: 6000}}, nil
	}}
	model := meter.Attempts(fake)
	a := &api{d: Deps{Logger: slog.New(slog.DiscardHandler), Spend: meter, AdminToken: testAdmin, TrustedProxyCIDRs: testProxyCIDRs(t)}}
	h := a.limit(light, func(w http.ResponseWriter, r *http.Request) {
		if _, err := model.Generate(r.Context(), llm.Request{Model: "gemini-4-argon"}); err != nil {
			t.Error(err)
		}
		w.WriteHeader(http.StatusCreated)
	})
	call := func(headers map[string]string) *httptest.ResponseRecorder {
		req := httptest.NewRequest("POST", "/v1/cases/predict", nil)
		for k, v := range headers {
			req.Header.Set(k, v)
		}
		rec := httptest.NewRecorder()
		h(rec, req)
		return rec
	}
	alice, bob := map[string]string{"X-Client-IP": "203.0.113.20"}, map[string]string{"X-Client-IP": "203.0.113.21"}
	tests := []struct {
		name    string
		headers map[string]string
		want    int
	}{
		{"alice first ($0.06 of $0.10)", alice, http.StatusCreated},
		{"alice second (reaches $0.12)", alice, http.StatusCreated},
		{"alice over her share", alice, http.StatusTooManyRequests},
		{"bob still has his", bob, http.StatusCreated},
		{"admin exempt", map[string]string{"X-Client-IP": "203.0.113.20", "X-Admin-Token": testAdmin}, http.StatusCreated},
	}
	for _, tt := range tests {
		rec := call(tt.headers)
		if rec.Code != tt.want {
			t.Errorf("%s = %d %s", tt.name, rec.Code, rec.Body)
		}
		if tt.want == http.StatusTooManyRequests && (!strings.Contains(rec.Body.String(), `"client_budget_reached"`) || rec.Header().Get("Retry-After") == "") {
			t.Errorf("%s: body %s, Retry-After %q", tt.name, rec.Body, rec.Header().Get("Retry-After"))
		}
	}
	if over, total := meter.Over(context.Background()); over || total < 0.239 || total > 0.241 {
		t.Errorf("global = %v %v, want $0.24 and under the cap", over, total)
	}
}

// Invalid API keys are throttled per client before the key lookup.
func TestAPIKeyFailuresThrottled(t *testing.T) {
	hs := newHarnessWith(t, "substantial_transformation", func(d *Deps) {
		withKeys(d)
		d.Limits = Limits{LightPerMin: 100, HeavyPerMin: 100, GlobalPerMin: 1000, AuthFailuresPerMin: 3}
	})
	secret, _ := issueKey(t, hs, "Acme", 100)
	bad := map[string]string{"X-API-Key": "op_not-a-real-key", "X-Client-IP": "203.0.113.30"}
	for i := range 3 {
		if code, out := hs.doAs(t, "GET", "/v1/cases", "", bad); code != http.StatusUnauthorized {
			t.Fatalf("bad key %d = %d %v, want 401", i+1, code, out)
		}
	}
	rec := serve(hs, "GET", "/v1/cases", "", bad)
	if rec.Code != http.StatusTooManyRequests || rec.Header().Get("Retry-After") == "" || !strings.Contains(rec.Body.String(), `"auth_rate_limited"`) {
		t.Errorf("fourth bad key = %d %s", rec.Code, rec.Body)
	}
	if code, _ := hs.doAs(t, "GET", "/v1/cases", "", map[string]string{"X-API-Key": secret, "X-Client-IP": "203.0.113.30"}); code != http.StatusTooManyRequests {
		t.Errorf("any key from the throttled client = %d, want 429 (checked before the lookup)", code)
	}
	if code, _ := hs.doAs(t, "GET", "/v1/cases", "", map[string]string{"X-API-Key": secret, "X-Client-IP": "203.0.113.31"}); code != http.StatusOK {
		t.Errorf("valid key from another client = %d", code)
	}
	if code, _ := hs.doAs(t, "GET", "/v1/cases", "", map[string]string{"X-API-Key": "op_bad", "X-Client-IP": "203.0.113.32"}); code != http.StatusUnauthorized {
		t.Errorf("bad key from another client = %d, want 401", code)
	}
}

// countingSlowStore models a key-store round trip: every Get takes long
// enough for other requests with the same address to reach the limiter.
type countingSlowStore struct {
	*apikey.Memory
	mu    sync.Mutex
	gets  int
	delay time.Duration
}

func (s *countingSlowStore) Get(ctx context.Context, id string) (apikey.Key, error) {
	s.mu.Lock()
	s.gets++
	s.mu.Unlock()
	time.Sleep(s.delay)
	return s.Memory.Get(ctx, id)
}

func (s *countingSlowStore) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.gets
}

// #386: the failures bucket is checked before the lookup, but a concurrent
// burst must not all see it empty first. With a slow store, at most
// AuthFailuresPerMin unknown keys from one address reach the store.
func TestAPIKeyFailuresThrottledUnderConcurrency(t *testing.T) {
	store := &countingSlowStore{Memory: apikey.NewMemory(), delay: 20 * time.Millisecond}
	hs := newHarnessWith(t, "substantial_transformation", func(d *Deps) {
		d.APIKeys = apikey.New(store)
		d.Limits = Limits{BasicPerMin: 1000, LightPerMin: 1000, HeavyPerMin: 1000, GlobalPerMin: 10000, AuthFailuresPerMin: 10}
	})
	const n = 50
	statuses := make([]int, n)
	var wg sync.WaitGroup
	for i := range n {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			secret := "op_" + strings.Repeat("a", 43-len(strconv.Itoa(i))) + strconv.Itoa(i)
			statuses[i] = serve(hs, "GET", "/v1/cases", "", map[string]string{"X-API-Key": secret, "X-Client-IP": "203.0.113.44"}).Code
		}(i)
	}
	wg.Wait()
	var limited int
	for _, code := range statuses {
		switch code {
		case http.StatusUnauthorized:
		case http.StatusTooManyRequests:
			limited++
		default:
			t.Errorf("status = %d, want 401 or 429", code)
		}
	}
	if got := store.count(); got > 10 {
		t.Errorf("key-store lookups = %d, want at most 10", got)
	}
	if limited == 0 {
		t.Error("no request was refused with 429 auth_rate_limited")
	}
}

// #505: guesses that arrive a few milliseconds apart, while earlier lookups
// are still in the store, must not reach it more than AuthFailuresPerMin
// times either: an in-flight lookup holds a token until it finishes.
func TestAPIKeyFailuresThrottledWhenStaggered(t *testing.T) {
	for _, gap := range []time.Duration{0, 2 * time.Millisecond, 5 * time.Millisecond, 10 * time.Millisecond} {
		store := &countingSlowStore{Memory: apikey.NewMemory(), delay: 50 * time.Millisecond}
		hs := newHarnessWith(t, "substantial_transformation", func(d *Deps) {
			d.APIKeys = apikey.New(store)
			d.Limits = Limits{BasicPerMin: 1000, LightPerMin: 1000, HeavyPerMin: 1000, GlobalPerMin: 10000, AuthFailuresPerMin: 10}
		})
		const n = 60
		statuses := make([]int, n)
		var wg sync.WaitGroup
		for i := range n {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				secret := "op_" + strings.Repeat("b", 43-len(strconv.Itoa(i))) + strconv.Itoa(i)
				statuses[i] = serve(hs, "GET", "/v1/cases", "", map[string]string{"X-API-Key": secret, "X-Client-IP": "203.0.113.45"}).Code
			}(i)
			time.Sleep(gap)
		}
		wg.Wait()
		var unauthorized, limited int
		for _, code := range statuses {
			switch code {
			case http.StatusUnauthorized:
				unauthorized++
			case http.StatusTooManyRequests:
				limited++
			default:
				t.Errorf("gap %v: status = %d, want 401 or 429", gap, code)
			}
		}
		if got := store.count(); got > 10 || unauthorized > 10 {
			t.Errorf("gap %v: %d key-store lookups, %d x 401; want at most 10", gap, got, unauthorized)
		}
		if limited < n-10 {
			t.Errorf("gap %v: %d x 429, want at least %d", gap, limited, n-10)
		}
	}
}

// A failure recorded when the bucket is already spent still lengthens the
// wait (#505): fail always takes a token, even into debt.
func TestAuthFailuresDebtLengthensWait(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	f := newAuthFailures(10)
	f.now = func() time.Time { return now }
	for range 10 {
		release, ok, _ := f.begin("c")
		if !ok {
			t.Fatal("refused within the budget")
		}
		f.fail("c")
		release()
	}
	_, ok, wait1 := f.begin("c")
	if ok {
		t.Fatal("11th lookup admitted")
	}
	f.fail("c") // a late failure from a lookup admitted earlier
	_, _, wait2 := f.begin("c")
	if wait2 <= wait1 {
		t.Errorf("wait after a failure into debt = %v, want more than %v", wait2, wait1)
	}
}

// failingUse is an API-key store whose usage counter is down.
type failingUse struct{ *apikey.Memory }

func (failingUse) Use(context.Context, string, string) (int64, error) {
	return 0, errors.New("usage store down")
}

// With the quota counter down, requests are served (fail open) but the key
// still stops at its quota on this instance.
func TestAPIKeyQuotaFailOpenIsCappedLocally(t *testing.T) {
	hs := newHarnessWith(t, "substantial_transformation", func(d *Deps) {
		d.APIKeys = apikey.New(failingUse{apikey.NewMemory()})
	})
	secret, _ := issueKey(t, hs, "Acme", 2)
	key := map[string]string{"X-API-Key": secret}
	var codes []int
	for range 3 {
		code, _ := hs.doAs(t, "GET", "/v1/cases", "", key)
		codes = append(codes, code)
	}
	if codes[0] != http.StatusOK || codes[1] != http.StatusOK || codes[2] != http.StatusTooManyRequests {
		t.Errorf("codes with the counter down = %v, want 200 200 429", codes)
	}
}

// Every API answer carries nosniff and no-referrer, /v1 answers are not
// cached, and no CORS header is ever sent.
func TestSecurityHeaders(t *testing.T) {
	hs := newHarness(t, "substantial_transformation")
	tests := []struct {
		method, path string
		noStore      bool
	}{
		{"GET", "/health", false},
		{"GET", "/v1/cases", true},
		{"GET", "/v1/demos", true},
		{"GET", "/v1/nope", true},
		{"OPTIONS", "/v1/cases", true},
	}
	for _, tt := range tests {
		req := httptest.NewRequest(tt.method, tt.path, nil)
		req.Header.Set("Origin", "https://evil.example")
		rec := httptest.NewRecorder()
		hs.h.ServeHTTP(rec, req)
		h := rec.Header()
		if h.Get("X-Content-Type-Options") != "nosniff" || h.Get("Referrer-Policy") != "no-referrer" {
			t.Errorf("%s %s headers = %v", tt.method, tt.path, h)
		}
		if got := h.Get("Cache-Control") == "no-store"; got != tt.noStore {
			t.Errorf("%s %s Cache-Control = %q", tt.method, tt.path, h.Get("Cache-Control"))
		}
		for k := range h {
			if strings.HasPrefix(strings.ToLower(k), "access-control-") {
				t.Errorf("%s %s sends CORS header %s", tt.method, tt.path, k)
			}
		}
	}
}

// REQUIRE_BEARER refuses requests without a bearer token, except /health;
// off (the default) nothing changes.
func TestRequireBearer(t *testing.T) {
	on := newHarnessWith(t, "substantial_transformation", func(d *Deps) { d.RequireBearer = true })
	off := newHarness(t, "substantial_transformation")
	tests := []struct {
		name   string
		hs     harness
		path   string
		header string
		want   int
	}{
		{"on, no token", on, "/v1/demos", "", http.StatusUnauthorized},
		{"on, wrong scheme", on, "/v1/demos", "Basic abc", http.StatusUnauthorized},
		{"on, empty bearer", on, "/v1/demos", "Bearer ", http.StatusUnauthorized},
		{"on, bearer", on, "/v1/demos", "Bearer eyJ.x.y", http.StatusOK},
		{"on, lower-case scheme", on, "/v1/demos", "bearer eyJ.x.y", http.StatusOK},
		{"on, health without token", on, "/health", "", http.StatusOK},
		{"off, no token", off, "/v1/demos", "", http.StatusOK},
	}
	for _, tt := range tests {
		h := map[string]string{}
		if tt.header != "" {
			h["Authorization"] = tt.header
		}
		if code, out := tt.hs.doAs(t, "GET", tt.path, "", h); code != tt.want {
			t.Errorf("%s = %d %v, want %d", tt.name, code, out, tt.want)
		}
	}
}

// The cheap routes run under a deadline.
func TestCheapRouteDeadline(t *testing.T) {
	a := &api{d: Deps{Logger: slog.New(slog.DiscardHandler)}}
	var deadline time.Time
	var ok bool
	a.cheap(func(_ http.ResponseWriter, r *http.Request) { deadline, ok = r.Context().Deadline() })(httptest.NewRecorder(), httptest.NewRequest("GET", "/v1/cases", nil))
	if !ok || time.Until(deadline) > basicDeadline || time.Until(deadline) < basicDeadline-5*time.Second {
		t.Errorf("deadline %v %v, want about %v from now", deadline, ok, basicDeadline)
	}
}

// An MCP tool call re-enters the API in-process and goes through its /v1
// route's limits there; the POST /mcp itself only takes the basic class.
// So the per-client caps apply once per tool call, never twice.
func TestMCPCallsCountOncePerToolCall(t *testing.T) {
	t.Run("in flight: one slot per call", func(t *testing.T) {
		hs := newHarnessWith(t, "substantial_transformation", func(d *Deps) {
			d.Limits = Limits{BasicPerMin: 100, LightPerMin: 100, HeavyPerMin: 100, GlobalPerMin: 1000, MaxInFlight: 1}
		})
		cs := mcpClient(t, hs, map[string]string{"X-Client-IP": "198.51.100.40"})
		for i := range 2 { // a second slot taken by the /mcp request would refuse the first call
			if res, _ := callTool(t, cs, "predict_origin", heatGunArgs(t)); res.IsError {
				t.Fatalf("predict_origin %d with one slot: %s", i+1, text(res))
			}
		}
		if code, _ := hs.doAs(t, "POST", "/v1/cases/predict", heatGunBody(t), map[string]string{"X-Client-IP": "198.51.100.40"}); code != http.StatusCreated {
			t.Errorf("REST predict after the MCP calls = %d, want the slot released", code)
		}
	})
	t.Run("failed keys: one failure per call", func(t *testing.T) {
		hs := newHarnessWith(t, "substantial_transformation", func(d *Deps) {
			withKeys(d)
			d.Limits = Limits{BasicPerMin: 100, LightPerMin: 100, HeavyPerMin: 100, GlobalPerMin: 1000, AuthFailuresPerMin: 2}
		})
		cs := mcpClient(t, hs, map[string]string{"X-Client-IP": "198.51.100.41", "Authorization": "Bearer op_not-a-real-key"})
		want := []string{"401", "401", "429 auth_rate_limited"}
		for i, w := range want {
			if res, _ := callTool(t, cs, "predict_origin", heatGunArgs(t)); !res.IsError || !strings.Contains(text(res), w) {
				t.Errorf("call %d = %s, want %s", i+1, text(res), w)
			}
		}
	})
	t.Run("spend share", func(t *testing.T) {
		meter := spend.New(spend.NewMemory(), 1).WithClientShare(0.1)
		meter.Add(spend.WithClient(context.Background(), "198.51.100.42"), 0.2)
		hs := newHarnessWith(t, "substantial_transformation", func(d *Deps) { d.Spend = meter })
		if res, _ := callTool(t, mcpClient(t, hs, map[string]string{"X-Client-IP": "198.51.100.42"}), "predict_origin", heatGunArgs(t)); !res.IsError || !strings.Contains(text(res), "client_budget_reached") {
			t.Errorf("over the share = %s", text(res))
		}
		if res, _ := callTool(t, mcpClient(t, hs, map[string]string{"X-Client-IP": "198.51.100.43"}), "predict_origin", heatGunArgs(t)); res.IsError {
			t.Errorf("another client = %s", text(res))
		}
	})
}

// POST /mcp takes the basic class and carries the security headers; with
// REQUIRE_BEARER on, its in-process tool calls are not refused for lacking
// the bearer the MCP guard removes.
func TestMCPPostLimitsAndHeaders(t *testing.T) {
	hs := newHarnessWith(t, "substantial_transformation", func(d *Deps) {
		d.Limits = Limits{BasicPerMin: 2, LightPerMin: 100, HeavyPerMin: 100, GlobalPerMin: 1000}
	})
	init := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"t","version":"0"}}}`
	hdr := map[string]string{"X-Client-IP": "198.51.100.44", "Content-Type": "application/json", "Accept": "application/json, text/event-stream"}
	rec := serve(hs, "POST", "/mcp", init, hdr)
	h := rec.Header()
	if rec.Code != http.StatusOK || h.Get("RateLimit-Limit") != "2" || h.Get("RateLimit-Remaining") != "1" || h.Get("X-Content-Type-Options") != "nosniff" || !strings.HasPrefix(h.Get("Cache-Control"), "no-") { // no-store, or the stream's no-cache
		t.Errorf("first /mcp = %d %v", rec.Code, h)
	}
	serve(hs, "POST", "/mcp", init, hdr)
	if rec := serve(hs, "POST", "/mcp", init, hdr); rec.Code != http.StatusTooManyRequests || rec.Header().Get("Retry-After") == "" {
		t.Errorf("third /mcp = %d %v, want 429", rec.Code, rec.Header())
	}

	strict := newHarnessWith(t, "substantial_transformation", func(d *Deps) { d.RequireBearer = true })
	cs := mcpClient(t, strict, map[string]string{"X-Client-IP": "198.51.100.45", "Authorization": "Bearer eyJ.platform.token"})
	if res, _ := callTool(t, cs, "predict_origin", heatGunArgs(t)); res.IsError {
		t.Errorf("tool call behind REQUIRE_BEARER = %s", text(res))
	}
	if rec := serve(strict, "POST", "/mcp", init, hdr); rec.Code != http.StatusUnauthorized {
		t.Errorf("/mcp without a bearer under REQUIRE_BEARER = %d, want 401", rec.Code)
	}
}
