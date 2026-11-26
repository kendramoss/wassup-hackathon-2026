package httpapi

import (
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"testing"
)

func erpBody(t *testing.T, csvPath string, commit bool) string {
	t.Helper()
	b, err := os.ReadFile(csvPath)
	if err != nil {
		t.Fatal(err)
	}
	out, _ := json.Marshal(map[string]any{"format": "generic", "source": "Test ERP", "csv": string(b), "commit": commit})
	return string(out)
}

func erpRows(t *testing.T, out map[string]any) map[string]map[string]any {
	t.Helper()
	rows := map[string]map[string]any{}
	list, _ := out["rows"].([]any)
	for _, r := range list {
		m := r.(map[string]any)
		rows[m["external_id"].(string)] = m
	}
	return rows
}

func TestERPImportDryRunThenIdempotentCommit(t *testing.T) {
	hs, _, _ := teamsHarness(t, func(d *Deps) { d.ERPImport = true })
	const fixture = "../erpimport/testdata/generic.csv"
	path := "/v1/products/erp-import"

	code, out := hs.doAs(t, "POST", path, erpBody(t, fixture, false), user("alice"))
	if code != http.StatusOK {
		t.Fatalf("dry run = %d %v", code, out)
	}
	rows := erpRows(t, out)
	if len(rows) != 2 || rows["FL-50"]["status"] != "new" || rows["FAN-1"]["status"] != "new" {
		t.Fatalf("dry run rows %v", rows)
	}
	if probs, _ := out["problems"].([]any); len(probs) == 0 {
		t.Fatal("BAD-1 (no country) should be reported as a problem")
	}
	if hs.engine.calls != 0 {
		t.Fatalf("dry run called the engine %d times", hs.engine.calls)
	}

	code, out = hs.doAs(t, "POST", path, erpBody(t, fixture, true), user("alice"))
	if code != http.StatusOK {
		t.Fatalf("commit = %d %v", code, out)
	}
	rows = erpRows(t, out)
	if rows["FL-50"]["status"] != "created" || rows["FAN-1"]["status"] != "created" || hs.engine.calls != 2 {
		t.Fatalf("commit rows %v, engine calls %d", rows, hs.engine.calls)
	}
	caseID := rows["FL-50"]["case_id"].(string)
	if code, c := hs.doAs(t, "GET", "/v1/cases/"+caseID, "", user("alice")); code != http.StatusOK || c["id"] != caseID {
		t.Fatalf("created case not readable: %d %v", code, c)
	}
	if code, _ := hs.doAs(t, "GET", "/v1/cases/"+caseID, "", user("mallory")); code != http.StatusNotFound {
		t.Fatalf("another user reads the imported case: %d", code)
	}

	// Same export again: nothing re-predicted.
	code, out = hs.doAs(t, "POST", path, erpBody(t, fixture, true), user("alice"))
	rows = erpRows(t, out)
	if code != http.StatusOK || rows["FL-50"]["status"] != "unchanged" || rows["FAN-1"]["status"] != "unchanged" || hs.engine.calls != 2 {
		t.Fatalf("re-import = %d %v, engine calls %d", code, rows, hs.engine.calls)
	}

	// One component changed: only that assembly is updated, on the same case.
	b, _ := os.ReadFile(fixture)
	changed := strings.Replace(string(b), "FL-50,LED floodlight,9405.42.8440,Mexico,Driver,China", "FL-50,LED floodlight,9405.42.8440,Mexico,Driver,Mexico", 1)
	body, _ := json.Marshal(map[string]any{"format": "generic", "source": "Test ERP", "csv": changed, "commit": true})
	code, out = hs.doAs(t, "POST", path, string(body), user("alice"))
	rows = erpRows(t, out)
	if code != http.StatusOK || rows["FL-50"]["status"] != "updated" || rows["FL-50"]["case_id"] != caseID || rows["FAN-1"]["status"] != "unchanged" || hs.engine.calls != 3 {
		t.Fatalf("changed re-import = %d %v, engine calls %d", code, rows, hs.engine.calls)
	}
	_, c := hs.doAs(t, "GET", "/v1/cases/"+caseID, "", user("alice"))
	if !strings.Contains(c["facts"].(string), "Driver") {
		t.Fatalf("updated case facts: %v", c["facts"])
	}
}

func TestERPImportCommitNeedsAnIdentity(t *testing.T) {
	hs := newHarnessWith(t, "substantial_transformation", func(d *Deps) { d.ERPImport = true })
	code, out := hs.do(t, "POST", "/v1/products/erp-import", erpBody(t, "../erpimport/testdata/generic.csv", true))
	if code != http.StatusUnauthorized {
		t.Fatalf("anonymous commit = %d %v", code, out)
	}
	if code, _ := hs.do(t, "POST", "/v1/products/erp-import", erpBody(t, "../erpimport/testdata/generic.csv", false)); code != http.StatusOK {
		t.Fatalf("anonymous dry run = %d", code)
	}
}

func TestERPImportRejectsBadInput(t *testing.T) {
	hs := newHarnessWith(t, "substantial_transformation", func(d *Deps) { d.ERPImport = true })
	for _, body := range []string{
		`{"format":"oracle","csv":"a,b\n1,2"}`,
		`{"format":"generic","csv":"x,y\n1,2"}`,
		`{"format":"generic","csv":"","unknown":1}`,
		`{"format":"generic","source":"` + strings.Repeat("s", 65) + `","csv":"product_id,component,made_in\nA,B,China"}`,
	} {
		if code, out := hs.do(t, "POST", "/v1/products/erp-import", body); code != http.StatusBadRequest {
			t.Errorf("%s = %d %v, want 400", body, code, out)
		}
	}
}

func TestERPImportOffByDefault(t *testing.T) {
	hs := newHarness(t, "substantial_transformation")
	if code, _ := hs.do(t, "POST", "/v1/products/erp-import", `{"format":"generic","csv":""}`); code != http.StatusNotFound && code != http.StatusMethodNotAllowed {
		t.Fatalf("route registered without ERP_IMPORT: %d", code)
	}
}
