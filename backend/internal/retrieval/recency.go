package retrieval

import (
	"context"
	"math"
	"sort"
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
)

// RecencyVersion names the recency re-rank (D904). Eval-only: the server
// does not use it.
const RecencyVersion = "recency-v1"

// RecencyBoost and RecencyScale fix the re-rank: a precedent's BM25 score is
// multiplied by 1 + RecencyBoost*exp(-months/RecencyScale), months being how
// much older it is than the case. A same-month ruling gains 10%, a year-old
// one 3.7%, a two-year-old one 1.4%: it only reorders near ties.
const (
	RecencyBoost = 0.10
	RecencyScale = 12.0
	// RecencyPool is how many BM25 candidates are re-ranked.
	RecencyPool = 20
)

type caseDateKey struct{}

// WithCaseDate carries the case date to a Recency retriever.
func WithCaseDate(ctx context.Context, d time.Time) context.Context {
	return context.WithValue(ctx, caseDateKey{}, d)
}

// Recency re-ranks BM25's top RecencyPool candidates by recency (D904).
// Without a case date in the context it is plain BM25.
type Recency struct{ BM25 *BM25 }

// Search implements Retriever.
func (r *Recency) Search(ctx context.Context, query string, k int, allow func(domain.Ruling) bool) ([]Hit, error) {
	d, ok := ctx.Value(caseDateKey{}).(time.Time)
	if !ok || d.IsZero() {
		return r.BM25.Top(query, k, allow), nil
	}
	return RecencyRerank(r.BM25.Top(query, max(k, RecencyPool), allow), d, k), nil
}

// RecencyRerank re-scores hits by recency relative to caseDate and keeps k.
// Ties keep BM25's order.
func RecencyRerank(hits []Hit, caseDate time.Time, k int) []Hit {
	out := make([]Hit, len(hits))
	for i, h := range hits {
		m := caseDate.Sub(h.Ruling.Date).Hours() / 24 / 30.4375
		if m < 0 {
			m = 0
		}
		out[i] = Hit{Ruling: h.Ruling, Score: h.Score * (1 + RecencyBoost*math.Exp(-m/RecencyScale))}
	}
	sort.SliceStable(out, func(a, b int) bool { return out[a].Score > out[b].Score })
	if len(out) > k {
		out = out[:k]
	}
	return out
}

var _ Retriever = (*Recency)(nil)
