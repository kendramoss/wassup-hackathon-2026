package clarify

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/planner"
)

func templates(t *testing.T) map[string]planner.Template {
	t.Helper()
	tm, err := planner.LoadTemplates()
	if err != nil {
		t.Fatal(err)
	}
	return tm
}

func ids(in FamilyIntake, state string) []string {
	var out []string
	for _, q := range in.Questions {
		if state == "" || q.State == state {
			out = append(out, q.ID)
		}
	}
	return out
}

func yes(b bool) *bool { return &b }

// writtenFor is the template version each family's questions were written
// for: the #772 families at version 2 (D810; display 3, D826), off-road
// and lifting machinery at 2 (D826), optical instruments at 1 (D822).
var writtenFor = map[string]string{
	"power_supplies_chargers": "2", "motors_pumps_compressors": "2", "furniture": "2", "medical_devices": "2",
	"display_camera_modules": "3", "offroad_vehicles_machinery": "2", "optical_precision_instruments": "1",
}

// Each question asks about one template group, names a document, and
// shows exactly that group's citations: a ruling is "decided origin" or
// "not enough" for a question only as the template cites it for that
// group (D817).
func TestFamilyQuestionsAreGroundedInTheirTemplates(t *testing.T) {
	tms := templates(t)
	for fam, qs := range familyQuestions {
		tm, ok := tms[fam]
		if !ok {
			t.Fatalf("%s: no template", fam)
		}
		if want := writtenFor[fam]; tm.Version != want {
			t.Errorf("%s: questions were written for version %s of the template, got %s", fam, want, tm.Version)
		}
		cited := 0
		for _, q := range qs {
			if q.evidence == "" || q.text == "" || (q.kind != KindCountry && q.kind != KindYesNo) {
				t.Errorf("%s/%s: incomplete", fam, q.id)
			}
			if len(q.groups) != 1 {
				t.Errorf("%s/%s: asks about %d groups, want one", fam, q.id, len(q.groups))
				continue
			}
			var g *planner.TemplateGroup
			for i := range tm.Groups {
				if tm.Groups[i].ID == q.groups[0] {
					g = &tm.Groups[i]
				}
			}
			if g == nil {
				t.Errorf("%s/%s: group %s is not in the template", fam, q.id, q.groups[0])
				continue
			}
			var conf, ne []string
			if g.Precedent != nil {
				conf, ne = g.Precedent.ConfersOrigin, g.Precedent.NotEnough
			}
			cited += len(conf) + len(ne)
			// The question as asked carries exactly the group's citations.
			asked := false
			for _, x := range Intake(FamilyCase{US: true, Family: fam, Facts: "x", Product: &domain.Product{Name: "Product", Description: q.text + map[string]string{"motors_pumps_compressors": " (a pump)",
				"offroad_vehicles_machinery": " (a floor jack)", "optical_precision_instruments": " (a digital pressure gauge with a lens)"}[fam]}}, tms).Questions {
				if x.ID != q.id {
					continue
				}
				asked = true
				if !slices.Equal(x.ConfersOrigin, append([]string{}, conf...)) || !slices.Equal(x.NotEnough, append([]string{}, ne...)) {
					t.Errorf("%s/%s: shows %v / %v, the group cites %v / %v", fam, q.id, x.ConfersOrigin, x.NotEnough, conf, ne)
				}
			}
			if !asked {
				t.Errorf("%s/%s: not asked for a product described by its own text", fam, q.id)
			}
		}
		if cited == 0 {
			t.Errorf("%s: no question rests on a cited ruling", fam)
		}
	}
	// The pack question: welding cells and fitting the BMS is final assembly,
	// which N347193 held not enough; formation is a cell step, not asked here.
	for _, q := range familyQuestions["power_supplies_chargers"] {
		if q.id == "pack_work" && (strings.Contains(strings.ToLower(q.text), "formation") || q.groups[0] != "final_assembly") {
			t.Errorf("pack_work: %+v", q)
		}
	}
}

func batteryPack() *domain.Product {
	return &domain.Product{Name: "Lithium battery pack for e-bikes", Plants: []domain.Plant{{ID: "mx", Country: "Mexico"}},
		Components: []domain.Component{{Name: "18650 cells", Group: "battery_cells"}, {Name: "BMS board", Group: "pcba_manufacturing"}, {Name: "case", MadeIn: "Mexico", Group: "housing_cables_parts"}},
		Operations: []domain.Operation{{Name: "assemble the pack", Plant: "mx", Group: "final_assembly"}}}
}

