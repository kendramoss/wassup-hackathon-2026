package planner

import (
	"context"
	"encoding/json"
	"flag"
	"os"
	"slices"
	"sort"
	"testing"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/corpus"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/engine"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/retrieval"
)

var updateB3Retrieval = flag.Bool("update-b3-retrieval", false, "rewrite eval/families/b3-retrieval.json (D820 M1)")

// b3Before is the template a D822 family's cases were checked against before
// it existed: the machines template for lifting machines, the display and
// lens template for optical instruments.
var b3Before = map[string]string{
	"lifting_handling_machines": "motors_pumps_compressors", "optical_precision_instruments": "display_camera_modules",
}

// D820 M1: for each frozen development case (b3-dev-splits.json), does BM25
// as in production (the bundled single-origin corpus, engine.PrecedentText,
// retrieval.PrecedentFilter) put a ruling cited by the case's expected
// template in the top 5 or top 20, with main's templates before D822
// (b3-before-citations.json) and with the current ones? Pinned in
// eval/families/b3-retrieval.json; -update-b3-retrieval rewrites it.
func TestB3RetrievalOfCitedPrecedents(t *testing.T) {
	if testing.Short() {
		t.Skip("indexes the corpus")
	}
	labels, err := corpus.LoadLabels("../../data/labels.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	store, err := corpus.Load(labels, "../../data/facts")
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string]corpus.Label{}
	for _, l := range labels {
		byID[l.Ruling] = l
	}
	var dev struct {
		Dev      []string          `json:"dev"`
		Expected map[string]string `json:"expected"`
	}
	readJSON(t, "../../../eval/families/b3-dev-splits.json", &dev)
	var before struct {
		Templates map[string]struct{ Cited []string } `json:"templates"`
	}
	readJSON(t, "../../../eval/families/b3-before-citations.json", &before)
	tmpls, err := LoadTemplates()
	if err != nil {
		t.Fatal(err)
	}
	cited := func(fam string) map[string]bool {
		out := map[string]bool{}
		for _, g := range tmpls[fam].Groups {
			if g.Precedent != nil {
				for _, id := range slices.Concat(g.Precedent.ConfersOrigin, g.Precedent.NotEnough) {
					out[id] = true
				}
			}
		}
		return out
	}
	bm := retrieval.NewBM25(store.Rulings(), engine.PrecedentText)
	type row struct {
		Case     string   `json:"case"`
		Before   string   `json:"before_template"`
		Before5  bool     `json:"before_top5"`
		After5   bool     `json:"after_top5"`
		Before20 bool     `json:"before_top20"`
		After20  bool     `json:"after_top20"`
		Top5     []string `json:"top5"`
	}
	type sum struct {
		N        int `json:"n"`
		Before5  int `json:"before_top5"`
		After5   int `json:"after_top5"`
		Before20 int `json:"before_top20"`
		After20  int `json:"after_top20"`
	}
	rows := map[string][]row{}
	sums := map[string]*sum{}
	ids := slices.Clone(dev.Dev)
	sort.Strings(ids)
	for _, id := range ids {
		fam := dev.Expected[id]
		if _, ok := tmpls[fam]; !ok {
			t.Fatalf("%s: expected template %q does not exist", id, fam)
		}
		bfam := fam
		if b, ok := b3Before[fam]; ok {
			bfam = b
		}
		if sums[fam] == nil {
			sums[fam] = &sum{}
		}
		l := byID[id]
		date, err := l.Time()
		if err != nil {
			t.Fatal(err)
		}
		b, err := os.ReadFile("../../data/facts/" + id + ".txt")
		if err != nil {
			t.Fatal(err)
		}
		c := domain.Case{ID: id, Date: date, Facts: string(b)}
		hits, err := bm.Search(context.Background(), c.Facts, 20, retrieval.PrecedentFilter(c))
		if err != nil {
			t.Fatal(err)
		}
		bset := map[string]bool{}
		for _, x := range before.Templates[bfam].Cited {
			bset[x] = true
		}
		after := cited(fam)
		r := row{Case: id, Before: bfam}
		for i, h := range hits {
			if i < 5 {
				r.Top5 = append(r.Top5, h.Ruling.ID)
				r.Before5 = r.Before5 || bset[h.Ruling.ID]
				r.After5 = r.After5 || after[h.Ruling.ID]
			}
			r.Before20 = r.Before20 || bset[h.Ruling.ID]
			r.After20 = r.After20 || after[h.Ruling.ID]
		}
		s := sums[fam]
		s.N++
		s.Before5 += b2i(r.Before5)
		s.After5 += b2i(r.After5)
		s.Before20 += b2i(r.Before20)
		s.After20 += b2i(r.After20)
		rows[fam] = append(rows[fam], r)
	}
	out, _ := json.MarshalIndent(map[string]any{
		"note":    "D820 M1: frozen development cases (b3-dev-splits.json) whose BM25 top 5 or top 20, as in production, holds a ruling cited by the expected template; before = main's templates before D822 (b3-before-citations.json; lifting machines against motors_pumps_compressors, optical instruments against display_camera_modules), after = current templates. Descriptive only. Written by TestB3RetrievalOfCitedPrecedents -update-b3-retrieval.",
		"summary": sums, "cases": rows,
	}, "", " ")
	out = append(out, '\n')
	const path = "../../../eval/families/b3-retrieval.json"
	if *updateB3Retrieval {
		if err := os.WriteFile(path, out, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(want) != string(out) {
		t.Errorf("%s is stale; run go test ./internal/planner -run TestB3Retrieval -update-b3-retrieval", path)
	}
}
