package nporigin

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/tariffshift"
)

var (
	reCodeEx    = regexp.MustCompile(`(?i)^ex\s+`)
	reCodeSplit = regexp.MustCompile(`\(([a-z])\)\s*$`)
	reSpaces    = regexp.MustCompile(`\s+`)
)

// ParseCode reads the code column of a row: "8527", "ex 8473", "0901 11",
// "ex 8202 39 (a)", "ex 04 08", "Ex 9021.10", "ex 6911 to ex 6913" or
// "ex 9401 and ex 9403". It returns the ranges covered, whether the row is
// an "ex" row, the split letter and the row key.
func ParseCode(code string) (applies []Range, ex bool, split, key string, err error) {
	c := strings.TrimSpace(reSpaces.ReplaceAllString(code, " "))
	if m := reCodeSplit.FindStringSubmatch(c); m != nil {
		split = m[1]
		c = strings.TrimSpace(c[:len(c)-len(m[0])])
	}
	parts := []string{c}
	span := false
	switch {
	case strings.Contains(c, " to "):
		parts, span = strings.SplitN(c, " to ", 2), true
	case strings.Contains(c, " and "):
		parts = strings.SplitN(c, " and ", 2)
	}
	var digits []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if reCodeEx.MatchString(p) {
			ex = true
			p = reCodeEx.ReplaceAllString(p, "")
		}
		d := tariffshift.Digits(p)
		if len(d) != 4 && len(d) != 6 {
			return nil, false, "", "", fmt.Errorf("nporigin: code %q: bad part %q", code, p)
		}
		digits = append(digits, d)
	}
	if span {
		if len(digits[0]) != len(digits[1]) || digits[0] > digits[1] {
			return nil, false, "", "", fmt.Errorf("nporigin: code %q: bad span", code)
		}
		applies = []Range{{Lo: digits[0], Hi: digits[1]}}
	} else {
		for _, d := range digits {
			applies = append(applies, Range{Lo: d, Hi: d})
		}
	}
	key = strings.Join(digits, "-")
	if ex {
		key += "x"
	}
	return applies, ex, split, key + split, nil
}

// reInlineOr is an "or" that starts another rule ("CTH or Assembly of
// semi-conductor products", "CTH, except from heading 8538; or Assembly
// ..."). An "or" inside a list of headings ("heading 7225 or 7226") is not
// one.
var reInlineOr = regexp.MustCompile(`\s*;?\s+or\s+((?:CC|CTH|CTSH|CTHS|CTSHS)\b|Assembly\b|change from\b|Manufacture\b|Printing\b)`)

// SplitAlternatives splits a rule cell into its alternatives: at a
// paragraph that reads "Or", and at an inline "or" that starts another rule.
func SplitAlternatives(text string) []string {
	var paras []string
	var cur []string
	flush := func() {
		if len(cur) > 0 {
			paras = append(paras, strings.Join(cur, "\n"))
		}
		cur = nil
	}
	for _, l := range strings.Split(text, "\n") {
		l = strings.TrimSpace(l)
		switch {
		case l == "":
		case strings.EqualFold(l, "or"):
			flush()
		default:
			cur = append(cur, l)
		}
	}
	flush()
	var out []string
	for _, p := range paras {
		for {
			loc := reInlineOr.FindStringSubmatchIndex(p)
			if loc == nil {
				break
			}
			if a := strings.TrimSpace(strings.TrimRight(strings.TrimSpace(p[:loc[0]]), ";,")); a != "" {
				out = append(out, a)
			}
			p = p[loc[2]:]
		}
		if a := strings.TrimSpace(p); a != "" {
			out = append(out, a)
		}
	}
	return out
}

// ParseRule splits a primary rule into alternatives and encodes each one it
// can read without guessing. Alternatives are keyed "<row key>.<n>", from 1.
func ParseRule(key, text string) []Alternative {
	if reSharedProviso.MatchString(oneLine(text)) {
		// "CTSH or CTSHS provided recasting ...": the proviso may govern
		// both changes, so the rule is left whole, to an answer.
		return []Alternative{{Key: key + ".1", Text: oneLine(text), Kind: KindProcess,
			Reason: "the proviso may apply to both changes"}}
	}
	var out []Alternative
	for i, t := range SplitAlternatives(text) {
		a := ParseAlternative(t)
		a.Key = fmt.Sprintf("%s.%d", key, i+1)
		out = append(out, a)
	}
	return out
}

var reSharedProviso = regexp.MustCompile(`^(?:CC|CTH|CTSH|CTHS|CTSHS) or (?:CC|CTH|CTSH|CTHS|CTSHS) provided `)

func oneLine(s string) string { return strings.TrimSpace(reSpaces.ReplaceAllString(s, " ")) }

var (
	reMaxValue   = regexp.MustCompile(`^Manufacture in which the value of the materials used does not exceed (\d+) ?% of the ex-works price of the product\.?$`)
	reChangeHead = regexp.MustCompile(`^(CC|CTH|CTSH|CTHS|CTSHS)\b`)
	reChangeFrom = regexp.MustCompile(`^change from (.+?)\s*,\s*provided (.+?)\.?$`)
)

