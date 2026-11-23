package httpapi

import (
	"errors"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/usmca"
)

const usmcaProfile = `"profile":{"simple_goods_confirmed":true,"materials_list_complete":true,
 "sets":"not_applicable","accessories_packing":"not_applicable","fungibles":"not_applicable",
 "intermediate_materials":"not_applicable","cost_averaging":"not_applicable","de_minimis":"not_applicable",
 "outside_party_operations":"not_applicable","special_valuation":"not_applicable","parts_route":"not_applicable"}`

func usmcaBody(goodHS, hs string) string {
	return `{"destination":"US","regime":"usmca","good_hs":"` + goodHS + `","rvc_method":"transaction_value",
	 "transaction_value":"100","vnm":"40",
	 "materials":[{"id":"m1","hs":"` + hs + `","originating":"non_originating"}],` + usmcaProfile + `}`
}

func TestUSMCACheckSuccessShape(t *testing.T) {
	hs := newHarness(t, "substantial_transformation")
	code, out := hs.do(t, "POST", "/v1/usmca/check", usmcaBody("820231", "880100"))
	if code != http.StatusOK {
		t.Fatalf("%d %v", code, out)
	}
	if out["status"] != "met" || out["destination"] != "US" || out["regime"] != "usmca" || out["good_hs"] != "820231" {
		t.Errorf("shape: %v", out)
	}
	for _, k := range []string{"alternatives", "source", "limits"} {
		if out[k] == nil {
			t.Errorf("missing %q", k)
		}
	}
	lim, _ := out["limits"].([]any)
	if len(lim) == 0 || !strings.Contains(lim[0].(string), "not a certification") {
		t.Errorf("limits = %v", out["limits"])
	}
	for _, banned := range []string{"duty_free", "tariff_rate", "originating_status", "certified"} {
		if _, ok := out[banned]; ok {
			t.Errorf("result carries %q", banned)
		}
	}
}

// Dotted HS codes are normalised to plain digits before the engine.
func TestUSMCACheckNormalisesDottedHS(t *testing.T) {
	hs := newHarness(t, "substantial_transformation")
	_, plain := hs.do(t, "POST", "/v1/usmca/check", usmcaBody("8202310000", "880100"))
	code, dotted := hs.do(t, "POST", "/v1/usmca/check", usmcaBody("8202.31.00", "8801.00"))
	if code != http.StatusOK {
		t.Fatalf("%d %v", code, dotted)
	}
	if dotted["good_hs"] != "82023100" {
		t.Errorf("good_hs = %v", dotted["good_hs"])
	}
	if dotted["status"] != plain["status"] {
		t.Errorf("dotted %v vs plain %v", dotted["status"], plain["status"])
	}
	_, dotted10 := hs.do(t, "POST", "/v1/usmca/check", usmcaBody("8202.31.0000", "880100"))
	if dotted10["good_hs"] != "8202310000" {
		t.Errorf("good_hs = %v", dotted10["good_hs"])
	}
}

func TestUSMCACheckRejectsBadInput(t *testing.T) {
	hs := newHarness(t, "substantial_transformation")
	ok := usmcaBody("820231", "880100")
	for name, body := range map[string]string{
		"number amount":      strings.Replace(ok, `"transaction_value":"100"`, `"transaction_value":100`, 1),
		"negative amount":    strings.Replace(ok, `"vnm":"40"`, `"vnm":"-1"`, 1),
		"exponent amount":    strings.Replace(ok, `"vnm":"40"`, `"vnm":"4e1"`, 1),
		"unknown field":      strings.Replace(ok, `"regime"`, `"extra":1,"regime"`, 1),
		"wrong destination":  strings.Replace(ok, `"US"`, `"CA"`, 1),
		"wrong regime":       strings.Replace(ok, `"usmca"`, `"rcep"`, 1),
		"malformed good hs":  usmcaBody("8202.3", "880100"),
		"letters good hs":    usmcaBody("82O231", "880100"),
		"dotted short good":  usmcaBody("82.02", "880100"),
		"bad material hs":    usmcaBody("820231", "88.0"),
		"bad method":         strings.Replace(ok, `"rvc_method":"transaction_value"`, `"rvc_method":"cheapest"`, 1),
		"bad claim":          strings.Replace(ok, `"non_originating"`, `"maybe"`, 1),
		"bad provision":      strings.Replace(ok, `"sets":"not_applicable"`, `"sets":"no"`, 1),
		"trailing data":      ok + ` {}`,
		"not json":           `nope`,
		"empty":              ``,
		"materials not list": strings.Replace(ok, `"materials":[`, `"materials":{"a":[`, 1),
	} {
		code, out := hs.do(t, "POST", "/v1/usmca/check", body)
		if code != http.StatusBadRequest || out["code"] != "invalid_input" {
			t.Errorf("%s: %d %v", name, code, out)
		}
	}
	big := `{"destination":"US","regime":"usmca","good_hs":"820231","materials":[` +
		strings.TrimSuffix(strings.Repeat(`{"hs":"880100"},`, 201), ",") + `],` + usmcaProfile + `}`
	if code, _ := hs.do(t, "POST", "/v1/usmca/check", big); code != http.StatusBadRequest {
		t.Errorf("201 materials: %d", code)
	}
	atCap := strings.Replace(big, strings.Repeat(`{"hs":"880100"},`, 1), "", 1)
	if code, _ := hs.do(t, "POST", "/v1/usmca/check", atCap); code != http.StatusOK {
		t.Errorf("200 materials: %d", code)
	}
	huge := `{"destination":"US","regime":"usmca","good_hs":"820231","pad":"` + strings.Repeat("x", maxBody+10) + `"}`
	if code, _ := hs.do(t, "POST", "/v1/usmca/check", huge); code != http.StatusBadRequest && code != http.StatusRequestEntityTooLarge {
		t.Errorf("oversized body: %d", code)
	}
}

