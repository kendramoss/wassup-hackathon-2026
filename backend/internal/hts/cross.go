package hts

import (
	"context"
	"math"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/cross"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
)

// CROSS candidates (D92). The local index holds only the 815 rulings of the
// origin corpus that state a subheading, all from 2023 on, so most products
// have no classification ruling in it: the heat gun got filter and machine-
// tool codes, while CBP has five rulings classifying heat guns in 8419.89.
// CROSS search returns each ruling's tariff numbers with the hit, so a live
// search of the product's words finds the classification rulings directly.

// Live is the CROSS search the suggester uses (cross.Client.Search).
type Live interface {
	Search(ctx context.Context, term string, toDate time.Time, pageSize int) ([]cross.SearchHit, error)
}

// Current reports whether a subheading is in today's HTS. Rulings from
// earlier HTS editions cite numbers that have since been split or renumbered
// (8419.89.90 became 8419.89.95); those are dropped. nil keeps every number.
type Current func(ctx context.Context, sub string) bool

// crossPage is how many hits each CROSS query asks for; crossPool how many
// subheadings go to the model to rank (MaxCandidates are returned).
const (
	crossPage = 30
	crossPool = 6
)

// crossQueries lists the searches for a product, broad to narrow: the
// product's name words first (what CBP's subjects name), then the start of
// the text. Each is a few words because CROSS needs every word to match.
func crossQueries(text, name string) []string {
	var out []string
	add := func(words []string) {
		q := strings.Join(words, " ")
		if q == "" {
			return
		}
		for _, have := range out {
			if have == q {
				return
			}
		}
		out = append(out, q)
	}
	nameWords := keywords(name)
	if len(nameWords) > 3 {
		nameWords = nameWords[:3]
	}
	add(nameWords)
	if len(nameWords) >= 2 {
		add(nameWords[len(nameWords)-2:]) // "12.5-amp heat gun" → "heat gun"
	}
	tw := keywords(productPart(text))
	if len(tw) > 4 {
		tw = tw[:4]
	}
	add(tw)
	if len(tw) >= 2 {
		add(tw[:2])
	}
	return out
}

// productPart skips the request letter's opening ("In your letter dated…,
// you requested a … ruling.", "In lieu of samples…") to the sentence that
// names the goods, so the search uses the product's words.
func productPart(text string) string {
	sentences := strings.SplitAfter(text, ". ")
	for i, sn := range sentences {
		l := strings.ToLower(sn)
		if strings.Contains(l, "your letter") || strings.Contains(l, "you requested") || strings.Contains(l, "in lieu of") ||
			strings.Contains(l, "ruling request") || strings.Contains(l, "on behalf of") || strings.Contains(l, "samples were") ||
			strings.Contains(l, "a description of the item") || strings.Contains(l, "were provided") {
			continue
		}
		return strings.Join(sentences[i:], "")
	}
	return text
}

// keywords are a text's content words in order, lower case: no numbers,
// model codes, countries or filler.
func keywords(s string) []string {
	var out []string
	for _, w := range strings.Fields(strings.ToLower(s)) {
		w = strings.Trim(w, `"'(),.;:!?[]{}`)
		if len(w) < 3 || filler[w] || strings.ContainsAny(w, "0123456789") || domain.NormalizeCountry(w) != titleWord(w) {
			continue
		}
		out = append(out, w)
	}
	return out
}

// titleWord is how NormalizeCountry returns a word that is not a country.
func titleWord(w string) string {
	if w == "" {
		return w
	}
	return strings.ToUpper(w[:1]) + w[1:]
}

var filler = map[string]bool{"the": true, "and": true, "for": true, "with": true, "that": true, "this": true, "are": true, "its": true,
	"from": true, "into": true, "which": true, "under": true, "item": true, "items": true, "merchandise": true, "product": true,
	"consideration": true, "identified": true, "described": true, "model": true, "number": true, "your": true, "you": true,
	"letter": true, "dated": true, "requested": true, "ruling": true, "country": true, "origin": true, "tariff": true,
	"classification": true, "consists": true, "features": true, "used": true, "made": true, "imported": true, "kobalt": true,
	"main": true, "components": true, "sku": true, "subject": true, "constructed": true, "description": true, "following": true, "request": true, "include": true, "includes": true, "was": true, "were": true, "has": true, "have": true}

