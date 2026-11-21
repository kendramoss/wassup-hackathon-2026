package planner

import (
	"reflect"
	"testing"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
)

// A plan sentence names a group by its template name only when that name fits
// the parts filed under it (judge panel round 3, D270). The power bank's
// battery cells sit in the template's "Power conversion or charging module"
// group, so the plan names the cells.
func TestGroupPhraseNamesThePartsWhenTheTemplateNameDoesNotFit(t *testing.T) {
	tms, err := LoadTemplates()
	if err != nil {
		t.Fatal(err)
	}
	plants := []domain.Plant{{ID: "cn", Country: "China"}, {ID: "vn", Country: "Vietnam"}}
	comp := func(name, group string) domain.Component {
		return domain.Component{Name: name, MadeIn: "China", Plant: "cn", Group: group}
	}
	for _, c := range []struct {
		name   string
		family string
		comps  []domain.Component
		group  string
		want   string
	}{
		{"power bank cells", "power_supplies_chargers", []domain.Component{comp("Two cylindrical lithium-ion battery cells", "power_charging_modules")},
			"power_charging_modules", "Move the two cylindrical lithium-ion battery cells to Vietnam"},
		{"a real charging module keeps the name", "power_supplies_chargers", []domain.Component{comp("Inductive charging module", "power_charging_modules")},
			"power_charging_modules", "Move the power conversion or charging module to Vietnam"},
		{"PCBA matches printed circuit board", "power_supplies_chargers", []domain.Component{comp("Printed circuit board assembly", "pcba_manufacturing")},
			"pcba_manufacturing", "Move the PCBA (SMT) to Vietnam"},
		{"plural and singular match", "gas_grills_cooking", []domain.Component{comp("Deep draw main lids", "lid_and_firebox")},
			"lid_and_firebox", "Move the lid and firebox forming to Vietnam"},
		{"forging under housings", "vehicle_parts", []domain.Component{comp("Aluminum bar / ingot", "housings_and_structures"), comp("Forged circular aluminum blank", "housings_and_structures")},
			"housings_and_structures", "Move the aluminum bar / ingot and the forged circular aluminum blank to Vietnam"},
		{"many parts are counted", "medical_devices", []domain.Component{comp("Support", "hubs_clamps_housings"), comp("Fixing plate", "hubs_clamps_housings"), comp("Wires", "hubs_clamps_housings"), comp("Guide rail", "hubs_clamps_housings")},
			"hubs_clamps_housings", "Move the support, the fixing plate and 2 other parts to Vietnam"},
		{"a generic word is not a match", "power_supplies_chargers", []domain.Component{comp("Battery module", "power_charging_modules")},
			"power_charging_modules", "Move the battery module to Vietnam"},
	} {
		p := domain.Product{Plants: plants, Components: c.comps}
		got := Describe(p, tms[c.family], domain.Move{{Group: c.group, ToPlant: "vn"}})
		if got != c.want {
			t.Errorf("%s: %q, want %q", c.name, got, c.want)
		}
	}
}

// What moves lists parts, and a group's steps only when none of its parts
// move (D270): the gas grill listed 18 items, operation sentences included.
func TestMovedItemsListsPartsBeforeSteps(t *testing.T) {
	p := domain.Product{
		Plants: []domain.Plant{{ID: "cn", Country: "China"}, {ID: "id", Country: "Indonesia"}},
		Components: []domain.Component{
			{Name: "lids", MadeIn: "China", Plant: "cn", Group: "lid_and_firebox"},
			{Name: "burners", MadeIn: "Indonesia", Plant: "id", Group: "burners_and_gas_train"},
		},
		Operations: []domain.Operation{
			{Name: "deep draw the lids", Plant: "cn", Group: "lid_and_firebox"},
			{Name: "weld the gas train", Plant: "cn", Group: "burners_and_gas_train"},
		},
	}
	got := MovedItems(p, domain.Move{{Group: "lid_and_firebox", ToPlant: "id"}, {Group: "burners_and_gas_train", ToPlant: "id"}})
	want := []string{"lids", "weld the gas train"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}
