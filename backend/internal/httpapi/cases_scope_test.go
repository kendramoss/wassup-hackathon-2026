package httpapi

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/apikey"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/store"
)

// scopeCases wraps a Memory with every optional lister and counts how the
// listing reached the store, so a test can assert it never scanned or capped
// globally before filtering.
type scopeCases struct {
	*store.Memory
	mu      sync.Mutex
	lists   int
	recent  []int
	owners  map[string][]int
	wspaces []string
}

func newScopeCases() *scopeCases {
	return &scopeCases{Memory: store.NewMemory(), owners: map[string][]int{}}
}

func (c *scopeCases) List(ctx context.Context) ([]store.Case, error) {
	c.mu.Lock()
	c.lists++
	c.mu.Unlock()
	return c.Memory.List(ctx)
}

func (c *scopeCases) ListRecent(ctx context.Context, n int) ([]store.Case, error) {
	c.mu.Lock()
	c.recent = append(c.recent, n)
	c.mu.Unlock()
	return store.ListRecent(ctx, c.Memory, n) // the Memory fallback; counted here, not in List
}

func (c *scopeCases) ListWorkspace(ctx context.Context, ws string) ([]store.Case, error) {
	c.mu.Lock()
	c.wspaces = append(c.wspaces, ws)
	c.mu.Unlock()
	return store.ListWorkspace(ctx, c.Memory, ws)
}

func (c *scopeCases) ListOwner(ctx context.Context, owner string, n int) ([]store.Case, error) {
	c.mu.Lock()
	c.owners[owner] = append(c.owners[owner], n)
	c.mu.Unlock()
	return c.Memory.ListOwner(ctx, owner, n)
}

func (c *scopeCases) reset() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.lists, c.recent, c.owners, c.wspaces = 0, nil, map[string][]int{}, nil
}

// bareCases implements only store.Cases: a third-party or wrong-interface
// store, so every optional lister falls back.
type bareCases struct {
	store.Cases
	lists int
}

func (b *bareCases) List(ctx context.Context) ([]store.Case, error) {
	b.lists++
	return b.Cases.List(ctx)
}

// wrongOwnerLister has a ListOwner with the wrong signature: it does not
// satisfy store.OwnerLister and must be treated as a plain store.
type wrongOwnerLister struct{ store.Cases }

func (wrongOwnerLister) ListOwner(owner string) []store.Case { panic("must not be called") }

func visitorOwner(id string) string {
	r := httptest.NewRequest("GET", "/", nil)
	r.Header.Set("X-Visitor-ID", id)
	return ownerOf(r)
}

const (
	scopeLimit  = 2
	aliceVis    = "alice-visitor-id-0001"
	bobVis      = "bob-visitor-id-000002"
	foreignVis  = "foreign-visitor-id-0003"
	foreignMail = "eve@example.com"
)

type scopeEnv struct {
	hs        harness
	cs        *scopeCases
	wsID      string // alice's workspace (bob is a member)
	brokerKey map[string]string
	brokerOwn string
	otherOwn  string // another broker's owner hash
	ids       map[string]string
	clock     time.Time
}

// put stores a case; age is how long ago it was updated.
func (e *scopeEnv) put(t *testing.T, id string, c store.Case, age time.Duration) {
	t.Helper()
	c.ID = id
	c.CreatedAt, c.UpdatedAt = e.clock.Add(-age-time.Hour), e.clock.Add(-age)
	if err := e.cs.Memory.Put(context.Background(), c); err != nil {
		t.Fatal(err)
	}
}

