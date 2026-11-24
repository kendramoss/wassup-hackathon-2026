package familycheck

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/llm"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/planner"
)

func checker(t *testing.T, answer string) (*Checker, *llm.Request) {
	t.Helper()
	tmpls, err := planner.LoadTemplates()
	if err != nil {
		t.Fatal(err)
	}
	var seen llm.Request
	m := &llm.Fake{Respond: func(req llm.Request) (llm.Response, error) {
		seen = req
		return llm.Response{Text: answer}, nil
	}}
	return New(m, "gemini-3.8-flash", tmpls), &seen
}

func TestCheckMatchesATemplate(t *testing.T) {
	c, req := checker(t, `{"family":"motorised_tools_small_appliances","confidence":1.4,"reason":"Motor, fan and heating element."}`)
	r, err := c.Check(context.Background(), "hair dryer")
	if err != nil {
		t.Fatal(err)
	}
	if r.Family != "motorised_tools_small_appliances" || r.Name == "" || r.Confidence != 1 {
		t.Errorf("result = %+v", r)
	}
	enum := req.JSONSchema["properties"].(map[string]any)["family"].(map[string]any)["enum"].([]string)
	if len(enum) < 4 || enum[len(enum)-1] != None || !strings.Contains(req.System, "led_lights") {
		t.Errorf("schema enum = %v", enum)
	}
}

func TestCheckNone(t *testing.T) {
	c, _ := checker(t, `{"family":"none","confidence":0.8,"reason":"A jet engine shares no groups."}`)
	if r, err := c.Check(context.Background(), "turbofan engine"); err != nil || r.Family != None || r.Name != "" {
		t.Errorf("none = %+v, %v", r, err)
	}
}

func TestCheckRejectsAnInventedFamily(t *testing.T) {
	c, _ := checker(t, `{"family":"jet_engines","confidence":0.9,"reason":"x"}`)
	if _, err := c.Check(context.Background(), "turbofan"); !errors.Is(err, llm.ErrBadResponse) {
		t.Errorf("err = %v", err)
	}
}

// D850 (#785): the model sees the machines and machinery parts scopes, so a
// whole machine or working unit is not read as a "subassembly" part; a
// template with no scope gets no scope line.
func TestPromptShowsTemplateScope(t *testing.T) {
	c, req := checker(t, `{"family":"none","confidence":0.5,"reason":"x"}`)
	if _, err := c.Check(context.Background(), "Typed: CNC lathe"); err != nil {
		t.Fatal(err)
	}
	sys := req.System
	if PromptVersion != "family-check-v2" || !strings.Contains(sys, "When a family gives a scope, follow it") {
		t.Errorf("prompt %s has no scope instruction", PromptVersion)
	}
	for _, want := range []string{
		"- machinery_parts (Machinery parts and subassemblies): operation groups",
		"  Scope: Loose parts and castings imported for machines, not machines",
		"  Scope: Complete machines and functional working units",
	} {
		if !strings.Contains(sys, want) {
			t.Errorf("prompt lacks %q", want)
		}
	}
	if n := strings.Count(sys, "  Scope: "); n != 2 {
		t.Errorf("%d scope lines, want 2 (machinery parts and machines only)", n)
	}
	if i := strings.Index(sys, "- led_lights ("); i < 0 || strings.HasPrefix(sys[strings.Index(sys[i:], "\n")+i+1:], "  Scope:") {
		t.Error("a template with no scope got a scope line")
	}
}
