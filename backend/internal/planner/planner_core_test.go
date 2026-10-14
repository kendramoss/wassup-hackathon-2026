package planner

import (
	"flag"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
)

var update = flag.Bool("update", false, "rewrite golden files")

func fixture(t *testing.T, name string) (Request, Template) {
	t.Helper()
	r, err := LoadRequest(filepath.Join("..", "..", "testdata", name))
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

func TestTemplatesLoad(t *testing.T) {
	tmpls, err := LoadTemplates()
	if err != nil {
		t.Fatal(err)
	}
	for _, fam := range []string{"motorised_tools_small_appliances", "lidar_modules"} {
		if _, ok := tmpls[fam]; !ok {
			t.Errorf("missing template %s", fam)
		}
	}
}

func TestEnumerateHeatGun(t *testing.T) {
	r, tm := fixture(t, "heatgun_2022.json")
	var keys []string
	for _, m := range Enumerate(r.Product, tm, r.Target) {
		keys = append(keys, m.Key())
	}
	// Heating element and housing are already Thai; final assembly is not movable.
	want := []string{
		"motor_fan_assembly@th", "pcba@th", "switch_cord_minor_parts@th",
		"motor_fan_assembly@th+pcba@th", "motor_fan_assembly@th+switch_cord_minor_parts@th", "pcba@th+switch_cord_minor_parts@th",
	}
	if !reflect.DeepEqual(keys, want) {
		t.Errorf("moves = %v\nwant    %v", keys, want)
	}
}

func TestEnumerateLidar(t *testing.T) {
	r, tm := fixture(t, "lidar_scenario1.json")
	var keys []string
	for _, m := range Enumerate(r.Product, tm, r.Target) {
		keys = append(keys, m.Key())
	}
	want := []string{"main_boards@vn", "rx_tx_boards@vn", "main_boards@vn+rx_tx_boards@vn"}
	if !reflect.DeepEqual(keys, want) {
		t.Errorf("moves = %v, want %v", keys, want)
	}
}

func TestApplyMovesComponentsAndOperations(t *testing.T) {
	r, tm := fixture(t, "heatgun_2022.json")
	m := domain.Move{{Group: "motor_fan_assembly", ToPlant: "th"}, {Group: "pcba", ToPlant: "th"}}
	v := Apply(r.Product, m)
	for _, c := range v.Components {
		switch c.Group {
		case "motor_fan_assembly", "pcba":
			if c.MadeIn != "Thailand" || c.Plant != "th" {
				t.Errorf("%s not moved: %+v", c.Name, c)
			}
		case "switch_cord_minor_parts":
			if c.MadeIn != "China" {
				t.Errorf("%s moved but should not be", c.Name)
			}
		}
	}
	if r.Product.Components[3].MadeIn != "China" {
		t.Error("Apply modified the original product")
	}
	if got := Describe(r.Product, tm, m); got != "Make Motor-fan assembly and PCBA (SMT) in Thailand" {
		t.Errorf("Describe = %q", got)
	}
}

func TestRenderGolden(t *testing.T) {
	r, tm := fixture(t, "heatgun_2022.json")
	for name, p := range map[string]domain.Product{
		"heatgun_2022":       r.Product,
		"heatgun_motor_pcba": Apply(r.Product, domain.Move{{Group: "motor_fan_assembly", ToPlant: "th"}, {Group: "pcba", ToPlant: "th"}}),
	} {
		got := Render(p, tm)
		if Render(p, tm) != got {
			t.Fatal("Render is not deterministic")
		}
		golden := filepath.Join("testdata", name+".golden")
		if *update {
			_ = os.MkdirAll("testdata", 0o755)
			if err := os.WriteFile(golden, []byte(got), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		want, err := os.ReadFile(golden)
		if err != nil {
			t.Fatalf("%v (run with -update)", err)
		}
		if got != string(want) {
			t.Errorf("%s render changed; run with -update if intended:\n%s", name, got)
		}
	}
	moved := Render(Apply(r.Product, domain.Move{{Group: "motor_fan_assembly", ToPlant: "th"}}), tm)
	if !strings.Contains(moved, "Motor-fan assembly (electric motor, fan and motor support), performed in Thailand: stamp rotor laminations") {
		t.Errorf("moved render missing motor operations:\n%s", moved)
	}
}

func TestDutyRateNormalises(t *testing.T) {
	e := Economics{DutyRates: map[string]float64{"PRC": 0.25, "Thailand": 0}}
	if r, ok := e.DutyRate("China"); !ok || r != 0.25 {
		t.Errorf("DutyRate(China) = %v %v", r, ok)
	}
	if _, ok := e.DutyRate("Vietnam"); ok {
		t.Error("missing rate should be ok=false")
	}
}
