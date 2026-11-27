package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/audit"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/metering"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/webhooks"
)

// Webhooks (#915, D936).

type whHit struct {
	header http.Header
	body   []byte
}

type whReceiver struct {
	srv  *httptest.Server
	mu   sync.Mutex
	hits []whHit
	code int
}

func newWhReceiver(t *testing.T) *whReceiver {
	t.Helper()
	r := &whReceiver{code: http.StatusOK}
	r.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		b, _ := io.ReadAll(req.Body)
		r.mu.Lock()
		r.hits = append(r.hits, whHit{req.Header.Clone(), b})
		code := r.code
		r.mu.Unlock()
		w.WriteHeader(code)
	}))
	t.Cleanup(r.srv.Close)
	return r
}

func (r *whReceiver) events() []webhooks.Envelope {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []webhooks.Envelope
	for _, h := range r.hits {
		var env webhooks.Envelope
		_ = json.Unmarshal(h.body, &env)
		out = append(out, env)
	}
	return out
}

func (r *whReceiver) ofType(typ string) []webhooks.Envelope {
	var out []webhooks.Envelope
	for _, e := range r.events() {
		if e.Type == typ {
			out = append(out, e)
		}
	}
	return out
}

func looseWebhooks() *webhooks.Service {
	return webhooks.New(webhooks.NewMemory(), webhooks.Options{AllowInsecure: true, Client: &http.Client{Timeout: 5 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}})
}

func whEnv(t *testing.T, svc *webhooks.Service, adjust ...func(*Deps)) *roleEnv {
	t.Helper()
	return newRoleEnv(t, true, append([]func(*Deps){func(d *Deps) { d.Webhooks = svc }}, adjust...)...)
}

func (e *roleEnv) whBase() string { return "/v1/workspaces/" + e.wsID + "/webhooks" }

func (e *roleEnv) addWebhook(url string, events ...string) map[string]any {
	e.t.Helper()
	evs, _ := json.Marshal(events)
	out := e.call("POST", e.whBase(), `{"url":"`+url+`","events":`+string(evs)+`}`, user("carol"), http.StatusCreated)
	return out["endpoint"].(map[string]any)
}

func TestWebhookRoutesAreOffByDefault(t *testing.T) {
	e := newRoleEnv(t, true)
	for _, rt := range []struct{ m, p string }{{"GET", e.whBase()}, {"POST", e.whBase()}, {"DELETE", e.whBase() + "/wh_0123456789abcdef"}, {"POST", "/v1/webhooks/sweep"}} {
		if code, _ := e.hs.doAs(t, rt.m, rt.p, `{}`, user("carol")); code != http.StatusNotFound && code != http.StatusMethodNotAllowed {
			t.Errorf("%s %s with WEBHOOKS off = %d, want 404", rt.m, rt.p, code)
		}
	}
}

