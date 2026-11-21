package verifier

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/llm"
)

// revokedSearch returns N345891 (11 March 2025), which H346161 revoked on
// 5 May 2025 (D320), next to an ordinary ruling.
type revokedSearch struct{ out []string }

func (s *revokedSearch) Search(_ context.Context, _ string, before time.Time, _ map[string]bool) ([]Hit, error) {
	hits := []Hit{
		withSupersession(Hit{ID: "N345891", Date: time.Date(2025, 3, 11, 0, 0, 0, 0, time.UTC), Subject: "pillow cover", Origin: "China"}, before),
		withSupersession(Hit{ID: "N340000", Date: time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC), Subject: "blower", Origin: "China"}, before),
	}
	s.out = append(s.out, formatHits(hits))
	return hits, nil
}

func (s *revokedSearch) Read(_ context.Context, id string, before time.Time, _ map[string]bool) (string, error) {
	out := "[" + id + " | 2025-03-11] pillow cover\n" + supersessionNote(id, before) + "FACTS: sewn in Thailand. CBP ANALYSIS AND HOLDING: Thailand."
	s.out = append(s.out, out)
	return out, nil
}

func citesRevoked(t *testing.T, before time.Time) (Result, *revokedSearch) {
	t.Helper()
	f, _ := scripted(
		llm.Response{ToolCalls: []llm.ToolCall{call("c1", "search_rulings", `{"query":"pillow cover sewn"}`)}},
		llm.Response{ToolCalls: []llm.ToolCall{call("c2", "read_ruling", `{"ruling":"N345891"}`)}},
		llm.Response{Text: `{"origin_a":"China","origin_b":"Thailand","changes_origin":true,"supporting_ruling":"N345891","counter_ruling":"N340000","confidence":0.95,"reasoning":"Like N345891."}`},
	)
	s := &revokedSearch{}
	tk := task()
	tk.Before = before
	res, err := New(f, s, Options{Model: "m", MaxToolCalls: 6, MaxOutputTokens: 6000}).Verify(context.Background(), tk)
	if err != nil {
		t.Fatal(err)
	}
	return res, s
}

func TestVerifierNeverCitesARulingRevokedByTheCaseDate(t *testing.T) {
	res, s := citesRevoked(t, time.Date(2026, 1, 14, 0, 0, 0, 0, time.UTC))
	v := res.Verdict
	if v.PrecedentSupported || v.SupportingRuling != "" || v.CounterRuling != "N340000" {
		t.Errorf("verdict = %+v", v)
	}
	if len(res.Dropped) != 1 || res.Dropped[0] != "N345891" {
		t.Errorf("dropped = %v", res.Dropped)
	}
	all := strings.Join(s.out, "\n")
	if !strings.Contains(all, "N345891 | 2025-03-11 | pillow cover | CBP held: China | SUPERSEDED: revoked by H346161 on 2025-05-05 (do not cite)") ||
		!strings.Contains(all, "NOTE: CBP revoked this ruling (H346161, 2025-05-05)") {
		t.Errorf("the agent was not shown the supersession:\n%s", all)
	}
	if strings.Contains(s.out[0], "N340000 | 2024-03-01 | blower | CBP held: China | SUPERSEDED") {
		t.Error("an ordinary ruling was marked superseded")
	}
}

func TestVerifierKeepsARulingRevokedAfterTheCaseDate(t *testing.T) {
	res, s := citesRevoked(t, time.Date(2025, 5, 1, 0, 0, 0, 0, time.UTC)) // a backtest before H346161
	if !res.Verdict.PrecedentSupported || res.Verdict.SupportingRuling != "N345891" || len(res.Dropped) != 0 {
		t.Errorf("verdict = %+v dropped %v", res.Verdict, res.Dropped)
	}
	if strings.Contains(strings.Join(s.out, "\n"), "SUPERSEDED") || strings.Contains(strings.Join(s.out, "\n"), "NOTE: CBP") {
		t.Error("a ruling still in force at the case date was marked superseded")
	}
}

func TestCachedVerdictCitingARevokedRulingIsAMiss(t *testing.T) {
	stored := func(tk Task) Result {
		return Result{OriginA: "China", OriginB: "Thailand", ChangesOrigin: true,
			Verdict: domain.Verdict{PrecedentSupported: true, Confidence: 0.95, SupportingRuling: "N345891", CounterRuling: "N340000"},
			Proof:   ProofFor(tk, "N345891", "N340000")}
	}
	late := task()
	if why := cachedViolation(stored(late), late); !strings.Contains(why, "N345891 revoked by H346161") {
		t.Errorf("violation = %q", why)
	}
	clean, changed := sanitize(stored(late), late)
	if !changed || clean.Verdict.PrecedentSupported || clean.Verdict.SupportingRuling != "" || clean.Verdict.CounterRuling != "N340000" {
		t.Errorf("sanitized = %+v", clean.Verdict)
	}
	early := task()
	early.Before = time.Date(2025, 5, 1, 0, 0, 0, 0, time.UTC)
	if why := cachedViolation(stored(early), early); why != "" {
		t.Errorf("a verdict from before the revocation was refused: %q", why)
	}
	counter := stored(late)
	counter.Verdict.SupportingRuling, counter.Verdict.CounterRuling = "N340000", "N345891"
	counter.Proof = ProofFor(late, "N340000", "N345891")
	if c, _ := sanitize(counter, late); c.Verdict.CounterRuling != "" || c.Verdict.SupportingRuling != "N340000" || !c.Verdict.PrecedentSupported {
		t.Errorf("a revoked counter ruling must be dropped and the support kept: %+v", c.Verdict)
	}
}

func TestEligibilityChecksSupersession(t *testing.T) {
	late := time.Date(2026, 1, 14, 0, 0, 0, 0, time.UTC)
	hits := []Hit{{ID: "N345891", Date: time.Date(2025, 3, 11, 0, 0, 0, 0, time.UTC)}, {ID: "N340000", Date: time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC)}}
	if got := eligibleHits(hits, late, nil); len(got) != 1 || got[0] != "N340000" {
		t.Errorf("eligibleHits = %v", got)
	}
	if got := eligibleRead("N345891", "[N345891 | 2025-03-11] x", late, nil); got != nil {
		t.Errorf("eligibleRead = %v", got)
	}
	if got := eligibleRead("N345891", "[N345891 | 2025-03-11] x", time.Date(2025, 5, 1, 0, 0, 0, 0, time.UTC), nil); len(got) != 1 {
		t.Errorf("eligibleRead before the revocation = %v", got)
	}
}
