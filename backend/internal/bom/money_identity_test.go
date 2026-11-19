package bom

import (
	"fmt"
	"strings"
	"testing"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/planner"
)

const moneyHeader = "name,country,plant,group,type,qty,unit_cost_usd,annual_units\n"

// importCSV runs the same chain the import handler runs: ReadRows, Parse,
// ParseMoney on the same rows and options.
func importCSV(t *testing.T, csv string) (domain.Product, Money, []string) {
	t.Helper()
	rows := readCSV(t, csv)
	opts := Options{Name: "x"}
	p, _, err := Parse(rows, opts)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	m, w := ParseMoney(rows, opts)
	return p, m, w
}

// reorder returns the CSV with its data rows reversed.
func reorder(csv string) string {
	lines := strings.Split(strings.TrimSpace(csv), "\n")
	head, data := lines[0], lines[1:]
	for i, j := 0, len(data)-1; i < j; i, j = i+1, j-1 {
		data[i], data[j] = data[j], data[i]
	}
	return head + "\n" + strings.Join(data, "\n") + "\n"
}

// Every component row "<name>" has group "g_<name>" and unit cost 1 x 1000
// units, so its yearly cost is 1000 at the plant Parse put the component in.
func checkMoneyMatchesParse(t *testing.T, csv string) {
	t.Helper()
	p, m, w := importCSV(t, csv)
	plantOf := map[string]string{}
	for _, c := range p.Components {
		plantOf[c.Name] = c.Plant
	}
	valid := map[string]bool{}
	for _, pl := range p.Plants {
		valid[pl.ID] = true
	}
	for g, byPlant := range m.GroupCosts {
		for id := range byPlant {
			if !valid[id] {
				t.Errorf("group %s cost at %q: not a plant of the product %v", g, id, p.Plants)
			}
		}
	}
	for name, id := range plantOf {
		if got := m.GroupCosts["g_"+name][id]; got != 1000 {
			t.Errorf("cost of %q at its Parse plant %q = %v, want 1000 (costs %v, warnings %v)", name, id, got, m.GroupCosts, w)
		}
	}
	if len(m.GroupCosts) != len(plantOf) {
		t.Errorf("costs for %d groups, want %d: %v", len(m.GroupCosts), len(plantOf), m.GroupCosts)
	}
}

func TestMoneyPlantIDsEqualParseIDs(t *testing.T) {
	cases := map[string]string{
		"colliding": moneyHeader + `a1,Vietnam,A-B,g_a1,component,1,1,1000
a2,Vietnam,A B,g_a2,component,1,1,1000
a3,China,Zed,g_a3,component,1,1,1000
`,
		"non-colliding": moneyHeader + `n1,Vietnam,Binh Duong,g_n1,component,1,1,1000
n2,China,Ningbo,g_n2,component,1,1,1000
`,
		"country fallback and alias": moneyHeader + `c1,PRC,,g_c1,component,1,1,1000
c2,Viet Nam,,g_c2,component,1,1,1000
c3,China,,g_c3,component,1,1,1000
`,
		"unicode": moneyHeader + `u1,Japan,東京工場,g_u1,component,1,1,1000
u2,Japan,大阪工場,g_u2,component,1,1,1000
u3,Japan,東京工場,g_u3,component,1,1,1000
`,
		"repeated supplier": moneyHeader + `r1,Vietnam,A B,g_r1,component,1,1,1000
r2,Vietnam,a  b,g_r2,component,1,1,1000
`,
	}
	for name, csv := range cases {
		t.Run(name+"/forward", func(t *testing.T) { checkMoneyMatchesParse(t, csv) })
		t.Run(name+"/reversed", func(t *testing.T) { checkMoneyMatchesParse(t, reorder(csv)) })
	}
}

func TestMoneyCountryFallbackCanonicalIDs(t *testing.T) {
	_, m, _ := importCSV(t, moneyHeader+"motor,PRC,,g,component,1,20,1000\nasm,Viet Nam,,h,component,1,5,1000\n")
	if m.GroupCosts["g"]["china"] != 20000 || m.GroupCosts["h"]["vietnam"] != 5000 || len(m.GroupCosts) != 2 {
		t.Errorf("costs = %v, want g@china 20000 and h@vietnam 5000", m.GroupCosts)
	}
}

func TestMoneyRejectedAndPlantRowsCreateNoCosts(t *testing.T) {
	csv := moneyHeader + `motor,China,Ningbo,g,component,1,2,1000
,China,Ghost,g,component,1,9,
nocountry,,Phantom,g,component,1,9,
Penang,Malaysia,Penang,,plant,,9,
`
	rows := readCSV(t, csv)
	m, _ := ParseMoney(rows, Options{})
	if len(m.GroupCosts) != 1 || len(m.GroupCosts["g"]) != 1 || m.GroupCosts["g"]["ningbo"] != 2000 {
		t.Errorf("costs = %v, want only g@ningbo 2000", m.GroupCosts)
	}
}

func TestMoneyUnknownPlantIsNotResolved(t *testing.T) {
	ids := NewPlantIDs(readCSV(t, moneyHeader+"a,Vietnam,A_B,g,component,1,1,1000\n"))
	if id, ok := ids.lookup("A_B", "Vietnam"); !ok || id != "a_b" {
		t.Errorf("known plant = %q, %v", id, ok)
	}
	if id, ok := ids.lookup("A-B", "Vietnam"); ok {
		t.Errorf("unknown plant resolved to %q; its fallback slug would equal a real plant", id)
	}
}

// The issue fixture through the real planner: moving the motor group from
// China ($20 x 1000) to Vietnam ($30 x 1000) adds $10,000 a year.
func TestMoneyIssueFixtureAddedCostViaCompare(t *testing.T) {
	csv := moneyHeader + `motor op,PRC,,motor,operation,1,20,1000
motor alt,Viet Nam,,motor,component,1,30,1000
assembly,Viet Nam,,assembly,component,1,5,1000
`
	for name, c := range map[string]string{"forward": csv, "reversed": reorder(csv)} {
		t.Run(name, func(t *testing.T) {
			p, m, _ := importCSV(t, c)
			if m.GroupCosts["motor"]["china"] != 20000 || m.GroupCosts["motor"]["vietnam"] != 30000 || m.GroupCosts["assembly"]["vietnam"] != 5000 {
				t.Fatalf("costs = %v", m.GroupCosts)
			}
			variant := planner.Apply(p, domain.Move{{Group: "motor", ToPlant: "vietnam"}})
			cmp := planner.Compare(planner.Economics{GroupCosts: m.GroupCosts}, &p, &variant, "China", "Vietnam")
			if cmp.AddedCostUSD == nil || *cmp.AddedCostUSD != 10000 {
				t.Errorf("added cost = %v, want 10000 (moved %v)", fmt.Sprint(cmp.AddedCostUSD), cmp.Moved)
			}
		})
	}
}
