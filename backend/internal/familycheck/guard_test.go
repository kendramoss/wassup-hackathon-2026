package familycheck

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"os"
	"slices"
	"sort"
	"strings"
	"testing"
)

var updateGuardDev = flag.Bool("update-guard-dev", false, "rewrite eval/families/b2-guard-dev.json (D781 M2)")

// Unsupported and ambiguous products (#766-#769): a product that only shares
// a cue with a family loses the match; the family's own goods keep it.
func TestGuardSendsLookalikesToReview(t *testing.T) {
	for _, c := range []struct {
		family, typed, product string
		rule                   string // "" keeps the match
	}{
		// Batteries: a battery pack, cell, power bank or station keeps the
		// power supplies, chargers and battery packs template.
		{"power_supplies_chargers", "Lithium-ion battery pack for cordless drills", "", ""},
		{"power_supplies_chargers", "ESS LFP pouch cell", "", ""},
		{"power_supplies_chargers", "Power bank", "Portable power bank", ""},
		{"power_supplies_chargers", "Portable power station", "", ""},
		{"power_supplies_chargers", "Lithium-ion jump starter", "", ""},
		{"power_supplies_chargers", "Battery charger", "", ""},
		{"power_supplies_chargers", "Rechargeable batteries", "", ""},
		// Review B1: units, BMS boards, backups and locations are not the product word.
		{"power_supplies_chargers", "Lithium battery 12V 100Ah", "", ""},
		{"power_supplies_chargers", "LiFePO4 battery pack 48V", "", ""},
		{"power_supplies_chargers", "Rechargeable power bank 10000mAh", "", ""},
		{"power_supplies_chargers", "Rechargeable power bank 10000 mAh", "", ""},
		{"power_supplies_chargers", "Battery management board", "", ""},
		{"power_supplies_chargers", "Home UPS battery backup", "", ""},
		{"power_supplies_chargers", "We assemble lithium battery packs in Vietnam", "", ""},
		// Re-review of d61a1069: a clause or unit after the product word is
		// not the product (family-guard-v3).
		{"power_supplies_chargers", "Lithium battery made in Vietnam", "", ""},
		{"power_supplies_chargers", "Rechargeable batteries made in Thailand", "", ""},
		{"power_supplies_chargers", "Lithium batteries manufactured in China", "", ""},
		{"power_supplies_chargers", "Lithium battery packs assembled in Vietnam", "", ""},
		{"power_supplies_chargers", "Battery packs assembled from Chinese cells", "", ""},
		{"power_supplies_chargers", "Battery pack imported from China", "", ""},
		{"power_supplies_chargers", "Battery pack that we assemble in Mexico", "", ""},
		{"power_supplies_chargers", "Lithium battery packs we sell in the US", "", ""},
		{"power_supplies_chargers", "Lithium battery pack assembly", "", ""},
		{"power_supplies_chargers", "lithium battery pack and BMS assembly", "", ""},
		{"power_supplies_chargers", "Battery protection circuit", "", ""},
		{"power_supplies_chargers", "Lithium battery 12V 100Ah LFP", "", ""},
		{"power_supplies_chargers", "Battery pack 12V 7Ah SLA", "", ""},
		{"power_supplies_chargers", "Lithium battery 12 volt", "", ""},
		{"power_supplies_chargers", "Lithium battery 100 amp hour", "", ""},
		{"power_supplies_chargers", "Lithium battery 12V 100Ah deep cycle", "", ""},
		{"power_supplies_chargers", "48V 200Ah rack mount", "", ""},
		{"power_supplies_chargers", "Lithium battery storage cabinet", "", ""},
		{"power_supplies_chargers", "Cordless drill made in Vietnam", "", "battery-device"},
		// ... a device that runs on one does not.
		{"power_supplies_chargers", "Cordless drill", "", "battery-device"},
		{"power_supplies_chargers", "Battery-powered lawn mower", "", "battery-device"},
		{"power_supplies_chargers", "Rechargeable LED flashlight", "", "battery-device"},
		{"power_supplies_chargers", "", "Wireless earbuds with charging case", ""},
		{"power_supplies_chargers", "Earbuds", "Rechargeable wireless earbuds", "battery-device"},
		{"power_supplies_chargers", "E-bike", "", "vehicle"},
		{"power_supplies_chargers", "Battery-electric semi-truck", "", "battery-device"},
		{"power_supplies_chargers", "Single crystal cathode material", "", "battery-material"},
		{"power_supplies_chargers", "Artificial graphite anode active material", "", "battery-material"},
		// Machinery: machines keep the motors, pumps, compressors and
		// industrial machines template; hand tools and vehicles do not.
		{"motors_pumps_compressors", "Industrial robot assembly", "", ""},
		{"motors_pumps_compressors", "Electric injection moulding machine", "", ""},
		{"motors_pumps_compressors", "Vacuum pump", "", ""},
		{"motors_pumps_compressors", "Concrete mixer", "", ""},
		{"motors_pumps_compressors", "Cordless hammer drill", "", "hand-tool-or-appliance"},
		{"motors_pumps_compressors", "Electric scooter", "", "vehicle"},
		{"motors_pumps_compressors", "Hydraulic pallet truck", "", ""},
		{"motors_pumps_compressors", "Electric forklift", "", ""},
		{"motors_pumps_compressors", "Industrial blender", "", ""},
		{"motors_pumps_compressors", "Personal blender", "", "hand-tool-or-appliance"},
		{"motors_pumps_compressors", "Floor sander", "", ""},
		{"motors_pumps_compressors", "Pneumatic rock drill", "", ""},
		{"motors_pumps_compressors", "Ride-on lawn mower", "", ""},
		{"motors_pumps_compressors", "Walk-behind lawn mower", "", "hand-tool-or-appliance"},
		// Furniture: furniture keeps it; lamps, pet beds and electrical
		// cabinets do not.
		{"furniture", "Dining table with stone top", "", ""},
		{"furniture", "Lamp table", "", ""},
		{"furniture", "Bamboo shelves", "", ""},
		{"furniture", "Upholstered wood chairs", "", ""},
		{"furniture", "Floor lamp", "", "lighting"},
		{"furniture", "Desk lamps", "", "lighting"},
		{"furniture", "Pet bed", "", "pet-product"},
		{"furniture", "Orthopedic dog sofa", "", "pet-product"},
		{"furniture", "High voltage distribution cabinets", "", "electrical-cabinet"},
		{"furniture", "Kitchen cabinet", "", ""},
		{"furniture", "Pet-friendly sofa", "", ""},
		{"furniture", "Cat-proof leather recliner", "", ""},
		{"furniture", "Bed frame", "", ""},
		{"furniture", "Cat tree", "", "pet-product"},
		{"furniture", "Bed for dogs", "", "pet-product"},
		{"furniture", "Floor lamp made in China", "", "lighting"},
		{"furniture", "Pet bed made in Vietnam", "", "pet-product"},
		{"furniture", "Pet-friendly bed", "", ""},
		{"furniture", "Dog-proof sofa bed", "", ""},
		{"furniture", "Dining tables made in Vietnam", "", ""},
		// Medical and optical: devices and lenses keep their templates;
		// drugs and toys do not.
		{"medical_devices", "Hypodermic needles", "", ""},
		{"medical_devices", "Digital blood pressure monitor", "", ""},
		{"medical_devices", "Gel electrode pads", "", ""},
		// A dosage form needs drug context: a tablet can be a computer and
		// a capsule a camera (re-review of d61a1069).
		{"medical_devices", "Ibuprofen 200 mg tablets", "", "drug"},
		{"medical_devices", "Ibuprofen tablets", "", ""},
		{"medical_devices", "Endoscopy capsule", "", ""},
		{"medical_devices", "Medical tablet", "", ""},
		{"display_camera_modules", "Android tablet", "", ""},
		{"medical_devices", "Vitamin C tablets", "", "drug"},
		{"medical_devices", "Vitamin D3 gummy supplement", "", "drug"},
		{"medical_devices", "Toy stethoscope", "", "toy"},
		{"medical_devices", "Ultrasound gel", "", ""},
		{"medical_devices", "Children's digital thermometer", "", ""},
		{"medical_devices", "Kids' nebulizer", "", ""},
		{"display_camera_modules", "Riflescope lens", "", "standalone-optics"},
		{"display_camera_modules", "Toy binoculars", "", "toy"},
		{"offroad_vehicles_machinery", "Bike stand", "", "support-stand"},
		{"offroad_vehicles_machinery", "Utility task vehicle", "", ""},
		{"display_camera_modules", "Plug and play USB camera module", "", ""},
		{"display_camera_modules", "Display module for children's tablet", "", ""},
		{"display_camera_modules", "Novelty sunglasses", "", "toy"},
		// 3D printers (#778, D823): no longer an industrial or lifting
		// machine; the motors template keeps real machines.
		{"motors_pumps_compressors", "3D printer", "", "printer"},
		{"motors_pumps_compressors", "Dental 3D printer made in Mexico", "", "printer"},
		{"motors_pumps_compressors", "", "SprintRay Pro 2 3D printer", "printer"},
		{"motors_pumps_compressors", "Laser marking and engraving machine", "", ""},
		{"motors_pumps_compressors", "Printer cartridge filling machine", "", ""},
		{"motors_pumps_compressors", "Industrial 3D printing machine", "", "printer"},
		{"motors_pumps_compressors", "Additive manufacturing system", "", "printer"},
		{"motors_pumps_compressors", "Bike stand", "", ""},
		{"offroad_vehicles_machinery", "3D printer", "", "printer"},
		// Off-road, lifting and mobile machinery (#778, D826): jacks, lifts, winches,
		// cranes and guided vehicles keep it; stands, dollies, vehicles and
		// toys do not.
		{"offroad_vehicles_machinery", "Hydraulic floor jack", "", ""},
		{"offroad_vehicles_machinery", "Hydraulic bottle jacks made in Thailand", "", ""},
		{"offroad_vehicles_machinery", "Scissor jack", "", ""},
		{"offroad_vehicles_machinery", "Electric scissor lift", "", ""},
		{"offroad_vehicles_machinery", "Electric winch 8000 lb", "", ""},
		{"offroad_vehicles_machinery", "Engine crane", "", ""},
		{"offroad_vehicles_machinery", "Automated guided vehicle with lifting mechanism", "", ""},
		{"offroad_vehicles_machinery", "3D printing machine", "", "printer"},
		{"offroad_vehicles_machinery", "AGV for battery plants", "", ""},
		{"offroad_vehicles_machinery", "Telehandler", "", ""},
		{"offroad_vehicles_machinery", "Hydraulic pallet truck", "", ""},
		{"offroad_vehicles_machinery", "Jack stand", "", "support-stand"},
		{"offroad_vehicles_machinery", "Automotive jack stands", "", "support-stand"},
		{"offroad_vehicles_machinery", "Engine stand", "", "support-stand"},
		{"offroad_vehicles_machinery", "Wheel dolly", "", "support-stand"},
		{"offroad_vehicles_machinery", "Toy crane", "", "toy-machine"},
		{"motors_pumps_compressors", "Automated guided vehicle", "", ""},
		// Optical and measuring instruments (#780, D822): lenses, sights and
		// instruments keep it; lights, network optics, smart glasses, toys and
		// drugs do not.
		{"optical_precision_instruments", "Riflescope lens", "", ""},
		{"optical_precision_instruments", "Optical windows", "", ""},
		{"optical_precision_instruments", "Carrier plate assembly for red dot sights", "", ""},
		{"optical_precision_instruments", "Binoculars", "", ""},
		{"optical_precision_instruments", "Digital multimeter", "", ""},
		{"optical_precision_instruments", "Pressure sensors made in Germany", "", ""},
		{"optical_precision_instruments", "Gas chromatograph", "", ""},
		{"optical_precision_instruments", "Digital tape measure", "", ""},
		{"optical_precision_instruments", "Laser level", "", ""},
		{"optical_precision_instruments", "Light meter", "", ""},
		{"optical_precision_instruments", "Wireless cooking thermometer", "", ""},
		{"optical_precision_instruments", "Fiber optic cable tester", "", ""},
		{"optical_precision_instruments", "Reading glasses", "", ""},
		{"optical_precision_instruments", "LED flashlight", "", "lighting-optics"},
		{"optical_precision_instruments", "Work lamp", "", "lighting-optics"},
		{"optical_precision_instruments", "Fiber optic cable assemblies", "", "network-optics"},
		{"optical_precision_instruments", "Optical transceiver module", "", "network-optics"},
		{"optical_precision_instruments", "Patch cord assemblies", "", "network-optics"},
		{"optical_precision_instruments", "Smart glasses", "", "smart-eyewear"},
		{"optical_precision_instruments", "Toy binoculars", "", "toy"},
		{"optical_precision_instruments", "Novelty sunglasses", "", "toy"},
		{"optical_precision_instruments", "Vitamin C tablets", "", "drug"},
		{"optical_precision_instruments", "Riflescope lenses", "", ""},
		{"optical_precision_instruments", "Camera module with lens", "", "camera-display-module"},
		{"optical_precision_instruments", "LCD display module", "", "camera-display-module"},
		// Lenses split by where they end up (D826): standalone optics go to the
		// optical template, lenses in a camera or display module to display.
		{"display_camera_modules", "Lens assemblies", "", "standalone-optics"},
		{"display_camera_modules", "Optical windows", "", "standalone-optics"},
		{"display_camera_modules", "Camera lens assembly", "", ""},
		{"display_camera_modules", "Lens module for a dashcam", "", ""},
		// Rules belong to their families: the same words elsewhere do nothing.
		{"led_lights", "Floor lamp", "", ""},
		{"motorised_tools_small_appliances", "Cordless drill", "", ""},
		{"vehicle_parts", "E-bike battery", "", ""},
		{"hand_tools", "Jack stand", "", ""},
		{"display_camera_modules", "Fiber optic cable", "", ""},
		{"furniture", "LED flashlight", "", "lighting"},
	} {
		text := "Parts: housing"
		if c.typed != "" {
			text = "Typed: " + c.typed + "\n" + text
		}
		in := Result{Family: c.family, Name: "x", Confidence: 0.9, Reason: "model", ProductName: c.product}
		got, rule := Guard(text, in)
		if rule != c.rule {
			t.Errorf("%s / %q / %q: rule %q, want %q", c.family, c.typed, c.product, rule, c.rule)
			continue
		}
		if rule == "" && got != in {
			t.Errorf("%q: result changed with no rule: %+v", c.typed, got)
		}
		if rule != "" && (got.Family != None || got.Name != "" || got.Confidence != 0 || got.GuardRule != rule ||
			got.ModelFamily != c.family || got.ModelReason != "model" || !strings.HasPrefix(got.Reason, "Not planned as x: ") ||
			strings.Contains(got.Reason, GuardVersion) || strings.Contains(got.Reason, "rule ") || strings.Contains(got.Reason, "_")) {
			t.Errorf("%q: guarded result %+v", c.typed, got)
		}
	}
}

