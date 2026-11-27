package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/audit"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/retention"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/scim"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/sso"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/sso/ssotest"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/store"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/teams"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/webhooks"
)

// Audit trail (#284) through the real handler over memory stores. Expectations
// are written from the contract (reports/284-api-contract.md), not read from
// the code.

var errBoom = errors.New("audit store down")

// faultStore wraps a store and fails appends on request. It inspects the
// sealed event, so a test can fail only completions, only one action, etc.
type faultStore struct {
	audit.Store
	mu      sync.Mutex
	fail    func(audit.Event) bool
	onState func(tenant string)               // runs after each State read
	tamper  func([]audit.Event) []audit.Event // edits what Range returns, as a rewritten datastore would
}

func (f *faultStore) State(ctx context.Context, tenant string) (audit.Head, *audit.Checkpoint, error) {
	h, c, err := f.Store.State(ctx, tenant)
	f.mu.Lock()
	hook := f.onState
	f.mu.Unlock()
	if hook != nil {
		hook(tenant)
	}
	return h, c, err
}

func (f *faultStore) Range(ctx context.Context, tenant string, r audit.Range) ([]audit.Event, error) {
	evs, err := f.Store.Range(ctx, tenant, r)
	f.mu.Lock()
	t := f.tamper
	f.mu.Unlock()
	if t != nil {
		evs = t(append([]audit.Event(nil), evs...))
	}
	return evs, err
}

func (f *faultStore) set(fn func(audit.Event) bool) { f.mu.Lock(); f.fail = fn; f.mu.Unlock() }

func (f *faultStore) Append(ctx context.Context, tenant string, seal func(audit.Head) (audit.Event, error)) (audit.Event, error) {
	return f.Store.Append(ctx, tenant, func(h audit.Head) (audit.Event, error) {
		e, err := seal(h)
		f.mu.Lock()
		fn := f.fail
		f.mu.Unlock()
		if err == nil && fn != nil && fn(e) {
			return audit.Event{}, errBoom
		}
		return e, err
	})
}

type auditClock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *auditClock) now() time.Time { c.mu.Lock(); defer c.mu.Unlock(); return c.t }
func (c *auditClock) advance(d time.Duration) {
	c.mu.Lock()
	c.t = c.t.Add(d)
	c.mu.Unlock()
}

type auditEnv struct {
	*roleEnv
	svc   *audit.Service
	fault *faultStore
	clk   *auditClock
	sso   *sso.MemoryStore
	scim  *scim.Memory
	ret   *retention.Memory
	wh    *webhooks.Service
}

const auditTestKey = "audit-test-actor-key-0123456789abcdef"

func newAuditEnvWith(t *testing.T, ownership bool, adjust ...func(*Deps)) *auditEnv {
	t.Helper()
	ps, err := audit.NewPseudonymizer("1", []byte(auditTestKey))
	if err != nil {
		t.Fatal(err)
	}
	ae := &auditEnv{fault: &faultStore{Store: audit.NewMemory()}, clk: &auditClock{t: time.Date(2026, 11, 20, 10, 0, 0, 0, time.UTC)}, sso: sso.NewMemoryStore(), scim: scim.NewMemory()}
	ae.svc = audit.NewService(ae.fault, ps, audit.Options{Now: ae.clk.now})
	withSSO := func(d *Deps) {
		keys, err := sso.NewKeys([]byte(strings.Repeat("a", 32)))
		if err != nil {
			t.Fatal(err)
		}
		svc, err := sso.New(sso.Config{SiteURL: "https://app.example.com"}, keys, ae.sso, sso.NewMemoryOneTime(nil), d.Workspaces)
		if err != nil {
			t.Fatal(err)
		}
		d.SSO, d.SCIM = svc, ae.scim
	}
	ae.ret = retention.NewMemory()
	ae.wh = looseWebhooks()
	ae.roleEnv = newRoleEnv(t, ownership, append([]func(*Deps){func(d *Deps) { d.Audit = ae.svc; d.Retention = ae.ret; d.AuditExport = true; d.Webhooks = ae.wh }, withSSO}, adjust...)...)
	return ae
}

func newAuditEnv(t *testing.T) *auditEnv { return newAuditEnvWith(t, true) }

func (e *auditEnv) tenant() string { return "ws:" + e.wsID }

// events returns every event of tenant, oldest first.
func (e *auditEnv) events(tenant string) []audit.Event {
	e.t.Helper()
	var out []audit.Event
	cur := ""
	for {
		p, err := e.svc.List(context.Background(), tenant, audit.ListQuery{Limit: 200, Cursor: cur})
		if err != nil {
			e.t.Fatal(err)
		}
		out = append(out, p.Events...)
		if p.NextCursor == "" {
			break
		}
		cur = p.NextCursor
	}
	return out
}

func (e *auditEnv) actor(email string) string {
	return e.svc.Pseudonyms().Actor(audit.ActorUser, email)
}

// since returns the events of tenant after the first n.
func (e *auditEnv) since(tenant string, n int) []audit.Event {
	all := e.events(tenant)
	if n > len(all) {
		return nil
	}
	return all[n:]
}

// pair asserts evs is exactly one intent followed by its completion.
func pair(t *testing.T, what string, evs []audit.Event, action audit.Action, detail string) (audit.Event, audit.Event) {
	t.Helper()
	if len(evs) != 2 {
		t.Fatalf("%s: %d events %s, want an intent and a completion", what, len(evs), brief(evs))
	}
	in, done := evs[0], evs[1]
	if in.Phase != audit.PhaseIntent || in.Outcome != audit.Allowed || done.Phase != audit.PhaseCompletion || done.Outcome != audit.Allowed {
		t.Errorf("%s: phases/outcomes = %s", what, brief(evs))
	}
	if in.Action != action || done.Action != action || in.Detail != detail || done.Detail != detail {
		t.Errorf("%s: action/detail = %s, want %s/%q", what, brief(evs), action, detail)
	}
	if done.Ref != in.Seq || in.Ref != 0 || in.RequestID == "" || in.RequestID != done.RequestID || in.Route == "" {
		t.Errorf("%s: ref/request/route wrong: %+v / %+v", what, in, done)
	}
	return in, done
}

func brief(evs []audit.Event) string {
	var parts []string
	for _, e := range evs {
		parts = append(parts, fmt.Sprintf("%d:%s/%s/%s/%s", e.Seq, e.Action, e.Phase, e.Outcome, e.Reason))
	}
	return "[" + strings.Join(parts, " ") + "]"
}

func TestEveryRegisteredRouteHasAnAuditRow(t *testing.T) {
	reg := registeredRoutes(t)
	seen := map[string]bool{}
	for _, p := range reg {
		seen[p] = true
		rt, ok := auditTable[p]
		if !ok {
			t.Errorf("route %q is registered in server.go but not in auditTable: decide what the audit trail records for it", p)
			continue
		}
		switch rt.Class {
		case auditNone:
			if rt.Action != "" {
				t.Errorf("%s: an unaudited route names action %q", p, rt.Action)
			}
		default:
			if !rt.Action.Known() {
				t.Errorf("%s: unknown action %q", p, rt.Action)
			}
		}
		// A route that changes a case, a workspace or a key is two-phase.
		mutating := strings.HasPrefix(p, "POST ") || strings.HasPrefix(p, "PUT ") || strings.HasPrefix(p, "PATCH ") || strings.HasPrefix(p, "DELETE ")
		if mutating && routeTable[p] != noCaseData || mutating && strings.Contains(p, "/workspaces") || mutating && strings.Contains(p, "/admin/") {
			if rt.Class != auditTwoPhase && rt.Class != auditDenyOnly {
				t.Errorf("%s changes state but is audit class %d", p, rt.Class)
			}
		}
	}
	for p := range auditTable {
		if !seen[p] {
			t.Errorf("auditTable lists %q, which server.go does not register", p)
		}
	}
	// A case-scoped read is deny-only: allowed reads are not audited in v1.
	for p, c := range routeTable {
		if c == caseScoped && strings.HasPrefix(p, "GET ") && auditTable[p].Class != auditDenyOnly {
			t.Errorf("%s: case reads are deny-only", p)
		}
	}
}

