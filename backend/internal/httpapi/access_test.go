package httpapi

import (
	"fmt"
	"net/http"
	"testing"
)

// can_write (#283 capability): a read-only boolean on every case DTO and list
// row that says whether this caller's write routes would pass authorization
// for this case. The test does not read the implementation: for every
// principal and every case it can open, it compares can_write with what a real
// PATCH (rename, the cheapest write route) answers.

func canWriteOf(t *testing.T, out map[string]any) (bool, bool) {
	t.Helper()
	v, ok := out["can_write"]
	if !ok {
		return false, false
	}
	b, isBool := v.(bool)
	if !isBool {
		t.Fatalf("can_write = %#v, want a boolean", v)
	}
	return b, true
}

// renameAllowed is whether the caller's write route accepts the case: 200 is
// yes, 403 is no. Anything else is a test failure (the case was visible).
func renameAllowed(t *testing.T, hs harness, id string, h map[string]string) bool {
	t.Helper()
	code, out := hs.doAs(t, "PATCH", "/v1/cases/"+id, `{"display_name":"probe"}`, h)
	switch code {
	case http.StatusOK:
		return true
	case http.StatusForbidden:
		return false
	}
	t.Fatalf("rename probe on a visible case = %d %v", code, out)
	return false
}

func TestCanWriteMatchesAuthorizationAcrossPrincipalsAndOwnership(t *testing.T) {
	for _, ownership := range []bool{true, false} {
		t.Run(fmt.Sprintf("CASE_OWNERSHIP=%v", ownership), func(t *testing.T) {
			e := newIsoEnv(t, ownership)
			caseNames := []string{"alice", "bob", "noid", "badid", "acme", "carol", "dave", "demo"}
			for p, h := range e.headrs {
				for _, c := range caseNames {
					id := e.ids[c]
					code, out := e.hs.doAs(t, "GET", "/v1/cases/"+id, "", h)
					if code == http.StatusNotFound || code == http.StatusUnauthorized {
						if _, leaked := canWriteOf(t, out); leaked {
							t.Errorf("%s on %s's hidden case: a %d answer carries can_write", p, c, code)
						}
						continue
					}
					if code != http.StatusOK {
						t.Fatalf("%s GET %s = %d %v", p, c, code, out)
					}
					got, present := canWriteOf(t, out)
					if !present {
						t.Fatalf("%s on %s: GET has no can_write", p, c)
					}
					want := renameAllowed(t, e.hs, id, h)
					if got != want {
						t.Errorf("%s on %s's case: can_write=%v but rename answers allowed=%v", p, c, got, want)
					}
					// Pinned demo: writable for the admin token only.
					if c == "demo" && got != (p == "admin") {
						t.Errorf("%s on the pinned demo: can_write=%v", p, got)
					}
				}
			}
		})
	}
}

func TestCanWriteOnListRowsEqualsCaseResponse(t *testing.T) {
	for _, ownership := range []bool{true, false} {
		t.Run(fmt.Sprintf("CASE_OWNERSHIP=%v", ownership), func(t *testing.T) {
			e := newIsoEnv(t, ownership)
			for p, h := range e.headrs {
				code, out := e.hs.doAs(t, "GET", "/v1/cases", "", h)
				if code != http.StatusOK {
					t.Fatalf("%s list = %d", p, code)
				}
				rows, _ := out["cases"].([]any)
				for _, r := range rows {
					row := r.(map[string]any)
					id := row["id"].(string)
					listed, ok := canWriteOf(t, row)
					if !ok {
						t.Fatalf("%s: list row %s has no can_write", p, id)
					}
					_, one := e.hs.doAs(t, "GET", "/v1/cases/"+id, "", h)
					single, _ := canWriteOf(t, one)
					if listed != single {
						t.Errorf("%s: row %s can_write=%v, case response %v", p, id, listed, single)
					}
				}
			}
		})
	}
}

