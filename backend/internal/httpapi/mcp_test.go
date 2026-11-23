package httpapi

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/extract"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/llm"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/spend"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// MCP endpoint (docs/MCP.md): every tool call goes through the /v1 route it
// wraps, so it is limited, counted and isolated exactly like the REST call.

// mcpClient connects an MCP client to hs over Streamable HTTP, sending the
// given headers on every request.
func mcpClient(t *testing.T, hs harness, headers map[string]string) *mcp.ClientSession {
	t.Helper()
	srv := httptest.NewServer(hs.h)
	t.Cleanup(srv.Close)
	hc := &http.Client{Transport: headerRT{headers: headers, next: http.DefaultTransport}}
	c := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "0"}, nil)
	cs, err := c.Connect(context.Background(), &mcp.StreamableClientTransport{Endpoint: srv.URL + "/mcp", HTTPClient: hc, DisableStandaloneSSE: true}, nil)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = cs.Close() })
	return cs
}

type headerRT struct {
	headers map[string]string
	next    http.RoundTripper
}

func (h headerRT) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	for k, v := range h.headers {
		r.Header.Set(k, v)
	}
	return h.next.RoundTrip(r)
}

// heatGunArgs is the heat-gun fixture as predict_origin arguments.
func heatGunArgs(t *testing.T) map[string]any {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "testdata", "heatgun_2022.json"))
	if err != nil {
		t.Fatal(err)
	}
	var fx map[string]any
	_ = json.Unmarshal(b, &fx)
	return map[string]any{"date": fx["date"], "product": fx["product"], "family": fx["family"], "current_origin": fx["current_origin"]}
}

func callTool(t *testing.T, cs *mcp.ClientSession, name string, args map[string]any) (*mcp.CallToolResult, map[string]any) {
	t.Helper()
	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	var out map[string]any
	if res.StructuredContent != nil {
		b, _ := json.Marshal(res.StructuredContent)
		_ = json.Unmarshal(b, &out)
	}
	return res, out
}

func text(res *mcp.CallToolResult) string {
	var b strings.Builder
	for _, c := range res.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			b.WriteString(tc.Text)
		}
	}
	return b.String()
}

func TestMCPListsToolsWithTightSchemas(t *testing.T) {
	hs := newHarnessWith(t, "substantial_transformation", func(d *Deps) { d.RulingSearch = fakeSearch{} })
	cs := mcpClient(t, hs, nil)
	lt, err := cs.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{ // name -> read-only
		"predict_origin": false, "extract_product": true, "plan_production_moves": false, "compare_target_countries": false,
		"search_rulings": true, "get_ruling": true, "get_duty_rates": true, "get_case": true, "list_cases": true, "export_ruling_request_draft": true,
		"create_broker_review_link": false, "get_broker_review_status": true, "check_rcep_preference": true, "check_usmca_origin_rule": true,
	}
	if len(lt.Tools) != len(want) {
		t.Errorf("listed %d tools, want %d", len(lt.Tools), len(want))
	}
	for _, tool := range lt.Tools {
		ro, ok := want[tool.Name]
		if !ok {
			t.Errorf("unexpected tool %q", tool.Name)
			continue
		}
		if tool.Annotations == nil || tool.Annotations.ReadOnlyHint != ro || tool.Annotations.DestructiveHint == nil || *tool.Annotations.DestructiveHint {
			t.Errorf("%s annotations = %+v, want readOnly %v and destructive false", tool.Name, tool.Annotations, ro)
		}
		b, _ := json.Marshal(tool.InputSchema)
		s := string(b)
		if !strings.Contains(s, `"additionalProperties":false`) {
			t.Errorf("%s: input schema is not closed: %s", tool.Name, s)
		}
		if !strings.Contains(tool.Description, "REST:") {
			t.Errorf("%s: description does not name its REST operation", tool.Name)
		}
	}
	// suggest_part_hts is listed only when the route is served.
	ps := partsHarness(t, llm.FakeText(`{"items":[]}`), nil)
	lp, err := mcpClient(t, ps, nil).ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	listed := false
	for _, tool := range lp.Tools {
		if tool.Name == "suggest_part_hts" {
			listed = true
			if tool.Annotations == nil || !tool.Annotations.ReadOnlyHint || !strings.Contains(tool.Description, "REST:") {
				t.Errorf("suggest_part_hts = %+v", tool)
			}
		}
	}
	if !listed {
		t.Error("suggest_part_hts is not listed although the route is served")
	}
	// The family enum comes from the API's templates.
	for _, tool := range lt.Tools {
		if tool.Name == "predict_origin" {
			b, _ := json.Marshal(tool.InputSchema)
			if !strings.Contains(string(b), "motorised_tools_small_appliances") || !strings.Contains(string(b), `"maxLength":12000`) {
				t.Errorf("predict_origin schema lacks the family enum or the facts cap: %s", b)
			}
		}
	}
}

