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

// The D650 checklists keep engine-v1's task, test and answer fields, require
// every field they define, and the engine keeps the whole answer.
func TestSTChecklistsKeepTheStandardAnswer(t *testing.T) {
	comp := `"assembly_country":"Vietnam","components":[{"part":"motor","origin":"China","essential":true,"processing":"fitted in housing","transforms":"no","precedents":["N300001"]}]`
	tail := `"origin_country":"China","essential_character":"motor","decisive_step":"motor made in China","flip":null,"confidence":0.8,"reasoning":"N300001"`
	for _, c := range []struct {
		name, answer string
		extra        []string
	}{
		{"ck1", "{" + comp + "," + tail + "}", nil},
		{"ck2", "{" + comp + `,"component_precedent":{"ruling":"N300001","differs":"none"},"assembly_precedent":{"ruling":null,"differs":"none"},"closer":"component",` + tail + "}",
			[]string{"component_precedent", "assembly_precedent", "closer"}},
	} {
		opts, ok := STChecklistOptions(c.name)
		if !ok {
			t.Fatalf("%s: not known", c.name)
		}
		req := opts.Schema["required"].([]string)
		props := opts.Schema["properties"].(map[string]any)
		for _, f := range append([]string{"assembly_country", "components", "origin_country", "essential_character", "decisive_step", "flip", "confidence", "reasoning"}, c.extra...) {
			if !slices.Contains(req, f) || props[f] == nil {
				t.Errorf("%s: %s not required", c.name, f)
			}
		}
		if len(req) != len(props) {
			t.Errorf("%s: %d required, %d properties", c.name, len(req), len(props))
		}
		if !strings.HasPrefix(opts.System, stHead) || strings.Contains(opts.System, "<=80 words") {
			t.Errorf("%s: system prompt %.80q", c.name, opts.System)
		}
		opts.Model = "m"
		e := New(llm.FakeText(c.answer), nil, opts)
		res, err := e.Predict(context.Background(), domain.Case{ID: "c", Date: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), Facts: "widgets"})
		if err != nil {
			t.Fatal(err)
		}
		var got map[string]any
		if err := json.Unmarshal(res.Answer, &got); err != nil || got["components"] == nil {
			t.Errorf("%s: answer %s", c.name, res.Answer)
		}
		if res.Prediction.Origin != "China" || res.Prediction.Trace.PromptVersion != opts.Version {
			t.Errorf("%s: origin %q version %q", c.name, res.Prediction.Origin, res.Prediction.Trace.PromptVersion)
		}
	}
	if _, ok := STChecklistOptions("v1"); ok {
		t.Error("v1 is engine-v1, not a checklist")
	}
}
