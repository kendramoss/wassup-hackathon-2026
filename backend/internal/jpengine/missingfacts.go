package jpengine

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
)

const (
	maxMissingShown = 3
	maxNameRunes    = 60
)

// MissingFacts lists, in plain English, what the user could add to the
// structured product so a specialist can finish the check. A blank country
// means unknown; it is never read as zero or as originating, and no origin is
// inferred from plants or suppliers. Free-text cases (no Product) return nil.
// The result is deterministic and has repeated items (case-insensitive) removed.
func MissingFacts(c domain.Case) []string {
	p := c.Product
	if p == nil {
		return nil
	}
	var out []string
	add := func(s string) {
		for _, kept := range out {
			if strings.EqualFold(kept, s) {
				return
			}
		}
		out = append(out, s)
	}
	name := cleanName
	if len(p.Components) == 0 {
		add("List the materials used.")
	}
	for _, part := range p.Components {
		if strings.TrimSpace(part.MadeIn) == "" {
			add("Say where " + name(part.Name, "a material") + " was made.")
		}
	}
	if len(p.Operations) == 0 {
		add("List the production steps.")
	}
	for _, op := range p.Operations {
		if pl, ok := p.PlantByID(op.Plant); !ok || strings.TrimSpace(pl.Country) == "" {
			add("Say where " + name(op.Name, "a step") + " happens.")
		}
	}
	return out
}

// cleanName makes a user-entered name safe to show: whitespace and control
// characters collapse to single spaces, and it is cut to maxNameRunes.
func cleanName(n, fallback string) string {
	n = strings.Join(strings.FieldsFunc(n, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) }), " ")
	if n == "" {
		return fallback
	}
	if utf8.RuneCountInString(n) > maxNameRunes {
		n = string([]rune(n)[:maxNameRunes]) + "…"
	}
	return n
}

// missingFactsSentence renders at most three items, then a count of the rest.
// It returns "" when there is nothing to add.
func missingFactsSentence(items []string) string {
	if len(items) == 0 {
		return ""
	}
	shown := items
	if len(shown) > maxMissingShown {
		shown = shown[:maxMissingShown]
	}
	first, size := utf8.DecodeRuneInString(shown[0])
	parts := append([]string{string(unicode.ToLower(first)) + shown[0][size:]}, shown[1:]...)
	s := " To help, " + strings.Join(parts, " ")
	if rest := len(items) - len(shown); rest > 0 {
		s += fmt.Sprintf(" And %d more.", rest)
	}
	return s
}