// crossCandidates searches CROSS and ranks the 8-digit subheadings its
// classification rulings state. Each ruling votes once for each of its
// subheadings, weighted by how early its query ran and its rank in that
// query, with a small bonus for newer rulings; the subheading with the most
// weight from the most rulings ranks first.
func crossCandidates(ctx context.Context, live Live, current Current, text, name string) ([]Candidate, error) {
	type vote struct {
		w    float64
		refs []Ref
		seen map[string]bool
	}
	votes := map[string]*vote{}
	var order []string
	var firstErr error
	answered := false
	for qi, q := range crossQueries(text, name) {
		hits, err := live.Search(ctx, q, time.Time{}, crossPage)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		answered = true
		for rank, h := range hits {
			if h.Tariffs == "" || !strings.Contains(strings.ToLower(h.Categories), "classification") {
				continue // origin and marking rulings state no subheading
			}
			w := 1 / (1 + float64(qi)) / (1 + 0.15*float64(rank))
			if d, err := h.Date(); err == nil {
				w *= 1 + 0.3*math.Max(0, 1-float64(time.Since(d).Hours())/(24*365*20)) // newer weighs a little more
			}
			for _, sub := range subheadings(h.Tariffs) {
				v := votes[sub]
				if v == nil {
					v = &vote{seen: map[string]bool{}}
					votes[sub] = v
					order = append(order, sub)
				}
				if v.seen[h.RulingNumber] {
					continue
				}
				v.seen[h.RulingNumber] = true
				v.w += w
				date := ""
				if d, err := h.Date(); err == nil {
					date = d.Format(time.DateOnly)
				}
				v.refs = append(v.refs, Ref{ID: h.RulingNumber, Date: date, Subject: strings.TrimSpace(h.Subject), URL: domain.RulingURL(h.RulingNumber)})
			}
		}
	}
	if !answered {
		return nil, firstErr
	}
	sort.SliceStable(order, func(i, j int) bool { return votes[order[i]].w > votes[order[j]].w })
	keep := currentOnly(ctx, current, order, crossPool)
	out := make([]Candidate, 0, len(keep))
	for _, sub := range keep {
		refs := votes[sub].refs
		sort.SliceStable(refs, func(i, j int) bool { return refs[i].Date > refs[j].Date })
		if len(refs) > 5 {
			refs = refs[:5]
		}
		out = append(out, Candidate{HTS: sub, Supporting: refs, Cited: []string{}})
	}
	return out, nil
}

// currentOnly keeps, in order, up to n subheadings that are in today's HTS,
// checking them in parallel.
func currentOnly(ctx context.Context, current Current, order []string, n int) []string {
	if current == nil {
		if len(order) > n {
			order = order[:n]
		}
		return order
	}
	check := order
	if len(check) > 3*n {
		check = check[:3*n]
	}
	ok := make([]bool, len(check))
	var wg sync.WaitGroup
	for i, sub := range check {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ok[i] = current(ctx, sub)
		}()
	}
	wg.Wait()
	var out []string
	for i, sub := range check {
		if ok[i] && len(out) < n {
			out = append(out, sub)
		}
	}
	return out
}

// subheadings are the distinct 8-digit subheadings in a CROSS tariffs field
// ("8516.80.8000, 8516.80.8000" → ["8516.80.80"]); chapter 98 and 99 numbers
// (special provisions and additional duties) are not classifications.
func subheadings(tariffs string) []string {
	var out []string
	for _, t := range strings.FieldsFunc(tariffs, func(r rune) bool { return r == ',' || r == ';' || r == ' ' }) {
		var d strings.Builder
		for _, r := range t {
			if r >= '0' && r <= '9' {
				d.WriteRune(r)
			}
		}
		s := d.String()
		if len(s) < 8 || strings.HasPrefix(s, "98") || strings.HasPrefix(s, "99") {
			continue
		}
		sub := s[:4] + "." + s[4:6] + "." + s[6:8]
		dup := false
		for _, o := range out {
			dup = dup || o == sub
		}
		if !dup {
			out = append(out, sub)
		}
	}
	return out
}
