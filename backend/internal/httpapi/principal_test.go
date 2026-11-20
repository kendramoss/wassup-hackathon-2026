package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/apikey"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/store"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/teams"
)

// principalReq builds a request carrying the given headers and, standing in
// for the verifying middleware, the verified key and user in its context.
func principalReq(h map[string]string, key *apikey.Key, id *identity) *http.Request {
	r := httptest.NewRequest("GET", "/v1/cases", nil)
	for k, v := range h {
		r.Header.Set(k, v)
	}
	ctx := r.Context()
	if key != nil {
		ctx = context.WithValue(ctx, apiKeyCtx{}, *key)
	}
	if id != nil {
		ctx = context.WithValue(ctx, userKey{}, *id)
	}
	return r.WithContext(ctx)
}

func TestPrincipalOf(t *testing.T) {
	a := &api{d: Deps{AdminToken: testAdmin}}
	key := apikey.Key{ID: "key_id_1"}
	alice := identity{email: "alice@example.com", owner: userOwner("alice@example.com"), workspace: "ws_1"}
	vis := map[string]string{"X-Visitor-ID": aliceVis}

	tests := []struct {
		name     string
		req      *http.Request
		kind     principalKind
		owner    string
		verified bool
	}{
		{"nothing", principalReq(nil, nil, nil), kindNone, "", false},
		{"visitor", principalReq(vis, nil, nil), kindVisitor, visitorOwner(aliceVis), false},
		{"invalid visitor id is none", principalReq(map[string]string{"X-Visitor-ID": "short"}, nil, nil), kindNone, "", false},
		{"visitor id with a bad character is none", principalReq(map[string]string{"X-Visitor-ID": "alice-visitor-id-000!"}, nil, nil), kindNone, "", false},
		{"api key beats a visitor id", principalReq(vis, &key, nil), kindAPIKey, keyOwner(key), true},
		{"signed-in user keeps the browser's visitor owner", principalReq(vis, nil, &alice), kindUser, visitorOwner(aliceVis), true},
		{"signed-in user with an API key is a user (the key's owner stays the credential owner)", principalReq(vis, &key, &alice), kindUser, keyOwner(key), true},
		{"admin", principalReq(map[string]string{"X-Admin-Token": testAdmin}, nil, nil), kindAdmin, "", true},
		{"admin with a visitor id", principalReq(with(vis, map[string]string{"X-Admin-Token": testAdmin}), nil, nil), kindAdmin, visitorOwner(aliceVis), true},
		{"wrong admin token is not admin", principalReq(map[string]string{"X-Admin-Token": "nope"}, nil, nil), kindNone, "", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := a.principalOf(tc.req)
			if p.Kind != tc.kind || p.Owner != tc.owner || p.Verified() != tc.verified {
				t.Errorf("principal = {kind %q owner %q verified %v}, want {%q %q %v}", p.Kind, p.Owner, p.Verified(), tc.kind, tc.owner, tc.verified)
			}
			if p.Owner != ownerOf(tc.req) {
				t.Errorf("principal owner %q differs from ownerOf %q", p.Owner, ownerOf(tc.req))
			}
		})
	}

	// Role comes from the verified identity only: an API key, an admin token, a visitor or
	// forged headers never carry one.
	withRole := identity{email: "vera@example.com", owner: userOwner("vera@example.com"), workspace: "ws_1", role: teams.RoleViewer}
	if got := a.principalOf(principalReq(vis, nil, &withRole)).Role; got != teams.RoleViewer {
		t.Errorf("user role = %q", got)
	}
	for name, r := range map[string]*http.Request{
		"api key":         principalReq(map[string]string{"X-Role": "admin"}, &key, nil),
		"visitor":         principalReq(with(vis, map[string]string{"X-Role": "admin", "X-Workspace": "ws_1"}), nil, nil),
		"admin token":     principalReq(map[string]string{"X-Admin-Token": testAdmin, "X-Role": "editor"}, nil, nil),
		"user with a key": principalReq(map[string]string{"X-Role": "admin"}, &key, &identity{email: "x@example.com", owner: "u_x"}),
	} {
		if got := a.principalOf(r).Role; got != "" {
			t.Errorf("%s has role %q", name, got)
		}
	}

	p := a.principalOf(principalReq(vis, nil, &alice))
	if p.Email != alice.email || p.UserOwner != alice.owner || p.Workspace != "ws_1" {
		t.Errorf("user fields = %+v", p)
	}
	if owner, ws := p.newCaseScope(); owner != alice.owner || ws != "ws_1" {
		t.Errorf("a signed-in user's new case is filed under %q/%q", owner, ws)
	}
	if owner, ws := a.principalOf(principalReq(vis, nil, nil)).newCaseScope(); owner != visitorOwner(aliceVis) || ws != "" {
		t.Errorf("a visitor's new case is filed under %q/%q", owner, ws)
	}
	if got := p.listOwners(); len(got) != 2 || got[0] != alice.owner || got[1] != visitorOwner(aliceVis) {
		t.Errorf("a user's listing owners = %v", got)
	}
}

