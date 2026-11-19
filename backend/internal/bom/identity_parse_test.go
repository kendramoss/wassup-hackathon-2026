package bom

import (
	"bytes"
	"sort"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
)

func parseCSV(t *testing.T, s string) (domain.Product, error) {
	t.Helper()
	rows, err := ReadRows(strings.NewReader(s))
	if err != nil {
		t.Fatal(err)
	}
	p, _, err := Parse(rows, Options{Name: "x"})
	return p, err
}

// idsByName maps plant name to ID, failing on a duplicated name.
func idsByName(t *testing.T, p domain.Product) map[string]string {
	t.Helper()
	m := map[string]string{}
	for _, pl := range p.Plants {
		if _, dup := m[pl.Name]; dup {
			t.Fatalf("plant name %q appears twice: %+v", pl.Name, p.Plants)
		}
		m[pl.Name] = pl.ID
	}
	return m
}

func TestParseDistinctPunctuationSameCountry(t *testing.T) {
	p, err := parseCSV(t, "Part,Country,Plant,Group,Capabilities\nmotor,China,A-B,g1,cap1\nfan,China,A B,g2,cap2\n")
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Plants) != 2 {
		t.Fatalf("plants = %+v, want 2 distinct", p.Plants)
	}
	ids := idsByName(t, p)
	if ids["A-B"] == ids["A B"] {
		t.Fatalf("same ID %q", ids["A-B"])
	}
	if p.Components[0].Plant != ids["A-B"] || p.Components[1].Plant != ids["A B"] {
		t.Errorf("component plants = %q %q, ids %v", p.Components[0].Plant, p.Components[1].Plant, ids)
	}
	for _, pl := range p.Plants {
		if len(pl.Capabilities) != 2 {
			t.Errorf("%s capabilities = %v, want its own two", pl.Name, pl.Capabilities)
		}
		if pl.Name == "A-B" && strings.Contains(strings.Join(pl.Capabilities, ","), "g2") {
			t.Errorf("capability union: %v", pl.Capabilities)
		}
	}
}

func TestParseDistinctPunctuationCrossCountry(t *testing.T) {
	p, err := parseCSV(t, "Part,Country,Plant\nmotor,China,A-B\nfan,Vietnam,A B\n")
	if err != nil {
		t.Fatalf("distinct plants in distinct countries rejected: %v", err)
	}
	if len(p.Plants) != 2 || p.Plants[0].Country != "China" || p.Plants[1].Country != "Vietnam" {
		t.Errorf("plants = %+v", p.Plants)
	}
}

func TestParseSameNameTwoCountriesStillConflicts(t *testing.T) {
	if _, err := parseCSV(t, "Part,Country,Plant\nmotor,China,A-B\nfan,Vietnam,a-b\n"); err == nil {
		t.Fatal("one supplier in two countries must still be rejected")
	}
}

func TestParseRepeatedSupplierIsOnePlant(t *testing.T) {
	p, err := parseCSV(t, "Part,Country,Plant\nmotor,China,A-B\nfan,China,a-b\nfoot,China,  A-B \n")
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Plants) != 1 || p.Plants[0].ID != "a_b" {
		t.Fatalf("plants = %+v, want one a_b", p.Plants)
	}
}

func TestParseUnicodeNamesDistinctAndRowOrderIndependent(t *testing.T) {
	names := []string{"東京工場", "大阪工場", "Zürich Werk", "Zürich-Werk", "工場"}
	var want map[string]string
	orders := [][]int{{0, 1, 2, 3, 4}, {4, 3, 2, 1, 0}, {2, 0, 4, 1, 3}, {3, 4, 0, 2, 1}}
	for _, ord := range orders {
		var b strings.Builder
		b.WriteString("Part,Country,Plant\n")
		for _, i := range ord {
			b.WriteString("p" + names[i] + ",Japan," + names[i] + "\n")
		}
		for run := 0; run < 2; run++ {
			p, err := parseCSV(t, b.String())
			if err != nil {
				t.Fatal(err)
			}
			if len(p.Plants) != len(names) {
				t.Fatalf("order %v: %d plants, want %d: %+v", ord, len(p.Plants), len(names), p.Plants)
			}
			got := idsByName(t, p)
			seen := map[string]bool{}
			for n, id := range got {
				if seen[id] {
					t.Fatalf("duplicate ID %q in %v", id, got)
				}
				seen[id] = true
				if id == "" || id != strings.ToLower(id) {
					t.Errorf("%q has bad ID %q", n, id)
				}
			}
			if want == nil {
				want = got
			}
			for n, id := range want {
				if got[n] != id {
					t.Errorf("order %v: %q -> %q, first run %q", ord, n, got[n], id)
				}
			}
			for _, c := range p.Components {
				if got[strings.TrimPrefix(c.Name, "p")] != c.Plant {
					t.Errorf("component %q -> plant %q, want %q", c.Name, c.Plant, got[strings.TrimPrefix(c.Name, "p")])
				}
			}
		}
	}
}