func TestCanWriteWorkspaceRoles(t *testing.T) {
	for _, ownership := range []bool{true, false} {
		t.Run(fmt.Sprintf("CASE_OWNERSHIP=%v", ownership), func(t *testing.T) {
			e := newRoleEnv(t, ownership)
			id := e.wsCase()
			for who, want := range map[string]bool{"carol": true, "dave": true, "vera": false} {
				_, out := e.hs.doAs(t, "GET", "/v1/cases/"+id, "", user(who))
				if got, ok := canWriteOf(t, out); !ok || got != want {
					t.Errorf("%s on a workspace case: can_write=%v present=%v, want %v", who, got, ok, want)
				}
			}
			// A viewer's own pre-workspace case is hers to change: the role
			// governs workspace cases only.
			_, out := e.hs.doAs(t, "GET", "/v1/cases/"+e.veraOwn, "", user("vera"))
			if got, _ := canWriteOf(t, out); !got {
				t.Error("viewer on her own personal case: can_write=false, want true")
			}
			if !renameAllowed(t, e.hs, e.veraOwn, user("vera")) {
				t.Error("viewer cannot rename her personal case")
			}
			// The admin token wins over the user token whatever the role.
			_, out = e.hs.doAs(t, "GET", "/v1/cases/"+id, "", with(admin, user("vera")))
			if got, _ := canWriteOf(t, out); !got {
				t.Error("admin token with a viewer's user token: can_write=false, want true")
			}
			// Another workspace's case stays hidden, with no capability.
			zcase := e.hs.newCase(t, user("zed"))
			code, out := e.hs.doAs(t, "GET", "/v1/cases/"+zcase, "", user("vera"))
			if _, leaked := canWriteOf(t, out); code != http.StatusNotFound || leaked {
				t.Errorf("other workspace's case for vera = %d, can_write present=%v", code, leaked)
			}
			// Create (201), rename and list rows carry it too.
			_, created := e.hs.doAs(t, "POST", "/v1/cases/predict", heatGunBody(t), user("dave"))
			if got, ok := canWriteOf(t, created); !ok || !got {
				t.Errorf("created case: can_write=%v present=%v", got, ok)
			}
			_, renamed := e.hs.doAs(t, "PATCH", "/v1/cases/"+id, `{"display_name":"x"}`, user("dave"))
			if got, ok := canWriteOf(t, renamed); !ok || !got {
				t.Errorf("rename response row: can_write=%v present=%v", got, ok)
			}
			_, list := e.hs.doAs(t, "GET", "/v1/cases", "", user("vera"))
			for _, r := range list["cases"].([]any) {
				row := r.(map[string]any)
				got, ok := canWriteOf(t, row)
				if !ok {
					t.Fatalf("row %v has no can_write", row["id"])
				}
				if row["id"] == id && got {
					t.Error("viewer's list row for a workspace case says can_write=true")
				}
				if row["id"] == e.veraOwn && !got {
					t.Error("viewer's list row for her personal case says can_write=false")
				}
			}
		})
	}
}

func TestCanWriteUnderStrictAuth(t *testing.T) {
	e := newIsoEnvStrict(t, true, true)
	for _, p := range []string{"carol", "dave", "acme"} {
		id := e.ids[p]
		_, out := e.hs.doAs(t, "GET", "/v1/cases/"+id, "", e.headrs[p])
		if got, ok := canWriteOf(t, out); !ok || !got {
			t.Errorf("strict: %s on their case: can_write=%v present=%v", p, got, ok)
		}
	}
	// The pinned demo is still open to an unverified visitor, read-only.
	_, out := e.hs.doAs(t, "GET", "/v1/cases/"+e.ids["demo"], "", e.headrs["alice"])
	if got, ok := canWriteOf(t, out); !ok || got {
		t.Errorf("strict: visitor on the pinned demo: can_write=%v present=%v, want false", got, ok)
	}
	// And an unverified visitor still cannot learn anything about a private case.
	code, out := e.hs.doAs(t, "GET", "/v1/cases/"+e.ids["alice"], "", e.headrs["alice"])
	if _, leaked := canWriteOf(t, out); code != http.StatusUnauthorized || leaked {
		t.Errorf("strict: visitor on a private case = %d, can_write present=%v", code, leaked)
	}
}

// A removed member loses the workspace's cases entirely: hidden, so no DTO and
// no capability, even for the cases they made.
func TestCanWriteRemovedMemberSeesNothing(t *testing.T) {
	e := newRoleEnv(t, true)
	own := e.hs.newCase(t, user("dave")) // dave is an editor and made it
	if code, out := e.hs.doAs(t, "DELETE", e.memberPath(e.wsID, "dave"), "", user("carol")); code != http.StatusOK {
		t.Fatalf("remove dave = %d %v", code, out)
	}
	code, out := e.hs.doAs(t, "GET", "/v1/cases/"+own, "", user("dave"))
	if _, leaked := canWriteOf(t, out); code != http.StatusNotFound || leaked {
		t.Errorf("removed member on the workspace case = %d, can_write present=%v", code, leaked)
	}
}
