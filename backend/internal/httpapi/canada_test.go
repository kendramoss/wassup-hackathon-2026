package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
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
	if d := detail(t, out); d["supported"] != true || d["follow_ups"] != false {
		t.Errorf("detail %v", out["jurisdiction_detail"])
	}
	code, out = hs.do(t, "POST", "/v1/cases/predict", marketBody("A compressor housing machined in Mexico from a Chinese casting, shipped to Canada.", "ca"))
	if code != http.StatusCreated || out["prediction"].(map[string]any)["origin_country"] != "" {
		t.Fatalf("no codes: %d %v", code, out["prediction"])
	}
}

// D923: a Canada case carries related published CBSA rulings, for reading
// only; the rules-check answer is the same with or without them.
func TestCanadaCaseShowsCBSARulings(t *testing.T) {
	rules, err := tariffshift.CanadaRules()
	if err != nil {
		t.Fatal(err)
	}
	reg, err := jurisdiction.NewRegistry(jurisdiction.USAuthority, CanadaAuthority(rules))
	if err != nil {
		t.Fatal(err)
	}
	hs := newHarnessWith(t, RegimeGeneral, func(d *Deps) { d.Jurisdictions, d.CanadaMarking = reg, rules })
	product := domain.Product{Name: "Decorative cushion", Family: "furniture", HTS: "9404.90",
		Plants:     []domain.Plant{{ID: "us", Country: "United States"}, {ID: "cn", Country: "China"}},
		Components: []domain.Component{{Name: "polyester cover", MadeIn: "China", Plant: "cn", HTS: "6307.90"}},
		Operations: []domain.Operation{{Name: "fill and close", Plant: "us", Group: "final_assembly"}}}
	body, _ := json.Marshal(map[string]any{"date": "2026-11-01", "jurisdiction": "ca", "family": "furniture", "product": product})
	code, out := hs.do(t, "POST", "/v1/cases/predict", string(body))
	if code != http.StatusCreated {
		t.Fatalf("%d %v", code, out)
	}
	p := out["prediction"].(map[string]any)
	got, _ := p["canada_rulings"].([]any)
	if len(got) == 0 || len(got) > 3 {
		t.Fatalf("canada_rulings %v", p["canada_rulings"])
	}
	first := got[0].(map[string]any)
	if first["id"] != "8000002867" && first["id"] != "8000002155" || first["url"] == "" || first["holding"] == "" {
		t.Fatalf("first %v", first)
	}
	want, err := canadaEngine{rules: rules}.Predict(context.Background(), domain.Case{Product: &product})
	if err != nil {
		t.Fatal(err)
	}
	if p["origin_country"] != want.Prediction.Origin || p["decisive_step"] != want.Prediction.DecisiveStep || hs.engine.calls != 0 {
		t.Fatalf("answer changed: %v vs %+v", p, want.Prediction)
	}
}

