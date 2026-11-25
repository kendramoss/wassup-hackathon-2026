package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"slices"
	"sort"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/corpus"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/engine"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/retrieval"
)

// retrDiffCmd lists the cases of a split whose top-k BM25 precedents differ
// between two precedent pools (D868): -a the production pool, -b the same
// with more labels. No model calls. A case whose precedents are the same gets
// a byte-identical prompt, so only the listed cases can change.
func retrDiffCmd(args []string) error {
	fl := flag.NewFlagSet("retrdiff", flag.ExitOnError)
	a := fl.String("a", "../eval/data/labels.jsonl", "labels of pool A, comma-separated")
	b := fl.String("b", "../eval/data/labels.jsonl,data/labels_cited.jsonl", "labels of pool B, comma-separated")
	factsDir := fl.String("facts", "data/facts,data/facts_cited", "facts directories, comma-separated")
	splitsPath := fl.String("splits", "../eval/splits/splits_final.json", "splits file")
	split := fl.String("split", "dev", "split")
	k := fl.Int("k", 5, "precedents per case")
	out := fl.String("out", "", "write the JSON here (default stdout)")
	recency := fl.Bool("recency", false, "B is pool A re-ranked by recency (recency-v1, D904) instead of pool B")
	_ = fl.Parse(args)
	if isHeldOut(*split) {
		return fmt.Errorf("retrdiff reads development splits only")
	}
	dirs, err := existingDirs(*factsDir)
	if err != nil {
		return err
	}
	load := func(p string) (*corpus.Store, error) {
		ls, err := loadLabelFiles(p)
		if err != nil {
			return nil, err
		}
		return corpus.LoadDirs(ls, dirs...)
	}
	sa, err := load(*a)
	if err != nil {
		return err
	}
	sb, err := load(*b)
	if err != nil {
		return err
	}
	sp, err := corpus.LoadSplits(*splitsPath)
	if err != nil {
		return err
	}
	ids, err := sp.IDs(*split)
	if err != nil {
		return err
	}
	ia := retrieval.NewBM25(sa.Rulings(), engine.PrecedentText)
	ib := retrieval.NewBM25(sb.Rulings(), engine.PrecedentText)
	inA := map[string]bool{}
	for _, r := range sa.Rulings() {
		inA[r.ID] = true
	}
	type row struct {
		Ruling string   `json:"ruling"`
		A      []string `json:"a"`
		B      []string `json:"b"`
		Added  []string `json:"added_from_b"`
	}
	var changed []row
	n := 0
	for _, id := range ids {
		c, ok := sa.Get(id)
		if !ok {
			continue
		}
		n++
		cs := domain.Case{ID: c.ID, Date: c.Date}
		top := func(ix *retrieval.BM25) []string {
			var out []string
			for _, h := range ix.Top(c.Facts, *k, retrieval.PrecedentFilter(cs)) {
				out = append(out, h.Ruling.ID)
			}
			return out
		}
		ta, tb := top(ia), top(ib)
		if *recency {
			tb = nil
			for _, h := range retrieval.RecencyRerank(ia.Top(c.Facts, max(*k, retrieval.RecencyPool), retrieval.PrecedentFilter(cs)), c.Date, *k) {
				tb = append(tb, h.Ruling.ID)
			}
		}
		if !slices.Equal(ta, tb) {
			var added []string
			for _, x := range tb {
				if !inA[x] {
					added = append(added, x)
				}
			}
			changed = append(changed, row{id, ta, tb, added})
		}
	}
	sort.Slice(changed, func(i, j int) bool { return changed[i].Ruling < changed[j].Ruling })
	withAdded := 0
	for _, r := range changed {
		if len(r.Added) > 0 {
			withAdded++
		}
	}
	if *recency {
		*b = *a + " re-ranked by " + retrieval.RecencyVersion
	}
	res := map[string]any{"split": *split, "cases": n, "k": *k, "pool_a": *a, "pool_b": *b,
		"changed": len(changed), "changed_with_a_b_ruling": withAdded, "rows": changed}
	enc, _ := json.MarshalIndent(res, "", " ")
	enc = append(enc, '\n')
	if *out == "" {
		_, err = os.Stdout.Write(enc)
		return err
	}
	return os.WriteFile(*out, enc, 0o644)
}
