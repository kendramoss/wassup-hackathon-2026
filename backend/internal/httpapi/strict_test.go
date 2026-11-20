package httpapi

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
)

// STRICT_AUTH (#283): private case operations need a verified principal; a
// visitor ID alone is not one. Run over CASE_OWNERSHIP x strict x principal
// kind x every case-scoped route, against a twin environment with strict off.

var strictUnverified = []string{"stranger", "badid", "alice", "bob"}
var strictVerified = []string{"acme", "beta", "carol", "dave", "erin", "admin"}

func TestStrictAuthRefusesUnverifiedOnEveryPrivateCaseRoute(t *testing.T) {
	for _, ownership := range []bool{true, false} {
		for _, rt := range isoRoutes() {
			if strings.HasSuffix(rt.pattern, "/pin") {
				continue
			}
			t.Run(fmt.Sprintf("own=%v/%s", ownership, rt.name), func(t *testing.T) {
				strict, relaxed := newIsoEnvStrict(t, ownership, true), newIsoEnvStrict(t, ownership, false)
				caseNames := []string{"alice", "bob", "noid", "badid", "acme", "carol", "dave", "demo"}
				for _, p := range strictUnverified {
					h := strict.headrs[p]
					gone := strict.call(rt, "case_000000000000", h)
					if gone.code != http.StatusUnauthorized || errCode(gone.body) != "unauthenticated" || !strings.HasPrefix(gone.ct, "application/json") {
						t.Errorf("%s as %s on a missing case = %+v, want a JSON 401 unauthenticated", rt.name, p, gone)
					}
					for _, c := range caseNames {
						got := strict.call(rt, strict.ids[c], h)
						if c == "demo" {
							// Pinned public reads keep working exactly as with strict off.
							if want := relaxed.call(rt, relaxed.ids[c], relaxed.headrs[p]); got.code != want.code {
								t.Errorf("%s as %s on the pinned demo = %d, strict off gives %d", rt.name, p, got.code, want.code)
							}
							continue
						}
						// Own case, other's case, hidden or not: one answer, so nothing leaks.
						if got != gone {
							t.Errorf("%s as %s on %s's case = %+v, want the same 401 as a missing case %+v", rt.name, p, c, got, gone)
						}
					}
				}
				for _, p := range strictVerified {
					for _, c := range append(caseNames, "missing") {
						sid, rid := "case_000000000000", "case_000000000000"
						if c != "missing" {
							sid, rid = strict.ids[c], relaxed.ids[c]
						}
						got, want := strict.call(rt, sid, strict.headrs[p]), relaxed.call(rt, rid, relaxed.headrs[p])
						if got.code != want.code {
							t.Errorf("%s as verified %s on %s: strict %d, off %d (strict changes nothing for a verified principal)", rt.name, p, c, got.code, want.code)
						}
					}
				}
			})
		}
	}
}

func TestStrictAuthCreateAndListNeedVerifiedPrincipal(t *testing.T) {
	for _, ownership := range []bool{true, false} {
		t.Run(fmt.Sprintf("CASE_OWNERSHIP=%v", ownership), func(t *testing.T) {
			e := newIsoEnvStrict(t, ownership, true)
			batch := `{"items":[{"name":"gun","facts":"A heat gun is assembled in Thailand."}]}`
			before := e.snapshot()
			for _, p := range strictUnverified {
				h := e.headrs[p]
				for name, req := range map[string][3]string{
					"predict": {"POST", "/v1/cases/predict", heatGunBody(t)},
					"batch":   {"POST", "/v1/cases/batch", batch},
					"list":    {"GET", "/v1/cases", ""},
				} {
					code, out := e.hs.doAs(t, req[0], req[1], req[2], h)
					if code != http.StatusUnauthorized || out["code"] != "unauthenticated" {
						t.Errorf("%s as %s = %d %v, want 401 unauthenticated", name, p, code, out)
					}
				}
				if code, _ := e.hs.doAs(t, "POST", "/v1/cases/predict", heatGunBody(t), with(h, map[string]string{"Accept": "text/event-stream"})); code != http.StatusUnauthorized {
					t.Errorf("streamed predict as %s = %d", p, code)
				}
			}
			if e.snapshot() != before {
				t.Error("a refused create stored a case")
			}
			// A visitor id plus a wrong admin token is still not verified.
			if code, _ := e.hs.doAs(t, "POST", "/v1/cases/predict", heatGunBody(t), with(e.headrs["alice"], map[string]string{"X-Admin-Token": "wrong"})); code != http.StatusUnauthorized {
				t.Errorf("visitor + wrong admin token = %d", code)
			}
			for _, p := range []string{"acme", "carol", "dave", "erin", "admin"} {
				h := e.headrs[p]
				if code, out := e.hs.doAs(t, "POST", "/v1/cases/predict", heatGunBody(t), h); code != http.StatusCreated {
					t.Errorf("predict as verified %s = %d %v", p, code, out)
				}
				if code, _ := e.hs.doAs(t, "GET", "/v1/cases", "", h); code != http.StatusOK {
					t.Errorf("list as verified %s = %d", p, code)
				}
			}
			// A verified user who also carries the visitor id gets their own and the browser's cases.
			if ids := e.hs.listIDs(t, with(e.headrs["erin"], e.headrs["alice"])); !ids[e.ids["alice"]] && ownership {
				t.Errorf("verified user lost the browser's anonymous case: %v", ids)
			}
		})
	}
}

