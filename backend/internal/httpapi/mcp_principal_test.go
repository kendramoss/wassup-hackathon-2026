package httpapi

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/apikey"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// MCP principal (#385, D280). An MCP caller is an API key or anonymous: a
// tool call's /v1 request takes its principal only from the API key (and, for
// an anonymous caller, the case access key) that the MCP guard forwards, never
// from the outer POST /mcp request. The admin token is refused with 403; a
// sign-in token is ignored (#481); the visitor ID, cookies and a broker review
// token are dropped.

// postMCP sends one raw tools/list request to POST /mcp.
func postMCP(t *testing.T, hs harness, headers map[string]string) (int, string) {
	t.Helper()
	req := httptest.NewRequest("POST", "/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	hs.h.ServeHTTP(rec, req)
	return rec.Code, rec.Body.String()
}

// mcpClientOn is mcpClient over h instead of the harness handler.
func mcpClientOn(t *testing.T, h http.Handler, headers map[string]string) *mcp.ClientSession {
	t.Helper()
	return mcpClient(t, harness{h: h}, headers)
}

// withCtx serves h with values added to every request's context, as if the
// API's own middleware had put them there before the MCP handler ran.
func withCtx(h http.Handler, add func(context.Context) context.Context) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.ServeHTTP(w, r.WithContext(add(r.Context())))
	})
}

func asUser(email string) func(context.Context) context.Context {
	return func(ctx context.Context) context.Context {
		return context.WithValue(ctx, userKey{}, identity{email: email, owner: userOwner(email)})
	}
}

// The issue's two examples, with the signed-in identity (and an API key)
// placed in the outer request's context directly: neither the guard nor
// withUser's /mcp skip is what stops them, the tool call's own context is.
// A later row sends the real header too.
func TestMCPToolCallsDoNotInheritTheOuterPrincipal(t *testing.T) {
	keys := apikey.New(apikey.NewMemory())
	hs, _, _ := teamsHarness(t, func(d *Deps) { d.APIKeys = keys })
	key, _ := issueKey(t, hs, "Acme", 1000)
	alice := hs.newCase(t, user("alice"))
	keyCase := hs.newCase(t, map[string]string{"X-API-Key": key})
	authed, err := keys.Authenticate(context.Background(), key)
	if err != nil {
		t.Fatal(err)
	}
	notFound := func(t *testing.T, cs *mcp.ClientSession, id string) {
		t.Helper()
		if res, _ := callTool(t, cs, "get_case", map[string]any{"case_id": id}); !res.IsError || !strings.Contains(text(res), "404") {
			t.Errorf("get_case %s = %s, want not_found", id, text(res))
		}
		_, out := callTool(t, cs, "list_cases", nil)
		if strings.Contains(mustJSON(out), id) {
			t.Errorf("list_cases lists %s", id)
		}
	}

	t.Run("anonymous control", func(t *testing.T) {
		notFound(t, mcpClient(t, hs, map[string]string{"X-Client-IP": "198.51.100.71"}), alice)
	})
	t.Run("signed-in user in the context, no key", func(t *testing.T) {
		cs := mcpClientOn(t, withCtx(hs.h, asUser("alice@example.com")), map[string]string{"X-Client-IP": "198.51.100.72"})
		notFound(t, cs, alice)
	})
	t.Run("API key in the context, not sent", func(t *testing.T) {
		cs := mcpClientOn(t, withCtx(hs.h, func(ctx context.Context) context.Context {
			return context.WithValue(ctx, apiKeyCtx{}, authed)
		}), map[string]string{"X-Client-IP": "198.51.100.73"})
		notFound(t, cs, keyCase)
	})
	t.Run("API key sent, another user in the context", func(t *testing.T) {
		cs := mcpClientOn(t, withCtx(hs.h, asUser("bob@example.com")), map[string]string{"X-API-Key": key})
		res, out := callTool(t, cs, "predict_origin", heatGunArgs(t))
		if res.IsError {
			t.Fatal(text(res))
		}
		id := out["id"].(string)
		if out["case_access_key"] != nil {
			t.Error("a key caller got an access key")
		}
		if code, _ := hs.doAs(t, "GET", "/v1/cases/"+id, "", map[string]string{"X-API-Key": key}); code != http.StatusOK {
			t.Errorf("GET with the key = %d, want 200: the key owns the case", code)
		}
		if code, _ := hs.doAs(t, "GET", "/v1/cases/"+id, "", user("bob")); code != http.StatusNotFound {
			t.Errorf("GET as bob = %d, want 404", code)
		}
		if res, _ := callTool(t, cs, "get_case", map[string]any{"case_id": alice}); !res.IsError {
			t.Error("the key reached alice's case")
		}
		if res, _ := callTool(t, cs, "get_case", map[string]any{"case_id": keyCase}); res.IsError {
			t.Errorf("the key cannot read its own case: %s", text(res))
		}
	})
	t.Run("sign-in token header is ignored", func(t *testing.T) {
		cs := mcpClient(t, hs, with(user("alice"), map[string]string{"X-Client-IP": "198.51.100.75"}))
		notFound(t, cs, alice)
		ck := mcpClient(t, hs, with(user("bob"), map[string]string{"X-API-Key": key}))
		res, out := callTool(t, ck, "predict_origin", heatGunArgs(t))
		if res.IsError {
			t.Fatal(text(res))
		}
		id := out["id"].(string)
		if code, _ := hs.doAs(t, "GET", "/v1/cases/"+id, "", map[string]string{"X-API-Key": key}); code != http.StatusOK {
			t.Errorf("GET with the key = %d, want 200", code)
		}
		if code, _ := hs.doAs(t, "GET", "/v1/cases/"+id, "", user("bob")); code != http.StatusNotFound {
			t.Errorf("GET as bob = %d, want 404", code)
		}
	})
	t.Run("admin token with a key is refused", func(t *testing.T) {
		if code, _ := postMCP(t, hs, with(admin, map[string]string{"X-API-Key": key})); code != http.StatusForbidden {
			t.Errorf("admin token with a key = %d, want 403", code)
		}
	})
}

