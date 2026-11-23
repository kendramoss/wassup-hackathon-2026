package httpapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/store"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/teams"
)

// fakeUsers verifies tokens of the form "tok-<name>" as <name>@example.com.
type fakeUsers struct {
	mu    sync.Mutex // concurrent requests in the role tests
	calls int
}

func (f *fakeUsers) Verify(_ context.Context, tok string) (teams.User, error) {
	f.mu.Lock()
	f.calls++
	f.mu.Unlock()
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

// addToWorkspace has admin invite name to wsID and name accept.
func (hs harness) addToWorkspace(t *testing.T, wsID, admin, name string) {
	t.Helper()
	if code, out := hs.doAs(t, "POST", "/v1/workspaces/"+wsID+"/members", `{"email":"`+name+`@example.com"}`, user(admin)); code != http.StatusOK {
		t.Fatalf("%s invites %s = %d %v", admin, name, code, out)
	}
	if code, out := hs.doAs(t, "POST", "/v1/workspaces/"+wsID+"/join", "", user(name)); code != http.StatusOK {
		t.Fatalf("%s joins = %d %v", name, code, out)
	}
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
	members, join := "/v1/workspaces/"+wsID+"/members", "/v1/workspaces/"+wsID+"/join"
	// Adding a member is an invitation: amy is not a member until she accepts.
	code, out := hs.doAs(t, "POST", members, `{"email":" Amy@Example.com "}`, user("alice"))
	if code != http.StatusOK || len(out["members"].([]any)) != 1 || len(out["invited"].([]any)) != 1 || out["invited"].([]any)[0] != "amy@example.com" {
		t.Fatalf("invite amy = %d %v", code, out)
	}
	if code, out := hs.doAs(t, "POST", members, `{"email":"amy@example.com"}`, user("alice")); code != http.StatusOK || len(out["invited"].([]any)) != 1 {
		t.Errorf("inviting amy again = %d %v, want 200 and one invitation", code, out)
	}
	code, mine = hs.doAs(t, "GET", "/v1/workspaces/mine", "", user("amy"))
	invites, _ := mine["invites"].([]any)
	if code != http.StatusOK || mine["workspace"] != nil || len(invites) != 1 {
		t.Fatalf("amy's workspace before accepting = %d %v", code, mine)
	}
	if inv := invites[0].(map[string]any); inv["workspace_id"] != wsID || inv["name"] != "Acme Imports" || inv["invited_by"] != "alice@example.com" || inv["members"] != nil {
		t.Errorf("amy's invitation = %v", inv)
	}
	if code, _ := hs.doAs(t, "POST", join, "", user("zed")); code != http.StatusNotFound {
		t.Errorf("uninvited join = %d, want 404", code)
	}
	if code, _ := hs.doAs(t, "POST", join, "", nil); code != http.StatusUnauthorized {
		t.Errorf("anonymous join = %d, want 401", code)
	}
	if code, out := hs.doAs(t, "POST", join, "", user("amy")); code != http.StatusOK || len(out["members"].([]any)) != 2 || len(out["invited"].([]any)) != 0 || out["admin"] != false {
		t.Fatalf("amy joins = %d %v", code, out)
	}
	code, mine = hs.doAs(t, "GET", "/v1/workspaces/mine", "", user("amy"))
	if w, _ := mine["workspace"].(map[string]any); code != http.StatusOK || w["id"] != wsID || w["admin"] != false || len(mine["invites"].([]any)) != 0 {
		t.Errorf("amy's workspace = %d %v", code, mine)
	}
	if code, _ := hs.doAs(t, "POST", join, "", user("amy")); code != http.StatusConflict {
		t.Errorf("amy joins again = %d, want 409", code)
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
		{"join an unknown workspace", "zed", "/v1/workspaces/ws_nope/join", "", http.StatusNotFound},
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
	// bob has his own workspace. alice can invite him (the answer does not
	// reveal that he is in one) but he cannot join a second.
	hs.doAs(t, "POST", "/v1/workspaces", `{"name":"Bob Co"}`, user("bob"))
	if code, _ := hs.doAs(t, "POST", members, `{"email":"bob@example.com"}`, user("alice")); code != http.StatusOK {
		t.Errorf("inviting a member of another workspace = %d, want 200", code)
	}
	if code, _ := hs.doAs(t, "POST", join, "", user("bob")); code != http.StatusConflict {
		t.Errorf("bob joins a second workspace = %d, want 409", code)
	}
}

// TestInvitationNeedsConsent: an admin who invites someone gains nothing
// until that person accepts. A pre-claimed email can still create its own
// workspace, and its cases stay personal.
func TestInvitationNeedsConsent(t *testing.T) {
	hs, _, cases := teamsHarness(t, nil)
	_, ws := hs.doAs(t, "POST", "/v1/workspaces", `{"name":"Mallory Co"}`, user("mallory"))
	malloryWS := ws["id"].(string)
	if code, _ := hs.doAs(t, "POST", "/v1/workspaces/"+malloryWS+"/members", `{"email":"victim@example.com"}`, user("mallory")); code != http.StatusOK {
		t.Fatalf("invite = %d", code)
	}

	// The victim signs in for the first time and makes a case.
	before := hs.newCase(t, user("victim"))
	if c, _ := cases.Get(context.Background(), before); c.Workspace != "" {
		t.Errorf("invited user's case filed in workspace %q", c.Workspace)
	}
	if code, _ := hs.doAs(t, "GET", "/v1/cases/"+before, "", user("mallory")); code != http.StatusNotFound {
		t.Errorf("inviter opens the uninvited victim's case = %d, want 404", code)
	}
	if hs.listIDs(t, user("mallory"))[before] {
		t.Error("inviter lists the victim's case")
	}

	// The victim is free to start their own team instead; the stale
	// invitation then cannot be used.
	code, own := hs.doAs(t, "POST", "/v1/workspaces", `{"name":"Victim Inc"}`, user("victim"))
	if code != http.StatusCreated {
		t.Fatalf("victim creates a workspace = %d %v", code, own)
	}
	if code, _ := hs.doAs(t, "POST", "/v1/workspaces/"+malloryWS+"/join", "", user("victim")); code != http.StatusConflict {
		t.Errorf("victim joins a second workspace = %d, want 409", code)
	}
	after := hs.newCase(t, user("victim"))
	for _, id := range []string{before, after} {
		if code, _ := hs.doAs(t, "GET", "/v1/cases/"+id, "", user("mallory")); code != http.StatusNotFound {
			t.Errorf("inviter opens %s = %d, want 404", id, code)
		}
	}
	if c, _ := cases.Get(context.Background(), after); c.Workspace != own["id"] {
		t.Errorf("victim's case in workspace %q, want their own %v", c.Workspace, own["id"])
	}
}

// TestJoinSharesOnlyNewCases: once an invitee accepts, the cases they create
// from then on are shared; those from before stay personal.
func TestJoinSharesOnlyNewCases(t *testing.T) {
	hs, _, _ := teamsHarness(t, nil)
	_, ws := hs.doAs(t, "POST", "/v1/workspaces", `{"name":"Acme"}`, user("alice"))
	wsID := ws["id"].(string)
	hs.doAs(t, "POST", "/v1/workspaces/"+wsID+"/members", `{"email":"amy@example.com"}`, user("alice"))
	before := hs.newCase(t, user("amy"))
	if code, _ := hs.doAs(t, "POST", "/v1/workspaces/"+wsID+"/join", "", user("amy")); code != http.StatusOK {
		t.Fatalf("amy joins = %d", code)
	}
	after := hs.newCase(t, user("amy"))
	got := hs.listIDs(t, user("alice"))
	if got[before] || !got[after] {
		t.Errorf("alice lists amy's case from before joining = %v, after = %v; want false, true", got[before], got[after])
	}
	if mine := hs.listIDs(t, user("amy")); !mine[before] || !mine[after] {
		t.Errorf("amy lists her own cases = %v", mine)
	}
}

// A join reads the target workspace, not the capped list used by /mine. With
// more pending invitations than MaxInvites, the newest one must still work
// (#374); an unrelated invitation-free workspace stays hidden.
func TestJoinBeyondInvitationListCap(t *testing.T) {
	hs, _, _ := teamsHarness(t, nil)
	var newest string
	for i := 0; i < teams.MaxInvites+1; i++ {
		admin := fmt.Sprintf("admin%02d", i)
		code, ws := hs.doAs(t, "POST", "/v1/workspaces", `{"name":"Team `+admin+`"}`, user(admin))
		if code != http.StatusCreated {
			t.Fatalf("create by %s = %d %v", admin, code, ws)
		}
		newest = ws["id"].(string)
		code, out := hs.doAs(t, "POST", "/v1/workspaces/"+newest+"/members", `{"email":"victim@example.com"}`, user(admin))
		if code != http.StatusOK {
			t.Fatalf("%s invites victim = %d %v", admin, code, out)
		}
	}
	// The UI list is still capped; the API's join is not.
	code, mine := hs.doAs(t, "GET", "/v1/workspaces/mine", "", user("victim"))
	if code != http.StatusOK || len(mine["invites"].([]any)) != teams.MaxInvites {
		t.Fatalf("mine = %d %v, want %d capped invites", code, mine, teams.MaxInvites)
	}
	code, joined := hs.doAs(t, "POST", "/v1/workspaces/"+newest+"/join", "", user("victim"))
	if code != http.StatusOK || joined["id"] != newest {
		t.Fatalf("join the %dth invitation = %d %v", teams.MaxInvites+1, code, joined)
	}
	_, other := hs.doAs(t, "POST", "/v1/workspaces", `{"name":"No invite here"}`, user("other"))
	otherID := other["id"].(string)
	if code, out := hs.doAs(t, "POST", "/v1/workspaces/"+otherID+"/join", "", user("stranger")); code != http.StatusNotFound {
		t.Errorf("join without an invitation = %d %v, want 404", code, out)
	}
}

func TestTeamsVisibility(t *testing.T) {
	for _, ownership := range []bool{false, true} {
		t.Run(fmt.Sprintf("case_ownership=%v", ownership), func(t *testing.T) {
			hs, _, _ := teamsHarness(t, func(d *Deps) { d.CaseOwnership = ownership })
			_, ws := hs.doAs(t, "POST", "/v1/workspaces", `{"name":"Acme"}`, user("alice"))
			hs.addToWorkspace(t, ws["id"].(string), "alice", "amy")
			hs.doAs(t, "POST", "/v1/workspaces/"+ws["id"].(string)+"/members", `{"email":"ivy@example.com"}`, user("alice"))

			browser := map[string]string{"X-Visitor-ID": "alice-browser-id-00001"}
			other := map[string]string{"X-Visitor-ID": "someone-else-id-000002"}
			alice := hs.newCase(t, user("alice")) // in Acme
			amy := hs.newCase(t, user("amy"))     // in Acme
			bob := hs.newCase(t, user("bob"))     // signed in, no workspace
			ivy := hs.newCase(t, user("ivy"))     // invited to Acme, not joined
			anonSame := hs.newCase(t, browser)    // anonymous, alice's browser
			anonOther := hs.newCase(t, other)     // anonymous, another browser
			anonNone := hs.newCase(t, nil)        // anonymous, no visitor ID
			demo := hs.newCase(t, admin)          // pinned below
			hs.doAs(t, "PUT", "/v1/cases/"+demo+"/pin", `{"pinned":true}`, admin)

			all := []string{alice, amy, bob, ivy, anonSame, anonOther, anonNone, demo}
			names := map[string]string{alice: "alice's", amy: "amy's", bob: "bob's", ivy: "ivy's", anonSame: "anon (alice's browser)",
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
				{"ivy (invited to Acme, not joined)", user("ivy"), []string{ivy, demo}},
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
	hs.addToWorkspace(t, wsID, "alice", "amy")
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

// Issue #712: the invitation names the admin who sent it, not the workspace
// creator, also after the creator is gone.
func TestInvitationNamesTheActualInviter(t *testing.T) {
	hs, _, _ := teamsHarness(t, nil)
	code, ws := hs.doAs(t, "POST", "/v1/workspaces", `{"name":"Acme"}`, user("alice"))
	if code != http.StatusCreated {
		t.Fatalf("create = %d %v", code, ws)
	}
	id := ws["id"].(string)
	hs.addToWorkspace(t, id, "alice", "bob")
	hs.addToWorkspace(t, id, "alice", "cara")
	members := "/v1/workspaces/" + id + "/members"
	role := func(who, target, r string) {
		t.Helper()
		if c, o := hs.doAs(t, "PUT", members+"/"+target+"%40example.com/role", `{"role":"`+r+`"}`, user(who)); c != http.StatusOK {
			t.Fatalf("%s sets %s to %s = %d %v", who, target, r, c, o)
		}
	}
	role("alice", "bob", "admin")
	role("alice", "cara", "admin")

	// Control: the creator herself invites.
	if c, o := hs.doAs(t, "POST", members, `{"email":"ann@example.com"}`, user("alice")); c != http.StatusOK {
		t.Fatalf("alice invites ann = %d %v", c, o)
	}
	// The creator leaves; a second admin invites.
	if c, o := hs.doAs(t, "DELETE", members+"/alice%40example.com", "", user("bob")); c != http.StatusOK {
		t.Fatalf("bob removes alice = %d %v", c, o)
	}
	if c, o := hs.doAs(t, "POST", members, `{"email":"victim@example.com"}`, user("bob")); c != http.StatusOK {
		t.Fatalf("bob invites victim = %d %v", c, o)
	}
	// A different admin invites the same person again: the first sender stays.
	if c, o := hs.doAs(t, "POST", members, `{"email":"victim@example.com"}`, user("cara")); c != http.StatusOK {
		t.Fatalf("cara re-invites victim = %d %v", c, o)
	}
	invitedBy := func(who string) (map[string]any, any) {
		t.Helper()
		c, mine := hs.doAs(t, "GET", "/v1/workspaces/mine", "", user(who))
		invs, _ := mine["invites"].([]any)
		if c != http.StatusOK || len(invs) != 1 {
			t.Fatalf("%s mine = %d %v", who, c, mine)
		}
		inv := invs[0].(map[string]any)
		return inv, inv["invited_by"]
	}
	inv, by := invitedBy("victim")
	if by != "bob@example.com" {
		t.Errorf("victim's invited_by = %v, want bob@example.com (not the creator, not the later re-inviter)", by)
	}
	if inv["created_at"] != ws["created_at"] {
		t.Errorf("created_at = %v, want the workspace's %v", inv["created_at"], ws["created_at"])
	}
	for k := range inv {
		if k != "workspace_id" && k != "name" && k != "invited_by" && k != "created_at" {
			t.Errorf("invitation has unexpected field %q", k)
		}
	}
	if _, by := invitedBy("ann"); by != "alice@example.com" {
		t.Errorf("ann's invited_by = %v, want alice@example.com", by)
	}

	// The members' view carries no inviter map.
	c, mine := hs.doAs(t, "GET", "/v1/workspaces/mine", "", user("bob"))
	if c != http.StatusOK {
		t.Fatalf("bob mine = %d", c)
	}
	for k := range mine["workspace"].(map[string]any) {
		if strings.Contains(k, "invited_by") || strings.Contains(k, "inviter") {
			t.Errorf("workspace view exposes %q", k)
		}
	}

	// Joining uses the invitation up: the same person invited again later is
	// attributed to the new sender.
	if c, o := hs.doAs(t, "POST", "/v1/workspaces/"+id+"/join", "", user("victim")); c != http.StatusOK {
		t.Fatalf("victim joins = %d %v", c, o)
	}
	if c, o := hs.doAs(t, "DELETE", members+"/victim%40example.com", "", user("bob")); c != http.StatusOK {
		t.Fatalf("bob removes victim = %d %v", c, o)
	}
	if c, o := hs.doAs(t, "POST", members, `{"email":"victim@example.com"}`, user("cara")); c != http.StatusOK {
		t.Fatalf("cara invites victim = %d %v", c, o)
	}
	if _, by := invitedBy("victim"); by != "cara@example.com" {
		t.Errorf("after rejoin cycle invited_by = %v, want cara@example.com", by)
	}
}

// An invitation stored before inviters were recorded has no known sender: the
// field is omitted, never guessed from the creator.
func TestLegacyInvitationOmitsInvitedBy(t *testing.T) {
	legacy := teams.NewMemory()
	if err := legacy.Create(context.Background(), teams.Workspace{ID: "ws_legacy", Name: "Old Co", Members: []string{"alice@example.com"}, Invited: []string{"zed@example.com"}, CreatedBy: "alice@example.com"}); err != nil {
		t.Fatal(err)
	}
	hs, _, _ := teamsHarness(t, func(d *Deps) { d.Workspaces = legacy })
	c, mine := hs.doAs(t, "GET", "/v1/workspaces/mine", "", user("zed"))
	invs, _ := mine["invites"].([]any)
	if c != http.StatusOK || len(invs) != 1 {
		t.Fatalf("mine = %d %v", c, mine)
	}
	inv := invs[0].(map[string]any)
	if v, present := inv["invited_by"]; present {
		t.Errorf("legacy invitation reports invited_by = %v, want the field omitted", v)
	}
	if inv["workspace_id"] != "ws_legacy" || inv["name"] != "Old Co" {
		t.Errorf("legacy invitation = %v", inv)
	}
	// The invitation still works.
	if c, o := hs.doAs(t, "POST", "/v1/workspaces/ws_legacy/join", "", user("zed")); c != http.StatusOK {
		t.Errorf("join legacy invitation = %d %v", c, o)
	}
}
