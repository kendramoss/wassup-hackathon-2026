package tariffshift

// RowsFor returns the 102.20 rows that cover code: the one row a full code
// falls in, or, for a code too short to pick one row (a heading such as
// "8414"), every row inside or overlapping it, in table order. None for a
// code out of the encoded chapters or not a code. Used to quote the rule
// text to the Part 102 engine (D550); it decides nothing.
func (r *Rules) RowsFor(code string) []Rule {
	g := Digits(code)
	if len(g) < 4 || !r.InScope(g) {
		return nil
	}
	var out []Rule
	for _, rule := range r.t.Rules {
		if rule.Applies.contains(g) != no {
			out = append(out, rule)
		}
	}
	return out
}
