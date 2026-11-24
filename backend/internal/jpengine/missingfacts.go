package jpengine

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
)

const maxMissingShown = 3

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
	seen := map[string]bool{}
	add := func(s string) {
		if k := strings.ToLower(s); !seen[k] {
			seen[k] = true
			out = append(out, s)
		}
	}
	name := func(n, fallback string) string {
		if n = strings.TrimSpace(n); n != "" {
			return n
		}
		return fallback
	}
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
			add("Say where " + name(op.Name, "a production step") + " happens.")
		}
	}
	return out
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
