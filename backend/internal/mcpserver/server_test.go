package mcpserver

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// stubAPI records the /v1 requests the tools make and answers from a table.
type stubAPI struct {
	mu   sync.Mutex
	seen []*http.Request
	body map[string]string // "METHOD path" -> JSON
	code map[string]int
}

func (s *stubAPI) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	s.seen = append(s.seen, r)
	s.mu.Unlock()
	k := r.Method + " " + r.URL.Path
	code := s.code[k]
	if code == 0 {
		code = http.StatusOK
	}
	b, ok := s.body[k]
	if !ok {
		code, b = http.StatusNotFound, `{"code":"not_found","message":"case not found"}`
	}
	w.Header().Set("Content-Type", "application/json")
	if code == http.StatusTooManyRequests {
		w.Header().Set("Retry-After", "7")
	}
	w.WriteHeader(code)
	_, _ = w.Write([]byte(b))
}

func (s *stubAPI) last() *http.Request {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.seen[len(s.seen)-1]
}

// connect serves New(...) over httptest and connects a client that sends
// headers on every request.
func connect(t *testing.T, api http.Handler, search bool, headers map[string]string) *mcp.ClientSession {
	t.Helper()
	return connectWith(t, Options{API: api, Families: []string{"motorised_tools_small_appliances"}, RulingSearch: search}, headers)
}

func connectOpts(t *testing.T, o Options) *mcp.ClientSession {
	t.Helper()
	o.Families = []string{"motorised_tools_small_appliances"}
	return connectWith(t, o, nil)
}

func connectWith(t *testing.T, o Options, headers map[string]string) *mcp.ClientSession {
	t.Helper()
	if o.ClientAddress == nil {
		// These tests act as the frontend proxy: a caller-supplied address
		// is honoured. The direct-client default is pinned separately.
		o.ClientAddress = func(remote, ip string) string {
			if ip = strings.TrimSpace(ip); ip != "" {
				return ip
			}
			if host, _, err := net.SplitHostPort(remote); err == nil {
				return host
			}
			return remote
		}
	}
	srv := httptest.NewServer(New(o))
	t.Cleanup(srv.Close)
	hc := &http.Client{Transport: rt(func(r *http.Request) (*http.Response, error) {
		r = r.Clone(r.Context())
		for k, v := range headers {
			r.Header.Set(k, v)
		}
		return http.DefaultTransport.RoundTrip(r)
	})}
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "t", Version: "0"}, nil).Connect(context.Background(),
		&mcp.StreamableClientTransport{Endpoint: srv.URL, HTTPClient: hc, DisableStandaloneSSE: true}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cs.Close() })
	return cs
}

type rt func(*http.Request) (*http.Response, error)

func (f rt) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func call(t *testing.T, cs *mcp.ClientSession, name string, args map[string]any) (*mcp.CallToolResult, string, map[string]any) {
	t.Helper()
	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	var txt strings.Builder
	for _, c := range res.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			txt.WriteString(tc.Text)
		}
	}
	var out map[string]any
	b, _ := json.Marshal(res.StructuredContent)
	_ = json.Unmarshal(b, &out)
	return res, txt.String(), out
}

const predicted = `{"id":"case_0123456789ab","prediction":{"origin_country":"Thailand","confidence":0.97,"cited_rulings":["N300001"],"trace":{"tokens":9}},"triage":{"route":"auto","reason":"fast path"},"history":[{"kind":"predicted"}],"disclaimer":"Not a ruling. Confirm with a licensed customs broker or a CBP binding ruling."}`