// ParseAlternative encodes one alternative. A change rule (CC to CTSHS,
// with "except from" clauses) is encoded when every clause is read; a value
// rule when its share is read; a rule that starts with neither is a process
// rule, decided only from an answer. A change rule that cannot be read is
// left not encoded, with the reason, and is treated as a process rule.
func ParseAlternative(text string) Alternative {
	t := oneLine(text)
	a := Alternative{Text: t}
	if m := reMaxValue.FindStringSubmatch(t); m != nil {
		pct, _ := strconv.Atoi(m[1])
		a.Kind, a.Encoded, a.MaxShare = KindMaxValue, true, float64(pct)/100
		return a
	}
	if m := reChangeFrom.FindStringSubmatch(t); m != nil {
		rs, described, err := parseList(m[1])
		if err == nil && !described {
			a.Kind, a.Encoded, a.From, a.Condition = KindChangeFrom, true, rs, m[2]
			return a
		}
		a.Kind, a.Reason = KindProcess, "the sources are named by description"
		if err != nil {
			a.Reason = err.Error()
		}
		return a
	}
	if !reChangeHead.MatchString(t) {
		a.Kind, a.Encoded = KindProcess, true
		a.Identifies = strings.HasPrefix(t, "The origin of the goods")
		return a
	}
	a.Kind = KindChange
	if err := parseChange(&a, t); err != nil {
		return Alternative{Text: t, Kind: KindProcess, Reason: err.Error()}
	}
	a.Encoded = true
	return a
}

var levels = map[string]struct {
	level int
	split bool
}{
	"CC": {2, false}, "CTH": {4, false}, "CTSH": {6, false}, "CTHS": {4, true}, "CTSHS": {6, true},
}

var (
	reExclusion = regexp.MustCompile(`^with the exclusion of (.+)$`)
	reCondTail  = regexp.MustCompile(`\s+when this change results from (.+)$`)
)

// parseChange reads "CTH", "CTH, except from heading 8529", "CTH, except
// from heading 7208, 7209 or 7212, and except from heading 7213 when this
// change results from cutting or bending", "CTH with the exclusion of ...".
func parseChange(a *Alternative, t string) error {
	t = strings.TrimSuffix(strings.TrimSpace(t), ".")
	head := reChangeHead.FindString(t)
	lv := levels[head]
	a.Level, a.Split = lv.level, lv.split
	rest := strings.TrimSpace(t[len(head):])
	if rest == "" {
		return nil
	}
	if m := reExclusion.FindStringSubmatch(rest); m != nil {
		a.Condition = "the work is not " + m[1]
		return nil
	}
	if !strings.HasPrefix(rest, ",") {
		return fmt.Errorf("unread text after %s: %q", head, rest)
	}
	rest = strings.TrimSpace(rest[1:])
	if !strings.HasPrefix(rest, "except from ") {
		return fmt.Errorf("unread clause %q", rest)
	}
	for _, clause := range regexp.MustCompile(`,?\s+and except from `).Split(rest, -1) {
		clause = strings.TrimPrefix(strings.TrimSpace(clause), "except from ")
		e := Exception{Text: "except from " + clause}
		if m := reCondTail.FindStringSubmatch(clause); m != nil {
			e.Condition = m[1]
			clause = strings.TrimSpace(clause[:len(clause)-len(m[0])])
		}
		if clause == "the same subheading" {
			return fmt.Errorf("unread clause %q", clause)
		}
		rs, described, err := parseList(clause)
		if err != nil {
			return err
		}
		e.From, e.Described = rs, described
		a.Except = append(a.Except, e)
	}
	return nil
}

var (
	reListSep   = regexp.MustCompile(`\s*,\s*|\s+or\s+`)
	reListWord  = regexp.MustCompile(`^(?:headings?|subheadings?)\s+`)
	reSplitItem = regexp.MustCompile(`^(?:split\s+)?(?:heading|subheading)\s+ex\s+(\d{2}\s?\d{2}(?:\s?\d{2})?)\s*\([a-z]\)$|^ex\s+(\d{2}\s?\d{2}(?:\s?\d{2})?)\s*\([a-z]\)$`)
	reGoodsOf   = regexp.MustCompile(`^([a-z][a-z -]+?) of (?:heading|subheading)\s+(\d{2}\s?\d{2}(?:\s?\d{2})?)$`)
)

// parseList reads "heading 7208, 7209, 7210 or 7212", "headings 7213 to
// 7215", "7221 to 7222", "split heading ex 7228 (a)" and "cold-rolled
// products of heading 7209". described is true when any item names goods
// by description inside a code.
func parseList(s string) (rs []Range, described bool, err error) {
	for _, it := range reListSep.Split(strings.TrimSpace(s), -1) {
		it = strings.TrimSpace(it)
		if it == "" {
			continue
		}
		if m := reSplitItem.FindStringSubmatch(it); m != nil {
			d := tariffshift.Digits(m[1] + m[2])
			rs, described = append(rs, Range{Lo: d, Hi: d}), true
			continue
		}
		if m := reGoodsOf.FindStringSubmatch(it); m != nil {
			d := tariffshift.Digits(m[2])
			rs, described = append(rs, Range{Lo: d, Hi: d}), true
			continue
		}
		it = reListWord.ReplaceAllString(it, "")
		lo, hi, isSpan := strings.Cut(it, " to ")
		if !isSpan {
			hi = lo
		}
		l, h := tariffshift.Digits(lo), tariffshift.Digits(hi)
		if l == "" || h == "" || len(l) != len(h) || l > h || (len(l) != 4 && len(l) != 6) {
			return nil, false, fmt.Errorf("unread item %q", it)
		}
		rs = append(rs, Range{Lo: l, Hi: h})
	}
	if len(rs) == 0 {
		return nil, false, fmt.Errorf("empty list")
	}
	return rs, described, nil
}
