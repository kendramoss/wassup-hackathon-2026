package clarify

import (
	"regexp"
	"strings"
)

// AnswersVersion is recorded with every re-analysis that used answers.
const AnswersVersion = "answers-v2"

// Answer is the requester's answer to one follow-up question.
type Answer struct {
	Question string
	Answer   string
}

// answersHeader tells the engine how to weigh the answers. In answers-v1 the
// engine read "not stated" lines as evidence against the processing country
// and over-weighted facts restated as Q&A (D26).
const answersHeader = "ADDITIONAL FACTS FROM THE REQUESTER (answers to follow-up questions). " +
	"Treat these exactly like the facts above: same weight, no more. " +
	"Points the requester did not address are unknown, not evidence for or against any country.\n"

var (
	// hedge marks a sentence that reports missing information rather than a fact.
	hedge       = regexp.MustCompile(`(?i)\b(not (\w+ly )?(stated|specified|provided|known|available|described|given|mentioned)|unknown|unclear|no information|not sure|unsure|do(es)? not (\w+ly )?(say|state|specify|know)|don'?t know|no idea|n/a|tbd)\b`)
	sentenceEnd = regexp.MustCompile(`[.!?](\s+|$)`)
)

// factual returns the sentences of an answer that state facts, dropping
// sentences that only report what is unknown.
func factual(answer string) string {
	var keep []string
	start := 0
	for _, loc := range sentenceEnd.FindAllStringIndex(answer, -1) {
		keep = appendFact(keep, answer[start:loc[1]])
		start = loc[1]
	}
	keep = appendFact(keep, answer[start:])
	return strings.Join(keep, " ")
}

func appendFact(keep []string, s string) []string {
	s = strings.TrimSpace(s)
	if s == "" || strings.Trim(s, ".-– ") == "" {
		return keep
	}
	if hedge.MatchString(s) {
		return append(keep, splitKnown(s)...)
	}
	return append(keep, s)
}

var (
	// clauseBreak is a point where a sentence can be cut into clauses.
	clauseBreak = regexp.MustCompile(`(?i)\s*;\s*|\s*,\s*(?:(?:but|and|however,?|although|though|while|whereas|yet)\s+)?|\s+(?:but|although|though|whereas|yet|while)\s+|\s+and\s+`)
	bareAnd     = regexp.MustCompile(`(?i)^\s+and\s+$`)
	// hasVerb: a kept clause must contain a finite verb, so a bare noun
	// fragment of an unknown subject list is never kept as a fact.
	hasVerb = regexp.MustCompile(`(?i)\b(is|are|was|were|am|has|have|had|does|did|will|would|can|could|must|should|made|built|cast|bought|sold|sent|come|comes|came|ship|ships|shipped|make|makes|use|uses|used|buy|buys|assemble|assembles|blend|blends|produce|produces|source|sources|remain|remains|stay|stays|contain|contains|cost|costs|import|imports|export|exports|[a-z]{3,}ed)\b`)
	// leadingVerb: a clause that opens with a verb has its subject elsewhere.
	leadingVerb = regexp.MustCompile(`(?i)^(is|are|was|were|am|has|have|had|does|did|will|would|can|could|must|should|made|built|cast|bought|sold|sent|came|comes|remain|remains|stay|stays|[a-z]{3,}ed)\b`)
	// bareAndSubject: after a plain "and" an unknown clause must open with its
	// own subject, else "A and B are unknown" would keep A as a fact.
	bareAndSubject = regexp.MustCompile(`(?i)^(the|a|an|its|their|his|her|our|my|it|this|that|these|those|there|whether)\b`)
	// ownSubject: an unknown clause must have its own subject and finite verb
	// ("its value share is unknown"). An elliptical tail ("but unknown",
	// "; unclear", "though not stated") qualifies the prior claim, so the
	// sentence is dropped whole instead.
	ownSubject = regexp.MustCompile(`(?i)^(the|a|an|its|their|his|her|our|my|this|that|these|those)\s+\S.*?\b(is|are|was|were|has|have|does|do|did|remains?|stays?)\b`)
	// subjectBlock: the words before the first verb must be a plain noun
	// phrase: no wh-word, negation or hedge (those make a later "is" belong to
	// a subordinate clause, as in "not stated where it is made").
	subjectBlock = regexp.MustCompile(`(?i)\b(not|no|never|n't|who|whom|whose|which|what|where|when|why|how|whether|unknown|unclear|unsure|stated|specified|known)\b|n't`)
	verbWord     = regexp.MustCompile(`(?i)\b(is|are|was|were|has|have|does|do|did|remains?|stays?)\b`)
	relative     = regexp.MustCompile(`(?i)^(which|who|whom|whose|that|where|when|it|this|they|these|those|he|she|the (same|former|latter))\b`)
	// scoped marks a sentence whose clauses may be conditioned or qualified by
	// one another (condition, scope, speaker uncertainty, hedged degree).
	// Cutting such a sentence could promote a qualified claim to a fact, so it
	// is dropped whole, as before.
	scoped = regexp.MustCompile(`(?i)\b(if|unless|whether|until|except|excluding|provided|providing|assuming|assume|only|otherwise|when|once|depend\w*|subject to|pending|` +
		`maybe|perhaps|probably|possibly|presumably|apparently|likely|unlikely|might|may|could|would|should|` +
		`think|thought|believe|guess|suppose|seems?|appears?|expect(ed)?|hope|approximately|roughly|about|around|` +
		`not (\w+ly )?(sure|certain|confirmed|verified)|unsure|uncertain|unconfirmed|unverified|tentative|no idea|don'?t know|do not know|` +
		`sometimes|usually|mostly|partly|some|several|many|most)\b`)
	negatedUnknown = regexp.MustCompile(`(?i)\b(not|never|no)\s+(un(known|clear)|n/a)\b`)
)

