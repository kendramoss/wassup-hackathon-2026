package verifier

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/llm"
)

// scriptSearch is a synthetic Searcher. When bypass is true it ignores the
// exclusion map and date bound, as a buggy or compromised backend would.
type scriptSearch struct {
	mu     sync.Mutex
	hits   []Hit
	reads  map[string]string
	bypass bool
}

func (s *scriptSearch) Search(_ context.Context, _ string, before time.Time, ex map[string]bool) ([]Hit, error) {
	var out []Hit
	for _, h := range s.hits {
		if !s.bypass && (ex[h.ID] || !h.Date.Before(before)) {
			continue
		}
		out = append(out, h)
	}
	return out, nil
}

func (s *scriptSearch) Read(_ context.Context, id string, before time.Time, ex map[string]bool) (string, error) {
	if !s.bypass && ex[id] {
		return notFound, nil
	}
	if r, ok := s.reads[id]; ok {
		return r, nil
	}
	return notFound, nil
}

var d2025 = time.Date(2025, 3, 1, 0, 0, 0, 0, time.UTC)

func verifyScript(t *testing.T, s Searcher, tk Task, final string, tools ...llm.ToolCall) Result {
	t.Helper()
	var turns []llm.Response
	for _, c := range tools {
		turns = append(turns, llm.Response{ToolCalls: []llm.ToolCall{c}})
	}
	turns = append(turns, llm.Response{Text: final})
	f, _ := scripted(turns...)
	res, err := New(f, s, Options{Model: "m", MaxToolCalls: 6}).Verify(context.Background(), tk)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func answerFor(support, counter string) string {
	q := func(s string) string {
		if s == "" {
			return "null"
		}
		return `"` + s + `"`
	}
	return `{"origin_a":"China","origin_b":"Thailand","changes_origin":true,"supporting_ruling":` + q(support) +
		`,"counter_ruling":` + q(counter) + `,"confidence":0.96,"reasoning":"x"}`
}

// An allowed ruling whose text mentions an excluded ruling must not make the
// excluded id eligible support.
func TestCrossReferencedExcludedRulingIsNotSupport(t *testing.T) {
	s := &scriptSearch{
		hits:  []Hit{{ID: "N350001", Date: d2025, Subject: "hair dryer", Origin: "Thailand"}},
		reads: map[string]string{"N350001": "[N350001 | 2025-03-01] x\nFACTS: like N328954 which also moved the motor. CBP ANALYSIS AND HOLDING: see N328954. Thailand."},
	}
	res := verifyScript(t, s, task(), answerFor("N328954", ""),
		call("c1", "search_rulings", `{"query":"q"}`), call("c2", "read_ruling", `{"ruling":"N350001"}`))
	if res.Verdict.PrecedentSupported || res.Verdict.SupportingRuling != "" {
		t.Errorf("excluded cross-reference accepted: %+v", res.Verdict)
	}
	if len(res.Dropped) != 1 || res.Dropped[0] != "N328954" {
		t.Errorf("dropped = %v", res.Dropped)
	}
	if len(res.Verdict.Trail) != 2 || strings.Join(res.Verdict.Trail[1].Rulings, ",") != "N350001" {
		t.Errorf("trail changed: %+v", res.Verdict.Trail)
	}
}

// An id that is merely mentioned (even an allowed-looking one) in a result is
// not a retrieved identity: only the tool-result list identities count.
func TestNeedleTextIsNotRetrievedIdentity(t *testing.T) {
	s := &scriptSearch{
		hits:  []Hit{{ID: "N350001", Date: d2025, Subject: "dryer, compare N340000 and N777777", Origin: "Thailand"}},
		reads: map[string]string{"N350001": "[N350001] FACTS: cf. N340000. CBP ANALYSIS AND HOLDING: H123456 differs."},
	}
	for _, id := range []string{"N340000", "N777777", "H123456"} {
		res := verifyScript(t, s, task(), answerFor(id, ""),
			call("c1", "search_rulings", `{"query":"q"}`), call("c2", "read_ruling", `{"ruling":"N350001"}`))
		if res.Verdict.PrecedentSupported || res.Verdict.SupportingRuling != "" {
			t.Errorf("%s mentioned in text accepted as support: %+v", id, res.Verdict)
		}
	}
	// Same for the counter-ruling slot.
	res := verifyScript(t, s, task(), answerFor("N350001", "N340000"),
		call("c1", "search_rulings", `{"query":"q"}`))
	if !res.Verdict.PrecedentSupported || res.Verdict.CounterRuling != "" {
		t.Errorf("text-only counter kept: %+v", res.Verdict)
	}
}

func TestValidDirectCitationsStillAccepted(t *testing.T) {
	s := &scriptSearch{
		hits:  []Hit{{ID: "N350001", Date: d2025, Subject: "hair dryer", Origin: "Thailand"}, {ID: "N340000", Date: d2025, Subject: "blower", Origin: "China"}},
		reads: map[string]string{"H345678": "[H345678 | 2024-05-01] y\nFACTS: z. CBP ANALYSIS AND HOLDING: Thailand."},
	}
	// Supported by a search hit, countered by a ruling read directly.
	res := verifyScript(t, s, task(), answerFor("N350001", "H345678"),
		call("c1", "search_rulings", `{"query":"q"}`), call("c2", "read_ruling", `{"ruling":"H345678"}`))
	if !res.Verdict.PrecedentSupported || res.Verdict.SupportingRuling != "N350001" || res.Verdict.CounterRuling != "H345678" || len(res.Dropped) != 0 {
		t.Errorf("valid direct citations rejected: %+v dropped=%v", res.Verdict, res.Dropped)
	}
	// Support that was only read (never in a search list) is direct too.
	res = verifyScript(t, s, task(), answerFor("H345678", ""), call("c1", "read_ruling", `{"ruling":"H345678"}`))
	if !res.Verdict.PrecedentSupported {
		t.Errorf("read-only direct support rejected: %+v", res.Verdict)
	}
}

// A backend that ignores the exclusion map must still not get an excluded
// ruling accepted: acceptance checks Task.Exclude on its own.
func TestExcludedRulingDirectlyRetrievedIsRejectedAtAcceptance(t *testing.T) {
	s := &scriptSearch{
		bypass: true,
		hits:   []Hit{{ID: "N328954", Date: d2025, Subject: "the case's own ruling", Origin: "China"}, {ID: "N340000", Date: d2025, Subject: "blower", Origin: "China"}},
		reads:  map[string]string{"N328954": "[N328954 | 2025-03-01] own\nFACTS: a. CBP ANALYSIS AND HOLDING: China."},
	}
	for name, tools := range map[string][]llm.ToolCall{
		"search hit": {call("c1", "search_rulings", `{"query":"q"}`)},
		"read":       {call("c1", "read_ruling", `{"ruling":"N328954"}`)},
	} {
		res := verifyScript(t, s, task(), answerFor("N328954", "N328954"), tools...)
		if res.Verdict.PrecedentSupported || res.Verdict.SupportingRuling != "" || res.Verdict.CounterRuling != "" {
			t.Errorf("%s: excluded ruling accepted: %+v", name, res.Verdict)
		}
	}
}

// The legal corpus date bound holds even if a backend returns a ruling dated
// on or after Task.Before.
func TestRulingOutsideDateBoundIsNotSupport(t *testing.T) {
	late := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	s := &scriptSearch{
		bypass: true,
		hits:   []Hit{{ID: "N360000", Date: late, Subject: "later ruling", Origin: "Thailand"}},
		reads:  map[string]string{"N360001": "[N360001 | 2026-02-01] later\nFACTS: a. CBP ANALYSIS AND HOLDING: Thailand."},
	}
	for _, id := range []string{"N360000", "N360001"} {
		res := verifyScript(t, s, task(), answerFor(id, ""),
			call("c1", "search_rulings", `{"query":"q"}`), call("c2", "read_ruling", `{"ruling":"N360001"}`))
		if res.Verdict.PrecedentSupported {
			t.Errorf("%s dated after the bound accepted: %+v", id, res.Verdict)
		}
	}
}
