package adcvd

import (
	"fmt"
	"sort"
	"strings"
	"sync"
)

// Part statuses.
const (
	// StatusPossible: the part's HTS code and origin match an active order.
	StatusPossible = "possible_exposure"
	// StatusNone: no active order lists the part's code for its origin.
	StatusNone = "no_match"
	// StatusNotChecked: the part gave no usable HTS code or origin.
	StatusNotChecked = "not_checked"
)

// MaxParts caps one request.
const MaxParts = 200

// Notice is shown with every result.
const Notice = "Screening only. The HTS numbers in an order's scope are given for convenience and customs purposes; the written scope of the order governs, and only the U.S. Department of Commerce decides whether a product is within it. This does not change the origin answer or any duty figure."

// Part is one part to screen.
type Part struct {
	Name   string `json:"name,omitempty"`
	HTS    string `json:"hts,omitempty"`
	Origin string `json:"origin,omitempty"`
}

// Input is the parts to screen.
type Input struct {
	Parts []Part `json:"parts"`
}

// Match is one order (with its companion AD or CVD order) that lists the
// part's code for the part's origin.
type Match struct {
	Cases     []string `json:"cases"`
	Types     []string `json:"types"`
	Country   string   `json:"country"`
	Product   string   `json:"product"`
	OrderDate string   `json:"order_date,omitempty"`
	// ListedHTS are the listed codes the part's code agrees with.
	ListedHTS []string `json:"listed_hts_matched"`
	Source    *Source  `json:"scope_notice,omitempty"`
}

// PartResult is the screen of one part.
type PartResult struct {
	Name    string  `json:"name,omitempty"`
	HTS     string  `json:"hts,omitempty"`
	Origin  string  `json:"origin,omitempty"`
	Status  string  `json:"status"`
	Reason  string  `json:"reason"`
	Matches []Match `json:"matches,omitempty"`
}

// Result is the screen of all parts.
type Result struct {
	Snapshot string       `json:"snapshot"`
	Fetched  string       `json:"fetched"`
	Orders   int          `json:"orders"`
	Parts    []PartResult `json:"parts"`
	Notice   string       `json:"notice"`
	Limits   []string     `json:"limits"`
}

// Limits says what the screen cannot see.
func Limits(snapshot string) []string {
	return []string{
		"The table lists the orders open to review in the twelve months to " + snapshot + ", with the HTS numbers printed in the newest Commerce notice that carries each scope. A newer order, revocation or scope change is not in it.",
		"A match needs both the HTS code and the country of origin you give. Goods finished in a third country from parts of an ordered country can be within an order's scope (circumvention) and are not seen here.",
		"An order whose scope prints no HTS numbers is never matched, and a product can be within a scope under a code the scope does not list.",
		"Possible exposure is not a finding that duties are owed, and no match is not a finding that they are not. Ask a customs broker or Commerce for a scope ruling.",
	}
}

// Check screens each part against the active orders by HTS code and country
// alone. CheckWith adds the refinements of a Config.
func (t *Table) Check(in Input) Result { return t.CheckWith(in, Config{}) }

// CheckWith screens each part against the active orders and, for a match, the
// refinements in cfg: a refinement can only remove an order from a part's
// matches, never add one.
func (t *Table) CheckWith(in Input, cfg Config) Result {
	res := Result{Snapshot: t.snapshot, Fetched: t.fetched, Orders: len(t.orders), Notice: Notice, Limits: Limits(t.snapshot), Parts: make([]PartResult, len(in.Parts))}
	if cfg.Reader != nil {
		res.Limits = append(res.Limits, ReaderLimit)
		if cfg.MaxReads > 0 {
			cfg.Reader = &cappedReader{inner: cfg.Reader, left: int64(cfg.MaxReads)}
		}
	}
	if cfg.Reader == nil || len(in.Parts) < 2 {
		for i, p := range in.Parts {
			res.Parts[i] = t.part(p, cfg)
		}
		return res
	}
	// a model reader answers in about a second a question: parts are read side by side
	var wg sync.WaitGroup
	sem := make(chan struct{}, readerParallel)
	for i, p := range in.Parts {
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			res.Parts[i] = t.part(p, cfg)
		}()
	}
	wg.Wait()
	return res
}

func (t *Table) part(p Part, cfg Config) PartResult {
	r := PartResult{Name: p.Name, HTS: strings.TrimSpace(p.HTS), Origin: strings.TrimSpace(p.Origin)}
	d := digits(r.HTS)
	switch {
	case r.HTS == "":
		r.Status, r.Reason = StatusNotChecked, "No HTS code given for this part."
		return r
	case d == "":
		r.Status, r.Reason = StatusNotChecked, fmt.Sprintf("%q is not an HTS heading or subheading (4, 6, 8 or 10 digits).", r.HTS)
		return r
	case r.Origin == "":
		r.Status, r.Reason = StatusNotChecked, "No country of origin given for this part."
		return r
	}
	type key struct{ country, product string }
	groups := map[key]*Match{}
	members := map[key][]int{}
	var order []key
	for _, code := range t.codes {
		if !htsMatch(d, code) {
			continue
		}
		for _, i := range t.byHTS[code] {
			o := t.orders[i]
			if !originMatches(r.Origin, o.Country) {
				continue
			}
			k := key{t.origin[i], strings.ToLower(strings.TrimSpace(o.Product))}
			m := groups[k]
			if m == nil {
				m = &Match{Country: o.Country, Product: o.Product, OrderDate: o.OrderDate, Source: o.ScopeNotice}
				groups[k] = m
				order = append(order, k)
			}
			members[k] = append(members[k], i)
			if !contains(m.Cases, o.Case) {
				m.Cases = append(m.Cases, o.Case)
			}
			if !contains(m.Types, o.Type) {
				m.Types = append(m.Types, o.Type)
			}
			for _, h := range o.HTS {
				if htsMatch(d, digits(h)) && !contains(m.ListedHTS, h) {
					m.ListedHTS = append(m.ListedHTS, h)
				}
			}
			if m.OrderDate == "" || (o.OrderDate != "" && o.OrderDate < m.OrderDate) {
				m.OrderDate = o.OrderDate
			}
		}
	}
	if len(order) == 0 {
		r.Status, r.Reason = StatusNone, "No active order lists this HTS code for this country of origin."
		return r
	}
	screened := 0
	for _, k := range order {
		m := groups[k]
		if why := t.screen(p, d, m, members[k], cfg); why != "" {
			screened++
			continue
		}
		sort.Strings(m.Cases)
		sort.Strings(m.Types)
		r.Matches = append(r.Matches, *m)
	}
	if len(r.Matches) == 0 {
		r.Status = StatusNone
		r.Reason = fmt.Sprintf("%d order(s) list this HTS code for this country of origin, but the written scope read against the product name points away from this part; the written scope governs.", screened)
		return r
	}
	sort.SliceStable(r.Matches, func(i, j int) bool { return r.Matches[i].Cases[0] < r.Matches[j].Cases[0] })
	r.Status = StatusPossible
	r.Reason = fmt.Sprintf("%d active order(s) list this HTS code for this country of origin; the written scope governs.", len(r.Matches))
	return r
}

func contains(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}
