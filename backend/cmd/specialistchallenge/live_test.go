package main

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/engine"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/llm"
)

func TestSelectLiveAppliesThePreregisteredRule(t *testing.T) {
	base := SplitSummary{Counts: Counts{Cases: 48, Matched: 46, Mismatched: 2},
		Decided: DecidedStats{Cases: 42, Agreed: 42, Confident: 40, ConfidentCorrect: 40}, Uncertain: UncertainStats{Cases: 6, AbstainedCorrectly: 4, Overconfident: 2}}
	with := func(f func(s *SplitSummary)) SplitSummary { s := base; f(&s); return s }
	for name, tc := range map[string]struct {
		cand SplitSummary
		want string
	}{
		"identical keeps the deployed prompt":       {base, LiveBase},
		"more matched, no regression":               {with(func(s *SplitSummary) { s.Matched, s.Mismatched = 47, 1 }), LiveCandidate},
		"fewer overconfident, same matched":         {with(func(s *SplitSummary) { s.Uncertain.Overconfident, s.Uncertain.AbstainedCorrectly = 1, 5 }), LiveCandidate},
		"matched falls":                             {with(func(s *SplitSummary) { s.Matched, s.Mismatched = 45, 3 }), LiveBase},
		"an error appears":                          {with(func(s *SplitSummary) { s.Matched, s.Errors, s.Mismatched = 47, 1, 0 }), LiveBase},
		"confident-wrong rises":                     {with(func(s *SplitSummary) { s.Matched = 47; s.Decided.ConfidentWrong = 1 }), LiveBase},
		"confident-correct falls by two":            {with(func(s *SplitSummary) { s.Matched = 47; s.Decided.ConfidentCorrect = 38 }), LiveBase},
		"confident-correct falls by one is allowed": {with(func(s *SplitSummary) { s.Matched = 47; s.Decided.ConfidentCorrect = 39 }), LiveCandidate},
	} {
		if got := SelectLive(base, tc.cand); got != tc.want {
			t.Errorf("%s: %s, want %s", name, got, tc.want)
		}
	}
}

func liveCandidate(t *testing.T, m llm.Model, variant string) *Candidate {
	t.Helper()
	rs := rulings()
	rs[0].Reasoning = "REASONING-OF-N000001"
	c, err := NewCandidate(m, "gemini-4-argon", Pools{Textile: rulings(), TextileV2: rs, Pharma: rulings(), Part102: rulings()})
	if err != nil {
		t.Fatal(err)
	}
	if err := c.SetVariant(variant); err != nil {
		t.Fatal(err)
	}
	return c
}

func TestLiveVariantsMatchTheDeployedTextileEngine(t *testing.T) {
	tx := mk("textile_specialist", "c0000000000h1", "dev", "Cotton shirts are sewn in India from fabric woven in China.", dec("India"))
	conf := `{"origin_country":"India","essential_character":"x","decisive_step":"y","flip":null,"confidence":0.93,"reasoning":"r"}`
	req := func(m *llm.Fake) string {
		s := m.Calls()[0].System
		for _, msg := range m.Calls()[0].Messages {
			s += msg.Content
		}
		return s
	}
	m := llm.FakeText(conf)
	row := RunCase(context.Background(), liveCandidate(t, m, LiveBase), tx)
	if !strings.Contains(req(m), engine.TextileSystem2) || !strings.Contains(req(m), "REASONING-OF-N000001") || row.PromptVersion != engine.TextileVersion2+"+"+engine.ReasoningVersion {
		t.Errorf("live-v2 is not textile-v2 with precedent reasoning: %q", row.PromptVersion)
	}
	if strings.Contains(req(m), engine.MissingFactRule) || row.Route != "needs_review" {
		t.Errorf("0.93 must not reach the deployed 0.95 textile bar (route %q) and the base arm has no added rule", row.Route)
	}
	m2 := llm.FakeText(conf)
	row2 := RunCase(context.Background(), liveCandidate(t, m2, LiveCandidate), tx)
	if !strings.HasPrefix(m2.Calls()[0].System, engine.TextileSystem2) || !strings.Contains(m2.Calls()[0].System, engine.MissingFactRule) ||
		!strings.Contains(row2.PromptVersion, engine.TextileVersion2) || !strings.Contains(row2.PromptVersion, "missing-v1") || !strings.Contains(row2.PromptVersion, "one-country") {
		t.Errorf("live candidate is not the same v2 plus the option: %q", row2.PromptVersion)
	}
	m3 := llm.FakeText(strings.Replace(conf, "0.93", "0.96", 1))
	if r := RunCase(context.Background(), liveCandidate(t, m3, LiveBase), tx); r.Route != "auto" {
		t.Errorf("0.96 is above the deployed bar, route %q", r.Route)
	}
}

func TestLiveComparatorStaysApartFromTheOriginalStudy(t *testing.T) {
	if ComparatorFeature == "textile_specialist" || comparatorTestFile == "textile-test.jsonl" {
		t.Fatal("the comparator must not reuse the original feature id or the original reserved result file")
	}
	_ = domain.Case{}
	_ = time.Now
}
