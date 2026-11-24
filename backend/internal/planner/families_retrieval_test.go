package planner

import (
	"context"
	"encoding/json"
	"flag"
	"os"
	"slices"
	"testing"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/corpus"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/engine"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/retrieval"
)

var updateRetrieval = flag.Bool("update-b2-retrieval", false, "rewrite eval/families/b2-retrieval.json (D781 M1)")

// b2Template is the template each frozen D781 family's development cases are
// checked against; the two optical cases use the lens template.
var b2Template = map[string]string{
	"batteries": "power_supplies_chargers", "machinery": "motors_pumps_compressors",
	"furniture": "furniture", "medical_optical": "medical_devices",
}
var b2Optical = []string{"N350680", "N350838"}

// D781 M1: for each frozen development case of tracks 3-6, does BM25 as in
// production (the bundled single-origin corpus, engine.PrecedentText,
// retrieval.PrecedentFilter) put a ruling its family's template cites in the
// top 5 or top 20, with the version 1 citations and with the current ones?
// Pinned in eval/families/b2-retrieval.json; -update-b2-retrieval rewrites it.
func TestB2RetrievalOfCitedPrecedents(t *testing.T) {
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
	var cases struct {
		Families map[string]struct {
			RegressionDev []string `json:"regression_dev"`
		} `json:"families"`
	}
	readJSON(t, "../../../eval/families/b2-cases.json", &cases)
	var v1 struct {
		Templates map[string]struct{ Cited []string } `json:"templates"`
	}
	readJSON(t, "../../../eval/families/b2-v1-citations.json", &v1)
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
	set := func(ids []string) map[string]bool {
		out := map[string]bool{}
		for _, id := range ids {
			out[id] = true
		}
		return out
	}
	bm := retrieval.NewBM25(store.Rulings(), engine.PrecedentText)
	type row struct {
		Case     string   `json:"case"`
		Template string   `json:"template"`
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
	for _, fam := range []string{"batteries", "machinery", "furniture", "medical_optical"} {
		for _, id := range cases.Families[fam].RegressionDev {
			tm, key := b2Template[fam], fam
			if slices.Contains(b2Optical, id) {
				tm, key = "display_camera_modules", "optical"
			} else if fam == "medical_optical" {
				key = "medical"
			}
			if sums[key] == nil {
				sums[key] = &sum{}
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
			before, after := set(v1.Templates[tm].Cited), cited(tm)
			r := row{Case: id, Template: tm}
			for i, h := range hits {
				if i < 5 {
					r.Top5 = append(r.Top5, h.Ruling.ID)
					r.Before5 = r.Before5 || before[h.Ruling.ID]
					r.After5 = r.After5 || after[h.Ruling.ID]
				}
				r.Before20 = r.Before20 || before[h.Ruling.ID]
				r.After20 = r.After20 || after[h.Ruling.ID]
			}
			s := sums[key]
			s.N++
			s.Before5 += b2i(r.Before5)
			s.After5 += b2i(r.After5)
			s.Before20 += b2i(r.Before20)
			s.After20 += b2i(r.After20)
			rows[key] = append(rows[key], r)
		}
	}
	out, _ := json.MarshalIndent(map[string]any{
		"note":    "D781 M1: development cases (b2-cases.json) whose BM25 top 5 or top 20, as in production, holds a ruling their family's template cites; before = version 1 citations (b2-v1-citations.json), after = current templates. Descriptive only. Written by TestB2RetrievalOfCitedPrecedents -update-b2-retrieval.",
		"summary": sums, "cases": rows,
	}, "", " ")
	out = append(out, '\n')
	const path = "../../../eval/families/b2-retrieval.json"
	if *updateRetrieval {
		if err := os.WriteFile(path, out, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(want) != string(out) {
		t.Errorf("%s is stale; run go test ./internal/planner -run TestB2Retrieval -update-b2-retrieval", path)
	}
}

func b2i(b bool) int {
	if b {
		return 1
	}
	return 0
}

func readJSON(t *testing.T, path string, v any) {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(b, v); err != nil {
		t.Fatal(err)
	}
}