func TestWebhookEndpointLifecycleAndSecrets(t *testing.T) {
	rcv := newWhReceiver(t)
	e := whEnv(t, looseWebhooks())

	// Only the workspace admin may register; everyone else is refused the
	// same way as for the other admin routes.
	body := `{"url":"` + rcv.srv.URL + `","events":["batch.finished","budget.alert"],"description":"ERP bridge"}`
	e.call("POST", e.whBase(), body, nil, http.StatusUnauthorized)
	e.call("POST", e.whBase(), body, user("dave"), http.StatusForbidden)
	e.call("POST", e.whBase(), body, user("vera"), http.StatusForbidden)
	e.call("POST", e.whBase(), body, user("zed"), http.StatusNotFound)
	e.call("GET", e.whBase(), "", user("dave"), http.StatusForbidden)

	ep := e.call("POST", e.whBase(), body, user("carol"), http.StatusCreated)["endpoint"].(map[string]any)
	secret, _ := ep["secret"].(string)
	id := ep["id"].(string)
	if !strings.HasPrefix(secret, webhooks.SecretPrefix) || !strings.HasPrefix(id, "wh_") || ep["enabled"] != true || ep["description"] != "ERP bridge" || ep["created_by"] != "carol@example.com" ||
		ep["secret_hint"] != secret[len(secret)-4:] {
		t.Fatalf("created = %v", ep)
	}
	if evs := ep["events"].([]any); len(evs) != 2 || evs[0] != "batch.finished" {
		t.Errorf("events = %v", evs)
	}

	// The secret is shown once: not in a listing, not on an update.
	list := e.call("GET", e.whBase(), "", user("carol"), http.StatusOK)
	raw, _ := json.Marshal(list)
	if strings.Contains(string(raw), secret) || strings.Contains(string(raw), `"secret":`) || len(list["endpoints"].([]any)) != 1 || len(list["available_events"].([]any)) != 5 {
		t.Fatalf("listing = %s", raw)
	}
	up := e.call("PATCH", e.whBase()+"/"+id, `{"events":["filing.status"],"description":"","enabled":false}`, user("carol"), http.StatusOK)["endpoint"].(map[string]any)
	if up["enabled"] != false || up["description"] != "" || up["secret"] != nil || len(up["events"].([]any)) != 1 || up["url"] != rcv.srv.URL {
		t.Errorf("updated = %v", up)
	}
	e.call("PATCH", e.whBase()+"/"+id, `{}`, user("carol"), http.StatusBadRequest)
	e.call("PATCH", e.whBase()+"/"+id, `{"events":["nope"]}`, user("carol"), http.StatusBadRequest)
	e.call("PATCH", e.whBase()+"/"+id, `{"enabled":true}`, user("dave"), http.StatusForbidden)

	// Rotation: a new secret, shown once; the old one signs beside it.
	rot := e.call("POST", e.whBase()+"/"+id+"/rotate", "", user("carol"), http.StatusOK)["endpoint"].(map[string]any)
	if rot["secret"] == secret || !strings.HasPrefix(rot["secret"].(string), webhooks.SecretPrefix) || rot["secret_rotated_at"] == nil || rot["previous_secret_expires_at"] == nil {
		t.Fatalf("rotated = %v", rot)
	}
	e.call("POST", e.whBase()+"/"+id+"/rotate", "", user("dave"), http.StatusForbidden)

	// Another workspace's admin cannot see, change or ping it.
	for _, rt := range []struct{ m, p, b string }{{"PATCH", "", `{"enabled":true}`}, {"DELETE", "", ""}, {"POST", "/rotate", ""}, {"POST", "/test", ""}, {"GET", "/deliveries", ""}} {
		code, _ := e.hs.doAs(t, rt.m, "/v1/workspaces/"+e.zed+"/webhooks/"+id+rt.p, rt.b, user("zed"))
		if code != http.StatusNotFound {
			t.Errorf("%s %s from another workspace = %d, want 404", rt.m, rt.p, code)
		}
	}
	if l := e.call("GET", "/v1/workspaces/"+e.zed+"/webhooks", "", user("zed"), http.StatusOK)["endpoints"].([]any); len(l) != 0 {
		t.Errorf("another workspace lists %v", l)
	}
	e.call("DELETE", e.whBase()+"/"+id, "", user("dave"), http.StatusForbidden)
	e.call("DELETE", e.whBase()+"/"+id, "", user("carol"), http.StatusNoContent)
	e.call("DELETE", e.whBase()+"/"+id, "", user("carol"), http.StatusNotFound)
	if l := e.call("GET", e.whBase(), "", user("carol"), http.StatusOK)["endpoints"].([]any); len(l) != 0 {
		t.Errorf("after delete: %v", l)
	}
}

func TestWebhookEndpointLimitAndBadBodies(t *testing.T) {
	rcv := newWhReceiver(t)
	e := whEnv(t, looseWebhooks())
	for i := 0; i < webhooks.MaxEndpoints; i++ {
		e.addWebhook(rcv.srv.URL, "batch.finished")
	}
	e.call("POST", e.whBase(), `{"url":"`+rcv.srv.URL+`","events":["batch.finished"]}`, user("carol"), http.StatusConflict)
	for _, bad := range []string{`{}`, `{"url":"` + rcv.srv.URL + `"}`, `{"events":["batch.finished"]}`, `{"url":"` + rcv.srv.URL + `","events":[]}`,
		`{"url":"` + rcv.srv.URL + `","events":["batch.finished"],"extra":1}`, `{"url":"` + rcv.srv.URL + `","events":["batch.finished"],"description":"` + strings.Repeat("x", 201) + `"}`, `not json`} {
		if code, _ := e.hs.doAs(t, "POST", e.whBase(), bad, user("carol")); code != http.StatusBadRequest {
			t.Errorf("%.60s = %d, want 400", bad, code)
		}
	}
}

