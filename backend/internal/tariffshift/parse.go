package tariffshift

import (
	"fmt"
	"regexp"
	"strings"
)

// SplitAlternatives splits a rule's text into its "A change to ..."
// sentences. Lines that do not start a sentence (the numbered process lists
// in chapter 73) stay with the sentence before them.
func SplitAlternatives(text string) []string {
	var out []string
	for _, part := range strings.Split(text, ";") {
		p := clean(part)
		bare := strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(p, "or "), "and "))
		if strings.HasPrefix(bare, "A change") || len(out) == 0 {
			if p != "" {
				out = append(out, p)
			}
			continue
		}
		if p != "" {
			out[len(out)-1] += "; " + p
		}
	}
	return out
}

var space = regexp.MustCompile(`\s+`)

func clean(s string) string {
	s = space.ReplaceAllString(strings.TrimSpace(s), " ")
	s = strings.ReplaceAll(s, " ,", ",")
	return s
}

// trimSentence strips the joining words and closing punctuation of one
// alternative: "or A change to heading 8501 from any other heading." →
// "A change to heading 8501 from any other heading".
func trimSentence(s string) string {
	s = clean(s)
	for _, p := range []string{"or ", "and "} {
		s = strings.TrimPrefix(s, p)
	}
	for {
		t := strings.TrimRight(s, ".;: ")
		t = strings.TrimSuffix(t, " or")
		t = strings.TrimSuffix(t, " and")
		t = strings.TrimSuffix(t, ",")
		if t == s {
			return s
		}
		s = t
	}
}

var (
	reTo       = regexp.MustCompile(`^(?:headings?|subheadings?) (\d{4}(?:\.\d{2})?)(?: through (\d{4}(?:\.\d{2})?))?$`)
	reAnyOther = regexp.MustCompile(`^any (other )?(chapter|heading|subheading)` +
		`(?:( outside that group)|(?:,)? including another (?:chapter|heading|subheading) within that group|( within that group))?$`)
	reProviso   = regexp.MustCompile(`,? provided that (?:the |that )?change is not the result of (?:a )?simple assembly$`)
	reWholeSA   = regexp.MustCompile(`,? except (?:a change resulting from|when resulting from|when that change is the result of) (?:a )?simple assembly$`)
	reWholeGRI  = regexp.MustCompile(`,? except when that change is pursuant to General Rule of Interpretation 2\(a\)$`)
	reExceptCut = regexp.MustCompile(`,? (?:and |or )?except (?:for )?`)
	reSACond    = regexp.MustCompile(` (?:when|where) (?:resulting from|that change is the result of|the change is the result of|such change from [^,]+ is the result of) (?:a )?simple assembly$`)
	reBendCond  = regexp.MustCompile(` unless the change from these headings involves bending to shape$`)
	reGRICond   = regexp.MustCompile(` when that change is pursuant to General Rule of Interpretation 2\(a\)$`)
	reNotSACond = regexp.MustCompile(`,? except when resulting from (?:a )?simple assembly$`)
	reChangeTo  = regexp.MustCompile(`^a change to ((?:headings?|subheadings?) [\d.]+(?: through [\d.]+)?) from (.+)$`)
	reWithinSA  = regexp.MustCompile(`^a change within that group resulting from (?:a )?simple assembly$`)
	// reExceptSeg splits an exception list into its "from ..." segments.
	reExceptSeg  = regexp.MustCompile(`,? (?:or|and) from |, from `)
	reMaterialOf = regexp.MustCompile(`of headings \d{4} to \d{4} of heading`)
	reLevelWord  = regexp.MustCompile(`\b(?:chapters?|subheadings?|headings?)\b`)
)

