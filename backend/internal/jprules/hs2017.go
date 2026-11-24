package jprules

import (
	"strings"
	"sync"
)

// hsCodes is the set of real six-digit subheadings of one HS edition. The HS
// 2017 set comes from data/hs2017_codes.txt (see docs/japan/ajcep-general-validity.md)
// and the HS 2022 set from data/hs2022_codes.txt (see docs/japan/gsp-pilot.md);
// both hold codes only, with the source, hash and licence caveat in those docs.
type hsCodes struct {
	six      map[string]bool
	headings map[string]bool // four-digit prefixes
}

func parseHSCodes(data []byte) *hsCodes {
	c := &hsCodes{six: map[string]bool{}, headings: map[string]bool{}}
	for _, l := range strings.Split(string(data), "\n") {
		if len(l) == 6 {
			c.six[l] = true
			c.headings[l[:4]] = true
		}
	}
	return c
}

var (
	hs2017Valid = sync.OnceValue(func() *hsCodes { return parseHSCodes(hs2017CodesTXT) })
	hs2022Valid = sync.OnceValue(func() *hsCodes { return parseHSCodes(hs2022CodesTXT) })
)

// hasPrefix reports whether digits (two, four or six) is a real code of the edition:
// a listed subheading, or a heading or chapter that has one under it.
func (c *hsCodes) hasPrefix(digits string) bool {
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
