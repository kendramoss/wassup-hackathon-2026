package httpapi

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// raw does a request and returns the status, content type and body.
func (e *roleEnv) raw(method, path, body string, h map[string]string) (int, string, []byte) {
	e.t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	for k, v := range h {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	e.hs.h.ServeHTTP(rec, req)
	return rec.Code, rec.Header().Get("Content-Type"), rec.Body.Bytes()
}

func (e *roleEnv) call(method, path, body string, h map[string]string, want int) map[string]any {
	e.t.Helper()
	code, out := e.hs.doAs(e.t, method, path, body, h)
	if code != want {
		e.t.Fatalf("%s %s = %d %v, want %d", method, path, code, out, want)
	}
	return out
}

func TestBrokerWorkflowRoutesAreOffByDefault(t *testing.T) {
	hs := newHarness(t, "")
	id := hs.newCase(t, nil)
	for _, rt := range []struct{ m, p string }{
		{"GET", "/v1/queue"}, {"PUT", "/v1/cases/" + id + "/queue"}, {"GET", "/v1/cases/" + id + "/collab"},
		{"POST", "/v1/cases/" + id + "/comments"}, {"POST", "/v1/cases/" + id + "/filing"}, {"GET", "/v1/review/comments"},
	} {
		if code, _ := hs.doAs(t, rt.m, rt.p, `{}`, nil); code != http.StatusNotFound && code != http.StatusMethodNotAllowed {
			t.Errorf("%s %s with BROKER_WORKFLOW off = %d, want 404", rt.m, rt.p, code)
		}
	}
}

func TestReviewQueueFlow(t *testing.T) {
	e := newRoleEnv(t, true)
	id := e.wsCase()
	base := "/v1/cases/" + id
	e.call("PUT", base+"/queue", `{"priority":"bogus","reason":"x"}`, user("dave"), http.StatusBadRequest)
	e.call("PUT", base+"/queue", `{"priority":"high"}`, user("dave"), http.StatusBadRequest)
	e.call("PUT", base+"/queue", `{"priority":"high","reason":"rule check disagrees"}`, user("vera"), http.StatusForbidden)
	q := e.call("PUT", base+"/queue", `{"priority":"high","reason":"rule check disagrees"}`, user("dave"), http.StatusOK)
	if item := q["item"].(map[string]any); item["state"] != "queued" || item["queued_by"] != "dave@example.com" {
		t.Fatalf("enqueue = %v", q)
	}
	list := e.call("GET", "/v1/queue", "", user("vera"), http.StatusOK)
	if items := list["items"].([]any); len(items) != 1 || items[0].(map[string]any)["case_id"] != id {
		t.Fatalf("queue as a viewer = %v", list)
	}
	// An editor may take a review, not hand it to someone else; an admin may.
	e.call("POST", base+"/queue/transition", `{"action":"assign","assignee":"vera@example.com"}`, user("dave"), http.StatusForbidden)
	e.call("POST", base+"/queue/transition", `{"action":"assign","assignee":"dave@example.com"}`, user("carol"), http.StatusOK)
	e.call("POST", base+"/queue/transition", `{"action":"start"}`, user("carol"), http.StatusConflict)
	e.call("POST", base+"/queue/transition", `{"action":"start"}`, user("dave"), http.StatusOK)
	e.call("POST", base+"/queue/transition", `{"action":"decide","outcome":"approve"}`, user("dave"), http.StatusBadRequest)
	done := e.call("POST", base+"/queue/transition", `{"action":"decide","outcome":"changes","reason":"ask for the motor's HTS code"}`, user("dave"), http.StatusOK)
	d := done["item"].(map[string]any)["decision"].(map[string]any)
	if d["outcome"] != "changes" || d["reason"] != "ask for the motor's HTS code" || d["on_time"] != true {
		t.Errorf("decision = %v", d)
	}
	if open := e.call("GET", "/v1/queue", "", user("dave"), http.StatusOK)["items"].([]any); len(open) != 0 {
		t.Errorf("open queue after the decision = %v", open)
	}
	if all := e.call("GET", "/v1/queue?state=decided&assignee=me", "", user("dave"), http.StatusOK)["items"].([]any); len(all) != 1 {
		t.Errorf("decided queue = %v", all)
	}
	e.call("GET", "/v1/queue?state=nope", "", user("dave"), http.StatusBadRequest)
	got := e.call("GET", base, "", user("dave"), http.StatusOK)
	hist, _ := json.Marshal(got["history"])
	if !strings.Contains(string(hist), "review decided: changes") {
		t.Errorf("history lacks the decision: %s", hist)
	}
}

func TestCollaborationFlow(t *testing.T) {
	e := newRoleEnv(t, true)
	id := e.wsCase()
	base := "/v1/cases/" + id
	c := e.call("POST", base+"/comments", `{"body":"Is the motor Chinese?"}`, user("vera"), http.StatusCreated)
	threads := c["threads"].([]any)
	first := threads[0].(map[string]any)
	if first["role"] != "viewer" || first["author"] != "vera@example.com" {
		t.Fatalf("comment = %v", first)
	}
	e.call("POST", base+"/comments", `{"body":"yes","parent_id":"`+first["id"].(string)+`"}`, user("dave"), http.StatusCreated)
	e.call("POST", base+"/comments", `{"body":"x","parent_id":"cm_nope"}`, user("dave"), http.StatusBadRequest)
	e.call("POST", base+"/comments", `{"body":"  "}`, user("dave"), http.StatusBadRequest)
	e.call("POST", base+"/approvals", `{"decision":"approve"}`, user("vera"), http.StatusForbidden)
	e.call("POST", base+"/approvals", `{"decision":"changes"}`, user("dave"), http.StatusBadRequest)
	e.call("PUT", base+"/collaborators", `{"email":"dave@example.com","role":"broker"}`, user("carol"), http.StatusOK)
	e.call("PUT", base+"/collaborators", `{"email":"dave@example.com","role":"boss"}`, user("carol"), http.StatusBadRequest)
	v := e.call("POST", base+"/approvals", `{"decision":"approve"}`, user("dave"), http.StatusCreated)
	so := v["sign_off"].(map[string]any)
	if so["state"] != "approved" || so["broker_approved"] != true {
		t.Errorf("sign-off = %v", so)
	}
	v = e.call("POST", base+"/approvals", `{"decision":"reject","reason":"wrong origin"}`, user("carol"), http.StatusCreated)
	if so := v["sign_off"].(map[string]any); so["state"] != "rejected" {
		t.Errorf("sign-off after a rejection = %v", so)
	}
	v = e.call("GET", base+"/collab", "", user("vera"), http.StatusOK)
	if th := v["threads"].([]any); len(th) != 1 || len(th[0].(map[string]any)["replies"].([]any)) != 1 {
		t.Errorf("threads = %v", th)
	}
}

func TestExternalBrokerCommentsAndApproval(t *testing.T) {
	e := newRoleEnv(t, true)
	id := e.wsCase()
	link := e.call("POST", "/v1/cases/"+id+"/reviews", `{}`, user("dave"), http.StatusCreated)
	tok := map[string]string{"X-Review-Token": link["token"].(string)}
	e.call("POST", "/v1/cases/"+id+"/comments", `{"body":"please check the housing"}`, user("dave"), http.StatusCreated)
	got := e.call("GET", "/v1/review/comments", "", tok, http.StatusOK)
	th := got["threads"].([]any)
	if len(th) != 1 || th[0].(map[string]any)["author"] != "editor" {
		t.Fatalf("broker sees = %v (authors must be roles only)", th)
	}
	e.call("POST", "/v1/review/comments", `{"body":"housing is fine"}`, tok, http.StatusCreated)
	e.call("POST", "/v1/review/comments", `{"body":"x"}`, map[string]string{"X-Review-Token": "ubr_000000000000_" + strings.Repeat("a", 43)}, http.StatusNotFound)
	e.call("POST", "/v1/review/decision", `{"decision":"approve"}`, tok, http.StatusOK)
	v := e.call("GET", "/v1/cases/"+id+"/collab", "", user("dave"), http.StatusOK)
	if so := v["sign_off"].(map[string]any); so["broker_approved"] != true {
		t.Errorf("a review-link approval should count: %v", so)
	}
	if th := v["threads"].([]any); len(th) != 2 || th[1].(map[string]any)["role"] != "external_broker" {
		t.Errorf("threads = %v", th)
	}
}

// fullLetter is a letter with every section and no placeholder left.
const fullLetter = `27 November 2026

**RE: Request for a binding ruling**

## 1. Interested parties

- Requester: Acme Tools Inc., 1 Main St. Interest: importer.

## 2. Port of entry

Laredo, Texas.

## 3. The transaction

Prospective importation from Mexico, first shipment March 2027.

## 4. The merchandise

- Name and model: drill D-20.

## 5. Materials, countries and processing

- Motor: China.

## 6. Question presented

What is the country of origin?

## 7. Requester's position and supporting authority

Mexico, under 19 CFR 102.20.

## 8. Prior or current consideration

Not considered by any CBP office or court.

## 9. Documents and samples

- Exhibit A.

## 10. Confidential treatment

No confidential treatment requested.

## 11. Declaration and signature

/s/ Jo Smith
`

func TestFilingPackageFlow(t *testing.T) {
	e := newRoleEnv(t, true)
	id := e.wsCase()
	base := "/v1/cases/" + id
	if v := e.call("GET", base+"/filing", "", user("vera"), http.StatusOK); v["ready_to_file"] != false {
		t.Errorf("no package yet = %v", v)
	}
	e.call("GET", base+"/filing?format=zip", "", user("dave"), http.StatusNotFound)
	e.call("POST", base+"/filing", `{}`, user("vera"), http.StatusForbidden)
	v := e.call("POST", base+"/filing", `{"submitter":{"requester_name":"Acme Tools Inc.","interest":"importer"}}`, user("dave"), http.StatusOK)
	if v["status"] != "prepared" || v["ready"] != false || v["ready_to_file"] != false || !strings.Contains(v["letter"].(string), "Acme Tools Inc.") {
		t.Fatalf("generated package = %v", v)
	}
	e.call("POST", base+"/filing", `{"submitter":{"interest":"spy"}}`, user("dave"), http.StatusBadRequest)
	e.call("POST", base+"/filing", `{"attachments":[{"name":"../x"}]}`, user("dave"), http.StatusBadRequest)
	// An edited, complete letter: every element present, but no broker yet.
	body, _ := json.Marshal(map[string]any{"letter": fullLetter, "attachments": []map[string]any{{"name": "photo.jpg", "description": "front view"}}})
	v = e.call("POST", base+"/filing", string(body), user("dave"), http.StatusOK)
	if v["ready"] != true || v["ready_to_file"] != false {
		t.Fatalf("complete letter = ready %v, ready_to_file %v, blocking %v", v["ready"], v["ready_to_file"], v["blocking"])
	}
	e.call("POST", base+"/filing/status", `{"status":"submitted","reference":"ER-1"}`, user("dave"), http.StatusConflict)
	e.call("PUT", base+"/collaborators", `{"email":"dave@example.com","role":"broker"}`, user("carol"), http.StatusOK)
	e.call("POST", base+"/approvals", `{"decision":"approve"}`, user("dave"), http.StatusCreated)
	if v := e.call("GET", base+"/filing", "", user("dave"), http.StatusOK); v["ready_to_file"] != true {
		t.Fatalf("after the broker's approval = %v", v["blocking"])
	}
	code, ct, z := e.raw("GET", base+"/filing?format=zip", "", user("vera"))
	if code != http.StatusOK || ct != "application/zip" {
		t.Fatalf("zip = %d %s", code, ct)
	}
	r, err := zip.NewReader(bytes.NewReader(z), int64(len(z)))
	if err != nil || len(r.File) != 5 {
		t.Fatalf("zip has %d files, %v", len(r.File), err)
	}
	if code, ct, pdf := e.raw("GET", base+"/filing?format=pdf", "", user("vera")); code != http.StatusOK || ct != "application/pdf" || !bytes.HasPrefix(pdf, []byte("%PDF-")) {
		t.Errorf("pdf = %d %s", code, ct)
	}
	pj := e.call("GET", base+"/filing?format=json", "", user("dave"), http.StatusOK)
	pkg := pj["package"].(map[string]any)
	if pkg["schema"] != "undelabs.erulings-package.v1" || pj["ready_to_file"] != true {
		t.Errorf("package json = %v", pj)
	}
	e.call("GET", base+"/filing?format=docx", "", user("dave"), http.StatusBadRequest)
	e.call("POST", base+"/filing/status", `{"status":"submitted"}`, user("dave"), http.StatusBadRequest)
	e.call("POST", base+"/filing/status", `{"status":"ruling_issued","reference":"N1"}`, user("dave"), http.StatusConflict)
	e.call("POST", base+"/filing/status", `{"status":"submitted","reference":"ER-2026-0042"}`, user("vera"), http.StatusForbidden)
	v = e.call("POST", base+"/filing/status", `{"status":"submitted","reference":"ER-2026-0042"}`, user("dave"), http.StatusOK)
	if v["cbp_reference"] != "ER-2026-0042" || v["status"] != "submitted" {
		t.Errorf("submitted = %v", v)
	}
	e.call("POST", base+"/filing", string(body), user("dave"), http.StatusConflict)
	e.call("POST", base+"/filing/status", `{"status":"acknowledged"}`, user("dave"), http.StatusOK)
	v = e.call("POST", base+"/filing/status", `{"status":"ruling_issued","reference":"N312345"}`, user("dave"), http.StatusOK)
	if v["ruling_number"] != "N312345" || len(v["next_statuses"].([]any)) != 0 || len(v["history"].([]any)) != 4 {
		t.Errorf("ruling issued = %v", v)
	}
}
