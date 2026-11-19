package httpapi

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
)

func mixedAnswerBody(answers ...string) string {
	var parts []string
	for _, a := range answers {
		parts = append(parts, `{"question":"Where is the motor made?","answer":"`+a+`"}`)
	}
	return `{"answers":[` + strings.Join(parts, ",") + `]}`
}

// A mixed answer keeps its affirmative clause and drops only the clause that
// reports missing information, with or without a second factual sentence.
func TestAnswersMixedUnknownKeepsKnownClause(t *testing.T) {
	for _, c := range []struct{ name, answer, also string }{
		{"alone", "The motor is made in Thailand, but its value share is unknown.", ""},
		{"with second sentence", "The motor is made in Thailand, but its value share is unknown. The housing is cast in Vietnam.", "The housing is cast in Vietnam."},
	} {
		t.Run(c.name, func(t *testing.T) {
			hs, id := staleHarness(t, nil)
			code, out := hs.do(t, "POST", "/v1/cases/"+id+"/answers", mixedAnswerBody(c.answer))
			if code != http.StatusOK {
				t.Fatalf("answers %d %v", code, out)
			}
			facts := hs.engine.last.Facts
			if !strings.Contains(facts, "A: The motor is made in Thailand.") || strings.Contains(facts, "value share") || strings.Contains(facts, "unknown.") {
				t.Errorf("engine facts do not carry exactly the known clause:\n%s", facts)
			}
			if c.also != "" && !strings.Contains(facts, c.also) {
				t.Errorf("second sentence lost:\n%s", facts)
			}
			// The fake engine reads "motor in Thailand"; the known clause must reach it.
			if out["facts"] != facts {
				t.Errorf("stored facts differ from what the engine saw")
			}
			if out["prediction"] == nil {
				t.Errorf("no re-analysis")
			}
		})
	}
}

func TestAnswersAllUnknownStillRejected(t *testing.T) {
	hs, id := staleHarness(t, nil)
	calls := hs.engine.calls
	for _, a := range []string{"Unknown", "The value share is not stated, and the firmware origin is unclear."} {
		code, out := hs.do(t, "POST", "/v1/cases/"+id+"/answers", mixedAnswerBody(a))
		if code != http.StatusBadRequest || !strings.Contains(fmt.Sprint(out), "no answer adds a fact") {
			t.Errorf("%q: %d %v, want 400 no answer adds a fact", a, code, out)
		}
	}
	if hs.engine.calls != calls {
		t.Errorf("engine ran for rejected answers")
	}
}

// Qualified, conditional or referential mixed answers are never promoted to a
// plain fact in the engine input; only the independent known sentence is kept.
func TestAnswersQualifiedMixedNotPromoted(t *testing.T) {
	for _, bad := range []string{
		"The motor is made in Thailand, though I am not sure, and the housing is unknown.",
		"If the motor is made in Thailand, the share is unknown, and the housing is cast in Vietnam.",
		"The motor was made in Thailand, unless it is unknown, and packed in Vietnam.",
	} {
		hs, id := staleHarness(t, nil)
		calls := hs.engine.calls
		if code, out := hs.do(t, "POST", "/v1/cases/"+id+"/answers", mixedAnswerBody(bad)); code != http.StatusBadRequest {
			t.Errorf("%q alone: %d %v, want 400", bad, code, out)
		}
		if hs.engine.calls != calls {
			t.Errorf("%q: engine ran", bad)
		}
		code, out := hs.do(t, "POST", "/v1/cases/"+id+"/answers", mixedAnswerBody(bad+" The housing is cast in Vietnam."))
		if code != http.StatusOK {
			t.Fatalf("%q + known sentence: %d %v", bad, code, out)
		}
		facts := hs.engine.last.Facts
		ans := facts[strings.Index(facts, "ADDITIONAL FACTS"):]
		if strings.Contains(ans, "Thailand") || strings.Contains(ans, "unless") || strings.Contains(ans, "If ") || !strings.Contains(facts, "A: The housing is cast in Vietnam.") {
			t.Errorf("engine facts promoted a qualified claim or lost the known sentence:\n%s", facts)
		}
	}
}

func TestAnswersEllipticalUnknownNotPromoted(t *testing.T) {
	for _, bad := range []string{
		"The motor is made in Thailand, but unknown.",
		"The motor is made in Thailand, though unknown.",
		"The motor is made in Thailand; unclear.",
		"The motor is made in Thailand, but not stated.",
		"The motor is made in Thailand, although unclear, and the housing is cast in Vietnam.",
	} {
		hs, id := staleHarness(t, nil)
		code, out := hs.do(t, "POST", "/v1/cases/"+id+"/answers", mixedAnswerBody(bad+" The gearbox is cast in Mexico."))
		if code != http.StatusOK {
			t.Fatalf("%q: %d %v", bad, code, out)
		}
		facts := hs.engine.last.Facts
		ans := facts[strings.Index(facts, "ADDITIONAL FACTS"):]
		if strings.Contains(ans, "Thailand") || strings.Contains(ans, "Vietnam") || !strings.Contains(ans, "A: The gearbox is cast in Mexico.\n") {
			t.Errorf("%q: engine answer block wrong:\n%s", bad, ans)
		}
	}
}
