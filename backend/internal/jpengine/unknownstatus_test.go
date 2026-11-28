package jpengine

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/llm"
)

// Dev defect (D970 base2): with a material whose origin status was "not
// known", the engine answered does_not_qualify at 0.95, although its own prompt
// reserves does_not_qualify for requirements that demonstrably fail.
func TestUnknownStatusRuleIsOptInAndVersioned(t *testing.T) {
	var systems []string
	m := &llm.Fake{Respond: func(r llm.Request) (llm.Response, error) {
		systems = append(systems, r.System)
		return llm.Response{Text: `{"outcome":"undetermined","criterion":"NONE","origin_country":"","deciding_rule":"CC","confidence":0.8,"reasoning":"r","cited_rulings":[]}`}, nil
	}}
	c := domain.Case{ID: "x1", Date: time.Date(2026, 11, 28, 0, 0, 0, 0, time.UTC), Japan: &domain.JapanInput{Agreement: "JP-Indonesia EPA", HS: "9605.00"},
		Facts: "Materials: seal kit (HS 9615.90), origin status not known, 31% of the ex-works price."}
	for _, withRule := range []bool{false, true} {
		e, err := NewDefault(m, DefaultModel, DefaultK)
		if err != nil {
			t.Fatal(err)
		}
		if withRule {
			e = e.WithUnknownStatusRule()
		}
		res, err := e.Predict(context.Background(), c)
		if err != nil {
			t.Fatal(err)
		}
		got := systems[len(systems)-1]
		if has := strings.Contains(got, UnknownStatusRule); has != withRule {
			t.Errorf("rule present = %v, want %v", has, withRule)
		}
		wantVer := PromptVersion
		if withRule {
			wantVer = PromptVersionUnknownStatus
		}
		if res.Prediction.Trace.PromptVersion != wantVer {
			t.Errorf("prompt version %q, want %q", res.Prediction.Trace.PromptVersion, wantVer)
		}
	}
}