func TestAuditCoverageEveryWriteRouteRecordsIntentAndCompletion(t *testing.T) {
	e := newAuditEnv(t)
	ten := e.tenant()
	do := func(method, path, body string, h map[string]string) int {
		code, _ := e.hs.doAs(t, method, path, body, h)
		return code
	}
	step := func(what string, wantStatus int, action audit.Action, detail string, run func() int) (audit.Event, audit.Event) {
		t.Helper()
		before := len(e.events(ten))
		if got := run(); got != wantStatus {
			t.Fatalf("%s = %d, want %d", what, got, wantStatus)
		}
		return pair(t, what, e.since(ten, before), action, detail)
	}
	dave := user("dave")
	dID := e.hs.newCase(t, dave)
	c := func(suffix string) string { return "/v1/cases/" + dID + suffix }

	in, _ := step("predict", 201, audit.CaseCreate, "", func() int {
		return do("POST", "/v1/cases/predict", heatGunBody(t), dave)
	})
	if in.Actor != e.actor("dave@example.com") || in.ActorKind != audit.ActorUser || in.Tenant != ten || !strings.HasPrefix(in.Object, "case:case_") {
		t.Errorf("predict intent = %+v", in)
	}
	step("batch", 200, audit.CaseCreate, "", func() int {
		return do("POST", "/v1/cases/batch", `{"items":[{"name":"a","facts":"The unit is assembled in Mexico from Chinese parts."}]}`, dave)
	})
	step("rename", 200, audit.CaseUpdate, "rename", func() int { return do("PATCH", c(""), `{"display_name":"renamed"}`, dave) })
	step("economics", 200, audit.CaseUpdate, "economics", func() int { return do("PUT", c("/economics"), `{"annual_import_value_usd":1000}`, dave) })
	step("answers", 200, audit.CaseUpdate, "answers", func() int {
		return do("POST", c("/answers"), `{"answers":[{"question":"Where is it made?","answer":"Assembled in Thailand from Taiwanese parts."}]}`, dave)
	})
	step("scenarios", 200, audit.CaseUpdate, "scenarios", func() int {
		return do("POST", c("/scenarios"), `{"scenarios":[{"label":"x","facts":"The unit is assembled in Mexico."}]}`, dave)
	})
	step("compare", 200, audit.CaseUpdate, "compare", func() int { return do("POST", c("/compare-targets"), `{"targets":["Thailand","Vietnam"]}`, dave) })
	step("ask", 200, audit.CaseUpdate, "ask", func() int { return do("POST", c("/ask"), `{"question":"Why China?"}`, dave) })
	step("watch", 200, audit.CaseUpdate, "watch", func() int { return do("POST", c("/watch"), `{}`, dave) })
	step("plan", 200, audit.CaseUpdate, "plan", func() int { return do("POST", c("/plan"), `{"target":"Thailand"}`, dave) })
	step("plan stream", 200, audit.CaseUpdate, "plan", func() int {
		return do("POST", c("/plan"), `{"target":"Thailand"}`, with(dave, map[string]string{"Accept": "text/event-stream"}))
	})
	step("bom version applied", 200, audit.CaseUpdate, "bom_version", func() int {
		code, _ := e.hs.uploadBOM(t, dID, motorInThailand(t), "1", dave)
		return code
	})
	_, done := step("export recording", 200, audit.CaseExport, "markdown", func() int { return do("POST", c("/export"), `{}`, dave) })
	if done.Object != "case:"+dID {
		t.Errorf("export object = %q", done.Object)
	}

	// A preview still hands the content out: one single event, no intent.
	before := len(e.events(ten))
	if code := do("POST", c("/export"), `{"preview":true}`, user("vera")); code != 200 {
		t.Fatalf("viewer preview = %d", code)
	}
	if evs := e.since(ten, before); len(evs) != 1 || evs[0].Phase != audit.PhaseSingle || evs[0].Action != audit.CaseExport || evs[0].Actor != e.actor("vera@example.com") {
		t.Errorf("preview events = %s", brief(evs))
	}
	if cs, _ := e.cs.Get(context.Background(), dID); cs.History[len(cs.History)-1].Kind != "exported" {
		t.Error("a preview must leave the case history as the recording export wrote it")
	}

	// Admin token actions.
	pinID := e.hs.newCase(t, dave)
	step("pin", 200, audit.CasePin, "pin", func() int { return do("PUT", "/v1/cases/"+pinID+"/pin", `{"pinned":true}`, admin) })
	// A pinned public demo: a visitor's read-only analysis is not audited.
	before = len(e.events(ten))
	do("POST", "/v1/cases/"+pinID+"/ask", `{"question":"Why China?"}`, user("zed"))
	do("POST", "/v1/cases/"+pinID+"/export", `{"preview":true}`, user("zed"))
	if got := len(e.events(ten)); got != before {
		t.Errorf("pinned demo analysis wrote %d events into the case's tenant", got-before)
	}
	pt := "platform"
	b4 := len(e.events(pt))
	if code := do("POST", "/v1/watch/run", `{}`, admin); code != 200 {
		t.Fatalf("watch run = %d", code)
	}
	wrIn, _ := pair(t, "watch run", lastN(e.events(pt), b4), audit.AdminWatchRun, "")
	if wrIn.ActorKind != audit.ActorAdmin || wrIn.Actor != "" {
		t.Errorf("admin actor = %s %q, want kind admin and no pseudonym", wrIn.ActorKind, wrIn.Actor)
	}

	// Delete (sensitive).
	step("delete", 204, audit.CaseDelete, "", func() int { return do("DELETE", "/v1/cases/"+dID, "", dave) })

	// Workspace lifecycle in a fresh workspace.
	nina := user("nina")
	_, ws := e.hs.doAs(t, "POST", "/v1/workspaces", `{"name":"Nina Secret Name Ltd"}`, nina)
	nid := ws["id"].(string)
	nt := "ws:" + nid
	if evs := e.events(nt); len(evs) != 2 || evs[0].Action != audit.WorkspaceCreate || evs[0].Phase != audit.PhaseIntent || evs[1].Phase != audit.PhaseCompletion {
		t.Errorf("workspace create events = %s", brief(evs))
	}
	nstep := func(what string, want int, action audit.Action, detail string, run func() int) (audit.Event, audit.Event) {
		t.Helper()
		before := len(e.events(nt))
		if got := run(); got != want {
			t.Fatalf("%s = %d, want %d", what, got, want)
		}
		return pair(t, what, e.since(nt, before), action, detail)
	}
	inv, _ := nstep("invite", 200, audit.WorkspaceInvite, "", func() int {
		return do("POST", "/v1/workspaces/"+nid+"/members", `{"email":"omar@example.com"}`, nina)
	})
	if inv.Target != e.actor("omar@example.com") {
		t.Errorf("invite target = %q", inv.Target)
	}
	nstep("join", 200, audit.WorkspaceJoin, "", func() int { return do("POST", "/v1/workspaces/"+nid+"/join", "", user("omar")) })
	rin, _ := nstep("role change", 200, audit.WorkspaceRoleChange, "viewer", func() int {
		return do("PUT", e.rolePath(nid, "omar"), `{"role":"viewer"}`, nina)
	})
	if rin.Target != e.actor("omar@example.com") || rin.Actor != e.actor("nina@example.com") {
		t.Errorf("role change actor/target = %q/%q", rin.Actor, rin.Target)
	}
	nstep("leave", 200, audit.WorkspaceLeave, "", func() int { return do("DELETE", e.memberPath(nid, "omar"), "", user("omar")) })
	do("POST", "/v1/workspaces/"+nid+"/members", `{"email":"pia@example.com"}`, nina)
	do("POST", "/v1/workspaces/"+nid+"/join", "", user("pia"))
	nstep("remove", 200, audit.WorkspaceMemberRemove, "", func() int { return do("DELETE", e.memberPath(nid, "pia"), "", nina) })

	// API keys (platform tenant).
	pstep := func(what string, want int, action audit.Action, run func() int) []audit.Event {
		t.Helper()
		before := len(e.events(pt))
		if got := run(); got != want {
			t.Fatalf("%s = %d, want %d", what, got, want)
		}
		return e.since(pt, before)
	}
	var keyID string
	evs := pstep("issue key", 201, audit.APIKeyIssue, func() int {
		code, out := e.hs.doAs(t, "POST", "/v1/admin/api-keys", `{"name":"Acme Broker Secret","daily_quota":5}`, admin)
		if code == 201 {
			keyID = out["api_key"].(map[string]any)["id"].(string)
		}
		return code
	})
	_, kdone := pair(t, "issue key", evs, audit.APIKeyIssue, "")
	if kdone.Object != "apikey:"+keyID {
		t.Errorf("issue completion object = %q, want apikey:%s", kdone.Object, keyID)
	}
	evs = pstep("list keys", 200, audit.APIKeyList, func() int { return do("GET", "/v1/admin/api-keys", "", admin) })
	if len(evs) != 1 || evs[0].Phase != audit.PhaseSingle || evs[0].Outcome != audit.Allowed {
		t.Errorf("list keys events = %s", brief(evs))
	}
	evs = pstep("disable key", 204, audit.APIKeyDisable, func() int { return do("DELETE", "/v1/admin/api-keys/"+keyID, "", admin) })
	pair(t, "disable key", evs, audit.APIKeyDisable, "")

	// Everything written verifies, and nothing in any tenant holds an identity,
	// a name, a credential or a fact.
	for _, tn := range []string{ten, nt, pt} {
		res, err := e.svc.Verify(context.Background(), tn, "")
		if err != nil || !res.OK || !res.Complete {
			t.Errorf("verify %s = %+v %v", tn, res, err)
		}
		blob, _ := json.Marshal(e.events(tn))
		for _, secret := range []string{"@", "example.com", "tok-", "dave", "nina", "omar", "Nina Secret", "Acme", "Broker", "op_", "Mexico", "Thailand", "renamed", auditTestKey} {
			if strings.Contains(string(blob), secret) {
				t.Errorf("tenant %s: the trail contains %q", tn, secret)
			}
		}
	}
}

