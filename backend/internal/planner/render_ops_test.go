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
}

func TestRenderUnchangedGroupKeepsLimitedDetail(t *testing.T) {
	p, tm := movedPCBAProduct(t, true)
	got := Render(p, tm)
	if strings.Contains(got, "produced in-house") || strings.Contains(got, handSolder) {
		t.Errorf("unchanged group got processing detail:\n%s", got)
	}
}
