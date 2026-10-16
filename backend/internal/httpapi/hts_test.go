package httpapi

import (
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/hts"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/llm"
)

// htsHarness serves hts-suggest from the shipped index and facts, with a fake
// model and the real duty table.
func htsHarness(t *testing.T, m llm.Model, adjust func(*Deps)) harness {
	t.Helper()
	x, err := hts.LoadIndex(filepath.Join("..", "..", "data", "hts_index.json"))
	if err != nil {
		t.Fatal(err)
	}
	table := mustDuty(t)
	s, err := hts.New(m, "gemini-3.8-flash", x, filepath.Join("..", "..", "data", "facts"), func(code string) bool { _, ok := table.Line(code); return ok })
	if err != nil {
		t.Fatal(err)
	}
	return newHarnessWith(t, "substantial_transformation", func(d *Deps) {
		d.HTS = s
		if adjust != nil {
			adjust(d)
		}
	})
}

const heatGunFacts = `{"facts":"A hand-held electric heat gun for stripping paint and shrinking tubing, with a motor, fan, heating element and plastic housing."}`

func TestHTSSuggestOffIs404(t *testing.T) {
	hs := newHarness(t, "substantial_transformation") // HTS_SUGGEST=off: no suggester
	if code, out := hs.do(t, "POST", "/v1/products/hts-suggest", heatGunFacts); code != http.StatusNotFound || out["code"] != "not_found" {
		t.Errorf("off = %d %v", code, out)
	}
}

func TestHTSSuggest(t *testing.T) {
	m := &llm.Fake{Respond: func(req llm.Request) (llm.Response, error) {
		// Answer for whichever candidates were offered, citing the first ruling.
		items := req.JSONSchema["properties"].(map[string]any)["candidates"].(map[string]any)["items"].(map[string]any)["properties"].(map[string]any)
		code := items["hts"].(map[string]any)["enum"].([]string)[0]
		id := items["cited"].(map[string]any)["items"].(map[string]any)["enum"].([]string)[0]
		return llm.Response{Text: `{"candidates":[{"hts":"` + code + `","reason":"Similar goods.","cited":["` + id + `","N999999"]}]}`}, nil
	}}
	hs := htsHarness(t, m, nil)
	code, out := hs.do(t, "POST", "/v1/products/hts-suggest", heatGunFacts)
	if code != http.StatusOK {
		t.Fatalf("suggest = %d %v", code, out)
	}
	if out["note"] != hts.Note || out["reasons_unavailable"] != false || out["prompt_version"] != hts.PromptVersion || out["trace"] == nil {
		t.Errorf("response = %v", out)
	}
	cs := out["candidates"].([]any)
	if len(cs) == 0 || len(cs) > hts.MaxCandidates {
		t.Fatalf("candidates = %v", cs)
	}
	best := cs[0].(map[string]any)
	sup := best["supporting_rulings"].([]any)
	if len(sup) == 0 || best["reason"] != "Similar goods." || len(best["cited"].([]any)) != 1 {
		t.Errorf("best = %v (N999999 was not retrieved, so it is dropped)", best)
	}
	ref := sup[0].(map[string]any)
	if ref["id"] == "" || ref["date"] == "" || ref["subject"] == "" || !strings.HasPrefix(ref["url"].(string), "https://rulings.cbp.gov/ruling/") {
		t.Errorf("ref = %v", ref)
	}
	if _, ok := best["duty_rates_available"].(bool); !ok {
		t.Errorf("duty_rates_available missing: %v", best)
	}
	if len(m.Calls()) != 1 {
		t.Errorf("model calls = %d, want 1", len(m.Calls()))
	}

	// A structured product works too.
	prod := `{"product":{"name":"heat gun","description":"Hand-held tool that blows hot air to strip paint.","components":[{"name":"motor","made_in":"China"},{"name":"heating element","made_in":"Thailand"}],"operations":[]}}`
	if code, out := hs.do(t, "POST", "/v1/products/hts-suggest", prod); code != http.StatusOK || len(out["candidates"].([]any)) == 0 {
		t.Errorf("product = %d %v", code, out)
	}
	if !strings.Contains(m.Calls()[1].Messages[0].Content, "The merchandise is a heat gun.") {
		t.Errorf("product text not sent: %q", m.Calls()[1].Messages[0].Content)
	}
}

func TestHTSSuggestModelFailureStillShowsCandidates(t *testing.T) {
	hs := htsHarness(t, &llm.Fake{Respond: func(llm.Request) (llm.Response, error) { return llm.Response{}, llm.ErrUnavailable }}, nil)
	code, out := hs.do(t, "POST", "/v1/products/hts-suggest", heatGunFacts)
	if code != http.StatusOK || out["reasons_unavailable"] != true || len(out["candidates"].([]any)) == 0 || out["note"] != hts.Note {
		t.Errorf("model down = %d %v", code, out)
	}
}

func TestHTSSuggestErrorsAndLimit(t *testing.T) {
	hs := htsHarness(t, llm.FakeText(`{"candidates":[]}`), func(d *Deps) { d.Limits = Limits{LightPerMin: 2} })
	tests := []struct {
		name, body string
		status     int
	}{
		{"empty", `{}`, http.StatusBadRequest},
		{"blank facts", `{"facts":"   "}`, http.StatusBadRequest},
		{"product without name", `{"product":{"name":" ","components":[]}}`, http.StatusBadRequest},
		{"too long", `{"facts":"` + strings.Repeat("a", 12001) + `"}`, http.StatusBadRequest},
		{"unknown field", `{"facts":"x","hts":"1"}`, http.StatusBadRequest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// The limit applies before validation, so each case is its own client.
			code, out := hs.doAs(t, "POST", "/v1/products/hts-suggest", tt.body, map[string]string{"X-Client-IP": "10.0.0." + tt.name})
			if code != tt.status || out["code"] != "invalid_input" {
				t.Errorf("%s = %d %v", tt.name, code, out)
			}
		})
	}
	ip := map[string]string{"X-Client-IP": "10.1.1.1"}
	for i := range 2 {
		if code, _ := hs.doAs(t, "POST", "/v1/products/hts-suggest", heatGunFacts, ip); code != http.StatusOK {
			t.Fatalf("request %d = %d", i, code)
		}
	}
	if code, _ := hs.doAs(t, "POST", "/v1/products/hts-suggest", heatGunFacts, ip); code != http.StatusTooManyRequests {
		t.Errorf("third light request = %d, want 429", code)
	}
}
