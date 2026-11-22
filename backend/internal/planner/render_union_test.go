package planner

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
)

// Issue #392 / D264: families written from many rulings have group operations
// that are the union of different products' processes. A product that supplies
// no operations for a moved group must not inherit another product's parts.

func familyFixture(t *testing.T, name string) (Request, Template) {
	t.Helper()
	r, err := LoadRequest(filepath.Join("..", "..", "testdata", "families", name))
	if err != nil {
		t.Fatal(err)
	}
	tmpls, err := LoadTemplates()
	if err != nil {
		t.Fatal(err)
	}
	tm, ok := tmpls[r.Family]
	if !ok {
		t.Fatalf("no template %q", r.Family)
	}
	return r, tm
}

func TestRenderUnionTemplateDoesNotAssertOtherProducts(t *testing.T) {
	r, tm := familyFixture(t, "water_dispenser_2025.json")
	if !tm.UnionDefaultOperations {
		t.Fatalf("%s must declare union template defaults", tm.Family)
	}
	moved := Apply(r.Product, domain.Move{{Group: "dispenser_electronics", ToPlant: "kr"}})
	got := Render(moved, tm, "dispenser_electronics")
	for _, bad := range []string{"UV lamp", "RFID", "RO membrane", "carbon block or RO membrane", "forceps"} {
		if strings.Contains(got, bad) {
			t.Errorf("water-dispenser facts assert another product's part %q:\n%s", bad, got)
		}
	}
	want := "In South Korea, the transparent water tank and tank lid and water pumping unit with circuit board, battery and touch panel will be produced in-house" + templateAssumption + "."
	if !strings.Contains(got, want) {
		t.Errorf("neutral moved-group line missing:\n%s", got)
	}
}

func TestRenderUnionTemplateNeedleDoesNotAssertForceps(t *testing.T) {
	tmpls, err := LoadTemplates()
	if err != nil {
		t.Fatal(err)
	}
	tm := tmpls["medical_devices"]
	if !tm.UnionDefaultOperations {
		t.Fatal("medical_devices must declare union template defaults")
	}
	p := domain.Product{
		Name: "Hypodermic needle",
		Plants: []domain.Plant{
			{ID: "cn", Name: "China plant", Country: "China"},
			{ID: "vn", Name: "Vietnam plant", Country: "Vietnam"},
		},
		Components: []domain.Component{
			{Name: "stainless steel cannula", MadeIn: "China", Plant: "cn", Group: "metal_cannula_forging"},
			{Name: "plastic hub", MadeIn: "China", Plant: "cn", Group: "final_assembly"},
		},
		Operations: []domain.Operation{
			{Name: "glue the cannula into the hub, cap and pack", Plant: "cn", Group: "final_assembly"},
		},
	}
	move := domain.Move{{Group: "metal_cannula_forging", ToPlant: "vn"}}
	got := Render(Apply(p, move), tm, "metal_cannula_forging")
	for _, bad := range []string{"forceps", "laser-weld", "cold-draw"} {
		if strings.Contains(got, bad) {
			t.Errorf("needle facts assert another product's process %q:\n%s", bad, got)
		}
	}
	want := "In Vietnam, the stainless steel cannula will be produced in-house" + templateAssumption + "."
	if !strings.Contains(got, want) {
		t.Errorf("neutral moved-group line missing:\n%s", got)
	}

	// Supplied operations still win in a union family: they are stated as
	// facts with no assumption label and no template steps.
	withOps := p
	withOps.Operations = append([]domain.Operation(nil), p.Operations...)
	withOps.Operations = append(withOps.Operations, domain.Operation{Name: "roll and weld the cannula tubing", Plant: "cn", Group: "metal_cannula_forging"})
	got = Render(Apply(withOps, move), tm, "metal_cannula_forging")
	if !strings.Contains(got, "roll and weld the cannula tubing") || strings.Contains(got, templateAssumption) {
		t.Errorf("supplied operations lost to the union default:\n%s", got)
	}
}

// The three earlier templates keep their product-specific defaults; the
// ruling-grounded families declare union defaults. Pinning the set prevents a
// new family from silently inheriting the unsafe default.
func TestUnionDefaultFlagPinned(t *testing.T) {
	tmpls, err := LoadTemplates()
	if err != nil {
		t.Fatal(err)
	}
	union := map[string]bool{
		"cables_harnesses": true, "display_camera_modules": true, "furniture": true,
		"gas_grills_cooking": true, "hand_tools": true, "medical_devices": true,
		"motors_pumps_compressors": true, "pcba_electronic_devices": true,
		"power_supplies_chargers": true, "vehicle_parts": true, "water_filters": true,
		// The Part 102 families (D580) are written from many rulings too.
		"electrical_control_apparatus": true, "food_preparations": true, "machinery_parts": true,
		"metal_products": true, "plastic_articles": true, "wood_panels_flooring": true,
	}
	for _, family := range allFamilies {
		got := tmpls[family].UnionDefaultOperations
		if want := union[family]; got != want {
			t.Errorf("%s UnionDefaultOperations = %v, want %v", family, got, want)
		}
	}
}