func lastN(evs []audit.Event, from int) []audit.Event {
	if from > len(evs) {
		return nil
	}
	return evs[from:]
}

func TestAuditDenialsAreRecordedInTheRequestersTenant(t *testing.T) {
	e := newAuditEnv(t)
	ten := e.tenant()
	id := e.wsCase()
	before := len(e.events(ten))

	// A viewer is forbidden to rename: denied, in her workspace, naming the real case.
	if code, _ := e.hs.doAs(t, "PATCH", "/v1/cases/"+id, `{"display_name":"x"}`, user("vera")); code != 403 {
		t.Fatalf("viewer rename = %d", code)
	}
	evs := e.since(ten, before)
	if len(evs) != 1 {
		t.Fatalf("events = %s", brief(evs))
	}
	d := evs[0]
	if d.Phase != audit.PhaseSingle || d.Outcome != audit.Denied || d.Reason != audit.ReasonForbidden || d.Action != audit.CaseUpdate ||
		d.Actor != e.actor("vera@example.com") || d.Object != "case:"+id || d.Route != "PATCH /v1/cases/{id}" {
		t.Errorf("denial = %+v", d)
	}

	// Another workspace's case is a hidden 404: the denial lands in the
	// REQUESTER'S tenant with an opaque object, and nothing in the owner's.
	zcase := e.hs.newCase(t, user("zed"))
	zt := "ws:" + e.zed
	zBefore, vBefore := len(e.events(zt)), len(e.events(ten))
	if code, _ := e.hs.doAs(t, "GET", "/v1/cases/"+zcase, "", user("vera")); code != 404 {
		t.Fatalf("cross-tenant get = %d", code)
	}
	if got := len(e.events(zt)); got != zBefore {
		t.Errorf("a cross-tenant probe wrote %d events into the victim's tenant", got-zBefore)
	}
	evs = e.since(ten, vBefore)
	if len(evs) != 1 || evs[0].Reason != audit.ReasonHidden || evs[0].Action != audit.CaseRead || evs[0].Outcome != audit.Denied {
		t.Fatalf("hidden denial = %s", brief(evs))
	}
	if strings.Contains(evs[0].Object, zcase) || !strings.HasPrefix(evs[0].Object, "case:h_") {
		t.Errorf("the hidden case's id is in the denial: %q", evs[0].Object)
	}

	// A missing case is a plain 404: not a denial, nothing recorded.
	n := len(e.events(ten))
	e.hs.doAs(t, "GET", "/v1/cases/case_000000000000", "", user("vera"))
	if len(e.events(ten)) != n {
		t.Error("a plain not-found was recorded as a denial")
	}
	// An allowed read is not recorded.
	e.hs.doAs(t, "GET", "/v1/cases/"+id, "", user("vera"))
	e.hs.doAs(t, "GET", "/v1/cases", "", user("vera"))
	if len(e.events(ten)) != n {
		t.Error("an allowed read was recorded")
	}

	// Workspace admin actions by an editor are refused BEFORE any intent: one
	// denial each, no intent/failure pair.
	for what, run := range map[string]func() int{
		"invite": func() int {
			c, _ := e.hs.doAs(t, "POST", "/v1/workspaces/"+e.wsID+"/members", `{"email":"x@example.com"}`, user("dave"))
			return c
		},
		"role change": func() int {
			c, _ := e.hs.doAs(t, "PUT", e.rolePath(e.wsID, "vera"), `{"role":"editor"}`, user("dave"))
			return c
		},
		"remove": func() int { c, _ := e.hs.doAs(t, "DELETE", e.memberPath(e.wsID, "vera"), "", user("dave")); return c },
	} {
		b := len(e.events(ten))
		if code := run(); code != 403 {
			t.Fatalf("editor %s = %d, want 403", what, code)
		}
		got := e.since(ten, b)
		if len(got) != 1 || got[0].Phase != audit.PhaseSingle || got[0].Outcome != audit.Denied || got[0].Reason != audit.ReasonForbidden {
			t.Errorf("editor %s: events = %s, want one denial and no intent", what, brief(got))
		}
	}
	n = len(e.events(ten))
	// A non-admin using an admin route: 403 denied, naming the attempted action.
	pb := 0
	e.hs.doAs(t, "POST", "/v1/admin/api-keys", `{"name":"x","daily_quota":1}`, user("dave"))
	_ = pb
	if got := e.since(ten, n); len(got) != 1 || got[0].Action != audit.APIKeyIssue || got[0].Reason != audit.ReasonForbidden || got[0].Actor != e.actor("dave@example.com") {
		t.Errorf("non-admin api-key denial = %s", brief(got))
	}
}

func TestAuditUnverifiedCallersCannotSpoofAnActorOrMintTenants(t *testing.T) {
	e := newAuditEnvWith(t, true, func(d *Deps) { d.StrictAuth = true })
	id := e.wsCase()
	pt := "platform"
	before := len(e.events(pt))
	spoof := map[string]string{"X-Visitor-ID": "alice-visitor-id-0001", "X-User-Email": "carol@example.com", "X-Tenant": e.tenant(), "X-Role": "admin",
		"X-Owner": "u_00000000000000000000000000000000"}
	if code, _ := e.hs.doAs(t, "PATCH", "/v1/cases/"+id, `{"display_name":"x"}`, spoof); code != 401 {
		t.Fatalf("strict visitor = %d", code)
	}
	evs := e.since(pt, before)
	if len(evs) != 1 {
		t.Fatalf("events = %s", brief(evs))
	}
	d := evs[0]
	if d.ActorKind != audit.ActorAnonymous || d.Actor != "" || d.Tenant != pt || d.Reason != audit.ReasonUnauthenticated || strings.Contains(d.Object, id) {
		t.Errorf("unverified denial = %+v", d)
	}
	tenants, _ := e.fault.Tenants(context.Background())
	for _, tn := range tenants {
		if strings.Contains(tn, "alice") || strings.Contains(tn, "00000000000000000000000000000000") {
			t.Errorf("a made-up visitor/owner id created tenant %q", tn)
		}
	}
	// An invalid user token and an invalid API key: auth.denied, anonymous, platform.
	before = len(e.events(pt))
	e.hs.doAs(t, "GET", "/v1/workspaces/mine", "", map[string]string{"X-User-Token": "forged-for-carol@example.com"})
	e.hs.doAs(t, "GET", "/v1/cases", "", map[string]string{"X-API-Key": "op_" + strings.Repeat("A", 43)})
	got := e.since(pt, before)
	if len(got) != 2 {
		t.Fatalf("auth denials = %s", brief(got))
	}
	for _, g := range got {
		blob, _ := json.Marshal(g)
		if g.Action != audit.AuthDenied || g.Reason != audit.ReasonInvalidCredential || g.ActorKind != audit.ActorAnonymous || g.Actor != "" ||
			strings.Contains(string(blob), "carol") || strings.Contains(string(blob), "forged") {
			t.Errorf("auth denial = %s", blob)
		}
	}
}

