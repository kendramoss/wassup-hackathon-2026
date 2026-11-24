package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/euorigin"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/jurisdiction"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/nporigin"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/ukorigin"
)

func originRules(t *testing.T) (*nporigin.Rules, *nporigin.Rules) {
	t.Helper()
	eu, err := euorigin.Rules()
	if err != nil {
		t.Fatal(err)
	}
	uk, err := ukorigin.Rules()
	if err != nil {
		t.Fatal(err)
	}
	return eu, uk
}

// Synthetic requests (D766): a radio assembled in Malaysia, a laptop the
// lists do not cover.
func TestOriginRulesEndpoints(t *testing.T) {
	eu, uk := originRules(t)
	hs := newHarnessWith(t, "substantial_transformation", func(d *Deps) { d.EUOrigin, d.UKOrigin = eu, uk })
	radio, _ := json.Marshal(map[string]any{"good_hts": "8527.13", "country": "Malaysia",
		"materials": []map[string]any{{"name": "board", "hts": "8534.00", "origin": "China"}}})
	for _, c := range []struct{ path, market, sha string }{
		{"/v1/rules/eu-origin", "eu", "87be4f1a432bc0ef35a6c27a94d237a65fc41d036278cf43fd4889a4f79818be"},
		{"/v1/rules/uk-origin", "uk", "239b6b3d103dd4d22bfc639638759bb41a6ba98233540e150c79b912d22b850d"},
	} {
		code, out := hs.do(t, "POST", c.path, string(radio))
		if code != http.StatusOK || out["jurisdiction"] != c.market || out["label"] != OriginRulesLabel {
			t.Fatalf("%s: %d %v", c.path, code, out)
		}
		res := out["result"].(map[string]any)
		if res["status"] != "decided" || res["origin"] != "Malaysia" || res["rule"].(map[string]any)["code"] != "8527" {
			t.Fatalf("%s: result %v", c.path, res)
		}
		if src := out["source"].(map[string]any); src["sha256"] != c.sha || src["version"] == "" {
			t.Errorf("%s: source %v", c.path, src)
		}
		if len(out["chapters"].([]any)) != 37 || out["administration"] == "" {
			t.Errorf("%s: %v %v", c.path, out["chapters"], out["administration"])
		}
		// Not in the list: a judgement, never an origin.
		code, out = hs.do(t, "POST", c.path, `{"good_hts":"8471.30","country":"Vietnam","materials":[{"hts":"8473.30","origin":"China"}]}`)
		if res := out["result"].(map[string]any); code != http.StatusOK || res["status"] != "needs_judgement" || res["origin"] != nil {
			t.Fatalf("%s: laptop %d %v", c.path, code, res)
		}
		// A row choice, then the row.
		_, out = hs.do(t, "POST", c.path, `{"good_hts":"8473.30","country":"Taiwan","materials":[{"hts":"8542.32","origin":"South Korea"}]}`)
		needs := out["result"].(map[string]any)["needs"].([]any)
		if needs[0].(map[string]any)["kind"] != "row" {
			t.Fatalf("%s: needs %v", c.path, needs)
		}
		_, out = hs.do(t, "POST", c.path, `{"good_hts":"8473.30","country":"Taiwan","row":"8473x","materials":[{"hts":"8542.32","origin":"South Korea"}]}`)
		if out["result"].(map[string]any)["origin"] != "Taiwan" {
			t.Fatalf("%s: row %v", c.path, out["result"])
		}
		for _, bad := range []string{`{}`, `{"good_hts":"8527","unknown":1}`,
			`{"good_hts":"8527","answers":{"simple_assembly":true}}`, `{"good_hts":"8527","row":"9999"}`,
			`{"good_hts":"8527","materials":[{"hts":"8534","origin":"China","value_share":1.5}]}`} {
			if code, _ := hs.do(t, "POST", c.path, bad); code != http.StatusBadRequest {
				t.Errorf("%s %s: %d", c.path, bad, code)
			}
		}
	}
	// A product, read like the case's rule check.
	body, _ := json.Marshal(map[string]any{"product": map[string]any{"name": "Radio", "hts": "8527.13",
		"plants":     []map[string]any{{"id": "my", "country": "Malaysia"}},
		"components": []map[string]any{{"name": "board", "hts": "8534.00", "made_in": "China", "plant": "my"}},
		"operations": []map[string]any{{"name": "assemble", "plant": "my", "group": "final_assembly"}}}})
	code, out := hs.do(t, "POST", "/v1/rules/uk-origin", string(body))
	if code != http.StatusOK || out["country"] != "Malaysia" || out["result"].(map[string]any)["origin"] != "Malaysia" {
		t.Fatalf("product: %d %v", code, out)
	}
}

