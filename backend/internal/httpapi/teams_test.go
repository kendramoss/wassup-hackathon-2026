package httpapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/store"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/teams"
)

// fakeUsers verifies tokens of the form "tok-<name>" as <name>@example.com.
type fakeUsers struct{ calls int }

func (f *fakeUsers) Verify(_ context.Context, tok string) (teams.User, error) {
	f.calls++
	name, ok := strings.CutPrefix(tok, "tok-")
	if !ok {
		return teams.User{}, fmt.Errorf("%w: bad signature", teams.ErrUnauthenticated)
	}
	return teams.User{Email: name + "@example.com"}, nil
}

// failingWorkspaces is a workspace store that is down.
type failingWorkspaces struct{ teams.Store }

func (failingWorkspaces) Of(context.Context, string) (teams.Workspace, error) {
	return teams.Workspace{}, errors.New("firestore unavailable")
}

func user(name string) map[string]string { return map[string]string{"X-User-Token": "tok-" + name} }

// with merges request headers.
func with(hs ...map[string]string) map[string]string {
	out := map[string]string{}
	for _, h := range hs {
		for k, v := range h {
			out[k] = v
		}
	}
	return out
}

func teamsHarness(t *testing.T, adjust func(*Deps)) (harness, *fakeUsers, *store.Memory) {
	t.Helper()
	users, cases := &fakeUsers{}, store.NewMemory()
	hs := newHarnessWith(t, "substantial_transformation", func(d *Deps) {
		d.Teams, d.Users, d.Workspaces, d.Cases = true, users, teams.NewMemory(), cases
		if adjust != nil {
			adjust(d)
		}
	})
	return hs, users, cases
}

func (hs harness) newCase(t *testing.T, headers map[string]string) string {
	t.Helper()
	code, out := hs.doAs(t, "POST", "/v1/cases/predict", heatGunBody(t), headers)
	if code != http.StatusCreated {
		t.Fatalf("predict as %v = %d %v", headers, code, out)
	}
	return out["id"].(string)
}

func (hs harness) listIDs(t *testing.T, headers map[string]string) map[string]bool {
	t.Helper()
	code, out := hs.doAs(t, "GET", "/v1/cases", "", headers)
	if code != http.StatusOK {
		t.Fatalf("list as %v = %d %v", headers, code, out)
	}
	m := map[string]bool{}
	for _, c := range out["cases"].([]any) {
		m[c.(map[string]any)["id"].(string)] = true
	}
	return m
}