func TestAuditDenialFloodIsBounded(t *testing.T) {
	e := newAuditEnv(t)
	pt := "platform"
	before := len(e.events(pt))
	for i := 0; i < 300; i++ {
		e.hs.doAs(t, "GET", "/v1/cases", "", map[string]string{"X-User-Token": fmt.Sprintf("bad-%d", i)})
	}
	got := e.since(pt, before)
	if len(got) > 31 { // the platform bucket's 30 per minute, plus the one carrying the count
		t.Fatalf("%d auth denials were written for 300 bad tokens", len(got))
	}
	e.clk.advance(2 * time.Minute)
	e.hs.doAs(t, "GET", "/v1/cases", "", map[string]string{"X-User-Token": "bad-again"})
	last := lastN(e.events(pt), 0)
	if s := last[len(last)-1].Suppressed; s < 250 {
		t.Errorf("suppressed = %d, want the ~270 dropped denials counted", s)
	}
}

// sensitiveCases drives each fail-closed action. The test below fails if an
// action the policy calls sensitive has no row.
func (e *auditEnv) sensitiveCases(t *testing.T) map[audit.Action]struct {
	run    func() (int, map[string]any)
	intact func() bool // the state the request would have changed is untouched
} {
	dID := e.wsCase()
	rID := e.wsCase()
	_, ws := e.hs.doAs(t, "POST", "/v1/workspaces", `{"name":"Fail Closed Co"}`, user("fiona"))
	fid := ws["id"].(string)
	e.hs.addToWorkspace(t, fid, "fiona", "gus")
	e.hs.addToWorkspace(t, fid, "fiona", "kim")
	e.hs.addToWorkspace(t, fid, "fiona", "lou") // one member per action: the cases run in map order
	e.hs.doAs(t, "POST", "/v1/workspaces/"+fid+"/members", `{"email":"hana@example.com"}`, user("fiona"))
	var keyID string
	{
		_, out := e.hs.doAs(t, "POST", "/v1/admin/api-keys", `{"name":"k","daily_quota":5}`, admin)
		keyID = out["api_key"].(map[string]any)["id"].(string)
	}
	conns := map[string]string{}
	for _, name := range []string{"upd", "del"} {
		c := sso.Connection{ID: sso.NewConnectionID(), Workspace: fid, Protocol: sso.ProtocolOIDC, Domains: []string{name + ".example.org"},
			OIDCIssuer: "https://idp.example.org", OIDCClientID: "c", DefaultRole: "viewer", VerifyToken: sso.NewVerifyToken()}
		if err := e.sso.Put(context.Background(), c); err != nil {
			t.Fatal(err)
		}
		conns[name] = c.ID
	}
	// SCIM: a verified domain on fid, two provisioned members and a token
	// that every SCIM row puts back first (the token rows replace it).
	{
		c := sso.Connection{ID: sso.NewConnectionID(), Workspace: fid, Protocol: sso.ProtocolOIDC, Domains: []string{"scim.example.org"},
			OIDCIssuer: "https://idp.example.org", OIDCClientID: "c", DefaultRole: "viewer", VerifyToken: sso.NewVerifyToken(),
			Verified: map[string]time.Time{"scim.example.org": time.Now()}}
		if err := e.sso.Put(context.Background(), c); err != nil {
			t.Fatal(err)
		}
		for _, u := range []string{"u1@scim.example.org", "u2@scim.example.org"} {
			if _, err := e.ws.Provision(context.Background(), fid, u, teams.RoleViewer); err != nil {
				t.Fatal(err)
			}
		}
	}
	scimSecret, scimHash := scim.NewSecret()
	scimTok := scim.Token{Workspace: fid, Hash: scimHash, Domains: []string{"scim.example.org"}, DefaultRole: "viewer"}
	scimRun := func(method, path, body string) (int, map[string]any) {
		_ = e.scim.Put(context.Background(), scimTok)
		return e.hs.doAs(t, method, path, body, map[string]string{scimTokenHeader: scimSecret})
	}
	_, zedHash := scim.NewSecret()
	_ = e.scim.Put(context.Background(), scim.Token{Workspace: e.zed, Hash: zedHash, Domains: []string{"zed.example.org"}, DefaultRole: "viewer"})
	// Webhooks (#915): an endpoint of fid for each row to change.
	whStore := e.wh.Store()
	whEndpoint := func(name string) string {
		id := "wh_" + fmt.Sprintf("%016x", len(name)*7919+int(name[0]))
		_ = whStore.PutEndpoint(context.Background(), webhooks.Endpoint{ID: id, Workspace: fid, URL: "https://" + name + ".example.org/in", Events: []string{webhooks.EventBatchFinished},
			Enabled: true, Secret: "whsec_" + name, CreatedAt: time.Now()})
		return id
	}
	whUpd, whDel, whRot := whEndpoint("upd"), whEndpoint("del"), whEndpoint("rot")
	whGet := func(id string) (webhooks.Endpoint, error) { return whStore.GetEndpoint(context.Background(), id) }
	roleOf := func(email string) string {
		w, _ := e.ws.Get(context.Background(), fid)
		r, _ := w.RoleOf(email)
		return string(r)
	}
	return map[audit.Action]struct {
		run    func() (int, map[string]any)
		intact func() bool
	}{
		audit.CaseDelete: {
			func() (int, map[string]any) { return e.hs.doAs(t, "DELETE", "/v1/cases/"+dID, "", user("dave")) },
			func() bool { _, err := e.cs.Get(context.Background(), dID); return err == nil },
		},
		audit.ReviewCreate: {
			func() (int, map[string]any) {
				return e.hs.doAs(t, "POST", "/v1/cases/"+rID+"/reviews", `{}`, user("dave"))
			},
			func() bool { c, _ := e.cs.Get(context.Background(), rID); return len(c.BrokerReviews) == 0 },
		},
		audit.SupplierRequestCreate: {
			func() (int, map[string]any) {
				return e.hs.doAs(t, "POST", "/v1/cases/"+rID+"/supplier-declarations", `{"supplier_name":"Acme Motors","parts":[0]}`, user("dave"))
			},
			func() bool { c, _ := e.cs.Get(context.Background(), rID); return len(c.SupplierRequests) == 0 },
		},
		audit.WorkspaceInvite: {
			func() (int, map[string]any) {
				return e.hs.doAs(t, "POST", "/v1/workspaces/"+fid+"/members", `{"email":"ivy@example.com"}`, user("fiona"))
			},
			func() bool { w, _ := e.ws.Get(context.Background(), fid); return !w.HasInvite("ivy@example.com") },
		},
		audit.WorkspaceJoin: {
			func() (int, map[string]any) {
				return e.hs.doAs(t, "POST", "/v1/workspaces/"+fid+"/join", "", user("hana"))
			},
			func() bool { w, _ := e.ws.Get(context.Background(), fid); return !w.HasMember("hana@example.com") },
		},
		audit.WorkspaceRoleChange: {
			func() (int, map[string]any) {
				return e.hs.doAs(t, "PUT", e.rolePath(fid, "lou"), `{"role":"viewer"}`, user("fiona"))
			},
			func() bool { return roleOf("lou@example.com") == "editor" },
		},
		audit.WorkspaceMemberRemove: {
			func() (int, map[string]any) {
				return e.hs.doAs(t, "DELETE", e.memberPath(fid, "gus"), "", user("fiona"))
			},
			func() bool { w, _ := e.ws.Get(context.Background(), fid); return w.HasMember("gus@example.com") },
		},
		audit.WorkspaceRetention: {
			func() (int, map[string]any) {
				return e.hs.doAs(t, "PUT", "/v1/workspaces/"+fid+"/retention", `{"case_days":30}`, user("fiona"))
			},
			func() bool { _, ok, _ := e.ret.Policy(context.Background(), fid); return !ok },
		},
		audit.WorkspaceLeave: {
			func() (int, map[string]any) { return e.hs.doAs(t, "DELETE", e.memberPath(fid, "kim"), "", user("kim")) },
			func() bool { w, _ := e.ws.Get(context.Background(), fid); return w.HasMember("kim@example.com") },
		},
		audit.APIKeyIssue: {
			func() (int, map[string]any) {
				return e.hs.doAs(t, "POST", "/v1/admin/api-keys", `{"name":"blocked","daily_quota":5}`, admin)
			},
			func() bool {
				_, out := e.hs.doAs(t, "GET", "/v1/admin/api-keys", "", admin)
				for _, k := range out["api_keys"].([]any) {
					if k.(map[string]any)["name"] == "blocked" {
						return false
					}
				}
				return true
			},
		},
		audit.SSOConnectionCreate: {
			func() (int, map[string]any) {
				certs, _ := json.Marshal([]string{ssotest.NewSAMLIdP(t).CertPEM})
				return e.hs.doAs(t, "POST", "/v1/workspaces/"+fid+"/sso", `{"protocol":"saml","domains":["new.example.org"],"saml_sso_url":"https://idp.example.org/sso","saml_issuer":"https://idp.example.org","saml_certs_pem":`+string(certs)+`}`, user("fiona"))
			},
			func() bool {
				cs, _ := e.sso.List(context.Background(), fid)
				for _, c := range cs {
					if len(c.Domains) > 0 && c.Domains[0] == "new.example.org" {
						return false
					}
				}
				return true
			},
		},
		audit.SSOConnectionUpdate: {
			func() (int, map[string]any) {
				return e.hs.doAs(t, "PUT", "/v1/workspaces/"+fid+"/sso/"+conns["upd"], `{"protocol":"oidc","domains":["upd.example.org"],"oidc_issuer":"https://idp.example.org","oidc_client_id":"changed"}`, user("fiona"))
			},
			func() bool { c, _ := e.sso.Get(context.Background(), conns["upd"]); return c.OIDCClientID == "c" },
		},
		audit.SSOConnectionDelete: {
			func() (int, map[string]any) {
				return e.hs.doAs(t, "DELETE", "/v1/workspaces/"+fid+"/sso/"+conns["del"], "", user("fiona"))
			},
			func() bool { _, err := e.sso.Get(context.Background(), conns["del"]); return err == nil },
		},
		audit.SCIMTokenIssue: {
			func() (int, map[string]any) {
				return e.hs.doAs(t, "POST", "/v1/workspaces/"+fid+"/scim/token", `{"domains":["scim.example.org"]}`, user("fiona"))
			},
			func() bool {
				tk, err := e.scim.Get(context.Background(), fid)
				return err != nil || tk.Hash == scimHash
			},
		},
		audit.WebhookCreate: {
			func() (int, map[string]any) {
				return e.hs.doAs(t, "POST", "/v1/workspaces/"+fid+"/webhooks", `{"url":"https://new.example.org/in","events":["batch.finished"]}`, user("fiona"))
			},
			func() bool {
				eps, _ := whStore.ListEndpoints(context.Background(), fid)
				for _, ep := range eps {
					if ep.URL == "https://new.example.org/in" {
						return false
					}
				}
				return true
			},
		},
		audit.WebhookUpdate: {
			func() (int, map[string]any) {
				return e.hs.doAs(t, "PATCH", "/v1/workspaces/"+fid+"/webhooks/"+whUpd, `{"enabled":false}`, user("fiona"))
			},
			func() bool { ep, err := whGet(whUpd); return err == nil && ep.Enabled },
		},
		audit.WebhookDelete: {
			func() (int, map[string]any) {
				return e.hs.doAs(t, "DELETE", "/v1/workspaces/"+fid+"/webhooks/"+whDel, "", user("fiona"))
			},
			func() bool { _, err := whGet(whDel); return err == nil },
		},
		audit.WebhookRotate: {
			func() (int, map[string]any) {
				return e.hs.doAs(t, "POST", "/v1/workspaces/"+fid+"/webhooks/"+whRot+"/rotate", "", user("fiona"))
			},
			func() bool { ep, err := whGet(whRot); return err == nil && ep.Secret == "whsec_rot" },
		},
		audit.SCIMTokenRevoke: {
			func() (int, map[string]any) {
				return e.hs.doAs(t, "DELETE", "/v1/workspaces/"+e.zed+"/scim/token", "", user("zed"))
			},
			func() bool { _, err := e.scim.Get(context.Background(), e.zed); return err == nil },
		},
		audit.SCIMUserCreate: {
			func() (int, map[string]any) {
				return scimRun("POST", "/v1/scim/v2/Users", `{"userName":"new@scim.example.org"}`)
			},
			func() bool { return roleOf("new@scim.example.org") == "" },
		},
		audit.SCIMUserUpdate: {
			func() (int, map[string]any) {
				return scimRun("PATCH", "/v1/scim/v2/Users/"+scim.UserID(fid, "u1@scim.example.org"), `{"Operations":[{"op":"replace","path":"roles","value":"editor"}]}`)
			},
			func() bool { return roleOf("u1@scim.example.org") == "viewer" },
		},
		audit.SCIMUserDelete: {
			func() (int, map[string]any) {
				return scimRun("DELETE", "/v1/scim/v2/Users/"+scim.UserID(fid, "u2@scim.example.org"), "")
			},
			func() bool { return roleOf("u2@scim.example.org") == "viewer" },
		},
		audit.APIKeyDisable: {
			func() (int, map[string]any) { return e.hs.doAs(t, "DELETE", "/v1/admin/api-keys/"+keyID, "", admin) },
			func() bool {
				_, out := e.hs.doAs(t, "GET", "/v1/admin/api-keys", "", admin)
				for _, k := range out["api_keys"].([]any) {
					if k.(map[string]any)["id"] == keyID {
						return k.(map[string]any)["disabled"] == false
					}
				}
				return false
			},
		},
	}
}

