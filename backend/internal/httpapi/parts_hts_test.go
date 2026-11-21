package httpapi

import (
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/hts"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/llm"
)

// partsHarness serves parts-hts-suggest from the shipped part index, with
// the product suggester for the good's code.
func partsHarness(t *testing.T, m llm.Model, adjust func(*Deps)) harness {
	t.Helper()
	px, err := hts.LoadPartIndex(filepath.Join("..", "..", "data", "part_hts_index.json"))
	if err != nil {
		t.Fatal(err)
	}
	x, err := hts.LoadIndex(filepath.Join("..", "..", "data", "hts_index.json"))
	if err != nil {
		t.Fatal(err)
	}
	s, err := hts.New(m, "gemini-3.8-flash", x, filepath.Join("..", "..", "data", "facts"), nil)
	if err != nil {
		t.Fatal(err)
	}
	return newHarnessWith(t, "substantial_transformation", func(d *Deps) {
		d.PartsHTS = hts.NewParts(m, "gemini-3.8-flash", px, s, nil)
		if adjust != nil {
			adjust(d)
		}
	})
}

const partsBody = `{"product":{"name":"heat gun","description":"Hand-held tool that blows hot air.","components":[{"name":"electric motor","made_in":"China"},{"name":"heating element","made_in":"China","hts":"8516.80"}],"operations":[]}}`

func TestPartsHTSSuggestOffIs404(t *testing.T) {
	hs := newHarness(t, "substantial_transformation")
	if code, out := hs.do(t, "POST", "/v1/products/parts-hts-suggest", partsBody); code != http.StatusNotFound {
		t.Errorf("off = %d %v", code, out)
	}
}

func TestPartsHTSSuggest(t *testing.T) {
	m := &llm.Fake{Respond: func(req llm.Request) (llm.Response, error) {
		return llm.Response{Text: `{"items":[{"key":"p0","candidates":[{"hts":"8501.31","reason":"A DC motor.","cited":[]}]},{"key":"good","candidates":[{"hts":"8516.79.00","reason":"Electro-thermic appliance.","cited":[]}]}]}`}, nil
	}}
	hs := partsHarness(t, m, nil)
	code, out := hs.do(t, "POST", "/v1/products/parts-hts-suggest", partsBody)
	if code != http.StatusOK {
		t.Fatalf("suggest = %d %v", code, out)
	}
	if out["note"] != hts.UnverifiedNote || out["prompt_version"] != hts.PartsPromptVersion || out["reasons_unavailable"] != false || out["max_parts"] != float64(hts.MaxParts) || out["disclaimer"] == "" {
		t.Errorf("response = %v", out)
	}
	parts := out["parts"].([]any)
	if len(parts) != 2 {
		t.Fatalf("parts = %v", parts)
	}
	p0 := parts[0].(map[string]any)
	c0 := p0["candidates"].([]any)[0].(map[string]any)
	if p0["index"] != float64(0) || c0["hts"] != "8501.31" || c0["reason"] != "A DC motor." || (c0["source"] != hts.SourceRulings && c0["source"] != hts.SourceModel) {
		t.Errorf("part 0 = %v", p0)
	}
	if good := out["good"].(map[string]any); good["candidates"].([]any)[0].(map[string]any)["hts"] != "8516.79.00" {
		t.Errorf("good = %v", good)
	}
	if len(m.Calls()) != 1 {
		t.Errorf("model calls = %d, want 1 for the whole product", len(m.Calls()))
	}

	// With the good's code given, the good is not asked.
	withCode := strings.Replace(partsBody, `"name":"heat gun",`, `"name":"heat gun","hts":"8419.89.95",`, 1)
	if code, out := hs.do(t, "POST", "/v1/products/parts-hts-suggest", withCode); code != http.StatusOK || out["good"] != nil {
		t.Errorf("with code = %d %v", code, out["good"])
	}
	if len(m.Calls()) != 1 {
		t.Errorf("model calls = %d: the parts were cached and the good has a code, so nothing is asked", len(m.Calls()))
	}
}

func TestPartsHTSSuggestModelDown(t *testing.T) {
	hs := partsHarness(t, &llm.Fake{Respond: func(llm.Request) (llm.Response, error) { return llm.Response{}, llm.ErrUnavailable }}, nil)
	code, out := hs.do(t, "POST", "/v1/products/parts-hts-suggest", partsBody)
	if code != http.StatusOK || out["reasons_unavailable"] != true {
		t.Errorf("model down = %d %v", code, out)
	}
}