// ParseAlternative turns one sentence into an Alternative. It never errors:
// a sentence it cannot read fully comes back with Encoded false and the
// reason, so the evaluator can say it needs a person.
func ParseAlternative(text string) Alternative {
	s := trimSentence(text)
	a := Alternative{Text: s}
	fail := func(reason string) Alternative {
		return Alternative{Text: s, Reason: reason}
	}
	body, ok := strings.CutPrefix(s, "A change to ")
	if !ok {
		return fail("not a change-in-classification sentence")
	}
	to, rest, ok := strings.Cut(body, " from ")
	if !ok {
		return fail("no source clause")
	}
	if strings.Contains(to, "other than a change to") || strings.Contains(rest, "other than a change to") {
		return fail("carve-out by goods description")
	}
	m := reTo.FindStringSubmatch(to)
	if m == nil {
		// Keep the codes the description names ("printer units of ADP
		// machines of subheading 8443.31 through 8443.32"), so the
		// evaluator knows which goods the sentence can apply to.
		f := fail("applies to goods by description, not by code")
		f.To = describedTo(to)
		return f
	}
	hi := m[2]
	if hi == "" {
		hi = m[1]
	}
	tr, err := ParseRange(m[1] + "-" + hi)
	if err != nil {
		return fail(err.Error())
	}
	a.To = []Range{tr}
	if strings.Contains(rest, ";") {
		return fail("process requirements in the text")
	}

	if loc := reProviso.FindStringIndex(rest); loc != nil {
		a.NotSimpleAssembly = true
		rest = rest[:loc[0]]
	}
	// Any other proviso turns on a share by weight or content the codes
	// cannot show ("provided that the good contains less than 65 percent by
	// dry weight of sugar", D460).
	if strings.Contains(rest, "provided that") {
		return fail("a proviso on content the codes cannot show")
	}
	// A trailing "except when resulting from a simple assembly" (or "...
	// pursuant to GRI 2(a)") limits the whole alternative when a comma sets
	// it off or it follows the only source. Written with no comma straight
	// after a listed source that follows other sources ("from any other
	// heading, or from subheading 8481.90 except when resulting from a
	// simple assembly"), it limits that last source only (#550).
	lastCond := Condition("")
	for _, x := range []struct {
		re    *regexp.Regexp
		whole *bool
		src   Condition
	}{{reWholeSA, &a.NotSimpleAssembly, CondNotSimpleAssembly}, {reWholeGRI, &a.NotGRI2a, CondNotGRI2a}} {
		loc := x.re.FindStringIndex(rest)
		if loc == nil {
			continue
		}
		pre := rest[:loc[0]]
		if srcs := splitSources(pre); !strings.HasPrefix(rest[loc[0]:], ",") && len(srcs) > 1 && !reAnyOther.MatchString(srcs[len(srcs)-1]) {
			lastCond = x.src
		} else {
			*x.whole = true
		}
		rest = pre
	}

	head, excepts := rest, ""
	if loc := reExceptCut.FindStringIndex(rest); loc != nil {
		head, excepts = rest[:loc[0]], rest[loc[0]:]
	}

	// Sources: "any other heading" and/or listed codes joined by "or from".
	parts := splitSources(head)
	for _, p := range parts {
		if m := reAnyOther.FindStringSubmatch(p); m != nil && a.From == 0 {
			a.From = levelDigits(m[2])
			switch {
			case m[3] != "":
				a.Group = "outside"
			case m[4] != "":
				a.Group = "within"
			case strings.Contains(p, "including another"):
				a.Group = "including"
			}
			continue
		}
		src := Source{Text: p}
		q := p
		if loc := reNotSACond.FindStringIndex(q); loc != nil {
			src.Condition = CondNotSimpleAssembly
			q = q[:loc[0]]
		}
		if g, ok := strings.CutPrefix(q, "any other good of "); ok {
			src.Described = true
			q = g
		}
		rs, described, err := parseList(q)
		if err != nil || len(rs) == 0 {
			return fail(fmt.Sprintf("source not read: %q", p))
		}
		src.Described = src.Described || described
		src.From = rs
		a.Also = append(a.Also, src)
	}
	if a.From == 0 && len(a.Also) == 0 {
		return fail("no source read")
	}
	if lastCond != "" {
		if len(a.Also) == 0 || a.Also[len(a.Also)-1].Condition != "" {
			return fail("source limitation not read")
		}
		a.Also[len(a.Also)-1].Condition = lastCond
	}
	if a.From != 0 && a.Group == "" {
		if t := tr.truncate(a.From); t.Lo != t.Hi {
			a.Group, a.GroupImplied = "outside", true
		}
	}

	if excepts != "" {
		for _, c := range reExceptCut.Split(excepts, -1) {
			c = strings.TrimSpace(c)
			if c == "" {
				continue
			}
			es, reason := parseExcept(c, tr)
			if reason != "" {
				return fail(reason)
			}
			a.Except = append(a.Except, es...)
		}
	}
	a.Encoded = true
	return a
}

// splitSources splits "any other heading, or from subheading 8481.90" into
// its sources.
func splitSources(head string) []string {
	head = strings.TrimSpace(head)
	head = strings.ReplaceAll(head, ", or from ", " or from ")
	var out []string
	for _, p := range strings.Split(head, " or from ") {
		if p = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(p), ",")); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// parseExcept reads one "except ..." clause (the word "except" already cut).