// With the real settings an endpoint must be https and public.
func TestWebhookURLsMustBeHTTPSAndPublic(t *testing.T) {
	pub := netip.MustParseAddr("93.184.216.34")
	svc := webhooks.New(webhooks.NewMemory(), webhooks.Options{Resolver: fixedWhResolver{"hooks.example.com": {pub}, "inward.example.com": {netip.MustParseAddr("10.0.0.7")}}})
	e := whEnv(t, svc)
	post := func(u string) int {
		code, _ := e.hs.doAs(t, "POST", e.whBase(), `{"url":"`+u+`","events":["batch.finished"]}`, user("carol"))
		return code
	}
	if code := post("https://hooks.example.com/in"); code != http.StatusCreated {
		t.Fatalf("a public https URL = %d", code)
	}
	for _, bad := range []string{"http://hooks.example.com/in", "https://127.0.0.1/in", "https://localhost/in", "https://169.254.169.254/latest/meta-data", "https://[::1]/in",
		"https://10.0.0.1/in", "https://192.168.0.1/in", "https://inward.example.com/in", "https://user:pw@hooks.example.com/in", "https://metadata.google.internal/in"} {
		if code := post(bad); code != http.StatusBadRequest {
			t.Errorf("%s = %d, want 400", bad, code)
		}
	}
	// The same checks apply when the URL is changed later.
	list := e.call("GET", e.whBase(), "", user("carol"), http.StatusOK)["endpoints"].([]any)
	id := list[0].(map[string]any)["id"].(string)
	for _, bad := range []string{"http://hooks.example.com/in", "https://127.0.0.1/in", "https://inward.example.com/in"} {
		if code, _ := e.hs.doAs(t, "PATCH", e.whBase()+"/"+id, `{"url":"`+bad+`"}`, user("carol")); code != http.StatusBadRequest {
			t.Errorf("PATCH %s = %d, want 400", bad, code)
		}
	}
	// And the ping to a stored private address is refused at dial time.
	_ = svc.Store().PutEndpoint(context.Background(), webhooks.Endpoint{ID: "wh_00000000000000aa", Workspace: e.wsID, URL: newWhReceiver(t).srv.URL, Events: []string{"batch.finished"}, Enabled: true, Secret: webhooks.NewSecret()})
	out := e.call("POST", e.whBase()+"/wh_00000000000000aa/test", "", user("carol"), http.StatusOK)["delivery"].(map[string]any)
	if out["status"] != "failed" || out["attempts"].([]any)[0].(map[string]any)["error"] != "blocked_address" {
		t.Errorf("ping to a local address = %v", out)
	}
}

type fixedWhResolver map[string][]netip.Addr

func (r fixedWhResolver) LookupNetIP(_ context.Context, _, host string) ([]netip.Addr, error) {
	if ips, ok := r[host]; ok {
		return ips, nil
	}
	return nil, context.DeadlineExceeded
}

