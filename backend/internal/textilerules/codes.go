package textilerules

import "strings"

// Digits keeps the digits of an HTS code.
func Digits(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// span is the interval of 10-digit codes a code of 4 to 10 digits stands
// for: "6104" is 6104000000 to 6104999999.
func span(d string) (lo, hi string, ok bool) {
	if len(d) < 4 || len(d) > 10 {
		return "", "", false
	}
	pad := 10 - len(d)
	return d + strings.Repeat("0", pad), d + strings.Repeat("9", pad), true
}

// rangeOf reads a table code or a code range ("5001-5002", "6209.20.1000-
// 6209.20.5035", "4202.12.40-89", "6501", "6505.00 (except ...)") into its
// 10-digit bounds.
func rangeOf(s string) (lo, hi string, ok bool) {
	if i := strings.Index(s, "("); i >= 0 {
		s = s[:i]
	}
	a, b, two := strings.Cut(strings.TrimSpace(s), "-")
	da := Digits(a)
	lo, hi, ok = span(da)
	if !ok {
		return "", "", false
	}
	if !two {
		return lo, hi, true
	}
	db := Digits(b)
	if len(db) < len(da) { // "4202.12.40-89": the tail replaces the end of the first code
		db = da[:len(da)-len(db)] + db
	}
	_, hi, ok = span(db)
	return lo, hi, ok
}

// relation of a good's code to a range.
type relation int

const (
	disjoint relation = iota
	inside            // the good's whole span is inside the range
	partial           // the code is too short to say: it overlaps the range without being inside it
)

func relate(good string, lo, hi string) relation {
	glo, ghi, ok := span(good)
	if !ok {
		return disjoint
	}
	switch {
	case ghi < lo || glo > hi:
		return disjoint
	case glo >= lo && ghi <= hi:
		return inside
	}
	return partial
}

// RowMatches reports whether a row's range overlaps a code (for display).
func RowMatches(r Row, code string) bool {
	return relate(Digits(code), r.Lo, r.Hi) != disjoint
}
