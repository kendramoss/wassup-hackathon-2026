package main

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/llm"
	"os"
	"strings"
	"testing"
)

type moveChallengeCostModel struct{ calls int }

func (m *moveChallengeCostModel) Provider() string { return "fake" }
func (m *moveChallengeCostModel) Generate(ctx context.Context, _ llm.Request) (llm.Response, error) {
	m.calls++
	llm.CostTallyFrom(ctx).Record(0.25)
	return llm.Response{}, nil
}
func TestMoveChallengeBudgetStopsBeforeAnotherPaidCall(t *testing.T) {
	inner := &moveChallengeCostModel{}
	b := &moveChallengeBudget{inner: inner, limit: 1}
	var tally llm.CostTally
	ctx := llm.ContextWithCostTally(t.Context(), &tally)
	for i := 0; i < 3; i++ {
		if _, err := b.Generate(ctx, llm.Request{}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := b.Generate(ctx, llm.Request{}); !errors.Is(err, errMoveChallengeBudget) {
		t.Fatalf("budget not enforced: %v", err)
	}
	if inner.calls != 3 || tally.USD() != 0.75 || b.total() != 0.75 {
		t.Fatalf("accounting mismatch calls=%d tally=%v total=%v", inner.calls, tally.USD(), b.total())
	}
}

func TestMoveChallengeDatasetHasDisjointSourcesAndEnoughCases(t *testing.T) {
	d, _, err := loadMoveChallenge("../../../eval/challenges/verifier/cases.json")
	if err != nil {
		t.Fatal(err)
	}
	counts := map[string]int{}
	positive, negative := 0, 0
	for _, c := range d.Cases {
		counts[c.Split]++
		if c.Split == "test" && c.Expected.ChangesOrigin != nil {
			if *c.Expected.ChangesOrigin {
				positive++
			} else {
				negative++
			}
		}
	}
	if counts["test"] != 220 || counts["dev"] < 20 || positive < 100 || negative < 100 {
		t.Fatalf("counts=%v positive=%d negative=%d", counts, positive, negative)
	}
}

func TestMoveChallengeRejectsDuplicateInputsAndSourceLeakage(t *testing.T) {
	b, err := os.ReadFile("../../../eval/challenges/verifier/cases.json")
	if err != nil {
		t.Fatal(err)
	}
	var d moveChallengeDataset
	if err := json.Unmarshal(b, &d); err != nil {
		t.Fatal(err)
	}
	first := d.Cases[0]
	duplicate := first
	duplicate.ID += "-duplicate"
	d.Cases = append(d.Cases, duplicate)
	if err := validateMoveChallenge(d); err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("duplicate not rejected: %v", err)
	}
	d.Cases = d.Cases[:len(d.Cases)-1]
	for i := range d.Cases {
		if d.Cases[i].Split == "test" {
			d.Cases[i].SourceFamily = first.SourceFamily
			break
		}
	}
	if err := validateMoveChallenge(d); err == nil || !strings.Contains(err.Error(), "source family") {
		t.Fatalf("source-family leakage not rejected: %v", err)
	}
}

func TestMoveChallengeWrongTargetCannotCountAsCorrectShownMove(t *testing.T) {
	yes := true
	s, err := scoreMoveChallenge([]moveChallengeCase{{ID: "p", Expected: moveChallengeExpected{ChangesOrigin: &yes, OriginB: "Thailand"}}}, []moveChallengeRow{{ID: "p", ChangesOrigin: true, Supported: true, OriginB: "Vietnam"}})
	if err != nil || s.TruePositive != 0 || s.FalsePositive != 1 || s.FalseNegative != 1 || s.Matched != 0 {
		t.Fatalf("wrong target rewarded: %+v %v", s, err)
	}
}

func TestMoveChallengeScorerRetainsErrorsAndUncertainControls(t *testing.T) {
	yes, no := true, false
	cases := []moveChallengeCase{
		{ID: "positive", SourceFamily: "one", Expected: moveChallengeExpected{ChangesOrigin: &yes, OriginB: "Thailand"}},
		{ID: "negative", SourceFamily: "two", Expected: moveChallengeExpected{ChangesOrigin: &no, OriginB: "China"}},
		{ID: "uncertain", SourceFamily: "three", Expected: moveChallengeExpected{Supported: &no}},
	}
	rows := []moveChallengeRow{
		{ID: "positive", OriginB: "China", ChangesOrigin: false},
		{ID: "negative", Error: "model_error"},
		{ID: "uncertain", Supported: false},
	}
	s, err := scoreMoveChallenge(cases, rows)
	if err != nil || s.Cases != 3 || s.Matched != 1 || s.Errors != 1 || s.Mismatched != 1 || s.PositiveCases != 1 || s.NegativeCases != 1 || s.UncertainCases != 1 || s.TruePositive != 0 || s.FalseNegative != 1 || s.TrueNegative != 0 {
		t.Fatalf("summary=%+v err=%v", s, err)
	}
	rows[2].Supported = true
	s, err = scoreMoveChallenge(cases, rows)
	if err != nil || s.Matched != 0 || s.UncertainShown != 1 {
		t.Fatalf("unsafe approval not scored: %+v %v", s, err)
	}
	if _, err = scoreMoveChallenge(cases, rows[:2]); err == nil {
		t.Fatal("missing result disappeared from denominator")
	}
}
