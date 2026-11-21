package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/audit"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/store"
)

// Broker review links (D200).

const (
	rvAlice = "alice-visitor-id-0001"
	rvBob   = "bob-visitor-id-000002"
)

func visitor(id string) map[string]string { return map[string]string{"X-Visitor-ID": id} }

func tokenHeader(tok string) map[string]string { return map[string]string{reviewTokenHeader: tok} }

// reviewHarness: ownership on, limits on (3 invalid tokens a minute).
func reviewHarness(t *testing.T, adjust ...func(*Deps)) (harness, *store.Memory) {
	t.Helper()
	cs := store.NewMemory()
	hs := newHarnessWith(t, "substantial_transformation", func(d *Deps) {
		d.Cases, d.CaseOwnership = cs, true
		d.Limits = Limits{BasicPerMin: 1000, LightPerMin: 1000, HeavyPerMin: 1000, GlobalPerMin: 10000, LookupPerMin: 1000, AuthFailuresPerMin: 3, ReviewPerMin: 1000}
		for _, f := range adjust {
			f(d)
		}
	})
	return hs, cs
}

// makeLink creates a link on id as h and returns the token and the review.
func makeLink(t *testing.T, hs harness, id, body string, h map[string]string) (string, map[string]any) {
	t.Helper()
	code, out := hs.doAs(t, "POST", "/v1/cases/"+id+"/reviews", body, h)
	if code != http.StatusCreated {
		t.Fatalf("create link = %d %v", code, out)
	}
	tok, _ := out["token"].(string)
	if !reviewTokenPattern.MatchString(tok) || out["review_path"] != "/review/"+tok {
		t.Fatalf("token %q path %v", tok, out["review_path"])
	}
	return tok, out["review"].(map[string]any)
}

