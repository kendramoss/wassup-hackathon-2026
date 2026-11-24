package evreport

import (
	"strings"
	"testing"
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
)

func TestMarkdownKeepsChecksApartAndValuesOnOneLine(t *testing.T) {
	v := 0.25
	r := Report{CaseID: "c1", GeneratedAt: time.Date(2026, 11, 24, 6, 0, 0, 0, time.UTC), Separate: Separate, NotARuling: NotARuling,
		Case: CaseFacts{Market: "us", Facts: "line one\n```\n# injected\n", Product: &domain.Product{Name: "Fan\n# not a heading",
			Components: []domain.Component{{Name: "motor | shaft", HTS: "8501.10", MadeIn: "China"}}}},
		Supplied: map[string]*domain.SuppliedOriginFacts{"eu": {Source: domain.SourceUserSupplied, Answers: map[string]bool{"ch85.r": false},
			Materials: []domain.SuppliedMaterial{{Index: 0, Name: "motor | shaft", ValueShare: &v}}}},
		Checks: []Check{
			{ID: "eu_origin", Title: "EU", Question: "Q1?", Status: NeedsInformation, Outcome: "Not decided", Missing: []Missing{{Label: "Value share", Evidence: "BOM"}}},
			{ID: "uk_origin", Title: "UK", Question: "Q2?", Status: NeedsJudgement, Outcome: "Rule not modelled", NotModelled: []string{"s. 17(3): judgement"}},
		}}
	md := Markdown(r)
	for _, want := range []string{"# Evidence report: Fan # not a heading", "## EU\n", "## UK\n", "- Value share — BOM", "**Not modelled (a person decides).**",
		"| 1 | motor \\| shaft |", "- Answer ch85.r: no", "value share 25%", "Source: user-supplied", Separate} {
		if !strings.Contains(md, want) {
			t.Errorf("missing %q in\n%s", want, md)
		}
	}
	// The facts text stays inside its one fenced block.
	if strings.Count(md, "```") != 2 || strings.Contains(strings.ToLower(md), "overall") {
		t.Errorf("a stored value broke out of its block, or the checks were combined:\n%s", md)
	}
}
