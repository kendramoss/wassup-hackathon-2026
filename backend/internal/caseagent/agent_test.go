package caseagent

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/engine"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/llm"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/verifier"
)

type fakeSearch struct{}

func (fakeSearch) Search(context.Context, string, time.Time, map[string]bool) ([]verifier.Hit, error) {
	return nil, nil
}
func (fakeSearch) Read(context.Context, string, time.Time, map[string]bool) (string, error) {
	return "", nil
}

type fakeEngine struct {
	mu   sync.Mutex
	seen []domain.Case
}

func (f *fakeEngine) Predict(_ context.Context, c domain.Case) (engine.Result, error) {
	f.mu.Lock()
	f.seen = append(f.seen, c)
	f.mu.Unlock()
	return engine.Result{Prediction: domain.Prediction{Origin: "Thailand", Confidence: 0.96, DecisiveStep: "motor made in Thailand"}}, nil
}

func TestAskReadsCaseRunsWhatIfAndChecksCitations(t *testing.T) {
	step := 0
	var mu sync.Mutex
	f := &llm.Fake{Respond: func(req llm.Request) (llm.Response, error) {
		mu.Lock()
		defer mu.Unlock()
		step++
		switch step {
		case 1:
			return llm.Response{ToolCalls: []llm.ToolCall{{ID: "1", Name: "get_case", Args: []byte(`{}`)}}}, nil
		case 2:
			return llm.Response{ToolCalls: []llm.ToolCall{{ID: "2", Name: "what_if", Args: []byte(`{"change":"motor in Thailand","facts":"Heat gun; motor made in Thailand."}`)}}}, nil
		default:
			return llm.Response{Text: "Moving the motor to Thailand flips it to Thailand (0.96), in line with N350001. N999999 agrees. Confirm with a licensed customs broker."}, nil
		}
	}}
	e := &fakeEngine{}
	date := time.Date(2026, 1, 14, 0, 0, 0, 0, time.UTC)
	ans, err := New(f, "m", fakeSearch{}, e).Ask(context.Background(), Input{
		Question: "What if the motor were made in Thailand?", CaseSummary: "Prediction: China 0.75. Earlier rulings used: N350001.",
		Facts: "Heat gun; motor made in China.", Date: date, Exclude: []string{"N328954"},
		History: []Turn{{Question: "Why China?", Answer: "The motor is Chinese."}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(ans.WhatIfs) != 1 || ans.WhatIfs[0].Origin != "Thailand" || len(e.seen) != 1 || !e.seen[0].Date.Equal(date) || e.seen[0].Exclude[0] != "N328954" {
		t.Errorf("what-if = %+v, engine saw %+v", ans.WhatIfs, e.seen)
	}
	if strings.Join(ans.Cited, ",") != "N350001" || strings.Join(ans.Dropped, ",") != "N999999" {
		t.Errorf("cited %v dropped %v", ans.Cited, ans.Dropped)
	}
	if len(ans.Steps) != 2 || ans.Steps[0].Tool != "get_case" || ans.Steps[1].Tool != "what_if" || ans.ModelCalls != 3 || ans.Trace.PromptVersion != PromptVersion {
		t.Errorf("steps %+v calls %d trace %+v", ans.Steps, ans.ModelCalls, ans.Trace)
	}
	if first := f.Calls()[0]; !strings.Contains(first.Messages[0].Content, "Q: Why China?") || !strings.Contains(first.System, System) {
		t.Errorf("history/system not sent: %+v", first.Messages)
	}
	if _, err := New(f, "m", fakeSearch{}, e).Ask(context.Background(), Input{Question: " "}); err == nil {
		t.Error("empty question accepted")
	}
}