func TestBrokerReviewLoop(t *testing.T) {
	hs, cs := reviewHarness(t)
	alice := visitor(rvAlice)
	id := hs.newCase(t, alice)
	tok, rv := makeLink(t, hs, id, `{}`, alice)
	if rv["status"] != reviewPending || rv["subject"] != "setup" || rv["draft_label"] != "Current setup" {
		t.Errorf("new link = %v", rv)
	}
	created, _ := time.Parse(time.RFC3339Nano, rv["created_at"].(string))
	expires, _ := time.Parse(time.RFC3339Nano, rv["expires_at"].(string))
	if got := expires.Sub(created); got != 14*24*time.Hour {
		t.Errorf("default life = %v, want 14 days", got)
	}

	// Only the hash is stored; the token is nowhere in the case.
	c, _ := cs.Get(context.Background(), id)
	blob, _ := json.Marshal(c)
	if strings.Contains(string(blob), tok) || strings.Contains(string(blob), tok[len(tok)-43:]) {
		t.Fatal("the token is stored in the case")
	}
	if len(c.BrokerReviews) != 1 || c.BrokerReviews[0].TokenHash != reviewTokenHash(tok) {
		t.Fatalf("stored review = %+v", c.BrokerReviews)
	}

	// The broker reads the packet with the token alone.
	rec := serve(hs, "GET", "/v1/review", "", tokenHeader(tok))
	if rec.Code != http.StatusOK {
		t.Fatalf("broker read = %d %s", rec.Code, rec.Body)
	}
	if rec.Header().Get("X-Robots-Tag") != "noindex, nofollow" || rec.Header().Get("Referrer-Policy") != "no-referrer" || rec.Header().Get("Cache-Control") != "no-store" {
		t.Errorf("review headers = %v", rec.Header())
	}
	var pk map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &pk)
	draft := pk["draft"].(map[string]any)
	if pk["disclaimer"] == "" || draft["content"] == "" || pk["prediction"].(map[string]any)["origin_country"] == "" || pk["review"].(map[string]any)["status"] != reviewViewed {
		t.Errorf("packet = %v", pk)
	}
	for _, leak := range []string{id, "token_hash", "owner", "history", "trace", "reasoning"} {
		if strings.Contains(rec.Body.String(), `"`+leak) {
			t.Errorf("the packet carries %q", leak)
		}
	}

	// Request changes: a comment is required, capped, kept as plain text.
	if code, out := hs.doAs(t, "POST", "/v1/review/decision", `{"decision":"changes"}`, tokenHeader(tok)); code != http.StatusBadRequest {
		t.Errorf("changes without a comment = %d %v", code, out)
	}
	long := strings.Repeat("x", MaxReviewComment+1)
	if code, _ := hs.doAs(t, "POST", "/v1/review/decision", `{"decision":"approve","comment":"`+long+`"}`, tokenHeader(tok)); code != http.StatusBadRequest {
		t.Errorf("an over-long comment = %d", code)
	}
	if code, _ := hs.doAs(t, "POST", "/v1/review/decision", `{"decision":"ship it"}`, tokenHeader(tok)); code != http.StatusBadRequest {
		t.Errorf("an unknown decision = %d", code)
	}
	comment := "<script>alert(1)</script>\r\nName the plant\u0007 in section 3."
	body, _ := json.Marshal(map[string]string{"decision": "changes", "comment": comment})
	code, out := hs.doAs(t, "POST", "/v1/review/decision", string(body), tokenHeader(tok))
	if code != http.StatusOK || out["review"].(map[string]any)["status"] != reviewChanges {
		t.Fatalf("request changes = %d %v", code, out)
	}

	// The owner sees the status and the comment, stored as plain text.
	code, out = hs.doAs(t, "GET", "/v1/cases/"+id+"/reviews", "", alice)
	if code != http.StatusOK {
		t.Fatalf("owner list = %d", code)
	}
	got := out["reviews"].([]any)[0].(map[string]any)
	ds := got["decisions"].([]any)
	if got["status"] != reviewChanges || len(ds) != 1 || ds[0].(map[string]any)["comment"] != "<script>alert(1)</script>\nName the plant in section 3." {
		t.Errorf("owner view = %v", got)
	}
	if got["first_viewed_at"] == nil {
		t.Error("first view not stamped")
	}
	rec = serve(hs, "GET", "/v1/cases/"+id, "", alice)
	if !strings.Contains(rec.Body.String(), `"broker_reviews"`) || strings.Contains(rec.Body.String(), "<script>") || strings.Contains(rec.Body.String(), "token_hash") {
		t.Errorf("case answer: comment must be JSON-escaped and no hash shown: %s", rec.Body.String()[:200])
	}

	// Approve after the change; the owner sees approved.
	if code, out := hs.doAs(t, "POST", "/v1/review/decision", `{"decision":"approve","comment":"Looks right."}`, tokenHeader(tok)); code != http.StatusOK || out["review"].(map[string]any)["status"] != reviewApproved {
		t.Fatalf("approve = %d %v", code, out)
	}
	_, out = hs.doAs(t, "GET", "/v1/cases/"+id+"/reviews", "", alice)
	if s := out["reviews"].([]any)[0].(map[string]any)["status"]; s != reviewApproved {
		t.Errorf("owner status = %v", s)
	}

	// The broker changed no analysis: only the link's record and history.
	after, _ := cs.Get(context.Background(), id)
	if after.Prediction.Origin != c.Prediction.Origin || !after.UpdatedAt.Equal(c.UpdatedAt) || after.ExportedAt != nil {
		t.Error("a broker answer changed the case analysis")
	}

	// Revoke: 410 from then on, for reads and answers.
	rid := got["id"].(string)
	if code, out := hs.doAs(t, "DELETE", "/v1/cases/"+id+"/reviews/"+rid, "", alice); code != http.StatusOK || out["review"].(map[string]any)["status"] != reviewRevoked {
		t.Fatalf("revoke = %d %v", code, out)
	}
	if code, out := hs.doAs(t, "GET", "/v1/review", "", tokenHeader(tok)); code != http.StatusGone || out["code"] != "link_revoked" {
		t.Errorf("revoked read = %d %v", code, out)
	}
	if code, out := hs.doAs(t, "POST", "/v1/review/decision", `{"decision":"approve"}`, tokenHeader(tok)); code != http.StatusGone || out["code"] != "link_revoked" {
		t.Errorf("revoked answer = %d %v", code, out)
	}
	if code, out := hs.doAs(t, "DELETE", "/v1/cases/"+id+"/reviews/rv_0000000000000000", "", alice); code != http.StatusNotFound || out["code"] != "review_not_found" {
		t.Errorf("revoke unknown = %d %v", code, out)
	}
}