// newScopeEnv builds a deployment with teams, API keys and (optionally) case
// ownership on, a list cap of 2, and a busy store: the principals' own cases
// are the OLDEST in it, under 200 newer foreign cases of every kind and
// newer orphaned health probes.
func newScopeEnv(t *testing.T, ownership bool, wrap ...func(store.Cases) store.Cases) *scopeEnv {
	t.Helper()
	e := &scopeEnv{cs: newScopeCases(), clock: time.Now().UTC().Truncate(time.Second), ids: map[string]string{}}
	hs, _, _ := teamsHarness(t, func(d *Deps) {
		d.Cases, d.CaseOwnership, d.CasesListLimit = store.Cases(e.cs), ownership, scopeLimit
		if len(wrap) > 0 {
			d.Cases = wrap[0](e.cs.Memory)
		}
		withKeys(d)
	})
	e.hs = hs
	_, ws := hs.doAs(t, "POST", "/v1/workspaces", `{"name":"Acme"}`, user("alice"))
	e.wsID = ws["id"].(string)
	hs.addToWorkspace(t, e.wsID, "alice", "bob")
	secret, id := issueKey(t, hs, "Acme Customs", 1000000)
	_, otherID := issueKey(t, hs, "Beta Brokers", 1000000)
	e.brokerKey = map[string]string{"X-API-Key": secret}
	e.brokerOwn, e.otherOwn = keyOwner(apikey.Key{ID: id}), keyOwner(apikey.Key{ID: otherID})

	age := func(h int) time.Duration { return time.Duration(h) * time.Hour }
	// Own cases, oldest of all (3 each so the cap of 2 matters).
	for i, n := range []string{"old", "mid", "new"} {
		e.put(t, "case_alice_vis_"+n, store.Case{Owner: visitorOwner(aliceVis)}, age(500-i))
		e.put(t, "case_alice_own_"+n, store.Case{Owner: userOwner("alice@example.com"), Workspace: e.wsID}, age(480-i))
		e.put(t, "case_bob_own_"+n, store.Case{Owner: userOwner("bob@example.com"), Workspace: e.wsID}, age(460-i))
		e.put(t, "case_broker_"+n, store.Case{Owner: e.brokerOwn}, age(440-i))
		e.put(t, "case_bobvis_"+n, store.Case{Owner: visitorOwner(bobVis)}, age(420-i))
		e.put(t, "case_legacy_"+n, store.Case{}, age(400-i)) // no owner (pre-ownership)
	}
	e.put(t, "case_demo", store.Case{Pinned: true, Owner: visitorOwner(foreignVis)}, age(900))
	e.put(t, "case_alice_pin", store.Case{Pinned: true, Owner: visitorOwner(aliceVis)}, age(880))
	// Busy foreign cases, all newer than any principal's own.
	for i := 0; i < 200; i++ {
		var c store.Case
		switch i % 4 {
		case 0:
			c = store.Case{Owner: visitorOwner(foreignVis)}
		case 1:
			c = store.Case{Owner: userOwner(foreignMail), Workspace: "ws_eve"}
		case 2:
			c = store.Case{Owner: e.otherOwn}
		case 3:
			c = store.Case{Owner: userOwner("mallory@example.com")}
		}
		e.put(t, fmt.Sprintf("case_foreign_%03d", i), c, age(100)-time.Duration(i)*time.Minute)
	}
	// Orphaned probes, the newest of everything: legacy, new, and a pinned one.
	e.put(t, HealthProbeID, store.Case{}, time.Minute)
	for i := 0; i < 5; i++ {
		e.put(t, newHealthProbeID(), store.Case{}, time.Duration(i+2)*time.Second)
	}
	e.put(t, newHealthProbeID(), store.Case{Pinned: true}, time.Second)
	e.cs.reset()
	return e
}

func (e *scopeEnv) listed(t *testing.T, h map[string]string) []string {
	t.Helper()
	code, out := e.hs.doAs(t, "GET", "/v1/cases", "", h)
	if code != http.StatusOK {
		t.Fatalf("list as %v = %d %v", h, code, out)
	}
	var ids []string
	for _, c := range out["cases"].([]any) {
		ids = append(ids, c.(map[string]any)["id"].(string))
	}
	return ids
}

