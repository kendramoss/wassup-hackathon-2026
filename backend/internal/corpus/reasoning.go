package corpus

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// ReasoningRunes caps the CBP reasoning shown with each precedent.
const ReasoningRunes = 900

var (
	space         = regexp.MustCompile(`\s+`)
	sentenceSplit = regexp.MustCompile(`([.!?])\s+([A-Z“"(])`)
	conclusion    = regexp.MustCompile(`(?i)substantial(ly)? transform|essential character|country of origin of the`)
	// boilerplate marks the legal standard and citations every ruling repeats.
	boilerplate = regexp.MustCompile(`v\. |F\. ?(2d|3d|Supp)|C\.?I\.?T|C\.?C\.?P\.?A|Cir\.|U\.S\. \d|test for determining|totality of the evidence|^See\b|aff.d|is defined (as|in)|analysis is applicable|Ferrostaal|Uniroyal|Anheuser|Gibson-Thomsen|19 C\.?F\.?R|19 U\.?S\.?C`)
)

// Reasoning returns the part of a ruling's text where CBP applies the
// substantial-transformation test to the facts: up to ReasoningRunes of the
// sentences leading to the last conclusion before the HOLDING section, with
// the repeated legal standard and citations removed.
func Reasoning(text string) string {
	t := space.ReplaceAllString(text, " ")
	end := len(t)
	for _, m := range []string{"HOLDING:", "The holding set forth above"} {
		if j := strings.Index(t, m); j > 0 && j < end {
			end = j
		}
	}
	body := t[:end]
	ms := conclusion.FindAllStringIndex(body, -1)
	if len(ms) == 0 {
		return ""
	}
	stop := len(body)
	if j := strings.Index(body[ms[len(ms)-1][1]:], ". "); j >= 0 {
		stop = ms[len(ms)-1][1] + j + 1
	}
	start := max(0, stop-3000)
	sents := strings.Split(sentenceSplit.ReplaceAllString(body[start:stop], "$1\x00$2"), "\x00")
	if start > 0 && len(sents) > 1 {
		sents = sents[1:] // first one is cut mid-sentence
	}
	var keep []string
	n := 0
	for i := len(sents) - 1; i >= 0; i-- {
		s := strings.TrimSpace(sents[i])
		if s == "" || boilerplate.MatchString(s) {
			continue
		}
		if n+len(s) > ReasoningRunes && len(keep) > 0 {
			break
		}
		keep = append([]string{s}, keep...)
		n += len(s) + 1
	}
	return strings.Join(keep, " ")
}

// AttachReasoning fills Ruling.Reasoning from the raw CROSS JSON in rawDir.
// Rulings without a raw file keep an empty reasoning. Returns how many were
// filled.
func (s *Store) AttachReasoning(rawDir string) (int, error) {
	n := 0
	for i := range s.rulings {
		b, err := os.ReadFile(filepath.Join(rawDir, s.rulings[i].ID+".json"))
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return n, fmt.Errorf("corpus: read raw %s: %w", s.rulings[i].ID, err)
		}
		var doc struct {
			Text string `json:"text"`
		}
		if err := json.Unmarshal(b, &doc); err != nil {
			return n, fmt.Errorf("corpus: parse raw %s: %w", s.rulings[i].ID, err)
		}
		if r := Reasoning(doc.Text); r != "" {
			s.rulings[i].Reasoning = r
			n++
		}
	}
	return n, nil
}
