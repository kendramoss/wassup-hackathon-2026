package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/adcvd"
)

func adcvdTable() *adcvd.Table {
	return adcvd.NewTable("test-v1", "2026-11-27", []adcvd.Order{
		{Case: "A-570-900", Type: "AD", Country: "The People's Republic of China", Product: "Diamond Sawblades and Parts Thereof", OrderDate: "2009-11-04", Status: "active",
			HTS:         []string{"8202.39.00.00"},
			ScopeNotice: &adcvd.Source{DocumentNumber: "2026-23253", Date: "2026-11-16", URL: "https://www.federalregister.gov/d/2026-23253"}},
		{Case: "A-583-814", Type: "AD", Country: "Taiwan", Product: "Circular Welded Non-Alloy Steel Pipe", Status: "active", HTS: []string{"7306.30"}},
	})
}

func adcvdHarness(t *testing.T) harness {
	return newHarnessWith(t, "substantial_transformation", func(d *Deps) { d.ADCVD = adcvdTable() })
}

func TestADCVDCheckEndpoint(t *testing.T) {
	hs := adcvdHarness(t)
	code, out := hs.do(t, "POST", "/v1/adcvd/check", `{"parts":[
	  {"name":"blade","hts":"8202.39.00.00","origin":"China"},
	  {"name":"blade from Germany","hts":"8202.39.00.00","origin":"Germany"},
	  {"name":"no code","origin":"China"}]}`)
	if code != http.StatusOK || out["snapshot"] != "2026-11-27" {
		t.Fatalf("check = %d %v", code, out)
	}
	parts := out["parts"].([]any)
	var status []string
	for _, p := range parts {
		status = append(status, p.(map[string]any)["status"].(string))
	}
	if !reflect.DeepEqual(status, []string{"possible_exposure", "no_match", "not_checked"}) {
		t.Fatalf("statuses = %v", status)
	}
	m := parts[0].(map[string]any)["matches"].([]any)[0].(map[string]any)
	if m["cases"].([]any)[0] != "A-570-900" || m["order_date"] != "2009-11-04" {
		t.Errorf("match = %v", m)
	}
	n := out["notice"].(string)
	if !strings.Contains(n, "written scope") || !strings.Contains(n, "convenience") || !strings.Contains(n, "does not change the origin answer") {
		t.Errorf("notice = %q", n)
	}
}

func TestADCVDCheckRejectsBadInput(t *testing.T) {
	hs := adcvdHarness(t)
	for _, body := range []string{`{}`, `{"parts":[]}`, `{"parts":[{"hts":"8202.39","origin":"China","bogus":1}]}`, `not json`} {
		if code, out := hs.do(t, "POST", "/v1/adcvd/check", body); code != http.StatusBadRequest || out["code"] != "invalid_input" {
			t.Errorf("%s: %d %v", body, code, out)
		}
	}
	big := `{"parts":[` + strings.TrimSuffix(strings.Repeat(`{"hts":"8202","origin":"China"},`, 201), ",") + `]}`
	if code, _ := hs.do(t, "POST", "/v1/adcvd/check", big); code != http.StatusBadRequest {
		t.Errorf("201 parts: %d", code)
	}
}

// Off by default: no route, no MCP tool, and no field on a case.
func TestADCVDOffByDefault(t *testing.T) {
	hs := newHarness(t, "substantial_transformation")
	if code, _ := hs.do(t, "POST", "/v1/adcvd/check", `{"parts":[{"hts":"8202","origin":"China"}]}`); code != http.StatusNotFound {
		t.Fatalf("route served while off: %d", code)
	}
	tl, err := mcpClient(t, hs, nil).ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, tool := range tl.Tools {
		if tool.Name == "check_adcvd_exposure" {
			t.Fatal("tool listed while off")
		}
	}
	code, out := hs.do(t, "POST", "/v1/cases/predict", adcvdCaseBody())
	if code != http.StatusCreated {
		t.Fatalf("predict = %d %v", code, out)
	}
	if _, has := out["adcvd_check"]; has {
		t.Errorf("adcvd_check on a case while off: %v", out["adcvd_check"])
	}
}