// Issue #711: shares are fractions in [0,1]; anything else is invalid_input.
func TestCanadaMarkingShareRange(t *testing.T) {
	rules, err := tariffshift.CanadaRules()
	if err != nil {
		t.Fatal(err)
	}
	hs := newHarnessWith(t, "substantial_transformation", func(d *Deps) { d.CanadaMarking = rules })
	post := func(good, hts, field, share string) (int, map[string]any) {
		body := `{"good_hts":"` + good + `","country":"Mexico","materials":[{"name":"m","hts":"` + hts + `","origin":"China"` + share + `}]}`
		_ = field
		return hs.do(t, "POST", "/v1/rules/canada-marking", body)
	}
	for _, field := range []string{"value_share", "weight_share"} {
		for _, bad := range []string{"-1", "1.5", "-0.0001", "1.0000001"} {
			code, out := post("8501.10", "8501.10", field, `,"`+field+`":`+bad)
			if code != http.StatusBadRequest {
				t.Errorf("%s=%s: %d %v", field, bad, code, out)
				continue
			}
			if out["code"] != "invalid_input" {
				t.Errorf("%s=%s: error body %v", field, bad, out)
			}
		}
		for _, ok := range []string{"0", "0.05", "1"} {
			if code, out := post("8501.10", "8501.10", field, `,"`+field+`":`+ok); code != http.StatusOK {
				t.Errorf("%s=%s: %d %v", field, ok, code, out)
			}
		}
	}
	// Omitted shares are accepted and 1e999 stays rejected by the decoder.
	if code, out := post("8501.10", "8501.10", "", ""); code != http.StatusOK {
		t.Errorf("nil shares: %d %v", code, out)
	}
	if code, _ := post("8501.10", "8501.10", "", `,"value_share":1e999`); code != http.StatusBadRequest {
		t.Errorf("1e999: %d", code)
	}
	// The valid de minimis case still decides for Mexico; -1 must not.
	_, out := post("8501.10", "8501.10", "", `,"value_share":0.05`)
	if res, _ := out["result"].(map[string]any); res["origin"] != "Mexico" {
		t.Errorf("0.05: %v", out)
	}
	// A negative share on a product-less shifting material is still rejected.
	if code, _ := post("6301.30", "5205.11", "", `,"value_share":-1`); code != http.StatusBadRequest {
		t.Errorf("shifting -1: %d", code)
	}
}

func toyCanada(t *testing.T) *tariffshift.Rules {
	t.Helper()
	rg := func(s string) []tariffshift.Range {
		r, err := tariffshift.ParseRange(s)
		if err != nil {
			t.Fatal(err)
		}
		return []tariffshift.Range{r}
	}
	return tariffshift.New(tariffshift.Table{Regime: tariffshift.Canada.ID, Chapters: []string{"84"}, Rules: []tariffshift.Rule{{
		ID: "toy", Paragraph: "84", HTS: "8415.10", Applies: rg("841510")[0],
		Alternatives: []tariffshift.Alternative{{Encoded: true, To: rg("841510"), From: 6,
			Except: []tariffshift.Exception{{From: rg("841590"), Condition: tariffshift.CondUnlessBending}}}},
	}}})
}

func TestCanadaMarkingGuidance(t *testing.T) {
	rules, err := tariffshift.CanadaRules()
	if err != nil {
		t.Fatal(err)
	}
	hs := newHarnessWith(t, "substantial_transformation", func(d *Deps) { d.CanadaMarking = rules })
	post := func(body string) (int, map[string]any) { return hs.do(t, "POST", "/v1/rules/canada-marking", body) }

	// Unmodeled rule: review guidance, no control.
	code, out := post(`{"good_hts":"0210.91","country":"Mexico","materials":[{"name":"meat","hts":"0201.10","origin":"Brazil"}]}`)
	g, _ := out["guidance"].(map[string]any)
	if code != 200 || out["result"].(map[string]any)["status"] != "undetermined" || g == nil || g["state"] != "needs_review" {
		t.Fatalf("%d %v", code, out)
	}
	if it := g["missing"].([]any)[0].(map[string]any); it["kind"] != "review" || it["answerable"] != false {
		t.Fatalf("%v", it)
	}
	if g["review"].(map[string]any)["reason"] == "" {
		t.Fatalf("%v", g)
	}

	// Missing material code: index 0 is sent as 0.
	code, out = post(`{"good_hts":"8473.30","country":"Mexico","materials":[{"name":"b","hts":"","origin":"China"}]}`)
	g, _ = out["guidance"].(map[string]any)
	it := g["missing"].([]any)[0].(map[string]any)
	if code != 200 || it["key"] != "material_hts" || it["material"] != float64(0) {
		t.Fatalf("%d %v", code, out)
	}

	// Product mode carries guidance too.
	code, out = post(`{"product":{"name":"Meat","hts":"0210.91","plants":[{"id":"mx","country":"Mexico"}],"components":[{"name":"meat","hts":"0201.10","made_in":"Brazil","plant":"mx"}],"operations":[{"name":"cure","plant":"mx","group":"final_assembly"}]}}`)
	if code != 200 || out["guidance"] == nil {
		t.Fatalf("product: %d %v", code, out)
	}

	// Decided: no guidance key at all.
	code, out = post(`{"good_hts":"6301.30","country":"Mexico","materials":[{"name":"y","hts":"5205.11","origin":"China"}]}`)
	if _, has := out["guidance"]; code != 200 || has {
		t.Fatalf("%d %v", code, out)
	}
	// Out of scope: none.
	if _, out = post(`{"good_hts":"7701","country":"Mexico","materials":[{"name":"y","hts":"5205.11","origin":"China"}]}`); out["guidance"] != nil {
		t.Fatalf("%v", out)
	}
	// Share out of range stays a 400 without a guidance body.
	if code, out = post(`{"good_hts":"6301.30","country":"Mexico","materials":[{"name":"y","hts":"5205.11","origin":"China","value_share":1.5}]}`); code != http.StatusBadRequest || out["guidance"] != nil {
		t.Fatalf("%d %v", code, out)
	}
}