func TestOriginRulesOffWithoutRules(t *testing.T) {
	hs := newHarness(t, "substantial_transformation")
	for _, p := range []string{"/v1/rules/eu-origin", "/v1/rules/uk-origin"} {
		if code, _ := hs.do(t, "POST", p, `{"good_hts":"8527"}`); code != http.StatusNotFound {
			t.Fatalf("%s served without rules: %d", p, code)
		}
	}
}

func TestOriginRulesMCPTools(t *testing.T) {
	eu, uk := originRules(t)
	hs := newHarnessWith(t, "substantial_transformation", func(d *Deps) { d.EUOrigin, d.UKOrigin = eu, uk })
	cs := mcpClient(t, hs, nil)
	for _, c := range []struct{ tool, want string }{{"check_eu_origin", "EU non-preferential origin: Malaysia"}, {"check_uk_origin", "UK non-preferential origin: Malaysia"}} {
		res, out := callTool(t, cs, c.tool, map[string]any{"good_hts": "8527.13", "country": "Malaysia",
			"materials": []any{map[string]any{"name": "board", "hts": "8534.00", "origin": "China"}}})
		if res.IsError || !strings.Contains(text(res), c.want) || !strings.Contains(text(res), "no accuracy figure") {
			t.Fatalf("%s: %s %v", c.tool, text(res), out)
		}
		res, _ = callTool(t, cs, c.tool, map[string]any{"good_hts": "8473.30", "country": "Taiwan",
			"materials": []any{map[string]any{"hts": "8542.32", "origin": "South Korea"}}})
		if !strings.Contains(text(res), "Pick a row") || !strings.Contains(text(res), "8473x") {
			t.Fatalf("%s: %s", c.tool, text(res))
		}
		res, _ = callTool(t, cs, c.tool, map[string]any{"good_hts": "8471.30", "country": "Vietnam",
			"materials": []any{map[string]any{"hts": "8473.30", "origin": "China"}}})
		if !strings.Contains(text(res), "substantial-transformation judgement") {
			t.Fatalf("%s: %s", c.tool, text(res))
		}
	}
	plain := mcpClient(t, newHarness(t, "substantial_transformation"), nil)
	tl, err := plain.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, tool := range tl.Tools {
		if tool.Name == "check_eu_origin" || tool.Name == "check_uk_origin" {
			t.Fatal("tool listed without the route")
		}
	}
}

// The EU and the UK are registered as rules-only markets (D766): a case gets
// the list-rule answer from its codes, is always reviewed, calls no model
// and has no follow-ups.
func TestOriginRulesRegisteredRulesOnly(t *testing.T) {
	eu, uk := originRules(t)
	reg, err := jurisdiction.NewRegistry(jurisdiction.USAuthority, OriginRulesAuthority(jurisdiction.EU, eu), OriginRulesAuthority(jurisdiction.UK, uk))
	if err != nil {
		t.Fatal(err)
	}
	hs := newHarnessWith(t, RegimeGeneral, func(d *Deps) { d.Jurisdictions, d.EUOrigin, d.UKOrigin = reg, eu, uk })
	for _, market := range []string{"eu", "uk"} {
		body, _ := json.Marshal(map[string]any{"date": "2026-11-01", "jurisdiction": market, "family": "pcba_electronic_devices",
			"product": map[string]any{"name": "Radio", "family": "pcba_electronic_devices", "hts": "8527.13",
				"plants":     []map[string]any{{"id": "my", "country": "Malaysia"}, {"id": "cn", "country": "China"}},
				"components": []map[string]any{{"name": "board", "made_in": "China", "plant": "cn", "hts": "8534.00"}},
				"operations": []map[string]any{{"name": "assemble", "plant": "my", "group": "final_assembly"}}}})
		code, out := hs.do(t, "POST", "/v1/cases/predict", string(body))
		if code != http.StatusCreated {
			t.Fatalf("%s: %d %v", market, code, out)
		}
		p := out["prediction"].(map[string]any)
		if p["origin_country"] != "Malaysia" || !strings.HasPrefix(p["reasoning"].(string), OriginRulesLabel) || hs.engine.calls != 0 {
			t.Fatalf("%s: prediction %v, U.S. engine calls %d", market, p, hs.engine.calls)
		}
		if r := out["triage"].(map[string]any)["route"]; r != "needs_review" {
			t.Errorf("%s: route %v", market, r)
		}
		if d := detail(t, out); d["supported"] != true || d["follow_ups"] != false {
			t.Errorf("%s: detail %v", market, out["jurisdiction_detail"])
		}
		code, out = hs.do(t, "POST", "/v1/cases/predict", marketBody("A radio assembled in Malaysia from Chinese boards.", market))
		if code != http.StatusCreated || out["prediction"].(map[string]any)["origin_country"] != "" {
			t.Fatalf("%s no codes: %d %v", market, code, out["prediction"])
		}
	}
}