// #479: the guard must decide the client address before tools copy it into
// their in-process requests. Without a trusted-proxy rule the connection
// wins; with one, the header is used and is the only value tools see.
func TestGuardResolvesTheClientAddressOnce(t *testing.T) {
	for _, tc := range []struct {
		name string
		o    Options
		want string
	}{
		{"direct caller without a trusted proxy", Options{}, "203.0.113.9"},
		{"trusted proxy header", Options{ClientAddress: func(_, ip string) string { return ip }}, "198.51.100.1"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var seen *http.Request
			h := guard(tc.o, http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
				seen = r.Clone(r.Context())
			}))
			req := httptest.NewRequest("POST", "/mcp", nil)
			req.RemoteAddr = "203.0.113.9:4567"
			req.Header.Set("X-Client-IP", "198.51.100.1")
			h.ServeHTTP(httptest.NewRecorder(), req)
			if got := seen.Header.Get("X-Client-IP"); got != tc.want {
				t.Errorf("tool client address = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestToolsListedAndSearchOptional(t *testing.T) {
	for _, search := range []bool{false, true} {
		cs := connect(t, &stubAPI{}, search, nil)
		lt, err := cs.ListTools(context.Background(), nil)
		if err != nil {
			t.Fatal(err)
		}
		names := map[string]bool{}
		for _, tool := range lt.Tools {
			names[tool.Name] = true
		}
		if names["search_rulings"] != search || len(lt.Tools) != 11+map[bool]int{false: 0, true: 1}[search] {
			t.Errorf("search %v: tools %v", search, names)
		}
	}
}

func TestPredictForwardsOnlyIdentityAndSummarises(t *testing.T) {
	api := &stubAPI{body: map[string]string{"POST /v1/cases/predict": predicted}, code: map[string]int{"POST /v1/cases/predict": 201}}
	cs := connect(t, api, false, map[string]string{
		"Authorization": "Bearer op_abc", "X-User-Token": "user-jwt", "X-Visitor-ID": "visitor-0123456789abcdef", "Cookie": "op_visitor=x", "X-Review-Token": "ubr_link", "X-Client-IP": "198.51.100.1",
	})
	res, txt, out := call(t, cs, "predict_origin", map[string]any{"facts": "A heat gun assembled in Thailand from Chinese parts."})
	if res.IsError {
		t.Fatal(txt)
	}
	r := api.last()
	if r.Header.Get("X-API-Key") != "op_abc" || r.Header.Get("X-Client-IP") != "198.51.100.1" {
		t.Errorf("identity not forwarded: %v", r.Header)
	}
	for _, h := range []string{"Authorization", "X-User-Token", "X-Visitor-ID", "X-Review-Token", "Cookie", "X-Admin-Token"} {
		if r.Header.Get(h) != "" {
			t.Errorf("%s forwarded to the API", h)
		}
	}
	if !strings.Contains(txt, "Likely origin: Thailand (confidence 97% as stated by the model, not calibrated)") || !strings.Contains(txt, "N300001") || !strings.Contains(txt, "case_0123456789ab") || !strings.HasSuffix(txt, Disclaimer) {
		t.Errorf("summary: %s", txt)
	}
	if out["history"] != nil || obj(out["prediction"])["trace"] != nil || out["case_access_key"] != nil {
		t.Errorf("structured content not trimmed, or an access key for a key caller: %v", out)
	}
}

func TestAnonymousPredictGetsAnAccessKey(t *testing.T) {
	api := &stubAPI{body: map[string]string{"POST /v1/cases/predict": predicted, "GET /v1/cases/case_0123456789ab": predicted}}
	cs := connect(t, api, false, nil)
	_, txt, out := call(t, cs, "predict_origin", map[string]any{"facts": "A heat gun assembled in Thailand from Chinese parts."})
	key, _ := out["case_access_key"].(string)
	if !strings.HasPrefix(key, "mcp_") || api.last().Header.Get("X-Visitor-ID") != key || !strings.Contains(txt, "case_access_key") {
		t.Fatalf("access key %q, sent %q", key, api.last().Header.Get("X-Visitor-ID"))
	}
	if api.last().Header.Get("X-Client-IP") == "" {
		t.Error("no client address for the anonymous rate limit")
	}
	call(t, cs, "get_case", map[string]any{"case_id": "case_0123456789ab", "case_access_key": key})
	if api.last().Header.Get("X-Visitor-ID") != key {
		t.Error("access key not sent on get_case")
	}
	// A given key is reused, not replaced.
	_, _, out = call(t, cs, "predict_origin", map[string]any{"facts": "A heat gun assembled in Thailand from Chinese parts.", "case_access_key": key})
	if out["case_access_key"] != key {
		t.Errorf("reused key = %v", out["case_access_key"])
	}
}

func TestAPIErrorsBecomeToolErrors(t *testing.T) {
	api := &stubAPI{body: map[string]string{"POST /v1/cases/case_x/plan": `{"code":"rate_limited","message":"too many requests; try again in 7 s"}`}, code: map[string]int{"POST /v1/cases/case_x/plan": 429}}
	cs := connect(t, api, false, nil)
	res, txt, out := call(t, cs, "plan_production_moves", map[string]any{"case_id": "case_x", "target_country": "Thailand"})
	if !res.IsError || !strings.Contains(txt, "429 rate_limited") || !strings.Contains(txt, "Retry after 7 seconds") || out["status"] != 429.0 {
		t.Errorf("429: %s %v", txt, out)
	}
	res, txt, _ = call(t, cs, "get_case", map[string]any{"case_id": "case_y"})
	if !res.IsError || !strings.Contains(txt, "not visible to this caller") {
		t.Errorf("404: %s", txt)
	}
}

func TestExportIsAPreview(t *testing.T) {
	var got map[string]any
	api := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&got)
		_, _ = w.Write([]byte(`{"case_id":"c","format":"markdown","content":"# Request for a binding ruling","notes":["Draft for review."]}`))
	})
	cs := connect(t, api, false, nil)
	_, txt, _ := call(t, cs, "export_ruling_request_draft", map[string]any{"case_id": "c", "plan_index": 0})
	if got["preview"] != true || got["plan_index"] != 0.0 || !strings.HasPrefix(txt, "# Request for a binding ruling") || !strings.Contains(txt, "Draft for review.") {
		t.Errorf("export body %v, text %s", got, txt)
	}
}