// splitKnown cuts a sentence that mentions something unknown into clauses and
// returns the clauses that do not report missing information. It is
// conservative: whenever the cut is not clearly delimited (a relative clause,
// a clause with no subject of its own, a verbless fragment on the unknown side)
// it returns nothing, which is the old drop-the-sentence behaviour.
func splitKnown(s string) []string {
	if scoped.MatchString(s) || negatedUnknown.MatchString(s) {
		return nil
	}
	term := ""
	if last := s[len(s)-1]; last == '.' || last == '!' || last == '?' {
		term = string(last)
		s = strings.TrimSpace(s[:len(s)-1])
	}
	locs := clauseBreak.FindAllStringIndex(s, -1)
	if len(locs) == 0 {
		return nil
	}
	type piece struct {
		text, before string
		unknown      bool
	}
	var ps []piece
	prev, delim := 0, ""
	for _, l := range locs {
		ps = append(ps, piece{text: s[prev:l[0]], before: delim})
		prev, delim = l[1], s[l[0]:l[1]]
	}
	ps = append(ps, piece{text: s[prev:], before: delim})
	for i := range ps {
		t := strings.TrimSpace(ps[i].text)
		ps[i].text, ps[i].unknown = t, hedge.MatchString(t)
		if t == "" {
			return nil
		}
		if ps[i].unknown && !hasOwnSubject(t) {
			return nil
		}
		if ps[i].unknown && (relative.MatchString(t) || leadingVerb.MatchString(t) ||
			(bareAnd.MatchString(ps[i].before) && !bareAndSubject.MatchString(t))) {
			return nil
		}
	}
	// A piece next to an unknown clause across a bare "and" must be a clause
	// itself ("A and the B are unknown" must not keep A).
	for i, p := range ps {
		if i > 0 && !p.unknown && ps[i-1].unknown && bareAnd.MatchString(p.before) && !hasVerb.MatchString(p.text) {
			return nil
		}
		if i+1 < len(ps) && !p.unknown && ps[i+1].unknown && bareAnd.MatchString(ps[i+1].before) && !hasVerb.MatchString(p.text) {
			return nil
		}
	}
	// Rejoin runs of known pieces with their original delimiters.
	var segs []string
	cur := ""
	flush := func() {
		if cur != "" {
			segs, cur = append(segs, cur), ""
		}
	}
	for _, p := range ps {
		if p.unknown {
			flush()
			continue
		}
		if cur == "" {
			cur = p.text
		} else {
			cur += p.before + p.text
		}
	}
	flush()
	var out []string
	for _, seg := range segs {
		// A verbless run is the subject list of the unknown clause; a run
		// that opens with a verb is the tail of one. Neither is a fact.
		if !hasVerb.MatchString(seg) || leadingVerb.MatchString(seg) {
			continue
		}
		if seg[len(seg)-1] != '.' && seg[len(seg)-1] != '!' && seg[len(seg)-1] != '?' {
			if term == "" {
				term = "."
			}
			seg += term
		}
		out = append(out, strings.ToUpper(seg[:1])+seg[1:])
	}
	return out
}

// AppendAnswers adds the factual part of each answer to the facts under a
// neutral header. ok is false when no answer states a fact; the facts are then
// returned unchanged.
func AppendAnswers(facts string, as []Answer) (out string, ok bool) {
	var lines []string
	for _, a := range as {
		if f := factual(strings.TrimSpace(a.Answer)); f != "" {
			lines = append(lines, "Q: "+strings.TrimSpace(a.Question)+"\nA: "+f)
		}
	}
	if len(lines) == 0 {
		return facts, false
	}
	return strings.TrimRight(facts, "\n") + "\n\n" + answersHeader + strings.Join(lines, "\n") + "\n", true
}

// hasOwnSubject reports whether an unknown clause opens with a plain noun
// phrase and then its own finite verb. It fails closed on anything else.
func hasOwnSubject(t string) bool {
	if !ownSubject.MatchString(t) {
		return false
	}
	loc := verbWord.FindStringIndex(t)
	return loc != nil && !subjectBlock.MatchString(t[:loc[0]])
}