// A broker review link token is never a credential on /mcp: it is dropped,
// and the tool calls stay anonymous.
func TestMCPIgnoresABrokerReviewToken(t *testing.T) {
	hs, _, _ := teamsHarness(t, func(d *Deps) { d.CaseOwnership = true })
	alice := hs.newCase(t, user("alice"))
	code, made := hs.doAs(t, "POST", "/v1/cases/"+alice+"/reviews", `{"expires_in_days":7}`, user("alice"))
	if code != http.StatusCreated && code != http.StatusOK {
		t.Fatalf("create link = %d %v", code, made)
	}
	tok := made["token"].(string)
	if code, _ := hs.doAs(t, "GET", "/v1/review", "", map[string]string{reviewTokenHeader: tok}); code != http.StatusOK {
		t.Fatalf("the token itself works over REST = %d", code)
	}
	cs := mcpClient(t, hs, map[string]string{reviewTokenHeader: tok, "X-Client-IP": "198.51.100.74"})
	for _, name := range []string{"get_case", "get_broker_review_status", "create_broker_review_link", "get_evidence_report"} {
		if res, _ := callTool(t, cs, name, map[string]any{"case_id": alice}); !res.IsError {
			t.Errorf("%s with a review token on /mcp reached the case: %s", name, text(res))
		}
	}
}

// Isolation matrix over MCP: every principal of the REST matrix, sent on
// POST /mcp. The admin token is refused; a user token is ignored and a
// visitor ID dropped (the caller is anonymous); an API key is the key. get_case and
// list_cases then see exactly what that MCP principal may see.
func TestIsolationMatrixOverMCP(t *testing.T) {
	for _, ownership := range []bool{true, false} {
		t.Run(fmt.Sprintf("CASE_OWNERSHIP=%v", ownership), func(t *testing.T) {
			e := newIsoEnv(t, ownership)
			asMCP := map[string]string{
				"stranger": "stranger", "badid": "stranger", "alice": "stranger", "bob": "stranger",
				"acme": "acme", "beta": "beta",
				"carol": "stranger", "dave": "stranger", "erin": "stranger", // sign-in token ignored
				"admin": "", // refused
			}
			if len(asMCP) != len(e.headrs) {
				t.Fatalf("principals %d, MCP mapping %d", len(e.headrs), len(asMCP))
			}
			before := e.snapshot()
			n := 0
			for p, h := range e.headrs {
				eff, ok := asMCP[p]
				if !ok {
					t.Fatalf("no MCP mapping for %s", p)
				}
				if eff == "" {
					if code, body := postMCP(t, e.hs, h); code != http.StatusForbidden {
						t.Errorf("%s on /mcp = %d %s, want 403", p, code, body)
					}
					continue
				}
				n++
				cs := mcpClient(t, e.hs, with(h, map[string]string{"X-Client-IP": fmt.Sprintf("198.51.100.%d", 100+n)}))
				_, listed := callTool(t, cs, "list_cases", nil)
				lj := mustJSON(listed)
				for c, id := range e.ids {
					want := e.canRead(eff, c)
					res, _ := callTool(t, cs, "get_case", map[string]any{"case_id": id})
					if got := !res.IsError; got != want {
						t.Errorf("get_case as %s (MCP %s) on %s's case: visible %v, want %v (%s)", p, eff, c, got, want, text(res))
					}
					if want == false && strings.Contains(lj, id) {
						t.Errorf("list_cases as %s (MCP %s) lists %s's case", p, eff, c)
					}
				}
				if eff == "acme" && !strings.Contains(lj, e.ids["acme"]) {
					t.Error("list_cases with the acme key misses its own case")
				}
			}
			if after := e.snapshot(); after != before {
				t.Error("an MCP read changed a stored case")
			}
		})
	}
}