func TestGuardKeepsNone(t *testing.T) {
	in := Result{Family: None, Reason: "nothing fits"}
	if got, rule := Guard("Typed: Floor lamp", in); rule != "" || got != in {
		t.Errorf("none = %+v, %q", got, rule)
	}
}

func TestCheckAppliesTheGuard(t *testing.T) {
	c, _ := checker(t, `{"family":"furniture","confidence":0.9,"reason":"Has a base and a shade.","product_name":"Floor lamp"}`)
	r, err := c.Check(context.Background(), "Typed: floor lamp")
	if err != nil {
		t.Fatal(err)
	}
	if r.Family != None || r.Name != "" || r.Confidence != 0 || r.GuardRule != "lighting" || r.ModelFamily != "furniture" ||
		r.Reason != "Not planned as Furniture and mattresses: a lamp or light fitting is lighting, not furniture." {
		t.Errorf("result = %+v", r)
	}
	b, _ := json.Marshal(r)
	if strings.Contains(string(b), "furniture\"") || strings.Contains(string(b), "lighting\"") {
		t.Errorf("the guard's log fields reached the JSON: %s", b)
	}
}

// Review B1: an import with no product name sends only "Parts: ..."; the
// guard reads no parts list, so a battery pack's parts never take its match.
func TestGuardIgnoresAPartsList(t *testing.T) {
	in := Result{Family: "power_supplies_chargers", Name: "x", Confidence: 0.9, Reason: "model"}
	if got, rule := Guard("Parts: housing, lithium cells, BMS board", in); rule != "" || got != in {
		t.Errorf("parts only = %+v, %q", got, rule)
	}
	// The model's product name is still read.
	in.ProductName = "Cordless drill"
	if _, rule := Guard("Parts: motor, lithium cells, chuck", in); rule != "battery-device" {
		t.Errorf("product name rule = %q", rule)
	}
}