// The request's booleans are the answers: absent is unknown, false settles it.
func TestCanadaMarkingGuidanceAnswers(t *testing.T) {
	hs := newHarnessWith(t, "substantial_transformation", func(d *Deps) { d.CanadaMarking = toyCanada(t) })
	base := `"good_hts":"8415.10","country":"Mexico","materials":[{"name":"chassis","hts":"8415.90","origin":"China","value_share":0.5}]`
	_, out := hs.do(t, "POST", "/v1/rules/canada-marking", `{`+base+`}`)
	g, _ := out["guidance"].(map[string]any)
	if g == nil || g["state"] != "answerable" || g["missing"].([]any)[0].(map[string]any)["key"] != "bending_to_shape" {
		t.Fatalf("%v", out)
	}
	for ans, origin := range map[string]string{"true": "Mexico", "false": "China"} {
		_, out = hs.do(t, "POST", "/v1/rules/canada-marking", `{`+base+`,"bending_to_shape":`+ans+`}`)
		res := out["result"].(map[string]any)
		if res["status"] != "decided" || res["origin"] != origin || out["guidance"] != nil {
			t.Fatalf("%s: %v", ans, out)
		}
	}
	// Product mode forwards the answer as well: with bending_to_shape true the
	// good is decided (Mexico), and the fact is not asked again.
	prod := `{"bending_to_shape":true,"product":{"name":"AC","hts":"8415.10","plants":[{"id":"mx","country":"Mexico"}],"components":[{"name":"chassis","hts":"8415.90","made_in":"China","plant":"mx"}],"operations":[{"name":"build","plant":"mx","group":"final_assembly"}]}}`
	_, out = hs.do(t, "POST", "/v1/rules/canada-marking", prod)
	if res := out["result"].(map[string]any); res["status"] != "decided" || res["origin"] != "Mexico" || out["guidance"] != nil {
		t.Fatalf("product mode ignored the answer: %v", out)
	}
	prodNo := strings.Replace(prod, `"bending_to_shape":true`, `"bending_to_shape":false`, 1)
	_, out = hs.do(t, "POST", "/v1/rules/canada-marking", prodNo)
	// false settles the fact as no: the material does not shift, so only the
	// de minimis share is open (never the fact again), and China holds if not.
	g, _ = out["guidance"].(map[string]any)
	res := out["result"].(map[string]any)
	if res["status"] != "undetermined" || g == nil || len(g["missing"].([]any)) != 1 || g["missing"].([]any)[0].(map[string]any)["key"] != "value_share" {
		t.Fatalf("false must settle the fact as no: %v", out)
	}
}

