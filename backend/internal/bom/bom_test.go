package bom

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/planner"
)

const heatGunCSV = "\xef\xbb\xbfPart,Made in,Factory,Operation group,Plant capabilities\n" +
	"main housing,Thailand,Rayong plant,housing_moulding,Motor-fan assembly; pcba; switch_cord_minor_parts\n" +
	"heating element,Thailand,Rayong plant,heating_element,\n" +
	"electric motor,China,China suppliers,Motor-fan assembly,\n" +
	"fan,China,China suppliers,motor_fan_assembly,\n" +
	"PCBA,china,China suppliers,PCBA,\n" +
	"switch,China,China suppliers,switch_cord_minor_parts,\n" +
	",,,,\n"

func template(t *testing.T) *planner.Template {
	t.Helper()
	tm, err := planner.LoadTemplates()
	if err != nil {
		t.Fatal(err)
	}
	g := tm["motorised_tools_small_appliances"]
	return &g
}

func TestParseCSVToPlannableProduct(t *testing.T) {
	rows, err := ReadRows(strings.NewReader(heatGunCSV))
	if err != nil {
		t.Fatal(err)
	}
	p, warnings, err := Parse(rows, Options{Name: "Heat gun", Family: "motorised_tools_small_appliances", Template: template(t)})
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Components) != 6 || len(p.Plants) != 2 || len(warnings) != 0 {
		t.Fatalf("product = %+v, warnings = %v", p, warnings)
	}
	th, _ := p.PlantByID("rayong_plant")
	if th.Country != "Thailand" || !th.Can("motor_fan_assembly") || !th.Can("pcba") || !th.Can("housing_moulding") {
		t.Errorf("Rayong plant = %+v", th)
	}
	if p.Components[2].Group != "motor_fan_assembly" || p.Components[4].Group != "pcba" || p.Components[4].MadeIn != "China" {
		t.Errorf("groups/countries not normalised: %+v", p.Components)
	}
	if moves := planner.Enumerate(p, *template(t), "Thailand"); len(moves) == 0 {
		t.Error("parsed product gives the plan search no moves")
	}
}

func TestParseExcel(t *testing.T) {
	f := excelize.NewFile()
	for i, row := range [][]any{{"Component", "Country"}, {"motor", "China"}, {"housing", "Vietnam"}} {
		cell, _ := excelize.CoordinatesToCellName(1, i+1)
		if err := f.SetSheetRow("Sheet1", cell, &row); err != nil {
			t.Fatal(err)
		}
	}
	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		t.Fatal(err)
	}
	rows, err := ReadRows(&buf)
	if err != nil {
		t.Fatal(err)
	}
	p, warnings, err := Parse(rows, Options{Name: "Fan"})
	if err != nil || len(p.Components) != 2 || len(p.Plants) != 2 || p.Plants[1].Country != "Vietnam" {
		t.Fatalf("excel product = %+v, %v", p, err)
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], "no product family") {
		t.Errorf("warnings = %v", warnings)
	}
}

func TestParseErrorsAndWarnings(t *testing.T) {
	for name, csv := range map[string]string{
		"missing country column": "Part,Factory\nmotor,A\n",
		"row without country":    "Part,Country\nmotor,\n",
		"plant in two countries": "Part,Country,Plant\nmotor,China,P1\nfan,Vietnam,P1\n",
		"header only":            "Part,Country\n",
	} {
		rows, _ := ReadRows(strings.NewReader(csv))
		if _, _, err := Parse(rows, Options{Name: "x"}); !errors.Is(err, domain.ErrInvalid) {
			t.Errorf("%s: err = %v", name, err)
		}
	}
	rows, _ := ReadRows(strings.NewReader("Part,Country,Group\nmotor,China,teleporter\n"))
	_, w, err := Parse(rows, Options{Name: "x", Family: "motorised_tools_small_appliances", Template: template(t)})
	if err != nil || len(w) < 3 || !strings.Contains(strings.Join(w, "|"), `group "teleporter" is not in`) {
		t.Errorf("warnings = %v, err = %v", w, err)
	}
}

func TestHeatGunSampleMatchesFixture(t *testing.T) {
	f, err := os.Open(filepath.Join("..", "..", "testdata", "heatgun_bom.csv"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	rows, err := ReadRows(f)
	if err != nil {
		t.Fatal(err)
	}
	p, w, err := Parse(rows, Options{Name: "Kobalt heat gun", Family: "motorised_tools_small_appliances", Template: template(t)})
	if err != nil || len(w) != 0 {
		t.Fatalf("sample BOM: %v %v", err, w)
	}
	r, err := planner.LoadRequest(filepath.Join("..", "..", "testdata", "heatgun_2022.json"))
	if err != nil {
		t.Fatal(err)
	}
	got, want := planner.Enumerate(p, *template(t), "Thailand"), planner.Enumerate(r.Product, *template(t), "Thailand")
	if len(p.Components) != len(r.Product.Components) || len(p.Operations) != len(r.Product.Operations) || len(got) != len(want) {
		t.Errorf("sample gives %d components, %d operations, %d moves; fixture %d, %d, %d",
			len(p.Components), len(p.Operations), len(got), len(r.Product.Components), len(r.Product.Operations), len(want))
	}
}
