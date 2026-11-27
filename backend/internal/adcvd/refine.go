package adcvd

import (
	"regexp"
	"strings"
	"sync/atomic"
)

// Verdict is a scope reader's answer for one product against one order's
// written scope.
type Verdict string

// Scope reader answers. VerdictOut removes a match; with Config.ReaderLean so does VerdictUnclearOut.
const (
	VerdictIn      Verdict = "in"
	VerdictOut     Verdict = "out"
	VerdictUnclear Verdict = "unclear"
	// VerdictUnclearOut and VerdictUnclearIn are an unclear answer with the
	// reader's best guess if it had to decide.
	VerdictUnclearOut Verdict = "unclear_out"
	VerdictUnclearIn  Verdict = "unclear_in"
)

// Reader reads a product against the written scope of an order. An error or
// an empty verdict means no answer, which never removes a match.
type Reader interface {
	Read(p Part, o Order) (Verdict, error)
}

// Config selects refinements applied to the matches of a part. The zero
// value is the plain HTS-and-country check.
type Config struct {
	// MinListedDigits removes a match when every listed code the part's code
	// agrees with has fewer digits (heading-level listings).
	MinListedDigits int
	// TitleTerms removes a match when the part's name shares no product term
	// with the order's product name.
	TitleTerms bool
	// ScopeTerms removes a match when the part's name shares no product term
	// with the order's written scope.
	ScopeTerms bool
	// Exclusions removes a match when the part's name is, by its terms, the
	// product an exclusion sentence of the written scope names.
	Exclusions bool
	// Reader, when set, is asked about each remaining match; "out" removes it.
	Reader Reader
	// MinScopeTerms, when above zero, keeps an order unread (no term step, no
	// exclusion step, no reader question) when its written scope holds fewer
	// product terms than this (D943). Zero is the method as D942 registered it.
	MinScopeTerms int
	// MaxReads caps the questions put to Reader in one CheckWith call (0: no
	// cap); past it a match is kept unread.
	MaxReads int
	// ReaderLean also removes a match the reader called unclear but, forced to
	// decide, would call out.
	ReaderLean bool
}

// MinScopeTerms is the value of Config.MinScopeTerms the shipped check uses:
// a written scope with fewer product terms is a pointer to an appendix the
// notice does not print.
const MinScopeTerms = 8

// representative returns the order of a match group whose written scope is
// the longest (companion AD and CVD orders print one scope).
func (t *Table) representative(idx []int) Order {
	best := t.orders[idx[0]]
	for _, i := range idx[1:] {
		if len(t.orders[i].ScopeText) > len(best.ScopeText) {
			best = t.orders[i]
		}
	}
	return best
}

// screen returns why a match is removed ("" keeps it). Nothing is removed
// that the refinement cannot read: a part without a name, an order without
// scope text.
func (t *Table) screen(p Part, partDigits string, m *Match, idx []int, cfg Config) string {
	if cfg.MinListedDigits > 0 {
		longest := 0
		for _, h := range m.ListedHTS {
			if d := digits(h); len(d) > longest {
				longest = len(d)
			}
		}
		if longest > 0 && longest < cfg.MinListedDigits {
			return "listed_level"
		}
	}
	name := strings.TrimSpace(p.Name)
	o := t.representative(idx)
	if name == "" {
		return ""
	}
	pt := Terms(name)
	if cfg.TitleTerms && len(pt) > 0 {
		if tt := Terms(o.Product); len(tt) > 0 && shared(pt, tt) == 0 {
			return "title_terms"
		}
	}
	// a scope text that is a pointer ("See the Appendix to this notice.") says
	// nothing to read a product against: such an order is kept (D943).
	if strings.TrimSpace(o.ScopeText) == "" {
		return ""
	}
	st := Terms(o.ScopeText)
	if cfg.MinScopeTerms > 0 && len(st) < cfg.MinScopeTerms {
		return ""
	}
	if cfg.ScopeTerms && len(pt) > 0 && shared(pt, st) == 0 {
		return "scope_terms"
	}
	if cfg.Exclusions && excludedByScope(pt, o.ScopeText) {
		return "exclusion"
	}
	if cfg.Reader != nil {
		if v, err := cfg.Reader.Read(p, o); err == nil && (v == VerdictOut || (cfg.ReaderLean && v == VerdictUnclearOut)) {
			return "reader"
		}
	}
	return ""
}

// excludedByScope reports whether an exclusion sentence of the scope is about
// the product the terms describe: at least two terms in common and half of
// the product's terms.
func excludedByScope(pt map[string]bool, scope string) bool {
	if len(pt) == 0 {
		return false
	}
	for _, s := range sentences(scope) {
		if !exclusionRE.MatchString(s) {
			continue
		}
		n := shared(pt, Terms(s))
		if n >= 2 && 2*n >= len(pt) {
			return true
		}
	}
	return false
}

var exclusionRE = regexp.MustCompile(`(?i)\b(exclud\w*|not covered|not included|does not include|do not include|outside the scope|not subject|exempt\w*|except\w*)\b`)

var sentenceSplit = regexp.MustCompile(`(?:[.;]\s+|\n)`)

func sentences(s string) []string {
	var out []string
	for _, x := range sentenceSplit.Split(s, -1) {
		if x = strings.TrimSpace(x); x != "" {
			out = append(out, x)
		}
	}
	return out
}

// readerParallel is how many parts are read at once.
const readerParallel = 8

// ReaderLimit is added to the limits of a result when a scope reader ran.
const ReaderLimit = "A model read each listed order's written scope against the part's name and removed an order only when the scope's words clearly point away from it; the reader sees the scope as the notice prints it, without any appendix the scope refers to, and can be wrong. A part with no name, or an order whose notice printed no scope text, is not read."

// cappedReader stops asking its reader after left questions.
type cappedReader struct {
	inner Reader
	left  int64
}

func (c *cappedReader) Read(p Part, o Order) (Verdict, error) {
	if atomic.AddInt64(&c.left, -1) < 0 {
		return "", nil
	}
	return c.inner.Read(p, o)
}