func TestBatteryPackAsksCellsPackWorkAndBMS(t *testing.T) {
	in := Intake(FamilyCase{US: true, Family: "power_supplies_chargers", Product: batteryPack()}, templates(t))
	if in.Status != FamilyMissing || !slices.Equal(ids(in, StateMissing), []string{"cells_made", "pack_work", "bms_made"}) {
		t.Fatalf("%+v", in)
	}
	q := in.Questions[0]
	if !strings.Contains(q.Evidence, "certificate of origin") || !slices.Contains(q.ConfersOrigin, "N347193") || !slices.Contains(q.NotEnough, "N348871") {
		t.Errorf("cells question: %+v", q)
	}
	// A plain charger is not asked about cells.
	charger := &domain.Product{Name: "USB-C wall charger", Components: []domain.Component{{Name: "PCBA"}}}
	in = Intake(FamilyCase{US: true, Family: "power_supplies_chargers", Product: charger}, templates(t))
	if got := ids(in, ""); !slices.Equal(got, []string{"pcba_made", "module_made"}) {
		t.Errorf("charger: %v", got)
	}
}

// The parts list needs a "made in" for every part, so "Unknown" is how a
// requester says they do not know: still missing.
func TestUnknownOnThePartsListIsStillAsked(t *testing.T) {
	p := batteryPack()
	p.Components[0].MadeIn, p.Components[1].MadeIn = "Unknown", "TBD"
	in := Intake(FamilyCase{US: true, Family: "power_supplies_chargers", Product: p}, templates(t))
	if got := ids(in, StateMissing); !slices.Equal(got, []string{"cells_made", "pack_work", "bms_made"}) {
		t.Errorf("%v", got)
	}
}

// One cells line marked China and one blank: still asked, with China shown.
func TestPartlyKnownIsStillAsked(t *testing.T) {
	p := batteryPack()
	p.Components[0].MadeIn = "China"
	p.Components = append(p.Components, domain.Component{Name: "spare cells", Group: "battery_cells"})
	in := Intake(FamilyCase{US: true, Family: "power_supplies_chargers", Product: p}, templates(t))
	q := in.Questions[0]
	if q.ID != "cells_made" || q.State != StateMissing || !strings.Contains(q.Known, "China") || !strings.Contains(q.Known, "1 of 2 lines") {
		t.Fatalf("%+v", q)
	}
	if err := ValidateFamilyAnswers(in, map[string]domain.FamilyAnswer{"cells_made": {Country: "Vietnam"}}); err != nil {
		t.Errorf("answer refused: %v", err)
	}
	// An operation at a plant with no country also keeps it open.
	p = batteryPack()
	p.Components[0].MadeIn = "China"
	p.Plants = append(p.Plants, domain.Plant{ID: "x", Country: "Unknown"})
	p.Operations = append(p.Operations, domain.Operation{Name: "stack cells", Plant: "x", Group: "battery_cells"})
	if q := Intake(FamilyCase{US: true, Family: "power_supplies_chargers", Product: p}, templates(t)).Questions[0]; q.State != StateMissing {
		t.Errorf("op without a country: %+v", q)
	}
}

func TestKnownFromThePartsListIsNotAsked(t *testing.T) {
	p := batteryPack()
	p.Components[0].MadeIn = "South Korea"
	in := Intake(FamilyCase{US: true, Family: "power_supplies_chargers", Product: p}, templates(t))
	if !slices.Equal(ids(in, StateKnown), []string{"cells_made"}) || in.Questions[0].Known != "South Korea, from the parts list" {
		t.Fatalf("%+v", in.Questions)
	}
	if err := ValidateFamilyAnswers(in, map[string]domain.FamilyAnswer{"cells_made": {Country: "China"}}); err == nil {
		t.Error("an answer to a question the parts list settles was accepted")
	}
}

