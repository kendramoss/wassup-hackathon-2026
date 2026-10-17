package bom

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/planner"
)

// heatGunSample parses backend/testdata/heatgun_bom.csv after edit changes
// its text (edit nil: as is). Line endings are normalised to \n first (the
// file has CRLF).
func heatGunSample(t *testing.T, edit func(*testing.T, string) string) domain.Product {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "testdata", "heatgun_bom.csv"))
	if err != nil {
		t.Fatal(err)
	}
	s := strings.ReplaceAll(string(b), "\r\n", "\n")
	if edit != nil {
		s = edit(t, s)
	}
	rows, err := ReadRows(strings.NewReader(s))
	if err != nil {
		t.Fatal(err)
	}
	p, _, err := Parse(rows, Options{Name: "Kobalt heat gun", Family: "motorised_tools_small_appliances", Template: template(t)})
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// fixtureProduct is the heat-gun case's stored product (plant IDs th, cn).
func fixtureProduct(t *testing.T) domain.Product {
	t.Helper()
	r, err := planner.LoadRequest(filepath.Join("..", "..", "testdata", "heatgun_2022.json"))
	if err != nil {
		t.Fatal(err)
	}
	return r.Product
}

func replaceOnce(from, to string) func(*testing.T, string) string {
	return func(t *testing.T, s string) string {
		t.Helper()
		if !strings.Contains(s, from) {
			t.Fatalf("sample BOM has no %q", from)
		}
		return strings.Replace(s, from, to, 1)
	}
}

func TestDiffHeatGunVersions(t *testing.T) {
	tests := []struct {
		name string
		edit func(*testing.T, string) string
		want []string // "kind: name" prefixes, in order
	}{
		{"same BOM", nil, nil},
		{"motor made in Thailand", replaceOnce("electric motor,China,China suppliers", "electric motor,Thailand,Rayong plant"), []string{"country_changed: electric motor"}},
		{"motor from another Chinese plant", replaceOnce("electric motor,China,China suppliers", "electric motor,China,Ningbo motor works"), []string{"plant_changed: electric motor"}},
		{"case and spacing in names are ignored", replaceOnce("component,fan,China", "component,  FAN ,china"), nil},
		{"component dropped and added", func(t *testing.T, s string) string {
			s = replaceOnce("component,knob,China,China suppliers,switch_cord_minor_parts,\n", "")(t, s)
			return s + "component,thermal fuse,Japan,Japan suppliers,switch_cord_minor_parts,\n"
		}, []string{"component_removed: knob", "component_added: thermal fuse"}},
		{"final assembly moved to Vietnam", func(_ *testing.T, s string) string {
			return strings.ReplaceAll(s, ",Thailand,Rayong plant,final_assembly,", ",Vietnam,Hanoi plant,final_assembly,")
		}, []string{"operation_moved: press fit", "operation_moved: crimp plugs", "operation_moved: fasten the PCBA", "operation_moved: secure the power cord"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// The stored product (plant IDs th, cn) against an upload (IDs
			// from plant names): only real changes are reported.
			got := Diff(fixtureProduct(t), heatGunSample(t, tt.edit))
			if len(got) != len(tt.want) {
				t.Fatalf("changes = %+v, want %v", got, tt.want)
			}
			for i, c := range got {
				if s := c.Kind + ": " + c.Name; !strings.HasPrefix(s, tt.want[i]) || c.Summary == "" {
					t.Errorf("change %d = %+v, want %s", i, c, tt.want[i])
				}
			}
		})
	}
	got := Diff(fixtureProduct(t), heatGunSample(t, replaceOnce("electric motor,China,China suppliers", "electric motor,Thailand,Rayong plant")))
	want := Change{Kind: CountryChanged, Name: "electric motor", FromCountry: "China", ToCountry: "Thailand", FromPlant: "China suppliers", ToPlant: "Rayong plant",
		Summary: "electric motor: made in Thailand, was China (plant Rayong plant, was China suppliers)"}
	if len(got) != 1 || got[0] != want {
		t.Errorf("motor change = %+v\nwant %+v", got, want)
	}
	moved := Diff(fixtureProduct(t), heatGunSample(t, tests[5].edit))[0]
	if moved.FromCountry != "Thailand" || moved.ToCountry != "Vietnam" || moved.ToPlant != "Hanoi plant" {
		t.Errorf("operation move = %+v", moved)
	}
}

func TestDiffRepeatedNamesPairInOrder(t *testing.T) {
	p := func(countries ...string) domain.Product {
		out := domain.Product{Name: "x"}
		for _, c := range countries {
			out.Components = append(out.Components, domain.Component{Name: "screw", MadeIn: c})
		}
		return out
	}
	if got := Diff(p("China", "Vietnam"), p("China", "Vietnam")); len(got) != 0 {
		t.Errorf("same repeated parts: %+v", got)
	}
	got := Diff(p("China", "Vietnam"), p("China"))
	if len(got) != 1 || got[0].Kind != ComponentRemoved || got[0].FromCountry != "Vietnam" {
		t.Errorf("second screw dropped: %+v", got)
	}
}

func TestKeepPlantIDs(t *testing.T) {
	old := fixtureProduct(t)
	p := KeepPlantIDs(old, heatGunSample(t, func(_ *testing.T, s string) string {
		return s + "plant,Hanoi plant,Vietnam,,,pcba\n"
	}))
	var ids []string
	for _, pl := range p.Plants {
		ids = append(ids, pl.ID)
	}
	if strings.Join(ids, ",") != "th,cn,hanoi_plant" {
		t.Errorf("plant IDs = %v, want th,cn,hanoi_plant", ids)
	}
	if err := p.Validate(); err != nil {
		t.Errorf("renamed product invalid: %v", err)
	}
	if p.Components[3].Plant != "cn" || p.Operations[0].Plant != "th" {
		t.Errorf("references not renamed: %+v %+v", p.Components[3], p.Operations[0])
	}
	if d := Diff(old, p); len(d) != 0 {
		t.Errorf("a plant that makes nothing is not an origin change: %+v", d)
	}
}
