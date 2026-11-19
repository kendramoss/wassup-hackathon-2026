package caseagent

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/llm"
)

// whatIfRun drives a real ADK tool round trip: the fake model calls what_if
// with factsArg, then answers. It returns the answer and the tool message the
// model saw, decoded from the real request bytes.
func whatIfRun(t *testing.T, p Predictor, factsArg string) (Answer, string) {
	t.Helper()
	args, err := json.Marshal(map[string]string{"change": "c", "facts": factsArg})
	if err != nil {
		t.Fatal(err)
	}
	step := 0
	f := &llm.Fake{Respond: func(llm.Request) (llm.Response, error) {
		step++
		if step == 1 {
			return llm.Response{ToolCalls: []llm.ToolCall{{ID: "1", Name: "what_if", Args: args}}}, nil
		}
		return llm.Response{Text: "done"}, nil
	}}
	ans, err := New(f, "m", fakeSearch{}, p).Ask(context.Background(), Input{Question: "What if?", Date: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)})
	if err != nil {
		t.Fatal(err)
	}
	reqs := f.Calls()
	var res string
	for _, m := range reqs[len(reqs)-1].Messages {
		if m.Role == llm.RoleTool && m.Name == "what_if" {
			var o struct{ Result string }
			if err := json.Unmarshal([]byte(m.Content), &o); err != nil {
				t.Fatalf("tool content %q: %v", m.Content, err)
			}
			res = o.Result
		}
	}
	return ans, res
}

func TestWhatIfCanonicalFactsAreOneString(t *testing.T) {
	for _, tt := range []struct{ name, in, want string }{
		{"plain", "Motor made in Thailand.", "Motor made in Thailand.\n"},
		{"trimmed", "  \n Motor made in Thailand.\n\n ", "Motor made in Thailand.\n"},
		{"multibyte", "é😀日 motor", "é😀日 motor\n"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			for _, mode := range []string{"bare", "classify"} {
				var seen *[]domain.Case
				var p Predictor
				if mode == "bare" {
					e := &fakeEngine{}
					p, seen = e, &e.seen
				} else {
					e := &classifyingEngine{k: Classification{Result: res("textile-v1"), Regime: "textile_102_21", Route: domain.RouteAuto}}
					p, seen = e, &e.seen
				}
				ans, _ := whatIfRun(t, p, tt.in)
				if len(*seen) != 1 || len(ans.WhatIfs) != 1 {
					t.Fatalf("%s: predictor saw %d, what-ifs %d", mode, len(*seen), len(ans.WhatIfs))
				}
				if got := (*seen)[0].Facts; got != tt.want || ans.WhatIfs[0].Facts != got {
					t.Errorf("%s: predicted %q returned %q want %q", mode, got, ans.WhatIfs[0].Facts, tt.want)
				}
				if again, _, ok := domain.CanonicalFacts(ans.WhatIfs[0].Facts); !ok || again != ans.WhatIfs[0].Facts {
					t.Errorf("%s: canonical form not stable: %q", mode, again)
				}
			}
		})
	}
}

func TestWhatIfLimitBoundaryCountsRunesNotBytes(t *testing.T) {
	for _, tt := range []struct {
		name  string
		unit  string
		n     int
		wantR bool // rejected
	}{
		{"ascii at limit", "a", domain.MaxFactsRunes, false},
		{"ascii over", "a", domain.MaxFactsRunes + 1, true},
		{"2-byte at limit", "é", domain.MaxFactsRunes, false},
		{"2-byte over", "é", domain.MaxFactsRunes + 1, true},
		{"4-byte emoji at limit", "😀", domain.MaxFactsRunes, false},
		{"4-byte emoji over", "😀", domain.MaxFactsRunes + 1, true},
		{"3-byte CJK at limit", "日", domain.MaxFactsRunes, false},
		{"3-byte CJK over", "日", domain.MaxFactsRunes + 1, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			facts := strings.Repeat(tt.unit, tt.n)
			// Also with surrounding whitespace and a terminal newline: the
			// limit applies to the trimmed text, the newline is not counted.
			for _, arg := range []string{facts, "  " + facts + "\n\n"} {
				e := &classifyingEngine{k: Classification{Result: res("textile-v1"), Regime: "textile_102_21", Route: domain.RouteAuto}}
				ans, msg := whatIfRun(t, e, arg)
				if tt.wantR {
					if e.n != 0 || len(e.seen) != 0 || len(ans.WhatIfs) != 0 {
						t.Fatalf("rejected facts reached the classifier %d times, what-ifs %d", e.n, len(ans.WhatIfs))
					}
					if !strings.Contains(msg, "12000") || !strings.Contains(msg, "Shorten") || !strings.Contains(msg, "Nothing was predicted") {
						t.Errorf("not a recoverable message: %q", msg)
					}
					if len(msg) > 400 {
						t.Errorf("rejection message echoes input (%d bytes)", len(msg))
					}
					continue
				}
				if e.n != 1 || len(ans.WhatIfs) != 1 {
					t.Fatalf("accepted facts: classifier calls %d what-ifs %d (%q)", e.n, len(ans.WhatIfs), msg)
				}
				if e.seen[0].Facts != facts+"\n" || ans.WhatIfs[0].Facts != facts+"\n" {
					t.Errorf("predicted and returned facts differ from the canonical full string")
				}
			}
		})
	}
}

func TestWhatIfOverlongRejectedOnBarePredictorToo(t *testing.T) {
	e := &fakeEngine{}
	ans, msg := whatIfRun(t, e, strings.Repeat("x", domain.MaxFactsRunes+1))
	if len(e.seen) != 0 || len(ans.WhatIfs) != 0 || !strings.Contains(msg, "Shorten") {
		t.Errorf("predict calls %d what-ifs %d msg %q", len(e.seen), len(ans.WhatIfs), msg)
	}
	// The rejection does not use the what-if budget: a shortened retry works.
	if len(ans.Steps) != 1 || ans.Steps[0].Tool != "what_if" {
		t.Errorf("steps = %+v", ans.Steps)
	}
}

func TestWhatIfEmptyFactsStillRejected(t *testing.T) {
	e := &fakeEngine{}
	ans, msg := whatIfRun(t, e, " \n ")
	if len(e.seen) != 0 || len(ans.WhatIfs) != 0 || msg != "facts are required" {
		t.Errorf("predict calls %d msg %q", len(e.seen), msg)
	}
}

func TestToolBudgetsUnchanged(t *testing.T) {
	if MaxToolCalls != 6 || MaxHistory != 6 || MaxWhatIfs != 2 {
		t.Errorf("budgets changed: tool calls %d history %d what-ifs %d", MaxToolCalls, MaxHistory, MaxWhatIfs)
	}
}