func TestWebhookTestPingAndDeliveryLog(t *testing.T) {
	rcv := newWhReceiver(t)
	svc := looseWebhooks()
	e := whEnv(t, svc)
	ep := e.addWebhook(rcv.srv.URL, "batch.finished")
	id, secret := ep["id"].(string), ep["secret"].(string)

	e.call("POST", e.whBase()+"/"+id+"/test", "", user("dave"), http.StatusForbidden)
	d := e.call("POST", e.whBase()+"/"+id+"/test", "", user("carol"), http.StatusOK)["delivery"].(map[string]any)
	if d["status"] != "delivered" || d["test"] != true || d["event"] != "webhook.test" || len(d["attempts"].([]any)) != 1 || d["attempts"].([]any)[0].(map[string]any)["status_code"] != 200.0 {
		t.Fatalf("ping = %v", d)
	}
	rcv.mu.Lock()
	h := rcv.hits[0]
	rcv.mu.Unlock()
	if err := webhooks.Verify(h.header.Get(webhooks.SignatureHeader), h.body, secret, time.Minute, time.Now()); err != nil {
		t.Errorf("ping signature: %v", err)
	}

	// A receiver that refuses shows in the log as a failed ping, not an error.
	rcv.mu.Lock()
	rcv.code = http.StatusGone
	rcv.mu.Unlock()
	d = e.call("POST", e.whBase()+"/"+id+"/test", "", user("carol"), http.StatusOK)["delivery"].(map[string]any)
	if d["status"] != "failed" || d["attempts"].([]any)[0].(map[string]any)["error"] != "http_status" {
		t.Errorf("refused ping = %v", d)
	}
	log := e.call("GET", e.whBase()+"/"+id+"/deliveries", "", user("carol"), http.StatusOK)["deliveries"].([]any)
	if len(log) != 2 || log[0].(map[string]any)["status"] != "failed" || log[1].(map[string]any)["status"] != "delivered" {
		t.Fatalf("log = %v", log)
	}
	raw, _ := json.Marshal(log)
	if strings.Contains(string(raw), secret) || strings.Contains(string(raw), "payload") || strings.Contains(string(raw), "This is a test event") {
		t.Errorf("the log carries the payload or secret: %s", raw)
	}
	if l := e.call("GET", e.whBase()+"/"+id+"/deliveries?limit=1", "", user("carol"), http.StatusOK)["deliveries"].([]any); len(l) != 1 {
		t.Errorf("limit=1 gave %d", len(l))
	}
	for _, q := range []string{"?limit=0", "?limit=101", "?limit=x", "?cursor=1"} {
		e.call("GET", e.whBase()+"/"+id+"/deliveries"+q, "", user("carol"), http.StatusBadRequest)
	}
	e.call("GET", e.whBase()+"/"+id+"/deliveries", "", user("vera"), http.StatusForbidden)
}

func TestWebhookSweepNeedsTheAdminToken(t *testing.T) {
	e := whEnv(t, looseWebhooks())
	if code, _ := e.hs.doAs(t, "POST", "/v1/webhooks/sweep", "", user("carol")); code != http.StatusForbidden {
		t.Errorf("sweep as a workspace admin = %d, want 403", code)
	}
	code, out := e.hs.doAs(t, "POST", "/v1/webhooks/sweep", "", map[string]string{"X-Admin-Token": testAdmin})
	if code != http.StatusOK || out["attempted"] != 0.0 {
		t.Errorf("sweep = %d %v", code, out)
	}
}

// ---- the events ----

func TestBatchFinishedEvent(t *testing.T) {
	rcv := newWhReceiver(t)
	svc := looseWebhooks()
	e := whEnv(t, svc)
	e.addWebhook(rcv.srv.URL, "batch.finished")
	e.addWebhook(rcv.srv.URL, "filing.status")
	// A batch from outside any workspace sends nothing.
	e.call("POST", "/v1/cases/batch", `{"items":[{"name":"a","facts":"The unit is assembled in Mexico from Chinese parts."}]}`, nil, http.StatusOK)
	svc.Wait()
	if n := len(rcv.events()); n != 0 {
		t.Fatalf("a batch outside a workspace sent %d events", n)
	}
	out := e.call("POST", "/v1/cases/batch", `{"items":[{"name":"a","facts":"The unit is assembled in Mexico from Chinese parts."},{"name":"b","facts":"A heat gun made in China."}]}`, user("dave"), http.StatusOK)
	svc.Wait()
	evs := rcv.ofType("batch.finished")
	if len(evs) != 1 {
		t.Fatalf("events = %+v", rcv.events())
	}
	d := evs[0].Data.(map[string]any)
	rows := out["rows"].([]any)
	if evs[0].Workspace != e.wsID || d["total"] != 2.0 || d["failed"] != 0.0 || len(d["case_ids"].([]any)) != 2 || d["case_ids"].([]any)[0] != rows[0].(map[string]any)["case_id"] {
		t.Errorf("batch event = %+v", evs[0])
	}
	if _, ok := d["needs_review"]; !ok {
		t.Errorf("batch event lacks the route counts: %v", d)
	}
}