func TestWorkspaceEndpoints(t *testing.T) {
	hs, _, _ := teamsHarness(t, nil)
	if code, out := hs.do(t, "GET", "/v1/workspaces/mine", ""); code != http.StatusUnauthorized || out["code"] != "unauthenticated" {
		t.Errorf("anonymous mine = %d %v", code, out)
	}
	if code, _ := hs.do(t, "POST", "/v1/workspaces", `{"name":"Acme"}`); code != http.StatusUnauthorized {
		t.Errorf("anonymous create = %d", code)
	}
	code, mine := hs.doAs(t, "GET", "/v1/workspaces/mine", "", user("alice"))
	if code != http.StatusOK || mine["email"] != "alice@example.com" || mine["workspace"] != nil {
		t.Fatalf("mine before create = %d %v", code, mine)
	}
	for _, body := range []string{`{"name":"  "}`, `{"name":"` + strings.Repeat("x", teams.MaxName+1) + `"}`, `{"name":"A","extra":1}`} {
		if code, _ := hs.doAs(t, "POST", "/v1/workspaces", body, user("alice")); code != http.StatusBadRequest {
			t.Errorf("create %s = %d, want 400", body, code)
		}
	}
	code, ws := hs.doAs(t, "POST", "/v1/workspaces", `{"name":"  Acme   Imports "}`, user("alice"))
	if code != http.StatusCreated || ws["name"] != "Acme Imports" || ws["created_by"] != "alice@example.com" || ws["admin"] != true {
		t.Fatalf("create = %d %v", code, ws)
	}
	wsID := ws["id"].(string)
	if code, _ := hs.doAs(t, "POST", "/v1/workspaces", `{"name":"Second"}`, user("alice")); code != http.StatusConflict {
		t.Errorf("a second workspace for alice = %d, want 409", code)
	}
	members := "/v1/workspaces/" + wsID + "/members"
	if code, out := hs.doAs(t, "POST", members, `{"email":" Amy@Example.com "}`, user("alice")); code != http.StatusOK || len(out["members"].([]any)) != 2 {
		t.Fatalf("add amy = %d %v", code, out)
	}
	code, mine = hs.doAs(t, "GET", "/v1/workspaces/mine", "", user("amy"))
	if w, _ := mine["workspace"].(map[string]any); code != http.StatusOK || w["id"] != wsID || w["admin"] != false {
		t.Errorf("amy's workspace = %d %v", code, mine)
	}
	tests := []struct {
		name, as, path, body string
		want                 int
	}{
		{"member but not admin", "amy", members, `{"email":"zed@example.com"}`, http.StatusForbidden},
		{"outsider: workspace hidden", "bob", members, `{"email":"bob2@example.com"}`, http.StatusNotFound},
		{"unknown workspace", "alice", "/v1/workspaces/ws_nope/members", `{"email":"zed@example.com"}`, http.StatusNotFound},
		{"bad email", "alice", members, `{"email":"not-an-email"}`, http.StatusBadRequest},
		{"already a member", "alice", members, `{"email":"amy@example.com"}`, http.StatusConflict},
		{"anonymous", "", members, `{"email":"zed@example.com"}`, http.StatusUnauthorized},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var h map[string]string
			if tt.as != "" {
				h = user(tt.as)
			}
			if code, out := hs.doAs(t, "POST", tt.path, tt.body, h); code != tt.want {
				t.Errorf("= %d %v, want %d", code, out, tt.want)
			}
		})
	}
	// bob has his own workspace; alice cannot pull him into hers.
	hs.doAs(t, "POST", "/v1/workspaces", `{"name":"Bob Co"}`, user("bob"))
	if code, _ := hs.doAs(t, "POST", members, `{"email":"bob@example.com"}`, user("alice")); code != http.StatusConflict {
		t.Errorf("adding a member of another workspace = %d, want 409", code)
	}
}

func TestTeamsVisibility(t *testing.T) {
	for _, ownership := range []bool{false, true} {
		t.Run(fmt.Sprintf("case_ownership=%v", ownership), func(t *testing.T) {
			hs, _, _ := teamsHarness(t, func(d *Deps) { d.CaseOwnership = ownership })
			_, ws := hs.doAs(t, "POST", "/v1/workspaces", `{"name":"Acme"}`, user("alice"))
			hs.doAs(t, "POST", "/v1/workspaces/"+ws["id"].(string)+"/members", `{"email":"amy@example.com"}`, user("alice"))

			browser := map[string]string{"X-Visitor-ID": "alice-browser-id-00001"}
			other := map[string]string{"X-Visitor-ID": "someone-else-id-000002"}
			alice := hs.newCase(t, user("alice")) // in Acme
			amy := hs.newCase(t, user("amy"))     // in Acme
			bob := hs.newCase(t, user("bob"))     // signed in, no workspace
			anonSame := hs.newCase(t, browser)    // anonymous, alice's browser
			anonOther := hs.newCase(t, other)     // anonymous, another browser
			anonNone := hs.newCase(t, nil)        // anonymous, no visitor ID
			demo := hs.newCase(t, admin)          // pinned below
			hs.doAs(t, "PUT", "/v1/cases/"+demo+"/pin", `{"pinned":true}`, admin)

			all := []string{alice, amy, bob, anonSame, anonOther, anonNone, demo}
			names := map[string]string{alice: "alice's", amy: "amy's", bob: "bob's", anonSame: "anon (alice's browser)",
				anonOther: "anon (other browser)", anonNone: "anon (no visitor ID)", demo: "demo"}
			// Anonymous visitors keep D58: everything except signed-in
			// users' cases with ownership off, their own with it on.
			anonSees := func(own string) []string {
				if ownership {
					return []string{own, demo}
				}
				return []string{anonSame, anonOther, anonNone, demo}
			}
			viewers := []struct {
				name    string
				headers map[string]string
				sees    []string
			}{
				{"alice (admin of Acme), her browser", with(user("alice"), browser), []string{alice, amy, anonSame, demo}},
				{"alice, another device", user("alice"), []string{alice, amy, demo}},
				{"amy (member of Acme)", user("amy"), []string{alice, amy, demo}},
				{"bob (no workspace)", user("bob"), []string{bob, demo}},
				{"carol (signed in, nothing yet)", user("carol"), []string{demo}},
				{"anonymous, alice's browser", browser, anonSees(anonSame)},
				{"anonymous, other browser", other, anonSees(anonOther)},
				{"admin token", admin, all},
			}
			for _, v := range viewers {
				want := map[string]bool{}
				for _, id := range v.sees {
					want[id] = true
				}
				got := hs.listIDs(t, v.headers)
				for _, id := range all {
					if got[id] != want[id] {
						t.Errorf("%s: lists %s = %v, want %v", v.name, names[id], got[id], want[id])
					}
					code, _ := hs.doAs(t, "GET", "/v1/cases/"+id, "", v.headers)
					if wantCode := map[bool]int{true: http.StatusOK, false: http.StatusNotFound}[want[id]]; code != wantCode {
						t.Errorf("%s: GET %s = %d, want %d", v.name, names[id], code, wantCode)
					}
				}
			}
		})
	}
}