func TestProductLinesAndHeadNoun(t *testing.T) {
	for in, want := range map[string][]string{
		"Typed: cordless drill\nProduct name: Drill\nParts: motor": {"cordless drill", "Drill"},
		"Product name: Floor lamp\nParts: base":                    {"Floor lamp"},
		"Product: Country of origin of a lamp\n\nX":                {"Country of origin of a lamp"},
		"\n  Power bank  \nmore":                                   nil,
		"Parts: housing, lithium cells, BMS board":                 nil,
	} {
		if got := ProductLines(in); !slices.Equal(got, want) {
			t.Errorf("ProductLines(%q) = %q, want %q", in, got, want)
		}
	}
	for in, want := range map[string]string{
		"cordless drill with two batteries": "drill",
		"floor lamps":                       "lamp",
		"rechargeable batteries":            "battery",
		"benches":                           "bench",
		"glass":                             "glass",
		"power bank (10,000 mah)":           "bank",
		"set of dining chairs":              "chair",
		"the country of origin and marking of battery chargers": "charger",
		"lithium battery 12v 100ah":                             "battery",
		"lifepo4 battery pack 48v":                              "pack",
		"rechargeable power bank 10000mah":                      "bank",
		"we assemble lithium battery packs in vietnam":          "pack",
		"12v": "",
	} {
		if got := headNoun(in); got != want {
			t.Errorf("headNoun(%q) = %q, want %q", in, got, want)
		}
	}
}

