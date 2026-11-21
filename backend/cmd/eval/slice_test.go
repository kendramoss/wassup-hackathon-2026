package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/config"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/corpus"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/engine"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/evalkit"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/evalslice"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/llm"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/retrieval"
)

const fixtures = "../../internal/evalslice/testdata/"

func TestSliceCmdComparesFiles(t *testing.T) {
	t.Setenv("LLM_PROVIDER", "") // -in needs no provider or project
	t.Setenv("GOOGLE_CLOUD_PROJECT", "")
	base, err := evalslice.LoadBaseline(fixtures + "baseline.json")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	slicePath := filepath.Join(dir, "slice.json")
	ids := make([]string, len(base.Cases))
	for i, c := range base.Cases {
		ids[i] = c.Ruling
	}
	if err := evalslice.WriteJSON(slicePath, evalslice.Slice{IDs: ids}); err != nil {
		t.Fatal(err)
	}
	args := func(in string) []string {
		return []string{"-slice", slicePath, "-baseline", fixtures + "baseline.json", "-in", fixtures + in}
	}
	tests := []struct {
		in, maxDrop string
		fail        bool
	}{
		{"same.jsonl", "", false},
		{"drop2.jsonl", "", false},
		{"drop3.jsonl", "", true},
		{"drop3.jsonl", "3", false},
		{"overconfident.jsonl", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.in+tt.maxDrop, func(t *testing.T) {
			t.Setenv("EVAL_SLICE_MAX_DROP", tt.maxDrop)
			err := sliceCmd(args(tt.in))
			if got := errors.Is(err, errSliceFailed); got != tt.fail || (err != nil && !got) {
				t.Errorf("err = %v, want failure %v", err, tt.fail)
			}
		})
	}
	// A slice the baseline does not cover is refused before comparing.
	other := filepath.Join(dir, "other.json")
	_ = evalslice.WriteJSON(other, evalslice.Slice{IDs: []string{"N100001"}})
	if err := sliceCmd([]string{"-slice", other, "-baseline", fixtures + "baseline.json", "-in", fixtures + "same.jsonl"}); err == nil || !strings.Contains(err.Error(), "same rulings") {
		t.Errorf("mismatched slice: %v", err)
	}
	// The live run spends model calls: off unless EVAL_SLICE=on.
	t.Setenv("LLM_PROVIDER", "fake")
	if err := sliceCmd([]string{"-slice", slicePath, "-baseline", fixtures + "baseline.json"}); err == nil || !strings.Contains(err.Error(), "EVAL_SLICE=on") {
		t.Errorf("live run without the flag: %v", err)
	}
}

