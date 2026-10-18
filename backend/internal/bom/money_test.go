package bom

import (
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

func readCSV(t *testing.T, s string) [][]string {
	t.Helper()
	rows, err := ReadRows(strings.NewReader(s))
	if err != nil {
		t.Fatal(err)
	}
	return rows
}
