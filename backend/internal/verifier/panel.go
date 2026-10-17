package verifier

import (
	"context"
	"fmt"
	"sort"
	"sync"
)

// Panel gives checks that land near the plan bar two second opinions from
// different views (D83). A check outside [Low, High) is returned as it is.
// Inside, the views run in parallel and the panel keeps the majority on
// precedent support (a tie goes against support), the median confidence of
// that majority, and the citations of the run with that confidence. A
// counter-ruling found by any view is kept, so a skeptic's find is never
// lost. With the seed (D76) a repeat of the same prompt mostly agrees with
// itself; different views search differently, so they add evidence.
type Panel struct {
	first     Checker
	views     []Checker
	low, high float64
}

// NewPanel wraps first with views that run when its confidence is in
// [low, high).
func NewPanel(first Checker, views []Checker, low, high float64) *Panel {
	return &Panel{first: first, views: views, low: low, high: high}
}

// Verify runs the first check and, near the bar, the views.
func (p *Panel) Verify(ctx context.Context, t Task) (Result, error) {
	r, err := p.first.Verify(ctx, t)
	if err != nil || len(p.views) == 0 {
		return r, err
	}
	c := r.Verdict.Confidence
	if c < p.low || c >= p.high {
		return r, nil
	}
	results := make([]Result, len(p.views))
	errs := make([]error, len(p.views))
	var wg sync.WaitGroup
	for i, v := range p.views {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results[i], errs[i] = v.Verify(ctx, t)
		}()
	}
	wg.Wait()
	runs := []Result{r}
	total := r
	for i, x := range results {
		total.ModelCalls += x.ModelCalls
		total.Steps = append(total.Steps, x.Steps...)
		total.Usage.InputTokens += x.Usage.InputTokens
		total.Usage.OutputTokens += x.Usage.OutputTokens
		total.Usage.TotalTokens += x.Usage.TotalTokens
		total.ToolErrors += x.ToolErrors
		if errs[i] == nil {
			runs = append(runs, x)
		}
	}
	var yes, no []Result
	counter := ""
	for _, x := range runs {
		if x.Verdict.PrecedentSupported {
			yes = append(yes, x)
		} else {
			no = append(no, x)
		}
		if counter == "" && x.Verdict.CounterRuling != "" {
			counter = x.Verdict.CounterRuling
		}
	}
	group := no
	if len(yes) > len(no) {
		group = yes
	}
	sort.SliceStable(group, func(i, j int) bool { return group[i].Verdict.Confidence < group[j].Verdict.Confidence })
	best := group[(len(group)-1)/2] // median; the lower one of an even pair
	if best.Verdict.CounterRuling == "" {
		best.Verdict.CounterRuling = counter
	}
	best.ModelCalls, best.Steps, best.Usage, best.ToolErrors = total.ModelCalls, total.Steps, total.Usage, total.ToolErrors
	best.Verdict.Agreement = fmt.Sprintf("%d/%d", len(group), len(runs))
	return best, nil
}
