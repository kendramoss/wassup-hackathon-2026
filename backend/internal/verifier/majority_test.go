package verifier

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
)

type scriptedChecker struct {
	mu   sync.Mutex
	next int
	runs []Result
	errs []error
}

func (s *scriptedChecker) Verify(context.Context, Task) (Result, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	i := s.next
	s.next++
	var err error
	if i < len(s.errs) {
		err = s.errs[i]
	}
	return s.runs[i], err
}

func run(supported bool, conf float64, ruling string) Result {
	return Result{Verdict: domain.Verdict{PrecedentSupported: supported, Confidence: conf, SupportingRuling: ruling}, ModelCalls: 4, Steps: make([]Step, 2)}
}

func TestMajorityKeepsMedianOfMajority(t *testing.T) {
	s := &scriptedChecker{runs: []Result{run(true, 0.97, "N1"), run(false, 0.6, ""), run(true, 0.95, "N2")}}
	r, err := NewMajority(s, 3).Verify(context.Background(), Task{})
	if err != nil {
		t.Fatal(err)
	}
	// Majority supported (2 of 3); median of {0.95, 0.97} as the lower = 0.95.
	if !r.Verdict.PrecedentSupported || r.Verdict.Confidence != 0.95 || r.Verdict.SupportingRuling != "N2" || r.Verdict.Agreement != "2/3" ||
		r.ModelCalls != 12 || len(r.Steps) != 6 {
		t.Errorf("majority = %+v (calls %d, steps %d)", r.Verdict, r.ModelCalls, len(r.Steps))
	}
}

func TestMajorityTieGoesAgainstSupportAndIgnoresFailures(t *testing.T) {
	s := &scriptedChecker{runs: []Result{run(true, 0.97, "N1"), run(false, 0.7, ""), {}}, errs: []error{nil, nil, errors.New("boom")}}
	r, err := NewMajority(s, 3).Verify(context.Background(), Task{})
	if err != nil || r.Verdict.PrecedentSupported || r.Verdict.Agreement != "1/2" {
		t.Errorf("tie = %+v, %v", r.Verdict, err)
	}
	all := &scriptedChecker{runs: make([]Result, 3), errs: []error{errors.New("a"), errors.New("b"), errors.New("c")}}
	if _, err := NewMajority(all, 3).Verify(context.Background(), Task{}); err == nil {
		t.Error("expected an error when every run fails")
	}
	if NewMajority(s, 1) != Checker(s) {
		t.Error("n < 2 should return the inner checker")
	}
}