// oracle is what the list must be: every case GET /v1/cases/{id} lets the
// principal read (the authorization authority), minus probes, then pinned
// plus the newest scopeLimit others. It scans the whole store, which the
// handler must not.
func (e *scopeEnv) oracle(t *testing.T, h map[string]string) []string {
	t.Helper()
	all, _ := e.cs.Memory.List(context.Background())
	var vis []store.Case
	for _, c := range all {
		if IsHealthProbeID(c.ID) {
			continue
		}
		if code, _ := e.hs.doAs(t, "GET", "/v1/cases/"+c.ID, "", h); code == http.StatusOK {
			vis = append(vis, c)
		}
	}
	vis = store.CapRecent(vis, scopeLimit)
	out := make([]string, len(vis))
	for i, c := range vis {
		out[i] = c.ID
	}
	return out
}

func (e *scopeEnv) principals() map[string]map[string]string {
	return map[string]map[string]string{
		"visitor":   {"X-Visitor-ID": aliceVis},
		"stranger":  nil,
		"user":      with(user("alice"), map[string]string{"X-Visitor-ID": aliceVis}),
		"member":    user("bob"),
		"broker":    e.brokerKey,
		"admin":     admin,
		"newvisit":  {"X-Visitor-ID": "brand-new-visitor-id-9999"},
		"userNoWs":  user("carol"),
		"otherUser": user("mallory"),
	}
}

func TestCaseListScopesBeforeTheRecentCap(t *testing.T) {
	e := newScopeEnv(t, true)
	for name, h := range e.principals() {
		got := e.listed(t, h)
		want := e.oracle(t, h)
		if fmt.Sprint(got) != fmt.Sprint(want) {
			t.Errorf("%s: list = %v\n want %v", name, got, want)
		}
		for _, id := range got {
			if IsHealthProbeID(id) {
				t.Errorf("%s: probe %s listed", name, id)
			}
		}
	}
}

func TestCaseListNamedExpectations(t *testing.T) {
	e := newScopeEnv(t, true)
	cases := map[string]struct {
		h    map[string]string
		want []string // pinned first, then newest
	}{
		"visitor":  {map[string]string{"X-Visitor-ID": aliceVis}, []string{"case_alice_pin", "case_demo", "case_alice_vis_new", "case_alice_vis_mid"}},
		"broker":   {e.brokerKey, []string{"case_alice_pin", "case_demo", "case_broker_new", "case_broker_mid"}},
		"user":     {user("alice"), []string{"case_alice_pin", "case_demo", "case_bob_own_new", "case_bob_own_mid"}},
		"member":   {user("bob"), []string{"case_alice_pin", "case_demo", "case_bob_own_new", "case_bob_own_mid"}},
		"stranger": {nil, []string{"case_alice_pin", "case_demo"}},
	}
	for name, c := range cases {
		got := e.listed(t, c.h)
		sort.Strings(got[:2])
		if fmt.Sprint(got) != fmt.Sprint(c.want) {
			t.Errorf("%s = %v, want %v", name, got, c.want)
		}
		for _, id := range got {
			if len(id) > 13 && id[:13] == "case_foreign_" {
				t.Errorf("%s was shown foreign case %s", name, id)
			}
		}
	}
}

