package main

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/blob"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/corpus"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/engine"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/llm"
)

// The pools below are built the way run() builds them (corpus.Load, then the
// textile reasoning overlay, the pharma id subset, the USMCA label file), from
// a temp corpus and a fake model: no real corpus, no paid call.

var citeRef = regexp.MustCompile(`[NH][0-9]{6}`)

func citeFirstFake() *llm.Fake {
	return &llm.Fake{Respond: func(req llm.Request) (llm.Response, error) {
		id := citeRef.FindString(req.Messages[0].Content)
		return llm.Response{Text: `{"origin_country":"Thailand","essential_character":"motor","decisive_step":"like ` + id + `","flip":null,"confidence":0.9,"reasoning":"see ` + id + `"}`}, nil
	}}
}

func writeLabels(t *testing.T, path string, ids ...string) {
	t.Helper()
	var b []byte
	for _, id := range ids {
		b = append(b, []byte(`{"ruling":"`+id+`","date":"2020-01-01","collection":"c","subject":"motor","regime":"substantial_transformation","multi_country":false,"method":"manual","primary_origin":"Thailand","evidence":["The country of origin is Thailand."]}`+"\n")...)
	}
	if err := os.WriteFile(path, b, 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeFacts(t *testing.T, dir, id, facts string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, id+".txt"), []byte(facts+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func loadPool(t *testing.T, labelsPath, factsDir string) *corpus.Store {
	t.Helper()
	labels, err := corpus.LoadLabels(labelsPath)
	if err != nil {
		t.Fatal(err)
	}
	s, err := corpus.Load(labels, factsDir)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestServerEvidenceRetrieversAreRevisionedAndCacheTracksCorrections(t *testing.T) {
	root := t.TempDir()
	facts := filepath.Join(root, "facts")
	if err := os.MkdirAll(facts, 0o755); err != nil {
		t.Fatal(err)
	}
	labels := filepath.Join(root, "labels.jsonl")
	ulabels := filepath.Join(root, "labels_usmca.jsonl")
	writeLabels(t, labels, "N100001", "N100002", "N100003")
	writeLabels(t, ulabels, "N100003")
	writeFacts(t, facts, "N100001", "motor made in Thailand")
	writeFacts(t, facts, "N100002", "gearbox made in Mexico")
	writeFacts(t, facts, "N100003", "pump housing cast in Canada")

	// shapes mirror the four server sites in run()
	shapes := map[string]func(t *testing.T) []domain.Ruling{
		"main": func(t *testing.T) []domain.Ruling { return loadPool(t, labels, facts).Rulings() },
		"textile": func(t *testing.T) []domain.Ruling { // subset, then reasoning overlay
			p := loadPool(t, labels, facts)
			var sub []domain.Ruling
			for _, id := range []string{"N100001", "N100002"} {
				if r, ok := p.Get(id); ok {
					sub = append(sub, r)
				}
			}
			return corpus.WithReasoning(sub, map[string]string{"N100001": "reasoning one"})
		},
		"usmca": func(t *testing.T) []domain.Ruling { return loadPool(t, ulabels, facts).Rulings() },
		"pharma": func(t *testing.T) []domain.Ruling {
			p := loadPool(t, labels, facts)
			var sub []domain.Ruling
			for _, id := range []string{"N100002", "N100003"} {
				if r, ok := p.Get(id); ok {
					sub = append(sub, r)
				}
			}
			return sub
		},
	}
	c := domain.Case{ID: "case_ev", Date: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), Facts: "motor made in Thailand"}
	opts := engine.Options{Model: "m", K: 2}
	for name, build := range shapes {
		t.Run(name, func(t *testing.T) {
			rs := build(t)
			r1 := evidenceBM25(rs)
			if r1.Revision() == "" || r1.Revision() != corpus.RevisionOf(rs) {
				t.Fatalf("retriever revision %q, want RevisionOf of the exact indexed slice", r1.Revision())
			}
			f := citeFirstFake()
			dir := blob.Dir{Dir: t.TempDir()}
			first := engine.NewCache(engine.New(f, r1, opts), dir)
			if _, err := first.Predict(context.Background(), c); err != nil {
				t.Fatal(err)
			}
			// unchanged evidence, fresh process-shaped rebuild: replay
			again := engine.NewCache(engine.New(f, evidenceBM25(build(t)), opts), dir)
			if r, err := again.Predict(context.Background(), c); err != nil || !r.Prediction.Trace.Cached || len(f.Calls()) != 1 {
				t.Fatalf("unchanged evidence did not hit: cached=%v calls=%d err=%v", r.Prediction.Trace.Cached, len(f.Calls()), err)
			}
			// a corrected precedent in the same shape: miss
			corrected := append([]domain.Ruling(nil), rs...)
			corrected[0].Facts += " (corrected)"
			miss := engine.NewCache(engine.New(f, evidenceBM25(corrected), opts), dir)
			if r, err := miss.Predict(context.Background(), c); err != nil || r.Prediction.Trace.Cached || len(f.Calls()) != 2 {
				t.Fatalf("corrected evidence replayed: cached=%v calls=%d err=%v", r.Prediction.Trace.Cached, len(f.Calls()), err)
			}
		})
	}

	// the reasoning overlay is part of the evidence: same facts, new reasoning
	rsA := shapes["textile"](t)
	rsB := corpus.WithReasoning(rsA, map[string]string{"N100001": "different reasoning"})
	if evidenceBM25(rsA).Revision() == evidenceBM25(rsB).Revision() {
		t.Error("reasoning change did not change the revision")
	}
	// and the Load-time store revision must not be what a subset is stamped with
	p := loadPool(t, labels, facts)
	if sub := shapes["pharma"](t); evidenceBM25(sub).Revision() == p.Revision() {
		t.Error("subset retriever carries the whole-store revision")
	}
}

// run() must build every engine retriever through evidenceBM25: a raw
// retrieval.NewBM25 would leave that engine unversioned, which silently turns
// its persisted prediction cache off (#316).
func TestServerBuildsEveryEngineRetrieverThroughEvidenceBM25(t *testing.T) {
	b, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatal(err)
	}
	src := string(b)
	if n := strings.Count(src, "retrieval.NewBM25("); n != 1 { // the one inside evidenceBM25
		t.Errorf("main.go calls retrieval.NewBM25 %d times; engines must use evidenceBM25", n)
	}
	if n := strings.Count(src, "evidenceBM25("); n != 5 { // 4 engines + the definition
		t.Errorf("evidenceBM25 appears %d times, want 5 (main, textile, usmca, pharma + definition)", n)
	}
}
