package httpapi

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/retrieval"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// fakeSearch indexes three rulings with the real BM25.
type fakeSearch struct{}

func (fakeSearch) Search(ctx context.Context, q string, k int, allow func(domain.Ruling) bool) ([]retrieval.Hit, error) {
	day := func(s string) time.Time { d, _ := time.Parse(time.DateOnly, s); return d }
	rs := []domain.Ruling{
		{ID: "N300001", Date: day("2019-05-01"), Subject: "heat gun", Facts: "A heat gun with a motor and fan assembled in Thailand from a Chinese printed circuit board.", Origin: "Thailand"},
		{ID: "N300002", Date: day("2023-02-01"), Subject: "hair dryer", Facts: "A hair dryer whose motor is wound in Vietnam and assembled in China.", Origin: "China"},
		{ID: "H300003", Date: day("2021-07-01"), Subject: "lidar sensor", Facts: "A lidar sensor module calibrated in Mexico."},
	}
	return retrieval.NewBM25(rs, func(r domain.Ruling) string { return r.Facts }).Search(ctx, q, k, allow)
}

func TestSearchRulings(t *testing.T) {
	hs := newHarnessWith(t, "substantial_transformation", func(d *Deps) { d.RulingSearch = fakeSearch{} })
	code, out := hs.do(t, "GET", "/v1/rulings?q=motor+fan+assembled&limit=5", "")
	rs, _ := out["rulings"].([]any)
	if code != http.StatusOK || len(rs) == 0 || out["disclaimer"] == nil {
		t.Fatalf("search = %d %v", code, out)
	}
	first := rs[0].(map[string]any)
	if first["id"] != "N300001" || first["origin"] != "Thailand" || first["url"] != "https://rulings.cbp.gov/ruling/N300001" || first["snippet"] == "" {
		t.Errorf("first hit = %v", first)
	}
	_, out = hs.do(t, "GET", "/v1/rulings?q=motor+assembled&before=2020-01-01", "")
	for _, r := range out["rulings"].([]any) {
		if r.(map[string]any)["id"] == "N300002" {
			t.Error("before did not filter a later ruling")
		}
	}
	_, out = hs.do(t, "GET", "/v1/rulings?q=lidar", "")
	if r := out["rulings"].([]any); len(r) != 1 || r[0].(map[string]any)["origin"] != nil {
		t.Errorf("no held origin should be null: %v", r)
	}
	for _, q := range []string{"", "q=", "q=a+b", "q=" + strings.Repeat("motor+", 60), "q=motor&limit=0", "q=motor&limit=21", "q=motor&before=2020", "q=motor;x"} {
		if code, _ := hs.do(t, "GET", "/v1/rulings?"+q, ""); code != http.StatusBadRequest {
			t.Errorf("%q = %d, want 400", q, code)
		}
	}
	// Without a search index the route is not served.
	if code, _ := newHarness(t, "substantial_transformation").do(t, "GET", "/v1/rulings?q=motor", ""); code != http.StatusNotFound {
		t.Errorf("without RulingSearch = %d, want 404", code)
	}
}

func TestSearchRulingsUsesTheLookupBucket(t *testing.T) {
	hs := newHarnessWith(t, "substantial_transformation", func(d *Deps) {
		d.RulingSearch = fakeSearch{}
		d.Limits = Limits{LightPerMin: 20, HeavyPerMin: 4, GlobalPerMin: 120, LookupPerMin: 1}
	})
	ip := map[string]string{"X-Client-IP": "198.51.100.40"}
	if code, _ := hs.doAs(t, "GET", "/v1/rulings?q=motor", "", ip); code != http.StatusOK {
		t.Fatalf("first search = %d", code)
	}
	cs := mcpClient(t, hs, ip)
	if res, _ := callTool(t, cs, "search_rulings", map[string]any{"query": "motor"}); !res.IsError || !strings.Contains(text(res), "429") {
		t.Errorf("search_rulings after the REST search used the bucket: %s", text(res))
	}
}

func TestMCPLookupTools(t *testing.T) {
	hs := newHarnessWith(t, "substantial_transformation", func(d *Deps) { d.RulingSearch = fakeSearch{} })
	cs := mcpClient(t, hs, nil)
	res, out := callTool(t, cs, "search_rulings", map[string]any{"query": "heat gun motor", "limit": 2})
	if res.IsError || !strings.Contains(text(res), "N300001") || len(out["rulings"].([]any)) > 2 {
		t.Errorf("search_rulings: %s", text(res))
	}
	res, out = callTool(t, cs, "get_ruling", map[string]any{"ruling_id": "N357771"})
	if res.IsError || out["url"] != "https://rulings.cbp.gov/ruling/N357771" || !strings.Contains(text(res), "Dear") {
		t.Errorf("get_ruling: %s %v", text(res), out)
	}
	res, out = callTool(t, cs, "get_duty_rates", map[string]any{"hts": "8419.89.95", "country": "Thailand"})
	if res.IsError {
		t.Fatalf("get_duty_rates: %s", text(res))
	}
	for _, r := range out["rates"].([]any) {
		if r.(map[string]any)["origin"] != "Thailand" {
			t.Errorf("country filter kept %v", r)
		}
	}
	if !strings.Contains(text(res), "From Thailand") {
		t.Errorf("duty summary: %s", text(res))
	}
	res, out = callTool(t, cs, "extract_product", map[string]any{"facts": "A heat gun assembled in Thailand from a Chinese motor."})
	if res.IsError || out["product"] == nil {
		t.Errorf("extract_product: %s", text(res))
	}
	// The demo resources and the prompt.
	rr, err := cs.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: "unde://demos"})
	if err != nil || len(rr.Contents) != 1 || !strings.Contains(rr.Contents[0].Text, "heatgun") {
		t.Errorf("demos resource: %v %v", err, rr)
	}
	if _, err := cs.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: "unde://demos/nope"}); err == nil {
		t.Error("unknown demo resource did not fail")
	}
	pr, err := cs.GetPrompt(context.Background(), &mcp.GetPromptParams{Name: "plan_production_move", Arguments: map[string]string{"product": "a heat gun", "target_country": "Thailand"}})
	if err != nil || len(pr.Messages) != 1 {
		t.Errorf("prompt: %v %v", err, pr)
	}
}