func TestSensitiveWritesFailClosedWhenTheIntentCannotBeStored(t *testing.T) {
	e := newAuditEnv(t)
	cases := e.sensitiveCases(t)
	for _, a := range audit.Actions() {
		if _, ok := cases[a]; a.Sensitive() && !ok {
			t.Errorf("sensitive action %s has no fail-closed test row", a)
		}
		if _, ok := cases[a]; !a.Sensitive() && ok {
			t.Errorf("%s has a fail-closed row but the policy says it is not sensitive", a)
		}
	}
	for action, tc := range cases {
		t.Run(string(action), func(t *testing.T) {
			if !tc.intact() {
				t.Fatal("fixture: the state to protect is already changed")
			}
			e.fault.set(func(ev audit.Event) bool { return ev.Phase == audit.PhaseIntent && ev.Action == action })
			code, out := tc.run()
			e.fault.set(nil)
			if code != http.StatusServiceUnavailable || out["code"] != "audit_unavailable" {
				t.Errorf("= %d %v, want 503 audit_unavailable", code, out)
			}
			if !tc.intact() {
				t.Error("the state changed although the intent could not be stored")
			}
			// With the trail healthy again the same request goes through.
			if code, out := tc.run(); code >= 400 {
				t.Errorf("after recovery = %d %v", code, out)
			}
		})
	}
}

func TestSensitiveWriteOrderIsAuthorizeValidateIntentMutate(t *testing.T) {
	e := newAuditEnv(t)
	e.fault.set(func(audit.Event) bool { return true }) // nothing can be stored
	// A refusal comes before the intent: a viewer deleting still gets 403, not 503.
	id := e.wsCase()
	if code, out := e.hs.doAs(t, "DELETE", "/v1/cases/"+id, "", user("vera")); code != 403 {
		t.Errorf("viewer delete with audit down = %d %v, want 403 (authorization precedes the intent)", code, out)
	}
	// Invalid input comes before the intent too.
	if code, _ := e.hs.doAs(t, "PUT", e.rolePath(e.wsID, "dave"), `{"role":"emperor"}`, user("carol")); code != 400 {
		t.Errorf("bad role with audit down = %d, want 400", code)
	}
	// A hidden case stays a 404, not a 503.
	zcase := e.hs.newCase(t, user("zed"))
	if code, _ := e.hs.doAs(t, "GET", "/v1/cases/"+zcase, "", user("vera")); code != 404 {
		t.Errorf("hidden case with audit down = %d, want 404", code)
	}
	// And a non-sensitive create still works with the trail down (the intent is best effort).
	if code, _ := e.hs.doAs(t, "POST", "/v1/cases/predict", heatGunBody(t), user("dave")); code != 201 {
		t.Errorf("create with audit down = %d, want 201", code)
	}
}

