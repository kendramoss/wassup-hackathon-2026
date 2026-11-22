package engine

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/blob"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/llm"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/tariffshift"
)

func defaultRules(t *testing.T) *tariffshift.Rules {
	t.Helper()
	r, err := tariffshift.Default()
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestCodesIn(t *testing.T) {
	c := domain.Case{Facts: "In 2024 you suggest the compressors are classified in subheading 8414.30, HTSUS. " +
		"The parts are of heading 8414 and subheadings 8501 and 8414.30.80. Model 4412 ships in 2025.",
		Product: &domain.Product{HTS: "8418.10"}}
	got := strings.Join(CodesIn(c), ",")
	if got != "8418.10,8414.30,8414,8501,8414.30.80" {
		t.Errorf("CodesIn = %s", got)
	}
}

func TestPart102RuleText(t *testing.T) {
	rt := Part102RuleText(defaultRules(t))
	got := rt(domain.Case{Facts: "you suggest the compressors are classified in subheading 8414.30, HTSUS; the stators are of 8414.90."})
	for _, want := range []string{
		"19 CFR 102.20(o), 8414.10-8414.80 (code named: 8414.30): A change to subheading 8414.10 through 8414.80 from any other subheading, including another subheading within that group.",
		"19 CFR 102.20(o), 8414.90 (code named: 8414.90): A change to subheading 8414.90 from any other heading.",
		"eCFR as of 2026-11-18",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("rule text lacks %q:\n%s", want, got)
		}
	}
	if s := rt(domain.Case{Facts: "A canopy of subheading 6306.22 made in Canada in 2024."}); s != "" {
		t.Errorf("chapter 63 is not encoded, want no text, got %q", s)
	}
	many := rt(domain.Case{Facts: "heading 8414, heading 8501, heading 8473, heading 8504"})
	if n := strings.Count(many, "\n- "); n != MaxRuleRows {
		t.Errorf("want %d rows quoted, got %d:\n%s", MaxRuleRows, n, many)
	}
}

func TestRuleTextFollowsTheFactsAndKeysTheCache(t *testing.T) {
	f := llm.FakeText(`{"origin_country":"Mexico","confidence":0.9}`)
	opts := Options{Model: "m", System: USMCASystem2, Version: USMCAVersion2, RuleText: Part102RuleText(defaultRules(t))}
	c := NewCache(New(f, nil, opts), blob.Dir{Dir: t.TempDir()})
	day := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	k := domain.Case{ID: "case_a", Date: day, Facts: "Compressors of subheading 8414.30 assembled in Mexico."}
	res, err := c.Predict(context.Background(), k)
	if err != nil {
		t.Fatal(err)
	}
	if res.Prediction.Trace.PromptVersion != USMCAVersion2 {
		t.Errorf("prompt version %q", res.Prediction.Trace.PromptVersion)
	}
	user := f.Calls()[0].Messages[0].Content
	if i, j := strings.Index(user, "FACTS"), strings.Index(user, "19 CFR 102.20 RULE TEXT"); i < 0 || j < i {
		t.Errorf("rule text must follow the facts:\n%s", user)
	}
	// Same facts, a product HTS that quotes another row: not the cached answer.
	k2 := k
	k2.Product = &domain.Product{HTS: "8414.90"}
	if r, _ := c.Predict(context.Background(), k2); r.Prediction.Trace.Cached {
		t.Error("a different quoted rule replayed the cached answer")
	}
	if r, _ := c.Predict(context.Background(), k); !r.Prediction.Trace.Cached {
		t.Error("the same case was not cached")
	}
	// Without RuleText the user message is unchanged.
	g := llm.FakeText(`{"origin_country":"Mexico","confidence":0.9}`)
	if _, err := New(g, nil, Options{Model: "m"}).Predict(context.Background(), k); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(g.Calls()[0].Messages[0].Content, "RULE TEXT") {
		t.Error("an engine without RuleText quoted rules")
	}
}
