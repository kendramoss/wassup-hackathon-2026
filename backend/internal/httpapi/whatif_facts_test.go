package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/caseagent"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/llm"
)

// realAgent wires the real case agent (fake model, the harness engine) so the
// what_if tool runs through the ADK round trip behind the real HTTP routes.
func realAgentHarness(t *testing.T, factsArg string) (harness, string) {
	t.Helper()
	args, err := json.Marshal(map[string]string{"change": "motor in Thailand", "facts": factsArg})
	if err != nil {
		t.Fatal(err)
	}
	step := 0
	m := &llm.Fake{Respond: func(llm.Request) (llm.Response, error) {
		step++
		if step == 1 {
			return llm.Response{ToolCalls: []llm.ToolCall{{ID: "1", Name: "what_if", Args: args}}}, nil
		}
		return llm.Response{Text: "done"}, nil
	}}
	var eng *fakeEngine
	hs := newHarnessWith(t, "substantial_transformation", func(d *Deps) {
		eng = d.Engine.(*fakeEngine)
		d.Agent = caseagent.New(m, "m", nil, eng)
	})
	_, c := hs.do(t, "POST", "/v1/cases/predict", heatGunBody(t))
	return hs, c["id"].(string)
}

// The facts the what-if predicted, the card it returned, the chat turn that
// was persisted and the scenario added from the card are one string.
func TestWhatIfFactsPredictedStoredAndAddedAsScenarioAreIdentical(t *testing.T) {
	for _, tt := range []struct{ name, facts string }{
		{"normal", "Heat gun; motor in Thailand."},
		{"padded", "\n  Heat gun; motor in Thailand.  \n\n"},
		{"multibyte at limit", strings.Repeat("é😀日", domain.MaxFactsRunes/3) + "x motor in Thailand"[:0] + strings.Repeat("y", domain.MaxFactsRunes%3)},
		{"exactly limit plus newline", strings.Repeat("a", domain.MaxFactsRunes-len(" motor in Thailand")) + " motor in Thailand\n"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if n := len([]rune(strings.TrimSpace(tt.facts))); n > domain.MaxFactsRunes {
				t.Fatalf("fixture is %d runes", n)
			}
			hs, id := realAgentHarness(t, tt.facts)
			before := hs.engine.calls
			code, out := hs.do(t, "POST", "/v1/cases/"+id+"/ask", `{"question":"What if the motor were made in Thailand?"}`)
			if code != http.StatusOK {
				t.Fatalf("ask %d %v", code, out)
			}
			if hs.engine.calls != before+1 {
				t.Fatalf("predictor calls %d -> %d", before, hs.engine.calls)
			}
			predicted := hs.engine.last.Facts
			card := out["reply"].(map[string]any)["what_ifs"].([]any)[0].(map[string]any)["facts"].(string)
			stored := out["chat"].([]any)[0].(map[string]any)["what_ifs"].([]any)[0].(map[string]any)["facts"].(string)
			_, reloaded := hs.do(t, "GET", "/v1/cases/"+id, "")
			persisted := reloaded["chat"].([]any)[0].(map[string]any)["what_ifs"].([]any)[0].(map[string]any)["facts"].(string)
			if card != predicted || stored != predicted || persisted != predicted {
				t.Fatalf("predicted %.40q card %.40q stored %.40q reloaded %.40q", predicted, card, stored, persisted)
			}
			// Add as scenario: send the persisted facts back unchanged.
			body, _ := json.Marshal(map[string]any{"scenarios": []map[string]string{{"label": "From what-if", "facts": persisted}}})
			code, sc := hs.do(t, "POST", "/v1/cases/"+id+"/scenarios", string(body))
			if code != http.StatusOK {
				t.Fatalf("add as scenario %d %v", code, sc)
			}
			if got := sc["scenarios"].([]any)[0].(map[string]any)["facts"]; got != predicted {
				t.Errorf("scenario facts %.40q != predicted %.40q", got, predicted)
			}
			if hs.engine.last.Facts != predicted {
				t.Errorf("scenario predicted %.40q, what-if predicted %.40q", hs.engine.last.Facts, predicted)
			}
		})
	}
}

// An over-limit revision is rejected before any prediction; the persisted
// turn carries no what-if and nothing can be added as a scenario from it.
func TestOverlongWhatIfPredictsNothingAndStoresNoCard(t *testing.T) {
	hs, id := realAgentHarness(t, strings.Repeat("日", domain.MaxFactsRunes+1))
	before := hs.engine.calls
	code, out := hs.do(t, "POST", "/v1/cases/"+id+"/ask", `{"question":"What if?"}`)
	if code != http.StatusOK {
		t.Fatalf("ask %d %v", code, out)
	}
	if hs.engine.calls != before {
		t.Errorf("overlong facts caused %d predict calls", hs.engine.calls-before)
	}
	if w := out["reply"].(map[string]any)["what_ifs"].([]any); len(w) != 0 {
		t.Errorf("what-ifs = %v", w)
	}
	if w, _ := out["chat"].([]any)[0].(map[string]any)["what_ifs"].([]any); len(w) != 0 {
		t.Errorf("stored what-ifs = %v", w)
	}
}

// The scenario endpoint holds the same limit and form as the tool.
func TestScenarioFactsLimitAndCanonicalForm(t *testing.T) {
	hs := newHarness(t, "substantial_transformation")
	_, c := hs.do(t, "POST", "/v1/cases/predict", heatGunBody(t))
	id := c["id"].(string)
	post := func(facts string) (int, map[string]any) {
		b, _ := json.Marshal(map[string]any{"scenarios": []map[string]string{{"facts": facts}}})
		return hs.do(t, "POST", "/v1/cases/"+id+"/scenarios", string(b))
	}
	for _, unit := range []string{"a", "é", "😀"} {
		at := strings.Repeat(unit, domain.MaxFactsRunes)
		if code, out := post(at); code != http.StatusOK || out["scenarios"].([]any)[0].(map[string]any)["facts"] != at+"\n" {
			t.Errorf("%q at limit: %d", unit, code)
		}
		if code, _ := post(at + "\n"); code != http.StatusOK {
			t.Errorf("%q at limit plus newline: %d", unit, code)
		}
		calls := hs.engine.calls
		if code, out := post(at + unit); code != http.StatusBadRequest || hs.engine.calls != calls {
			t.Errorf("%q over limit: %d %v, predict calls %d", unit, code, out, hs.engine.calls-calls)
		}
	}
}