func TestPartsHTSSuggestErrorsAndLimit(t *testing.T) {
	hs := partsHarness(t, llm.FakeText(`{"items":[]}`), func(d *Deps) { d.Limits = Limits{LightPerMin: 2} })
	many := make([]string, hts.MaxParts+1)
	for i := range many {
		many[i] = fmt.Sprintf(`{"name":"part %d","made_in":"China"}`, i)
	}
	tests := []struct{ name, body string }{
		{"empty", `{}`},
		{"no name", `{"product":{"name":" ","components":[{"name":"motor","made_in":"China"}]}}`},
		{"no parts", `{"product":{"name":"fan","components":[{"name":" ","made_in":"China"}]}}`},
		{"too many parts", `{"product":{"name":"fan","components":[` + strings.Join(many, ",") + `]}}`},
		{"long part", `{"product":{"name":"fan","components":[{"name":"` + strings.Repeat("a", 201) + `","made_in":"China"}]}}`},
		{"long description", `{"product":{"name":"fan","description":"` + strings.Repeat("a", 12001) + `","components":[{"name":"motor","made_in":"China"}]}}`},
		{"unknown field", `{"product":{"name":"fan","components":[]},"facts":"x"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, out := hs.doAs(t, "POST", "/v1/products/parts-hts-suggest", tt.body, map[string]string{"X-Client-IP": "10.0.2." + tt.name})
			if code != http.StatusBadRequest || out["code"] != "invalid_input" {
				t.Errorf("%s = %d %v", tt.name, code, out)
			}
		})
	}
	ip := map[string]string{"X-Client-IP": "10.1.2.1"}
	for i := range 2 {
		if code, _ := hs.doAs(t, "POST", "/v1/products/parts-hts-suggest", partsBody, ip); code != http.StatusOK {
			t.Fatalf("request %d = %d", i, code)
		}
	}
	if code, _ := hs.doAs(t, "POST", "/v1/products/parts-hts-suggest", partsBody, ip); code != http.StatusTooManyRequests {
		t.Errorf("third light request = %d, want 429", code)
	}
}

// suggest_part_hts runs the REST route in-process: same limiter, same answer.
func TestMCPSuggestPartHTS(t *testing.T) {
	m := &llm.Fake{Respond: func(req llm.Request) (llm.Response, error) {
		return llm.Response{Text: `{"items":[{"key":"p0","candidates":[{"hts":"8501.31","reason":"A DC motor.","cited":[]}]}]}`}, nil
	}}
	hs := partsHarness(t, m, func(d *Deps) { d.Limits = Limits{LightPerMin: 1, HeavyPerMin: 1, GlobalPerMin: 1000} })
	ip := "198.51.100.71"
	cs := mcpClient(t, hs, map[string]string{"X-Client-IP": ip})
	res, out := callTool(t, cs, "suggest_part_hts", map[string]any{"product": map[string]any{
		"name": "heat gun", "hts": "8419.89.95", "components": []any{map[string]any{"name": "electric motor", "made_in": "China"}},
	}})
	if res.IsError || len(out["parts"].([]any)) != 1 {
		t.Fatalf("suggest_part_hts: %s", text(res))
	}
	for _, want := range []string{"unverified", "electric motor: 8501.31 (", "only when the user accepts it"} {
		if !strings.Contains(text(res), want) {
			t.Errorf("%q lacks %q", text(res), want)
		}
	}
	if code, _ := hs.doAs(t, "POST", "/v1/products/parts-hts-suggest", partsBody, map[string]string{"X-Client-IP": ip}); code != http.StatusTooManyRequests {
		t.Errorf("REST after the MCP call = %d, want 429", code)
	}
}

// #577: the per-part cache is scoped to the caller and keyed on the
// description, so one caller never gets reasons the model wrote from
// another caller's product.
func TestPartsHTSSuggestCacheIsPerCallerAndDescription(t *testing.T) {
	m := &llm.Fake{Respond: func(req llm.Request) (llm.Response, error) {
		desc := ""
		for _, line := range strings.Split(req.Messages[0].Content, "\n") {
			if d, ok := strings.CutPrefix(line, "DESCRIPTION: "); ok {
				desc = d
			}
		}
		return llm.Response{Text: fmt.Sprintf(`{"items":[{"key":"p0","candidates":[{"hts":"8538.90","reason":%q,"cited":[]}]}]}`, "Housing of "+desc)}, nil
	}}
	hs := partsHarness(t, m, nil)
	body := func(desc string) string {
		return fmt.Sprintf(`{"product":{"name":"Controller","hts":"8537.10","description":%q,"components":[{"name":"housing","made_in":"China"}]}}`, desc)
	}
	reason := func(out map[string]any) (string, bool) {
		p := out["parts"].([]any)[0].(map[string]any)
		return p["candidates"].([]any)[0].(map[string]any)["reason"].(string), p["cached"].(bool)
	}
	secret := "ALPHA-SECRET: lidar controller rev 9, customer Foo"
	alice := map[string]string{"X-Visitor-ID": "alice-visitor-id-000001"}
	bob := map[string]string{"X-Visitor-ID": "bob-visitor-id-00000002"}
	if code, out := hs.doAs(t, "POST", "/v1/products/parts-hts-suggest", body(secret), alice); code != http.StatusOK {
		t.Fatalf("alice = %d %v", code, out)
	}
	for _, tc := range []struct {
		name    string
		desc    string
		headers map[string]string
	}{
		{"another visitor, same words", secret, bob},
		{"another visitor, another description", "Garden irrigation controller", bob},
		{"same visitor, another description", "Garden irrigation controller", alice},
		{"anonymous caller, same words", secret, nil},
	} {
		code, out := hs.doAs(t, "POST", "/v1/products/parts-hts-suggest", body(tc.desc), tc.headers)
		if code != http.StatusOK {
			t.Fatalf("%s = %d %v", tc.name, code, out)
		}
		r, cached := reason(out)
		if cached {
			t.Errorf("%s: served from the cache", tc.name)
		}
		if tc.desc != secret && strings.Contains(r, "ALPHA-SECRET") {
			t.Errorf("%s: reason %q carries another caller's description", tc.name, r)
		}
	}
	if _, out := hs.doAs(t, "POST", "/v1/products/parts-hts-suggest", body(secret), alice); out != nil {
		if r, cached := reason(out); !cached || !strings.Contains(r, "ALPHA-SECRET") {
			t.Errorf("alice's repeat = %q cached=%v, want her own cached answer", r, cached)
		}
	}
	if n := len(m.Calls()); n != 5 {
		t.Errorf("model calls = %d, want 5 (one per request but alice's repeat)", n)
	}
}