func TestTeamsCaseActions(t *testing.T) {
	hs, _, cases := teamsHarness(t, nil)
	_, ws := hs.doAs(t, "POST", "/v1/workspaces", `{"name":"Acme"}`, user("alice"))
	wsID := ws["id"].(string)
	hs.doAs(t, "POST", "/v1/workspaces/"+wsID+"/members", `{"email":"amy@example.com"}`, user("alice"))
	id := hs.newCase(t, user("alice"))

	stored, err := cases.Get(context.Background(), id)
	if err != nil || stored.Workspace != wsID || !strings.HasPrefix(stored.Owner, userOwnerPrefix) {
		t.Fatalf("stored case workspace %q owner %q %v", stored.Workspace, stored.Owner, err)
	}
	code, out := hs.doAs(t, "GET", "/v1/cases/"+id, "", user("amy"))
	if code != http.StatusOK {
		t.Fatalf("amy opens alice's case = %d", code)
	}
	for _, k := range []string{"owner", "workspace"} {
		if _, ok := out[k]; ok {
			t.Errorf("case response leaks %s", k)
		}
	}
	// A teammate's edits are recorded with their email.
	if code, _ := hs.doAs(t, "PATCH", "/v1/cases/"+id, `{"display_name":"Heat gun, Q3"}`, user("amy")); code != http.StatusOK {
		t.Fatalf("amy renames = %d", code)
	}
	if code, _ := hs.doAs(t, "POST", "/v1/cases/"+id+"/export", "", user("alice")); code != http.StatusOK {
		t.Fatalf("alice exports = %d", code)
	}
	_, out = hs.doAs(t, "GET", "/v1/cases/"+id, "", user("alice"))
	actors := map[string]string{}
	for _, e := range out["history"].([]any) {
		ev := e.(map[string]any)
		actors[ev["kind"].(string)], _ = ev["actor"].(string)
	}
	if actors["predicted"] != "alice@example.com" || actors["renamed"] != "amy@example.com" || actors["exported"] != "alice@example.com" {
		t.Errorf("history actors = %v", actors)
	}
	// Outsiders get 404 on every case endpoint.
	for _, req := range []struct{ method, path, body string }{
		{"GET", "", ""}, {"PATCH", "", `{"display_name":"x"}`}, {"DELETE", "", ""}, {"POST", "/plan", `{"target":"Thailand"}`},
		{"POST", "/export", ""}, {"PUT", "/economics", `{"annual_import_value_usd":1}`}, {"POST", "/ask", `{"question":"why?"}`},
	} {
		for name, h := range map[string]map[string]string{"bob": user("bob"), "anonymous": nil, "anonymous with a visitor ID": {"X-Visitor-ID": "some-visitor-id-000001"}} {
			if code, _ := hs.doAs(t, req.method, "/v1/cases/"+id+req.path, req.body, h); code != http.StatusNotFound {
				t.Errorf("%s %s alice's case%s = %d, want 404", name, req.method, req.path, code)
			}
		}
	}
	// Anonymous cases record no actor.
	anon := hs.newCase(t, nil)
	_, out = hs.do(t, "GET", "/v1/cases/"+anon, "")
	if ev := out["history"].([]any)[0].(map[string]any); ev["actor"] != nil {
		t.Errorf("anonymous event actor = %v", ev["actor"])
	}
	if code, _ := hs.doAs(t, "DELETE", "/v1/cases/"+id, "", user("amy")); code != http.StatusNoContent {
		t.Errorf("a member deletes a workspace case = %d", code)
	}
}