func TestCaseListQueriesAreScopedAndBounded(t *testing.T) {
	e := newScopeEnv(t, true)
	type want struct {
		owners []string
		ws     bool
	}
	cases := map[string]struct {
		h map[string]string
		w want
	}{
		"visitor":  {map[string]string{"X-Visitor-ID": aliceVis}, want{[]string{visitorOwner(aliceVis)}, false}},
		"broker":   {e.brokerKey, want{[]string{e.brokerOwn}, false}},
		"user":     {with(user("alice"), map[string]string{"X-Visitor-ID": aliceVis}), want{[]string{userOwner("alice@example.com"), visitorOwner(aliceVis)}, true}},
		"stranger": {nil, want{nil, false}},
	}
	for name, c := range cases {
		e.cs.reset()
		e.listed(t, c.h)
		if e.cs.lists != 0 {
			t.Errorf("%s: full List called %d times on the OwnerLister path", name, e.cs.lists)
		}
		for _, n := range e.cs.recent {
			if n != 0 {
				t.Errorf("%s: ListRecent(n=%d) caps globally before filtering; only n=0 (pinned) is allowed", name, n)
			}
		}
		if len(e.cs.recent) != 1 {
			t.Errorf("%s: pinned read %d times, want exactly once", name, len(e.cs.recent))
		}
		if len(e.cs.owners) != len(c.w.owners) {
			t.Errorf("%s: owners queried %v, want %v", name, e.cs.owners, c.w.owners)
		}
		for _, o := range c.w.owners {
			if ns := e.cs.owners[o]; len(ns) != 1 || ns[0] != scopeLimit {
				t.Errorf("%s: owner %s queried with n=%v, want once with n=%d", name, o, ns, scopeLimit)
			}
		}
		if c.w.ws && (len(e.cs.wspaces) != 1 || e.cs.wspaces[0] != e.wsID) || !c.w.ws && len(e.cs.wspaces) != 0 {
			t.Errorf("%s: workspace reads %v", name, e.cs.wspaces)
		}
	}
}

func TestCaseListAdminKeepsGlobalListingAndSkipsProbesBeforeTheCap(t *testing.T) {
	e := newScopeEnv(t, true)
	got := e.listed(t, admin)
	// Admin sees everything: pinned (2 real + the hidden probe), then the 2
	// newest real cases. The six unpinned probes are newer than all of
	// them, but must not take cap slots.
	if len(got) != 4 {
		t.Fatalf("admin list = %v", got)
	}
	for _, id := range got {
		if IsHealthProbeID(id) {
			t.Errorf("probe %s listed", id)
		}
	}
	nonPinned := got[2:]
	if nonPinned[0] != "case_foreign_199" || nonPinned[1] != "case_foreign_198" {
		t.Errorf("admin newest = %v, want case_foreign_199, case_foreign_198", nonPinned)
	}
	if e.cs.lists != 0 || len(e.cs.recent) == 0 {
		t.Errorf("admin reads: lists=%d recent=%v (want windowed ListRecent, no full scan)", e.cs.lists, e.cs.recent)
	}
}

// Far more orphaned probes than any fixed number of widenings: the probes are
// the newest cases and must still never take a cap slot, nor a real case be
// lost, nor the cap be applied twice.
func TestCaseListAdminManyOrphanProbes(t *testing.T) {
	e := newScopeEnv(t, true)
	for i := 0; i < 500; i++ {
		e.put(t, newHealthProbeID(), store.Case{}, time.Duration(i)*time.Millisecond)
	}
	e.cs.reset()
	got := e.listed(t, admin)
	want := e.oracle(t, admin)
	if fmt.Sprint(got) != fmt.Sprint(want) || len(got) != 4 {
		t.Errorf("admin list = %v, want %v", got, want)
	}
	if got[2] != "case_foreign_199" || got[3] != "case_foreign_198" {
		t.Errorf("admin newest = %v", got)
	}
	if e.cs.lists != 0 || len(e.cs.recent) > 12 {
		t.Errorf("reads: lists=%d recent=%v (want logarithmic widening)", e.cs.lists, e.cs.recent)
	}
	// Only probes: nothing real is invented, loop terminates.
	empty := newScopeEnvEmptyProbes(t)
	if rows := empty.listed(t, admin); len(rows) != 0 {
		t.Errorf("probe-only store listed %v", rows)
	}
}

func newScopeEnvEmptyProbes(t *testing.T) *scopeEnv {
	t.Helper()
	e := &scopeEnv{cs: newScopeCases(), clock: time.Now().UTC()}
	hs, _, _ := teamsHarness(t, func(d *Deps) { d.Cases, d.CasesListLimit = e.cs, scopeLimit })
	e.hs = hs
	for i := 0; i < 30; i++ {
		e.put(t, newHealthProbeID(), store.Case{}, time.Duration(i)*time.Second)
	}
	return e
}