func TestCanadaMarkingMCPGuidance(t *testing.T) {
	rules, err := tariffshift.CanadaRules()
	if err != nil {
		t.Fatal(err)
	}
	hs := newHarnessWith(t, "substantial_transformation", func(d *Deps) { d.CanadaMarking = rules })
	cs := mcpClient(t, hs, nil)

	// Unmodeled rule: guidance in structuredContent, review line in text.
	res, out := callTool(t, cs, "check_canada_marking", map[string]any{"good_hts": "0210.91", "country": "Mexico",
		"materials": []any{map[string]any{"name": "meat", "hts": "0201.10", "origin": "Brazil"}}})
	g, _ := out["guidance"].(map[string]any)
	if res.IsError || g == nil || g["state"] != "needs_review" || !strings.Contains(text(res), "Needs human review (no flag can answer it)") {
		t.Fatalf("%s %v", text(res), out)
	}
	// Decided: no guidance, no guidance lines.
	res, out = callTool(t, cs, "check_canada_marking", map[string]any{"good_hts": "6301.30", "country": "Mexico",
		"materials": []any{map[string]any{"name": "cotton yarn", "hts": "5205.11", "origin": "China"}}})
	if _, has := out["guidance"]; has || strings.Contains(text(res), "human review") || strings.Contains(text(res), "Would resolve") {
		t.Fatalf("%s %v", text(res), out)
	}

	// bending_to_shape is in the input schema and is forwarded.
	tl, err := cs.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, tool := range tl.Tools {
		if tool.Name == "check_canada_marking" {
			b, _ := json.Marshal(tool.InputSchema)
			found = strings.Contains(string(b), `"bending_to_shape"`) && strings.Contains(tool.Description, "guidance")
		}
	}
	if !found {
		t.Fatal("bending_to_shape or guidance missing from the tool")
	}
	toy := mcpClient(t, newHarnessWith(t, "substantial_transformation", func(d *Deps) { d.CanadaMarking = toyCanada(t) }), nil)
	args := map[string]any{"good_hts": "8415.10", "country": "Mexico",
		"materials": []any{map[string]any{"name": "chassis", "hts": "8415.90", "origin": "China", "value_share": 0.5}}}
	res, out = callTool(t, toy, "check_canada_marking", args)
	if g, _ := out["guidance"].(map[string]any); g == nil || g["state"] != "answerable" || !strings.Contains(text(res), "send bending_to_shape true or false") {
		t.Fatalf("%s %v", text(res), out)
	}
	for ans, origin := range map[bool]string{true: "Mexico", false: "China"} {
		args["bending_to_shape"] = ans
		res, out = callTool(t, toy, "check_canada_marking", args)
		if r := out["result"].(map[string]any); r["origin"] != origin || out["guidance"] != nil {
			t.Fatalf("%v: %s %v", ans, text(res), out)
		}
	}
}

