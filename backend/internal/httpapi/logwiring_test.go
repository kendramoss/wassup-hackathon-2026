package httpapi

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/llm"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/logsafe"
)

// #285 wiring, through the REAL handler (mux, middleware, auth, audit) with
// fakes only. Sentinels are obviously synthetic. A logger is injected as a
// plain JSON handler: the call sites must be safe on their own, and the
// production allowlist handler (logwiring_policy_test.go) is a second layer.
const (
	sentEmail    = "zz.user285+leak@example.invalid"
	sentToken    = "tok_285_SYNTHETIC_DO_NOT_USE_0123456789"
	sentSupplier = "Zzyzx-Synthetic-Supplier-285 GmbH"
	sentBOM      = "PN-285-SYNTH-0001 stator laminations 4200 pcs @ USD 1.234"
	sentQuestion = "Is Zzyzx-Synthetic-Supplier-285 GmbH our only source for PN-285-SYNTH-0001?"
	sentKeyName  = "Zzyzx key name 285"
)

var allSentinels = []string{sentEmail, sentToken, sentSupplier, sentBOM, sentQuestion, sentKeyName, "Zzyzx", "PN-285-SYNTH", "zz.user285"}

func assertNoSentinel(t *testing.T, where, out string) {
	t.Helper()
	for _, s := range allSentinels {
		if strings.Contains(out, s) {
			t.Errorf("%s leaked sentinel %q:\n%s", where, s, out)
		}
	}
}

// useLogger makes every harness built afterwards log to the returned buffer.
func useLogger(t *testing.T, h func(io.Writer) slog.Handler) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	harnessLogger = slog.New(h(&buf))
	t.Cleanup(func() { harnessLogger = nil })
	return &buf
}

func plainJSON(w io.Writer) slog.Handler {
	return slog.NewJSONHandler(w, &slog.HandlerOptions{Level: slog.LevelDebug})
}

// policyJSON is the production handler: the logsafe allowlist over JSON with
// the real LogPolicy (routes, tools, check names, the models the tests use).
func policyJSON(w io.Writer) slog.Handler {
	h, err := logsafe.NewHandler(slog.NewJSONHandler(w, &slog.HandlerOptions{Level: slog.LevelDebug}),
		LogPolicy("gemini-4-argon", "gemini-3.8-flash"))
	if err != nil {
		panic(err)
	}
	return h
}

// eachLogger runs f under a plain JSON logger (the call sites must be safe on
// their own) and under the production allowlist handler (which must keep the
// operational fields while masking everything else).
func eachLogger(t *testing.T, f func(t *testing.T, mk func(io.Writer) slog.Handler)) {
	t.Helper()
	for _, c := range []struct {
		name string
		mk   func(io.Writer) slog.Handler
	}{{"plain", plainJSON}, {"allowlist", policyJSON}} {
		t.Run(c.name, func(t *testing.T) { f(t, c.mk) })
	}
}

func logLines(t *testing.T, buf *bytes.Buffer) []map[string]any {
	t.Helper()
	var out []map[string]any
	for _, l := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		if l == "" {
			continue
		}
		m := map[string]any{}
		if err := json.Unmarshal([]byte(l), &m); err != nil {
			t.Fatalf("log line is not JSON: %q", l)
		}
		out = append(out, m)
	}
	return out
}

func wantRef(header string) string {
	sum := sha256.Sum256([]byte(header))
	return "h_" + hex.EncodeToString(sum[:])[:16]
}

var hexRE = func(s string) bool {
	if len(s) != 16 {
		return false
	}
	_, err := hex.DecodeString(s)
	return err == nil
}

