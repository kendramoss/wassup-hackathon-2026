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

func TestEnumerateUpToAddsTheHeatGunTriple(t *testing.T) {
	r, tm := fixture(t, "heatgun_2022.json")
	var keys []string
	for _, m := range EnumerateUpTo(r.Product, tm, r.Target, r.Economics.GroupCosts, DefaultMaxMoves) {
		keys = append(keys, m.Key())
	}
	// The six singles and pairs keep their order (and so their cached
	// verdicts); CBP's real 2026 change, all three groups, comes last.
	want := []string{
		"motor_fan_assembly@th", "pcba@th", "switch_cord_minor_parts@th",
		"motor_fan_assembly@th+pcba@th", "motor_fan_assembly@th+switch_cord_minor_parts@th", "pcba@th+switch_cord_minor_parts@th",
		"motor_fan_assembly@th+pcba@th+switch_cord_minor_parts@th",
	}
	if !reflect.DeepEqual(keys, want) {
		t.Errorf("moves = %v\nwant    %v", keys, want)
	}
	m := EnumerateUpTo(r.Product, tm, r.Target, r.Economics.GroupCosts, DefaultMaxMoves)[6]
	if got := Describe(r.Product, tm, m); got != "Make Motor-fan assembly, PCBA (SMT) and Switch, cord and minor parts in Thailand" {
		t.Errorf("describe = %q", got)
	}
	if err := m.Validate(); err != nil {
		t.Error(err)
	}
	if n := len(EnumerateUpTo(r.Product, tm, r.Target, r.Economics.GroupCosts, 6)); n != 6 {
		t.Errorf("a cap of 6 should add no triple, got %d moves", n)
	}
	if n := len(EnumerateUpTo(r.Product, tm, r.Target, r.Economics.GroupCosts, 0)); n != 6 {
		t.Errorf("cap 0 should add no triple, got %d moves", n)
	}
}

func TestEnumerateUpToCapsAndPrefersCostlyGroups(t *testing.T) {
	// Five movable groups: 5 singles + 10 pairs = 15 moves already, so a cap
	// of 12 adds nothing and a cap of 18 adds the 3 costliest of 10 triples.
	tm := Template{Family: "f", Groups: []TemplateGroup{}}
	p := domain.Product{Name: "x", Plants: []domain.Plant{{ID: "cn", Country: "China"}, {ID: "th", Country: "Thailand"}}}
	costs := map[string]map[string]float64{}
	for i, id := range []string{"a", "b", "c", "d", "e"} {
		tm.Groups = append(tm.Groups, TemplateGroup{OperationGroup: domain.OperationGroup{ID: id, Name: strings.ToUpper(id), Operations: []string{"op " + id}}, Movable: true})
		p.Plants[1].Capabilities = append(p.Plants[1].Capabilities, id)
		p.Operations = append(p.Operations, domain.Operation{Name: "op " + id, Plant: "cn", Group: id})
		costs[id] = map[string]float64{"cn": float64(10 * (i + 1))} // e costs most, a least
	}
	if n := len(EnumerateUpTo(p, tm, "Thailand", costs, 12)); n != 15 {
		t.Errorf("cap below the pairs: %d moves, want 15 (singles and pairs are never cut)", n)
	}
	ms := EnumerateUpTo(p, tm, "Thailand", costs, 18)
	if len(ms) != 18 {
		t.Fatalf("moves = %d, want 18", len(ms))
	}
	var triples []string
	for _, m := range ms[15:] {
		triples = append(triples, m.Key())
	}
	want := []string{"c@th+d@th+e@th", "b@th+d@th+e@th", "a@th+d@th+e@th"}
	if !reflect.DeepEqual(triples, want) {
		t.Errorf("triples = %v, want %v (costliest first)", triples, want)
	}
	again := EnumerateUpTo(p, tm, "Thailand", costs, 18)
	if !reflect.DeepEqual(ms, again) {
		t.Error("not deterministic")
	}
}

func TestEnumerateUpToOtherFamilies(t *testing.T) {
	r, tm := fixture(t, "lidar_scenario1.json")
	if got, want := len(EnumerateUpTo(r.Product, tm, r.Target, r.Economics.GroupCosts, DefaultMaxMoves)), len(Enumerate(r.Product, tm, r.Target)); got != want {
		t.Errorf("LiDAR has two movable groups, so no triple: %d vs %d", got, want)
	}
	r, tm = fixture(t, "tublight_2025.json")
	ms := EnumerateUpTo(r.Product, tm, r.Target, r.Economics.GroupCosts, DefaultMaxMoves)
	base := Enumerate(r.Product, tm, r.Target)
	if len(ms) != len(base)+1 || !reflect.DeepEqual(ms[:len(base)], base) || ms[len(base)].Key() != "led_pcba@th+lens@th+base_harness_seals@th" {
		t.Errorf("tub light: %d moves, last %v", len(ms), ms[len(ms)-1].Key())
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
		detail := []string{}
		if name == "heatgun_motor_pcba" {
			detail = []string{"motor_fan_assembly", "pcba"}
		}
		got := Render(p, tm, detail...)
		if Render(p, tm, detail...) != got {
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
	moved := Render(Apply(r.Product, domain.Move{{Group: "motor_fan_assembly", ToPlant: "th"}}), tm, "motor_fan_assembly")
	if !strings.Contains(moved, "In Thailand, the electric motor, fan and motor support will be produced in-house: stamp rotor laminations") {
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
