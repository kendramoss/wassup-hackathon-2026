package httpapi

import (
	"context"
	"encoding/csv"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/audit"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/retention"
)

func rawGet(t *testing.T, e *auditEnv, path string, h map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest("GET", path, nil)
	for k, v := range h {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	e.hs.h.ServeHTTP(rec, req)
	return rec
}

func TestAuditExportJSONLVerifiesOffline(t *testing.T) {
	e := newAuditEnv(t)
	e.wsCase()
	e.wsCase()
	rec := rawGet(t, e, "/v1/audit/export", user("carol"))
	if rec.Code != http.StatusOK || rec.Header().Get("Content-Type") != "application/x-ndjson" {
		t.Fatalf("export = %d %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Header().Get("Content-Disposition"), "attachment") || rec.Header().Get("X-Audit-Head-Hash") == "" {
		t.Fatalf("headers %v", rec.Header())
	}
	res, err := audit.VerifyExport(strings.NewReader(rec.Body.String()))
	if err != nil {
		t.Fatal(err)
	}
	if !res.OK || !res.Whole || res.Events == 0 || res.Tenant != e.tenant() {
		t.Fatalf("verify %+v", res)
	}
	// The export itself is recorded, after the snapshot it exported.
	evs := e.events(e.tenant())
	last := evs[len(evs)-1]
	if last.Action != audit.AuditExport || last.Detail != "jsonl" {
		t.Fatalf("last event %+v", last)
	}
}

func TestAuditExportCSVAndAccess(t *testing.T) {
	e := newAuditEnv(t)
	e.wsCase()
	rec := rawGet(t, e, "/v1/audit/export?format=csv", user("carol"))
	if rec.Code != http.StatusOK {
		t.Fatalf("csv = %d", rec.Code)
	}
	rows, err := csv.NewReader(strings.NewReader(rec.Body.String())).ReadAll()
	if err != nil || len(rows) < 2 || rows[0][1] != "seq" || rows[0][len(rows[0])-1] != "hash" {
		t.Fatalf("csv rows %v err %v", rows, err)
	}
	for name, h := range map[string]map[string]string{"viewer": user("vera"), "editor": user("dave"), "anonymous": {}} {
		if rec := rawGet(t, e, "/v1/audit/export", h); rec.Code != http.StatusForbidden && rec.Code != http.StatusUnauthorized {
			t.Errorf("%s export = %d", name, rec.Code)
		}
	}
	for _, q := range []string{"?format=xml", "?from=yesterday", "?nope=1", "?tenant=ws:" + e.wsID} {
		if rec := rawGet(t, e, "/v1/audit/export"+q, user("carol")); rec.Code != http.StatusBadRequest {
			t.Errorf("%s = %d", q, rec.Code)
		}
	}
}

func TestAuditExportIsOffByDefault(t *testing.T) {
	e := newAuditEnvWith(t, true, func(d *Deps) { d.AuditExport = false; d.Retention = nil })
	if rec := rawGet(t, e, "/v1/audit/export", user("carol")); rec.Code != http.StatusNotFound {
		t.Fatalf("export with the flag off = %d", rec.Code)
	}
	if code, _ := e.hs.doAs(t, "GET", "/v1/workspaces/"+e.wsID+"/retention", "", user("carol")); code != http.StatusNotFound {
		t.Fatalf("retention with it off = %d", code)
	}
}

func TestRetentionPolicyRoutes(t *testing.T) {
	e := newAuditEnv(t)
	path := "/v1/workspaces/" + e.wsID + "/retention"
	if code, out := e.hs.doAs(t, "GET", path, "", user("vera")); code != http.StatusOK || out["case_days"] != float64(0) {
		t.Fatalf("viewer GET = %d %v", code, out)
	}
	if code, _ := e.hs.doAs(t, "PUT", path, `{"case_days":30}`, user("dave")); code != http.StatusForbidden {
		t.Fatalf("editor PUT = %d", code)
	}
	for _, body := range []string{`{"case_days":3}`, `{"case_days":9999}`, `{}`, `{"case_days":"30"}`} {
		if code, _ := e.hs.doAs(t, "PUT", path, body, user("carol")); code != http.StatusBadRequest {
			t.Errorf("PUT %s = %d", body, code)
		}
	}
	if code, out := e.hs.doAs(t, "PUT", path, `{"case_days":30}`, user("carol")); code != http.StatusOK || out["case_days"] != float64(30) {
		t.Fatalf("admin PUT = %d %v", code, out)
	}
	if p, ok, _ := e.ret.Policy(context.Background(), e.wsID); !ok || p.CaseDays != 30 {
		t.Fatalf("stored %+v %v", p, ok)
	}
	// Another workspace is hidden, not forbidden.
	if code, _ := e.hs.doAs(t, "GET", "/v1/workspaces/"+e.zed+"/retention", "", user("carol")); code != http.StatusNotFound {
		t.Fatalf("other workspace = %d", code)
	}
	evs := e.events(e.tenant())
	var saw bool
	for _, ev := range evs {
		saw = saw || (ev.Action == audit.WorkspaceRetention && ev.Detail == "expire" && ev.Phase == audit.PhaseCompletion)
	}
	if !saw {
		t.Fatal("no workspace.retention completion")
	}
}

func TestDeletingAWorkspaceCaseLeavesATombstone(t *testing.T) {
	e := newAuditEnv(t)
	id := e.wsCase()
	if code, _ := e.hs.doAs(t, "DELETE", "/v1/cases/"+id, "", user("dave")); code != http.StatusNoContent {
		t.Fatalf("delete = %d", code)
	}
	code, out := e.hs.doAs(t, "GET", "/v1/workspaces/"+e.wsID+"/tombstones", "", user("carol"))
	if code != http.StatusOK {
		t.Fatalf("tombstones = %d %v", code, out)
	}
	ts := out["tombstones"].([]any)
	if len(ts) != 1 || ts[0].(map[string]any)["case_id"] != id || ts[0].(map[string]any)["reason"] != retention.ReasonUser {
		t.Fatalf("tombstones %v", ts)
	}
	if code, _ := e.hs.doAs(t, "GET", "/v1/workspaces/"+e.wsID+"/tombstones", "", user("vera")); code != http.StatusForbidden {
		t.Fatalf("viewer tombstones = %d", code)
	}
}
