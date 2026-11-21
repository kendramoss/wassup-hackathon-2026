package router

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/llm"
)

func TestClassify(t *testing.T) {
	f := &llm.Fake{Respond: func(llm.Request) (llm.Response, error) {
		return llm.Response{Text: `{"route":"assembly","subfamily":"motorised","regime":"substantial_transformation","reason":"heat gun built from parts"}`,
			Usage: llm.Usage{InputTokens: 400, OutputTokens: 30}}, nil
	}}
	l, err := New(f, "gemini-3.8-flash", 1000).Classify(context.Background(), "A heat gun ...")
	if err != nil {
		t.Fatal(err)
	}
	if l.Route != "assembly" || l.Subfamily != "motorised" || l.PromptVersion != PromptVersion || l.ModelID != "gemini-3.8-flash" || l.InputTokens != 400 {
		t.Errorf("label = %+v", l)
	}
	if in, _ := l.Scope(); !in {
		t.Error("substantial transformation should be in scope")
	}
	req := f.Calls()[0]
	if !strings.Contains(req.System, "Do not decide origin") || req.JSONSchema == nil || req.Messages[0].Content != RenderUser("A heat gun ...") ||
		!strings.HasPrefix(req.Messages[0].Content, "CASE FACTS:\n") {
		t.Errorf("request = %+v", req)
	}
}

func TestScope(t *testing.T) {
	for regime, want := range map[string]bool{
		"substantial_transformation": true, "unclear": true,
		"textile_102_21": false, "usmca_part102": false, "pharma_api": false,
	} {
		in, reason := Label{Regime: regime}.Scope()
		if in != want || (!in && reason == "") {
			t.Errorf("%s: in=%v reason=%q", regime, in, reason)
		}
	}
}

func TestClassifyRejectsUnknownValues(t *testing.T) {
	f := llm.FakeText(`{"route":"assembly","subfamily":"spaceships","regime":"substantial_transformation","reason":"x"}`)
	if _, err := New(f, "m", 100).Classify(context.Background(), "x"); !errors.Is(err, llm.ErrBadResponse) {
		t.Fatalf("error = %v", err)
	}
}

func TestNewVersion(t *testing.T) {
	if _, err := NewVersion(llm.FakeText("{}"), "m", 100, "router-v9"); err == nil {
		t.Fatal("unknown version accepted")
	}
	for _, v := range Versions() {
		sys, ok := SystemFor(v)
		if !ok || !strings.Contains(sys, "Do not decide origin") {
			t.Errorf("%s: system prompt missing", v)
		}
	}
	if s, _ := SystemFor(PromptVersion); s != System {
		t.Error("System is not the production version's prompt")
	}
}

func TestV3USMCARule(t *testing.T) {
	answer := func(regime string) *llm.Fake {
		return llm.FakeText(`{"route":"assembly","subfamily":"motorised","regime":"` + regime + `","reason":"x"}`)
	}
	for _, tc := range []struct {
		regime, facts, want, rule string
	}{
		{"substantial_transformation", "You ask whether the pump qualifies for USMCA preference.", RegimeUSMCA, "usmca-named"},
		{"unclear", "A NAFTA ruling on a valve.", RegimeUSMCA, "usmca-named"},
		{"substantial_transformation", "A heat gun assembled in Vietnam.", "substantial_transformation", ""},
		{RegimeTextile, "Knit shirts; USMCA preference requested.", RegimeTextile, ""},
		{RegimePharma, "Tablets; NAFTA.", RegimePharma, ""},
		{"substantial_transformation", "Made by usmcab corp.", "substantial_transformation", ""},
	} {
		r, err := NewVersion(answer(tc.regime), "m", 100, PromptV3)
		if err != nil {
			t.Fatal(err)
		}
		l, err := r.Classify(context.Background(), tc.facts)
		if err != nil {
			t.Fatal(err)
		}
		if l.Regime != tc.want || l.Rule != tc.rule || l.PromptVersion != PromptV3 {
			t.Errorf("%q / %s: got %s rule %q version %s", tc.facts, tc.regime, l.Regime, l.Rule, l.PromptVersion)
		}
	}
	// router-v2 has no rule.
	r, _ := NewVersion(answer("substantial_transformation"), "m", 100, PromptV2)
	if l, _ := r.Classify(context.Background(), "USMCA preference"); l.Regime != "substantial_transformation" || l.PromptVersion != PromptV2 {
		t.Errorf("router-v2 changed the label: %+v", l)
	}
}