func TestParseXLSXCollisions(t *testing.T) {
	f := excelize.NewFile()
	for i, row := range [][]any{{"Component", "Country", "Plant"}, {"motor", "China", "A-B"}, {"fan", "China", "A B"}, {"cap", "Japan", "東京工場"}, {"lid", "Japan", "大阪工場"}, {"lid2", "Japan", "大阪工場"}} {
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
	p, _, err := Parse(rows, Options{Name: "x"})
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Plants) != 4 {
		t.Fatalf("plants = %+v, want 4", p.Plants)
	}
	ids := idsByName(t, p)
	if p.Components[3].Plant != p.Components[4].Plant || p.Components[3].Plant != ids["大阪工場"] {
		t.Errorf("repeated supplier split: %+v", p.Components)
	}
}

func TestParseKeepsLegacyIDsWhenUnambiguous(t *testing.T) {
	p, err := parseCSV(t, "Part,Country,Plant\na,Thailand,Rayong plant\nb,China,\nc,Vietnam,A-B\n")
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, pl := range p.Plants {
		ids = append(ids, pl.ID)
	}
	if strings.Join(ids, ",") != "rayong_plant,china,a_b" {
		t.Errorf("IDs = %v", ids)
	}
}

func collisionProduct(t *testing.T, rows ...string) domain.Product {
	t.Helper()
	p, err := parseCSV(t, "Part,Country,Plant\n"+strings.Join(rows, "\n")+"\n")
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestDiffAndKeepPlantIDsAcrossCollisionPair(t *testing.T) {
	before := collisionProduct(t, "motor,China,A-B", "fan,China,A B")
	again := collisionProduct(t, "fan,China,A B", "motor,China,A-B") // reordered upload
	kept := KeepPlantIDs(before, again)
	if d := Diff(before, kept); len(d) != 0 {
		t.Fatalf("same BOM reordered shows changes: %+v", d)
	}
	b, k := idsByName(t, before), idsByName(t, kept)
	for n, id := range b {
		if k[n] != id {
			t.Errorf("%q: %q, was %q", n, k[n], id)
		}
	}
	// moving a part between the pair is a plant change, not "no change"
	moved := collisionProduct(t, "motor,China,A B", "fan,China,A B")
	d := Diff(before, KeepPlantIDs(before, moved))
	if len(d) != 1 || d[0].Kind != PlantChanged || d[0].Name != "motor" {
		t.Errorf("diff = %+v, want motor plant_changed", d)
	}
}

func TestKeepPlantIDsLegacyMergedStoredProduct(t *testing.T) {
	// a stored product made by the old importer: one plant a_b named "A-B"
	legacy := domain.Product{Name: "x", Plants: []domain.Plant{{ID: "a_b", Name: "A-B", Country: "China"}},
		Components: []domain.Component{{Name: "motor", MadeIn: "China", Plant: "a_b"}}}
	for _, rows := range [][]string{{"motor,China,A-B", "fan,China,A B"}, {"fan,China,A B", "motor,China,A-B"}} {
		after := KeepPlantIDs(legacy, collisionProduct(t, rows...))
		ids := idsByName(t, after)
		if ids["A-B"] != "a_b" {
			t.Errorf("%v: A-B -> %q, want the stored a_b", rows, ids["A-B"])
		}
		if ids["A B"] == "a_b" || ids["A B"] == "" {
			t.Errorf("%v: A B took %q", rows, ids["A B"])
		}
		if err := after.Validate(); err != nil {
			t.Errorf("invalid: %v", err)
		}
	}
	// the other legacy name must not steal the bare slug
	after := KeepPlantIDs(domain.Product{Plants: []domain.Plant{{ID: "a_b", Name: "A B", Country: "China"}}}, collisionProduct(t, "motor,China,A-B", "fan,China,A B"))
	if ids := idsByName(t, after); ids["A B"] != "a_b" || ids["A-B"] == "a_b" {
		t.Errorf("ids = %v", ids)
	}
}

func TestKeepPlantIDsAmbiguousStoredNamesAreNotMerged(t *testing.T) {
	// stored plants "A-B" and "A B" both must keep their own IDs, whichever order
	stored := domain.Product{Plants: []domain.Plant{{ID: "p2", Name: "A B", Country: "China"}, {ID: "p1", Name: "A-B", Country: "China"}}}
	after := KeepPlantIDs(stored, collisionProduct(t, "motor,China,A-B", "fan,China,A B"))
	ids := idsByName(t, after)
	got := []string{ids["A-B"], ids["A B"]}
	sort.Strings(got)
	if ids["A-B"] != "p1" || ids["A B"] != "p2" {
		t.Errorf("ids = %v", ids)
	}
}
