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
