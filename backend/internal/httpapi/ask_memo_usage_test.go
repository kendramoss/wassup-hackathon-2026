package httpapi

import (
	"reflect"
	"strings"
	"testing"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/caseagent"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
)

// #367 (was #359): an Ask answered from memory keeps the cached answer and its
// provenance, but the repeat is not a new model call, so it records no new
// tokens or cost.
func askedEvents(t *testing.T, hs harness, id string) []map[string]any {
	t.Helper()
	_, got := hs.do(t, "GET", "/v1/cases/"+id, "")
	var out []map[string]any
	for _, e := range got["history"].([]any) {
		if m := e.(map[string]any); m["kind"] == "asked" {
			out = append(out, m)
		}
	}
	return out
}

func TestAskMemoHitRecordsNoNewUsage(t *testing.T) {
	hs := newHarness(t, "substantial_transformation")
	hs.agent.reply = &caseagent.Answer{Answer: "Because of the motor.", Cited: []string{"N350001"}, WhatIfs: []caseagent.WhatIf{}, Steps: []caseagent.Step{},
		Trace: domain.Trace{Provider: "vertex", ModelID: "gemini-4-argon", PromptVersion: caseagent.PromptVersion, InputTokens: 100000, OutputTokens: 5000}}
	_, c := hs.do(t, "POST", "/v1/cases/predict", heatGunBody(t))
	id := c["id"].(string)

	_, first := hs.do(t, "POST", "/v1/cases/"+id+"/ask", `{"question":"Why China?"}`)
	_, repeat := hs.do(t, "POST", "/v1/cases/"+id+"/ask", `{"question":"Why China?","reset":true}`)
	if hs.agent.calls != 1 {
		t.Fatalf("agent calls = %d, want 1 (the repeat comes from memory)", hs.agent.calls)
	}
	if !reflect.DeepEqual(first["reply"], repeat["reply"]) {
		t.Errorf("the repeat changed the answer or its provenance:\n first:  %v\n repeat: %v", first["reply"], repeat["reply"])
	}

	ev := askedEvents(t, hs, id)
	if len(ev) != 2 {
		t.Fatalf("asked events = %d, want 2", len(ev))
	}
	// The uncached reply keeps its actual usage: 100000 in, 5000 out at the promo price.
	if ev[0]["input_tokens"] != 100000.0 || ev[0]["output_tokens"] != 5000.0 || ev[0]["cost_usd"] != 0.25 {
		t.Errorf("first ask usage = in %v out %v cost %v", ev[0]["input_tokens"], ev[0]["output_tokens"], ev[0]["cost_usd"])
	}
	// The repeat charges nothing, keeps the provenance, and says it came from memory.
	for _, k := range []string{"input_tokens", "output_tokens", "cost_usd"} {
		if v, ok := ev[1][k]; ok && v != 0.0 {
			t.Errorf("repeat recorded %s = %v", k, v)
		}
	}
	for _, k := range []string{"llm_provider", "model_id", "prompt_version"} {
		if ev[1][k] != ev[0][k] || ev[1][k] == nil {
			t.Errorf("repeat lost provenance %s: %v vs %v", k, ev[1][k], ev[0][k])
		}
	}
	if s := ev[1]["summary"].(string); !strings.Contains(s, "from memory") {
		t.Errorf("repeat summary lacks the memory marker: %q", s)
	}
	if s := ev[0]["summary"].(string); strings.Contains(s, "from memory") {
		t.Errorf("first ask marked as memory: %q", s)
	}
}