// Every client-supplied X-Request-ID is hashed once at the top, whatever its
// shape, and the SAME value is the response header, the log field and the
// audit event's request id.
func TestOneRequestRefAcrossHeaderLogAndAudit(t *testing.T) {
	eachLogger(t, func(t *testing.T, mk func(io.Writer) slog.Handler) {
		headers := map[string]string{
			"email":      sentEmail,
			"token":      sentToken,
			"raw 16 hex": "0123456789abcdef",
			"h_ plant":   "h_0123456789abcdef",
			"huge":       strings.Repeat("a1b2", 2000),
			"injection":  "x\"},{\"msg\":\"forged\",\"request_id\":\"ffffffffffffffff\"",
			"newline":    "line1\nline2",
			"absent":     "",
		}
		for name, hdr := range headers {
			t.Run(name, func(t *testing.T) {
				buf := useLogger(t, mk)
				e := newAuditEnvWith(t, true)
				before := len(e.events(e.tenant()))
				buf.Reset()
				req := httptest.NewRequest("PUT", e.rolePath(e.wsID, "vera")+"?token="+url.QueryEscape(sentToken), strings.NewReader(`{"role":"editor"}`))
				for k, v := range user("carol") {
					req.Header.Set(k, v)
				}
				if hdr != "" {
					req.Header.Set("X-Request-ID", hdr)
				}
				rec := httptest.NewRecorder()
				e.hs.h.ServeHTTP(rec, req)
				if rec.Code != http.StatusOK {
					t.Fatalf("role change = %d %s", rec.Code, rec.Body.String())
				}
				got := rec.Header().Get("X-Request-ID")
				if hdr != "" && got != wantRef(hdr) {
					t.Fatalf("response id = %q, want %q", got, wantRef(hdr))
				}
				if hdr == "" && !hexRE(got) {
					t.Fatalf("minted id = %q, want 16 hex", got)
				}
				if hdr != "" && (got == hdr || strings.Contains(buf.String(), hdr)) {
					t.Fatalf("the raw client id reached the response or the log")
				}
				var reqLines int
				for _, l := range logLines(t, buf) {
					if l["msg"] == "request" {
						reqLines++
						if l["request_id"] != got {
							t.Errorf("log request_id = %v, want %q", l["request_id"], got)
						}
						if l["route"] != "PUT /v1/workspaces/{id}/members/{email}/role" {
							t.Errorf("logged route = %v", l["route"])
						}
						if _, has := l["path"]; has {
							t.Errorf("raw path is still logged: %v", l)
						}
					}
				}
				if reqLines != 1 {
					t.Fatalf("%d request log lines, want 1:\n%s", reqLines, buf.String())
				}
				assertNoSentinel(t, "request log", buf.String())
				if strings.Contains(buf.String(), "[masked]") || strings.Contains(buf.String(), "masked_attrs") {
					t.Errorf("an operational field was masked:\n%s", buf.String())
				}
				for _, s := range []string{"vera@example.com", "vera%40", "token="} {
					if strings.Contains(buf.String(), s) {
						t.Errorf("request log carries %q:\n%s", s, buf.String())
					}
				}
				evs := e.events(e.tenant())[before:]
				if len(evs) == 0 {
					t.Fatal("no audit events for the request")
				}
				for _, ev := range evs {
					if ev.RequestID != got {
						t.Errorf("audit event %s request id = %q, want the same %q", ev.Action, ev.RequestID, got)
					}
				}
			})
		}
	})
}

// The matched pattern is logged, never the raw path or query (case ids,
// tokens, supplier names). A request that matches no route logs the catch-all.
func TestRequestLogUsesMatchedRoutePattern(t *testing.T) {
	eachLogger(t, func(t *testing.T, mk func(io.Writer) slog.Handler) {
		buf := useLogger(t, mk)
		hs := newHarness(t, "substantial_transformation")
		for _, tc := range []struct{ method, path, route string }{
			{"GET", "/v1/cases/" + url.PathEscape(sentSupplier) + "?q=" + url.QueryEscape(sentToken), "GET /v1/cases/{id}"},
			{"GET", "/v1/rulings/" + url.PathEscape(sentBOM), "GET /v1/rulings/{id}"},
			{"GET", "/v1/nope/" + url.PathEscape(sentEmail), "/"},
			{"GET", "/health", "GET /health"},
		} {
			buf.Reset()
			hs.do(t, tc.method, tc.path, "")
			assertNoSentinel(t, tc.path, buf.String())
			var saw bool
			for _, l := range logLines(t, buf) {
				if l["msg"] == "request" {
					saw = true
					if l["route"] != tc.route {
						t.Errorf("%s: route = %v, want %q", tc.path, l["route"], tc.route)
					}
					if l["status"] == nil || l["latency_ms"] == nil || l["method"] != tc.method {
						t.Errorf("%s: operational fields lost: %v", tc.path, l)
					}
				}
			}
			if !saw {
				t.Errorf("%s: no request line", tc.path)
			}
		}
	})
}

