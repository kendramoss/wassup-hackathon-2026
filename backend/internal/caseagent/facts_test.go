package caseagent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/llm"
)

// bigFacts is n runes: multi-byte filler with a decisive sentence after rune
// 6000 and an appended clarification answer at the very end.
func bigFacts(n int) (facts, decisive, answer string) {
	decisive = "DECISIVE: the gearbox is cast in Vietnamé😀."
	answer = "A: The stator is wound in Malaysia."
	body := []rune(strings.Repeat("é😀日x", n))[:n-len([]rune(answer))]
	r := body
	pos := 6500
	if m := len(r) - len([]rune(decisive)); pos > m {
		pos = m
	}
	copy(r[pos:], []rune(decisive))
	return string(r) + answer, decisive, answer
}

type toolRun struct {
	results []string // tool results, in order, as the model saw them
	steps   []Step
}

// runCalls scripts the fake model to make the given get_case_facts calls
// after get_case, and returns every tool result from the real request bytes.
func runCalls(t *testing.T, facts string, calls []string) toolRun {
	t.Helper()
	var mu sync.Mutex
	n := 0
	f := &llm.Fake{Respond: func(llm.Request) (llm.Response, error) {
		mu.Lock()
		defer mu.Unlock()
		n++
		if n == 1 {
			return llm.Response{ToolCalls: []llm.ToolCall{{ID: "0", Name: "get_case", Args: []byte(`{}`)}}}, nil
		}
		if n-2 < len(calls) {
			return llm.Response{ToolCalls: []llm.ToolCall{{ID: fmt.Sprint(n), Name: "get_case_facts", Args: []byte(calls[n-2])}}}, nil
		}
		return llm.Response{Text: "done"}, nil
	}}
	ans, err := New(f, "m", fakeSearch{}, &fakeEngine{}).Ask(context.Background(), Input{
		Question: "q", CaseSummary: "CASE\n" + FactsSection(facts), Facts: facts, Date: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	reqs := f.Calls()
	last := reqs[len(reqs)-1]
	var run toolRun
	run.steps = ans.Steps
	for _, m := range last.Messages {
		if m.Role != llm.RoleTool {
			continue
		}
		var o struct{ Result string }
		if err := json.Unmarshal([]byte(m.Content), &o); err != nil {
			t.Fatalf("tool content %q: %v", m.Content, err)
		}
		run.results = append(run.results, o.Result)
	}
	return run
}

func body(res string) string { return res[strings.Index(res, "\n")+1:] }

func TestFactsToolRoundTripReconstructsAllFactsWithZeroLoss(t *testing.T) {
	for _, n := range []int{5999, 6000, 6001, 12000} {
		facts, decisive, answer := bigFacts(n)
		sec := FactsSection(facts)
		got := body(sec)
		got = strings.TrimSuffix(got, "\n")
		off := len([]rune(got))
		if n <= FactsHeadRunes {
			if got != facts || strings.Contains(sec, "get_case_facts") {
				t.Fatalf("n=%d: short facts must be whole and unflagged", n)
			}
			continue
		}
		if off != FactsHeadRunes || !strings.Contains(sec, fmt.Sprintf("of %d", n)) || !strings.Contains(sec, fmt.Sprintf("offset=%d", off)) {
			t.Fatalf("n=%d: head not labelled truncated: %q", n, sec[:120])
		}
		var calls []string
		for o := off; o < n; o += MaxFactsPageRunes {
			calls = append(calls, fmt.Sprintf(`{"offset":%d}`, o))
		}
		run := runCalls(t, facts, calls)
		if len(run.results) != 1+len(calls) {
			t.Fatalf("n=%d: %d results", n, len(run.results))
		}
		if !strings.Contains(run.results[0], "get_case_facts") {
			t.Errorf("n=%d: get_case does not say the facts continue", n)
		}
		for _, r := range run.results[1:] {
			if l := len([]rune(body(r))); l > MaxFactsPageRunes {
				t.Errorf("n=%d: page of %d runes exceeds cap", n, l)
			}
			got += body(r)
		}
		if got != facts {
			t.Fatalf("n=%d: reconstruction lost data (%d vs %d runes)", n, len([]rune(got)), n)
		}
		if !strings.Contains(got, decisive) || !strings.HasSuffix(got, answer) {
			t.Errorf("n=%d: decisive fact or appended answer missing", n)
		}
		if last := run.results[len(run.results)-1]; !strings.Contains(last, "end of facts") {
			t.Errorf("n=%d: final page not marked as end: %q", n, last)
		}
	}
}

func TestFactsToolCountsRunesAndKeepsUnicodeWhole(t *testing.T) {
	facts := strings.Repeat("😀", 7000) // 4 bytes per rune
	run := runCalls(t, facts, []string{`{"offset":6999,"limit":10}`})
	r := run.results[1]
	if body(r) != "😀" || !strings.Contains(r, "runes 6999-7000 of 7000") || !strings.Contains(r, "end of facts") {
		t.Errorf("rune offsets wrong: %q", r)
	}
	if run.steps[len(run.steps)-1].Tool != "get_case_facts" {
		t.Errorf("step not recorded: %+v", run.steps)
	}
}

func TestFactsToolLimitsAndInvalidRanges(t *testing.T) {
	facts, _, _ := bigFacts(12000)
	run := runCalls(t, facts, []string{
		`{"offset":6000,"limit":999999}`, // capped, said so
		`{"offset":6000,"limit":50}`,     // smaller limit honoured
		`{"offset":-1}`,
		`{"offset":12001}`,
	})
	run2 := runCalls(t, facts, []string{`{"offset":6000,"limit":-5}`, `{"offset":12000}`})
	run.results = append(run.results, run2.results[1:]...)
	rs := run.results[1:]
	if l := len([]rune(body(rs[0]))); l != MaxFactsPageRunes || !strings.Contains(rs[0], fmt.Sprintf("next_offset=%d", 6000+MaxFactsPageRunes)) {
		t.Errorf("cap not applied/declared: %d %q", l, strings.SplitN(rs[0], "\n", 2)[0])
	}
	if l := len([]rune(body(rs[1]))); l != 50 {
		t.Errorf("limit 50 gave %d", l)
	}
	for i, want := range []string{"invalid offset", "invalid offset", "invalid limit"} {
		if !strings.Contains(rs[2+i], want) || strings.Contains(rs[2+i], "next_offset") {
			t.Errorf("result %d = %q, want %q", 2+i, rs[2+i], want)
		}
	}
	if !strings.Contains(rs[5], "no more facts") || !strings.Contains(rs[5], "12000") {
		t.Errorf("offset==total = %q", rs[5])
	}
	if len(rs) != 6 {
		t.Errorf("results = %d", len(rs))
	}
}

func TestFactsToolServesOnlyThisCase(t *testing.T) {
	// The tool has no path or case parameter: extra arguments cannot widen it.
	run := runCalls(t, "THIS CASE FACTS", []string{`{"offset":0,"path":"/etc/passwd","case_id":"other"}`})
	for _, r := range run.results[1:] {
		if strings.Contains(r, "root:") {
			t.Errorf("leak: %q", r)
		}
	}
	run = runCalls(t, "THIS CASE FACTS", []string{`{"offset":0}`})
	if r := run.results[1]; !strings.Contains(r, "THIS CASE FACTS") {
		t.Errorf("result = %q", r)
	}
}

func TestHistoryBoundUnchanged(t *testing.T) {
	var h []Turn
	for i := 0; i < 9; i++ {
		h = append(h, Turn{Question: fmt.Sprintf("Q%d?", i), Answer: fmt.Sprintf("A%d.", i)})
	}
	m := userMessage(Input{Question: "now", History: h})
	if strings.Contains(m, "Q2?") || !strings.Contains(m, "Q3?") || !strings.Contains(m, "Q8?") || strings.Count(m, "\nQ: ") != MaxHistory || !strings.HasPrefix(m, "EARLIER IN THIS CONVERSATION:\nQ: Q3?") {
		t.Errorf("history not bounded to %d: %q", MaxHistory, m)
	}
}
