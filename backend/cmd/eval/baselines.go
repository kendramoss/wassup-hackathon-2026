package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sort"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/corpus"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/evalkit"
)

// baselinesCmd scores simple rules that need no model on a split: the most
// mentioned country in the facts (ties go to the first mentioned), the first
// and the last country mentioned, and always the most common held origin of
// the development split (fixed before the scored split is read). No model
// calls (D155).
func baselinesCmd(args []string) error {
	fl := flag.NewFlagSet("baselines", flag.ExitOnError)
	split := fl.String("split", "final", "split name")
	splitsPath := fl.String("splits", "../eval/splits/splits_final.json", "splits file")
	labelsPath := fl.String("labels", "../eval/data/labels.jsonl", "labels (comma-separated files)")
	factsDir := fl.String("facts", "data/facts", "facts directory (the committed bundle by default)")
	adjust := fl.String("adjust", "../eval/data/scoring_adjustments.json", "scoring adjustments")
	out := fl.String("out", "", "write the result as JSON here")
	majoritySplit := fl.String("majority-from", "dev", "split (of -majority-splits) whose most common held origin is the majority rule")
	majoritySplits := fl.String("majority-splits", "../eval/splits/splits_final.json", "splits file for -majority-from")
	_ = fl.Parse(args)

	labels, err := loadLabelFiles(*labelsPath)
	if err != nil {
		return err
	}
	store, err := corpus.LoadDirs(labels, splitComma(*factsDir)...)
	if err != nil {
		return err
	}
	splits, err := corpus.LoadSplits(*splitsPath)
	if err != nil {
		return err
	}
	ids, err := splits.IDs(*split)
	if err != nil {
		return err
	}
	adj, err := evalkit.LoadAdjustments(*adjust)
	if err != nil {
		return err
	}
	rules := []string{"most_mentioned", "first_mentioned", "last_mentioned", "majority_origin"}
	rows := map[string][]evalkit.Row{}
	var golds map[string]int
	type item struct {
		id, gold string
		counts   map[string]int
		order    []string
	}
	var items []item
	for _, id := range ids {
		r, ok := store.Get(id)
		if !ok {
			return fmt.Errorf("%s: no facts or label", id)
		}
		lab, _ := store.Label(id)
		gold := domain.NormalizeCountry(lab.Origin())
		counts, order := corpus.CountryMentions(r.Facts)
		items = append(items, item{id, gold, counts, order})
	}
	ms, err := corpus.LoadSplits(*majoritySplits)
	if err != nil {
		return err
	}
	mids, err := ms.IDs(*majoritySplit)
	if err != nil {
		return err
	}
	golds = map[string]int{}
	for _, id := range mids {
		if lab, ok := store.Label(id); ok && lab.Origin() != "" {
			golds[domain.NormalizeCountry(lab.Origin())]++
		}
	}
	majority := ""
	for g, n := range golds {
		if n > golds[majority] || (n == golds[majority] && g < majority) {
			majority = g
		}
	}
	for _, it := range items {
		preds := map[string]string{"majority_origin": majority}
		if len(it.order) > 0 {
			best := it.order[0]
			for _, c := range it.order {
				if it.counts[c] > it.counts[best] {
					best = c
				}
			}
			preds["most_mentioned"], preds["first_mentioned"], preds["last_mentioned"] = best, it.order[0], it.order[len(it.order)-1]
		}
		for _, rule := range rules {
			p := domain.NormalizeCountry(preds[rule])
			rows[rule] = append(rows[rule], evalkit.Row{Ruling: it.id, Gold: it.gold, Pred: p, Correct: p != "" && p == it.gold})
		}
	}
	res := map[string]any{"split": *split, "splits_file": *splitsPath, "facts": *factsDir, "majority_origin": majority}
	var names []string
	for _, rule := range rules {
		rs := adj.Apply(rows[rule])
		k := 0
		for _, r := range rs {
			if r.Correct {
				k++
			}
		}
		p, lo, hi := evalkit.Wilson(k, len(rs))
		res[rule] = map[string]any{"n": len(rs), "correct": k, "accuracy": round(p), "ci95": []float64{round(lo), round(hi)}}
		names = append(names, rule)
		fmt.Printf("%-16s %d / %d = %.1f%% (%.1f–%.1f)\n", rule, k, len(rs), 100*p, 100*lo, 100*hi)
	}
	sort.Strings(names)
	if *out != "" {
		b, _ := json.MarshalIndent(res, "", "  ")
		return os.WriteFile(*out, append(b, '\n'), 0o644)
	}
	return nil
}

// loadLabelFiles reads one or more comma-separated labels files in order.
func loadLabelFiles(paths string) ([]corpus.Label, error) {
	var all []corpus.Label
	for _, p := range splitComma(paths) {
		ls, err := corpus.LoadLabels(p)
		if err != nil {
			return nil, err
		}
		all = append(all, ls...)
	}
	return all, nil
}

func splitComma(s string) []string {
	var out []string
	start := 0
	for i := 0; i <= len(s); i++ {
		if i == len(s) || s[i] == ',' {
			if i > start {
				out = append(out, s[start:i])
			}
			start = i + 1
		}
	}
	return out
}