// D781 M2: the guard on the 469 recorded development family checks with 26
// templates, the subject as the product line. The changed rows are pinned in
// eval/families/b2-guard-dev.json; -update-guard-dev rewrites it.
func TestGuardOnDevelopmentRows(t *testing.T) {
	subjects := map[string]string{}
	lf, err := os.Open("../../data/labels.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	defer lf.Close()
	sc := bufio.NewScanner(lf)
	sc.Buffer(make([]byte, 1<<20), 1<<24)
	for sc.Scan() {
		var l struct{ Ruling, Subject string }
		if err := json.Unmarshal(sc.Bytes(), &l); err != nil {
			t.Fatal(err)
		}
		subjects[l.Ruling] = l.Subject
	}
	f, err := os.Open("../../../eval/results/families-coverage-dev-all-litellm-f26.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	type changed struct {
		Ruling  string `json:"ruling"`
		Subject string `json:"subject"`
		Family  string `json:"family"`
		Rule    string `json:"rule"`
	}
	var rows, matched int
	var got []changed
	sc = bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<24)
	for sc.Scan() {
		var row struct {
			Ruling, Family, Error string
			Confidence            float64
		}
		if err := json.Unmarshal(sc.Bytes(), &row); err != nil {
			t.Fatal(err)
		}
		rows++
		if row.Error != "" || row.Family == "" || row.Family == None {
			continue
		}
		matched++
		subj, ok := subjects[row.Ruling]
		if !ok {
			t.Fatalf("%s: no label", row.Ruling)
		}
		if _, rule := Guard("Product: "+subj, Result{Family: row.Family, Confidence: row.Confidence}); rule != "" {
			got = append(got, changed{row.Ruling, subj, row.Family, rule})
		}
	}
	if rows != 469 {
		t.Fatalf("%d development rows, want 469", rows)
	}
	sort.Slice(got, func(i, j int) bool { return got[i].Ruling < got[j].Ruling })
	out := map[string]any{
		"note": "D781 M2: rows of families-coverage-dev-all-litellm-f26.jsonl whose recorded family the " + GuardVersion +
			" guard takes away, the subject as the product line. Written by TestGuardOnDevelopmentRows -update-guard-dev.",
		"rows": rows, "matched": matched, "changed": got, "rules": GuardRules(),
	}
	b, _ := json.MarshalIndent(out, "", " ")
	b = append(b, '\n')
	const path = "../../../eval/families/b2-guard-dev.json"
	if *updateGuardDev {
		if err := os.WriteFile(path, b, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(want) != string(b) {
		t.Errorf("%s is stale; run go test ./internal/familycheck -run TestGuardOnDevelopmentRows -update-guard-dev", path)
	}
	if !slices.Contains(GuardRules(), "battery-device") {
		t.Error("rule list changed")
	}
}

var updateGuardG3 = flag.Bool("update-guard-g3", false, "rewrite eval/families/b3-guard-g3.json (D820 M2)")

// D820 M2: the current guard on the recorded #772 run with guard v3 and the
// version 2 templates (families-coverage-dev-all-litellm-f26-b2-g3.jsonl),
// the subject as the product line: the 3D printer (H346536) that run matched
// to the machines template is taken away. No model is re-run.
func TestGuardOnG3Rows(t *testing.T) {
	subjects := map[string]string{}
	lf, err := os.Open("../../data/labels.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	defer lf.Close()
	sc := bufio.NewScanner(lf)
	sc.Buffer(make([]byte, 1<<20), 1<<24)
	for sc.Scan() {
		var l struct{ Ruling, Subject string }
		if err := json.Unmarshal(sc.Bytes(), &l); err != nil {
			t.Fatal(err)
		}
		subjects[l.Ruling] = l.Subject
	}
	f, err := os.Open("../../../eval/results/families-coverage-dev-all-litellm-f26-b2-g3.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	type changed struct {
		Ruling  string `json:"ruling"`
		Subject string `json:"subject"`
		Family  string `json:"family"`
		Rule    string `json:"rule"`
	}
	var rows, matched int
	got := []changed{}
	sc = bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<24)
	for sc.Scan() {
		var row struct{ Ruling, Family, Error string }
		if err := json.Unmarshal(sc.Bytes(), &row); err != nil {
			t.Fatal(err)
		}
		rows++
		if row.Error != "" || row.Family == "" || row.Family == None {
			continue
		}
		matched++
		if _, rule := Guard("Product: "+subjects[row.Ruling], Result{Family: row.Family, Confidence: 1}); rule != "" {
			got = append(got, changed{row.Ruling, subjects[row.Ruling], row.Family, rule})
		}
	}
	sort.Slice(got, func(i, j int) bool { return got[i].Ruling < got[j].Ruling })
	b, _ := json.MarshalIndent(map[string]any{
		"note": "D820 M2: rows of families-coverage-dev-all-litellm-f26-b2-g3.jsonl (guard v3 applied during that run) whose recorded family the " +
			GuardVersion + " guard takes away, the subject as the product line. Written by TestGuardOnG3Rows -update-guard-g3.",
		"rows": rows, "matched": matched, "changed": got,
	}, "", " ")
	b = append(b, '\n')
	const path = "../../../eval/families/b3-guard-g3.json"
	if *updateGuardG3 {
		if err := os.WriteFile(path, b, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(want) != string(b) {
		t.Errorf("%s is stale; run go test ./internal/familycheck -run TestGuardOnG3Rows -update-guard-g3", path)
	}
}
