package retrieval

import (
	"context"
	"math"
	"regexp"
	"sort"
	"strings"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
)

// Retriever returns up to k rulings most similar to query among those for
// which allow returns true, best first.
type Retriever interface {
	Search(ctx context.Context, query string, k int, allow func(domain.Ruling) bool) ([]Hit, error)
}

// Hit is one retrieved ruling and its score.
type Hit struct {
	Ruling domain.Ruling
	Score  float64
}

// BM25 is Okapi BM25 over the rulings' facts, ported from the BM25 class in
// hackathon-research/experiments/origin-proof/predict.py so retrieved
// precedents match the research runs:
//   - tokens are runs of 3+ ASCII letters, lower-cased, minus a stop list;
//   - document text is the first 3,000 characters of the facts;
//   - query terms are de-duplicated;
//   - ties are broken by higher corpus index first (Python's reverse sort of
//     (score, index) tuples).
//
// IDF is computed over the whole corpus, including rulings later than a
// given case. That exposes only term frequencies, never later rulings' text
// or holdings, and is kept for reproducibility (docs/DECISIONS.md).
type BM25 struct {
	docs []domain.Ruling
	tf   []map[string]int
	len  []int
	avg  float64
	idf  map[string]float64
	k1   float64
	b    float64
}

var tokenPattern = regexp.MustCompile(`[a-z]{3,}`)

var stopWords = func() map[string]bool {
	m := map[string]bool{}
	for _, w := range strings.Fields("the and for with that this are was were from which into has have its been will their there such other these those than then also") {
		m[w] = true
	}
	return m
}()

// Tokens splits text into BM25 terms.
func Tokens(s string) []string {
	var out []string
	for _, t := range tokenPattern.FindAllString(strings.ToLower(s), -1) {
		if !stopWords[t] {
			out = append(out, t)
		}
	}
	return out
}

// DocRunes is how much of each ruling's facts is indexed (predict.py: f[:3000]).
const DocRunes = 3000

// NewBM25 indexes rulings with k1=1.5, b=0.75. text extracts the indexed
// text of a ruling.
func NewBM25(rulings []domain.Ruling, text func(domain.Ruling) string) *BM25 {
	idx := &BM25{docs: rulings, k1: 1.5, b: 0.75, idf: map[string]float64{}}
	df := map[string]int{}
	total := 0
	for _, r := range rulings {
		tf := map[string]int{}
		n := 0
		for _, t := range Tokens(text(r)) {
			tf[t]++
			n++
		}
		idx.tf = append(idx.tf, tf)
		idx.len = append(idx.len, n)
		total += n
		for t := range tf {
			df[t]++
		}
	}
	idx.avg = float64(total) / math.Max(1, float64(len(rulings)))
	n := float64(len(rulings))
	for w, f := range df {
		idx.idf[w] = math.Log(1 + (n-float64(f)+0.5)/(float64(f)+0.5))
	}
	return idx
}

// Search implements Retriever.
func (idx *BM25) Search(_ context.Context, query string, k int, allow func(domain.Ruling) bool) ([]Hit, error) {
	return idx.Top(query, k, allow), nil
}

// Top returns the k best BM25 hits (no error path; used by Hybrid).
func (idx *BM25) Top(query string, k int, allow func(domain.Ruling) bool) []Hit {
	seen := map[string]bool{}
	var terms []string
	for _, t := range Tokens(query) {
		if !seen[t] {
			seen[t] = true
			terms = append(terms, t)
		}
	}
	type scored struct {
		score float64
		i     int
	}
	var all []scored
	for i, r := range idx.docs {
		if !allow(r) {
			continue
		}
		tf, L := idx.tf[i], float64(idx.len[i])
		s := 0.0
		for _, w := range terms {
			if f, ok := tf[w]; ok {
				ff := float64(f)
				s += idx.idf[w] * ff * (idx.k1 + 1) / (ff + idx.k1*(1-idx.b+idx.b*L/idx.avg))
			}
		}
		all = append(all, scored{s, i})
	}
	sort.Slice(all, func(a, b int) bool {
		if all[a].score != all[b].score {
			return all[a].score > all[b].score
		}
		return all[a].i > all[b].i
	})
	if len(all) > k {
		all = all[:k]
	}
	hits := make([]Hit, len(all))
	for j, s := range all {
		hits[j] = Hit{Ruling: idx.docs[s.i], Score: s.score}
	}
	return hits
}

// PrecedentFilter allows only rulings strictly earlier than the case, never
// the case itself and never a ruling the case excludes.
func PrecedentFilter(c domain.Case) func(domain.Ruling) bool {
	excluded := map[string]bool{}
	for _, id := range c.Exclude {
		excluded[id] = true
	}
	return func(r domain.Ruling) bool { return r.UsableAsPrecedentFor(c.ID, c.Date) && !excluded[r.ID] }
}

var _ Retriever = (*BM25)(nil)