func TestBudgetAlertEvent(t *testing.T) {
	rcv := newWhReceiver(t)
	svc := looseWebhooks()
	m := metering.New(metering.NewMemory())
	e := whEnv(t, svc, func(d *Deps) { d.Metering = m })
	e.addWebhook(rcv.srv.URL, "budget.alert")
	e.call("PUT", "/v1/workspaces/"+e.wsID+"/budget", `{"monthly_usd":10,"alert_percent":50}`, user("carol"), http.StatusOK)
	ctx := metering.WithWorkspace(context.Background(), e.wsID)
	m.Charge(ctx, 4)
	svc.Wait()
	if n := len(rcv.events()); n != 0 {
		t.Fatalf("under the alert share sent %d events", n)
	}
	m.Charge(ctx, 1.5)
	m.Charge(ctx, 1)
	svc.Wait()
	evs := rcv.ofType("budget.alert")
	if len(evs) != 1 {
		t.Fatalf("alert events = %+v", rcv.events())
	}
	d := evs[0].Data.(map[string]any)
	if d["level"] != "alert" || d["spent_usd"] != 5.5 || d["budget_usd"] != 10.0 || d["alert_percent"] != 50.0 || d["hard_cap"] != false || d["month"] != m.Month() {
		t.Errorf("alert = %v", d)
	}
	m.Charge(ctx, 5)
	svc.Wait()
	if evs = rcv.ofType("budget.alert"); len(evs) != 2 || evs[1].Data.(map[string]any)["level"] != "cap" {
		t.Errorf("cap events = %+v", rcv.events())
	}
	// Another workspace's spend does not reach this endpoint.
	m.Charge(metering.WithWorkspace(context.Background(), e.zed), 50)
	svc.Wait()
	if len(rcv.events()) != 2 {
		t.Errorf("another workspace's spend sent an event")
	}
}

func TestReviewDecisionEventCarriesNoComment(t *testing.T) {
	rcv := newWhReceiver(t)
	svc := looseWebhooks()
	e := whEnv(t, svc)
	e.addWebhook(rcv.srv.URL, "review.decision")
	id := e.wsCase()
	link := e.call("POST", "/v1/cases/"+id+"/reviews", `{}`, user("dave"), http.StatusCreated)
	tok := map[string]string{"X-Review-Token": link["token"].(string)}
	e.call("POST", "/v1/review/decision", `{"decision":"changes","comment":"Confidential: the supplier is Acme Ltd."}`, tok, http.StatusOK)
	svc.Wait()
	evs := rcv.ofType("review.decision")
	if len(evs) != 1 {
		t.Fatalf("events = %+v", rcv.events())
	}
	d := evs[0].Data.(map[string]any)
	if d["case_id"] != id || d["decision"] != "changes" || d["has_comment"] != true || evs[0].Workspace != e.wsID {
		t.Errorf("decision event = %v", d)
	}
	rcv.mu.Lock()
	raw := string(rcv.hits[0].body)
	rcv.mu.Unlock()
	if strings.Contains(raw, "Acme") || strings.Contains(raw, "Confidential") {
		t.Errorf("the broker's comment reached the callback: %s", raw)
	}
}

func TestFilingStatusEvent(t *testing.T) {
	rcv := newWhReceiver(t)
	svc := looseWebhooks()
	e := whEnv(t, svc)
	e.addWebhook(rcv.srv.URL, "filing.status")
	id := e.wsCase()
	base := "/v1/cases/" + id
	body, _ := json.Marshal(map[string]any{"letter": fullLetter, "attachments": []map[string]any{{"name": "photo.jpg", "description": "front view"}}})
	e.call("POST", base+"/filing", string(body), user("dave"), http.StatusOK)
	e.call("PUT", base+"/collaborators", `{"email":"dave@example.com","role":"broker"}`, user("carol"), http.StatusOK)
	e.call("POST", base+"/approvals", `{"decision":"approve"}`, user("dave"), http.StatusCreated)
	svc.Wait()
	if n := len(rcv.events()); n != 0 {
		t.Fatalf("preparing a package sent %d events", n)
	}
	e.call("POST", base+"/filing/status", `{"status":"submitted","reference":"ER-2026-0042"}`, user("dave"), http.StatusOK)
	e.call("POST", base+"/filing/status", `{"status":"acknowledged"}`, user("dave"), http.StatusOK)
	e.call("POST", base+"/filing/status", `{"status":"submitted"}`, user("dave"), http.StatusBadRequest) // refused: no event
	svc.Wait()
	evs := rcv.ofType("filing.status")
	if len(evs) != 2 {
		t.Fatalf("events = %+v", rcv.events())
	}
	got := map[string]map[string]any{}
	for _, ev := range evs {
		d := ev.Data.(map[string]any)
		got[d["status"].(string)] = d
	}
	if s := got["submitted"]; s["case_id"] != id || s["reference"] != "ER-2026-0042" || s["previous_status"] == "" {
		t.Errorf("submitted = %v", s)
	}
	if a := got["acknowledged"]; a["previous_status"] != "submitted" || a["reference"] != nil {
		t.Errorf("acknowledged = %v", a)
	}
}