// Error text from the model, the store or a provider body never reaches the
// log: only a class (and a status number).
func TestErrorPathsLogClassNotText(t *testing.T) {
	eachLogger(t, func(t *testing.T, mk func(io.Writer) slog.Handler) {
		cases := map[string]error{
			"internal":     errors.New("firestore: " + sentBOM + " " + sentEmail),
			"bad response": fmt.Errorf("vertex: %w: %s", llm.ErrBadResponse, sentSupplier),
			"unavailable":  fmt.Errorf("vertex: %w: %s", llm.ErrUnavailable, sentToken),
			"api error":    &llm.APIError{Provider: "vertex", StatusCode: 503, Body: sentBOM + sentEmail},
		}
		for name, cause := range cases {
			t.Run(name, func(t *testing.T) {
				buf := useLogger(t, mk)
				eng := &fakeEngine{conf: 0.97, err: cause}
				hs := newHarnessWith(t, "substantial_transformation", func(d *Deps) { d.Engine = eng })
				buf.Reset()
				code, _ := hs.do(t, "POST", "/v1/cases/predict", heatGunBody(t))
				if code < 500 {
					t.Fatalf("status %d, want a 5xx", code)
				}
				assertNoSentinel(t, name, buf.String())
				var sawClass bool
				for _, l := range logLines(t, buf) {
					if g, ok := l["err"].(map[string]any); ok && g["class"] != nil {
						sawClass = true
					}
				}
				if !sawClass {
					t.Errorf("no err class in log:\n%s", buf.String())
				}
			})
		}
	})
}

// The deep health response and log carry a fixed class message for a failing
// store or provider, not the error text (a project path, a provider body).
func TestHealthDeepFailureIsAFixedClass(t *testing.T) {
	eachLogger(t, func(t *testing.T, mk func(io.Writer) slog.Handler) {
		for name, cause := range map[string]error{
			"store":    errors.New("rpc error: projects/zzyzx-285/databases/" + sentBOM),
			"provider": &llm.APIError{Provider: "vertex", StatusCode: 400, Body: sentBOM + sentEmail},
		} {
			t.Run(name, func(t *testing.T) {
				buf := useLogger(t, mk)
				hs := newHarnessWith(t, "substantial_transformation", func(d *Deps) {
					d.Checks = []HealthCheck{{Name: "case_store", Run: func(context.Context) error { return cause }}}
				})
				buf.Reset()
				req := httptest.NewRequest("GET", "/health?deep=1", nil)
				rec := httptest.NewRecorder()
				hs.h.ServeHTTP(rec, req)
				if rec.Code != http.StatusServiceUnavailable {
					t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
				}
				assertNoSentinel(t, "response", rec.Body.String())
				assertNoSentinel(t, "log", buf.String())
				var out struct {
					Status string `json:"status"`
					Checks map[string]struct {
						OK    bool   `json:"ok"`
						Error string `json:"error"`
					} `json:"checks"`
				}
				if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil || out.Status != "degraded" || out.Checks["case_store"].OK || out.Checks["case_store"].Error == "" {
					t.Errorf("response shape changed: %s", rec.Body.String())
				}
			})
		}
	})
}

// New history events carry no free text; the actor email stays for the
// workspace's own history drawer; old events are not rewritten.
func TestHistorySummariesCarryNoFreeText(t *testing.T) {
	hs := newHarness(t, "substantial_transformation")
	_, c := hs.do(t, "POST", "/v1/cases/predict", heatGunBody(t))
	id := c["id"].(string)
	if code, out := hs.do(t, "POST", "/v1/cases/"+id+"/ask", `{"question":`+jsonStr285(sentQuestion)+`}`); code != 200 {
		t.Fatalf("ask = %d %v", code, out)
	}
	for _, n := range []string{sentSupplier, "Second name"} {
		if code, out := hs.do(t, "PATCH", "/v1/cases/"+id, `{"display_name":`+jsonStr285(n)+`}`); code != 200 {
			t.Fatalf("rename = %d %v", code, out)
		}
	}
	_, got := hs.do(t, "GET", "/v1/cases/"+id, "")
	var summaries []string
	for _, e := range got["history"].([]any) {
		m := e.(map[string]any)
		if k := m["kind"]; k == "asked" || k == "renamed" {
			summaries = append(summaries, m["summary"].(string))
			if strings.Contains(m["summary"].(string), "Zzyzx") || strings.Contains(m["summary"].(string), "Second name") || strings.Contains(m["summary"].(string), "PN-285") {
				t.Errorf("%v summary carries free text: %q", k, m["summary"])
			}
		}
	}
	if len(summaries) != 3 {
		t.Fatalf("summaries = %v", summaries)
	}
	if !strings.HasPrefix(summaries[0], fmt.Sprintf("question (%d characters)", len([]rune(sentQuestion)))) {
		t.Errorf("asked summary = %q", summaries[0])
	}
	if summaries[1] != "renamed" || summaries[2] != "renamed" {
		t.Errorf("renamed summaries = %q", summaries[1:])
	}
	// The question itself is still in the case chat, accurate (Regime B).
	chat, _ := json.Marshal(got["chat"])
	if !strings.Contains(string(chat), sentQuestion) {
		t.Errorf("the chat lost the question: %s", chat)
	}
}

