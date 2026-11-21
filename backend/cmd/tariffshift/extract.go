package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/llm"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/tariffshift"
)

// ExtractPrompt names the extraction prompt below (D361).
const ExtractPrompt = "tariffshift-extract-v1"

const extractSystem = `You read a U.S. Customs (CBP) ruling text and copy out the facts a tariff-shift rule needs. You never classify anything yourself.

Return JSON:
- good_hts: the HTSUS code the text gives for the finished good, copied exactly as written (e.g. "8473.30.5100"), or "" if the text gives none.
- good_hts_quote: the shortest sentence fragment, copied verbatim from the text, that contains that code.
- country_of_production: the country where the finished good is made or assembled, as the text says; "" if not stated.
- materials: every component or raw material named as going into the good (not packaging, labels or instructions). For each: name (short, from the text); hts (the HTSUS code the text gives for THAT material, copied exactly, or "" if none is given for it; never a code given for another item, never "outside heading ..."); hts_quote (verbatim fragment containing that code, or ""); origin (the country the text says the material comes from, or "" if not stated).

If the text covers several goods, use the first one described. Copy codes and quotes exactly; do not infer, complete or correct a code.`

var extractSchema = map[string]any{
	"type": "object",
	"properties": map[string]any{
		"good_hts":              map[string]any{"type": "string"},
		"good_hts_quote":        map[string]any{"type": "string"},
		"country_of_production": map[string]any{"type": "string"},
		"materials": map[string]any{"type": "array", "items": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name":      map[string]any{"type": "string"},
				"hts":       map[string]any{"type": "string"},
				"hts_quote": map[string]any{"type": "string"},
				"origin":    map[string]any{"type": "string"},
			},
			"required": []string{"name", "hts", "hts_quote", "origin"},
		}},
	},
	"required": []string{"good_hts", "good_hts_quote", "country_of_production", "materials"},
}

type extracted struct {
	GoodHTS      string `json:"good_hts"`
	GoodHTSQuote string `json:"good_hts_quote"`
	Country      string `json:"country_of_production"`
	Materials    []struct {
		Name     string `json:"name"`
		HTS      string `json:"hts"`
		HTSQuote string `json:"hts_quote"`
		Origin   string `json:"origin"`
	} `json:"materials"`
}

// InputRow is one committed extraction: the rule input and its provenance.
type InputRow struct {
	Ruling  string            `json:"ruling"`
	Split   string            `json:"split"`
	Source  string            `json:"source"` // "facts" or "full_text"
	Input   tariffshift.Input `json:"input"`
	Quotes  map[string]string `json:"quotes,omitempty"`  // code -> verbatim quote
	Dropped []string          `json:"dropped,omitempty"` // codes whose quote failed the check
	Error   string            `json:"error,omitempty"`
	Model   string            `json:"model,omitempty"`
	Prompt  string            `json:"prompt"`
	CostUSD float64           `json:"cost_usd"`
}

func cmdExtract(args []string) error {
	fl := flag.NewFlagSet("extract", flag.ExitOnError)
	split := fl.String("split", "dev", "dev, final or final2")
	source := fl.String("source", "facts", "facts or full_text")
	raw := fl.String("raw", "", "directory of raw CROSS ruling JSON (full_text only; not committed)")
	model := fl.String("model", "gemini-3.8-flash", "gateway model")
	workers := fl.Int("workers", 4, "parallel calls")
	confirm := fl.Bool("confirm-heldout", false, "required for final and final2 (D361: once)")
	_ = fl.Parse(args)
	if *split != "dev" && !*confirm {
		return errors.New("held-out extraction runs once (D361); pass -confirm-heldout")
	}
	if *source == "full_text" && *raw == "" {
		return errors.New("-raw is required for full_text")
	}
	base, key := os.Getenv("LITELLM_BASE_URL"), os.Getenv("LITELLM_API_KEY")
	if base == "" || key == "" {
		return errors.New("needs LITELLM_BASE_URL and LITELLM_API_KEY")
	}
	ids, err := splitIDs(*split)
	if err != nil {
		return err
	}
	out := filepath.Join(evalDir, fmt.Sprintf("inputs-%s-%s.jsonl", *split, *source))
	if _, err := os.Stat(out); err == nil {
		return fmt.Errorf("%s exists; extraction runs once", out)
	}
	ll := llm.NewLiteLLM(base, key, 120*time.Second)
	ll.SkipCache(true)
	ctx := context.Background()

	rows := make([]InputRow, len(ids))
	var wg sync.WaitGroup
	sem := make(chan struct{}, *workers)
	for i, id := range ids {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			rows[i] = extractOne(ctx, ll, *model, id, *split, *source, *raw)
		}()
	}
	wg.Wait()

	f, err := os.Create(out)
	if err != nil {
		return err
	}
	w := bufio.NewWriter(f)
	errs, cost := 0, 0.0
	for _, r := range rows {
		if r.Error != "" {
			errs++
		}
		cost += r.CostUSD
		b, _ := json.Marshal(r)
		w.Write(append(b, '\n'))
	}
	if err := w.Flush(); err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	runlog := map[string]any{
		"run": fmt.Sprintf("tariffshift-extract-%s-%s", *split, *source), "prereg": "D361",
		"cases": len(ids), "errors": errs, "cost_usd": round4(cost), "model": *model,
		"prompt": ExtractPrompt, "llm_provider": "litellm", "llm_no_cache": true,
		"held_out": *split != "dev", "split": *split, "source": *source,
		"git_revision": revision(), "time": time.Now().Format(time.RFC3339),
		"output": "eval/tariffshift/" + filepath.Base(out),
	}
	if err := appendRunlog(runlog); err != nil {
		return err
	}
	fmt.Printf("%s: %d rulings, %d errors, $%.4f\n", out, len(ids), errs, cost)
	return nil
}

