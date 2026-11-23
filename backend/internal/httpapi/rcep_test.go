package httpapi

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
)

func TestRCEPCheckEndpoint(t *testing.T) {
	hs := newHarness(t, "substantial_transformation")
	// ABF guide p.42: spice mix 0910.91 made in Malaysia, RVC 44.4 %.
	body := `{"hs":"0910.91","produced_in":"Malaysia","fob":18,
	 "costs":{"labour":4,"overhead":0,"profit":2.7,"other":1.3},
	 "materials":[
	  {"name":"vanilla pods","hs":"0905.10","value":5,"country":"Madagascar","status":"non_originating"},
	  {"name":"cinnamon bark","hs":"0906.11","value":5,"country":"Sri Lanka","status":"non_originating"}]}`
	code, out := hs.do(t, "POST", "/v1/rcep/check", body)
	if code != http.StatusOK || out["outcome"] != "yes" || out["applied"] != "RVC40" || out["regime"] != "rcep_preferential" {
		t.Fatalf("check = %d %v", code, out)
	}
	if f := out["failing"].([]any); len(f) != 0 {
		t.Errorf("a yes lists failing parts: %v", f)
	}
	if !strings.Contains(out["note"].(string), "Not marking origin") {
		t.Errorf("note = %v", out["note"])
	}
}

func TestRCEPCheckRejectsBadInput(t *testing.T) {
	hs := newHarness(t, "substantial_transformation")
	for _, body := range []string{
		`{"produced_in":"Japan"}`,
		`{"hs":"8471.30","produced_in":"Japan","materials":[{"hs":"8471","status":"maybe"}]}`,
		`{"hs":"8471.30","produced_in":"Japan","unknown_field":1}`,
		`{"hs":"8471.30","fob":-5}`,
	} {
		code, out := hs.do(t, "POST", "/v1/rcep/check", body)
		if code != http.StatusBadRequest || out["code"] != "invalid_input" {
			t.Errorf("%s: %d %v", body, code, out)
		}
	}
	big := `{"hs":"8471.30","produced_in":"Japan","materials":[` + strings.TrimSuffix(strings.Repeat(`{"hs":"8471"},`, 201), ",") + `]}`
	if code, _ := hs.do(t, "POST", "/v1/rcep/check", big); code != http.StatusBadRequest {
		t.Errorf("201 materials: %d", code)
	}
}

func TestRCEPCheckOverMCP(t *testing.T) {
	hs := newHarness(t, "substantial_transformation")
	cs := mcpClient(t, hs, nil)
	// ABF guide p.41: the cinnamon ground in Vietnam is not originating, so
	// the spice mix fails CC and reaches only 38.9 % RVC.
	res, out := callTool(t, cs, "check_rcep_preference", map[string]any{
		"hs": "0910.91", "produced_in": "Malaysia", "fob": 18,
		"costs": map[string]any{"labour": 3.5, "profit": 2.5, "other": 1},
		"materials": []any{
			map[string]any{"name": "vanilla pods", "hs": "0905.10", "value": 5, "country": "Madagascar", "status": "non_originating"},
			map[string]any{"name": "ground cinnamon", "hs": "0906.20", "value": 6, "country": "Vietnam", "status": "non_originating"},
		},
	})
	if res.IsError || out["outcome"] != "no" || !strings.Contains(text(res), "Does not qualify") || !strings.Contains(text(res), "Not marking origin") {
		t.Fatalf("check_rcep_preference: %s %v", text(res), out)
	}
	if res, _ := callTool(t, cs, "check_rcep_preference", map[string]any{"hs": "0910.91", "produced_in": "Malaysia", "materials": []any{map[string]any{"status": "maybe"}}}); !res.IsError {
		t.Error("a bad status was accepted")
	}
}

// #671: a 4- or 5-digit good under a CTSH heading used to panic and drop the
// HTTP and MCP connections. It must answer undetermined instead.
func TestRCEPCheckShortGoodNeverDropsConnection(t *testing.T) {
	hs := newHarness(t, "substantial_transformation")
	for _, body := range []string{
		`{"hs":"2811","produced_in":"Japan","materials":[{"hs":"2804.10","status":"non_originating"}]}`,
		`{"hs":"38081","produced_in":"Vietnam","materials":[{"hs":"2903.11","status":"non_originating"}]}`,
	} {
		code, out := hs.do(t, "POST", "/v1/rcep/check", body)
		if code != http.StatusOK || out["outcome"] != "undetermined" {
			t.Errorf("%s: got %d %v", body, code, out)
		}
		if !strings.Contains(fmt.Sprint(out["reasons"]), "6-digit") {
			t.Errorf("%s: reasons do not name the missing digits: %v", body, out["reasons"])
		}
	}
	cs := mcpClient(t, hs, nil)
	res, out := callTool(t, cs, "check_rcep_preference", map[string]any{
		"hs": "2811", "produced_in": "Japan",
		"materials": []any{map[string]any{"hs": "2804.10", "status": "non_originating"}},
	})
	if res.IsError || out["outcome"] != "undetermined" || !strings.Contains(text(res), "6-digit") {
		t.Fatalf("check_rcep_preference: %s %v", text(res), out)
	}
}

// #672: finite but overflowing inputs used to encode Inf and get a 500.
func TestRCEPCheckRejectsNonFiniteComputedShares(t *testing.T) {
	hs := newHarness(t, "substantial_transformation")
	for _, body := range []string{
		`{"hs":"8471.30","produced_in":"Japan","fob":1e-300,"materials":[{"hs":"8471.30","value":1e10,"status":"non_originating"}]}`,
		`{"hs":"8471.30","produced_in":"Japan","fob":1e308,"materials":[{"hs":"8471.30","value":1e308,"status":"non_originating"},{"hs":"8471.30","value":1e308,"status":"non_originating"}]}`,
		`{"hs":"8471.30","produced_in":"Japan","fob":100,"costs":{"labour":1e308,"overhead":1e308,"profit":1e308,"other":1e308},"materials":[{"hs":"8471.50","value":10,"status":"non_originating"}]}`,
	} {
		code, out := hs.do(t, "POST", "/v1/rcep/check", body)
		if code != http.StatusBadRequest || out["code"] != "invalid_input" {
			t.Errorf("%s: got %d %v", body, code, out)
		}
	}
}
