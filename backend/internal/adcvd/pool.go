package adcvd

import "strings"

// PoolEntry is one order group (an AD order with its CVD companion) of a
// product's pool.
type PoolEntry struct {
	Candidate
	// Rank is the 1-based rank in the retrieval, 0 when only the plain
	// HTS-and-country check proposed the group.
	Rank int
	// HTSMatch is true when the plain check matches the group.
	HTSMatch bool
}

// Pool returns the order groups put to one product (D945): the top k of the
// retrieval, then every group the plain HTS-and-country check matches. The
// evaluation labelled this pool and the scope-first check reads it, so a
// flag outside it was never measured.
func (t *Table) Pool(q Query, k int) []PoolEntry {
	seen := map[string]int{}
	var out []PoolEntry
	add := func(c Candidate, rank int, htsMatch bool) {
		id := strings.Join(c.Cases, " ")
		if i, ok := seen[id]; ok {
			if htsMatch {
				out[i].HTSMatch = true
			}
			return
		}
		seen[id] = len(out)
		out = append(out, PoolEntry{Candidate: c, Rank: rank, HTSMatch: htsMatch})
	}
	for i, c := range t.Candidates(q, k) {
		add(c, i+1, false)
	}
	res := t.Check(Input{Parts: []Part{{HTS: q.HTS, Origin: q.Country}}})
	if len(res.Parts) == 1 {
		for _, m := range res.Parts[0].Matches {
			if g, ok := t.GroupOf(m.Cases); ok {
				add(g, 0, true)
			}
		}
	}
	return out
}

// orderOf returns the first order of a case number.
func (t *Table) orderOf(c string) (Order, bool) {
	for _, o := range t.orders {
		if o.Case == c {
			return o, true
		}
	}
	return Order{}, false
}