func TestTeamsTokens(t *testing.T) {
	hs, users, _ := teamsHarness(t, nil)
	tests := []struct {
		name  string
		token string
		want  int
	}{
		{"valid", "tok-alice", http.StatusOK},
		{"Bearer prefix", "Bearer tok-alice", http.StatusOK},
		{"invalid", "forged", http.StatusUnauthorized},
		{"too long", "tok-" + strings.Repeat("a", maxUserToken), http.StatusUnauthorized},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// An invalid token is refused even on endpoints that work
			// anonymously: the user thinks they are signed in.
			code, out := hs.doAs(t, "GET", "/v1/cases", "", map[string]string{"X-User-Token": tt.token})
			if code != tt.want {
				t.Errorf("= %d %v, want %d", code, out, tt.want)
			}
			if tt.want == http.StatusUnauthorized && out["code"] != "unauthenticated" {
				t.Errorf("error code = %v", out["code"])
			}
		})
	}
	before := users.calls
	hs.do(t, "GET", "/v1/cases", "")
	if users.calls != before {
		t.Error("an anonymous request called the verifier")
	}
	// A workspace store outage is a 500, not a silent fall-back to anonymous.
	down, _, _ := teamsHarness(t, func(d *Deps) { d.Workspaces = failingWorkspaces{teams.NewMemory()} })
	if code, _ := down.doAs(t, "GET", "/v1/cases", "", user("alice")); code != http.StatusInternalServerError {
		t.Errorf("workspace store down = %d, want 500", code)
	}
}

func TestTeamsOff(t *testing.T) {
	users := &fakeUsers{}
	hs := newHarnessWith(t, "substantial_transformation", func(d *Deps) { d.Users, d.Workspaces = users, teams.NewMemory() })
	if code, _ := hs.doAs(t, "POST", "/v1/workspaces", `{"name":"Acme"}`, user("alice")); code != http.StatusNotFound {
		t.Errorf("workspaces served with teams off: %d", code)
	}
	// The token is ignored: the case is anonymous and visible as before.
	id := hs.newCase(t, user("alice"))
	if code, _ := hs.doAs(t, "GET", "/v1/cases/"+id, "", nil); code != http.StatusOK {
		t.Errorf("teams off, anonymous GET = %d", code)
	}
	if code, _ := hs.doAs(t, "GET", "/v1/cases", "", map[string]string{"X-User-Token": "forged"}); code != http.StatusOK {
		t.Errorf("teams off, a bad token was checked: %d", code)
	}
	_, out := hs.do(t, "GET", "/v1/cases/"+id, "")
	if ev := out["history"].([]any)[0].(map[string]any); ev["actor"] != nil {
		t.Errorf("teams off recorded actor %v", ev["actor"])
	}
	if users.calls != 0 {
		t.Errorf("verifier called %d times with teams off", users.calls)
	}
}

func TestWorkspaceCasesSurviveTheListCap(t *testing.T) {
	hs, _, _ := teamsHarness(t, func(d *Deps) { d.CasesListLimit = 2 })
	hs.doAs(t, "POST", "/v1/workspaces", `{"name":"Acme"}`, user("alice"))
	mine := hs.newCase(t, user("alice"))
	for i := 0; i < 3; i++ { // three newer cases from strangers fill the cap
		hs.newCase(t, nil)
	}
	if got := hs.listIDs(t, user("alice")); !got[mine] {
		t.Errorf("alice's workspace case fell out of a capped list: %v", got)
	}
}