func TestCaseListOwnershipOffKeepsThePublicDemoIntent(t *testing.T) {
	e := newScopeEnv(t, false)
	// Ownership off: anonymous requests share the unowned/visitor cases, and
	// only team (user/workspace) and broker cases stay private. 200 newer
	// foreign team/broker cases and probes must not push them out.
	for name, h := range map[string]map[string]string{"stranger": nil, "visitor": {"X-Visitor-ID": aliceVis}} {
		got := e.listed(t, h)
		want := e.oracle(t, h)
		if fmt.Sprint(got) != fmt.Sprint(want) {
			t.Errorf("%s: list = %v, want %v", name, got, want)
		}
		anon := 0
		for _, id := range got {
			c, _ := e.cs.Memory.Get(context.Background(), id)
			if !c.Pinned {
				anon++
				if teamCase(c) || len(c.Owner) > 3 && c.Owner[:4] == keyOwnerPrefix {
					t.Errorf("%s: private case %s listed with ownership off", name, id)
				}
			}
		}
		if anon != scopeLimit {
			t.Errorf("%s: %d unpinned rows, want %d: %v", name, anon, scopeLimit, got)
		}
	}
	// A user and a broker still get their scoped list with ownership off.
	for name, h := range map[string]map[string]string{"user": user("alice"), "broker": e.brokerKey} {
		if got, want := e.listed(t, h), e.oracle(t, h); fmt.Sprint(got) != fmt.Sprint(want) {
			t.Errorf("%s: list = %v, want %v", name, got, want)
		}
	}
}

func TestCaseListFallbackStores(t *testing.T) {
	for name, wrap := range map[string]func(store.Cases) store.Cases{
		"bare":                 func(c store.Cases) store.Cases { return &bareCases{Cases: c} },
		"wrong ListOwner type": func(c store.Cases) store.Cases { return wrongOwnerLister{c} },
	} {
		e := newScopeEnv(t, true, wrap)
		for pname, h := range e.principals() {
			got, want := e.listed(t, h), e.oracle(t, h)
			if fmt.Sprint(got) != fmt.Sprint(want) {
				t.Errorf("%s/%s: list = %v, want %v", name, pname, got, want)
			}
		}
	}
}

func TestCaseListNeverWritesAndPinnedIsReadable(t *testing.T) {
	e := newScopeEnv(t, true)
	before, _ := e.cs.Memory.List(context.Background())
	for _, h := range e.principals() {
		e.listed(t, h)
	}
	after, _ := e.cs.Memory.List(context.Background())
	if fmt.Sprint(before) != fmt.Sprint(after) {
		t.Error("listing changed stored cases")
	}
	vis := map[string]string{"X-Visitor-ID": aliceVis}
	if code, _ := e.hs.doAs(t, "GET", "/v1/cases/case_demo", "", vis); code != http.StatusOK {
		t.Errorf("pinned demo GET = %d", code)
	}
	if code, _ := e.hs.doAs(t, "GET", "/v1/cases/case_missing", "", vis); code != http.StatusNotFound {
		t.Errorf("missing case GET = %d", code)
	}
	if code, _ := e.hs.doAs(t, "GET", "/v1/cases/case_bobvis_new", "", vis); code != http.StatusNotFound {
		t.Errorf("foreign case GET = %d", code)
	}
}

func TestCaseListDedupesAndKeepsRecentOrder(t *testing.T) {
	e := newScopeEnv(t, true)
	// The user's own workspace case is found by both the owner and the
	// workspace read; alice's own pinned case by pinned and owner reads.
	got := e.listed(t, user("alice"))
	seen := map[string]bool{}
	for _, id := range got {
		if seen[id] {
			t.Errorf("%s listed twice in %v", id, got)
		}
		seen[id] = true
	}
	rows, _ := e.cs.Memory.List(context.Background())
	pos := map[string]int{}
	for i, c := range rows {
		pos[c.ID] = i
	}
	if !sort.SliceIsSorted(got, func(i, j int) bool { return pos[got[i]] < pos[got[j]] }) {
		t.Errorf("not pinned-first then newest: %v", got)
	}
}