// A clause whose last segment carries a condition ("..., or from heading
// 8473 when the change is the result of a simple assembly") splits into an
// unconditional part and a conditional part.
func parseExcept(c string, to Range) ([]Exception, string) {
	c = strings.TrimSpace(strings.TrimSuffix(c, ","))
	if reWithinSA.MatchString(c) {
		return []Exception{{From: []Range{to}, Condition: CondSimpleAssembly, Text: c}}, ""
	}
	var narrow []Range
	if m := reChangeTo.FindStringSubmatch(c); m != nil {
		r := reTo.FindStringSubmatch(m[1])
		if r == nil {
			return nil, fmt.Sprintf("exception target not read: %q", c)
		}
		hi := r[2]
		if hi == "" {
			hi = r[1]
		}
		nr, err := ParseRange(r[1] + "-" + hi)
		if err != nil {
			return nil, err.Error()
		}
		narrow = []Range{nr}
		c = "from " + m[2]
	}
	body, ok := strings.CutPrefix(c, "from ")
	if !ok {
		return nil, fmt.Sprintf("exception not read: %q", c)
	}
	cond := Condition("")
	for _, x := range []struct {
		re *regexp.Regexp
		c  Condition
	}{{reSACond, CondSimpleAssembly}, {reBendCond, CondUnlessBending}, {reGRICond, CondGRI2a}} {
		if loc := x.re.FindStringIndex(body); loc != nil {
			cond = x.c
			body = body[:loc[0]]
			break
		}
	}
	// Each "from ..." is its own exception, with or without a comma before
	// "or from" / "and from", and at a bare ", from " between list items
	// (#551). A segment that starts with a number takes the level word of
	// the segment before it ("from subheading 8475.21 through 8475.29, from
	// 8486.10 through 8486.40").
	segs := reExceptSeg.Split(body, -1)
	var out []Exception
	level := ""
	for i, seg := range segs {
		seg = strings.TrimSuffix(strings.TrimPrefix(strings.TrimSpace(seg), "from "), ",")
		if m := reLevelWord.FindAllString(seg, -1); len(m) > 0 {
			level = m[len(m)-1]
		} else if seg != "" && seg[0] >= '0' && seg[0] <= '9' && level != "" {
			seg = level + " " + seg
		}
		segCond := Condition("")
		if i == len(segs)-1 {
			segCond = cond
		}
		// "articles ... of other materials of headings 3901 to 3914 of
		// heading 9619" quotes the HTSUS text of goods of heading 9619; the
		// inner headings describe their material, not a source (D460).
		rs, described, err := parseList(reMaterialOf.ReplaceAllString(seg, "of heading"))
		if err != nil || len(rs) == 0 {
			return nil, fmt.Sprintf("exception list not read: %q", seg)
		}
		out = append(out, Exception{To: narrow, From: rs, Condition: segCond, Described: described, Text: seg})
	}
	return out, ""
}

var (
	reListTok = regexp.MustCompile(`\d{4}\.\d{2}\.\d{2,4}|\d{4}\.\d{2}|\d{1,4}|[A-Za-z][A-Za-z()'’“”-]*`)
	fillers   = map[string]bool{"or": true, "and": true, "from": true, "of": true, "the": true}
)

// parseList reads a list of codes: "heading 7303, 7304, 7305, or 7306",
// "subheading 8486.10 through 8486.40", "chapter 16". described is true when
// the list also holds words that describe goods, which makes any match only
// a possible one.
func parseList(s string) (rs []Range, described bool, err error) {
	level := 0
	var pending string
	through := false
	for _, tok := range reListTok.FindAllString(s, -1) {
		switch t := strings.ToLower(tok); {
		case t == "chapter" || t == "chapters":
			level = 2
		case t == "heading" || t == "headings":
			level = 4
		case t == "subheading" || t == "subheadings":
			level = 6
		case t == "through" || t == "to":
			if pending == "" {
				return nil, false, fmt.Errorf("range without start in %q", s)
			}
			through = true
		case tok[0] >= '0' && tok[0] <= '9':
			d := Digits(tok)
			if len(tok) <= 2 {
				if level != 2 {
					return nil, false, fmt.Errorf("bare number %q in %q", tok, s)
				}
				d = fmt.Sprintf("%02s", tok)
			}
			if d == "" {
				return nil, false, fmt.Errorf("bad code %q", tok)
			}
			if level == 0 {
				return nil, false, fmt.Errorf("code without level in %q", s)
			}
			if through {
				if len(d) != len(pending) || d < pending {
					return nil, false, fmt.Errorf("bad range %s-%s", pending, d)
				}
				rs = append(rs, Range{Lo: pending, Hi: d})
				pending, through = "", false
				continue
			}
			if pending != "" {
				rs = append(rs, Range{Lo: pending, Hi: pending})
			}
			pending = d
		case fillers[t]:
		default:
			described = true
		}
	}
	if through {
		return nil, false, fmt.Errorf("open range in %q", s)
	}
	if pending != "" {
		rs = append(rs, Range{Lo: pending, Hi: pending})
	}
	return rs, described, nil
}

// describedTo reads the codes after the last level word of a described
// target, or nil when there are none or they do not read cleanly.
func describedTo(to string) []Range {
	i := -1
	for _, w := range []string{"subheading ", "subheadings ", "heading ", "headings "} {
		if j := strings.LastIndex(to, w); j > i {
			i = j
		}
	}
	if i < 0 {
		return nil
	}
	rs, described, err := parseList(to[i:])
	if err != nil || described {
		return nil
	}
	return rs
}

func levelDigits(w string) int {
	switch w {
	case "chapter":
		return 2
	case "heading":
		return 4
	}
	return 6
}
