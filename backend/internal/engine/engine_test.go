package engine

import (
	"context"
	"errors"
	"flag"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/llm"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/retrieval"
)

var update = flag.Bool("update", false, "rewrite golden files")

func day(s string) time.Time { t, _ := time.Parse(time.DateOnly, s); return t }

func precedents() []domain.Ruling {
	return []domain.Ruling{
		{ID: "N350001", Date: day("2025-03-01"), Facts: "A hair dryer is assembled in Thailand; the motor is made in China.\n",
			Holding: "The country of origin of the hair dryer is China."},
		{ID: "N350002", Date: day("2025-06-01"), Facts: "A heat gun motor, PCBA and fan are made in Thailand.\n",
			Holding: "The country of origin of the heat gun is Thailand."},
		{ID: "N370000", Date: day("2026-03-01"), Facts: "A later heat gun ruling that must never be shown.\n",
			Holding: "Thailand."},
	}
}

func TestRenderUserGolden(t *testing.T) {
	got := RenderUser("A heat gun is assembled in Thailand from a Chinese motor.\n", precedents()[:2])
	golden := filepath.Join("testdata", "render_user.golden")
	if *update {
		if err := os.WriteFile(golden, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatalf("%v (run go test ./internal/engine -update)", err)
	}
	if got != string(want) {
		t.Errorf("RenderUser changed; run with -update if intended.\ngot:\n%s", got)
	}
}

func TestRenderUserFactsOnly(t *testing.T) {
	if got := RenderUser("x", nil); got != "FACTS (ruling request, analysis removed):\nx" {
		t.Errorf("facts-only render = %q", got)
	}
}

func TestRenderUserTruncates(t *testing.T) {
	long := strings.Repeat("é", FactsRunes+50)
	got := RenderUser(long, []domain.Ruling{{ID: "N1", Date: day("2025-01-01"), Facts: strings.Repeat("a", 4000), Holding: strings.Repeat("h", 900)}})
	if strings.Count(got, "é") != FactsRunes || strings.Count(got, "a") < PrecedentFactsRunes || strings.Count(got, "h") != 600 {
		t.Errorf("truncation wrong: é=%d h=%d", strings.Count(got, "é"), strings.Count(got, "h"))
	}
}

func newEngine(f *llm.Fake) *Engine {
	idx := retrieval.NewBM25(precedents(), PrecedentText)
	return New(f, idx, Options{Model: "gemini-4-argon", K: 5, MaxOutputTokens: 8000})
}

func TestPredict(t *testing.T) {
	f := &llm.Fake{Respond: func(req llm.Request) (llm.Response, error) {
		return llm.Response{Text: `{"origin_country":"PRC","essential_character":"motor-fan assembly",
			"decisive_step":"motor made in China","flip":"make the motor in Thailand","confidence":0.97,
			"reasoning":"Like N350001 (and not N999999), the motor drives character."}`,
			Usage: llm.Usage{InputTokens: 900, OutputTokens: 80}}, nil
	}}
	c := domain.Case{ID: "N357771", Date: day("2026-01-14"), Facts: "A heat gun is assembled in Thailand from a Chinese motor and fan."}
	res, err := newEngine(f).Predict(context.Background(), c)
	if err != nil {
		t.Fatal(err)
	}
	p := res.Prediction
	if p.Origin != "China" || p.Confidence != 0.97 || res.Flip != "make the motor in Thailand" {
		t.Errorf("prediction = %+v", p)
	}
	if !reflect.DeepEqual(p.KeyFactors, []string{"motor-fan assembly"}) {
		t.Errorf("key factors = %v", p.KeyFactors)
	}
	if !reflect.DeepEqual(p.CitedRulings, []string{"N350001"}) {
		t.Errorf("cited = %v (hallucinated N999999 must be dropped)", p.CitedRulings)
	}
	for _, id := range p.Precedents {
		if id == "N370000" {
			t.Fatal("a ruling dated after the case was shown to the model")
		}
	}
	if len(p.Precedents) != 2 {
		t.Errorf("precedents = %v", p.Precedents)
	}
	if p.Trace.PromptVersion != PromptVersion || p.Trace.Provider != "fake" || p.Trace.ModelID != "gemini-4-argon" || p.Trace.InputTokens != 900 {
		t.Errorf("trace = %+v", p.Trace)
	}

	req := f.Calls()[0]
	if req.System != System || req.JSONSchema == nil || req.Model != "gemini-4-argon" || req.MaxOutputTokens != 8000 {
		t.Errorf("request = %+v", req)
	}
	if strings.Contains(req.Messages[0].Content, "N370000") {
		t.Error("later ruling leaked into the prompt")
	}
}

func TestPredictErrors(t *testing.T) {
	c := domain.Case{ID: "X", Date: day("2026-01-14"), Facts: "facts"}
	tests := []struct {
		name string
		text string
		err  error
		want error
	}{
		{"model error is wrapped", "", &llm.APIError{StatusCode: 503}, nil},
		{"no JSON", "sorry", nil, llm.ErrBadResponse},
		{"empty origin", `{"origin_country":"","confidence":0.5}`, nil, llm.ErrBadResponse},
		{"confidence out of range", `{"origin_country":"China","confidence":7}`, nil, llm.ErrBadResponse},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &llm.Fake{Respond: func(llm.Request) (llm.Response, error) { return llm.Response{Text: tt.text}, tt.err }}
			_, err := newEngine(f).Predict(context.Background(), c)
			if err == nil {
				t.Fatal("want error")
			}
			var apiErr *llm.APIError
			if tt.err != nil && !errors.As(err, &apiErr) {
				t.Errorf("error %v does not wrap the APIError", err)
			}
			if tt.want != nil && !errors.Is(err, tt.want) {
				t.Errorf("error %v, want %v", err, tt.want)
			}
		})
	}
	if _, err := newEngine(llm.FakeText("{}")).Predict(context.Background(), domain.Case{Date: day("2026-01-01")}); !errors.Is(err, ErrNoFacts) {
		t.Errorf("no facts: %v", err)
	}
	if _, err := newEngine(llm.FakeText("{}")).Predict(context.Background(), domain.Case{Facts: "x"}); err == nil {
		t.Error("no date: want error")
	}
}

