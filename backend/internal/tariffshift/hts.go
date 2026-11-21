package tariffshift

import (
	"fmt"
	"strings"
)

// tri is three-valued: a code that is only known to the heading cannot say
// whether it is in a range of subheadings, so some answers are "maybe".
type tri int8

const (
	no tri = iota
	maybe
	yes
)

func (t tri) not() tri { return 2 - t }

func and(a, b tri) tri {
	if a < b {
		return a
	}
	return b
}

func or(a, b tri) tri {
	if a > b {
		return a
	}
	return b
}

// Digits returns the HTS digits of s ("8473.30.5100" → "8473305100"), or ""
// when s holds no digits or an odd chapter-only form. Spaces, dots and
// dashes are dropped; any other character makes it invalid.
func Digits(s string) string {
	var b strings.Builder
	for _, r := range strings.TrimSpace(s) {
		switch {
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '.' || r == ' ' || r == '-':
		default:
			return ""
		}
	}
	d := b.String()
	if len(d) < 2 || len(d) > 10 {
		return ""
	}
	return d
}

// Range is an inclusive span of HTS codes at one level: chapters (2 digits),
// headings (4), subheadings (6) or statistical breakouts (8). Lo and Hi have
// the same length.
type Range struct {
	Lo string `json:"lo"`
	Hi string `json:"hi"`
}

// Level is the number of digits the range compares.
func (r Range) Level() int { return len(r.Lo) }

func (r Range) String() string {
	if r.Lo == r.Hi {
		return format(r.Lo)
	}
	return format(r.Lo) + "-" + format(r.Hi)
}

func format(d string) string {
	switch {
	case len(d) <= 4:
		return d
	case len(d) <= 6:
		return d[:4] + "." + d[4:]
	default:
		return d[:4] + "." + d[4:6] + "." + d[6:]
	}
}

// ParseRange reads "8473", "8470.10-8471.50" or "0101-0106".
func ParseRange(s string) (Range, error) {
	lo, hi, found := strings.Cut(s, "-")
	if !found {
		hi = lo
	}
	l, h := Digits(lo), Digits(hi)
	if l == "" || h == "" || len(l) != len(h) || l > h {
		return Range{}, fmt.Errorf("tariffshift: bad range %q", s)
	}
	return Range{Lo: l, Hi: h}, nil
}

// contains says whether code is in r. A code shorter than the range's level
// is "maybe" when its span overlaps the range without lying inside it.
func (r Range) contains(code string) tri {
	n := r.Level()
	if len(code) >= n {
		c := code[:n]
		if c >= r.Lo && c <= r.Hi {
			return yes
		}
		return no
	}
	lo := code + strings.Repeat("0", n-len(code))
	hi := code + strings.Repeat("9", n-len(code))
	switch {
	case hi < r.Lo || lo > r.Hi:
		return no
	case lo >= r.Lo && hi <= r.Hi:
		return yes
	}
	return maybe
}

// truncate widens r to a coarser level (n digits).
func (r Range) truncate(n int) Range {
	if n >= r.Level() {
		return r
	}
	return Range{Lo: r.Lo[:n], Hi: r.Hi[:n]}
}

// differ says whether two codes differ in their first n digits.
func differ(a, b string, n int) tri {
	m := min(len(a), len(b), n)
	if a[:m] != b[:m] {
		return yes
	}
	if len(a) >= n && len(b) >= n {
		return no
	}
	return maybe
}

func anyContains(rs []Range, code string) tri {
	t := no
	for _, r := range rs {
		t = or(t, r.contains(code))
	}
	return t
}
