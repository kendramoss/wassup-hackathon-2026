// Command mediacheck runs the image import (extract.ExtractMedia) on the
// rendered demo BOMs in eval/media/images and scores the draft against the
// CSV each image was drawn from: for every expected component, was its name
// read, and its country. Run from backend/:
//
//	LLM_PROVIDER=fake go run ./cmd/mediacheck      # wiring only; simulated rows
//	LLM_PROVIDER=vertex go run ./cmd/mediacheck    # live Gemini; needs ADC
//	LLM_PROVIDER=litellm go run ./cmd/mediacheck -out ../eval/results/<run>.json -runlog ../eval/results/RUNLOG.jsonl
//
// litellm is an OpenAI-compatible LiteLLM gateway to Gemini (needs
// LITELLM_BASE_URL and LITELLM_API_KEY). Like cmd/eval (D151), only this
// command accepts it; the server's config refuses it (D49). See D167.
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
	"runtime/debug"
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
	Style      string `json:"style,omitempty"`
	Components []struct {
		Name    string `json:"name"`
		Country string `json:"country"`
	} `json:"components"`
}

// Row is one part as the model read it: synthetic test data, so the rows
// are kept in the result file to show why a part was missed.
type Row struct {
	Name    string `json:"name"`
	Country string `json:"country"`
}

// Score is one image's result.
type Score struct {
	Image         string   `json:"image"`
	Style         string   `json:"style,omitempty"`
	Expected      int      `json:"expected"`
	NameRead      int      `json:"name_read"`
	CountryRight  int      `json:"country_right"`
	Extra         int      `json:"extra_rows"`
	Missed        []string `json:"missed,omitempty"`
	Read          []Row    `json:"read,omitempty"`
	Warnings      []string `json:"warnings,omitempty"`
	Simulated     bool     `json:"simulated"`
	InputTokens   int      `json:"input_tokens"`
	OutputTokens  int      `json:"output_tokens"`
	LatencyMS     int64    `json:"latency_ms"`
	PromptVersion string   `json:"prompt_version"`
	ModelID       string   `json:"model_id"`
	CostUSD       float64  `json:"cost_usd_gateway,omitempty"`
	Error         string   `json:"error,omitempty"`
}

func main() {
	dir := flag.String("dir", filepath.Join("..", "eval", "media", "images"), "folder with manifest.json and the images")
	out := flag.String("out", "", "write the scores as JSON here")
	runlog := flag.String("runlog", "", "append a summary line to this RUNLOG.jsonl")
	only := flag.String("only", "", "comma-separated image names to run (default all)")
	flag.Parse()
	if err := run(*dir, *out, *runlog, *only); err != nil {
		fmt.Fprintln(os.Stderr, "mediacheck:", err)
		os.Exit(1)
	}
}

// providerLiteLLM is accepted by this command and cmd/eval only (D151, D167).
const providerLiteLLM = "litellm"

// loadConfig reads the server's environment. LLM_PROVIDER=litellm is
// validated here, because the server's config refuses it.
func loadConfig() (config.Config, error) {
	if !strings.EqualFold(os.Getenv("LLM_PROVIDER"), providerLiteLLM) {
		return config.Load()
	}
	if os.Getenv("LITELLM_BASE_URL") == "" || os.Getenv("LITELLM_API_KEY") == "" {
		return config.Config{}, fmt.Errorf("LLM_PROVIDER=litellm needs LITELLM_BASE_URL and LITELLM_API_KEY")
	}
	_ = os.Setenv("LLM_PROVIDER", config.ProviderFake)
	cfg, err := config.Load()
	_ = os.Setenv("LLM_PROVIDER", providerLiteLLM)
	if err != nil {
		return config.Config{}, err
	}
	cfg.LLMProvider = providerLiteLLM
	return cfg, nil
}

