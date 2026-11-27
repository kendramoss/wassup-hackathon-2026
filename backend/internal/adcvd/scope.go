package adcvd

import (
	"math"
	"sort"
	"strings"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
)

// Query is a product to find the orders whose written scope may cover it: a
// description in an importer's words and the country of origin. The HTS code
// is an optional hint; it never removes an order.
type Query struct {
	Description string
	Country     string
	HTS         string
}

// Candidate is an order group (an AD order and its CVD companion are one) the
// retrieval proposes, with the written scope a reader is given.
type Candidate struct {
	Cases   []string
	Types   []string
	Country string
	Product string
	// Scope is the longest written scope among the group's orders.
	Scope string
	Score float64
}

type group struct {
	orders []int
	title  []string // terms of the product name
	body   []string // terms of the name and the written scope
	hts    []string // listed codes, digits only
}

func termList(s string) []string {
	m := Terms(s)
	out := make([]string, 0, len(m))
	for w := range m {
		out = append(out, w)
	}
	sort.Strings(out)
	return out
}

func (t *Table) buildGroups() {
	t.groupsOnce.Do(func() {
		type key struct{ country, product string }
		idx := map[key]int{}
		for i, o := range t.orders {
			k := key{t.origin[i], strings.ToLower(strings.TrimSpace(o.Product))}
			g, ok := idx[k]
			if !ok {
				g = len(t.groups)
				idx[k] = g
				t.groups = append(t.groups, group{})
			}
			t.groups[g].orders = append(t.groups[g].orders, i)
		}
		for gi := range t.groups {
			g := &t.groups[gi]
			rep := t.representative(g.orders)
			g.title = termList(rep.Product)
			g.body = termList(rep.Product + " " + rep.ScopeText)
			seen := map[string]bool{}
			for _, i := range g.orders {
				for _, h := range t.orders[i].HTS {
					if d := digits(h); d != "" && !seen[d] {
						seen[d] = true
						g.hts = append(g.hts, d)
					}
				}
			}
		}
	})
}

// termMatch is a term of the description and a term of an order's words: the
// same word, or one the start of the other at four letters or more (wood,
// wooden).
func termMatch(d, w string) bool {
	if d == w {
		return true
	}
	if len(d) < 4 || len(w) < 4 {
		return false
	}
	return strings.HasPrefix(w, d) || strings.HasPrefix(d, w)
}

func hit(d string, words []string) bool {
	for _, w := range words {
		if termMatch(d, w) {
			return true
		}
	}
	return false
}

// Candidates returns up to k order groups of the query's country, best first:
// the description's product terms, weighted by how rare each is among all
// orders, found in an order's name (counted three times) and written scope. A
// listed HTS code that agrees with the hint at heading level adds a small
// boost, at subheading level a larger one. Orders that print no scope are
// ranked by name and code alone. No order is dropped for its code.
func (t *Table) Candidates(q Query, k int) []Candidate {
	if k <= 0 || strings.TrimSpace(q.Country) == "" {
		return nil
	}
	t.buildGroups()
	want := originCanon(q.Country)
	parts := domain.CountryParts(want)
	terms := termList(q.Description)
	hint := digits(q.HTS)
	n := float64(len(t.groups))
	idf := make([]float64, len(terms))
	for i, w := range terms {
		df := 0
		for gi := range t.groups {
			if hit(w, t.groups[gi].body) {
				df++
			}
		}
		idf[i] = math.Log(1 + (n-float64(df)+0.5)/(float64(df)+0.5))
	}
	type scored struct {
		g int
		s float64
	}
	var all []scored
	for gi := range t.groups {
		g := &t.groups[gi]
		if !countryIn(t.origin[g.orders[0]], parts) {
			continue
		}
		s := 0.0
		for i, w := range terms {
			if hit(w, g.title) {
				s += 3 * idf[i]
			} else if hit(w, g.body) {
				s += idf[i]
			}
		}
		if hint != "" {
			best := 0.0
			for _, h := range g.hts {
				switch m := commonLen(hint, h); {
				case m >= 6:
					best = math.Max(best, 1.5)
				case m >= 4:
					best = math.Max(best, 0.75)
				}
			}
			s += best
		}
		if s > 0 {
			all = append(all, scored{gi, s})
		}
	}
	sort.SliceStable(all, func(i, j int) bool {
		if all[i].s != all[j].s {
			return all[i].s > all[j].s
		}
		return t.orders[t.groups[all[i].g].orders[0]].Case < t.orders[t.groups[all[j].g].orders[0]].Case
	})
	if len(all) > k {
		all = all[:k]
	}
	out := make([]Candidate, 0, len(all))
	for _, sc := range all {
		out = append(out, t.candidate(t.groups[sc.g], sc.s))
	}
	return out
}

// HasCountry reports whether some order in the table is on the country (one
// of several written "A/B" counts). It is for choosing which products the
// check applies to: those made in a country with an order.
func (t *Table) HasCountry(country string) bool {
	if strings.TrimSpace(country) == "" {
		return false
	}
	parts := domain.CountryParts(originCanon(country))
	for _, o := range t.origin {
		if countryIn(o, parts) {
			return true
		}
	}
	return false
}

// commonLen is how many leading digits two codes share, up to the shorter.
func commonLen(a, b string) int {
	n := 0
	for n < len(a) && n < len(b) && a[n] == b[n] {
		n++
	}
	return n
}

func countryIn(orderCountry string, parts []string) bool {
	for _, p := range parts {
		if p != "" && p == orderCountry {
			return true
		}
	}
	return false
}

// GroupOf returns the order group that holds one of the case numbers, as a
// candidate with no score. It is for evaluation: reading an order whatever
// its rank in the retrieval.
func (t *Table) GroupOf(cases []string) (Candidate, bool) {
	t.buildGroups()
	for _, g := range t.groups {
		for _, i := range g.orders {
			for _, c := range cases {
				if t.orders[i].Case == c {
					return t.candidate(g, 0), true
				}
			}
		}
	}
	return Candidate{}, false
}

func (t *Table) candidate(g group, score float64) Candidate {
	rep := t.representative(g.orders)
	c := Candidate{Country: rep.Country, Product: rep.Product, Scope: rep.ScopeText, Score: math.Round(score*1000) / 1000}
	for _, i := range g.orders {
		if !contains(c.Cases, t.orders[i].Case) {
			c.Cases = append(c.Cases, t.orders[i].Case)
		}
		if !contains(c.Types, t.orders[i].Type) {
			c.Types = append(c.Types, t.orders[i].Type)
		}
	}
	sort.Strings(c.Cases)
	sort.Strings(c.Types)
	return c
}
