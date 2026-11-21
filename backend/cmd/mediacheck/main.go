// Command mediacheck runs the image import (extract.ExtractMedia) on the
// rendered demo BOMs in eval/media/images and scores the draft against the
// CSV each image was drawn from: for every expected component, was its name
// read, and its country. Run from backend/:
//
//	LLM_PROVIDER=fake go run ./cmd/mediacheck      # wiring only; simulated rows
//	LLM_PROVIDER=vertex go run ./cmd/mediacheck    # live Gemini; needs ADC
//
// The images are synthetic renderings of illustrative BOMs (see
// eval/media/render_boms.py), so a score from them is illustrative, not a
// measure of accuracy on real scans or photos.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/config"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/extract"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/llm"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/media"
)

type item struct {
	Image      string `json:"image"`
	Source     string `json:"source"`
	Components []struct {
		Name    string `json:"name"`
		Country string `json:"country"`
	} `json:"components"`
}

// Score is one image's result.
type Score struct {
	Image         string   `json:"image"`
	Expected      int      `json:"expected"`
	NameRead      int      `json:"name_read"`
	CountryRight  int      `json:"country_right"`
	Extra         int      `json:"extra_rows"`
	Missed        []string `json:"missed,omitempty"`
	Simulated     bool     `json:"simulated"`
	InputTokens   int      `json:"input_tokens"`
	OutputTokens  int      `json:"output_tokens"`
	LatencyMS     int64    `json:"latency_ms"`
	PromptVersion string   `json:"prompt_version"`
	ModelID       string   `json:"model_id"`
}

func main() {
	dir := flag.String("dir", filepath.Join("..", "eval", "media", "images"), "folder with manifest.json and the images")
	out := flag.String("out", "", "write the scores as JSON here")
	flag.Parse()
	if err := run(*dir, *out); err != nil {
		fmt.Fprintln(os.Stderr, "mediacheck:", err)
		os.Exit(1)
	}
}

func run(dir, out string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	ctx := context.Background()
	var model llm.Model
	switch cfg.LLMProvider {
	case config.ProviderFake:
		model = llm.FakeText(extract.SimulatedMediaAnswer)
	case config.ProviderVertex:
		v, err := llm.NewVertex(ctx, llm.VertexOptions{Project: cfg.GCPProject, Location: cfg.GCPLocation, Timeout: 2 * time.Minute})
		if err != nil {
			return err
		}
		model = v
	default:
		return fmt.Errorf("LLM_PROVIDER %q is not supported here", cfg.LLMProvider)
	}
	b, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil {
		return err
	}
	var items []item
	if err := json.Unmarshal(b, &items); err != nil {
		return err
	}
	x := extract.New(model, cfg.EngineModel)
	var scores []Score
	for _, it := range items {
		data, err := os.ReadFile(filepath.Join(dir, it.Image))
		if err != nil {
			return err
		}
		info, err := media.Check(data)
		if err != nil {
			return fmt.Errorf("%s: %w", it.Image, err)
		}
		d, err := x.ExtractMedia(ctx, []extract.File{{Info: info, Data: data}}, "", nil)
		if err != nil {
			return fmt.Errorf("%s: %w", it.Image, err)
		}
		got := map[string]string{}
		for _, r := range d.Components {
			got[norm(r.Name)] = r.MadeIn
		}
		s := Score{Image: it.Image, Expected: len(it.Components), Simulated: d.Simulated, InputTokens: d.Trace.InputTokens,
			OutputTokens: d.Trace.OutputTokens, LatencyMS: d.Trace.LatencyMS, PromptVersion: d.Trace.PromptVersion, ModelID: d.Trace.ModelID}
		matched := 0
		for _, c := range it.Components {
			country, ok := got[norm(c.Name)]
			if !ok {
				s.Missed = append(s.Missed, c.Name)
				continue
			}
			matched++
			s.NameRead++
			if country == domain.NormalizeCountry(c.Country) {
				s.CountryRight++
			}
		}
		s.Extra = len(d.Components) - matched
		scores = append(scores, s)
		fmt.Printf("%-22s names %2d/%-2d countries %2d/%-2d extra %d simulated=%v\n", it.Image, s.NameRead, s.Expected, s.CountryRight, s.Expected, s.Extra, s.Simulated)
	}
	if out != "" {
		b, _ := json.MarshalIndent(map[string]any{"provider": cfg.LLMProvider, "model": cfg.EngineModel, "scores": scores,
			"note": "synthetic renderings of illustrative BOMs; illustrative only"}, "", " ")
		return os.WriteFile(out, append(b, '\n'), 0o644)
	}
	return nil
}

func norm(s string) string { return strings.Join(strings.Fields(strings.ToLower(s)), " ") }