func TestPredictWithoutRetrieverIsFactsOnly(t *testing.T) {
	f := llm.FakeText(`{"origin_country":"Vietnam","confidence":0.8}`)
	e := New(f, nil, Options{Model: "m", K: 5})
	res, err := e.Predict(context.Background(), domain.Case{ID: "X", Date: day("2026-01-01"), Facts: "facts"})
	if err != nil || len(res.Prediction.Precedents) != 0 || strings.Contains(f.Calls()[0].Messages[0].Content, "EARLIER CBP RULINGS") {
		t.Fatalf("facts-only failed: %+v %v", res, err)
	}
}

func TestPredictListsAreNeverNil(t *testing.T) {
	f := llm.FakeText(`{"origin_country":"Vietnam","essential_character":"","confidence":0.8,"reasoning":"no citations"}`)
	res, err := New(f, nil, Options{Model: "m"}).Predict(context.Background(), domain.Case{ID: "X", Date: day("2026-01-01"), Facts: "f"})
	if err != nil {
		t.Fatal(err)
	}
	p := res.Prediction
	if p.KeyFactors == nil || p.CitedRulings == nil || p.Precedents == nil {
		t.Errorf("nil lists: %+v", p)
	}
}

func votingEngine(f *llm.Fake) *Engine {
	ps := []domain.Ruling{
		{ID: "N300001", Date: day("2025-01-01"), Facts: "heat gun motor"},
		{ID: "N300002", Date: day("2025-01-02"), Facts: "heat gun pcba"},
		{ID: "N300003", Date: day("2025-01-03"), Facts: "heat gun fan"},
	}
	return New(f, retrieval.NewBM25(ps, PrecedentText), Options{Model: "m", K: 3, Votes: 5, VoteBelow: 0.95})
}

// firstShown returns the ruling ID of the first precedent block in a prompt.
func firstShown(req llm.Request) string {
	s := req.Messages[0].Content
	i := strings.Index(s, "[N")
	return s[i+1 : i+8]
}

func TestVotingPicksMajorityOverRotations(t *testing.T) {
	var mu sync.Mutex
	f := &llm.Fake{Respond: func(req llm.Request) (llm.Response, error) {
		mu.Lock()
		defer mu.Unlock()
		// The model follows whichever precedent comes first: Thailand unless
		// N300003 leads.
		if firstShown(req) == "N300003" {
			return llm.Response{Text: `{"origin_country":"China","confidence":0.9}`, Usage: llm.Usage{InputTokens: 10}}, nil
		}
		return llm.Response{Text: `{"origin_country":"Thailand","confidence":0.8}`, Usage: llm.Usage{InputTokens: 10}}, nil
	}}
	res, err := votingEngine(f).Predict(context.Background(), domain.Case{ID: "X", Date: day("2026-01-01"), Facts: "heat gun"})
	if err != nil {
		t.Fatal(err)
	}
	if len(f.Calls()) != 5 || res.Votes != 5 || res.Usage.InputTokens != 50 {
		t.Errorf("calls=%d votes=%d tokens=%d", len(f.Calls()), res.Votes, res.Usage.InputTokens)
	}
	if res.Prediction.VoteAgreement == "" || res.Prediction.Confidence > 0.9 {
		t.Errorf("prediction = %+v", res.Prediction)
	}
	orders := map[string]bool{}
	for _, c := range f.Calls() {
		orders[firstShown(c)] = true
	}
	if len(orders) != 3 {
		t.Errorf("rotations should lead with each precedent: %v", orders)
	}
}

func TestNoVotingWhenConfident(t *testing.T) {
	f := llm.FakeText(`{"origin_country":"Thailand","confidence":0.97}`)
	res, err := votingEngine(f).Predict(context.Background(), domain.Case{ID: "X", Date: day("2026-01-01"), Facts: "heat gun"})
	if err != nil || len(f.Calls()) != 1 || res.Votes != 1 || res.Prediction.VoteAgreement != "" {
		t.Fatalf("calls=%d res=%+v err=%v", len(f.Calls()), res, err)
	}
}

func TestVotingConfidenceIsNotInflated(t *testing.T) {
	f := llm.FakeText(`{"origin_country":"Thailand","confidence":0.85}`)
	res, err := votingEngine(f).Predict(context.Background(), domain.Case{ID: "X", Date: day("2026-01-01"), Facts: "heat gun"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Prediction.Confidence != 0.85 || res.Prediction.VoteAgreement != "5/5" {
		t.Errorf("unanimous low-confidence vote: %+v", res.Prediction)
	}
}