func adcvdCaseBody() string {
	p := map[string]any{
		"name": "Stone saw", "hts": "8465.91.00",
		"plants": []map[string]any{{"id": "mx1", "country": "Mexico"}},
		"components": []map[string]any{
			{"name": "diamond blade", "made_in": "China", "hts": "8202.39.00.00"},
			{"name": "steel pipe frame", "made_in": "Taiwan", "hts": "7306.30.5025"},
			{"name": "handle", "made_in": "Vietnam"},
		},
		"operations": []map[string]any{{"name": "assemble the saw", "plant": "mx1", "group": "final_assembly"}},
	}
	b, _ := json.Marshal(map[string]any{"date": "2026-03-01", "family": "pcba_electronic_devices", "product": p})
	return string(b)
}

// The check rides along with a case that gives part codes, and changes
// neither the prediction nor the triage (D939).
func TestADCVDOnACaseIsDisplayOnly(t *testing.T) {
	off := newHarnessWith(t, "substantial_transformation", nil)
	on := adcvdHarness(t)
	_, base := off.do(t, "POST", "/v1/cases/predict", adcvdCaseBody())
	code, got := on.do(t, "POST", "/v1/cases/predict", adcvdCaseBody())
	if code != http.StatusCreated {
		t.Fatalf("predict = %d %v", code, got)
	}
	for _, k := range []string{"prediction", "triage", "router", "economics"} {
		if !reflect.DeepEqual(base[k], got[k]) {
			t.Errorf("%s changed by the AD/CVD check:\n off %v\n on  %v", k, base[k], got[k])
		}
	}
	ac, ok := got["adcvd_check"].(map[string]any)
	if !ok {
		t.Fatalf("adcvd_check missing: %v", got["adcvd_check"])
	}
	parts := ac["parts"].([]any)
	if len(parts) != 3 {
		t.Fatalf("parts = %v", parts)
	}
	status := func(i int) string { return parts[i].(map[string]any)["status"].(string) }
	if status(0) != "possible_exposure" || status(1) != "possible_exposure" || status(2) != "not_checked" {
		t.Errorf("statuses = %s %s %s", status(0), status(1), status(2))
	}
	if parts[0].(map[string]any)["name"] != "diamond blade" {
		t.Errorf("part name = %v", parts[0])
	}
	if !strings.Contains(ac["notice"].(string), "written scope") {
		t.Errorf("notice = %v", ac["notice"])
	}
	// Read back, the same case carries the same check.
	code, again := on.do(t, "GET", "/v1/cases/"+got["id"].(string), "")
	if code != http.StatusOK || again["adcvd_check"] == nil {
		t.Fatalf("GET = %d, adcvd_check %v", code, again["adcvd_check"])
	}
}

// A case without a single part code has nothing to screen: no field.
func TestADCVDNothingToScreen(t *testing.T) {
	on := adcvdHarness(t)
	p := map[string]any{
		"name": "Widget", "plants": []map[string]any{{"id": "mx1", "country": "Mexico"}},
		"components": []map[string]any{{"name": "case", "made_in": "China"}},
		"operations": []map[string]any{{"name": "assemble", "plant": "mx1", "group": "final_assembly"}},
	}
	b, _ := json.Marshal(map[string]any{"date": "2026-03-01", "family": "pcba_electronic_devices", "product": p})
	code, out := on.do(t, "POST", "/v1/cases/predict", string(b))
	if code != http.StatusCreated {
		t.Fatalf("predict = %d %v", code, out)
	}
	if _, has := out["adcvd_check"]; has {
		t.Errorf("adcvd_check with no part codes: %v", out["adcvd_check"])
	}
}

func TestADCVDOverMCP(t *testing.T) {
	cs := mcpClient(t, adcvdHarness(t), nil)
	res, out := callTool(t, cs, "check_adcvd_exposure", map[string]any{
		"parts": []any{map[string]any{"name": "blade", "hts": "8202.39.00.00", "origin": "China"}},
	})
	txt := text(res)
	if res.IsError || !strings.Contains(txt, "Possible exposure") || !strings.Contains(txt, "A-570-900") || !strings.Contains(txt, "written scope") {
		t.Fatalf("%s %v", txt, out)
	}
}
