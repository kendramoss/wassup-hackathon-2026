package verifier

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/llm"
)

type fakeSearch struct {
	mu       sync.Mutex
	queries  []string
	reads    []string
	before   time.Time
	excluded map[string]bool
}

func (f *fakeSearch) Search(_ context.Context, q string, before time.Time, ex map[string]bool) ([]Hit, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.queries, f.before, f.excluded = append(f.queries, q), before, ex
	d := time.Date(2025, 3, 1, 0, 0, 0, 0, time.UTC)
	return []Hit{{ID: "N350001", Date: d, Subject: "hair dryer", Origin: "Thailand"}, {ID: "N340000", Date: d, Subject: "blower", Origin: "China"}}, nil
}

func (f *fakeSearch) Read(_ context.Context, id string, _ time.Time, _ map[string]bool) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.reads = append(f.reads, id)
	return "[" + id + "] FACTS: motor made in Thailand. CBP ANALYSIS AND HOLDING: Thailand.", nil
}

// scripted answers each model turn in order; after the script it repeats the last.
func scripted(turns ...llm.Response) (*llm.Fake, *[]llm.Request) {
	var mu sync.Mutex
	var reqs []llm.Request
	i := 0
	f := &llm.Fake{Respond: func(r llm.Request) (llm.Response, error) {
		mu.Lock()
		defer mu.Unlock()
		reqs = append(reqs, r)
		t := turns[min(i, len(turns)-1)]
		i++
		t.Usage = llm.Usage{InputTokens: 100, OutputTokens: 10, TotalTokens: 110}
		return t, nil
	}}
	return f, &reqs
}

func call(id, name, args string) llm.ToolCall {
	return llm.ToolCall{ID: id, Name: name, Args: json.RawMessage(args)}
}

func task() Task {
	return Task{FactsA: "A: motor in China", FactsB: "B: motor in Thailand", Target: "Thailand",
		Before: time.Date(2026, 1, 14, 0, 0, 0, 0, time.UTC), Exclude: []string{"N328954"}}
}

func TestVerifySupported(t *testing.T) {
	f, reqs := scripted(
		llm.Response{ToolCalls: []llm.ToolCall{call("c1", "search_rulings", `{"query":"heat gun motor Thailand"}`)}},
		llm.Response{ToolCalls: []llm.ToolCall{call("c2", "read_ruling", `{"ruling":"N350001"}`)}},
		llm.Response{Text: `{"origin_a":"PRC","origin_b":"Thailand","changes_origin":true,"supporting_ruling":"N350001","counter_ruling":"N340000","confidence":0.96,"reasoning":"Like N350001."}`},
	)
	s := &fakeSearch{}
	res, err := New(f, s, Options{Model: "gemini-4-argon", MaxToolCalls: 6, MaxOutputTokens: 6000}).Verify(context.Background(), task())
	if err != nil {
		t.Fatal(err)
	}
	v := res.Verdict
	if !v.PrecedentSupported || v.SupportingRuling != "N350001" || v.CounterRuling != "N340000" || v.Confidence != 0.96 {
		t.Errorf("verdict = %+v", v)
	}
	if res.OriginA != "China" || res.OriginB != "Thailand" || len(res.Steps) != 2 || res.ModelCalls != 3 {
		t.Errorf("result = %+v", res)
	}
	if res.Usage.InputTokens != 300 || v.Trace.PromptVersion != PromptVersion {
		t.Errorf("usage/trace = %+v %+v", res.Usage, v.Trace)
	}
	if !s.excluded["N328954"] || !s.before.Equal(task().Before) {
		t.Errorf("search bounds not passed: before=%v exclude=%v", s.before, s.excluded)
	}
	first := (*reqs)[0]
	if !strings.Contains(first.System, "at most 6 calls") || len(first.Tools) != 2 || !strings.Contains(first.Messages[0].Content, "VARIANT B FACTS:") {
		t.Errorf("first request = %+v", first)
	}
	// The second request must carry the tool call and its result back.
	second := (*reqs)[1]
	var sawCall, sawResult bool
	for _, m := range second.Messages {
		if m.Role == llm.RoleAssistant && len(m.ToolCalls) == 1 && m.ToolCalls[0].ID == "c1" {
			sawCall = true
		}
		if m.Role == llm.RoleTool && m.ToolCallID == "c1" && strings.Contains(m.Content, "N350001 | 2025-03-01 | hair dryer | CBP held: Thailand") {
			sawResult = true
		}
	}
	if !sawCall || !sawResult {
		t.Errorf("tool round trip missing in %+v", second.Messages)
	}
}