func extractOne(ctx context.Context, ll *llm.LiteLLM, model, id, split, source, rawDir string) InputRow {
	row := InputRow{Ruling: id, Split: split, Source: source, Prompt: ExtractPrompt, Model: model}
	text, err := readText(id, split, source, rawDir)
	if err != nil {
		row.Error = err.Error()
		return row
	}
	t := &llm.CostTally{}
	cctx := llm.ContextWithCostTally(ctx, t)
	var resp llm.Response
	for attempt := 0; attempt < 2; attempt++ {
		resp, err = ll.Generate(cctx, llm.Request{
			Model: model, System: extractSystem,
			Messages:        []llm.Message{{Role: llm.RoleUser, Content: text}},
			JSONSchema:      extractSchema,
			SchemaName:      "tariffshift_extract",
			MaxOutputTokens: 4096,
		})
		if err == nil {
			break
		}
	}
	row.CostUSD = round4(t.USD())
	if err != nil {
		row.Error = "model call failed: " + err.Error()
		return row
	}
	var x extracted
	if err := resp.DecodeJSON(&x); err != nil {
		row.Error = "bad JSON: " + err.Error()
		return row
	}
	norm := normText(text)
	row.Quotes = map[string]string{}
	check := func(code, quote string) string {
		code = strings.TrimSpace(code)
		if code == "" {
			return ""
		}
		d := tariffshift.Digits(code)
		q := normText(quote)
		if d == "" || q == "" || !strings.Contains(norm, q) || !strings.Contains(digitsOnly(q), d) {
			row.Dropped = append(row.Dropped, code)
			return ""
		}
		row.Quotes[code] = strings.TrimSpace(quote)
		return code
	}
	row.Input.GoodHTS = check(x.GoodHTS, x.GoodHTSQuote)
	row.Input.Country = strings.TrimSpace(x.Country)
	for _, m := range x.Materials {
		row.Input.Materials = append(row.Input.Materials, tariffshift.Material{
			Name: strings.TrimSpace(m.Name), HTS: check(m.HTS, m.HTSQuote), Origin: strings.TrimSpace(m.Origin),
		})
	}
	if len(row.Quotes) == 0 {
		row.Quotes = nil
	}
	return row
}

func normText(s string) string {
	s = strings.NewReplacer("“", `"`, "”", `"`, "’", "'", "‘", "'", " ", " ").Replace(s)
	return strings.ToLower(strings.Join(strings.Fields(s), " "))
}

func digitsOnly(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func readText(id, split, source, rawDir string) (string, error) {
	if source == "full_text" {
		b, err := os.ReadFile(filepath.Join(rawDir, id+".json"))
		if err != nil {
			return "", err
		}
		var r struct {
			Text string `json:"text"`
		}
		if err := json.Unmarshal(b, &r); err != nil {
			return "", err
		}
		if r.Text == "" {
			return "", fmt.Errorf("%s: empty text", id)
		}
		return r.Text, nil
	}
	dir := "data/facts"
	if split == "final2" {
		dir = "../eval/data/facts_final2"
	}
	b, err := os.ReadFile(filepath.Join(dir, id+".txt"))
	return string(b), err
}