// Headers a client might forge to look like a tenant, a role or an owner are
// never read: only the verified context and the visitor/admin headers count.
func TestPrincipalIgnoresForgedHeaders(t *testing.T) {
	a := &api{d: Deps{AdminToken: testAdmin}}
	forged := map[string]string{
		"X-Tenant": "ws_1", "X-Workspace": "ws_1", "X-Workspace-ID": "ws_1", "X-Role": "admin", "X-User-Role": "admin",
		"X-Owner": "u_deadbeef", "X-User-Email": "alice@example.com", "X-Forwarded-User": "alice@example.com",
		"X-Goog-Authenticated-User-Email": "accounts.google.com:alice@example.com", "X-Is-Admin": "true",
	}
	p := a.principalOf(principalReq(forged, nil, nil))
	if p.Kind != kindNone || p.Owner != "" || p.Email != "" || p.Workspace != "" || p.Admin || p.Verified() {
		t.Errorf("forged headers produced %+v", p)
	}
	// A visitor ID that spells another owner's stored hash is hashed again,
	// so it cannot become that owner.
	victim := visitorOwner(aliceVis)
	if got := a.principalOf(principalReq(map[string]string{"X-Visitor-ID": victim}, nil, nil)); got.Owner == victim {
		t.Error("a stored owner hash used as X-Visitor-ID became that owner")
	}
}

// canSee is the single visibility rule; the cases here fix its current
// behavior for each principal, with ownership on and off.
func TestPrincipalCanSee(t *testing.T) {
	a := &api{d: Deps{AdminToken: testAdmin}}
	key := apikey.Key{ID: "key_id_1"}
	user := identity{email: "carol@example.com", owner: userOwner("carol@example.com"), workspace: "ws_1"}
	vis := map[string]string{"X-Visitor-ID": aliceVis}
	cases := map[string]store.Case{
		"mine":      {Owner: visitorOwner(aliceVis)},
		"other":     {Owner: visitorOwner(bobVis)},
		"unowned":   {},
		"demo":      {Pinned: true, Owner: visitorOwner(bobVis)},
		"mykey":     {Owner: keyOwner(key)},
		"otherkey":  {Owner: keyOwner(apikey.Key{ID: "other"})},
		"carol":     {Owner: user.owner, Workspace: "ws_1"},
		"workspace": {Owner: userOwner("dave@example.com"), Workspace: "ws_1"},
		"foreignws": {Owner: userOwner("eve@example.com"), Workspace: "ws_9"},
	}
	type row struct {
		off, on map[string]bool
	}
	set := func(names ...string) map[string]bool {
		m := map[string]bool{}
		for _, n := range names {
			m[n] = true
		}
		return m
	}
	tests := []struct {
		name string
		req  *http.Request
		row  row
	}{
		{"stranger", principalReq(nil, nil, nil), row{set("mine", "other", "unowned", "demo"), set("demo")}},
		{"visitor", principalReq(vis, nil, nil), row{set("mine", "other", "unowned", "demo"), set("mine", "demo")}},
		{"api key", principalReq(vis, &key, nil), row{set("mykey", "demo"), set("mykey", "demo")}},
		{"user", principalReq(nil, nil, &user), row{set("carol", "workspace", "demo"), set("carol", "workspace", "demo")}},
		{"user from alice's browser", principalReq(vis, nil, &user), row{set("carol", "workspace", "mine", "demo"), set("carol", "workspace", "mine", "demo")}},
		{"admin", principalReq(map[string]string{"X-Admin-Token": testAdmin}, nil, nil), row{set("mine", "other", "unowned", "demo", "mykey", "otherkey", "carol", "workspace", "foreignws"), set("mine", "other", "unowned", "demo", "mykey", "otherkey", "carol", "workspace", "foreignws")}},
	}
	for _, tc := range tests {
		for _, own := range []bool{false, true} {
			want := tc.row.off
			if own {
				want = tc.row.on
			}
			a.d.CaseOwnership = own
			p := a.principalOf(tc.req)
			for name, c := range cases {
				if got := p.canSee(c, own); got != want[name] {
					t.Errorf("%s, ownership %v: canSee(%s) = %v, want %v", tc.name, own, name, got, want[name])
				}
				if got := a.visible(tc.req, c); got != want[name] {
					t.Errorf("%s, ownership %v: visible(%s) = %v, want %v", tc.name, own, name, got, want[name])
				}
			}
		}
	}
}

