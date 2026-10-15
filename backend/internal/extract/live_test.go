package extract

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/config"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/llm"
)

// TestLiveHeatGun runs only with EXTRACT_LIVE_MODEL set (development).
func TestLiveHeatGun(t *testing.T) {
	m := os.Getenv("EXTRACT_LIVE_MODEL")
	if m == "" {
		t.Skip("EXTRACT_LIVE_MODEL not set")
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	model := llm.NewLiteLLM(cfg.LiteLLMBaseURL, cfg.LiteLLMAPIKey, cfg.LLMTimeout)
	var rec struct{ Facts string }
	b, _ := os.ReadFile("../../../eval/results/predict-heatgun-2022-litellm.json")
	_ = json.Unmarshal(b, &rec)
	start := time.Now()
	d, err := New(model, m).Extract(context.Background(), rec.Facts, "", tmpl(t))
	if err != nil {
		t.Fatal(err)
	}
	th, cn := 0, 0
	for _, c := range d.Components {
		switch c.MadeIn {
		case "Thailand":
			th++
		case "China":
			cn++
		}
	}
	t.Logf("%s: %.1fs, components TH %d CN %d (total %d), operations %d, warnings %d", m, time.Since(start).Seconds(), th, cn, len(d.Components), len(d.Operations), len(d.Warnings))
	for _, o := range d.Operations {
		t.Logf("  op p%d %s %s: %.80s", o.SourceParagraph, o.MadeIn, o.Group, o.Name)
	}
}
