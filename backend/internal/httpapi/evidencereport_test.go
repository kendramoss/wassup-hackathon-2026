package httpapi

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
)

// The report runs each check on its own and never merges them (#771,
// D803-D805). Synthetic case; the committed tables.
func TestEvidenceReportSeparateChecks(t *testing.T) {
	hs, mem := originHarness(t)
	c := seedCase(t, mem, "case_us_radio", "us", radioProduct())
	code, out := hs.do(t, "GET", "/v1/cases/"+c.ID+"/evidence-report", "")
	if code != 200 {
		t.Fatalf("%d %v", code, out)
	}
	var ids []string
	byID := map[string]map[string]any{}
	for _, x := range asList(out["checks"]) {
		ch := x.(map[string]any)
		ids = append(ids, ch["id"].(string))
		byID[ch["id"].(string)] = ch
		for _, k := range []string{"title", "question", "status", "outcome", "label", "method"} {
			if s, _ := ch[k].(string); s == "" {
				t.Errorf("%s: no %s", ch["id"], k)
			}
		}
	}
	if !reflect.DeepEqual(ids, []string{"us_origin", "us_part102", "ca_marking", "eu_origin", "uk_origin", "rcep", "usmca"}) {
		t.Fatalf("checks %v", ids)
	}
	if us := byID["us_origin"]; us["origin"] != "Malaysia" || us["method"] != "model" || us["review"] != true {
		t.Errorf("us: %v", us)
	}
	for _, id := range []string{"ca_marking", "eu_origin", "uk_origin"} {
		if byID[id]["review"] != true || byID[id]["rule_version"] == "" || len(asList(byID[id]["sources"])) == 0 {
			t.Errorf("%s: rules-only markets are always reviewed and versioned: %v", id, byID[id])
		}
	}
	if eu := byID["eu_origin"]; eu["status"] != "needs_information" || !strings.Contains(asList(eu["missing"])[0].(map[string]any)["evidence"].(string), "bill of materials") {
		t.Errorf("eu: %v", eu)
	}
	// No merged verdict anywhere.
	for _, k := range []string{"overall", "verdict", "score"} {
		if _, ok := out[k]; ok {
			t.Errorf("report has %q", k)
		}
	}
	md := out["markdown"].(string)
	for _, want := range []string{"different legal questions", "Not a ruling", "## EU non-preferential origin (Annex 22-01)", "## Case facts as stored", "| 1 | tuner | 8529.90 | China |", "no accuracy figure"} {
		if !strings.Contains(md, want) {
			t.Errorf("markdown lacks %q", want)
		}
	}
	if strings.Contains(strings.ToLower(md), "overall") {
		t.Error("markdown says overall")
	}
	// Read only: nothing stored, no event.
	got, _ := mem.Get(context.Background(), c.ID)
	if len(got.History) != len(c.History) || !got.UpdatedAt.Equal(c.UpdatedAt) {
		t.Errorf("the report changed the case")
	}
}

// A non-U.S. case: the U.S. model answer is not run; the EU check reads the
// supplied facts and the report lists them apart from the case facts.
func TestEvidenceReportNoImplicitModelAndSuppliedFacts(t *testing.T) {
	hs, mem := originHarness(t)
	c := seedCase(t, mem, "case_eu_radio2", "eu", radioProduct())
	if code, out := hs.do(t, "PUT", "/v1/cases/"+c.ID+"/origin-facts/eu", `{"answers":{"ch85.r":false},"materials":[{"index":0,"name":"tuner","part_hts":"8529.90","value_share":0.7},{"index":1,"name":"case","part_hts":"3926.90","value_share":0.2}]}`); code != 200 {
		t.Fatalf("%d %v", code, out)
	}
	hs.engine.mu.Lock()
	before := hs.engine.calls
	hs.engine.mu.Unlock()
	_, out := hs.do(t, "GET", "/v1/cases/"+c.ID+"/evidence-report", "")
	hs.engine.mu.Lock()
	after := hs.engine.calls
	hs.engine.mu.Unlock()
	if after != before {
		t.Error("the report called the model")
	}
	checks := map[string]map[string]any{}
	for _, x := range asList(out["checks"]) {
		ch := x.(map[string]any)
		checks[ch["id"].(string)] = ch
	}
	if us := checks["us_origin"]; us["status"] != "not_run" {
		t.Errorf("us on an EU case: %v", us)
	}
	if eu := checks["eu_origin"]; eu["status"] != "answered" || eu["origin"] != "China" || eu["uses_supplied"] != true {
		t.Errorf("eu: %v", eu)
	}
	if uk := checks["uk_origin"]; uk["uses_supplied"] == true {
		t.Errorf("uk read the EU's facts: %v", uk)
	}
	md := out["markdown"].(string)
	if !strings.Contains(md, "## Supplied facts") || !strings.Contains(md, "Source: "+domain.SourceUserSupplied) || !strings.Contains(md, "value share 70%") {
		t.Errorf("markdown supplied section:\n%s", md)
	}
	if out["case"].(map[string]any)["product"].(map[string]any)["components"].([]any)[0].(map[string]any)["hts"] != "8529.90" {
		t.Error("case facts not as stored")
	}
}

func TestEvidenceReportLaptopIsNotModelled(t *testing.T) {
	hs, mem := originHarness(t)
	laptop := &domain.Product{Name: "Laptop", HTS: "8471.30", Plants: []domain.Plant{{ID: "vn", Country: "Vietnam"}},
		Components: []domain.Component{{Name: "board", HTS: "8473.30", MadeIn: "China", Plant: "vn"}}}
	c := seedCase(t, mem, "case_uk_laptop", "uk", laptop)
	_, out := hs.do(t, "GET", "/v1/cases/"+c.ID+"/evidence-report", "")
	for _, x := range asList(out["checks"]) {
		ch := x.(map[string]any)
		if ch["id"] == "uk_origin" || ch["id"] == "eu_origin" {
			if ch["status"] != "needs_judgement" || len(asList(ch["missing"])) != 0 || len(asList(ch["not_modelled"])) != 1 {
				t.Errorf("%s: %v", ch["id"], ch)
			}
		}
	}
}

func TestEvidenceReportMCPTool(t *testing.T) {
	hs, mem := originHarness(t)
	c := seedCase(t, mem, "case_us_radio_mcp", "us", radioProduct())
	cs := mcpClient(t, hs, nil)
	res, out := callTool(t, cs, "get_evidence_report", map[string]any{"case_id": c.ID})
	if res.IsError || !strings.Contains(text(res), "different legal questions") || !strings.Contains(text(res), "## UK non-preferential origin (Part Four)") {
		t.Fatalf("%s %v", text(res), out)
	}
	if _, dup := out["markdown"]; dup {
		t.Error("markdown sent twice")
	}
	res, _ = callTool(t, cs, "get_evidence_report", map[string]any{"case_id": "case_missing"})
	if !res.IsError {
		t.Error("a missing case answered")
	}
}
