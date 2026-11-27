package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"net/http"
	"net/http/httptest"
	"slices"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/apikey"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/store"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/teams"
)

// Case isolation matrix (#273). Every route registered in server.go is
// classified below; a route added without a row fails
// TestEveryRegisteredRouteIsClassified, so a new case-scoped endpoint cannot
// ship without isolation coverage. Case-scoped routes are then driven with
// every principal against every kind of case, with CASE_OWNERSHIP on and off.

type routeClass int

const (
	caseScoped  routeClass = iota // reads or changes an existing case named by {id}; must go through loadCase
	caseCreate                    // makes new cases owned by the caller, or lists the caller's view
	adminRoute                    // refuses a request without the admin token
	noCaseData                    // reads or changes no stored case (static data, stateless helpers, workspaces)
	reviewToken                   // reaches one case's review link with its token only (D200); see reviews_test.go
)

// routeTable is keyed by the mux pattern, exactly as registered in server.go.
var routeTable = map[string]routeClass{
	"POST /v1/cases/predict":                            caseCreate,
	"POST /v1/cases/batch":                              caseCreate,
	"GET /v1/cases":                                     caseCreate,
	"POST /v1/products/import":                          noCaseData,
	"POST /v1/products/erp-import":                      caseCreate, // files cases under the caller's scope; IDs derive from it
	"GET /v1/workspaces/{id}/usage":                     noCaseData,
	"PUT /v1/workspaces/{id}/budget":                    noCaseData,
	"DELETE /v1/workspaces/{id}/budget":                 noCaseData,
	"POST /v1/products/extract":                         noCaseData,
	"POST /v1/products/extract-media":                   noCaseData,
	"POST /v1/products/money-prefill":                   noCaseData,
	"POST /v1/products/hts-suggest":                     noCaseData,
	"POST /v1/products/parts-hts-suggest":               noCaseData, // its day cache is per caller and per description (D410): TestPartsHTSSuggestCacheIsPerCallerAndDescription
	"POST /v1/products/cost-estimate":                   noCaseData,
	"POST /v1/products/family-check":                    noCaseData,
	"POST /v1/cases/{id}/bom-version":                   caseScoped,
	"POST /v1/cases/{id}/scenarios":                     caseScoped,
	"POST /v1/cases/{id}/compare-targets":               caseScoped,
	"POST /v1/cases/{id}/ask":                           caseScoped,
	"PUT /v1/cases/{id}/economics":                      caseScoped,
	"PUT /v1/cases/{id}/pin":                            adminRoute,
	"POST /v1/cases/{id}/watch":                         caseScoped,
	"POST /v1/watch/run":                                adminRoute,
	"GET /v1/cases/{id}":                                caseScoped,
	"DELETE /v1/cases/{id}":                             caseScoped,
	"PATCH /v1/cases/{id}":                              caseScoped,
	"POST /v1/cases/{id}/plan":                          caseScoped,
	"POST /v1/cases/{id}/plan/estimate":                 caseScoped,
	"POST /v1/cases/{id}/export":                        caseScoped,
	"POST /v1/cases/{id}/answers":                       caseScoped,
	"PUT /v1/cases/{id}/origin-facts/{market}":          caseScoped,
	"PUT /v1/cases/{id}/family-facts":                   caseScoped,
	"GET /v1/cases/{id}/evidence-report":                caseScoped,
	"GET /v1/rulings/{id}":                              noCaseData,
	"GET /v1/rulings":                                   noCaseData,
	"POST /mcp":                                         noCaseData, // its tools call the /v1 routes, each checked on its own row; the MCP principal: TestIsolationMatrixOverMCP
	"GET /v1/eval/summary":                              noCaseData,
	"GET /v1/status":                                    noCaseData,
	"GET /v1/duty-rates":                                noCaseData,
	"POST /v1/rcep/check":                               noCaseData,
	"POST /v1/atiga/check":                              noCaseData,
	"POST /v1/usmca/check":                              noCaseData,
	"POST /v1/rules/canada-marking":                     noCaseData, // computes from the request alone; nothing stored (D720)
	"POST /v1/rules/eu-origin":                          noCaseData, // computes from the request alone; nothing stored (D760)
	"POST /v1/rules/uk-origin":                          noCaseData, // computes from the request alone; nothing stored (D760)
	"GET /v1/demos":                                     noCaseData,
	"GET /v1/jurisdictions":                             noCaseData,
	"GET /v1/demos/{id}":                                noCaseData,
	"POST /v1/workspaces":                               noCaseData,
	"GET /v1/workspaces/mine":                           noCaseData,
	"POST /v1/workspaces/{id}/members":                  noCaseData,
	"POST /v1/workspaces/{id}/join":                     noCaseData,
	"PUT /v1/workspaces/{id}/members/{email}/role":      noCaseData,
	"DELETE /v1/workspaces/{id}/members/{email}":        noCaseData,
	"POST /v1/sso/start":                                noCaseData,
	"POST /v1/sso/oidc/callback":                        noCaseData,
	"POST /v1/sso/saml/acs":                             noCaseData,
	"GET /v1/sso/saml/metadata":                         noCaseData,
	"GET /v1/workspaces/{id}/sso":                       noCaseData,
	"POST /v1/workspaces/{id}/sso":                      noCaseData,
	"PUT /v1/workspaces/{id}/sso/{conn}":                noCaseData,
	"POST /v1/workspaces/{id}/sso/{conn}/verify":        noCaseData,
	"DELETE /v1/workspaces/{id}/sso/{conn}":             noCaseData,
	"GET /v1/workspaces/{id}/admin":                     noCaseData,
	"GET /v1/workspaces/{id}/webhooks":                  noCaseData,
	"POST /v1/workspaces/{id}/webhooks":                 noCaseData,
	"PATCH /v1/workspaces/{id}/webhooks/{wid}":          noCaseData,
	"DELETE /v1/workspaces/{id}/webhooks/{wid}":         noCaseData,
	"POST /v1/workspaces/{id}/webhooks/{wid}/rotate":    noCaseData,
	"POST /v1/workspaces/{id}/webhooks/{wid}/test":      noCaseData,
	"GET /v1/workspaces/{id}/webhooks/{wid}/deliveries": noCaseData,
	"POST /v1/webhooks/sweep":                           adminRoute,
	"GET /v1/workspaces/{id}/scim":                      noCaseData,
	"POST /v1/workspaces/{id}/scim/token":               noCaseData,
	"DELETE /v1/workspaces/{id}/scim/token":             noCaseData,
	"GET /v1/scim/v2/ServiceProviderConfig":             noCaseData,
	"GET /v1/scim/v2/ResourceTypes":                     noCaseData,
	"GET /v1/scim/v2/Schemas":                           noCaseData,
	"GET /v1/scim/v2/Users":                             noCaseData,
	"POST /v1/scim/v2/Users":                            noCaseData,
	"GET /v1/scim/v2/Users/{uid}":                       noCaseData,
	"PUT /v1/scim/v2/Users/{uid}":                       noCaseData,
	"PATCH /v1/scim/v2/Users/{uid}":                     noCaseData,
	"DELETE /v1/scim/v2/Users/{uid}":                    noCaseData,
	"POST /v1/admin/api-keys":                           adminRoute,
	"GET /v1/admin/api-keys":                            adminRoute,
	"DELETE /v1/admin/api-keys/{id}":                    adminRoute,
	"POST /v1/api-keys":                                 noCaseData,
	"GET /v1/api-keys":                                  noCaseData,
	"DELETE /v1/api-keys/{id}":                          noCaseData,
	"GET /v1/audit":                                     noCaseData, // reads the audit trail, no case
	"GET /v1/audit/verify":                              noCaseData,
	"GET /v1/audit/export":                              noCaseData,
	"GET /v1/workspaces/{id}/retention":                 noCaseData,
	"PUT /v1/workspaces/{id}/retention":                 noCaseData,
	"GET /v1/workspaces/{id}/tombstones":                noCaseData, // case IDs and dates only, workspace admins
	"POST /v1/cases/{id}/reviews":                       caseScoped,
	"GET /v1/cases/{id}/reviews":                        caseScoped,
	"DELETE /v1/cases/{id}/reviews/{review}":            caseScoped,
	"GET /v1/review":                                    reviewToken,
	"POST /v1/review/decision":                          reviewToken,
	"PUT /v1/cases/{id}/queue":                          caseScoped,
	"POST /v1/cases/{id}/queue/transition":              caseScoped,
	"GET /v1/queue":                                     caseCreate, // lists the caller's view, like GET /v1/cases
	"GET /v1/cases/{id}/collab":                         caseScoped,
	"POST /v1/cases/{id}/comments":                      caseScoped,
	"POST /v1/cases/{id}/approvals":                     caseScoped,
	"PUT /v1/cases/{id}/collaborators":                  caseScoped,
	"GET /v1/review/comments":                           reviewToken,
	"POST /v1/review/comments":                          reviewToken,
	"POST /v1/cases/{id}/filing":                        caseScoped,
	"GET /v1/cases/{id}/filing":                         caseScoped,
	"POST /v1/cases/{id}/filing/status":                 caseScoped,
	"GET /health":                                       noCaseData,
}

