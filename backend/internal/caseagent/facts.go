package caseagent

import (
	"fmt"
	"strings"
)

// Facts the agent can read. get_case shows the first FactsHeadRunes and says
// when more exist; get_case_facts returns the rest in pages of at most
// MaxFactsPageRunes. Every count is in runes, never bytes.
const (
	FactsHeadRunes    = 6000
	MaxFactsPageRunes = 4000
)

// FactsSection is the FACTS block of the get_case text: the whole facts when
// they fit, otherwise the first FactsHeadRunes with an explicit note that the
// text continues and how to read it. It never silently truncates.
func FactsSection(facts string) string {
	r := []rune(facts)
	if len(r) <= FactsHeadRunes {
		return fmt.Sprintf("FACTS (%d runes, complete):\n%s\n", len(r), facts)
	}
	return fmt.Sprintf("FACTS (runes 0-%d of %d; INCOMPLETE: the remaining %d runes, including any appended clarification answers, are NOT shown here; call get_case_facts with offset=%d):\n%s\n",
		FactsHeadRunes, len(r), len(r)-FactsHeadRunes, FactsHeadRunes, string(r[:FactsHeadRunes]))
}

// factsPage serves runes [offset, offset+limit) of the current case's facts.
// limit 0 means the page maximum. Bad ranges get a plain error result, never
// a partial page.
func factsPage(facts string, offset, limit int) string {
	r := []rune(facts)
	total := len(r)
	if offset < 0 || offset > total {
		return fmt.Sprintf("invalid offset %d: the facts are %d runes; use 0 to %d", offset, total, total)
	}
	if limit < 0 {
		return fmt.Sprintf("invalid limit %d: use 1 to %d", limit, MaxFactsPageRunes)
	}
	if offset == total {
		return fmt.Sprintf("no more facts: offset %d equals the total of %d runes", offset, total)
	}
	note := ""
	if limit == 0 {
		limit = MaxFactsPageRunes
	} else if limit > MaxFactsPageRunes {
		limit, note = MaxFactsPageRunes, fmt.Sprintf("; limit capped at %d", MaxFactsPageRunes)
	}
	end := offset + limit
	if end > total {
		end = total
	}
	var h strings.Builder
	fmt.Fprintf(&h, "FACTS runes %d-%d of %d", offset, end, total)
	if end < total {
		fmt.Fprintf(&h, "; more follows, next_offset=%d", end)
	} else {
		h.WriteString("; end of facts")
	}
	h.WriteString(note)
	return h.String() + "\n" + string(r[offset:end])
}
