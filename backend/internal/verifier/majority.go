package verifier

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
)

// Majority runs the inner checker n times in parallel and keeps the majority
// answer on precedent support, so one unlucky agent path cannot flip a plan
// (D33). Its confidence is the median of the runs in the majority, and the
// cited rulings come from that median run. A tie is decided against support.
// Runs that fail are ignored while at least one succeeds.
type Majority struct {
	inner Checker
	n     int
}

// NewMajority wraps inner; n < 2 returns inner unchanged.
func NewMajority(inner Checker, n int) Checker {
	if n < 2 {
		return inner
	}
	return &Majority{inner: inner, n: n}
}

// Verify runs the task n times and returns the majority verdict.
func (m *Majority) Verify(ctx context.Context, t Task) (Result, error) {
	results := make([]Result, m.n)
	errs := make([]error, m.n)
	var wg sync.WaitGroup
	for i := range m.n {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			results[i], errs[i] = m.inner.Verify(ctx, t)
		}(i)
	}
	wg.Wait()
	var ok, yes, no []Result
	var total Result
	for i, r := range results {
		total.ModelCalls += r.ModelCalls
		total.Steps = append(total.Steps, r.Steps...)
		total.Usage.InputTokens += r.Usage.InputTokens
		total.Usage.OutputTokens += r.Usage.OutputTokens
		total.Usage.TotalTokens += r.Usage.TotalTokens
		if errs[i] != nil {
			continue
		}
		ok = append(ok, r)
		if r.Verdict.PrecedentSupported {
			yes = append(yes, r)
		} else {
			no = append(no, r)
		}
	}
	if len(ok) == 0 {
		return total, fmt.Errorf("verifier: all %d runs failed: %w", m.n, errors.Join(errs...))
	}
	group := no
	if len(yes) > len(no) {
		group = yes
	}
	sort.SliceStable(group, func(i, j int) bool { return group[i].Verdict.Confidence < group[j].Verdict.Confidence })
	best := group[(len(group)-1)/2] // median; the lower one of an even pair
	best.ModelCalls, best.Steps, best.Usage = total.ModelCalls, total.Steps, total.Usage
	best.Verdict.Agreement = fmt.Sprintf("%d/%d", len(group), len(ok))
	return best, nil
}
