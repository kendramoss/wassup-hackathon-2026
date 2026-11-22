package planner

import (
	"strings"
	"testing"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
)

const handSolder = "hand-solder through-hole parts onto the board; no reflow oven is used"

func movedPCBAProduct(t *testing.T, supplied bool) (domain.Product, Template) {
	t.Helper()
	r, tm := fixture(t, "heatgun_2022.json")
	p := r.Product
	p.Operations = append([]domain.Operation(nil), p.Operations...)
	if supplied {
		p.Operations = append(p.Operations, domain.Operation{Name: handSolder, Plant: pcbaPlants(t, p), Group: "pcba"})
	}
	return p, tm
}

func pcbaPlants(t *testing.T, p domain.Product) (from string) {
	t.Helper()
	for _, pl := range p.Plants {
		if pl.Country == "China" {
			return pl.ID
		}
	}
	t.Fatal("no China plant in fixture")
	return ""
}

func TestRenderMovedGroupKeepsSuppliedOperations(t *testing.T) {
	p, tm := movedPCBAProduct(t, true)
	moved := Apply(p, domain.Move{{Group: "pcba", ToPlant: "th"}})
	got := Render(moved, tm, "pcba")
	want := "In Thailand, the printed circuit board assembly (PCBA) will be produced in-house: " + handSolder + "."
	if strings.Contains(got, "assumed from") {
		t.Errorf("supplied operations must not carry the template-assumption label:\n%s", got)
	}
	if !strings.Contains(got, want) {
		t.Errorf("supplied operation missing or relocated wrongly:\n%s", got)
	}
	for _, bad := range []string{"SMT placement", "reflow of the printed"} {
		if strings.Contains(got, bad) {
			t.Errorf("template process %q asserted despite supplied operations:\n%s", bad, got)
		}
	}
}

func TestRenderMovedGroupAbsentOperationsUsesTemplate(t *testing.T) {
	p, tm := movedPCBAProduct(t, false)
	moved := Apply(p, domain.Move{{Group: "pcba", ToPlant: "th"}})
	got := Render(moved, tm, "pcba")
	if !strings.Contains(got, "In Thailand, the printed circuit board assembly (PCBA) will be produced in-house: SMT placement and reflow") {
		t.Errorf("template default missing when no operations supplied:\n%s", got)
	}
	if !strings.Contains(got, "test the PCBA"+templateAssumption+".") {
		t.Errorf("template operations not labelled as an assumption:\n%s", got)
	}
}

func TestRenderUnchangedGroupKeepsLimitedDetail(t *testing.T) {
	p, tm := movedPCBAProduct(t, true)
	got := Render(p, tm)
	if strings.Contains(got, "produced in-house") || strings.Contains(got, "assumed from") || strings.Contains(got, handSolder) {
		t.Errorf("unchanged group got processing detail:\n%s", got)
	}
}

// Final assembly follows the same rule as the other groups: supplied
// operations are stated as supplied; the template's are an assumption, and
// say so (the final reviewer's note on #290).
func TestRenderFinalAssemblySuppliedOperationsUnlabelled(t *testing.T) {
	p, tm := movedPCBAProduct(t, false)
	moved := Apply(p, domain.Move{{Group: "pcba", ToPlant: "th"}})
	got := Render(moved, tm, "pcba")
	// The fixture supplies final-assembly operations: stated as given.
	i := strings.Index(got, "Final assembly in ")
	if i < 0 {
		t.Fatalf("no final assembly paragraph:\n%s", got)
	}
	para := got[i:]
	if j := strings.Index(para, "\n\n"); j >= 0 {
		para = para[:j]
	}
	if strings.Contains(para, "assumed from") {
		t.Errorf("supplied final-assembly operations were labelled an assumption:\n%s", para)
	}
}

func TestRenderFinalAssemblyAbsentOperationsLabelled(t *testing.T) {
	p, tm := movedPCBAProduct(t, false)
	var kept []domain.Operation
	for _, o := range p.Operations {
		if o.Group != "final_assembly" {
			kept = append(kept, o)
		}
	}
	p.Operations = kept
	// Final assembly is known only from a component placed in that group, so
	// the paragraph is written from the template's steps.
	p.Components = append(append([]domain.Component(nil), p.Components...), domain.Component{Name: "outer casing", MadeIn: "Thailand", Plant: "th", Group: "final_assembly"})
	moved := Apply(p, domain.Move{{Group: "pcba", ToPlant: "th"}})
	got := Render(moved, tm, "pcba")
	i := strings.Index(got, "Final assembly in ")
	if i < 0 {
		t.Fatalf("no final assembly paragraph:\n%s", got)
	}
	para := got[i:]
	if j := strings.Index(para, "\n\n"); j >= 0 {
		para = para[:j]
	}
	if !strings.HasSuffix(strings.TrimSpace(para), templateAssumption+".") {
		t.Errorf("template final-assembly operations not labelled as an assumption:\n%s", para)
	}
}

func TestClassificationNoteStatesTheProductsCodes(t *testing.T) {
	p := domain.Product{Name: "speed sensor", HTS: "8543.70.98", Components: []domain.Component{
		{Name: "PCBA", MadeIn: "China", HTS: "8543.90"}, {Name: "screws", MadeIn: "Thailand"}, {Name: "housing", MadeIn: "Thailand", HTS: " 3926.90 "}}}
	got := ClassificationNote(p)
	want := "\nTariff classifications given with the product (HTSUS; the product's own codes, not verified by CBP): the speed sensor (the finished good), 8543.70.98; the PCBA (made in China), 8543.90; the housing (made in Thailand), 3926.90.\n"
	if got != want {
		t.Fatalf("got %q\nwant %q", got, want)
	}
	if ClassificationNote(domain.Product{Name: "x", Components: []domain.Component{{Name: "a", MadeIn: "China"}}}) != "" {
		t.Fatal("no code, no note")
	}
}
