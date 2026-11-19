package bom

import (
	"fmt"
	"strings"
	"testing"
)

func TestPlantIdentity(t *testing.T) {
	same := [][2]string{{"A-B", " a-b "}, {"Zürich", "Zürich"}, {"ＡＢ plant", "ab plant"}, {"A  B", "a b"}, {"A​B", "ab"}}
	for _, c := range same {
		if PlantIdentity(c[0]) != PlantIdentity(c[1]) {
			t.Errorf("%q and %q should be the same supplier", c[0], c[1])
		}
	}
	diff := [][2]string{{"A-B", "A B"}, {"A.B", "AB"}, {"東京", "大阪"}, {"A_B", "A-B"}}
	for _, c := range diff {
		if PlantIdentity(c[0]) == PlantIdentity(c[1]) {
			t.Errorf("%q and %q should differ", c[0], c[1])
		}
	}
}

// Money-side lookup (plant cell + country cell) must resolve to the very ID the
// product parser gave, for colliding and non-colliding suppliers, with the
// country fallback, in any row order.
func TestPlantIDsMatchParserForMoneyLookup(t *testing.T) {
	type row struct{ part, country, plant string }
	rowsIn := []row{
		{"a", "China", "A-B"}, {"b", "China", "A B"}, {"c", "china", ""}, {"d", "Thailand", "Rayong plant"},
		{"e", "Japan", "東京工場"}, {"f", "Japan", "大阪工場"}, {"g", "Vietnam", "A.B"}, {"h", "Vietnam", "a.b"},
		{"i", "viet nam", ""}, {"j", "China", "Hanoi"},
	}
	perms := [][]int{{0, 1, 2, 3, 4, 5, 6, 7, 8, 9}, {9, 8, 7, 6, 5, 4, 3, 2, 1, 0}, {5, 2, 9, 0, 7, 1, 8, 3, 6, 4}}
	var first map[string]string
	for _, perm := range perms {
		var b strings.Builder
		b.WriteString("Part,Country,Plant\n")
		var ordered []row
		for _, i := range perm {
			r := rowsIn[i]
			ordered = append(ordered, r)
			fmt.Fprintf(&b, "%s,%s,%s\n", r.part, r.country, r.plant)
		}
		rows, err := ReadRows(strings.NewReader(b.String()))
		if err != nil {
			t.Fatal(err)
		}
		p, _, err := Parse(rows, Options{Name: "x"})
		if err != nil {
			t.Fatal(err)
		}
		ids := NewPlantIDs(rows)
		got := map[string]string{}
		for i, r := range ordered {
			if want, have := p.Components[i].Plant, ids.ID(r.plant, r.country); want != have {
				t.Errorf("%+v: parser %q, money lookup %q", r, want, have)
			}
			got[r.part] = p.Components[i].Plant
		}
		if first == nil {
			first = got
		}
		for k, v := range first {
			if got[k] != v {
				t.Errorf("row %s: %q in this order, %q before", k, got[k], v)
			}
		}
	}
	if first["a"] == first["b"] || first["e"] == first["f"] || first["g"] != first["h"] || first["a"] == first["c"] {
		t.Errorf("ids = %v", first)
	}
	if first["c"] != "china" || first["d"] != "rayong_plant" || first["i"] != "vietnam" {
		t.Errorf("unambiguous IDs changed: %v", first)
	}
}

func TestNoFirstRowWinsForSlugCollision(t *testing.T) {
	ab := func(names ...string) map[string]string {
		var b strings.Builder
		b.WriteString("Part,Country,Plant\n")
		for i, n := range names {
			fmt.Fprintf(&b, "p%d,China,%s\n", i, n)
		}
		p, err := parseCSV(t, b.String())
		if err != nil {
			t.Fatal(err)
		}
		return idsByName(t, p)
	}
	x, y := ab("A-B", "A B"), ab("A B", "A-B")
	for n, id := range x {
		if y[n] != id {
			t.Errorf("%q: %q vs %q depending on row order", n, id, y[n])
		}
		if id == "a_b" {
			t.Errorf("%q kept the bare slug although it collides", n)
		}
	}
	if z := ab("A-B"); z["A-B"] != "a_b" {
		t.Errorf("uncontested A-B = %q, want a_b", z["A-B"])
	}
}

func TestIDForAndCountryFallbackBlankPlant(t *testing.T) {
	if got := IDFor("Rayong plant", nil); got != "rayong_plant" {
		t.Errorf("got %q", got)
	}
	if IDFor("東京工場", nil) == IDFor("大阪工場", nil) || !strings.HasPrefix(IDFor("東京工場", nil), "plant_") {
		t.Errorf("unicode ids: %q %q", IDFor("東京工場", nil), IDFor("大阪工場", nil))
	}
	if IDFor("A-B", []string{"A B"}) == IDFor("A B", []string{"A-B"}) {
		t.Error("colliding pair got equal IDs")
	}
}