func TestMCPSchemaRefusesBadInputBeforeAnyModelCall(t *testing.T) {
	hs := newHarness(t, "substantial_transformation")
	cs := mcpClient(t, hs, nil)
	for name, args := range map[string]map[string]any{
		"facts too long":  {"facts": strings.Repeat("a", 12001)},
		"neither":         {"family": "motorised_tools_small_appliances"},
		"unknown field":   {"facts": "A heat gun assembled in Thailand from Chinese parts.", "admin": true},
		"bad family":      {"facts": "A heat gun assembled in Thailand from Chinese parts.", "family": "rockets"},
		"bad date":        {"facts": "A heat gun assembled in Thailand from Chinese parts.", "date": "yesterday"},
		"bad access key":  {"facts": "A heat gun assembled in Thailand from Chinese parts.", "case_access_key": "short"},
		"too many plants": {"product": map[string]any{"name": "x", "components": []any{map[string]any{"name": "a", "made_in": "China"}}, "plants": make([]any, 51)}},
	} {
		res, _ := callTool(t, cs, "predict_origin", args)
		if !res.IsError {
			t.Errorf("%s: accepted", name)
		}
	}
	if res, _ := callTool(t, cs, "compare_target_countries", map[string]any{"case_id": "case_1", "targets": []any{"Thailand", "Thailand"}}); !res.IsError {
		t.Error("duplicate targets accepted")
	}
	if res, _ := callTool(t, cs, "get_ruling", map[string]any{"ruling_id": "../admin"}); !res.IsError {
		t.Error("a non-ruling id reached the API")
	}
	if hs.engine.calls != 0 {
		t.Errorf("the engine ran %d times on refused input", hs.engine.calls)
	}
}

// A predict_origin call over MCP creates the case under the API key from the
// Authorization header, counts against that key's quota and returns the
// disclaimer; another key cannot read the case.
func TestMCPBearerKeyOwnsCasesAndKeysAreIsolated(t *testing.T) {
	hs := newHarnessWith(t, "substantial_transformation", func(d *Deps) { withKeys(d); d.CaseOwnership = true })
	acme, _ := issueKey(t, hs, "Acme", 3)
	beta, _ := issueKey(t, hs, "Beta", 100)

	a := mcpClient(t, hs, map[string]string{"Authorization": "Bearer " + acme})
	res, out := callTool(t, a, "predict_origin", heatGunArgs(t))
	if res.IsError {
		t.Fatalf("predict_origin: %s", text(res))
	}
	id, _ := out["id"].(string)
	if id == "" || out["case_access_key"] != nil {
		t.Fatalf("case id %q, access key %v (a key caller needs none)", id, out["case_access_key"])
	}
	if !strings.Contains(text(res), "Not a ruling") || out["disclaimer"] == nil {
		t.Errorf("no disclaimer: %s", text(res))
	}
	if code, _ := hs.doAs(t, "GET", "/v1/cases/"+id, "", map[string]string{"X-API-Key": acme}); code != http.StatusOK {
		t.Errorf("REST read by the same key = %d", code)
	}

	b := mcpClient(t, hs, map[string]string{"X-API-Key": beta})
	if res, _ := callTool(t, b, "get_case", map[string]any{"case_id": id}); !res.IsError || !strings.Contains(text(res), "404") {
		t.Errorf("key B read key A's case: %s", text(res))
	}
	if res, _ := callTool(t, b, "plan_production_moves", map[string]any{"case_id": id, "target_country": "Thailand"}); !res.IsError {
		t.Errorf("key B planned on key A's case: %s", text(res))
	}
	_, list := callTool(t, b, "list_cases", nil)
	if strings.Contains(mustJSON(list), id) {
		t.Error("key B lists key A's case")
	}
	anon := mcpClient(t, hs, nil)
	if res, _ := callTool(t, anon, "get_case", map[string]any{"case_id": id}); !res.IsError {
		t.Error("an anonymous caller read a key's case")
	}

	// The quota of 3: predict above, now get_case and list_cases, then refused.
	callTool(t, a, "get_case", map[string]any{"case_id": id})
	callTool(t, a, "list_cases", nil)
	res, _ = callTool(t, a, "list_cases", nil)
	if !res.IsError || !strings.Contains(text(res), "quota_exceeded") {
		t.Errorf("4th call on a quota of 3: %s", text(res))
	}

	bad := mcpClient(t, hs, map[string]string{"Authorization": "Bearer op_not_a_real_key_0123456789abcdef0123456"})
	if res, _ := callTool(t, bad, "list_cases", nil); !res.IsError || !strings.Contains(text(res), "401 unauthorized") {
		t.Errorf("unknown key: %s", text(res))
	}
}

