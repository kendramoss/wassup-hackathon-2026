package evreport

import (
	"strings"
	"testing"
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
)

func TestMarkdownKeepsChecksApartAndValuesOnOneLine(t *testing.T) {
	v := 0.25
	facts := "line one\n```\n# injected\n<script>alert(1)</script>\n"
	r := Report{CaseID: "c1", GeneratedAt: time.Date(2026, 11, 24, 6, 0, 0, 0, time.UTC), Separate: Separate, NotARuling: NotARuling,
		Case: CaseFacts{Market: "us", Facts: facts, Product: &domain.Product{Name: "Fan\n# not a heading **bold**",
			Components: []domain.Component{
				{Name: "motor | shaft", HTS: "8501.10", MadeIn: "China"},
				{Name: "<script>alert(1)</script>", HTS: "<img src=x onerror=alert(1)>", MadeIn: "[x](javascript:alert(1))"},
			}}},
		Supplied: map[string]*domain.SuppliedOriginFacts{"eu": {Source: domain.SourceUserSupplied, Stale: true, Answers: map[string]bool{"ch85.r": false},
			Materials: []domain.SuppliedMaterial{{Index: 0, Name: "motor | shaft", ValueShare: &v}}}},
		Checks: []Check{
			{ID: "eu_origin", Title: "EU", Question: "Q1?", Status: NeedsInformation, Outcome: "Not decided", Missing: []Missing{{Label: "Value share", Evidence: "BOM"}}},
			{ID: "uk_origin", Title: "UK", Question: "Q2?", Status: NeedsJudgement, Outcome: "Rule not modelled", NotModelled: []string{"s. 17(3): judgement"}},
		}}
	md := Markdown(r)
	for _, want := range []string{"# Evidence report: Fan \\# not a heading \\*\\*bold\\*\\*", "## EU\n", "## UK\n", "- Value share — BOM", "**Not modelled (a person decides).**",
		"| 1 | motor \\| shaft |", "&lt;script&gt;alert\\(1\\)&lt;/script&gt;", "&lt;img src=x onerror=alert\\(1\\)&gt;", "\\[x\\]\\(javascript:alert\\(1\\)\\)",
		"- Answer ch85.r: no", "value share 25%", "Source: user-supplied", "Not used: saved before the parts list was replaced", Separate} {
		if !strings.Contains(md, want) {
			t.Errorf("missing %q in\n%s", want, md)
		}
	}
	// The facts text sits byte for byte inside one fence longer than its
	// own backtick run.
	if !strings.Contains(md, "````text\n"+strings.TrimSuffix(facts, "\n")+"\n````\n") {
		t.Errorf("facts not exact in a safe fence:\n%s", md)
	}
	outside := strings.Replace(md, facts, "", 1)
	for _, bad := range []string{"<script", "<img", "](javascript:", "\n# injected", "**bold**"} {
		if strings.Contains(outside, bad) {
			t.Errorf("unescaped %q outside the facts block", bad)
		}
	}
	if strings.Contains(strings.ToLower(md), "overall") {
		t.Error("the checks were combined")
	}
}
