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

// Every question asks about groups its template has, names a document, and
// cites only rulings the template cites for those groups.
func TestFamilyQuestionsAreGroundedInTheirTemplates(t *testing.T) {
	tms := templates(t)
	for fam, qs := range familyQuestions {
		tm, ok := tms[fam]
		if !ok {
			t.Fatalf("%s: no template", fam)
		}
		if tm.Version != "2" {
			t.Errorf("%s: questions were written for version 2 templates, got %s", fam, tm.Version)
		}
		cited := 0
		for _, q := range qs {
			if q.evidence == "" || q.text == "" || (q.kind != KindCountry && q.kind != KindYesNo) {
				t.Errorf("%s/%s: incomplete", fam, q.id)
			}
			for _, g := range q.groups {
				found := false
				for _, tg := range tm.Groups {
					if tg.ID == g {
						found = true
						if tg.Precedent != nil {
							cited += len(tg.Precedent.ConfersOrigin) + len(tg.Precedent.NotEnough)
						}
					}
				}
				if !found {
					t.Errorf("%s/%s: group %s is not in the template", fam, q.id, g)
				}
			}
		}
		if cited == 0 {
			t.Errorf("%s: no question rests on a cited ruling", fam)
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

func TestKnownFromThePartsListIsNotAsked(t *testing.T) {
	p := batteryPack()
	p.Components[0].MadeIn = "South Korea"
	p.Operations = append(p.Operations, domain.Operation{Name: "program the BMS", Plant: "mx"}, domain.Operation{Name: "laser weld cells", Plant: "mx"}, domain.Operation{Name: "formation and grading", Plant: "mx"})
	in := Intake(FamilyCase{US: true, Family: "power_supplies_chargers", Product: p}, templates(t))
	if !slices.Equal(ids(in, StateKnown), []string{"cells_made", "pack_work"}) || in.Questions[0].Known != "South Korea, from the parts list" {
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
		{"display_camera_modules", &domain.Product{Name: "Riflescope lens"}, []string{"lens_ground", "polish_only"}},
		{"display_camera_modules", &domain.Product{Name: "Camera module"}, []string{"sensor_made"}},
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