func TestGuard(t *testing.T) {
	h := New(Options{API: &stubAPI{}})
	for name, hd := range map[string]map[string]string{
		"admin token":     {"X-Admin-Token": "anything"},
		"two keys differ": {"X-API-Key": "op_a", "Authorization": "Bearer op_b"},
	} {
		req := httptest.NewRequest("POST", "/", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json, text/event-stream")
		for k, v := range hd {
			req.Header.Set(k, v)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusForbidden && rec.Code != http.StatusBadRequest {
			t.Errorf("%s: %d", name, rec.Code)
		}
	}
	if k, ok := bearerKey("bearer op_x"); !ok || k != "op_x" {
		t.Error("lower-case bearer scheme")
	}
	if _, ok := bearerKey("Bearer eyJhbGciOi.identity.token"); ok {
		t.Error("a platform identity token was read as an API key")
	}
}

func TestConfPhraseNamesCalibration(t *testing.T) {
	if got := confPhrase(map[string]any{"confidence": 0.97, "calibrated_confidence": 0.9355}); got != "calibrated confidence 94%; the model stated 97%" {
		t.Errorf("calibrated: %q", got)
	}
	if got := confPhrase(map[string]any{"confidence": 0.97}); got != "confidence 97% as stated by the model, not calibrated" {
		t.Errorf("stated only: %q", got)
	}
}

type outerKey struct{}

// A tool call's request carries none of the MCP request's context values
// (#385): the API's middleware runs on POST /mcp first and stores a verified
// identity there, which must not reach the tool call. Cancellation still
// reaches it.
func TestToolRequestsDoNotInheritContextValues(t *testing.T) {
	type seen struct {
		value any
		done  bool
	}
	got := make(chan seen, 1)
	api := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got <- seen{value: r.Context().Value(outerKey{}), done: r.Context().Done() != nil}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"cases":[]}`))
	})
	inner := New(Options{API: api, Families: []string{"motorised_tools_small_appliances"}})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		inner.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), outerKey{}, "signed-in user")))
	}))
	t.Cleanup(srv.Close)
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "t", Version: "0"}, nil).Connect(context.Background(),
		&mcp.StreamableClientTransport{Endpoint: srv.URL, DisableStandaloneSSE: true}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cs.Close() })
	call(t, cs, "list_cases", nil)
	s := <-got
	if s.value != nil {
		t.Errorf("the tool request inherited %v from the MCP request's context", s.value)
	}
	if !s.done {
		t.Error("the tool request cannot be cancelled")
	}
	ctx, cancel := context.WithCancel(context.WithValue(context.Background(), outerKey{}, 1))
	iso := isolated(ctx)
	if v := iso.Value(outerKey{}); v != nil {
		t.Errorf("isolated kept a value: %v", v)
	}
	cancel()
	if iso.Err() == nil {
		t.Error("isolated is not cancelled with its parent")
	}
}