// registeredRoutes parses server.go for every mux.HandleFunc pattern.
func registeredRoutes(t *testing.T) []string {
	t.Helper()
	f, err := parser.ParseFile(token.NewFileSet(), "server.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	ast.Inspect(f, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok || len(call.Args) < 2 {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != "HandleFunc" {
			return true
		}
		if lit, ok := call.Args[0].(*ast.BasicLit); ok && lit.Kind == token.STRING {
			if p, err := strconv.Unquote(lit.Value); err == nil && p != "/" {
				out = append(out, p)
			}
		}
		return true
	})
	sort.Strings(out)
	return slices.Compact(out) // /health is registered twice (with and without deps)
}

func TestEveryRegisteredRouteIsClassified(t *testing.T) {
	reg := registeredRoutes(t)
	if len(reg) < 35 {
		t.Fatalf("found only %d registered routes; the parse is broken", len(reg))
	}
	seen := map[string]bool{}
	for _, p := range reg {
		seen[p] = true
		if _, ok := routeTable[p]; !ok {
			t.Errorf("route %q is registered in server.go but not classified in routeTable: decide whether it is case-scoped and add it", p)
		}
		// A path with a case ID is a case route, whatever its row says.
		if strings.Contains(p, "/v1/cases/{id}") && routeTable[p] == noCaseData {
			t.Errorf("route %q names a case but is classified noCaseData", p)
		}
	}
	for p := range routeTable {
		if !seen[p] {
			t.Errorf("routeTable lists %q, which server.go does not register", p)
		}
	}
}