func run(dir, out, runlog, only string) error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	ctx := context.Background()
	var model llm.Model
	switch cfg.LLMProvider {
	case providerLiteLLM:
		model = llm.NewLiteLLM(os.Getenv("LITELLM_BASE_URL"), os.Getenv("LITELLM_API_KEY"), 2*time.Minute)
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
	if only != "" {
		keep := map[string]bool{}
		for _, n := range strings.Split(only, ",") {
			keep[strings.TrimSpace(n)] = true
		}
		var sel []item
		for _, it := range items {
			if keep[it.Image] {
				sel = append(sel, it)
			}
		}
		items = sel
	}
	x := extract.New(model, cfg.EngineModel)
	var scores []Score
	var tot struct{ expected, names, countries, extra, errors int }
	var costUSD float64
	start := time.Now()
	for _, it := range items {
		data, err := os.ReadFile(filepath.Join(dir, it.Image))
		if err != nil {
			return err
		}
		info, err := media.Check(data)
		if err != nil {
			return fmt.Errorf("%s: %w", it.Image, err)
		}
		tally := &llm.CostTally{}
		d, err := x.ExtractMedia(llm.ContextWithCostTally(ctx, tally), []extract.File{{Info: info, Data: data}}, "", nil)
		costUSD += tally.USD()
		if err != nil {
			// A failed call scores zero for every part and the run goes on.
			s := Score{Image: it.Image, Style: it.Style, Expected: len(it.Components), CostUSD: tally.USD(), Error: err.Error()}
			for _, c := range it.Components {
				s.Missed = append(s.Missed, c.Name)
			}
			scores = append(scores, s)
			tot.expected += s.Expected
			tot.errors++
			fmt.Printf("%-28s error: %v\n", it.Image, err)
			continue
		}
		got := map[string]string{}
		var read []Row
		for _, r := range d.Components {
			got[norm(r.Name)] = r.MadeIn
			read = append(read, Row{Name: r.Name, Country: r.MadeIn})
		}
		s := Score{Image: it.Image, Style: it.Style, CostUSD: tally.USD(), Read: read, Warnings: d.Warnings, Expected: len(it.Components), Simulated: d.Simulated, InputTokens: d.Trace.InputTokens,
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
		tot.expected += s.Expected
		tot.names += s.NameRead
		tot.countries += s.CountryRight
		tot.extra += s.Extra
		fmt.Printf("%-28s names %2d/%-2d countries %2d/%-2d extra %d simulated=%v\n", it.Image, s.NameRead, s.Expected, s.CountryRight, s.Expected, s.Extra, s.Simulated)
	}
	summary := map[string]any{"images": len(items), "expected_parts": tot.expected, "names_read": tot.names,
		"countries_right": tot.countries, "extra_rows": tot.extra, "failed_calls": tot.errors,
		"cost_usd_gateway": costUSD}
	fmt.Printf("total: names %d/%d, countries %d/%d, extra rows %d, failed calls %d, gateway cost $%.4f\n",
		tot.names, tot.expected, tot.countries, tot.expected, tot.extra, tot.errors, costUSD)
	if out != "" {
		b, _ := json.MarshalIndent(map[string]any{"provider": cfg.LLMProvider, "model": cfg.EngineModel, "scores": scores,
			"summary": summary, "git_revision": revision(), "time": time.Now().Format(time.RFC3339),
			"note": "synthetic renderings of illustrative BOMs; illustrative only"}, "", " ")
		if err := os.WriteFile(out, append(b, '\n'), 0o644); err != nil {
			return err
		}
	}
	if runlog != "" {
		entry := map[string]any{"run": strings.TrimSuffix(filepath.Base(out), ".json"), "kind": "mediacheck",
			"llm_provider": cfg.LLMProvider, "engine_model": cfg.EngineModel, "images_dir": filepath.Base(dir),
			"held_out": false, "git_revision": revision(), "time": time.Now().Format(time.RFC3339),
			"wall_s": int(time.Since(start).Seconds())}
		for k, v := range summary {
			entry[k] = v
		}
		b, _ := json.Marshal(entry)
		f, err := os.OpenFile(runlog, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
		if err != nil {
			return err
		}
		defer f.Close()
		if _, err := f.Write(append(b, '\n')); err != nil {
			return err
		}
	}
	return nil
}

// revision is the VCS revision the binary was built from, "-dirty" when the
// tree had changes.
func revision() string {
	bi, ok := debug.ReadBuildInfo()
	if !ok {
		return "unknown"
	}
	rev, dirty := "", false
	for _, s := range bi.Settings {
		switch s.Key {
		case "vcs.revision":
			rev = s.Value
		case "vcs.modified":
			dirty = s.Value == "true"
		}
	}
	if rev == "" {
		return "unknown"
	}
	if dirty {
		rev += "-dirty"
	}
	return rev
}

func norm(s string) string { return strings.Join(strings.Fields(strings.ToLower(s)), " ") }