func TestBrokerReviewExpiry(t *testing.T) {
	hs, cs := reviewHarness(t)
	alice := visitor(rvAlice)
	id := hs.newCase(t, alice)
	for _, bad := range []string{`{"expires_in_days":0}`, `{"expires_in_days":31}`, `{"expires_in_days":-1}`, `{"token":"x"}`} {
		if code, _ := hs.doAs(t, "POST", "/v1/cases/"+id+"/reviews", bad, alice); code != http.StatusBadRequest {
			t.Errorf("create %s = %d, want 400", bad, code)
		}
	}
	tok, rv := makeLink(t, hs, id, `{"expires_in_days":30}`, alice)
	created, _ := time.Parse(time.RFC3339Nano, rv["created_at"].(string))
	expires, _ := time.Parse(time.RFC3339Nano, rv["expires_at"].(string))
	if expires.Sub(created) != 30*24*time.Hour {
		t.Errorf("30-day link lives %v", expires.Sub(created))
	}
	// Expiry is the server's: move the stored expiry into the past.
	if _, err := cs.Update(context.Background(), id, func(c *store.Case) error {
		c.BrokerReviews[0].ExpiresAt = time.Now().Add(-time.Second)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if code, out := hs.doAs(t, "GET", "/v1/review", "", tokenHeader(tok)); code != http.StatusGone || out["code"] != "link_expired" {
		t.Errorf("expired read = %d %v", code, out)
	}
	if code, _ := hs.doAs(t, "POST", "/v1/review/decision", `{"decision":"approve"}`, tokenHeader(tok)); code != http.StatusGone {
		t.Errorf("expired answer = %d", code)
	}
	_, out := hs.doAs(t, "GET", "/v1/cases/"+id+"/reviews", "", alice)
	if s := out["reviews"].([]any)[0].(map[string]any)["status"]; s != reviewExpired {
		t.Errorf("owner status = %v", s)
	}
}

func TestBrokerReviewOpenLinkCap(t *testing.T) {
	hs, _ := reviewHarness(t)
	alice := visitor(rvAlice)
	id := hs.newCase(t, alice)
	for i := 0; i < maxActiveReviews; i++ {
		makeLink(t, hs, id, `{}`, alice)
	}
	if code, out := hs.doAs(t, "POST", "/v1/cases/"+id+"/reviews", `{}`, alice); code != http.StatusConflict {
		t.Errorf("link %d = %d %v, want 409", maxActiveReviews+1, code, out)
	}
}

// A token opens its own link's snapshot and nothing else.
func TestBrokerReviewTokenIsolation(t *testing.T) {
	hs, _ := reviewHarness(t)
	alice, bob := visitor(rvAlice), visitor(rvBob)
	aID, bID := hs.newCase(t, alice), hs.newCase(t, bob)
	aTok, _ := makeLink(t, hs, aID, `{}`, alice)
	bTok, _ := makeLink(t, hs, bID, `{}`, bob)

	// Alice's secret with Bob's case id: the hash names no link on Bob's case.
	forged := "ubr_" + strings.TrimPrefix(bID, "case_") + aTok[len("ubr_")+12:]
	if code, out := hs.doAs(t, "GET", "/v1/review", "", tokenHeader(forged)); code != http.StatusNotFound || out["code"] != "not_found" {
		t.Errorf("alice's secret on bob's case = %d %v", code, out)
	}
	// The token is not an identity for the owner's routes.
	for _, p := range []string{"/v1/cases/" + aID, "/v1/cases/" + aID + "/reviews"} {
		if code, _ := hs.doAs(t, "GET", p, "", tokenHeader(aTok)); code != http.StatusNotFound {
			t.Errorf("GET %s with only a review token = %d, want 404", p, code)
		}
	}
	if code, _ := hs.doAs(t, "POST", "/v1/cases/"+aID+"/reviews", `{}`, tokenHeader(aTok)); code != http.StatusNotFound {
		t.Errorf("a review token made a link = %d", code)
	}
	// Bob cannot make, list or revoke links on Alice's case: the same 404 as a missing case.
	_, aList := hs.doAs(t, "GET", "/v1/cases/"+aID+"/reviews", "", alice)
	aRid := aList["reviews"].([]any)[0].(map[string]any)["id"].(string)
	for _, rt := range []struct{ method, path, body string }{
		{"POST", "/v1/cases/" + aID + "/reviews", `{}`},
		{"GET", "/v1/cases/" + aID + "/reviews", ""},
		{"DELETE", "/v1/cases/" + aID + "/reviews/" + aRid, ""},
	} {
		code, out := hs.doAs(t, rt.method, rt.path, rt.body, bob)
		gone, _ := hs.doAs(t, rt.method, strings.Replace(rt.path, aID, "case_000000000000", 1), rt.body, bob)
		if code != http.StatusNotFound || out["code"] != "not_found" || gone != code {
			t.Errorf("bob %s %s = %d %v", rt.method, rt.path, code, out)
		}
	}
	// Bob's own link id on Alice's case is not found either.
	_, bList := hs.doAs(t, "GET", "/v1/cases/"+bID+"/reviews", "", bob)
	bRid := bList["reviews"].([]any)[0].(map[string]any)["id"].(string)
	if code, out := hs.doAs(t, "DELETE", "/v1/cases/"+aID+"/reviews/"+bRid, "", alice); code != http.StatusNotFound || out["code"] != "review_not_found" {
		t.Errorf("alice revokes bob's link id on her case = %d %v", code, out)
	}
	if code, _ := hs.doAs(t, "GET", "/v1/review", "", tokenHeader(bTok)); code != http.StatusOK {
		t.Errorf("bob's link stopped working = %d", code)
	}
	// A token in the query or the path is never read.
	if code, _ := hs.doAs(t, "GET", "/v1/review?token="+aTok, "", nil); code != http.StatusNotFound {
		t.Errorf("token in the query = %d, want 404", code)
	}
}

// Invalid tokens are throttled per client before any lookup, and a pinned
// demo is never shared out.
func TestBrokerReviewBadTokensAndDemos(t *testing.T) {
	hs, _ := reviewHarness(t)
	alice := visitor(rvAlice)
	id := hs.newCase(t, alice)
	tok, _ := makeLink(t, hs, id, `{}`, alice)
	client := map[string]string{"X-Client-IP": "203.0.113.9"}
	bad := []string{"", "nope", tok[:len(tok)-1] + "A", tok + "x", strings.ToUpper(tok)}
	for i, b := range bad[:3] {
		if code, out := hs.doAs(t, "GET", "/v1/review", "", with(client, tokenHeader(b))); code != http.StatusNotFound || out["code"] != "not_found" {
			t.Errorf("bad token %d = %d %v", i, code, out)
		}
	}
	// The budget (3) is spent: even the valid token waits, from this client only.
	rec := serve(hs, "GET", "/v1/review", "", with(client, tokenHeader(tok)))
	if rec.Code != http.StatusTooManyRequests || !strings.Contains(rec.Body.String(), "auth_rate_limited") || rec.Header().Get("Retry-After") == "" {
		t.Errorf("after 3 bad tokens = %d %s", rec.Code, rec.Body)
	}
	if code, _ := hs.doAs(t, "GET", "/v1/review", "", with(map[string]string{"X-Client-IP": "203.0.113.10"}, tokenHeader(tok))); code != http.StatusOK {
		t.Errorf("another client = %d", code)
	}

	// Pinned demo: no link, even with the admin token, and no field.
	demo := hs.newCase(t, admin)
	hs.doAs(t, "PUT", "/v1/cases/"+demo+"/pin", `{"pinned":true}`, admin)
	for _, h := range []map[string]string{alice, admin} {
		if code, _ := hs.doAs(t, "POST", "/v1/cases/"+demo+"/reviews", `{}`, h); code != http.StatusForbidden {
			t.Errorf("link on the pinned demo as %v = %d, want 403", h, code)
		}
	}
}

func TestBrokerReviewRateClass(t *testing.T) {
	hs, _ := reviewHarness(t, func(d *Deps) { d.Limits.ReviewPerMin = 2 })
	alice := visitor(rvAlice)
	id := hs.newCase(t, alice)
	tok, _ := makeLink(t, hs, id, `{}`, alice)
	client := with(map[string]string{"X-Client-IP": "203.0.113.20"}, tokenHeader(tok))
	for i, want := range []string{"1", "0"} {
		rec := serve(hs, "GET", "/v1/review", "", client)
		if rec.Code != http.StatusOK || rec.Header().Get("RateLimit-Limit") != "2" || rec.Header().Get("RateLimit-Remaining") != want {
			t.Errorf("read %d = %d %v", i+1, rec.Code, rec.Header())
		}
	}
	if rec := serve(hs, "GET", "/v1/review", "", client); rec.Code != http.StatusTooManyRequests || !strings.Contains(rec.Body.String(), `"rate_limited"`) {
		t.Errorf("past the review limit = %d %s", rec.Code, rec.Body)
	}
	// The owner's cheap routes have their own bucket.
	if code, _ := hs.doAs(t, "GET", "/v1/cases/"+id+"/reviews", "", with(alice, map[string]string{"X-Client-IP": "203.0.113.20"})); code != http.StatusOK {
		t.Errorf("owner list after the broker's limit = %d", code)
	}
}

// A plan draft freezes the plan, its prediction and its supporting ruling.
func TestBrokerReviewPlanDraft(t *testing.T) {
	hs, cs := reviewHarness(t)
	alice := visitor(rvAlice)
	id := hs.newCase(t, alice)
	if code, out := hs.doAs(t, "POST", "/v1/cases/"+id+"/plan", `{"target":"Thailand"}`, alice); code != http.StatusOK {
		t.Fatalf("plan = %d %v", code, out)
	}
	if code, _ := hs.doAs(t, "POST", "/v1/cases/"+id+"/reviews", `{"plan_index":99}`, alice); code != http.StatusBadRequest {
		t.Errorf("bad plan index = %d", code)
	}
	tok, rv := makeLink(t, hs, id, `{"plan_index":0}`, alice)
	if rv["subject"] != "plan" || rv["draft_label"] == "Current setup" {
		t.Errorf("plan link = %v", rv)
	}
	_, pk := hs.doAs(t, "GET", "/v1/review", "", tokenHeader(tok))
	plan, _ := pk["plan"].(map[string]any)
	if plan == nil || plan["description"] != rv["draft_label"] || plan["predicted_origin"] == "" {
		t.Errorf("packet plan = %v", pk["plan"])
	}
	// The snapshot is frozen: a later change to the plan does not change what the broker reads.
	before := pk["draft"].(map[string]any)["content"]
	if _, err := cs.Update(context.Background(), id, func(c *store.Case) error {
		c.Plan.Plans[0].Description = "a different move"
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	_, pk = hs.doAs(t, "GET", "/v1/review", "", tokenHeader(tok))
	if pk["draft"].(map[string]any)["content"] != before || pk["plan"].(map[string]any)["description"] == "a different move" {
		t.Error("the broker's draft changed after the link was made")
	}
}

// The audit trail records create, view, decision and revoke in the case's
// tenant, and never the comment, the token or case content.
func TestBrokerReviewAudit(t *testing.T) {
	e := newAuditEnv(t)
	ten := e.tenant()
	dave := user("dave")
	id := e.wsCase()
	before := len(e.events(ten))
	code, out := e.hs.doAs(t, "POST", "/v1/cases/"+id+"/reviews", `{}`, dave)
	if code != http.StatusCreated {
		t.Fatalf("create = %d %v", code, out)
	}
	tok := out["token"].(string)
	rid := out["review"].(map[string]any)["id"].(string)
	in, done := pair(t, "create", e.since(ten, before), audit.ReviewCreate, "setup")
	if in.Object != "case:"+id || done.Object != "case:"+id || in.Actor != e.actor("dave@example.com") {
		t.Errorf("create events = %+v %+v", in, done)
	}

	before = len(e.events(ten))
	if code, _ := e.hs.doAs(t, "GET", "/v1/review", "", tokenHeader(tok)); code != http.StatusOK {
		t.Fatalf("view = %d", code)
	}
	if evs := e.since(ten, before); len(evs) != 1 || evs[0].Action != audit.ReviewView || evs[0].Phase != audit.PhaseSingle || evs[0].ActorKind != audit.ActorAnonymous {
		t.Errorf("view events = %s", brief(evs))
	}

	before = len(e.events(ten))
	if code, _ := e.hs.doAs(t, "POST", "/v1/review/decision", `{"decision":"changes","comment":"Secret Widget plant in Mexico"}`, tokenHeader(tok)); code != http.StatusOK {
		t.Fatalf("decision = %d", code)
	}
	pair(t, "decision", e.since(ten, before), audit.ReviewDecision, "changes")

	before = len(e.events(ten))
	if code, _ := e.hs.doAs(t, "DELETE", "/v1/cases/"+id+"/reviews/"+rid, "", dave); code != http.StatusOK {
		t.Fatalf("revoke = %d", code)
	}
	pair(t, "revoke", e.since(ten, before), audit.ReviewRevoke, "")

	// A revoked or invalid token is an anonymous auth denial in the platform tenant.
	pBefore := len(e.events("platform"))
	e.hs.doAs(t, "GET", "/v1/review", "", tokenHeader(tok))
	e.hs.doAs(t, "GET", "/v1/review", "", tokenHeader("ubr_nope"))
	if evs := e.since("platform", pBefore); len(evs) != 2 || evs[0].Action != audit.AuthDenied || evs[1].Action != audit.AuthDenied {
		t.Errorf("platform denials = %s", brief(evs))
	}

	for _, tn := range []string{ten, "platform"} {
		blob, _ := json.Marshal(e.events(tn))
		for _, secret := range []string{tok, tok[len(tok)-43:], "Secret Widget", "Mexico", "rv_", "dave"} {
			if strings.Contains(string(blob), secret) {
				t.Errorf("tenant %s holds %q", tn, secret)
			}
		}
	}
	res, err := e.svc.Verify(context.Background(), ten, "")
	if err != nil || !res.OK {
		t.Errorf("verify = %+v %v", res, err)
	}
}

// No log line carries the token: the route is logged by pattern, and the
// handlers log nothing of their own.
func TestBrokerReviewTokenNeverLogged(t *testing.T) {
	buf := useLogger(t, func(w io.Writer) slog.Handler {
		return slog.NewJSONHandler(w, &slog.HandlerOptions{Level: slog.LevelDebug})
	})
	hs, _ := reviewHarness(t)
	alice := visitor(rvAlice)
	id := hs.newCase(t, alice)
	tok, _ := makeLink(t, hs, id, `{}`, alice)
	for _, rt := range []struct{ method, path, body string }{{"GET", "/v1/review", ""}, {"POST", "/v1/review/decision", `{"decision":"approve"}`}, {"GET", "/v1/review", ""}} {
		req := httptest.NewRequest(rt.method, rt.path, strings.NewReader(rt.body))
		req.Header.Set(reviewTokenHeader, tok)
		hs.h.ServeHTTP(httptest.NewRecorder(), req)
	}
	hs.h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/v1/review", nil)) // no token: a refusal
	out := buf.String()
	if strings.Contains(out, tok) || strings.Contains(out, tok[len(tok)-43:]) || !strings.Contains(out, "GET /v1/review") {
		t.Errorf("log = %s", out)
	}
}
