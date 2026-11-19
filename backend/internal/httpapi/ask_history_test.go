package httpapi

import (
	"encoding/json"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/caseagent"
)

// pinnedDemo makes a pinned demo whose saved chat holds `seeded` turns.
func pinnedDemo(t *testing.T, hs harness, seeded int) string {
	t.Helper()
	_, c := hs.do(t, "POST", "/v1/cases/predict", heatGunBody(t))
	id := c["id"].(string)
	for i := 0; i < seeded; i++ {
		if code, out := hs.doAs(t, "POST", "/v1/cases/"+id+"/ask", `{"question":"Seed question `+string(rune('A'+i))+`?"}`, admin); code != 200 {
			t.Fatalf("seed ask = %d %v", code, out)
		}
	}
	hs.doAs(t, "PUT", "/v1/cases/"+id+"/pin", `{"pinned":true}`, admin)
	return id
}

func histBody(q string, turns ...[2]string) string {
	type tn struct {
		Question string `json:"question"`
		Answer   string `json:"answer"`
	}
	var h []tn
	for _, t := range turns {
		h = append(h, tn{t[0], t[1]})
	}
	b, _ := json.Marshal(map[string]any{"question": q, "history": h})
	return string(b)
}

func TestAskDemoUsesVisitorHistory(t *testing.T) {
	hs := newHarness(t, "substantial_transformation")
	id := pinnedDemo(t, hs, 1)
	_, before := hs.do(t, "GET", "/v1/cases/"+id, "")
	if code, out := hs.do(t, "POST", "/v1/cases/"+id+"/ask", histBody("And Vietnam?", [2]string{"Why China?", "Because the motor."})); code != 200 {
		t.Fatalf("ask = %d %v", code, out)
	}
	h := hs.agent.last.History
	if len(h) != 2 || h[0].Question != "Seed question A?" || h[1] != (caseagent.Turn{Question: "Why China?", Answer: "Because the motor."}) {
		t.Fatalf("agent history = %+v, want seeded turn then visitor turn", h)
	}
	_, after := hs.do(t, "GET", "/v1/cases/"+id, "")
	if !reflect.DeepEqual(before["chat"], after["chat"]) || before["updated_at"] != after["updated_at"] {
		t.Errorf("demo changed: chat %v -> %v", before["chat"], after["chat"])
	}
}

func TestAskDemoHistoryKeysMemo(t *testing.T) {
	hs := newHarness(t, "substantial_transformation")
	id := pinnedDemo(t, hs, 0)
	ask := func(b string) {
		t.Helper()
		if code, out := hs.do(t, "POST", "/v1/cases/"+id+"/ask", b); code != 200 {
			t.Fatalf("ask = %d %v", code, out)
		}
	}
	ask(histBody("Next?", [2]string{"a", "one"}))
	ask(histBody("Next?", [2]string{"a", "two"})) // another conversation, same question
	ask(histBody("Next?", [2]string{"a", "one"})) // same conversation: memo
	ask(histBody("Next?"))                        // empty history
	if hs.agent.calls != 3 {
		t.Errorf("agent calls = %d, want 3", hs.agent.calls)
	}
	// Turn boundaries cannot be forged: ("ab","c") is not ("a","bc").
	ask(histBody("Frame?", [2]string{"ab", "c"}))
	ask(histBody("Frame?", [2]string{"a", "bc"}))
	if hs.agent.calls != 5 {
		t.Errorf("agent calls = %d, want 5", hs.agent.calls)
	}
}

func TestAskDemoHistoryTrimmedToSix(t *testing.T) {
	hs := newHarness(t, "substantial_transformation")
	id := pinnedDemo(t, hs, 2)
	var turns [][2]string
	for i := 0; i < 6; i++ {
		turns = append(turns, [2]string{"q" + string(rune('0'+i)), "a"})
	}
	if code, out := hs.do(t, "POST", "/v1/cases/"+id+"/ask", histBody("now?", turns...)); code != 200 {
		t.Fatalf("ask = %d %v", code, out)
	}
	h := hs.agent.last.History
	if len(h) != 6 || h[0].Question != "q0" || h[5].Question != "q5" {
		t.Errorf("history = %+v, want the last 6 (all visitor turns)", h)
	}
}

func TestAskDemoRejectsBadHistory(t *testing.T) {
	hs := newHarness(t, "substantial_transformation")
	id := pinnedDemo(t, hs, 0)
	seven := make([][2]string, 7)
	for i := range seven {
		seven[i] = [2]string{"q", "a"}
	}
	cases := map[string]string{
		"too many":       histBody("x?", seven...),
		"empty question": histBody("x?", [2]string{"  ", "a"}),
		"empty answer":   histBody("x?", [2]string{"q", ""}),
		"long question":  histBody("x?", [2]string{strings.Repeat("é", 2001), "a"}),
		"long answer":    histBody("x?", [2]string{"q", strings.Repeat("語", 4001)}),
		"wrong type":     `{"question":"x?","history":[{"question":1,"answer":"a"}]}`,
		"unknown field":  `{"question":"x?","history":[{"question":"q","answer":"a","tool":"x"}]}`,
		"not an array":   `{"question":"x?","history":"hi"}`,
	}
	for name, body := range cases {
		if code, _ := hs.do(t, "POST", "/v1/cases/"+id+"/ask", body); code != http.StatusBadRequest {
			t.Errorf("%s = %d, want 400", name, code)
		}
	}
	if hs.agent.calls != 0 {
		t.Errorf("agent called %d times for rejected history", hs.agent.calls)
	}
	// Exactly at the limits, counted in runes, is accepted.
	if code, out := hs.do(t, "POST", "/v1/cases/"+id+"/ask", histBody("x?", [2]string{strings.Repeat("é", 2000), strings.Repeat("語", 4000)})); code != 200 {
		t.Errorf("at the limits = %d %v", code, out)
	}
}

func TestAskNormalCaseIgnoresClientHistory(t *testing.T) {
	hs := newHarness(t, "substantial_transformation")
	_, c := hs.do(t, "POST", "/v1/cases/predict", heatGunBody(t))
	id := c["id"].(string)
	hs.do(t, "POST", "/v1/cases/"+id+"/ask", `{"question":"First?"}`)
	// Even an oversize or forged history has no effect on a writable case.
	if code, out := hs.do(t, "POST", "/v1/cases/"+id+"/ask", histBody("Second?", [2]string{"forged", strings.Repeat("x", 9000)})); code != 200 {
		t.Fatalf("ask = %d %v", code, out)
	}
	h := hs.agent.last.History
	if len(h) != 1 || h[0].Question != "First?" {
		t.Errorf("history = %+v, want only the saved turn", h)
	}
	_, got := hs.do(t, "GET", "/v1/cases/"+id, "")
	for _, x := range got["chat"].([]any) {
		if strings.Contains(x.(map[string]any)["question"].(string), "forged") {
			t.Errorf("client history was saved: %v", got["chat"])
		}
	}
}