func TestCompletionLostAfterTheMutationIsSurfacedNotHidden(t *testing.T) {
	e := newAuditEnv(t)
	ten := e.tenant()
	id := e.wsCase()
	e.fault.set(func(ev audit.Event) bool { return ev.Phase == audit.PhaseCompletion })
	req := httptest.NewRequest("DELETE", "/v1/cases/"+id, nil)
	req.Header.Set("X-User-Token", "tok-dave")
	rec := httptest.NewRecorder()
	e.hs.h.ServeHTTP(rec, req)
	e.fault.set(nil)
	// The delete happened and is reported as it happened: not rolled back, not failed.
	if rec.Code != http.StatusNoContent {
		t.Fatalf("delete = %d, want 204", rec.Code)
	}
	if _, err := e.cs.Get(context.Background(), id); !errors.Is(err, store.ErrNotFound) {
		t.Fatal("the case should be gone")
	}
	// ...and the response says the audit record is incomplete.
	if rec.Header().Get("X-Audit-Incomplete") != "completion-not-recorded" {
		t.Errorf("X-Audit-Incomplete = %q", rec.Header().Get("X-Audit-Incomplete"))
	}
	// The trail holds the intent and no completion; once old enough, verify lists it as unresolved.
	e.clk.advance(10 * time.Minute)
	res, err := e.svc.Verify(context.Background(), ten, "")
	if err != nil || res.UnresolvedIntents == nil || res.UnresolvedIntents.Count != 1 {
		t.Fatalf("verify = %+v %v", res.UnresolvedIntents, err)
	}
	evs := e.events(ten)
	var intent audit.Event
	for _, ev := range evs {
		if ev.Action == audit.CaseDelete {
			intent = ev
		}
	}
	if intent.Phase != audit.PhaseIntent || res.UnresolvedIntents.Seqs[0] != intent.Seq {
		t.Errorf("unresolved seqs %v, delete intent is %d", res.UnresolvedIntents.Seqs, intent.Seq)
	}
	// A healthy request carries no such header.
	id2 := e.wsCase()
	req = httptest.NewRequest("DELETE", "/v1/cases/"+id2, nil)
	req.Header.Set("X-User-Token", "tok-dave")
	rec = httptest.NewRecorder()
	e.hs.h.ServeHTTP(rec, req)
	if rec.Code != 204 || rec.Header().Get("X-Audit-Incomplete") != "" {
		t.Errorf("healthy delete = %d header %q", rec.Code, rec.Header().Get("X-Audit-Incomplete"))
	}
}

func TestNonSensitiveWriteProceedsWhenTheIntentCannotBeStored(t *testing.T) {
	e := newAuditEnv(t)
	ten := e.tenant()
	id := e.wsCase()
	e.fault.set(func(ev audit.Event) bool { return ev.Phase == audit.PhaseIntent })
	code, _ := e.hs.doAs(t, "PATCH", "/v1/cases/"+id, `{"display_name":"kept"}`, user("dave"))
	e.fault.set(nil)
	if code != 200 {
		t.Fatalf("rename with the intent unstorable = %d, want 200 (not a fail-closed action)", code)
	}
	evs := lastN(e.events(ten), len(e.events(ten))-1)
	if len(evs) != 1 || evs[0].Phase != audit.PhaseCompletion || evs[0].Ref != 0 || evs[0].Outcome != audit.Allowed {
		t.Errorf("completion without an intent = %s", brief(evs))
	}
}

func TestFailedOperationsAreRecordedAsFailures(t *testing.T) {
	e := newAuditEnv(t)
	ten := e.tenant()
	id := e.wsCase()
	// The last admin cannot leave: 409 after the intent, recorded as a failure with a closed reason.
	before := len(e.events(ten))
	if code, _ := e.hs.doAs(t, "DELETE", e.memberPath(e.wsID, "carol"), "", user("carol")); code != 409 {
		t.Fatalf("last admin leaving = %d", code)
	}
	in, done := e.since(ten, before)[0], e.since(ten, before)[1]
	if in.Action != audit.WorkspaceLeave || done.Outcome != audit.Failed || done.Reason != audit.ReasonConflict || done.Ref != in.Seq {
		t.Errorf("failure = %+v / %+v", in, done)
	}
	// A model failure after the intent: failed, upstream.
	e.hs.engine.err = errors.New("model down")
	before = len(e.events(ten))
	code, _ := e.hs.doAs(t, "POST", "/v1/cases/"+id+"/answers", `{"answers":[{"question":"q?","answer":"Assembled in Thailand."}]}`, user("dave"))
	e.hs.engine.err = nil
	evs := e.since(ten, before)
	if code < 500 || len(evs) != 2 || evs[1].Outcome != audit.Failed || evs[1].Reason == audit.ReasonNone {
		t.Errorf("answers with the model down = %d %s", code, brief(evs))
	}
}

func TestAuditReadAccess(t *testing.T) {
	e := newAuditEnv(t)
	e.hs.addToWorkspace(t, e.wsID, "carol", "erin2")
	get := func(path string, h map[string]string) (int, map[string]any) { return e.hs.doAs(t, "GET", path, "", h) }

	// The workspace admin reads their own tenant, implied.
	code, out := get("/v1/audit", user("carol"))
	if code != 200 || out["tenant"] != e.tenant() || out["retention_days"] != float64(90) {
		t.Fatalf("admin read = %d %v", code, out)
	}
	if evs, _ := out["events"].([]any); len(evs) == 0 {
		t.Error("no events")
	}
	// Everyone else is refused the same way.
	for name, h := range map[string]map[string]string{
		"editor": user("dave"), "viewer": user("vera"), "no workspace": user("nobody"),
		"api key": {"X-API-Key": e.keys["acme"]},
		"visitor": {"X-Visitor-ID": "alice-visitor-id-0001"},
		"nobody":  nil,
		"zed (another workspace's admin) is their own tenant only": nil,
	} {
		if strings.Contains(name, "zed") {
			continue
		}
		for _, p := range []string{"/v1/audit", "/v1/audit/verify"} {
			code, body := get(p, h)
			if code != 403 || body["code"] != "forbidden" {
				t.Errorf("%s on %s = %d %v, want 403 forbidden", name, p, code, body)
			}
		}
	}
	// The other workspace's admin sees only their own tenant, and cannot name another.
	code, out = get("/v1/audit", user("zed"))
	if code != 200 || out["tenant"] != "ws:"+e.zed {
		t.Errorf("zed read = %d %v", code, out)
	}
	if blob, _ := json.Marshal(out); strings.Contains(string(blob), e.wsID) {
		t.Error("zed's page mentions carol's workspace")
	}
	if code, _ := get("/v1/audit?tenant="+url.QueryEscape(e.tenant()), user("zed")); code != 400 {
		t.Errorf("zed naming another tenant = %d, want 400", code)
	}
	if code, _ := get("/v1/audit?tenant="+url.QueryEscape("ws:"+e.zed), user("carol")); code != 400 {
		t.Errorf("a tenant admin passing tenant= is %d, want 400", code)
	}
	// The admin token must name a tenant.
	if code, _ := get("/v1/audit", admin); code != 400 {
		t.Errorf("admin without tenant = %d, want 400", code)
	}
	if code, out := get("/v1/audit?tenant="+url.QueryEscape(e.tenant()), admin); code != 200 || out["tenant"] != e.tenant() {
		t.Errorf("admin read = %d %v", code, out)
	}
	if code, _ := get("/v1/audit?tenant=alice@example.com", admin); code != 400 {
		t.Errorf("admin with a non-tenant = %d, want 400", code)
	}
	// A personal tenant is for the admin token only.
	if code, out := get("/v1/audit?tenant=own:u_"+strings.Repeat("a", 32), admin); code != 200 {
		t.Errorf("admin on a personal tenant = %d %v", code, out)
	}
	// Admin token + a viewer's user token: the admin token wins (existing precedence).
	if code, _ := get("/v1/audit?tenant="+url.QueryEscape(e.tenant()), with(admin, user("vera"))); code != 200 {
		t.Errorf("admin+viewer = %d", code)
	}
	// A demoted admin loses access on the next request.
	if code, out := e.hs.doAs(t, "PUT", e.rolePath(e.wsID, "dave"), `{"role":"admin"}`, user("carol")); code != 200 {
		t.Fatalf("promote dave = %d %v", code, out)
	}
	if code, _ := get("/v1/audit", user("dave")); code != 200 {
		t.Errorf("new admin dave = %d", code)
	}
	if code, out := e.hs.doAs(t, "PUT", e.rolePath(e.wsID, "carol"), `{"role":"viewer"}`, user("dave")); code != 200 {
		t.Fatalf("demote carol = %d %v", code, out)
	}
	if code, _ := get("/v1/audit", user("carol")); code != 403 {
		t.Errorf("demoted carol = %d, want 403", code)
	}
}