// isoRoute is one case-scoped request, for a given case ID.
type isoRoute struct {
	name, method, path, body string
	header                   map[string]string
	pattern                  string
}

// isoRoutes is the request set for every caseScoped pattern (plus the SSE
// variant of plan). The bodies are empty-ish on purpose: every handler loads
// the case before it reads the body, so a hidden case answers 404 whatever
// the body is.
func isoRoutes() []isoRoute {
	sse := map[string]string{"Accept": "text/event-stream"}
	return []isoRoute{
		{"get", "GET", "", "", nil, "GET /v1/cases/{id}"},
		{"rename", "PATCH", "", `{"display_name":"renamed"}`, nil, "PATCH /v1/cases/{id}"},
		{"delete", "DELETE", "", "", nil, "DELETE /v1/cases/{id}"},
		{"economics", "PUT", "/economics", `{"annual_import_value_usd":1}`, nil, "PUT /v1/cases/{id}/economics"},
		{"scenarios", "POST", "/scenarios", `{"scenarios":[{"label":"x","facts":"The unit is assembled in Mexico."}]}`, nil, "POST /v1/cases/{id}/scenarios"},
		{"compare", "POST", "/compare-targets", `{"targets":["Thailand","Vietnam"]}`, nil, "POST /v1/cases/{id}/compare-targets"},
		{"ask", "POST", "/ask", `{"question":"Why China?"}`, nil, "POST /v1/cases/{id}/ask"},
		{"watch", "POST", "/watch", `{}`, nil, "POST /v1/cases/{id}/watch"},
		{"plan", "POST", "/plan", `{"target":"Thailand"}`, nil, "POST /v1/cases/{id}/plan"},
		{"plan stream", "POST", "/plan", `{"target":"Thailand"}`, sse, "POST /v1/cases/{id}/plan"},
		{"plan estimate", "POST", "/plan/estimate", `{"target":"Thailand"}`, nil, "POST /v1/cases/{id}/plan/estimate"},
		{"export", "POST", "/export", `{"plan_index":0}`, nil, "POST /v1/cases/{id}/export"},
		{"answers", "POST", "/answers", `{"answers":[{"question_id":"q","answer":"yes"}]}`, nil, "POST /v1/cases/{id}/answers"},
		{"origin facts", "PUT", "/origin-facts/eu", `{"row":"none"}`, nil, "PUT /v1/cases/{id}/origin-facts/{market}"},
		{"family facts", "PUT", "/family-facts", `{}`, nil, "PUT /v1/cases/{id}/family-facts"},
		{"evidence report", "GET", "/evidence-report", "", nil, "GET /v1/cases/{id}/evidence-report"},
		{"bom version", "POST", "/bom-version", `{"csv":"x","apply":false}`, nil, "POST /v1/cases/{id}/bom-version"},
		{"pin", "PUT", "/pin", `{"pinned":true}`, nil, "PUT /v1/cases/{id}/pin"},
		{"review create", "POST", "/reviews", `{"expires_in_days":7}`, nil, "POST /v1/cases/{id}/reviews"},
		{"review list", "GET", "/reviews", "", nil, "GET /v1/cases/{id}/reviews"},
		{"review revoke", "DELETE", "/reviews/rv_0123456789abcdef", "", nil, "DELETE /v1/cases/{id}/reviews/{review}"},
		{"queue", "PUT", "/queue", `{"reason":"check the rule"}`, nil, "PUT /v1/cases/{id}/queue"},
		{"queue transition", "POST", "/queue/transition", `{"action":"start"}`, nil, "POST /v1/cases/{id}/queue/transition"},
		{"collab", "GET", "/collab", "", nil, "GET /v1/cases/{id}/collab"},
		{"comment", "POST", "/comments", `{"body":"looks right"}`, nil, "POST /v1/cases/{id}/comments"},
		{"approval", "POST", "/approvals", `{"decision":"approve"}`, nil, "POST /v1/cases/{id}/approvals"},
		{"collaborator", "PUT", "/collaborators", `{"email":"b@example.com","role":"broker"}`, nil, "PUT /v1/cases/{id}/collaborators"},
		{"filing prepare", "POST", "/filing", `{}`, nil, "POST /v1/cases/{id}/filing"},
		{"filing get", "GET", "/filing", "", nil, "GET /v1/cases/{id}/filing"},
		{"filing status", "POST", "/filing/status", `{"status":"withdrawn"}`, nil, "POST /v1/cases/{id}/filing/status"},
	}
}