func TestVerifyRequiresEvidenceAndTarget(t *testing.T) {
	tests := []struct {
		name, final string
		supported   bool
		dropped     int
	}{
		{"no change claimed", `{"origin_a":"China","origin_b":"China","changes_origin":false,"supporting_ruling":null,"counter_ruling":"N340000","confidence":0.9,"reasoning":"x"}`, false, 0},
		{"wrong target country", `{"origin_a":"China","origin_b":"Vietnam","changes_origin":true,"supporting_ruling":"N350001","confidence":0.97,"reasoning":"x"}`, false, 0},
		{"hallucinated support is dropped", `{"origin_a":"China","origin_b":"Thailand","changes_origin":true,"supporting_ruling":"N999999","confidence":0.97,"reasoning":"x"}`, false, 1},
		{"null string is ignored", `{"origin_a":"China","origin_b":"Thailand","changes_origin":true,"supporting_ruling":"null","confidence":0.97,"reasoning":"x"}`, false, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, _ := scripted(
				llm.Response{ToolCalls: []llm.ToolCall{call("c1", "search_rulings", `{"query":"q"}`)}},
				llm.Response{Text: tt.final},
			)
			res, err := New(f, &fakeSearch{}, Options{Model: "m", MaxToolCalls: 6}).Verify(context.Background(), task())
			if err != nil {
				t.Fatal(err)
			}
			if res.Verdict.PrecedentSupported != tt.supported || len(res.Dropped) != tt.dropped {
				t.Errorf("supported=%v dropped=%v", res.Verdict.PrecedentSupported, res.Dropped)
			}
		})
	}
}

func TestVerifyEnforcesToolBudget(t *testing.T) {
	// The model keeps calling tools; after the budget the tools are withheld
	// and it must answer.
	var turn int
	f := &llm.Fake{Respond: func(r llm.Request) (llm.Response, error) {
		turn++
		if len(r.Tools) == 0 {
			return llm.Response{Text: `{"origin_a":"China","origin_b":"China","changes_origin":false,"confidence":0.8,"reasoning":"budget"}`}, nil
		}
		return llm.Response{ToolCalls: []llm.ToolCall{call("c", "search_rulings", `{"query":"again"}`)}}, nil
	}}
	s := &fakeSearch{}
	res, err := New(f, s, Options{Model: "m", MaxToolCalls: 3}).Verify(context.Background(), task())
	if err != nil {
		t.Fatal(err)
	}
	if len(s.queries) != 3 || res.Verdict.PrecedentSupported {
		t.Errorf("searches = %d (want 3), result %+v", len(s.queries), res)
	}
}

func TestKnownOriginAInTask(t *testing.T) {
	tk := task()
	if strings.Contains(tk.UserMessage(), "already ruled") || tk.promptVersion() != PromptVersion {
		t.Error("unknown A must leave the research task text unchanged")
	}
	tk.KnownOriginA, tk.KnownRulingA = "China", "N328954"
	if !strings.HasSuffix(tk.UserMessage(), "CBP has already ruled on variant A: origin China (N328954; that ruling is not available to you).") {
		t.Errorf("message = %q", tk.UserMessage())
	}
	if tk.promptVersion() != "verifier-v1+known-a" {
		t.Errorf("version = %q", tk.promptVersion())
	}
}
