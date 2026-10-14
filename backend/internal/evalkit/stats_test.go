package evalkit

import (
	"math"
	"strings"
	"testing"
)

func near(a, b float64) bool { return math.Abs(a-b) < 5e-4 }

func TestWilsonMatchesResearch(t *testing.T) {
	// RESULTS-v2: 89.1% (85.6–91.9) on 377 = 336 correct; 97.8% (95.3–99.0) on 273 = 267.
	for _, tt := range []struct {
		k, n      int
		p, lo, hi float64
	}{{336, 377, 0.8912, 0.8560, 0.9187}, {267, 273, 0.9780, 0.9529, 0.9899}, {32, 32, 1, 0.8928, 1}} {
		p, lo, hi := Wilson(tt.k, tt.n)
		if !near(p, tt.p) || !near(lo, tt.lo) || !near(hi, tt.hi) {
			t.Errorf("Wilson(%d,%d) = %.4f %.4f %.4f, want %.4f %.4f %.4f", tt.k, tt.n, p, lo, hi, tt.p, tt.lo, tt.hi)
		}
	}
	if p, lo, hi := Wilson(0, 0); p != 0 || lo != 0 || hi != 0 {
		t.Error("Wilson(0,0) should be zeros")
	}
}

func TestPercentile(t *testing.T) {
	v := []int64{5, 1, 4, 2, 3, 10, 9, 8, 7, 6}
	if Percentile(v, 50) != 5 || Percentile(v, 95) != 10 || Percentile(v, 0) != 1 || Percentile(nil, 50) != 0 {
		t.Errorf("percentiles wrong: %d %d", Percentile(v, 50), Percentile(v, 95))
	}
}

func TestCost(t *testing.T) {
	if c := Cost(PromoPrices, "gemini-4-argon", 1_000_000, 100_000); !near(c, 3.0) {
		t.Errorf("promo cost = %v", c)
	}
	if c := Cost(ListPrices, "gemini-4-argon-001", 1_000_000, 100_000); !near(c, 6.0) {
		t.Errorf("list cost for versioned id = %v", c)
	}
	if Cost(PromoPrices, "unknown", 10, 10) != 0 {
		t.Error("unknown model should cost 0")
	}
}

func TestSummarize(t *testing.T) {
	rows := []Row{
		{Correct: true, Confidence: 0.97, TriageRoute: "auto", LabelMethod: "regex", Provider: "litellm", ModelID: "gemini-4-argon", PromptVersion: "engine-v1", LatencyMS: 1000, CostUSD: 0.01},
		{Correct: false, Confidence: 0.96, TriageRoute: "auto", LabelMethod: "regex", Provider: "litellm", ModelID: "gemini-4-argon", PromptVersion: "engine-v1", LatencyMS: 3000, CostUSD: 0.01},
		{Correct: true, Confidence: 0.85, TriageRoute: "needs_review", LabelMethod: "manual", Provider: "litellm", ModelID: "gemini-4-argon", PromptVersion: "engine-v1", LatencyMS: 2000, CostUSD: 0.01},
		{Correct: true, Error: "timeout", LabelMethod: "regex"},
	}
	s := Summarize(rows)
	if s.N != 4 || s.Correct != 2 || s.Errors != 1 {
		t.Fatalf("n=%d correct=%d errors=%d", s.N, s.Correct, s.Errors)
	}
	if s.FastN != 2 || s.FastK != 1 || s.AtLeast95.N != 2 || s.Bins[2].N != 1 || s.Bins[2].K != 1 {
		t.Errorf("bins/fast path wrong: %+v", s)
	}
	if s.LatencyP50 != 2000 || !near(s.CostPerCase, 0.01) {
		t.Errorf("latency/cost wrong: p50=%d cost=%v", s.LatencyP50, s.CostPerCase)
	}
	md := Markdown("Dev run", "dev", s)
	for _, want := range []string{"Proxy, development only", "2 / 4 = 50.0%", "1 failed calls", "manual-labelled: 1 / 1", "Fast path"} {
		if !strings.Contains(md, want) {
			t.Errorf("markdown missing %q:\n%s", want, md)
		}
	}
}
