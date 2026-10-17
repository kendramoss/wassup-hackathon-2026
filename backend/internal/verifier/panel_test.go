package verifier

import (
	"context"
	"errors"
	"testing"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
)

type stub struct {
	r     Result
	err   error
	calls int
}

func (s *stub) Verify(context.Context, Task) (Result, error) { s.calls++; return s.r, s.err }

func verdict(supported bool, conf float64, support, counter string) Result {
	return Result{ModelCalls: 2, Verdict: domain.Verdict{PrecedentSupported: supported, Confidence: conf, SupportingRuling: support, CounterRuling: counter}}
}

func TestPanelRunsOnlyNearTheBar(t *testing.T) {
	for _, c := range []float64{0.80, 0.95, 0.98} {
		first, a, b := &stub{r: verdict(true, c, "N1", "")}, &stub{}, &stub{}
		r, err := NewPanel(first, []Checker{a, b}, 0.89, 0.95).Verify(context.Background(), Task{})
		if err != nil || a.calls+b.calls != 0 || r.Verdict.Confidence != c || r.Verdict.Agreement != "" {
			t.Errorf("conf %v: views ran %d times, %+v %v", c, a.calls+b.calls, r.Verdict, err)
		}
	}
}

func TestPanelMajorityMedianAndCounter(t *testing.T) {
	first := &stub{r: verdict(true, 0.90, "N1", "")}
	skeptic := &stub{r: verdict(true, 0.94, "N2", "H9")}
	product := &stub{r: verdict(true, 0.96, "N3", "")}
	r, err := NewPanel(first, []Checker{skeptic, product}, 0.89, 0.95).Verify(context.Background(), Task{})
	if err != nil || r.Verdict.Confidence != 0.94 || r.Verdict.SupportingRuling != "N2" || r.Verdict.CounterRuling != "H9" || r.Verdict.Agreement != "3/3" || r.ModelCalls != 6 {
		t.Errorf("3 supported: %+v calls %d %v", r.Verdict, r.ModelCalls, err)
	}
	product.r = verdict(false, 0.85, "", "H8")
	skeptic.r = verdict(false, 0.80, "", "")
	r, _ = NewPanel(first, []Checker{skeptic, product}, 0.89, 0.95).Verify(context.Background(), Task{})
	if r.Verdict.PrecedentSupported || r.Verdict.Agreement != "2/3" || r.Verdict.CounterRuling != "H8" {
		t.Errorf("2 against: %+v", r.Verdict)
	}
}

func TestPanelTieGoesAgainstSupportAndFailedViewsAreSkipped(t *testing.T) {
	first := &stub{r: verdict(true, 0.92, "N1", "")}
	r, _ := NewPanel(first, []Checker{&stub{r: verdict(false, 0.70, "", "")}, &stub{err: errors.New("504")}}, 0.89, 0.95).Verify(context.Background(), Task{})
	if r.Verdict.PrecedentSupported || r.Verdict.Agreement != "1/2" {
		t.Errorf("1-1 tie: %+v", r.Verdict)
	}
}

func TestPanelKeepsTheCacheKey(t *testing.T) {
	v := New(nil, nil, Options{Prompt: "v2"})
	p := NewPanel(v, []Checker{New(nil, nil, Options{Prompt: "v2", View: ViewSkeptic})}, 0.89, 0.95)
	if checkerVersion(p) != PromptVersion2 {
		t.Errorf("panel cache version = %q, want %q so stored verdicts are kept", checkerVersion(p), PromptVersion2)
	}
	if got := New(nil, nil, Options{Prompt: "v2", View: ViewSkeptic}).Version(); got != "verifier-v2-skeptic" {
		t.Errorf("view version = %q", got)
	}
	if SystemView(6, ViewSkeptic) == System2(6) || SystemView(6, ViewProduct) == System2(6) {
		t.Error("views must change the instruction")
	}
}
