package httpapi

import (
	"math"
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/evalkit"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/router"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/store"
)

// usage sums the model calls behind one history event. Cost uses the same
// promo prices as the eval (evalkit.PromoPrices), so per-case figures match
// eval/RESULTS.md; an unpriced model adds tokens but no cost.
type usage struct {
	in, out int
	cost    float64
}

func (u *usage) add(model string, in, out int) {
	u.in += in
	u.out += out
	u.cost += evalkit.Cost(evalkit.PromoPrices, model, in, out)
}

func (u *usage) trace(t domain.Trace) { u.add(t.ModelID, t.InputTokens, t.OutputTokens) }

func (u *usage) label(l *router.Label) {
	if l != nil {
		u.add(l.ModelID, l.InputTokens, l.OutputTokens)
	}
}

// on copies the usage onto e. A zero start leaves the latency as set.
func (u usage) on(e store.Event, start time.Time) store.Event {
	e.InputTokens, e.OutputTokens = u.in, u.out
	e.CostUSD = math.Round(u.cost*1e6) / 1e6
	if !start.IsZero() {
		e.LatencyMS = time.Since(start).Milliseconds()
	}
	return e
}