func TestAuditReadUnderStrictAuth(t *testing.T) {
	e := newAuditEnvWith(t, true, func(d *Deps) { d.StrictAuth = true })
	for _, h := range []map[string]string{nil, {"X-Visitor-ID": "alice-visitor-id-0001"}} {
		if code, body := e.hs.doAs(t, "GET", "/v1/audit", "", h); code != 401 || body["code"] != "unauthenticated" {
			t.Errorf("strict unverified %v = %d %v, want 401", h, code, body)
		}
	}
	if code, _ := e.hs.doAs(t, "GET", "/v1/audit", "", user("vera")); code != 403 {
		t.Errorf("strict viewer = %d, want 403", code)
	}
	if code, _ := e.hs.doAs(t, "GET", "/v1/audit", "", user("carol")); code != 200 {
		t.Errorf("strict admin = %d", code)
	}
}

func TestAuditListPaginationFiltersAndBounds(t *testing.T) {
	e := newAuditEnv(t)
	id := e.wsCase()
	for i := 0; i < 6; i++ {
		e.hs.doAs(t, "PATCH", "/v1/cases/"+id, fmt.Sprintf(`{"display_name":"n%d"}`, i), user("dave"))
	}
	page := func(q string) (int, map[string]any) {
		return e.hs.doAs(t, "GET", "/v1/audit"+q, "", user("carol"))
	}
	var seen []float64
	cursor := ""
	for i := 0; i < 20; i++ {
		q := "?limit=5&order=asc"
		if cursor != "" {
			q += "&cursor=" + url.QueryEscape(cursor)
		}
		code, out := page(q)
		if code != 200 {
			t.Fatalf("page = %d %v", code, out)
		}
		for _, ev := range out["events"].([]any) {
			seen = append(seen, ev.(map[string]any)["seq"].(float64))
		}
		next, _ := out["next_cursor"].(string)
		if next == "" {
			break
		}
		cursor = next
	}
	for i := 1; i < len(seen); i++ {
		if seen[i] != seen[i-1]+1 {
			t.Fatalf("ascending pages are not consecutive: %v", seen)
		}
	}
	// The audit read is recorded once, on the first page only.
	n := 0
	for _, ev := range e.events(e.tenant()) {
		if ev.Action == audit.AuditRead {
			n++
		}
	}
	if n != 1 {
		t.Errorf("audit.read events = %d, want 1 for a multi-page listing", n)
	}
	// Filters.
	code, out := page("?action=case.update&phase=completion&limit=200")
	if code != 200 {
		t.Fatal(code, out)
	}
	for _, ev := range out["events"].([]any) {
		m := ev.(map[string]any)
		if m["action"] != "case.update" || m["phase"] != "completion" {
			t.Errorf("filter leaked %v", m)
		}
	}
	if code, _ := page("?actor=" + e.actor("dave@example.com") + "&limit=200"); code != 200 {
		t.Error("actor filter by pseudonym refused")
	}
	// Bounds and bad input.
	for _, q := range []string{"?limit=201", "?limit=0", "?limit=x", "?order=sideways", "?action=case.explode", "?outcome=meh", "?phase=middle",
		"?actor=dave@example.com", "?object=Acme", "?from=yesterday", "?cursor=junk", "?bogus=1", "?actor_email=dave@example.com"} {
		if code, _ := page(q); code != 400 {
			t.Errorf("GET /v1/audit%s = %d, want 400", q, code)
		}
	}
	// A cursor is bound to the filters and to the order it was issued under.
	_, first := page("?limit=2&order=asc")
	cur := first["next_cursor"].(string)
	if code, _ := page("?limit=2&order=asc&action=case.update&cursor=" + url.QueryEscape(cur)); code != 400 {
		t.Errorf("cursor reused under another filter = %d", code)
	}
	if code, _ := page("?limit=2&order=desc&cursor=" + url.QueryEscape(cur)); code != 400 {
		t.Errorf("cursor reused with another order = %d", code)
	}
	// ...and to the tenant: zed cannot use carol's cursor.
	if code, _ := e.hs.doAs(t, "GET", "/v1/audit?limit=2&order=asc&cursor="+url.QueryEscape(cur), "", user("zed")); code != 400 {
		t.Errorf("another tenant's cursor = %d, want 400", code)
	}
}

func TestAuditVerifyRouteReportsTamper(t *testing.T) {
	e := newAuditEnv(t)
	e.wsCase()
	code, out := e.hs.doAs(t, "GET", "/v1/audit/verify", "", user("carol"))
	if code != 200 || out["ok"] != true || out["complete"] != true || out["unresolved_intents"] == nil {
		t.Fatalf("verify = %d %v", code, out)
	}
	// Edit a stored event behind the store's back.
	e.fault.mu.Lock()
	e.fault.tamper = func(evs []audit.Event) []audit.Event {
		if len(evs) > 1 {
			evs[1].Outcome, evs[1].Reason = audit.Failed, audit.ReasonInternal
		}
		return evs
	}
	e.fault.mu.Unlock()
	code, out = e.hs.doAs(t, "GET", "/v1/audit/verify", "", user("carol"))
	if code != 200 || out["ok"] != false || len(out["findings"].([]any)) == 0 {
		t.Errorf("verify after tamper = %d %v", code, out)
	}
}

func TestAuditOffChangesNothing(t *testing.T) {
	e := newRoleEnv(t, true) // Deps.Audit nil
	if code, _ := e.hs.doAs(t, "GET", "/v1/audit", "", user("carol")); code != 404 {
		t.Errorf("GET /v1/audit with audit off = %d, want 404 (route not registered)", code)
	}
	if code, _ := e.hs.doAs(t, "GET", "/v1/audit/verify", "", admin); code != 404 {
		t.Errorf("verify with audit off = %d, want 404", code)
	}
	id := e.wsCase()
	req := httptest.NewRequest("DELETE", "/v1/cases/"+id, nil)
	req.Header.Set("X-User-Token", "tok-dave")
	rec := httptest.NewRecorder()
	e.hs.h.ServeHTTP(rec, req)
	if rec.Code != 204 || rec.Header().Get("X-Audit-Incomplete") != "" {
		t.Errorf("delete with audit off = %d header %q", rec.Code, rec.Header().Get("X-Audit-Incomplete"))
	}
}

func TestJoinWithoutAnInvitationWritesNothingIntoThatWorkspace(t *testing.T) {
	e := newAuditEnv(t)
	ten := e.tenant()
	before := len(e.events(ten))
	if code, _ := e.hs.doAs(t, "POST", "/v1/workspaces/"+e.wsID+"/join", "", user("mallory")); code != 404 {
		t.Fatalf("join without invite = %d, want 404", code)
	}
	if got := len(e.events(ten)); got != before {
		t.Errorf("an uninvited join attempt wrote %d events into the workspace's trail", got-before)
	}
	// It is a denial in the requester's own place (platform: she has no workspace or verified tenant of her own... she has a personal one).
	own := "own:" + userOwner("mallory@example.com")
	evs := e.events(own)
	if len(evs) != 1 || evs[0].Reason != audit.ReasonHidden || evs[0].Action != audit.WorkspaceJoin || strings.Contains(evs[0].Object, e.wsID) {
		t.Errorf("denial in the requester's tenant = %s", brief(evs))
	}
}

