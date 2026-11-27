package httpapi

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

func aseanFTAHarness(t *testing.T) harness {
	return newHarnessWith(t, "substantial_transformation", func(d *Deps) { d.AKFTACheck = true; d.ACFTACheck = true })
}

func TestAKFTACheckEndpoint(t *testing.T) {
	hs := aseanFTAHarness(t)
	// 1103.11: CC or RVC 40; Australian wheat milled in Vietnam.
	code, out := hs.do(t, "POST", "/v1/akfta/check", `{"hs":"1103.11","produced_in":"Vietnam","fob":100,
	 "materials":[{"name":"wheat","hs":"1001.99","value":70,"country":"Australia"}]}`)
	if code != http.StatusOK || out["outcome"] != "yes" || out["applied"] != "CC" || out["regime"] != "akfta_preferential" {
		t.Fatalf("akfta = %d %v", code, out)
	}
	if r, _ := out["rule"].(map[string]any); r["listed"] != true {
		t.Fatalf("rule = %v", out["rule"])
	}
	// Korea is an AKFTA Party and not an ACFTA one.
	body := `{"hs":"3926.90","produced_in":"Korea","fob":100,"materials":[{"hs":"3901.10","value":10,"country":"Japan"}]}`
	if code, out := hs.do(t, "POST", "/v1/akfta/check", body); code != http.StatusOK || out["outcome"] != "yes" {
		t.Fatalf("akfta Korea = %d %v", code, out)
	}
	if code, out := hs.do(t, "POST", "/v1/acfta/check", body); code != http.StatusOK || out["outcome"] != "no" {
		t.Fatalf("acfta Korea = %d %v", code, out)
	}
}

func TestACFTACheckEndpoint(t *testing.T) {
	hs := aseanFTAHarness(t)
	code, out := hs.do(t, "POST", "/v1/acfta/check", `{"hs":"2902.70","produced_in":"Malaysia","fob":100,
	 "materials":[{"name":"benzene","hs":"2902.20","value":60,"country":"Japan"}]}`)
	if code != http.StatusOK || out["outcome"] != "yes" || out["applied"] != "RVC40" || out["regime"] != "acfta_preferential" {
		t.Fatalf("check = %d %v", code, out)
	}
	if !strings.Contains(out["note"].(string), "HS 2017") {
		t.Errorf("note = %v", out["note"])
	}
	// Build-down only: the direct method is not an ACFTA method.
	if code, out := hs.do(t, "POST", "/v1/acfta/check", `{"hs":"2902.70","produced_in":"Malaysia","method":"direct"}`); code != http.StatusBadRequest || out["code"] != "invalid_input" {
		t.Errorf("direct = %d %v", code, out)
	}
}

func TestASEANFTACheckRejectsBadInput(t *testing.T) {
	hs := aseanFTAHarness(t)
	for _, path := range []string{"/v1/akfta/check", "/v1/acfta/check"} {
		for _, body := range []string{
			`{"produced_in":"Vietnam"}`,
			`{"hs":"1103.11","produced_in":"Vietnam","method":"both"}`,
			`{"hs":"1103.11","produced_in":"Vietnam","processes":{"sewing":true}}`,
			`{"hs":"1103.11","produced_in":"Vietnam","weight":-1}`,
			`{"hs":"1103.11","produced_in":"Vietnam","unknown_field":1}`,
		} {
			code, out := hs.do(t, "POST", path, body)
			if code != http.StatusBadRequest || out["code"] != "invalid_input" {
				t.Errorf("%s %s: %d %v", path, body, code, out)
			}
		}
		big := `{"hs":"1103.11","produced_in":"Vietnam","materials":[` + strings.TrimSuffix(strings.Repeat(`{"hs":"1001"},`, 201), ",") + `]}`
		if code, _ := hs.do(t, "POST", path, big); code != http.StatusBadRequest {
			t.Errorf("%s 201 materials: %d", path, code)
		}
	}
}

// Off by default: no route and no MCP tool; each flag is separate.
func TestASEANFTACheckOffByDefault(t *testing.T) {
	hs := newHarness(t, "substantial_transformation")
	for _, p := range []string{"/v1/akfta/check", "/v1/acfta/check"} {
		if code, _ := hs.do(t, "POST", p, `{"hs":"1103.11","produced_in":"Vietnam"}`); code != http.StatusNotFound {
			t.Fatalf("%s served while off: %d", p, code)
		}
	}
	tl, err := mcpClient(t, hs, nil).ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, tool := range tl.Tools {
		if tool.Name == "check_akfta_preference" || tool.Name == "check_acfta_preference" {
			t.Fatalf("%s listed while off", tool.Name)
		}
	}
	only := newHarnessWith(t, "substantial_transformation", func(d *Deps) { d.AKFTACheck = true })
	if code, _ := only.do(t, "POST", "/v1/acfta/check", `{"hs":"1103.11","produced_in":"Vietnam"}`); code != http.StatusNotFound {
		t.Fatalf("acfta served with only AKFTA_CHECK: %d", code)
	}
}

func TestASEANFTACheckOverMCP(t *testing.T) {
	cs := mcpClient(t, aseanFTAHarness(t), nil)
	res, out := callTool(t, cs, "check_akfta_preference", map[string]any{
		"hs": "3926.90", "produced_in": "Thailand", "fob": 100,
		"materials": []any{map[string]any{"name": "resin", "hs": "3901.10", "value": 70, "country": "China"}},
	})
	if res.IsError || !strings.Contains(text(res), "AKFTA preferential origin: Qualifies (CTH)") || !strings.Contains(text(res), "Rule 4.1") {
		t.Fatalf("%s %v", text(res), out)
	}
	res, out = callTool(t, cs, "check_acfta_preference", map[string]any{
		"hs": "2902.70", "produced_in": "Malaysia", "fob": 100,
		"materials": []any{map[string]any{"name": "benzene", "hs": "2902.20", "value": 60, "country": "Japan"}},
	})
	if res.IsError || !strings.Contains(text(res), "ACFTA preferential origin: Qualifies (RVC40)") {
		t.Fatalf("%s %v", text(res), out)
	}
}
