package bom

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestParseMoney(t *testing.T) {
	csv := `name,country,plant,group,type,qty,unit_cost_usd,annual_units,unit_price_usd,hts
housing,Vietnam,Binh Duong,housing_moulding,component,1,$1.50,"20,000",$25,8516.31.0000
motor,China,Ningbo,motor_fan_assembly,component,1,4,,,
fan,China,Ningbo,motor_fan_assembly,component,2,0.5,,,
wiring,China,Ningbo,switch_cord_minor_parts,component,5,abc,,,
Penang,Malaysia,Penang,,plant,,9,,,`
	rows := readCSV(t, csv)
	m, w := ParseMoney(rows, Options{})
	if m.HTS != "8516.31.0000" || m.AnnualUnits != 20000 || m.UnitPriceUSD != 25 || m.AnnualImportValueUSD != 500000 {
		t.Errorf("money = %+v", m)
	}
	if m.GroupCosts["housing_moulding"]["binh_duong"] != 30000 || m.GroupCosts["motor_fan_assembly"]["ningbo"] != 100000 {
		t.Errorf("group costs = %v", m.GroupCosts)
	}
	if _, ok := m.GroupCosts["switch_cord_minor_parts"]; ok {
		t.Errorf("a bad unit cost was counted: %v", m.GroupCosts)
	}
	if len(w) != 1 || !strings.Contains(w[0], `unit cost "abc"`) {
		t.Errorf("warnings = %v", w)
	}
}

func TestParseMoneyNeedsAnnualUnits(t *testing.T) {
	m, w := ParseMoney(readCSV(t, "name,country,group,unit cost\nmotor,China,motor_fan_assembly,4\n"), Options{})
	if m.GroupCosts != nil || len(w) != 1 || !strings.Contains(w[0], "annual_units") {
		t.Errorf("money = %+v, warnings = %v", m, w)
	}
}

func TestParseMoneyNoColumns(t *testing.T) {
	m, w := ParseMoney(readCSV(t, "name,country,group\nmotor,China,motor_fan_assembly\n"), Options{})
	if m.HTS != "" || m.AnnualImportValueUSD != 0 || m.GroupCosts != nil || w != nil {
		t.Errorf("money = %+v, warnings = %v", m, w)
	}
}

func TestPlantIdentityIsSharedByProductAndMoney(t *testing.T) {
	rows := readCSV(t, `name,country,plant,group,qty,unit_cost_usd,annual_units
motor,PRC,,motor_assembly,1,20,1000
assembly,Viet Nam,,final_assembly,1,5,`)
	m, w := ParseMoney(rows, Options{})
	if len(w) != 0 {
		t.Fatalf("warnings = %v", w)
	}
	if got := m.GroupCosts["motor_assembly"]["china"]; got != 20000 {
		t.Errorf("PRC cost = %v, want 20000 under china: %v", got, m.GroupCosts)
	}
	if got := m.GroupCosts["final_assembly"]["vietnam"]; got != 5000 {
		t.Errorf("Viet Nam cost = %v, want 5000 under vietnam: %v", got, m.GroupCosts)
	}
	p, _, err := Parse(rows, Options{Name: "x"})
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"china", "vietnam"} {
		if _, ok := p.PlantByID(id); !ok {
			t.Errorf("product has no plant %q: %+v", id, p.Plants)
		}
	}
	if _, ok := p.PlantByID("prc"); ok {
		t.Errorf("dangling raw alias plant: %+v", p.Plants)
	}
}

func TestPlantIdentityKeepsExplicitPlantColumn(t *testing.T) {
	rows := readCSV(t, `name,country,plant,group,qty,unit_cost_usd,annual_units
motor,PRC,Rayong plant,motor_assembly,1,20,1000`)
	m, _ := ParseMoney(rows, Options{})
	if got := m.GroupCosts["motor_assembly"]["rayong_plant"]; got != 20000 {
		t.Errorf("explicit plant cost = %v, want 20000: %v", got, m.GroupCosts)
	}
}

func TestAmountRejectsNonFiniteNumbers(t *testing.T) {
	for _, s := range []string{"Inf", "+Inf", "Infinity", "+Infinity", "$Inf", "-Inf", "NaN", "1e309"} {
		if _, ok := amount(s); ok {
			t.Errorf("amount(%q) was accepted; want rejected", s)
		}
	}
	for _, s := range []string{"0", "1.5", "$25", "20,000"} {
		if f, ok := amount(s); !ok || !finite(f) {
			t.Errorf("amount(%q) = %v, %v; want a finite number", s, f, ok)
		}
	}
}

func TestParseMoneyNonFiniteCellsAreIgnored(t *testing.T) {
	csv := `name,country,group,qty,unit_cost_usd,annual_units,unit_price_usd
motor,China,motor_fan_assembly,Inf,Inf,Inf,Inf`
	m, w := ParseMoney(readCSV(t, csv), Options{})
	if m.AnnualUnits != 0 || m.UnitPriceUSD != 0 || m.AnnualImportValueUSD != 0 || m.GroupCosts != nil {
		t.Errorf("nonfinite cells were kept: %+v", m)
	}
	if len(w) != 3 {
		t.Fatalf("warnings = %v, want one each for units, price and cost", w)
	}
	if _, err := json.Marshal(m); err != nil {
		t.Errorf("money cannot be serialized: %v", err)
	}
}

func TestParseMoneyOverflowIsIgnored(t *testing.T) {
	csv := `name,country,group,qty,unit_cost_usd,annual_units,unit_price_usd
motor,China,motor_fan_assembly,1e308,1e308,1e308,100
fan,China,motor_fan_assembly,1,1e308,,`
	m, w := ParseMoney(readCSV(t, csv), Options{})
	if !finite(m.AnnualImportValueUSD) || !finite(m.AnnualUnits) || !finite(m.UnitPriceUSD) {
		t.Errorf("nonfinite import value: %+v", m)
	}
	for group, plants := range m.GroupCosts {
		for plant, cost := range plants {
			if !finite(cost) {
				t.Errorf("nonfinite group cost %s/%s: %v", group, plant, cost)
			}
		}
	}
	var tooLarge int
	for _, msg := range w {
		if strings.Contains(msg, "too large") {
			tooLarge++
		}
	}
	if tooLarge != 3 {
		t.Errorf("overflow warnings = %v, want 3", w)
	}
	if _, err := json.Marshal(m); err != nil {
		t.Errorf("money cannot be serialized: %v", err)
	}
}

func readCSV(t *testing.T, s string) [][]string {
	t.Helper()
	rows, err := ReadRows(strings.NewReader(s))
	if err != nil {
		t.Fatal(err)
	}
	return rows
}
