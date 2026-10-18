package httpapi

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/llm"
)

func TestImportFillsMoneyFromBOM(t *testing.T) {
	hs := newHarness(t, "substantial_transformation")
	csv := "Part,Made in,Factory,Operation group,Plant capabilities,Qty,Unit cost (USD),Annual units,Unit price (USD),HTS\n" +
		"main housing,Thailand,Rayong plant,housing_moulding,motor_fan_assembly,1,2,\"10,000\",$30,8419.89.9500\n" +
		"electric motor,China,China suppliers,motor_fan_assembly,,1,5,,,\n"
	body, _ := json.Marshal(map[string]any{"name": "Heat gun", "family": "motorised_tools_small_appliances", "csv": csv})
	code, out := hs.do(t, "POST", "/v1/products/import", string(body))
	if code != http.StatusOK {
		t.Fatalf("import = %d %v", code, out)
	}
	if out["product"].(map[string]any)["hts"] != "8419.89.95" {
		t.Errorf("hts = %v", out["product"])
	}
	e := out["economics"].(map[string]any)
	gc := e["group_costs"].(map[string]any)
	if e["annual_import_value_usd"] != 300000.0 || gc["housing_moulding"].(map[string]any)["rayong_plant"] != 20000.0 ||
		gc["motor_fan_assembly"].(map[string]any)["china_suppliers"] != 50000.0 || e["illustrative"] != true {
		t.Errorf("economics = %v", e)
	}
	src := out["economics_sources"].(map[string]any)
	if src["hts"] != "file" || src["annual_import_value_usd"] != "file" || src["group_costs"] != "file" {
		t.Errorf("sources = %v", src)
	}
	// A BOM without money columns answers as before.
	_, plain := hs.do(t, "POST", "/v1/products/import", `{"name":"Fan","csv":"Part,Country\nmotor,China\n"}`)
	if _, ok := plain["economics"]; ok {
		t.Errorf("plain import has economics: %v", plain)
	}
	if _, ok := plain["economics_sources"]; ok {
		t.Errorf("plain import has sources: %v", plain)
	}
}

const prefillProduct = `{"name":"Heat gun","hts":"8419.89.95","plants":[{"id":"cn","name":"China suppliers","country":"China"},{"id":"th","name":"Rayong","country":"Thailand"}],` +
	`"components":[{"name":"motor","made_in":"China","plant":"cn","group":"motor_fan_assembly"}],"operations":[]}`

func TestMoneyPrefillFillsDutyFromTable(t *testing.T) {
	hs := newHarness(t, "substantial_transformation")
	code, out := hs.do(t, "POST", "/v1/products/money-prefill", `{"product":`+prefillProduct+`,"economics":{"duty_rates":{"Thailand":0.05}}}`)
	if code != http.StatusOK {
		t.Fatalf("prefill = %d %v", code, out)
	}
	e := out["economics"].(map[string]any)
	rates := e["duty_rates"].(map[string]any)
	src := out["duty_sources"].(map[string]any)
	if out["hts"] != "8419.89.95" || out["hts_source"] != "product" || rates["China"] == nil || src["China"] != "table" {
		t.Errorf("prefill = %v", out)
	}
	// The user's rate wins, and the import value stays unset.
	if rates["Thailand"] != 0.05 || src["Thailand"] != "given" || e["annual_import_value_usd"] != 0.0 {
		t.Errorf("user's values changed: %v / %v", e, src)
	}
}

func TestMoneyPrefillWithoutHTS(t *testing.T) {
	hs := newHarness(t, "substantial_transformation") // no suggester
	noHTS := `{"name":"Heat gun","plants":[{"id":"cn","country":"China"}],"components":[{"name":"motor","made_in":"China","plant":"cn"}],"operations":[]}`
	code, out := hs.do(t, "POST", "/v1/products/money-prefill", `{"product":`+noHTS+`}`)
	if code != http.StatusOK || out["hts"] != nil || len(out["economics"].(map[string]any)["duty_rates"].(map[string]any)) != 0 {
		t.Errorf("no hts = %d %v", code, out)
	}
	if code, _ := hs.do(t, "POST", "/v1/products/money-prefill", `{}`); code != http.StatusBadRequest {
		t.Errorf("no product = %d", code)
	}
}

func TestMoneyPrefillSuggestsHTS(t *testing.T) {
	m := &llm.Fake{Respond: func(req llm.Request) (llm.Response, error) {
		items := req.JSONSchema["properties"].(map[string]any)["candidates"].(map[string]any)["items"].(map[string]any)["properties"].(map[string]any)
		code := items["hts"].(map[string]any)["enum"].([]string)[0]
		return llm.Response{Text: `{"candidates":[{"hts":"` + code + `","reason":"Similar goods.","cited":[]}]}`}, nil
	}}
	hs := htsHarness(t, m, nil)
	noHTS := `{"name":"Heat gun","description":"hand-held electric heat gun with a motor, fan and heating element","plants":[{"id":"cn","country":"China"}],` +
		`"components":[{"name":"motor","made_in":"China","plant":"cn"},{"name":"heating element","made_in":"China","plant":"cn"}],"operations":[]}`
	code, out := hs.do(t, "POST", "/v1/products/money-prefill", `{"product":`+noHTS+`}`)
	if code != http.StatusOK || out["hts_source"] != "suggested" || out["hts"] == nil || out["hts_suggestion"] == nil || out["hts_note"] == nil {
		t.Errorf("suggested = %d %v", code, out)
	}
}