func TestIsolationRequestSetCoversEveryCaseScopedRoute(t *testing.T) {
	have := map[string]bool{}
	for _, rt := range isoRoutes() {
		have[rt.pattern] = true
	}
	for p, c := range routeTable {
		if (c == caseScoped || (c == adminRoute && strings.Contains(p, "{id}/"))) && strings.Contains(p, "/v1/cases/{id}") && !have[p] {
			t.Errorf("no isolation request for case-scoped route %q", p)
		}
	}
	for p := range have {
		if _, ok := routeTable[p]; !ok {
			t.Errorf("isoRoutes has a request for unknown route %q", p)
		}
	}
}

type isoCall struct {
	code int
	body string
	ct   string
}

type isoEnv struct {
	t      *testing.T
	hs     harness
	cs     *store.Memory
	own    bool
	ids    map[string]string // named cases
	headrs map[string]map[string]string
}

const (
	isoAlice = "alice-visitor-id-0001"
	isoBob   = "bob-visitor-id-000002"
)

// newIsoEnv: teams, API keys and BOM versions on; ownership as given. The
// cases are made through the real create path by their owners. "noid" is made
// with no identity, "badid" with a malformed X-Visitor-ID.
func newIsoEnv(t *testing.T, ownership bool) *isoEnv { return newIsoEnvStrict(t, ownership, false) }

// newIsoEnvStrict is newIsoEnv with STRICT_AUTH as given. The fixtures are made
// through a handler with strict off (strict mode refuses to create cases for
// a bare visitor ID), then the strict handler is built over the same stores.
func newIsoEnvStrict(t *testing.T, ownership, strict bool) *isoEnv {
	t.Helper()
	e := &isoEnv{t: t, cs: store.NewMemory(), own: ownership, ids: map[string]string{}, headrs: map[string]map[string]string{}}
	wsStore, keys := teams.NewMemory(), apikey.New(apikey.NewMemory())
	build := func(strict bool) harness {
		hs, _, _ := teamsHarness(t, func(d *Deps) {
			d.Cases, d.CaseOwnership, d.BOMVersions, d.StrictAuth = e.cs, ownership, true, strict
			d.BrokerWorkflow = true
			d.Planner = &estimatingPlanner{fakePlanner: &fakePlanner{}} // also serves plan/estimate
			d.Workspaces, d.APIKeys = wsStore, keys
		})
		return hs
	}
	hs := build(false)
	e.hs = hs
	acme, _ := issueKey(t, hs, "Acme", 1000000)
	beta, _ := issueKey(t, hs, "Beta", 1000000)
	e.headrs = map[string]map[string]string{
		"stranger": nil,
		"badid":    {"X-Visitor-ID": "short"},
		"alice":    {"X-Visitor-ID": isoAlice},
		"bob":      {"X-Visitor-ID": isoBob},
		"acme":     {"X-API-Key": acme},
		"beta":     {"X-API-Key": beta},
		"carol":    user("carol"),
		"erin":     user("erin"), // a signed-in user with no workspace and no browser in common
		"admin":    admin,
	}
	_, ws := hs.doAs(t, "POST", "/v1/workspaces", `{"name":"Acme Co"}`, user("carol"))
	hs.addToWorkspace(t, ws["id"].(string), "carol", "dave")
	e.headrs["dave"] = user("dave")
	for name, h := range map[string]map[string]string{
		"alice": e.headrs["alice"], "bob": e.headrs["bob"], "noid": nil, "badid": e.headrs["badid"],
		"acme": e.headrs["acme"], "carol": e.headrs["carol"], "dave": e.headrs["dave"],
	} {
		e.ids[name] = hs.newCase(t, h)
	}
	e.ids["demo"] = hs.newCase(t, admin)
	if code, out := hs.doAs(t, "PUT", "/v1/cases/"+e.ids["demo"]+"/pin", `{"pinned":true}`, admin); code != http.StatusOK {
		t.Fatalf("pin demo = %d %v", code, out)
	}
	if strict {
		e.hs = build(true)
	}
	return e
}

