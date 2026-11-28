package engine

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/llm"
)

func answerWith(origin string, conf string) string {
	return `{"origin_country":"` + origin + `","essential_character":"x","decisive_step":"y","flip":null,"confidence":` + conf + `,"reasoning":"r"}`
}

func predictWith(t *testing.T, o Options, origin string) (domain.Prediction, []llm.Request) {
	t.Helper()
	m := llm.FakeText(answerWith(origin, "0.95"))
	e := New(m, nil, o)
	res, err := e.Predict(context.Background(), domain.Case{ID: "c1", Facts: "facts", Date: time.Date(2026, 11, 28, 0, 0, 0, 0, time.UTC)})
	if err != nil {
		t.Fatal(err)
	}
	return res.Prediction, m.Calls()
}

// Dev defect (D970 base run): the pharmaceutical engine answered "Unknown", a
// restated question or two countries at 0.95 confidence, so a fast path above
// 0.92 would accept it. An unresolved origin must never reach the auto bar.
func TestUnresolvedOriginNeverReachesTheAutoBar(t *testing.T) {
	for _, origin := range []string{"Unknown", "Country Where Desoximetasone Api Is Manufactured", "Netherlands/India", "Unknown (Country of Api Manufacture)", "Not stated", ""} {
		p, _ := predictWith(t, Options{Model: "m", RequireSingleCountry: true}, origin)
		if p.Confidence > UnresolvedConfidenceCap {
			t.Errorf("%q kept confidence %.2f", origin, p.Confidence)
		}
	}
	for _, origin := range []string{"India", "Malta", "Latvia", "Macau", "United States"} {
		if p, _ := predictWith(t, Options{Model: "m", RequireSingleCountry: true}, origin); p.Confidence != 0.95 {
			t.Errorf("%q lost its confidence: %.2f", origin, p.Confidence)
		}
	}
	if p, _ := predictWith(t, Options{Model: "m"}, "Unknown"); p.Confidence != 0.95 {
		t.Errorf("the guard is off by default, confidence %.2f", p.Confidence)
	}
}

func TestMissingFactRuleIsAddedAndVersioned(t *testing.T) {
	o := WithMissingFactRule(Options{Model: "m", System: TextileSystem, Version: TextileVersion}, true)
	if !o.RequireSingleCountry || o.RulesVersion != MissingFactVersion {
		t.Fatalf("options = %+v", o)
	}
	p, calls := predictWith(t, o, "India")
	if !strings.Contains(calls[0].System, MissingFactRule) || !strings.HasPrefix(calls[0].System, TextileSystem) {
		t.Error("the rule is not appended to the specialist prompt")
	}
	if !strings.Contains(p.Trace.PromptVersion, MissingFactVersion) || !strings.Contains(p.Trace.PromptVersion, "one-country") {
		t.Errorf("prompt version %q does not record the change", p.Trace.PromptVersion)
	}
	if strings.Contains(MissingFactRule, "\n\n\n") || !strings.Contains(MissingFactRule, "below 0.5") {
		t.Error("rule text")
	}
}

// Dev defect: the 5208-5212 row's first alternative (dyeing and printing with
// two finishing operations) was ignored and the weaving country answered.
func TestTextileV3StatesTheFabricFinishingAlternative(t *testing.T) {
	if TextileVersion3 == TextileVersion || !strings.Contains(TextileSystem3, "both dyeing and printing") ||
		!strings.Contains(TextileSystem3, "5208-5212") || !strings.HasPrefix(TextileSystem3, TextileSystem[:200]) {
		t.Error("textile-v3 prompt")
	}
	if strings.Contains(TextileSystem, "both dyeing and printing") {
		t.Error("textile-v1 must stay as it was")
	}
}