// Guidance on the real Canadian table (no synthetic rules): what it asks for
// today is codes, shares and human review. The real rows carry no
// simple-assembly, GRI 2(a) or bending clause, so no yes/no fact is asked.
func TestCanadaMarkingGuidanceRealRows(t *testing.T) {
	rules, err := tariffshift.CanadaRules()
	if err != nil {
		t.Fatal(err)
	}
	hs := newHarnessWith(t, "substantial_transformation", func(d *Deps) { d.CanadaMarking = rules })
	// run returns status, origin and "kind/key@material" of every item.
	run := func(body string) (string, string, []string) {
		t.Helper()
		code, out := hs.do(t, "POST", "/v1/rules/canada-marking", body)
		if code != 200 {
			t.Fatalf("%d %v", code, out)
		}
		res := out["result"].(map[string]any)
		origin, _ := res["origin"].(string)
		var items []string
		if g, _ := out["guidance"].(map[string]any); g != nil {
			for _, m := range g["missing"].([]any) {
				m := m.(map[string]any)
				if m["kind"] == "fact" || m["answerable"] == true {
					t.Fatalf("a real row asked a fact: %v", out)
				}
				s := m["kind"].(string) + "/" + m["key"].(string)
				if i, ok := m["material"].(float64); ok {
					s += "@" + strconv.Itoa(int(i))
				}
				items = append(items, s)
			}
		} else if res["status"] == "undetermined" {
			t.Fatalf("undetermined without guidance: %v", out)
		}
		return res["status"].(string), origin, items
	}
	one := func(good, hts, extra string) string {
		return `{"good_hts":"` + good + `","country":"Mexico","materials":[{"name":"m","hts":"` + hts + `","origin":"China"` + extra + `}]}`
	}
	for _, c := range []struct {
		name, body, status, origin string
		items                      []string
	}{
		// (a) de minimis open: value share (1601), weight share (chapters 50-63).
		{"value share missing", one("1601", "1601", ""), "undetermined", "", []string{"share/value_share@0"}},
		{"value share 0 is a real 0", one("1601", "1601", `,"value_share":0`), "decided", "Mexico", nil},
		{"value share 5%", one("1601", "1601", `,"value_share":0.05`), "decided", "Mexico", nil},
		{"value share 7%", one("1601", "1601", `,"value_share":0.07`), "decided", "Mexico", nil},
		{"value share 20%", one("1601", "1601", `,"value_share":0.2`), "decided", "China", nil},
		{"value share just above 7%", one("1601", "1601", `,"value_share":0.0701`), "decided", "China", nil},
		{"chapter 22 share 10%", one("2203", "2203", `,"value_share":0.10`), "decided", "Mexico", nil},
		{"chapter 22 share above 10%", one("2203", "2203", `,"value_share":0.1001`), "decided", "China", nil},
		{"weight share missing", one("5001", "5001", ""), "undetermined", "", []string{"share/weight_share@0"}},
		{"weight share 0 is a real 0", one("5001", "5001", `,"weight_share":0`), "decided", "Mexico", nil},
		{"weight share 5%", one("5001", "5001", `,"weight_share":0.05`), "decided", "Mexico", nil},
		{"weight share 20%", one("5001", "5001", `,"weight_share":0.2`), "decided", "China", nil},
		// (b) a missing material code; index 0 and index 1.
		{"material code missing, index 0", one("6301.30", "", ""), "undetermined", "", []string{"code/material_hts@0"}},
		{"material code given, index 0", one("6301.30", "5205.11", ""), "decided", "Mexico", nil},
		{"material code missing, index 1", `{"good_hts":"6301.30","country":"Mexico","materials":[{"name":"d","hts":"5205.11","origin":"Mexico"},{"name":"m","hts":"","origin":"China"}]}`, "undetermined", "", []string{"code/material_hts@1"}},
		// (c) short codes.
		{"good code too short", one("6301", "5205.11", ""), "undetermined", "", []string{"code/good_code"}},
		{"good code of 6 digits", one("6301.30", "5205.11", ""), "decided", "Mexico", nil},
		{"material code too short", one("0305.10", "0305", ""), "undetermined", "", []string{"code/material_code@0"}},
		{"material code of 6 digits", one("0305.10", "0301.11", ""), "decided", "Mexico", nil},
		// (d) rules written by goods description: a person, never a control.
		{"goods description row 0302", one("0302.11", "0301.11", ""), "undetermined", "", []string{"review/goods_description"}},
		{"goods description row 0303", one("0303.11", "0301.11", ""), "undetermined", "", []string{"review/goods_description"}},
		{"judgment row 0210.91", one("0210.91", "0201.10", ""), "undetermined", "", []string{"review/person_reading"}},
	} {
		status, origin, items := run(c.body)
		if status != c.status || origin != c.origin || !slices.Equal(items, c.items) {
			t.Errorf("%s: got %s %q %v, want %s %q %v", c.name, status, origin, items, c.status, c.origin, c.items)
		}
	}
}