func jsonStr285(s string) string { b, _ := json.Marshal(s); return string(b) }

// An API-key name is a free label and a path id is whatever the caller sent:
// neither may reach the log.
func TestAPIKeyLogsNeverCarryTheNameOrPathID(t *testing.T) {
	eachLogger(t, func(t *testing.T, mk func(io.Writer) slog.Handler) {
		buf := useLogger(t, mk)
		hs := newHarnessWith(t, "substantial_transformation", withKeys)
		buf.Reset()
		_, id := issueKey(t, hs, sentKeyName, 5)
		if code, _ := hs.doAs(t, "DELETE", "/v1/admin/api-keys/"+url.PathEscape(sentToken), "", admin); code != http.StatusNotFound {
			t.Fatalf("disable unknown = %d", code)
		}
		if code, _ := hs.doAs(t, "DELETE", "/v1/admin/api-keys/"+id, "", admin); code != http.StatusNoContent {
			t.Fatalf("disable = %d", code)
		}
		assertNoSentinel(t, "api key logs", buf.String())
		if n := strings.Count(buf.String(), `"key_id":"`+id+`"`); n != 2 {
			t.Errorf("the server-made key id is logged %d times, want 2 (issued, disabled):\n%s", n, buf.String())
		}
		if strings.Contains(buf.String(), `"name"`) {
			t.Errorf("a name field is logged:\n%s", buf.String())
		}
	})
}

// The tool names the log policy accepts are the tools the code declares.
func TestLogToolNamesMatchDeclaredTools(t *testing.T) {
	declared := map[string]bool{}
	for _, dir := range []string{"../caseagent", "../verifier"} {
		files, _ := filepath.Glob(filepath.Join(dir, "*.go"))
		for _, f := range files {
			if strings.HasSuffix(f, "_test.go") {
				continue
			}
			af, err := parser.ParseFile(token.NewFileSet(), f, nil, 0)
			if err != nil {
				t.Fatal(err)
			}
			ast.Inspect(af, func(n ast.Node) bool {
				cl, ok := n.(*ast.CompositeLit)
				if !ok {
					return true
				}
				switch typeName(cl.Type) {
				case "functiontool.Config", "llm.Tool":
					for _, el := range cl.Elts {
						if kv, ok := el.(*ast.KeyValueExpr); ok && fmt.Sprint(kv.Key) == "Name" {
							if lit, ok := kv.Value.(*ast.BasicLit); ok {
								declared[strings.Trim(lit.Value, `"`)] = true
							}
						}
					}
				}
				return true
			})
		}
	}
	for n := range declared {
		found := false
		for _, l := range LogToolNames {
			found = found || l == n
		}
		if !found {
			t.Errorf("tool %q is declared in the code but missing from LogToolNames", n)
		}
	}
	for _, l := range LogToolNames {
		if !declared[l] {
			t.Errorf("LogToolNames has %q, which no declared tool uses", l)
		}
	}
}

func typeName(e ast.Expr) string {
	if se, ok := e.(*ast.SelectorExpr); ok {
		return fmt.Sprint(se.X) + "." + se.Sel.Name
	}
	return ""
}

// Every MintedRef is audited: logsafe.MintedRef is called in exactly one place
// (logattrs.go), and every caseAttr / keyAttr argument is one of the values
// below, each of which comes from the store, the key store or the generator
// (the reason is the comment at the call site). A new call, or a new argument,
// fails this test until someone has proved the id is server-made.
func TestMintedRefCallSitesAreAudited(t *testing.T) {
	want := map[string]string{
		"apikeys.go:keyAttr(key.ID)":  "2", // key returned by APIKeys.Authenticate
		"apikeys.go:keyAttr(k.ID)":    "1", // key returned by APIKeys.Issue
		"apikeys.go:keyAttr(id)":      "1", // path id confirmed by APIKeys.Exists and acted on by Disable
		"scenarios.go:caseAttr(c.ID)": "1", // case listed by the case store
	}
	got := map[string]int{}
	mint := 0
	files, _ := filepath.Glob("*.go")
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		src, _ := os.ReadFile(f)
		fset := token.NewFileSet()
		af, err := parser.ParseFile(fset, f, src, 0)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(af, func(n ast.Node) bool {
			c, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			switch fn := c.Fun.(type) {
			case *ast.SelectorExpr:
				if fn.Sel.Name == "MintedRef" {
					mint++
					if f != "logattrs.go" {
						t.Errorf("%s: logsafe.MintedRef outside logattrs.go", f)
					}
				}
			case *ast.Ident:
				if (fn.Name == "caseAttr" || fn.Name == "keyAttr") && f != "logattrs.go" {
					got[f+":"+fn.Name+"("+string(src[fset.Position(c.Args[0].Pos()).Offset:fset.Position(c.Args[0].End()).Offset])+")"]++
				}
			}
			return true
		})
	}
	if mint != 2 {
		t.Errorf("logsafe.MintedRef is called %d times, want 2 (caseAttr and keyAttr)", mint)
	}
	for k, n := range got {
		if want[k] != fmt.Sprint(n) {
			t.Errorf("unaudited or changed MintedRef use %s x%d", k, n)
		}
	}
	for k := range want {
		if _, ok := got[k]; !ok {
			t.Errorf("audited use %s no longer exists", k)
		}
	}
}