// Unsupported goods and missing facts are results, not errors.
func TestUSMCACheckNotSupportedAndNeedsInformation(t *testing.T) {
	hs := newHarness(t, "substantial_transformation")
	code, out := hs.do(t, "POST", "/v1/usmca/check", usmcaBody("940360", "880100"))
	if code != http.StatusOK || out["status"] != "not_supported" {
		t.Errorf("unsupported good: %d %v", code, out["status"])
	}
	code, out = hs.do(t, "POST", "/v1/usmca/check", `{"destination":"US","regime":"usmca","good_hs":"820231"}`)
	if code != http.StatusOK || out["status"] == "met" || out["status"] == "not_met" {
		t.Errorf("missing facts: %d %v", code, out["status"])
	}
}

// No model is called and no case is stored, with working or failing
// providers.
func TestUSMCACheckCallsNoProviderAndStoresNothing(t *testing.T) {
	for name, fail := range map[string]bool{"working": false, "failing": true} {
		t.Run(name, func(t *testing.T) {
			hs := newHarnessWith(t, "substantial_transformation", func(d *Deps) {
				if fail {
					d.Router = failingRouter{}
				}
			})
			if fail {
				hs.engine.err = errors.New("provider down")
			}
			code, out := hs.do(t, "POST", "/v1/usmca/check", usmcaBody("820231", "880100"))
			if code != http.StatusOK || out["status"] != "met" {
				t.Fatalf("%d %v", code, out)
			}
			if hs.engine.calls != 0 || hs.agent.calls != 0 || hs.extract.tm != nil || len(hs.clar.facts) != 0 {
				t.Errorf("a provider was called: engine=%d agent=%d", hs.engine.calls, hs.agent.calls)
			}
			code, list := hs.do(t, "GET", "/v1/cases", "")
			if code == http.StatusOK {
				if cs, _ := list["cases"].([]any); len(cs) != 0 {
					t.Errorf("a case was stored: %v", cs)
				}
			}
		})
	}
}

// The route answers exactly what the engine computes.
func TestUSMCACheckMatchesEngine(t *testing.T) {
	hs := newHarness(t, "substantial_transformation")
	body := usmcaBody("820231", "880100")
	in, err := usmca.DecodeInput([]byte(body))
	if err != nil {
		t.Fatal(err)
	}
	want, err := usmca.Check(in)
	if err != nil {
		t.Fatal(err)
	}
	_, got := hs.do(t, "POST", "/v1/usmca/check", body)
	if got["status"] != string(want.Status) || got["row_id"] != want.RowID {
		t.Errorf("got %v want %s %s", got["status"], want.Status, want.RowID)
	}
}

func TestUSMCACheckOverMCPMatchesREST(t *testing.T) {
	hs := newHarness(t, "substantial_transformation")
	cs := mcpClient(t, hs, nil)
	args := map[string]any{
		"destination": "US", "regime": "usmca", "good_hs": "8202.31", "rvc_method": "transaction_value",
		"transaction_value": "100", "vnm": "40",
		"materials": []any{map[string]any{"id": "m1", "hs": "8801.00", "originating": "non_originating"}},
		"profile": map[string]any{
			"simple_goods_confirmed": true, "materials_list_complete": true,
			"sets": "not_applicable", "accessories_packing": "not_applicable", "fungibles": "not_applicable",
			"intermediate_materials": "not_applicable", "cost_averaging": "not_applicable", "de_minimis": "not_applicable",
			"outside_party_operations": "not_applicable", "special_valuation": "not_applicable", "parts_route": "not_applicable",
		},
	}
	res, out := callTool(t, cs, "check_usmca_origin_rule", args)
	if res.IsError || out["status"] != "met" {
		t.Fatalf("%s %v", text(res), out)
	}
	if !strings.Contains(text(res), "selected rule") || !strings.Contains(text(res), "not a certification") {
		t.Errorf("text = %s", text(res))
	}
	_, rest := hs.do(t, "POST", "/v1/usmca/check", usmcaBody("8202.31", "8801.00"))
	for _, k := range []string{"status", "destination", "regime", "good_hs", "row_id", "alternatives", "source", "limits"} {
		if !reflect.DeepEqual(out[k], rest[k]) {
			t.Errorf("%s differs: mcp %v rest %v", k, out[k], rest[k])
		}
	}
	// A JSON number for an amount is refused on the MCP path too.
	args["vnm"] = 40
	if res, _ := callTool(t, cs, "check_usmca_origin_rule", args); !res.IsError {
		t.Error("a numeric amount was accepted")
	}
	args["vnm"] = "40"
	args["regime"] = "rcep"
	if res, _ := callTool(t, cs, "check_usmca_origin_rule", args); !res.IsError {
		t.Error("a wrong regime was accepted")
	}
}