func TestAnswersAreReadAndValidated(t *testing.T) {
	tms := templates(t)
	c := FamilyCase{US: true, Family: "power_supplies_chargers", Product: batteryPack()}
	in := Intake(c, tms)
	for name, a := range map[string]map[string]domain.FamilyAnswer{
		"unknown id":        {"lens_ground": {Country: "China"}},
		"yes on country":    {"cells_made": {Yes: yes(true)}},
		"blank country":     {"cells_made": {Country: " "}},
		"junk country":      {"cells_made": {Country: "<script>"}},
		"unknown":           {"cells_made": {Country: "Unknown"}},
		"tbd":               {"cells_made": {Country: "TBD"}},
		"not a country":     {"cells_made": {Country: "Mars"}},
		"a customs union":   {"cells_made": {Country: "European Union"}},
		"country on yes/no": {"pack_work": {Country: "China"}},
		"empty yes/no":      {"pack_work": {}},
	} {
		if err := ValidateFamilyAnswers(in, a); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	good := map[string]domain.FamilyAnswer{"cells_made": {Country: "China"}, "pack_work": {Yes: yes(false)}}
	if err := ValidateFamilyAnswers(in, good); err != nil {
		t.Fatal(err)
	}
	c.Supplied = &domain.SuppliedFamilyFacts{Source: domain.SourceUserSupplied, UpdatedAt: time.Now(), Family: c.Family, TemplateVersion: "2",
		Answers: map[string]domain.FamilyAnswer{"cells_made": {Country: "China"}, "pack_work": {Yes: yes(false)}, "old_question": {Country: "Peru"}}}
	in = Intake(c, tms)
	if !slices.Equal(ids(in, StateAnswered), []string{"cells_made", "pack_work"}) || !slices.Equal(in.Dropped, []string{"old_question"}) || in.Status != FamilyMissing {
		t.Fatalf("%+v", in)
	}
	c.Supplied.Answers["bms_made"] = domain.FamilyAnswer{Country: "Mexico"}
	if in = Intake(c, tms); in.Status != FamilyComplete || in.Open() != 0 {
		t.Errorf("complete: %+v", in)
	}
	c.Supplied.Stale = true
	if in = Intake(c, tms); !in.StaleAll || len(ids(in, StateAnswered)) != 0 || in.Status != FamilyMissing {
		t.Errorf("stale answers were read: %+v", in)
	}
}

func TestEachDeepenedFamilyAsksItsDecidingFacts(t *testing.T) {
	tms := templates(t)
	for _, tc := range []struct {
		fam  string
		p    *domain.Product
		want []string
	}{
		{"motors_pumps_compressors", &domain.Product{Name: "Industrial robot", Components: []domain.Component{{Name: "robot arm"}, {Name: "pedestal"}}}, []string{"working_unit_made", "frame_made"}},
		{"motors_pumps_compressors", &domain.Product{Name: "Centrifugal water pump"}, []string{"motor_made", "rotary_made", "housing_made"}},
		{"furniture", &domain.Product{Name: "Dining table", Description: "stone top on a metal base"}, []string{"panels_cut", "frame_made", "knock_down"}},
		{"furniture", &domain.Product{Name: "Upholstered sofa"}, []string{"frame_made", "upholstered"}},
		{"medical_devices", &domain.Product{Name: "Surgical drill bits"}, []string{"blanks_made", "sterile_only"}},
		{"medical_devices", &domain.Product{Name: "Blood pressure monitor"}, []string{"sensors_made", "sterile_only"}},
		{"display_camera_modules", &domain.Product{Name: "Camera lens assembly"}, []string{"lens_ground", "polish_only", "sensor_made"}},
		{"display_camera_modules", &domain.Product{Name: "Camera module"}, []string{"sensor_made"}},
		{"offroad_vehicles_machinery", &domain.Product{Name: "Hydraulic floor jack"}, []string{"lift_made", "castings_machined_only", "arms_made", "chassis_made", "assembly_only"}},
		{"offroad_vehicles_machinery", &domain.Product{Name: "Automated guided vehicle", Description: "lifts and carries reel stands"}, []string{"lift_made", "castings_machined_only", "arms_made", "chassis_made", "assembly_only"}},
		{"offroad_vehicles_machinery", &domain.Product{Name: "Utility task vehicle"}, []string{"arms_made", "chassis_made", "assembly_only"}},
		{"optical_precision_instruments", &domain.Product{Name: "Pipe with a 2 inch diameter"}, nil},
		{"optical_precision_instruments", &domain.Product{Name: "Riflescope lens"}, []string{"optics_ground", "optics_polish_only"}},
		{"optical_precision_instruments", &domain.Product{Name: "Digital pressure sensor"}, []string{"sensing_made", "electronics_made", "calibrate_only"}},
		{"optical_precision_instruments", &domain.Product{Name: "Speed square"}, []string{"sensing_made", "calibrate_only"}},
	} {
		in := Intake(FamilyCase{US: true, Family: tc.fam, Product: tc.p}, tms)
		if got := ids(in, StateMissing); !slices.Equal(got, tc.want) {
			t.Errorf("%s %q: %v, want %v", tc.fam, tc.p.Name, got, tc.want)
		}
	}
}

// Unsupported and ambiguous products: no question, a reason, and nothing to
// answer.
func TestNotModelled(t *testing.T) {
	tms := templates(t)
	for _, tc := range []struct {
		name string
		c    FamilyCase
		code string
	}{
		{"another market", FamilyCase{US: false, Family: "furniture", Product: &domain.Product{Name: "Table"}}, "market"},
		{"no family", FamilyCase{US: true, Product: &domain.Product{Name: "Widget"}}, "no_family"},
		{"family none", FamilyCase{US: true, Family: "none", Product: &domain.Product{Name: "Widget"}}, "no_family"},
		{"unknown family", FamilyCase{US: true, Family: "spaceships", Product: &domain.Product{Name: "Rocket"}}, "unknown_family"},
		{"family without questions", FamilyCase{US: true, Family: "toys_build_kits", Product: &domain.Product{Name: "Building blocks"}}, "no_questions"},
		{"no product", FamilyCase{US: true, Family: "furniture"}, "no_product"},
		{"cordless drill as a battery pack", FamilyCase{US: true, Family: "power_supplies_chargers", Product: &domain.Product{Name: "Cordless drill with battery"}}, "guard"},
		{"floor lamp as furniture", FamilyCase{US: true, Family: "furniture", Product: &domain.Product{Name: "Floor lamp"}}, "guard"},
		{"pet bed as furniture", FamilyCase{US: true, Family: "furniture", Product: &domain.Product{Name: "Pet bed made in Vietnam"}}, "guard"},
		{"toy binoculars as optics", FamilyCase{US: true, Family: "display_camera_modules", Product: &domain.Product{Name: "Toy binoculars"}}, "guard"},
		{"vitamin as a device", FamilyCase{US: true, Family: "medical_devices", Product: &domain.Product{Name: "Vitamin D supplement"}}, "guard"},
		{"ambiguous machine", FamilyCase{US: true, Family: "motors_pumps_compressors", Product: &domain.Product{Name: "Widget assembly"}}, "no_match"},
		{"3D printer as a machine", FamilyCase{US: true, Family: "motors_pumps_compressors", Product: &domain.Product{Name: "3D printer"}}, "guard"},
		{"3D printer as a lifting machine", FamilyCase{US: true, Family: "offroad_vehicles_machinery", Product: &domain.Product{Name: "Dental 3D printer"}}, "guard"},
		{"jack stand as a lifting machine", FamilyCase{US: true, Family: "offroad_vehicles_machinery", Product: &domain.Product{Name: "Automotive jack stands"}}, "guard"},
		{"riflescope lens as a display module", FamilyCase{US: true, Family: "display_camera_modules", Product: &domain.Product{Name: "Riflescope lens"}}, "guard"},
		{"flashlight as an instrument", FamilyCase{US: true, Family: "optical_precision_instruments", Product: &domain.Product{Name: "LED flashlight"}}, "guard"},
		{"fibre cable as an instrument", FamilyCase{US: true, Family: "optical_precision_instruments", Product: &domain.Product{Name: "Fiber optic cable assemblies"}}, "guard"},
		{"smart glasses as optics", FamilyCase{US: true, Family: "optical_precision_instruments", Product: &domain.Product{Name: "Smart glasses"}}, "guard"},
		{"ambiguous instrument", FamilyCase{US: true, Family: "optical_precision_instruments", Product: &domain.Product{Name: "Widget assembly"}}, "no_match"},
	} {
		in := Intake(tc.c, tms)
		if in.Status != FamilyNotModelled || in.NotModelled == nil || in.NotModelled.Code != tc.code || len(in.Questions) != 0 {
			t.Errorf("%s: %+v", tc.name, in)
			continue
		}
		if err := ValidateFamilyAnswers(in, map[string]domain.FamilyAnswer{"x": {Country: "China"}}); err == nil {
			t.Errorf("%s: answers accepted", tc.name)
		}
	}
	// A real battery pack is not refused by the guard.
	if in := Intake(FamilyCase{US: true, Family: "power_supplies_chargers", Product: &domain.Product{Name: "Battery pack imported from China"}}, tms); in.Status == FamilyNotModelled {
		t.Errorf("battery pack refused: %+v", in.NotModelled)
	}
}

// The intake reads facts text when there is no product.
func TestFactsOnlyCase(t *testing.T) {
	in := Intake(FamilyCase{US: true, Family: "medical_devices", Facts: "Hypodermic needles. Cannulae drawn in Japan, hubs moulded in China."}, templates(t))
	if got := ids(in, StateMissing); !slices.Equal(got, []string{"blanks_made", "sterile_only"}) {
		t.Errorf("%v", got)
	}
}