func TestPlanStreamIsClosedExplicitly(t *testing.T) {
	e := newAuditEnv(t)
	ten := e.tenant()
	id := e.wsCase()
	before := len(e.events(ten))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/v1/cases/"+id+"/plan", strings.NewReader(`{"target":"Thailand"}`))
	req.Header.Set("X-User-Token", "tok-dave")
	req.Header.Set("Accept", "text/event-stream")
	e.hs.h.ServeHTTP(rec, req)
	if !strings.HasPrefix(rec.Header().Get("Content-Type"), "text/event-stream") || !strings.Contains(rec.Body.String(), "event: done") {
		t.Fatalf("stream = %q %q", rec.Header().Get("Content-Type"), rec.Body.String())
	}
	pair(t, "plan stream", e.since(ten, before), audit.CaseUpdate, "plan")
}

func TestInvalidRequestsAreRefusedBeforeAnyIntentEvenWithTheTrailDown(t *testing.T) {
	e := newAuditEnv(t)
	_, out := e.hs.doAs(t, "POST", "/v1/admin/api-keys", `{"name":"k","daily_quota":5}`, admin)
	known := out["api_key"].(map[string]any)["id"].(string)
	id := e.wsCase()
	before := len(e.events("platform"))
	tb := len(e.events(e.tenant()))
	for _, down := range []bool{false, true} {
		e.fault.set(nil)
		if down {
			e.fault.set(func(audit.Event) bool { return true })
		}
		for what, tc := range map[string]struct {
			method, path, body string
			h                  map[string]string
			want               int
		}{
			"issue empty name":     {"POST", "/v1/admin/api-keys", `{"name":"","daily_quota":0}`, admin, 400},
			"issue bad quota":      {"POST", "/v1/admin/api-keys", `{"name":"x","daily_quota":-1}`, admin, 400},
			"disable unknown key":  {"DELETE", "/v1/admin/api-keys/0123456789abcdef", "", admin, 404},
			"disable malformed id": {"DELETE", "/v1/admin/api-keys/not-a-key", "", admin, 404},
			"ask empty question":   {"POST", "/v1/cases/" + id + "/ask", `{"question":"   "}`, user("dave"), 400},
			"ask too long":         {"POST", "/v1/cases/" + id + "/ask", `{"question":"` + strings.Repeat("x", 2001) + `"}`, user("dave"), 400},
		} {
			if code, body := e.hs.doAs(t, tc.method, tc.path, tc.body, tc.h); code != tc.want {
				t.Errorf("%s (audit down=%v) = %d %v, want %d", what, down, code, body, tc.want)
			}
		}
	}
	e.fault.set(nil)
	if got := len(e.events("platform")) - before; got != 0 {
		t.Errorf("invalid API-key requests left %d events", got)
	}
	if got := len(e.events(e.tenant())) - tb; got != 0 {
		t.Errorf("invalid ask requests left %d events", got)
	}
	_ = known
	// A valid disable still records its pair.
	if code, _ := e.hs.doAs(t, "DELETE", "/v1/admin/api-keys/"+known, "", admin); code != 204 {
		t.Fatalf("valid disable = %d", code)
	}
	pair(t, "disable", lastN(e.events("platform"), before), audit.APIKeyDisable, "")
}

func TestVerifyRouteAnswers409WhenAPurgeKeepsMovingTheCheckpoint(t *testing.T) {
	e := newAuditEnv(t)
	for i := 0; i < 4; i++ {
		e.wsCase()
	}
	e.clk.advance(200 * 24 * time.Hour) // everything is past retention
	mem := e.fault.Store
	e.fault.mu.Lock()
	e.fault.onState = func(tenant string) { // a sweeper that removes one event after every read
		_, _ = mem.Purge(context.Background(), tenant, e.clk.now().Add(-90*24*time.Hour), e.clk.now(), 1)
	}
	e.fault.mu.Unlock()
	code, body := e.hs.doAs(t, "GET", "/v1/audit/verify", "", user("carol"))
	e.fault.mu.Lock()
	e.fault.onState = nil
	e.fault.mu.Unlock()
	if code != 409 || body["code"] != "conflict" {
		t.Errorf("verify during purges = %d %v, want 409 conflict (not 200 with findings)", code, body)
	}
	code, body = e.hs.doAs(t, "GET", "/v1/audit/verify", "", user("carol"))
	if code != 200 || body["ok"] != true || body["verified_through_seq"] == nil {
		t.Errorf("verify after = %d %v", code, body)
	}
}

// Self-service keys are audited like admin keys, but in the requester's
// tenant and under their pseudonymous user actor (#788).
func TestSelfServiceAPIKeysAuditedInRequesterTenant(t *testing.T) {
	e := newAuditEnvWith(t, true, withKeys)
	ten := e.tenant() // dave is a member of carol's workspace
	n := len(e.events(ten))
	code, out := e.hs.doAs(t, "POST", "/v1/api-keys", `{"name":"Agent"}`, user("dave"))
	if code != http.StatusCreated {
		t.Fatalf("issue = %d %v", code, out)
	}
	issue, _ := pair(t, "self-service issue", e.since(ten, n), audit.APIKeyIssue, "")
	if issue.Actor != e.actor("dave@example.com") {
		t.Errorf("issue actor = %q, want dave's pseudonym", issue.Actor)
	}
	id, _ := out["api_key"].(map[string]any)["id"].(string)
	if id == "" {
		t.Fatalf("no key id in %v", out)
	}

	n = len(e.events(ten))
	if code, _ := e.hs.doAs(t, "DELETE", "/v1/api-keys/"+id, "", user("dave")); code != http.StatusNoContent {
		t.Fatalf("revoke = %d", code)
	}
	revoke, _ := pair(t, "self-service revoke", e.since(ten, n), audit.APIKeyDisable, "")
	if revoke.Actor != e.actor("dave@example.com") {
		t.Errorf("revoke actor = %q, want dave's pseudonym", revoke.Actor)
	}
}

// A borrowed or admin key id is a plain 404 to the caller, but unlike an
// unknown id it is recorded as a hidden denial in the requester's tenant
// (#788, D887). The victim's tenant sees nothing.
func TestSelfServiceAPIKeyHiddenProbesAreAudited(t *testing.T) {
	e := newAuditEnvWith(t, true, withKeys)
	ten := e.tenant()
	code, out := e.hs.doAs(t, "POST", "/v1/api-keys", `{"name":"Bob's agent"}`, user("bob"))
	if code != http.StatusCreated {
		t.Fatalf("bob's issue = %d %v", code, out)
	}
	bobID, _ := out["api_key"].(map[string]any)["id"].(string)
	_, adminID := issueKey(t, e.hs, "Broker", 10)
	if bobID == "" || adminID == "" {
		t.Fatalf("no ids: bob %q, admin %q", bobID, adminID)
	}
	bobTen := "own:" + userOwner("bob@example.com")
	beforeBob, beforeDave := len(e.events(bobTen)), len(e.events(ten))

	for _, id := range []string{bobID, adminID} {
		if code, _ := e.hs.doAs(t, "DELETE", "/v1/api-keys/"+id, "", user("dave")); code != http.StatusNotFound {
			t.Errorf("probe of %s = %d, want 404", id, code)
		}
	}
	if got := len(e.events(bobTen)); got != beforeBob {
		t.Errorf("a probe wrote %d events into bob's tenant", got-beforeBob)
	}
	evs := e.since(ten, beforeDave)
	if len(evs) != 2 {
		t.Fatalf("hidden denials = %s", brief(evs))
	}
	for i, ev := range evs {
		probed := []string{bobID, adminID}[i]
		if ev.Phase != audit.PhaseSingle || ev.Outcome != audit.Denied || ev.Reason != audit.ReasonHidden || ev.Action != audit.APIKeyDisable {
			t.Errorf("probe denial = %+v", ev)
		}
		if strings.Contains(ev.Object, probed) || !strings.HasPrefix(ev.Object, "apikey:h_") {
			t.Errorf("the probed key id is in the denial: %q", ev.Object)
		}
	}

	n := len(e.events(ten))
	if code, _ := e.hs.doAs(t, "DELETE", "/v1/api-keys/0000000000000000", "", user("dave")); code != http.StatusNotFound {
		t.Errorf("unknown id = %d, want 404", code)
	}
	if got := len(e.events(ten)); got != n {
		t.Errorf("an unknown id wrote %d events", got-n)
	}
}
