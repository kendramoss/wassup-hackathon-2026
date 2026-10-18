package verifier

import (
	"context"
	"strings"
	"time"
)

// Relaxed makes CROSS search usable for an agent's queries (D89). CROSS
// returns only rulings containing every word of the query, and the agent
// writes long queries ("electric motor stator rotor laminations SMT PCBA
// Thailand China"): on the heat gun 26 of 41 searches returned nothing, so
// checks ran with no evidence to read. When the full query returns fewer
// than SearchHits, Relaxed retries shorter forms of it and then fills the
// remaining slots from Fill (the local labelled corpus, ranked by BM25,
// which matches partially). Hits keep the order they were found in: the
// full query's first. Read goes to Primary, so the agent still reads CBP's
// full ruling text.
type Relaxed struct {
	Primary Searcher
	// Fill is optional; nil uses CROSS only.
	Fill Searcher
}

// Search implements Searcher.
func (r Relaxed) Search(ctx context.Context, query string, before time.Time, exclude map[string]bool) ([]Hit, error) {
	var out []Hit
	seen := map[string]bool{}
	add := func(hits []Hit) {
		for _, h := range hits {
			if len(out) == SearchHits || seen[h.ID] {
				continue
			}
			seen[h.ID] = true
			out = append(out, h)
		}
	}
	var firstErr error
	for i, q := range relaxations(query) {
		if len(out) >= SearchHits {
			break
		}
		hits, err := r.Primary.Search(ctx, q, before, exclude)
		if err != nil {
			if i == 0 {
				firstErr = err
			}
			continue
		}
		add(hits)
	}
	if len(out) < SearchHits && r.Fill != nil {
		if hits, err := r.Fill.Search(ctx, query, before, exclude); err == nil {
			add(hits)
		}
	}
	if len(out) == 0 && firstErr != nil {
		return nil, firstErr // CROSS down and nothing to fill with
	}
	return out, nil
}

// Read implements Searcher.
func (r Relaxed) Read(ctx context.Context, id string, before time.Time, exclude map[string]bool) (string, error) {
	return r.Primary.Read(ctx, id, before, exclude)
}

// maxRelaxations bounds the extra CROSS calls per search.
const maxRelaxations = 4

// relaxations lists the query and up to maxRelaxations shorter forms, each
// different from those before it: without country names and search
// operators, its first and second halves, then its first two words.
func relaxations(query string) []string {
	out := []string{strings.TrimSpace(query)}
	add := func(words []string) {
		q := strings.Join(words, " ")
		if q == "" || len(out) > maxRelaxations {
			return
		}
		for _, have := range out {
			if have == q {
				return
			}
		}
		out = append(out, q)
	}
	var core []string
	for _, w := range strings.Fields(query) {
		w = strings.Trim(w, `"'(),.;:`)
		if w == "" || stopWord[strings.ToLower(w)] || isCountryWord(w) {
			continue
		}
		core = append(core, w)
	}
	add(core)
	if len(core) >= 4 {
		half := (len(core) + 1) / 2
		add(core[:half])
		add(core[half:])
	}
	if len(core) >= 3 {
		add(core[:2])
	}
	return out
}

// stopWord holds search operators and filler the agent puts in queries;
// CROSS treats them as words a ruling must contain.
var stopWord = map[string]bool{"or": true, "and": true, "not": true, "the": true, "of": true, "in": true, "for": true,
	"a": true, "an": true, "to": true, "with": true, "origin": true, "country": true, "made": true, "ruling": true, "rulings": true, "cbp": true}

// isCountryWord reports whether w names a country (the origin words a ruling
// may not repeat in its subject or text).
func isCountryWord(w string) bool {
	switch strings.ToLower(w) {
	case "china", "prc", "chinese", "thailand", "thai", "vietnam", "viet", "nam", "malaysia", "malaysian", "mexico", "mexican",
		"taiwan", "cambodia", "indonesia", "india", "japan", "korea", "philippines", "usa", "us", "united", "states":
		return true
	}
	return false
}