func TestRulingWatchHitEvent(t *testing.T) {
	rcv := newWhReceiver(t)
	svc := looseWebhooks()
	e := whEnv(t, svc)
	e.addWebhook(rcv.srv.URL, "ruling_watch.hit")
	id := e.wsCase()
	// Unchanged: nothing is sent.
	e.call("POST", "/v1/cases/"+id+"/watch", `{}`, user("dave"), http.StatusOK)
	svc.Wait()
	if n := len(rcv.events()); n != 0 {
		t.Fatalf("an unchanged watch sent %d events", n)
	}
	// Newer rulings change the origin: one hit, and not again for the same change.
	e.hs.engine.later = "Thailand"
	w := e.call("POST", "/v1/cases/"+id+"/watch", `{}`, user("dave"), http.StatusOK)["watch"].(map[string]any)
	if w["status"] != "changed" || w["basis"] != "precedent" {
		t.Fatalf("watch = %v", w)
	}
	e.call("POST", "/v1/cases/"+id+"/watch", `{}`, user("dave"), http.StatusOK)
	svc.Wait()
	evs := rcv.ofType("ruling_watch.hit")
	if len(evs) != 1 {
		t.Fatalf("events = %+v", rcv.events())
	}
	d := evs[0].Data.(map[string]any)
	if d["case_id"] != id || d["previous_origin"] != "China" || d["new_origin"] != "Thailand" || d["newer_rulings"] != 1.0 || evs[0].Workspace != e.wsID {
		t.Errorf("hit = %v", d)
	}
	// A case that is not in a workspace sends nothing, whoever watches it.
	own := e.hs.newCase(t, user("zed"))
	e.call("POST", "/v1/cases/"+own+"/watch", `{}`, user("zed"), http.StatusOK)
	svc.Wait()
	if len(rcv.events()) != 1 {
		t.Errorf("another workspace's watch reached this endpoint")
	}
}

func TestWebhookRoutesAreAudited(t *testing.T) {
	rcv := newWhReceiver(t)
	ae := newAuditEnvWith(t, true)
	ten := ae.tenant()
	base := "/v1/workspaces/" + ae.wsID + "/webhooks"
	carol := user("carol")
	step := func(what string, want int, action audit.Action, method, path, body string) map[string]any {
		t.Helper()
		before := len(ae.events(ten))
		code, out := ae.hs.doAs(t, method, path, body, carol)
		if code != want {
			t.Fatalf("%s = %d %v, want %d", what, code, out, want)
		}
		if action != "" {
			pair(t, what, ae.since(ten, before), action, "")
		}
		return out
	}
	out := step("create", 201, audit.WebhookCreate, "POST", base, `{"url":"`+rcv.srv.URL+`","events":["batch.finished"]}`)
	ep := out["endpoint"].(map[string]any)
	id := ep["id"].(string)
	step("update", 200, audit.WebhookUpdate, "PATCH", base+"/"+id, `{"enabled":false}`)
	step("rotate", 200, audit.WebhookRotate, "POST", base+"/"+id+"/rotate", "")
	step("test", 200, audit.WebhookTest, "POST", base+"/"+id+"/test", "")
	step("delete", 204, audit.WebhookDelete, "DELETE", base+"/"+id, "")
	for _, ev := range ae.events(ten) {
		raw, _ := json.Marshal(ev)
		if strings.Contains(string(raw), ep["secret"].(string)) || strings.Contains(string(raw), rcv.srv.URL) {
			t.Fatalf("the trail carries the secret or the URL: %s", raw)
		}
	}
	// A denied attempt by an editor is recorded.
	before := len(ae.events(ten))
	if code, _ := ae.hs.doAs(t, "POST", base, `{"url":"`+rcv.srv.URL+`","events":["batch.finished"]}`, user("dave")); code != http.StatusForbidden {
		t.Fatal("editor create")
	}
	if got := ae.since(ten, before); len(got) != 1 || got[0].Outcome != audit.Denied || got[0].Action != audit.WebhookCreate {
		t.Errorf("denial = %s", brief(got))
	}
}
