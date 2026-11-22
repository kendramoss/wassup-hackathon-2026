package engine

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/llm"
)

// The D600 compact checklists keep the standard answer fields and the
// usmca-v1 hierarchy, and the engine keeps the whole answer.
func TestCompactChecklistsKeepTheStandardAnswer(t *testing.T) {
	for _, c := range []struct {
		version, system, field, answer string
		schema                         map[string]any
	}{
		{USMCAVersion4, USMCASystem4, "foreign_materials",
			`{"good_hts":"8501.31","foreign_materials":[{"part":"rotor","hts":"8503.00","origin":"China","shift":"yes"}],"de_minimis":"not needed","rule_step":"(a)(3)","origin_country":"Mexico","essential_character":"(a)(3) shift to 8501.31","decisive_step":"motor assembled in Mexico","flip":null,"confidence":0.9,"reasoning":"r"}`,
			CompactChecklistSchema},
		{USMCAVersion5, USMCASystem5, "failing_materials",
			`{"good_hts":"8501.31","failing_materials":[],"de_minimis":"not needed","rule_step":"(a)(3)","origin_country":"Mexico","essential_character":"(a)(3) shift to 8501.31","decisive_step":"motor assembled in Mexico","flip":null,"confidence":0.9,"reasoning":"r"}`,
			FailingChecklistSchema},
	} {
		req := c.schema["required"].([]string)
		for _, f := range []string{"origin_country", "essential_character", "decisive_step", "flip", "confidence", "reasoning", c.field, "de_minimis", "rule_step", "good_hts"} {
			if !slices.Contains(req, f) {
				t.Errorf("%s: %s not required", c.version, f)
			}
		}
		if !strings.HasPrefix(c.system, usmcaHead) || !strings.Contains(c.system, "102.13") || strings.Contains(c.system, "<=80 words") {
			t.Errorf("%s: system prompt %.80q", c.version, c.system)
		}
		f := llm.FakeText(c.answer)
		e := New(f, nil, Options{Model: "m", System: c.system, Version: c.version, Schema: c.schema})
		res, err := e.Predict(context.Background(), domain.Case{ID: "c", Date: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), Facts: "motors"})
		if err != nil {
			t.Fatal(err)
		}
		var got map[string]any
		if err := json.Unmarshal(res.Answer, &got); err != nil || got[c.field] == nil {
			t.Errorf("%s: answer %s", c.version, res.Answer)
		}
		if res.Prediction.Origin != "Mexico" || res.Prediction.Trace.PromptVersion != c.version {
			t.Errorf("%s: origin %q version %q", c.version, res.Prediction.Origin, res.Prediction.Trace.PromptVersion)
		}
	}
}
