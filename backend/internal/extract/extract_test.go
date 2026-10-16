package extract

import (
	"context"
	"strings"
	"testing"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/llm"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/planner"
)

const facts = "Request for a heat gun.\n\nEach heat gun is assembled in Thailand using 24 components. The main housing and heating element are produced in Thailand. The PCBA and motor are sourced from China.\n\nThe fan is press fitted to the motor."

const reply = `{"product":"Kobalt heat gun","components":[
 {"name":"main housing","country":"Thailand","group":"housing_moulding","paragraph":2},
 {"name":"heating element","country":"thailand","group":"heating_element","paragraph":2},
 {"name":"PCBA","country":"China","group":"pcba","paragraph":2},
 {"name":"motor","country":"China","group":"teleport","paragraph":2},
 {"name":"motor","country":"China","group":"motor_fan_assembly","paragraph":2},
 {"name":"knob","country":"","group":"","paragraph":2}],
 "operations":[{"name":"press fit the fan to the motor","country":"Thailand","group":"final_assembly","paragraph":3}],
 "plants":[],"stated_component_count":24,"notes":[]}`

func tmpl(t *testing.T) *planner.Template {
	t.Helper()
	tm, err := planner.LoadTemplates()
	if err != nil {
		t.Fatal(err)
	}
	g := tm["motorised_tools_small_appliances"]
	return &g
}

func TestExtractDraftsCheckedProduct(t *testing.T) {
	f := llm.FakeText(reply)
	d, err := New(f, "m").Extract(context.Background(), facts, "", tmpl(t))
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Components) != 4 || d.Paragraphs != 3 || d.Product.Name != "Kobalt heat gun" || d.Trace.PromptVersion != PromptVersion {
		t.Fatalf("draft = %+v", d)
	}
	if len(d.Plants) != 2 || d.Plants[0].Name != "Thailand plant" || !d.Plants[0].NameGuessed || d.Plants[1].Country != "China" {
		t.Errorf("plants = %+v", d.Plants)
	}
	motor := d.Components[3]
	if motor.Group != "" || !motor.GroupGuessed || motor.Plant != "china_plant" || motor.SourceParagraph != 2 {
		t.Errorf("unknown group not cleared: %+v", motor)
	}
	if d.Operations[0].Plant != "thailand_plant" || d.Operations[0].Group != "final_assembly" {
		t.Errorf("operation = %+v", d.Operations[0])
	}
	w := strings.Join(d.Warnings, " | ")
	for _, want := range []string{"The text mentions knob without saying where it is made", "The text says 24 components; it names 4.", "1 component(s) have no operation group"} {
		if !strings.Contains(w, want) {
			t.Errorf("warnings %q lack %q", w, want)
		}
	}
	if len(d.Questions) != 2 || !strings.HasPrefix(d.Questions[0].Ask, "What else could the Thailand plant make?") ||
		strings.Contains(strings.Join(d.Questions[0].Options, ","), "housing_moulding") || strings.Contains(strings.Join(d.Questions[0].Options, ","), "final_assembly") {
		t.Errorf("questions = %+v", d.Questions)
	}
	if err := d.Product.Validate(); err != nil {
		t.Errorf("draft product invalid: %v", err)
	}
	if !strings.Contains(f.Calls()[0].Messages[0].Content, "[2] Each heat gun") || !strings.Contains(f.Calls()[0].Messages[0].Content, "- pcba:") {
		t.Errorf("prompt lacks numbered paragraphs or groups")
	}
}

func TestExtractNeedsFacts(t *testing.T) {
	if _, err := New(llm.FakeText("{}"), "m").Extract(context.Background(), "  \n\n ", "", nil); err == nil {
		t.Error("empty facts accepted")
	}
}