// Anonymous callers get a case access key; another key cannot see the case.
func TestMCPAnonymousCaseAccessKey(t *testing.T) {
	hs := newHarnessWith(t, "substantial_transformation", func(d *Deps) { d.CaseOwnership = true })
	cs := mcpClient(t, hs, nil)
	res, out := callTool(t, cs, "predict_origin", heatGunArgs(t))
	if res.IsError {
		t.Fatalf("predict: %s", text(res))
	}
	id, key := out["id"].(string), out["case_access_key"].(string)
	if !strings.HasPrefix(key, "mcp_") {
		t.Fatalf("access key = %q", key)
	}
	if res, _ := callTool(t, cs, "get_case", map[string]any{"case_id": id}); !res.IsError {
		t.Error("read without the access key (ownership on)")
	}
	if res, _ := callTool(t, cs, "get_case", map[string]any{"case_id": id, "case_access_key": key}); res.IsError {
		t.Errorf("read with the access key: %s", text(res))
	}
	other := "mcp_" + strings.Repeat("0", 48)
	if res, _ := callTool(t, cs, "get_case", map[string]any{"case_id": id, "case_access_key": other}); !res.IsError {
		t.Error("another access key read the case")
	}
	if res, _ := callTool(t, cs, "plan_production_moves", map[string]any{"case_id": id, "target_country": "Thailand", "case_access_key": key}); res.IsError {
		t.Errorf("plan with the access key: %s", text(res))
	}
	if res, _ := callTool(t, cs, "export_ruling_request_draft", map[string]any{"case_id": id, "case_access_key": key}); res.IsError || !strings.Contains(text(res), "Not a ruling") {
		t.Errorf("export: %s", text(res))
	}
	_, l := callTool(t, cs, "list_cases", map[string]any{"case_access_key": key})
	if !strings.Contains(mustJSON(l), id) {
		t.Error("list_cases with the access key does not list the case")
	}
}

// The tool call takes from the same limiter as the REST route: the light
// bucket for predict_origin, the heavy one for plan_production_moves, and the
// global spend cap.
func TestMCPToolCallsConsumeTheRESTLimiter(t *testing.T) {
	meter := spend.New(spend.NewMemory(), 0.01)
	hs := newHarnessWith(t, "substantial_transformation", func(d *Deps) {
		d.Limits = Limits{LightPerMin: 1, HeavyPerMin: 1, GlobalPerMin: 1000}
		d.Spend = meter
	})
	ip := "198.51.100.23"
	cs := mcpClient(t, hs, map[string]string{"X-Client-IP": ip})
	res, out := callTool(t, cs, "predict_origin", heatGunArgs(t))
	if res.IsError {
		t.Fatalf("first predict_origin: %s", text(res))
	}
	id, key := out["id"].(string), out["case_access_key"].(string)
	// The MCP call used this address's only light token: REST is refused too.
	if code, body := hs.doAs(t, "POST", "/v1/cases/predict", heatGunBody(t), map[string]string{"X-Client-IP": ip}); code != http.StatusTooManyRequests {
		t.Errorf("REST predict after the MCP call = %d %v, want 429", code, body)
	}
	if res, _ := callTool(t, cs, "predict_origin", heatGunArgs(t)); !res.IsError || !strings.Contains(text(res), "429 rate_limited") || !strings.Contains(text(res), "Retry after") {
		t.Errorf("second predict_origin: %s", text(res))
	}
	// Heavy: one plan over MCP, then the REST plan is refused.
	if res, _ := callTool(t, cs, "plan_production_moves", map[string]any{"case_id": id, "target_country": "Thailand", "case_access_key": key}); res.IsError {
		t.Fatalf("plan: %s", text(res))
	}
	if code, _ := hs.doAs(t, "POST", "/v1/cases/"+id+"/plan", `{"target":"Thailand"}`, map[string]string{"X-Client-IP": ip, "X-Visitor-ID": key}); code != http.StatusTooManyRequests {
		t.Errorf("REST plan after the MCP plan = %d, want 429", code)
	}
	// Spend cap.
	meter.Add(context.Background(), 0.02)
	other := mcpClient(t, hs, map[string]string{"X-Client-IP": "203.0.113.50"})
	if res, _ := callTool(t, other, "predict_origin", heatGunArgs(t)); !res.IsError || !strings.Contains(text(res), "budget_reached") {
		t.Errorf("predict over the spend cap: %s", text(res))
	}
}

