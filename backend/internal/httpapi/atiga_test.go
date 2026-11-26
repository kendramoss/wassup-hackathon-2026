package httpapi

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

func atigaHarness(t *testing.T) harness {
	return newHarnessWith(t, "substantial_transformation", func(d *Deps) { d.ATIGACheck = true })
}

func TestATIGACheckEndpoint(t *testing.T) {
	hs := atigaHarness(t)
	// 6101.20 (RVC40 or CC and cut and sewn in any AMS or textile process
	// rule): Chinese fabric, cut and sewn in Cambodia.
	body := `{"hs":"6101.20","produced_in":"Cambodia","fob":100,"processes":{"cut_and_sewn":true},
	 "materials":[{"name":"cotton fabric","hs":"5208.12","value":70,"country":"China","status":"non_originating"}]}`
	code, out := hs.do(t, "POST", "/v1/atiga/check", body)
	if code != http.StatusOK || out["outcome"] != "yes" || out["applied"] != "CC and cut and sewn in any AMS" || out["regime"] != "atiga_preferential" {
		t.Fatalf("check = %d %v", code, out)
	}
	if r, _ := out["rule"].(map[string]any); r["listed"] != true || r["code"] != "6101.20" {
		t.Fatalf("rule = %v", out["rule"])
	}
	if !strings.Contains(out["note"].(string), "Not marking origin") {
		t.Errorf("note = %v", out["note"])
	}
}

func TestATIGACheckRejectsBadInput(t *testing.T) {
	hs := atigaHarness(t)
	for _, body := range []string{
		`{"produced_in":"Vietnam"}`,
		`{"hs":"6101.20","produced_in":"Vietnam","method":"both"}`,
		`{"hs":"6101.20","produced_in":"Vietnam","processes":{"sewing":true}}`,
		`{"hs":"6101.20","produced_in":"Vietnam","unknown_field":1}`,
	} {
		code, out := hs.do(t, "POST", "/v1/atiga/check", body)
		if code != http.StatusBadRequest || out["code"] != "invalid_input" {
			t.Errorf("%s: %d %v", body, code, out)
		}
	}
	big := `{"hs":"6101.20","produced_in":"Vietnam","materials":[` + strings.TrimSuffix(strings.Repeat(`{"hs":"5208"},`, 201), ",") + `]}`
	if code, _ := hs.do(t, "POST", "/v1/atiga/check", big); code != http.StatusBadRequest {
		t.Errorf("201 materials: %d", code)
	}
}

// Off by default: no route and no MCP tool.
func TestATIGACheckOffByDefault(t *testing.T) {
	hs := newHarness(t, "substantial_transformation")
	if code, _ := hs.do(t, "POST", "/v1/atiga/check", `{"hs":"6101.20","produced_in":"Vietnam"}`); code != http.StatusNotFound {
		t.Fatalf("route served while off: %d", code)
	}
	tl, err := mcpClient(t, hs, nil).ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, tool := range tl.Tools {
		if tool.Name == "check_atiga_preference" {
			t.Fatal("tool listed while off")
		}
	}
}

func TestATIGACheckOverMCP(t *testing.T) {
	cs := mcpClient(t, atigaHarness(t), nil)
	// 3926.90 is not in Annex 3: the general rule (RVC40 or CTH) applies.
	res, out := callTool(t, cs, "check_atiga_preference", map[string]any{
		"hs": "3926.90", "produced_in": "Thailand", "fob": 100,
		"materials": []any{map[string]any{"name": "resin", "hs": "3901.10", "value": 70, "country": "China"}},
	})
	if res.IsError || !strings.Contains(text(res), "ATIGA preferential origin: Qualifies (CTH)") || !strings.Contains(text(res), "Art. 28.1") {
		t.Fatalf("%s %v", text(res), out)
	}
}