func (e *isoEnv) call(rt isoRoute, id string, h map[string]string) isoCall {
	e.t.Helper()
	req := httptest.NewRequest(rt.method, "/v1/cases/"+id+rt.path, strings.NewReader(rt.body))
	for k, v := range with(rt.header, h) {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	e.hs.h.ServeHTTP(rec, req)
	return isoCall{rec.Code, rec.Body.String(), rec.Header().Get("Content-Type")}
}

// canRead is the intended visibility, written from the documented rules and
// independent of the implementation: who may open which case.
func (e *isoEnv) canRead(principal, c string) bool {
	if principal == "admin" || c == "demo" {
		return true
	}
	switch principal {
	case "carol", "dave": // a workspace; "dave" joined carol's
		return c == "carol" || c == "dave"
	case "erin":
		return false
	case "acme":
		return c == "acme"
	case "beta":
		return false
	}
	// Anonymous principals. Key and user cases are private even with ownership off.
	if c == "acme" || c == "carol" || c == "dave" {
		return false
	}
	if !e.own {
		return true
	}
	return principal == c && (principal == "alice" || principal == "bob")
}

func TestIsolationMatrix(t *testing.T) {
	for _, ownership := range []bool{true, false} {
		t.Run(fmt.Sprintf("CASE_OWNERSHIP=%v", ownership), func(t *testing.T) {
			e := newIsoEnv(t, ownership)
			caseNames := []string{"alice", "bob", "noid", "badid", "acme", "carol", "dave", "demo"}
			before := e.snapshot()
			missing := "case_000000000000"
			for _, rt := range isoRoutes() {
				if strings.HasSuffix(rt.pattern, "/pin") {
					continue // admin-only; see TestIsolationBadCredentialsAndAdminRoutes
				}
				for p, h := range e.headrs {
					if p == "admin" {
						continue
					}
					for _, c := range caseNames {
						if e.canRead(p, c) || (c == "demo") {
							continue // allowed cells run on fresh cases in TestIsolationAllowedCells
						}
						got := e.call(rt, e.ids[c], h)
						gone := e.call(rt, missing, h)
						if got.code != http.StatusNotFound || !strings.HasPrefix(got.ct, "application/json") {
							t.Errorf("%s as %s on %s's case: %d (%s), want a JSON 404", rt.name, p, c, got.code, got.ct)
						}
						if got != gone {
							t.Errorf("%s as %s on %s's case reveals it exists: %+v vs the missing-case answer %+v", rt.name, p, c, got, gone)
						}
					}
				}
			}
			if after := e.snapshot(); after != before {
				t.Error("a refused request changed a stored case")
			}
		})
	}
}

// snapshot is every stored case, serialized, in ID order.
func (e *isoEnv) snapshot() string {
	e.t.Helper()
	all, err := e.cs.List(context.Background())
	if err != nil {
		e.t.Fatal(err)
	}
	sort.Slice(all, func(i, j int) bool { return all[i].ID < all[j].ID })
	b, err := json.Marshal(all)
	if err != nil {
		e.t.Fatal(err)
	}
	return string(b)
}

// Every principal can use every case route on a case it may see: a fresh case
// per cell, because some routes delete or change it.
func TestIsolationAllowedCells(t *testing.T) {
	for _, ownership := range []bool{true, false} {
		t.Run(fmt.Sprintf("CASE_OWNERSHIP=%v", ownership), func(t *testing.T) {
			e := newIsoEnv(t, ownership)
			owners := []string{"alice", "bob", "acme", "carol", "dave"}
			for _, rt := range isoRoutes() {
				if strings.HasSuffix(rt.pattern, "/pin") {
					continue
				}
				for _, p := range owners {
					id := e.hs.newCase(t, e.headrs[p])
					if got := e.call(rt, id, e.headrs[p]); (got.code == http.StatusNotFound && !noSuchLink(got.body)) || got.code == http.StatusUnauthorized || got.code >= 500 {
						t.Errorf("%s as %s on their own fresh case = %d %s", rt.name, p, got.code, got.body)
					}
					id = e.hs.newCase(t, e.headrs[p])
					if got := e.call(rt, id, admin); (got.code == http.StatusNotFound && !noSuchLink(got.body)) || got.code >= 500 {
						t.Errorf("%s as admin on %s's case = %d %s", rt.name, p, got.code, got.body)
					}
				}
				// With ownership off, anonymous visitors share anonymous cases.
				if !ownership {
					id := e.hs.newCase(t, e.headrs["bob"])
					if got := e.call(rt, id, e.headrs["alice"]); (got.code == http.StatusNotFound && !noSuchLink(got.body)) || got.code >= 500 {
						t.Errorf("ownership off: %s as alice on bob's anonymous case = %d", rt.name, got.code)
					}
				}
			}
		})
	}
}

// A pinned demo is readable by everyone and read-only for everyone but the admin.
func TestIsolationPinnedDemoIsReadOnlyForNonAdmins(t *testing.T) {
	for _, ownership := range []bool{true, false} {
		t.Run(fmt.Sprintf("CASE_OWNERSHIP=%v", ownership), func(t *testing.T) {
			e := newIsoEnv(t, ownership)
			demo := e.ids["demo"]
			refused := map[string]bool{"rename": true, "delete": true, "economics": true, "answers": true, "review create": true, "review revoke": true}
			for _, rt := range isoRoutes() {
				if strings.HasSuffix(rt.pattern, "/pin") {
					continue
				}
				for p, h := range e.headrs {
					if p == "admin" {
						continue
					}
					before := e.snapshot()
					got := e.call(rt, demo, h)
					if got.code == http.StatusNotFound || got.code == http.StatusUnauthorized {
						t.Errorf("%s as %s on the pinned demo = %d, a demo is open to everyone", rt.name, p, got.code)
					}
					if refused[rt.name] && got.code != http.StatusForbidden {
						t.Errorf("%s as %s on the pinned demo = %d, want 403", rt.name, p, got.code)
					}
					if after := e.snapshot(); after != before {
						t.Errorf("%s as %s changed the pinned demo", rt.name, p)
					}
				}
			}
			if code, _ := e.hs.doAs(t, "GET", "/v1/cases/"+demo, "", nil); code != http.StatusOK {
				t.Errorf("an anonymous visitor cannot read the demo: %d", code)
			}
		})
	}
}

// Routes that name no case still do not widen access: an unverifiable
// credential is 401, whatever the route, and admin routes need the token.
func TestIsolationBadCredentialsAndAdminRoutes(t *testing.T) {
	for _, ownership := range []bool{true, false} {
		t.Run(fmt.Sprintf("CASE_OWNERSHIP=%v", ownership), func(t *testing.T) {
			e := newIsoEnv(t, ownership)
			bad := map[string]map[string]string{
				"garbage user token": {"X-User-Token": "not-a-token"},
				"empty-ish bearer":   {"X-User-Token": "Bearer nope"},
				"oversized token":    {"X-User-Token": "tok-" + strings.Repeat("x", maxUserToken)},
				"unknown api key":    {"X-API-Key": "op_unknown_secret"},
			}
			for name, h := range bad {
				for _, c := range []string{"alice", "demo"} {
					for _, rt := range isoRoutes() {
						if got := e.call(rt, e.ids[c], h); got.code != http.StatusUnauthorized {
							t.Errorf("%s: %s on %s = %d, want 401 (never a silent anonymous fallback)", name, rt.name, c, got.code)
						}
					}
				}
				if code, _ := e.hs.doAs(t, "GET", "/v1/cases", "", h); code != http.StatusUnauthorized {
					t.Errorf("%s: list = %d, want 401", name, code)
				}
				if code, _ := e.hs.doAs(t, "POST", "/v1/cases/predict", heatGunBody(t), h); code != http.StatusUnauthorized {
					t.Errorf("%s: predict = %d, want 401, and nothing stored", name, code)
				}
			}
			for p, h := range e.headrs {
				if p == "admin" {
					continue
				}
				for _, c := range []string{"alice", "demo"} {
					if got := e.call(isoRoute{"pin", "PUT", "/pin", `{"pinned":true}`, nil, ""}, e.ids[c], h); got.code != http.StatusForbidden {
						t.Errorf("pin as %s on %s = %d, want 403", p, c, got.code)
					}
				}
				if code, _ := e.hs.doAs(t, "POST", "/v1/watch/run", "", h); code != http.StatusForbidden {
					t.Errorf("watch/run as %s = %d, want 403", p, code)
				}
				if code, _ := e.hs.doAs(t, "GET", "/v1/admin/api-keys", "", h); code != http.StatusForbidden {
					t.Errorf("list api keys as %s = %d, want 403", p, code)
				}
			}
			wrong := map[string]string{"X-Admin-Token": testAdmin + "x"}
			if got := e.call(isoRoute{"get", "GET", "", "", nil, ""}, e.ids["alice"], with(wrong, e.headrs["bob"])); (got.code == http.StatusOK) != !ownership {
				t.Errorf("a wrong admin token acted as admin or broke access: %d", got.code)
			}
			// Public pages stay open to anonymous callers.
			for _, p := range []string{"/health", "/v1/demos", "/v1/duty-rates", "/v1/eval/summary"} {
				if code, _ := e.hs.doAs(t, "GET", p, "", nil); code != http.StatusOK {
					t.Errorf("GET %s anonymous = %d", p, code)
				}
			}
		})
	}
}

// Headers that look like a tenant, role or owner grant nothing.
func TestIsolationForgedIdentityHeadersGrantNothing(t *testing.T) {
	e := newIsoEnv(t, true)
	_, mine := e.hs.doAs(t, "GET", "/v1/workspaces/mine", "", user("carol"))
	wsID, _ := mine["workspace"].(map[string]any)["id"].(string)
	if wsID == "" {
		t.Fatalf("no workspace for carol: %v", mine)
	}
	forged := map[string]string{
		"X-Tenant": wsID, "X-Workspace": wsID, "X-Workspace-ID": wsID, "X-Role": "admin", "X-User-Role": "admin",
		"X-Owner": userOwner("carol@example.com"), "X-User-Email": "carol@example.com", "X-Forwarded-User": "carol@example.com",
		"X-Goog-Authenticated-User-Email": "accounts.google.com:carol@example.com", "X-Is-Admin": "true",
		// the stored owner hash, sent where a raw visitor ID goes: it is hashed again
		"X-Visitor-ID": visitorOwner(isoAlice),
	}
	for _, c := range []string{"alice", "carol", "dave", "acme", "bob"} {
		for _, rt := range isoRoutes() {
			if strings.HasSuffix(rt.pattern, "/pin") {
				continue
			}
			if got := e.call(rt, e.ids[c], forged); got.code != http.StatusNotFound {
				t.Errorf("forged headers: %s on %s's case = %d, want 404", rt.name, c, got.code)
			}
		}
	}
	ids := e.hs.listIDs(t, forged)
	for _, c := range []string{"alice", "bob", "carol", "dave", "acme"} {
		if ids[e.ids[c]] {
			t.Errorf("forged headers list %s's case", c)
		}
	}
	// A case made with forged headers is still owned by the real credential (none).
	if code, out := e.hs.doAs(t, "POST", "/v1/cases/predict", heatGunBody(t), forged); code == http.StatusCreated {
		c, err := e.cs.Get(context.Background(), out["id"].(string))
		if err != nil || c.Workspace != "" || strings.HasPrefix(c.Owner, userOwnerPrefix) || strings.HasPrefix(c.Owner, keyOwnerPrefix) {
			t.Errorf("a forged-header case was filed as %q workspace %q (%v)", c.Owner, c.Workspace, err)
		}
	}
}

// Lists show exactly the cases the caller may open, with ownership on and off.
func TestIsolationListMatchesReadAccess(t *testing.T) {
	for _, ownership := range []bool{true, false} {
		t.Run(fmt.Sprintf("CASE_OWNERSHIP=%v", ownership), func(t *testing.T) {
			e := newIsoEnv(t, ownership)
			caseNames := []string{"alice", "bob", "noid", "badid", "acme", "carol", "dave", "demo"}
			for p, h := range e.headrs {
				ids := e.hs.listIDs(t, h)
				for _, c := range caseNames {
					want := e.canRead(p, c)
					if ids[e.ids[c]] != want {
						t.Errorf("list as %s: has %s's case = %v, want %v", p, c, ids[e.ids[c]], want)
					}
				}
			}
		})
	}
}

// Streamed and batch creation file cases under the caller, like predict.
func TestIsolationStreamAndBatchCreateOwnedCases(t *testing.T) {
	e := newIsoEnv(t, true)
	before := map[string]bool{}
	all, _ := e.cs.List(context.Background())
	for _, c := range all {
		before[c.ID] = true
	}
	req := httptest.NewRequest("POST", "/v1/cases/predict", strings.NewReader(`{"date":"2026-01-01","facts":"A heat gun is assembled in Thailand."}`))
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("X-Visitor-ID", isoAlice)
	e.hs.h.ServeHTTP(httptest.NewRecorder(), req)
	code, out := e.hs.doAs(t, "POST", "/v1/cases/batch", `{"items":[{"name":"Fan","facts":"A fan with the motor in Thailand, assembled in Thailand.","date":"2022-06-01"}]}`, e.headrs["alice"])
	if code != http.StatusOK {
		t.Fatalf("batch = %d %v", code, out)
	}
	all, _ = e.cs.List(context.Background())
	made := 0
	for _, c := range all {
		if before[c.ID] {
			continue
		}
		made++
		if c.Owner != visitorOwner(isoAlice) {
			t.Errorf("new case %s is owned by %q, want alice's visitor hash", c.ID, c.Owner)
		}
		if code, _ := e.hs.doAs(t, "GET", "/v1/cases/"+c.ID, "", e.headrs["bob"]); code != http.StatusNotFound {
			t.Errorf("bob opens alice's streamed or batch case %s: %d", c.ID, code)
		}
	}
	if made < 2 {
		t.Errorf("made %d new cases, want the streamed and the batch case", made)
	}
}

// With 230 newer cases from other visitors, the default cap of 200 must not
// push a caller's own older case out, or let another visitor's case in.
func TestIsolationListFiltersBeforeTheDefaultCap(t *testing.T) {
	for _, ownership := range []bool{true, false} {
		t.Run(fmt.Sprintf("CASE_OWNERSHIP=%v", ownership), func(t *testing.T) {
			cs := store.NewMemory()
			hs, _, _ := teamsHarness(t, func(d *Deps) { d.Cases, d.CaseOwnership = cs, ownership }) // CasesListLimit unset: 200
			now := time.Now().UTC().Truncate(time.Second)
			put := func(id string, c store.Case, age time.Duration) {
				c.ID, c.CreatedAt, c.UpdatedAt = id, now.Add(-age-time.Hour), now.Add(-age)
				if err := cs.Put(context.Background(), c); err != nil {
					t.Fatal(err)
				}
			}
			put("case_alice_old", store.Case{Owner: visitorOwner(isoAlice)}, 1000*time.Hour)
			put("case_demo", store.Case{Pinned: true, Owner: visitorOwner("someone-else-visitor-01")}, 2000*time.Hour)
			for i := 0; i < 230; i++ {
				owner := visitorOwner(fmt.Sprintf("other-visitor-id-%05d", i))
				if i%5 == 0 {
					owner = userOwner(fmt.Sprintf("u%d@example.com", i))
				}
				put(fmt.Sprintf("case_other_%03d", i), store.Case{Owner: owner}, time.Duration(i+1)*time.Minute)
			}
			ids := hs.listIDs(t, map[string]string{"X-Visitor-ID": isoAlice})
			if !ids["case_demo"] {
				t.Error("the pinned demo is missing from the list")
			}
			if ownership {
				if len(ids) != 2 || !ids["case_alice_old"] {
					t.Errorf("ownership on: alice sees %d cases %v, want her own case and the demo only", len(ids), ids)
				}
				return
			}
			// Off: the public-demo intent. Anonymous cases are shared and the 200
			// newest show; user-owned cases never do. The own case is older than
			// the 200th newest, so it is capped out, which is the documented
			// behavior of the default.
			for id := range ids {
				if strings.HasPrefix(id, "case_other_") {
					var i int
					fmt.Sscanf(id, "case_other_%d", &i)
					if i%5 == 0 {
						t.Errorf("ownership off: a signed-in user's case %s is in an anonymous list", id)
					}
				}
			}
			if n := len(ids); n > 201 {
				t.Errorf("ownership off: %d cases listed, the cap is the demo plus 200", n)
			}
		})
	}
}

// noSuchLink reports the revoke route's answer for a link id the caller's own
// (visible) case does not have: a 404 that names the link, never the case.
// The fixed id in isoRoutes exists on no case, so an allowed revoke gets it.
func noSuchLink(body string) bool { return errCode(body) == "review_not_found" }

// A broker review token (D200) opens its own link's snapshot and nothing
// else: it is no identity on any case route, it cannot reach another case's
// links, and no principal reaches a link without its token.
func TestIsolationReviewTokenOpensOneLinkOnly(t *testing.T) {
	for _, ownership := range []bool{true, false} {
		t.Run(fmt.Sprintf("CASE_OWNERSHIP=%v", ownership), func(t *testing.T) {
			e := newIsoEnv(t, ownership)
			code, out := e.hs.doAs(t, "POST", "/v1/cases/"+e.ids["acme"]+"/reviews", `{}`, e.headrs["acme"])
			if code != http.StatusCreated {
				t.Fatalf("acme makes a link = %d %v", code, out)
			}
			tok := map[string]string{reviewTokenHeader: out["token"].(string)}
			before := e.snapshot()
			// The token alone, on every case route of every case: as if no credential.
			for _, rt := range isoRoutes() {
				if strings.HasSuffix(rt.pattern, "/pin") {
					continue
				}
				for _, c := range []string{"acme", "carol", "alice"} {
					if c == "alice" && !ownership {
						continue // an anonymous case is shared with everyone with ownership off
					}
					if got := e.call(rt, e.ids[c], tok); got.code != http.StatusNotFound {
						t.Errorf("%s on %s's case with only acme's review token = %d %s", rt.name, c, got.code, got.body)
					}
				}
			}
			if after := e.snapshot(); after != before {
				t.Error("a review token changed a stored case through a case route")
			}
			// Every principal, without the token, gets nothing from the broker routes.
			for p, h := range e.headrs {
				if code, _ := e.hs.doAs(t, "GET", "/v1/review", "", h); code != http.StatusNotFound && code != http.StatusTooManyRequests {
					t.Errorf("GET /v1/review as %s without a token = %d", p, code)
				}
				if code, _ := e.hs.doAs(t, "POST", "/v1/review/decision", `{"decision":"approve"}`, h); code != http.StatusNotFound && code != http.StatusTooManyRequests {
					t.Errorf("POST /v1/review/decision as %s without a token = %d", p, code)
				}
			}
			// With the token, the packet is acme's snapshot only: no other case's id.
			rec := httptest.NewRecorder()
			req := httptest.NewRequest("GET", "/v1/review", nil)
			req.Header.Set(reviewTokenHeader, tok[reviewTokenHeader])
			req.Header.Set("X-Client-IP", "198.51.100.77") // its own failure budget
			e.hs.h.ServeHTTP(rec, req)
			if rec.Code != http.StatusOK {
				t.Fatalf("broker read = %d %s", rec.Code, rec.Body)
			}
			for name, id := range e.ids {
				if strings.Contains(rec.Body.String(), id) {
					t.Errorf("the packet names %s's case id", name)
				}
			}
		})
	}
}
