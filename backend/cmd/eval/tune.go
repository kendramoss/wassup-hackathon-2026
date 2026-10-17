package main

// Supervised tuning data for the origin engine (#38, D38).
//
//	eval tunesplit  writes eval/splits/splits_tune.json: labelled
//	                single-origin substantial-transformation rulings dated
//	                before the dev window, minus every dev, final and pilot
//	                ruling, split into train and tune_check (~10%, by ID hash).
//	eval run -split train -splits ../eval/splits/splits_tune.json
//	                the engine's answers on the training rulings (model calls).
//	eval tunedata   turns those answers into Vertex AI supervised tuning JSONL:
//	                the engine's exact system prompt and user message (facts
//	                plus the 5 dated precedents), and as the target the
//	                engine's JSON answer with CBP's held origin and CBP's own
//	                reasoning.

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"hash/fnv"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/corpus"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/engine"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/evalkit"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/retrieval"
)

// tuneCheckPercent of eligible rulings are held out for tuning checks, so dev
// is never used to choose epochs.
const tuneCheckPercent = 10

// absentCorrectedConfidence is the target confidence when the engine named
// the wrong origin, so its recorded confidence is not a confidence in CBP's
// origin (D56). A constant 0.9 would teach one confidence on every correction.
const absentCorrectedConfidence = 0.75

func tuneSplitCmd(args []string) error {
	fl := flag.NewFlagSet("tunesplit", flag.ExitOnError)
	labelsPath := fl.String("labels", "../eval/data/labels.jsonl", "labels file")
	splitsPath := fl.String("splits", "../eval/splits/splits_final.json", "frozen eval splits")
	factsDir := fl.String("facts", "data/facts", "facts directory")
	out := fl.String("out", "../eval/splits/splits_tune.json", "output")
	_ = fl.Parse(args)

	labels, err := corpus.LoadLabels(*labelsPath)
	if err != nil {
		return err
	}
	store, err := corpus.Load(labels, *factsDir)
	if err != nil {
		return err
	}
	splits, err := corpus.LoadSplits(*splitsPath)
	if err != nil {
		return err
	}
	train, check, cutoff, err := tuneSplit(labels, store, splits)
	if err != nil {
		return err
	}
	s := corpus.Splits{FrozenAt: time.Now().Format(time.RFC3339), Train: train, TuneCheck: check}
	b, _ := json.MarshalIndent(struct {
		corpus.Splits
		Rule string `json:"rule"`
	}{s, fmt.Sprintf("labelled single-origin substantial-transformation rulings with facts, dated before %s (first dev/final/pilot ruling), minus every dev, final and pilot ruling; %d%% tune_check by FNV-1a of the ruling ID",
		cutoff.Format(time.DateOnly), tuneCheckPercent)}, "", "  ")
	if err := os.WriteFile(*out, append(b, '\n'), 0o644); err != nil {
		return err
	}
	fmt.Printf("train %d, tune_check %d, before %s -> %s\n", len(train), len(check), cutoff.Format(time.DateOnly), *out)
	return nil
}

// tuneSplit picks the tuning rulings: in the precedent pool (single origin,
// facts present), substantial transformation, dated strictly before the
// earliest eval ruling, and not an eval ruling.
func tuneSplit(labels []corpus.Label, store *corpus.Store, s corpus.Splits) (train, check []string, cutoff time.Time, err error) {
	eval := map[string]bool{}
	for _, id := range slices.Concat(s.Dev, s.Final, s.Pilot) {
		eval[id] = true
		l, ok := store.Label(id)
		if !ok {
			continue
		}
		if t, _ := l.Time(); cutoff.IsZero() || t.Before(cutoff) {
			cutoff = t
		}
	}
	if cutoff.IsZero() {
		return nil, nil, cutoff, errors.New("tunesplit: no dated eval rulings")
	}
	for _, l := range labels {
		r, ok := store.Get(l.Ruling)
		if !ok || eval[l.Ruling] || l.Regime != "substantial_transformation" || !r.Date.Before(cutoff) {
			continue
		}
		h := fnv.New32a()
		h.Write([]byte(l.Ruling))
		if h.Sum32()%100 < tuneCheckPercent {
			check = append(check, l.Ruling)
		} else {
			train = append(train, l.Ruling)
		}
	}
	return train, check, cutoff, nil
}

// tuneExample is one Vertex AI supervised tuning example for Gemini.
type tuneExample struct {
	SystemInstruction tuneContent   `json:"systemInstruction"`
	Contents          []tuneContent `json:"contents"`
}

type tuneContent struct {
	Role  string     `json:"role"`
	Parts []tunePart `json:"parts"`
}

type tunePart struct {
	Text string `json:"text"`
}

// tuneTarget has the engine's answer shape (engine.Schema), in its key order.
type tuneTarget struct {
	Origin             string  `json:"origin_country"`
	EssentialCharacter string  `json:"essential_character"`
	DecisiveStep       string  `json:"decisive_step"`
	Flip               *string `json:"flip"`
	Confidence         float64 `json:"confidence"`
	Reasoning          string  `json:"reasoning"`
}