// #479: with no trusted proxy, a direct /mcp caller must not be able to
// choose a fresh light or heavy bucket by changing X-Client-IP. Each client
// below is a separate connection with a different header.
func TestMCPIgnoresACallerChosenClientAddress(t *testing.T) {
	hs := newHarnessWith(t, "substantial_transformation", func(d *Deps) {
		d.TrustedProxyCIDRs = nil
		d.Limits = Limits{LightPerMin: 1, HeavyPerMin: 1, GlobalPerMin: 1000}
	})
	for i, ip := range []string{"203.0.113.1", "203.0.113.2", "203.0.113.3"} {
		res, _ := callTool(t, mcpClient(t, hs, map[string]string{"X-Client-IP": ip}), "predict_origin", heatGunArgs(t))
		if i == 0 && res.IsError {
			t.Fatalf("first predict_origin: %s", text(res))
		}
		if i > 0 && (!res.IsError || !strings.Contains(text(res), "429 rate_limited")) {
			t.Errorf("predict_origin with header %s: %s, want 429", ip, text(res))
		}
	}

	// The same rule applies to the heavy class. A wide light allowance lets
	// the first tool call create the case without using up the light bucket.
	hs = newHarnessWith(t, "substantial_transformation", func(d *Deps) {
		d.TrustedProxyCIDRs = nil
		d.Limits = Limits{LightPerMin: 100, HeavyPerMin: 1, GlobalPerMin: 1000}
	})
	res, out := callTool(t, mcpClient(t, hs, map[string]string{"X-Client-IP": "203.0.113.10"}), "predict_origin", heatGunArgs(t))
	if res.IsError {
		t.Fatalf("predict_origin for the plan: %s", text(res))
	}
	id, key := out["id"].(string), out["case_access_key"].(string)
	plan := map[string]any{"case_id": id, "target_country": "Thailand", "case_access_key": key}
	if res, _ = callTool(t, mcpClient(t, hs, map[string]string{"X-Client-IP": "203.0.113.11"}), "plan_production_moves", plan); res.IsError {
		t.Fatalf("first plan: %s", text(res))
	}
	if res, _ = callTool(t, mcpClient(t, hs, map[string]string{"X-Client-IP": "203.0.113.12"}), "plan_production_moves", plan); !res.IsError || !strings.Contains(text(res), "429 rate_limited") {
		t.Errorf("third plan with a rotated header: %s, want 429", text(res))
	}
}

func TestMCPHonoursTheHeaderFromATrustedProxy(t *testing.T) {
	hs := newHarnessWith(t, "substantial_transformation", func(d *Deps) {
		d.Limits = Limits{LightPerMin: 1, HeavyPerMin: 1, GlobalPerMin: 1000}
	})
	// The harness trusts every source range (testProxyCIDRs): two different
	// proxy-supplied addresses get two buckets, as SECURITY.md promises.
	for i, ip := range []string{"203.0.113.1", "203.0.113.2"} {
		if res, _ := callTool(t, mcpClient(t, hs, map[string]string{"X-Client-IP": ip}), "predict_origin", heatGunArgs(t)); res.IsError {
			t.Errorf("predict %d from trusted proxy: %s", i, text(res))
		}
	}
	if res, _ := callTool(t, mcpClient(t, hs, map[string]string{"X-Client-IP": "203.0.113.1"}), "predict_origin", heatGunArgs(t)); !res.IsError || !strings.Contains(text(res), "429 rate_limited") {
		t.Errorf("reused trusted address: %s, want 429", text(res))
	}
}