// Regime B stays accurate (#285): an authorized requester gets their own data
// back unchanged, the model gets the supplier and BOM text, and the export
// carries it. Only the log is minimised. The same request under the
// production handler logs none of it.
func TestAuthorizedDataStaysAccurateWhileLogsStayClean(t *testing.T) {
	buf := useLogger(t, policyJSON)
	hs := newHarness(t, "substantial_transformation")
	var in map[string]any
	if err := json.Unmarshal([]byte(heatGunBody(t)), &in); err != nil {
		t.Fatal(err)
	}
	prod := in["product"].(map[string]any)
	prod["name"] = sentSupplier + " heat gun"
	prod["plants"].([]any)[0].(map[string]any)["name"] = sentSupplier
	prod["components"].([]any)[0].(map[string]any)["name"] = sentBOM
	body, _ := json.Marshal(in)
	buf.Reset()
	code, out := hs.do(t, "POST", "/v1/cases/predict", string(body))
	if code != http.StatusCreated {
		t.Fatalf("predict = %d %v", code, out)
	}
	id := out["id"].(string)
	for _, s := range []string{sentSupplier, sentBOM} {
		if !strings.Contains(hs.engine.last.Facts+fmt.Sprintf("%+v", hs.engine.last.Product), s) {
			t.Errorf("the model input lost %q", s)
		}
	}
	_, got := hs.do(t, "GET", "/v1/cases/"+id, "")
	stored, _ := json.Marshal(got)
	for _, s := range []string{sentSupplier, sentBOM} {
		if !strings.Contains(string(stored), s) {
			t.Errorf("the authorized case response lost %q", s)
		}
	}
	if !strings.Contains(string(stored), `"origin_country":"China"`) && !strings.Contains(string(stored), `"origin":"China"`) {
		t.Errorf("origin changed: %s", stored)
	}
	code, ex := hs.do(t, "POST", "/v1/cases/"+id+"/export", `{"plan_index":0}`)
	if code != http.StatusOK && code != http.StatusBadRequest {
		t.Fatalf("export = %d %v", code, ex)
	}
	assertNoSentinel(t, "production log for a request that carries the supplier and BOM", buf.String())
	if strings.Contains(buf.String(), "[masked]") || strings.Contains(buf.String(), "masked_attrs") {
		t.Errorf("an operational field was masked:\n%s", buf.String())
	}
}

// T21: one tenant's case text never shows up in a log line or error produced by
// another tenant's failing request.
func TestOtherTenantFailureDoesNotLogThisTenantsCase(t *testing.T) {
	buf := useLogger(t, policyJSON)
	e := newAuditEnvWith(t, true)
	var in map[string]any
	_ = json.Unmarshal([]byte(heatGunBody(t)), &in)
	in["product"].(map[string]any)["name"] = sentSupplier
	b, _ := json.Marshal(in)
	code, out := e.hs.doAs(t, "POST", "/v1/cases/predict", string(b), user("dave"))
	if code != http.StatusCreated {
		t.Fatalf("dave predict = %d %v", code, out)
	}
	id := out["id"].(string)
	buf.Reset()
	// zoe is in another workspace: she cannot read it, and a failing call of
	// hers must not carry Acme's text into any log line.
	e.hs.engine.err = fmt.Errorf("engine failed on %s", sentSupplier)
	for _, c := range []struct{ method, path, body string }{
		{"GET", "/v1/cases/" + id, ""},
		{"POST", "/v1/cases/" + id + "/plan", `{"target":"Thailand"}`},
		{"POST", "/v1/cases/predict", string(b)},
	} {
		e.hs.doAs(t, c.method, c.path, c.body, user("zoe"))
	}
	assertNoSentinel(t, "zoe's failing requests", buf.String())
}
