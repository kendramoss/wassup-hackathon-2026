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
	if s == "" || hedge.MatchString(s) || strings.Trim(s, ".-– ") == "" {
		return keep
	}
	return append(keep, s)
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