func TestMCPRefusesTheAdminToken(t *testing.T) {
	hs := newHarness(t, "substantial_transformation")
	srv := httptest.NewServer(hs.h)
	defer srv.Close()
	req, _ := http.NewRequest("POST", srv.URL+"/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("X-Admin-Token", testAdmin)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusForbidden {
		t.Errorf("admin token on /mcp = %d, want 403", res.StatusCode)
	}
	// A bearer that is not an API key (the admin token, say) is not forwarded:
	// the tool call is anonymous and limited like any other.
	hs = newHarnessWith(t, "substantial_transformation", func(d *Deps) { d.Limits = Limits{LightPerMin: 1, HeavyPerMin: 1, GlobalPerMin: 1000} })
	cs := mcpClient(t, hs, map[string]string{"Authorization": "Bearer " + testAdmin, "X-Client-IP": "198.51.100.99"})
	callTool(t, cs, "predict_origin", heatGunArgs(t))
	if res, _ := callTool(t, cs, "predict_origin", heatGunArgs(t)); !res.IsError || !strings.Contains(text(res), "rate_limited") {
		t.Errorf("a bearer admin token lifted the limits: %s", text(res))
	}
}

// extract_product_from_media goes through POST /v1/products/extract-media:
// its byte checks, its light rate-limit bucket and the model call behind it.
func TestMCPMediaToolUsesTheRESTRoute(t *testing.T) {
	fake := &llm.Fake{Respond: func(llm.Request) (llm.Response, error) {
		return llm.Response{Text: readAnswer, FinishReason: "STOP", Usage: llm.Usage{InputTokens: 1500, OutputTokens: 200}}, nil
	}}
	hs := newHarnessWith(t, "substantial_transformation", func(d *Deps) {
		d.MediaExtractor = extract.New(llm.WithLogging(fake, slog.New(suiteHandler)), "gemini-4-argon")
		d.Limits = Limits{LightPerMin: 1, HeavyPerMin: 1, GlobalPerMin: 1000}
	})
	ip := "198.51.100.61"
	cs := mcpClient(t, hs, map[string]string{"X-Client-IP": ip})
	png := base64.StdEncoding.EncodeToString(testPNG(t, 40, 30))
	res, out := callTool(t, cs, "extract_product_from_media", map[string]any{
		"files": []any{map[string]any{"type": "image", "data": png, "mimeType": "image/png"}}, "family": "motorised_tools_small_appliances",
	})
	if res.IsError || out["product"] == nil {
		t.Fatalf("extract_product_from_media: %s", text(res))
	}
	if strings.Contains(mustJSON(out), secret) || strings.Contains(text(res), secret) {
		t.Error("file bytes leaked into the result")
	}
	if len(fake.Calls()) != 1 {
		t.Errorf("model calls = %d", len(fake.Calls()))
	}
	// The MCP call used the address's light token: the REST upload is refused.
	body, ct := mediaBody(t, []upload{{"a.png", "image/png", testPNG(t, 40, 30)}}, nil)
	req := httptest.NewRequest("POST", "/v1/products/extract-media", bytes.NewReader(body))
	req.Header.Set("Content-Type", ct)
	req.Header.Set("X-Client-IP", ip)
	rec := httptest.NewRecorder()
	hs.h.ServeHTTP(rec, req)
	if rec.Code != http.StatusTooManyRequests {
		t.Errorf("REST extract-media after the MCP call = %d, want 429", rec.Code)
	}
	// A file the route refuses by its bytes (declared PNG, really text) is a
	// tool error from the route, with no model call.
	other := mcpClient(t, hs, map[string]string{"X-Client-IP": "198.51.100.62"})
	res, _ = callTool(t, other, "extract_product_from_media", map[string]any{
		"files": []any{map[string]any{"data": base64.StdEncoding.EncodeToString([]byte("just some text, not an image")), "mime_type": "image/png"}},
	})
	if !res.IsError || !strings.Contains(text(res), "415") {
		t.Errorf("non-image bytes: %s", text(res))
	}
	if len(fake.Calls()) != 1 {
		t.Errorf("a refused file reached the model")
	}
}