// legacyVisible is the visibility rule exactly as it stood before the
// principal was introduced (owner.go visible + teams.go teamVisible at the
// #273 base), kept as a frozen oracle: the refactor must not change who sees
// what for any existing case.
func legacyVisible(a *api, r *http.Request, c store.Case) bool {
	if c.Pinned || a.isAdmin(r) {
		return true
	}
	if id := identityOf(r.Context()); id.email != "" {
		switch {
		case c.Workspace != "" && c.Workspace == id.workspace:
			return true
		case c.Owner == id.owner:
			return true
		case teamCase(c):
			return false
		}
		o := ownerOf(r)
		return o != "" && o == c.Owner
	}
	if teamCase(c) {
		return false
	}
	if _, withKey := keyFrom(r.Context()); !a.d.CaseOwnership && !withKey && !strings.HasPrefix(c.Owner, keyOwnerPrefix) {
		return true
	}
	o := ownerOf(r)
	return o != "" && o == c.Owner
}

func TestPrincipalVisibilityMatchesTheLegacyRule(t *testing.T) {
	a := &api{d: Deps{AdminToken: testAdmin}}
	key, other := apikey.Key{ID: "key_id_1"}, apikey.Key{ID: "key_id_2"}
	carol := identity{email: "carol@example.com", owner: userOwner("carol@example.com"), workspace: "ws_1"}
	loner := identity{email: "erin@example.com", owner: userOwner("erin@example.com")}
	var reqs []*http.Request
	for _, h := range []map[string]string{nil, {"X-Visitor-ID": aliceVis}, {"X-Visitor-ID": "short"}, {"X-Admin-Token": testAdmin}, with(map[string]string{"X-Visitor-ID": bobVis}, admin)} {
		for _, k := range []*apikey.Key{nil, &key} {
			for _, id := range []*identity{nil, &carol, &loner} {
				reqs = append(reqs, principalReq(h, k, id))
			}
		}
	}
	var cases []store.Case
	for _, owner := range []string{"", visitorOwner(aliceVis), visitorOwner(bobVis), keyOwner(key), keyOwner(other), carol.owner, loner.owner, userOwner("dave@example.com")} {
		for _, ws := range []string{"", "ws_1", "ws_9"} {
			for _, pinned := range []bool{false, true} {
				cases = append(cases, store.Case{Owner: owner, Workspace: ws, Pinned: pinned})
			}
		}
	}
	n := 0
	for _, own := range []bool{false, true} {
		a.d.CaseOwnership = own
		for _, r := range reqs {
			for _, c := range cases {
				n++
				want := legacyVisible(a, r, c)
				// The one deliberate divergence (#283): a signed-in user's own case
				// that is stamped with a workspace they are NOT in (they were removed
				// or left) belongs to that workspace, so it is no longer theirs.
				if id := identityOf(r.Context()); id.email != "" && !a.isAdmin(r) && !c.Pinned && c.Owner == id.owner && c.Workspace != "" && c.Workspace != id.workspace {
					want = false
				}
				if got := a.visible(r, c); got != want {
					t.Fatalf("ownership %v, kind %q, case %+v: visible = %v, legacy rule = %v", own, a.principalOf(r).Kind, c, got, want)
				}
			}
		}
	}
	t.Logf("%d request/case/mode combinations agree with the legacy rule", n)
}