// The live path: every slice case goes through the production engine setup
// and comes back in slice order, ready for the comparison.
func TestSliceRunCasesWithFakeModel(t *testing.T) {
	str := func(s string) *string { return &s }
	labels := []corpus.Label{
		{Ruling: "N100001", Date: "2024-01-01", Regime: "substantial_transformation", PrimaryOrigin: str("China"), Evidence: []string{"China."}},
		{Ruling: "N100002", Date: "2025-08-01", Regime: "substantial_transformation", PrimaryOrigin: str("China")},
		{Ruling: "N100003", Date: "2025-09-01", Regime: "substantial_transformation", PrimaryOrigin: str("Vietnam")},
	}
	dir := t.TempDir()
	for _, l := range labels {
		if err := os.WriteFile(filepath.Join(dir, l.Ruling+".txt"), []byte("motor made in China, assembled in Vietnam"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	store, err := corpus.Load(labels, dir)
	if err != nil {
		t.Fatal(err)
	}
	model := llm.FakeText(`{"origin_country":"China","essential_character":"motor","decisive_step":"motor made in China","flip":null,"confidence":0.97,"reasoning":"See N100001."}`)
	cfg := config.Config{LLMProvider: "fake", EngineModel: "gemini-4-argon", PrecedentK: 5, FastPathConfidence: 0.92}
	eng := engine.New(model, retrieval.NewBM25(store.Rulings(), engine.PrecedentText), engine.Options{Model: cfg.EngineModel, K: cfg.PrecedentK})
	var cases []domain.Ruling
	for _, id := range []string{"N100003", "N100002"} {
		r, _ := store.Get(id)
		cases = append(cases, r)
	}
	ctx := context.Background()
	rows := runCases(ctx, cases, 2, func(c domain.Ruling) evalkit.Row { return evaluate(ctx, eng, nil, cfg, store, c, "slice30") })
	if len(rows) != 2 || rows[0].Ruling != "N100003" || rows[1].Ruling != "N100002" {
		t.Fatalf("rows = %+v", rows)
	}
	if rows[0].Correct || !rows[1].Correct || rows[1].Split != "slice30" || rows[1].PromptVersion != engine.PromptVersion || rows[1].Error != "" {
		t.Errorf("rows = %+v", rows)
	}
	if len(model.Calls()) != 2 {
		t.Errorf("model calls = %d", len(model.Calls()))
	}
	path := filepath.Join(t.TempDir(), "run.jsonl")
	if err := writeRows(path, rows); err != nil {
		t.Fatal(err)
	}
	back, err := readRows(path, nil)
	if err != nil || len(back) != 2 {
		t.Fatalf("read back %d rows: %v", len(back), err)
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if r := runCases(canceled, cases, 1, func(domain.Ruling) evalkit.Row { t.Fatal("ran after cancel"); return evalkit.Row{} }); r[0].Error == "" {
		t.Errorf("cancelled run: %+v", r)
	}
}

// A Vertex 504 after the client's retries is an outage, not a wrong answer
// (D261): the slice re-runs such cases once after a cool-down. A case that
// answers then is scored normally; one that fails again is reported as
// unavailable after retry and still counts as wrong. A bad answer is never
// re-run.
func TestSliceRetriesProviderErrorsOnce(t *testing.T) {
	str := func(s string) *string { return &s }
	labels := []corpus.Label{
		{Ruling: "N100001", Date: "2024-01-01", Regime: "substantial_transformation", PrimaryOrigin: str("China"), Evidence: []string{"China."}},
		{Ruling: "N100002", Date: "2025-09-01", Regime: "substantial_transformation", PrimaryOrigin: str("China")},
		{Ruling: "N100003", Date: "2025-09-01", Regime: "substantial_transformation", PrimaryOrigin: str("China")},
		{Ruling: "N100004", Date: "2025-09-01", Regime: "substantial_transformation", PrimaryOrigin: str("China")},
	}
	facts := map[string]string{
		"N100001": "motor made in China, assembled in Vietnam",
		"N100002": "alpha unit: motor made in China, assembled in Vietnam",
		"N100003": "beta unit: motor made in China, assembled in Vietnam",
		"N100004": "gamma unit: motor made in China, assembled in Vietnam",
	}
	dir := t.TempDir()
	for id, f := range facts {
		if err := os.WriteFile(filepath.Join(dir, id+".txt"), []byte(f), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	store, err := corpus.Load(labels, dir)
	if err != nil {
		t.Fatal(err)
	}
	const answer = `{"origin_country":"China","essential_character":"motor","decisive_step":"motor made in China","flip":null,"confidence":0.97,"reasoning":"See N100001."}`
	outage := fmt.Errorf("llm: %w after 4 attempts: %w", llm.ErrUnavailable, &llm.APIError{Provider: "vertex", StatusCode: 504, Body: "Deadline expired before operation could complete."})
	var mu sync.Mutex
	calls := map[string]int{}
	model := &llm.Fake{Respond: func(req llm.Request) (llm.Response, error) {
		text := req.System
		for _, m := range req.Messages {
			text += m.Content
		}
		mu.Lock()
		defer mu.Unlock()
		switch {
		case strings.Contains(text, "alpha unit"):
			calls["alpha"]++
			return llm.Response{Text: "not an answer"}, nil
		case strings.Contains(text, "beta unit"):
			calls["beta"]++
			if calls["beta"] == 1 {
				return llm.Response{}, outage
			}
			return llm.Response{Text: answer}, nil
		case strings.Contains(text, "gamma unit"):
			calls["gamma"]++
			return llm.Response{}, outage
		}
		t.Errorf("unexpected request")
		return llm.Response{}, errors.New("unexpected")
	}}
	cfg := config.Config{LLMProvider: "fake", EngineModel: "gemini-4-argon", PrecedentK: 5, FastPathConfidence: 0.92}
	eng := engine.New(model, retrieval.NewBM25(store.Rulings(), engine.PrecedentText), engine.Options{Model: cfg.EngineModel, K: cfg.PrecedentK})
	var cases []domain.Ruling
	for _, id := range []string{"N100002", "N100003", "N100004"} {
		r, _ := store.Get(id)
		cases = append(cases, r)
	}
	ctx := context.Background()
	eval := func(ctx context.Context, c domain.Ruling) evalkit.Row {
		return evaluate(ctx, eng, nil, cfg, store, c, "slice30")
	}
	var waited []time.Duration
	rp := retryPolicy{on: true, cool: 45 * time.Second, budget: time.Minute, wait: func(_ context.Context, d time.Duration) error { waited = append(waited, d); return nil }}

	first := runCases(ctx, cases, 3, func(c domain.Ruling) evalkit.Row { return eval(ctx, c) })
	if first[0].ErrorClass != "bad_response" || first[1].ErrorClass != "unavailable" || first[2].ErrorClass != "unavailable" {
		t.Fatalf("first pass classes: %q %q %q", first[0].ErrorClass, first[1].ErrorClass, first[2].ErrorClass)
	}
	rows := retryTransient(ctx, cases, first, 3, rp, eval)
	if len(waited) != 1 || waited[0] != 45*time.Second {
		t.Errorf("cool-downs = %v, want one of 45s", waited)
	}
	if calls["alpha"] != 1 || calls["beta"] != 2 || calls["gamma"] != 2 {
		t.Errorf("calls = %v: a bad answer must not be re-run; each outage exactly once", calls)
	}
	if r := rows[0]; r.Retries != 0 || r.Error == "" {
		t.Errorf("bad answer row = %+v", r)
	}
	if r := rows[1]; r.Retries != 1 || r.Error != "" || !r.Correct || !strings.Contains(r.RetriedError, "HTTP 504") {
		t.Errorf("recovered row = %+v", r)
	}
	if r := rows[2]; r.Retries != 1 || r.ErrorClass != "unavailable" || r.Correct {
		t.Errorf("still unavailable row = %+v", r)
	}
	if first[1].Retries != 0 || first[1].Error == "" {
		t.Errorf("the first pass rows were changed in place: %+v", first[1])
	}

	// The report keeps outages apart from wrong answers; they still fail the gate.
	base := evalslice.Baseline{N: 3, Correct: 3, Cases: []evalslice.Case{{Ruling: "N100002", Correct: true}, {Ruling: "N100003", Correct: true}, {Ruling: "N100004", Correct: true}}}
	rep := evalslice.Compare(base, rows, evalslice.Tolerance{MaxDrop: 1, BandMin: 0.95, MaxErrors: 3})
	if rep.Unavailable != 1 || rep.UnavailableNoRetry != 0 || rep.Retried != 2 || rep.Recovered != 1 || rep.Errors != 2 || rep.Correct != 1 || rep.Passed() {
		t.Errorf("report = %+v", rep)
	}
	md := rep.Markdown("slice", base)
	for _, want := range []string{"1 cases unavailable after retry", "2 cases hit a model provider error", "1 answered", "1 of 3 cases failed for another reason"} {
		if !strings.Contains(md, want) {
			t.Errorf("report lacks %q:\n%s", want, md)
		}
	}

	// A re-run still going when the budget ends keeps an outage error.
	slow := retryPolicy{on: true, budget: time.Millisecond, wait: func(context.Context, time.Duration) error { return nil }}
	cut := retryTransient(ctx, cases, first, 3, slow, func(ctx context.Context, c domain.Ruling) evalkit.Row {
		<-ctx.Done()
		return evalkit.Row{Ruling: c.ID, Error: ctx.Err().Error(), ErrorClass: errorClass(ctx.Err())}
	})
	if r := cut[1]; r.Retries != 1 || !evalslice.Transient(r) {
		t.Errorf("cut-off re-run = %+v", r)
	}

	// -retry=false keeps the first pass as it was.
	if got := retryTransient(ctx, cases, first, 3, retryPolicy{}, eval); got[1].Retries != 0 || got[1].Error == "" {
		t.Errorf("retry off: %+v", got[1])
	}
	// A cancelled cool-down re-runs nothing.
	stop := retryPolicy{on: true, cool: time.Minute, wait: func(context.Context, time.Duration) error { return context.Canceled }}
	if got := retryTransient(ctx, cases, first, 3, stop, func(context.Context, domain.Ruling) evalkit.Row {
		t.Fatal("re-ran after a cancelled wait")
		return evalkit.Row{}
	}); got[1].Retries != 0 {
		t.Errorf("cancelled: %+v", got[1])
	}
}