// The broker review tools (D200) run the REST routes in-process with the
// caller's case access key: the link is made on the caller's case only, the
// status never carries a token, and another key sees nothing.
func TestMCPBrokerReviewTools(t *testing.T) {
	hs := newHarnessWith(t, "substantial_transformation", func(d *Deps) { d.CaseOwnership = true })
	cs := mcpClient(t, hs, nil)
	_, out := callTool(t, cs, "predict_origin", heatGunArgs(t))
	id, key := out["id"].(string), out["case_access_key"].(string)
	res, made := callTool(t, cs, "create_broker_review_link", map[string]any{"case_id": id, "case_access_key": key, "expires_in_days": 7})
	if res.IsError || !strings.HasPrefix(made["review_path"].(string), "/review#ubr_") || !strings.Contains(text(res), "Not a ruling") {
		t.Fatalf("create link: %s", text(res))
	}
	tok := made["token"].(string)
	if code, _ := hs.doAs(t, "POST", "/v1/review/decision", `{"decision":"changes","comment":"Add the plant address."}`, map[string]string{reviewTokenHeader: tok}); code != 200 {
		t.Fatalf("broker answer = %d", code)
	}
	res, st := callTool(t, cs, "get_broker_review_status", map[string]any{"case_id": id, "case_access_key": key})
	if res.IsError || !strings.Contains(text(res), "changes_requested") || !strings.Contains(text(res), "Add the plant address.") || strings.Contains(mustJSON(st), tok) {
		t.Errorf("status: %s", text(res))
	}
	other := "mcp_" + strings.Repeat("0", 48)
	for _, name := range []string{"create_broker_review_link", "get_broker_review_status"} {
		if res, _ := callTool(t, cs, name, map[string]any{"case_id": id, "case_access_key": other}); !res.IsError {
			t.Errorf("%s with another access key succeeded", name)
		}
	}
}

// A signed-in identity sent to POST /mcp must not reach the in-process tool
// calls through the request context (issue #385). The guard drops the header,
// but withUser ran earlier in the outer chain; skipping it on /mcp is what
// makes "an MCP caller is an API key or anonymous" true.
func TestMCPUserTokenDoesNotReachToolContext(t *testing.T) {
	hs, _, _ := teamsHarness(t, nil)
	alice := user("alice")
	id := hs.newCase(t, alice)
	if got := hs.listIDs(t, alice); !got[id] {
		t.Fatalf("REST as Alice cannot see her case: %v", got)
	}

	cs := mcpClient(t, hs, alice)
	if res, _ := callTool(t, cs, "get_case", map[string]any{"case_id": id}); !res.IsError || !strings.Contains(text(res), "404") {
		t.Errorf("MCP get_case with a user token = %s, want 404", text(res))
	}
	_, list := callTool(t, cs, "list_cases", nil)
	if strings.Contains(mustJSON(list), id) {
		t.Error("MCP list_cases with a user token listed Alice's case")
	}
}

// With both credentials, the API key owns the case and is billed for the
// call; the user token is ignored on /mcp (issue #385).
func TestMCPUserTokenWithKeyStillUsesTheKey(t *testing.T) {
	hs, _, _ := teamsHarness(t, func(d *Deps) { withKeys(d) })
	key, _ := issueKey(t, hs, "Acme", 10)

	cs := mcpClient(t, hs, with(user("alice"), map[string]string{"X-API-Key": key}))
	res, out := callTool(t, cs, "predict_origin", heatGunArgs(t))
	if res.IsError {
		t.Fatalf("predict_origin: %s", text(res))
	}
	id, _ := out["id"].(string)
	if id == "" {
		t.Fatalf("no case id in %v", out)
	}
	if code, got := hs.doAs(t, "GET", "/v1/cases/"+id, "", map[string]string{"X-API-Key": key}); code != http.StatusOK {
		t.Errorf("the API key cannot read the case its MCP call made: %d %v", code, got)
	}
	if code, _ := hs.doAs(t, "GET", "/v1/cases/"+id, "", user("alice")); code != http.StatusNotFound {
		t.Errorf("the ignored user token owns the MCP case: %d", code)
	}
}
