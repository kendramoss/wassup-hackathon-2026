package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/corpus"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/familycheck"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/planner"
)

// flipsFamilyCmd runs the product's family check (D138) on the starting
// setup (variant A) of every post-cutoff flip pair (D170), the step that
// decides whether the plan search has a template for the product. One row
// per pair; the file is written once.
func flipsFamilyCmd(args []string) error {
	fl := flag.NewFlagSet("flipsfamily", flag.ExitOnError)
	flipsPath := fl.String("flips", "../eval/data/flips_post.jsonl", "flip pairs")
	labelsPaths := fl.String("labels", "../eval/data/labels.jsonl,../eval/data/labels_final2.jsonl", "label files (for each ruling's subject)")
	resultsDir := fl.String("results", "../eval/results", "results directory")
	model := fl.String("model", "gemini-3.8-flash", "family-check model")
	_ = fl.Parse(args)
	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	m, err := newModel(ctx, cfg, slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn})))
	if err != nil {
		return err
	}
	tmpls, err := planner.LoadTemplates()
	if err != nil {
		return err
	}
	checker := familycheck.New(m, *model, tmpls)
	subjects := map[string]string{}
	for _, p := range strings.Split(*labelsPaths, ",") {
		ls, err := corpus.LoadLabels(p)
		if err != nil {
			return err
		}
		for _, l := range ls {
			subjects[l.Ruling] = l.Subject
		}
	}
	items, err := readFlipItems(*flipsPath)
	if err != nil {
		return err
	}
	path := filepath.Join(*resultsDir, "planner-post-familycheck-"+cfg.LLMProvider+".jsonl")
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("%s exists; the post-cutoff pairs are checked once", path)
	}
	out, err := os.Create(path)
	if err != nil {
		return err
	}
	defer out.Close()
	w := bufio.NewWriter(out)
	var cost float64
	for _, it := range items {
		r := domain.Ruling{ID: it.VariantA.Ruling, Subject: subjects[it.VariantA.Ruling], Facts: it.VariantA.Facts}
		row := checkOne(ctx, checker, r, *model)
		cost += row.CostUSD
		b, _ := json.Marshal(struct {
			Pair string `json:"pair"`
			familyCovRow
		}{it.ID, row})
		_, _ = w.Write(append(b, '\n'))
		fmt.Printf("- %s (%s): %s %.2f covered=%t %s\n", it.ID, it.VariantA.Ruling, row.Family, row.Confidence, row.Covered, row.Error)
	}
	fmt.Printf("- Cost: $%.4f (promo)\n- Saved: %s\n", cost, path)
	return w.Flush()
}

func readFlipItems(path string) ([]flipItem, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var items []flipItem
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 16<<20)
	for sc.Scan() {
		var it flipItem
		if err := json.Unmarshal(sc.Bytes(), &it); err != nil {
			return nil, err
		}
		items = append(items, it)
	}
	return items, sc.Err()
}