// The MCP tool returns the same guidance on a real row.
func TestCanadaMarkingMCPRealShare(t *testing.T) {
	rules, err := tariffshift.CanadaRules()
	if err != nil {
		t.Fatal(err)
	}
	cs := mcpClient(t, newHarnessWith(t, "substantial_transformation", func(d *Deps) { d.CanadaMarking = rules }), nil)
	args := map[string]any{"good_hts": "5001", "country": "Mexico", "materials": []any{map[string]any{"name": "silk", "hts": "5001", "origin": "China"}}}
	res, out := callTool(t, cs, "check_canada_marking", args)
	g, _ := out["guidance"].(map[string]any)
	if res.IsError || g == nil || g["state"] != "needs_input" || g["missing"].([]any)[0].(map[string]any)["key"] != "weight_share" {
		t.Fatalf("%s %v", text(res), out)
	}
	args["materials"] = []any{map[string]any{"name": "silk", "hts": "5001", "origin": "China", "weight_share": 0.05}}
	_, out = callTool(t, cs, "check_canada_marking", args)
	if _, has := out["guidance"]; has || out["result"].(map[string]any)["origin"] != "Mexico" {
		t.Fatalf("%v", out)
	}
}

// The UI re-runs a product check as good_hts + country + materials (a product
// cannot carry a share). Without a share the two bodies must give the same
// result, including a part of another origin and a part with no code.
func TestCanadaMarkingProductEqualsMaterialsBody(t *testing.T) {
	rules, err := tariffshift.CanadaRules()
	if err != nil {
		t.Fatal(err)
	}
	hs := newHarnessWith(t, "substantial_transformation", func(d *Deps) { d.CanadaMarking = rules })
	for name, comps := range map[string]string{
		"foreign and domestic parts":  `{"name":"a","hts":"1601","made_in":"China","plant":"mx"},{"name":"b","hts":"3926.90","made_in":"Mexico","plant":"mx"}`,
		"a part with no code":         `{"name":"a","hts":"1601","made_in":"China","plant":"mx"},{"name":"b","made_in":"Korea","plant":"mx"}`,
		"a part of another origin":    `{"name":"a","hts":"1601","made_in":"Korea","plant":"mx"},{"name":"b","hts":"1601","made_in":"China","plant":"mx"}`,
		"the first part has no code":  `{"name":"a","made_in":"China","plant":"mx"},{"name":"b","hts":"1601","made_in":"China","plant":"mx"}`,
		"part with a short code only": `{"name":"a","hts":"16","made_in":"China","plant":"mx"}`,
	} {
		product := `{"name":"p","hts":"1601","plants":[{"id":"mx","country":"Mexico"}],"components":[` + comps + `],"operations":[{"name":"build","plant":"mx","group":"final_assembly"}]}`
		_, a := hs.do(t, "POST", "/v1/rules/canada-marking", `{"product":`+product+`}`)
		var prod struct {
			Components []map[string]string `json:"components"`
		}
		if err := json.Unmarshal([]byte(product), &prod); err != nil {
			t.Fatal(err)
		}
		var mats []map[string]string
		for _, c := range prod.Components {
			m := map[string]string{"name": c["name"], "origin": c["made_in"]}
			if c["hts"] != "" {
				m["hts"] = c["hts"]
			}
			mats = append(mats, m)
		}
		body, _ := json.Marshal(map[string]any{"good_hts": "1601", "country": a["country"], "materials": mats})
		_, b := hs.do(t, "POST", "/v1/rules/canada-marking", string(body))
		ja, _ := json.Marshal(a["result"])
		jb, _ := json.Marshal(b["result"])
		if string(ja) != string(jb) || a["country"] != b["country"] {
			t.Errorf("%s:\nproduct   %s\nmaterials %s", name, ja, jb)
		}
	}
}
