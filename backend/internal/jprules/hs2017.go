package jprules

import (
	"strings"
	"sync"
)

// hs2017Codes is the set of real HS 2017 six-digit subheadings. It comes from
// data/hs2017_codes.txt (codes only); see docs/japan/ajcep-general-validity.md
// for the source, hash and licence caveat.
type hs2017Codes struct {
	six      map[string]bool
	headings map[string]bool // four-digit prefixes
}

var hs2017Valid = sync.OnceValue(func() *hs2017Codes {
	c := &hs2017Codes{six: map[string]bool{}, headings: map[string]bool{}}
	for _, l := range strings.Split(string(hs2017CodesTXT), "\n") {
		if len(l) == 6 {
			c.six[l] = true
			c.headings[l[:4]] = true
		}
	}
	return c
})

// hasPrefix reports whether digits (two, four or six) is a real HS 2017 code:
// a listed subheading, or a heading or chapter that has one under it.
func (c *hs2017Codes) hasPrefix(digits string) bool {
	switch len(digits) {
	case 6:
		return c.six[digits]
	case 4:
		return c.headings[digits]
	case 2:
		for h := range c.headings {
			if h[:2] == digits {
				return true
			}
		}
	}
	return false
}
