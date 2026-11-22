package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/jurisdiction"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/tariffshift"
)

func TestCanadaMarkingEndpoint(t *testing.T) {
	rules, err := tariffshift.CanadaRules()
	if err != nil {
		t.Fatal(err)
	}
	hs := newHarnessWith(t, "substantial_transformation", func(d *Deps) { d.CanadaMarking = rules })

	// Codes and origins sent directly.
	body, _ := json.Marshal(map[string]any{"good_hts": "6301.30", "country": "Mexico",
		"materials": []map[string]any{{"name": "cotton yarn", "hts": "5205.11", "origin": "China"}}})
	code, out := hs.do(t, "POST", "/v1/rules/canada-marking", string(body))
	if code != http.StatusOK || out["jurisdiction"] != "ca" || out["label"] != CanadaLabel {
		t.Fatalf("%d %v", code, out)
	}
	res := out["result"].(map[string]any)
	if res["status"] != "decided" || res["origin"] != "Mexico" || res["rule"].(map[string]any)["id"] != "SOR/94-23 Sch. III, 6301.20-6301.90" {
		t.Fatalf("result %v", res)
	}

	// A product, read like the case's rule check.
	body, _ = json.Marshal(map[string]any{"product": map[string]any{"name": "Blanket", "hts": "6301.30",
		"plants":     []map[string]any{{"id": "mx", "country": "Mexico"}},
		"components": []map[string]any{{"name": "cotton yarn", "hts": "5205.11", "made_in": "China", "plant": "mx"}},
		"operations": []map[string]any{{"name": "weave", "plant": "mx", "group": "final_assembly"}}}})
	code, out = hs.do(t, "POST", "/v1/rules/canada-marking", string(body))
	if code != http.StatusOK || out["country"] != "Mexico" || out["result"].(map[string]any)["origin"] != "Mexico" {
		t.Fatalf("product: %d %v", code, out)
	}

	for _, bad := range []string{`{}`, `{"good_hts":"6301.30","unknown":1}`} {
		if code, _ := hs.do(t, "POST", "/v1/rules/canada-marking", bad); code != http.StatusBadRequest {
			t.Errorf("%s: %d", bad, code)
		}
	}
}

func TestCanadaMarkingOffWithoutRules(t *testing.T) {
	hs := newHarness(t, "substantial_transformation")
	if code, _ := hs.do(t, "POST", "/v1/rules/canada-marking", `{"good_hts":"6301.30"}`); code != http.StatusNotFound {
		t.Fatalf("route served without rules: %d", code)
	}
}

func TestCanadaMarkingMCPTool(t *testing.T) {
	rules, err := tariffshift.CanadaRules()
	if err != nil {
		t.Fatal(err)
	}
	hs := newHarnessWith(t, "substantial_transformation", func(d *Deps) { d.CanadaMarking = rules })
	cs := mcpClient(t, hs, nil)
	res, out := callTool(t, cs, "check_canada_marking", map[string]any{"good_hts": "6301.30", "country": "Mexico",
		"materials": []any{map[string]any{"name": "cotton yarn", "hts": "5205.11", "origin": "China"}}})
	if res.IsError || !strings.Contains(text(res), "Canada marking origin: Mexico") || !strings.Contains(text(res), "no accuracy figure") {
		t.Fatalf("%s %v", text(res), out)
	}
	// Listed only when the route is served.
	plain := mcpClient(t, newHarness(t, "substantial_transformation"), nil)
	tl, err := plain.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, tool := range tl.Tools {
		if tool.Name == "check_canada_marking" {
			t.Fatal("tool listed without the route")
		}
	}
}

// Canada is registered as a rules-only market (D725): a Canadian case gets
// the SOR/94-23 answer from its codes, is always reviewed, and calls no
// model; without codes it gets no origin and says what to add.
func TestCanadaRegisteredRulesOnly(t *testing.T) {
	rules, err := tariffshift.CanadaRules()
	if err != nil {
		t.Fatal(err)
	}
	reg, err := jurisdiction.NewRegistry(jurisdiction.USAuthority, CanadaAuthority(rules))
	if err != nil {
		t.Fatal(err)
	}
	hs := newHarnessWith(t, RegimeGeneral, func(d *Deps) { d.Jurisdictions, d.CanadaMarking = reg, rules })
	body, _ := json.Marshal(map[string]any{"date": "2026-11-01", "jurisdiction": "ca", "family": "motors_pumps_compressors",
		"product": map[string]any{"name": "Compressor housing", "family": "motors_pumps_compressors", "hts": "8414.90",
			"plants":     []map[string]any{{"id": "mx", "country": "Mexico"}, {"id": "cn", "country": "China"}},
			"components": []map[string]any{{"name": "rough casting", "made_in": "China", "plant": "cn", "hts": "8414.90"}},
			"operations": []map[string]any{{"name": "machine", "plant": "mx", "group": "final_assembly"}}}})
	code, out := hs.do(t, "POST", "/v1/cases/predict", string(body))
	if code != http.StatusCreated {
		t.Fatalf("%d %v", code, out)
	}
	p := out["prediction"].(map[string]any)
	if p["origin_country"] != "Mexico" || !strings.HasPrefix(p["reasoning"].(string), CanadaLabel) || hs.engine.calls != 0 {
		t.Fatalf("prediction %v, U.S. engine calls %d", p, hs.engine.calls)
	}
	if r := out["triage"].(map[string]any)["route"]; r != "needs_review" {
		t.Errorf("route %v", r)
	}
	if detail(t, out)["supported"] != true {
		t.Errorf("detail %v", out["jurisdiction_detail"])
	}
	code, out = hs.do(t, "POST", "/v1/cases/predict", marketBody("A compressor housing machined in Mexico from a Chinese casting, shipped to Canada.", "ca"))
	if code != http.StatusCreated || out["prediction"].(map[string]any)["origin_country"] != "" {
		t.Fatalf("no codes: %d %v", code, out["prediction"])
	}
}