func tuneDataCmd(args []string) error {
	fl := flag.NewFlagSet("tunedata", flag.ExitOnError)
	answers := fl.String("answers", "", "engine run on the train and tune_check splits (eval run -split train|tune_check -splits ../eval/splits/splits_tune.json)")
	labelsPath := fl.String("labels", "../eval/data/labels.jsonl", "labels file")
	splitsPath := fl.String("splits", "../eval/splits/splits_tune.json", "tuning splits (eval tunesplit)")
	factsDir := fl.String("facts", "data/facts", "facts directory")
	rawDir := fl.String("raw", "../data/corpus/raw", "raw ruling JSON (go run ./cmd/corpus fetch), for CBP's reasoning")
	k := fl.Int("k", 5, "precedents per case, as the engine")
	outDir := fl.String("out", "../data/tuning", "writes train.jsonl and tune_check.jsonl here (gitignored; upload to the corpus bucket)")
	_ = fl.Parse(args)
	if *answers == "" {
		return errors.New("tunedata: -answers is required (the targets start from the engine's own answers)")
	}

	labels, err := corpus.LoadLabels(*labelsPath)
	if err != nil {
		return err
	}
	store, err := corpus.Load(labels, *factsDir)
	if err != nil {
		return err
	}
	// Precedents are retrieved from the store before reasoning is attached,
	// so the user message is exactly the engine-v1 one.
	bm := retrieval.NewBM25(store.Rulings(), engine.PrecedentText)
	reasoned, err := corpus.Load(labels, *factsDir)
	if err != nil {
		return err
	}
	if n, err := reasoned.AttachReasoning(*rawDir); err != nil {
		return err
	} else if n == 0 {
		return fmt.Errorf("tunedata: no CBP reasoning found in %s (run: go run ./cmd/corpus fetch)", *rawDir)
	}
	splits, err := corpus.LoadSplits(*splitsPath)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(*outDir, 0o755); err != nil {
		return err
	}
	for _, name := range []string{"train", "tune_check"} {
		ids, _ := splits.IDs(name)
		rows, err := readRows(*answers, ids)
		if err != nil {
			return err
		}
		out := filepath.Join(*outDir, name+".jsonl")
		st, err := writeTuneData(context.Background(), out, rows, store, reasoned, bm, *k)
		if err != nil {
			return err
		}
		fmt.Printf("%s: %d examples (%d corrected to CBP's origin), skipped %d without reasoning, %d errored, %d precedent mismatches -> %s\n",
			name, st.written, st.corrected, st.noReasoning, st.errored, st.mismatch, out)
	}
	return nil
}

type tuneStats struct{ written, corrected, noReasoning, errored, mismatch int }

func writeTuneData(ctx context.Context, path string, rows []evalkit.Row, store, reasoned *corpus.Store, retr retrieval.Retriever, k int) (tuneStats, error) {
	var st tuneStats
	f, err := os.Create(path)
	if err != nil {
		return st, err
	}
	defer f.Close()
	w := bufio.NewWriter(f)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	for _, row := range rows {
		if row.Error != "" || row.Pred == "" {
			st.errored++
			continue
		}
		r, ok := store.Get(row.Ruling)
		rr, _ := reasoned.Get(row.Ruling)
		if !ok || rr.Reasoning == "" {
			st.noReasoning++
			continue
		}
		c := domain.Case{ID: r.ID, Date: r.Date, Facts: r.Facts}
		hits, err := retr.Search(ctx, c.Facts, k, retrieval.PrecedentFilter(c))
		if err != nil {
			return st, err
		}
		precedents := make([]domain.Ruling, len(hits))
		ids := make([]string, len(hits))
		for i, h := range hits {
			precedents[i], ids[i] = h.Ruling, h.Ruling.ID
		}
		if len(row.Precedents) > 0 && !slices.Equal(ids, row.Precedents) {
			st.mismatch++ // the run used other precedents than this rebuild would show
		}
		target := tuneTargetFor(row, r, rr.Reasoning)
		if target.Origin != domain.NormalizeCountry(row.Pred) {
			st.corrected++
		}
		tb, err := json.Marshal(target)
		if err != nil {
			return st, err
		}
		ex := tuneExample{
			SystemInstruction: tuneContent{Role: "system", Parts: []tunePart{{Text: engine.System}}},
			Contents: []tuneContent{
				{Role: "user", Parts: []tunePart{{Text: engine.RenderUser(c.Facts, precedents)}}},
				{Role: "model", Parts: []tunePart{{Text: string(tb)}}},
			},
		}
		if err := enc.Encode(ex); err != nil {
			return st, err
		}
		st.written++
	}
	return st, w.Flush()
}

// tuneTargetFor is the engine's answer with CBP's held origin and CBP's own
// reasoning. When the engine's origin was right, its essential character,
// decisive step, flip and confidence are kept. When it was wrong, its decisive
// step and flip argued for the wrong country, so the decisive step becomes
// CBP's holding, flip is null and confidence is the engine's own confidence
// for CBP's origin when that is what the row recorded, otherwise
// absentCorrectedConfidence (D56).
func tuneTargetFor(row evalkit.Row, r domain.Ruling, reasoning string) tuneTarget {
	t := tuneTarget{Origin: r.Origin, Reasoning: reasoning}
	if len(row.KeyFactors) > 0 {
		t.EssentialCharacter = row.KeyFactors[0]
	}
	if domain.NormalizeCountry(row.Pred) == r.Origin {
		t.DecisiveStep, t.Confidence = row.DecisiveStep, row.Confidence
		if row.Flip != "" {
			flip := row.Flip
			t.Flip = &flip
		}
		return t
	}
	t.DecisiveStep = corpus.Truncate(r.Holding, 400)
	t.Confidence = correctedTargetConfidence(row, r.Origin)
	return t
}

// correctedTargetConfidence is the confidence to teach when the target origin
// is CBP's and the engine named something else. The row's confidence belongs
// to row.Pred. It is the engine's confidence for CBP's origin only when the
// engine named that origin; otherwise it is not available and the target is
// absentCorrectedConfidence.
func correctedTargetConfidence(row evalkit.Row, gold string) float64 {
	if domain.NormalizeCountry(row.Pred) == domain.NormalizeCountry(gold) && row.Confidence > 0 && row.Confidence <= 1 {
		return row.Confidence
	}
	return absentCorrectedConfidence
}