// Public, non-case endpoints and pinned demos stay available to everyone.
func TestStrictAuthKeepsPublicEndpointsOpen(t *testing.T) {
	e := newIsoEnvStrict(t, true, true)
	for _, p := range []string{"/health", "/v1/demos", "/v1/duty-rates", "/v1/eval/summary", "/v1/demos/" + e.firstDemo(), "/v1/cases/" + e.ids["demo"]} {
		for _, who := range []string{"stranger", "alice", "badid"} {
			if code, _ := e.hs.doAs(t, "GET", p, "", e.headrs[who]); code != http.StatusOK {
				t.Errorf("GET %s as %s under STRICT_AUTH = %d, want 200", p, who, code)
			}
		}
	}
	// Workspace routes already need a user; strict does not change their answers.
	if code, _ := e.hs.doAs(t, "GET", "/v1/workspaces/mine", "", e.headrs["alice"]); code != http.StatusUnauthorized {
		t.Errorf("workspaces/mine as a visitor = %d", code)
	}
	// Admin routes still want the admin token (403), not a generic 401.
	if code, _ := e.hs.doAs(t, "GET", "/v1/admin/api-keys", "", e.headrs["alice"]); code != http.StatusForbidden {
		t.Errorf("admin route as a visitor = %d, want 403", code)
	}
}

// An unverifiable credential is 401 whatever the mode: never an anonymous fallback.
func TestStrictAuthInvalidCredentialsStay401(t *testing.T) {
	for _, strict := range []bool{true, false} {
		e := newIsoEnvStrict(t, true, strict)
		bad := map[string]map[string]string{
			"garbage token":                    {"X-User-Token": "not-a-token"},
			"unknown key":                      {"X-API-Key": "op_unknown_secret"},
			"bad token with a good visitor id": with(map[string]string{"X-User-Token": "nope"}, e.headrs["alice"]),
		}
		for name, h := range bad {
			for _, c := range []string{"alice", "demo"} {
				for _, rt := range isoRoutes() {
					if got := e.call(rt, e.ids[c], h); got.code != http.StatusUnauthorized {
						t.Errorf("strict=%v %s: %s on %s = %d, want 401", strict, name, rt.name, c, got.code)
					}
				}
			}
			if code, _ := e.hs.doAs(t, "GET", "/v1/cases", "", h); code != http.StatusUnauthorized {
				t.Errorf("strict=%v %s: list = %d", strict, name, code)
			}
		}
	}
}

// With STRICT_AUTH off (the default) a visitor id works exactly as before.
func TestStrictAuthOffLeavesVisitorFlowUnchanged(t *testing.T) {
	e := newIsoEnvStrict(t, true, false)
	h := e.headrs["alice"]
	if code, _ := e.hs.doAs(t, "POST", "/v1/cases/predict", heatGunBody(t), h); code != http.StatusCreated {
		t.Errorf("visitor predict with strict off = %d", code)
	}
	if code, _ := e.hs.doAs(t, "GET", "/v1/cases/"+e.ids["alice"], "", h); code != http.StatusOK {
		t.Errorf("visitor read with strict off = %d", code)
	}
	if code, _ := e.hs.doAs(t, "GET", "/v1/cases", "", h); code != http.StatusOK {
		t.Errorf("visitor list with strict off = %d", code)
	}
}

// firstDemo is the ID of the first listed demo (the demos are static fixtures, not stored cases).
func (e *isoEnv) firstDemo() string {
	_, out := e.hs.doAs(e.t, "GET", "/v1/demos", "", nil)
	list, _ := out["demos"].([]any)
	if len(list) == 0 {
		e.t.Fatalf("no demos listed: %v", out)
	}
	return list[0].(map[string]any)["id"].(string)
}
