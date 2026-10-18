package httpapi

import "testing"

// The same question on the same case and conversation is answered from
// memory; a new conversation turn or another question asks the agent.
func TestAskRemembersAnswers(t *testing.T) {
	hs := newHarness(t, "substantial_transformation")
	_, c := hs.do(t, "POST", "/v1/cases/predict", heatGunBody(t))
	id := c["id"].(string)
	ask := func(body string) {
		t.Helper()
		if code, out := hs.do(t, "POST", "/v1/cases/"+id+"/ask", body); code != 200 {
			t.Fatalf("ask %s = %d %v", body, code, out)
		}
	}
	ask(`{"question":"Why China?"}`)
	ask(`{"question":"Why  china?","reset":true}`) // same question, same (empty) history
	if hs.agent.calls != 1 {
		t.Errorf("agent calls after a repeat = %d, want 1", hs.agent.calls)
	}
	ask(`{"question":"Why China?"}`) // the history now has a turn: a new context
	ask(`{"question":"Which ruling matters most?"}`)
	if hs.agent.calls != 3 {
		t.Errorf("agent calls = %d, want 3", hs.agent.calls)
	}
}
