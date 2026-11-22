package export

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fixtureNames are the product names of the Part 102 plan fixtures, built
// from CBP ruling subjects (D501).
func fixtureNames(t *testing.T) map[string]string {
	t.Helper()
	files, err := filepath.Glob("../../testdata/planner_part102/*.json")
	if err != nil || len(files) == 0 {
		t.Fatalf("no Part 102 fixtures: %v", err)
	}
	out := map[string]string{}
	for _, f := range files {
		raw, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		var fx struct {
			Product struct{ Name string } `json:"product"`
		}
		if err := json.Unmarshal(raw, &fx); err != nil {
			t.Fatal(err)
		}
		out[strings.TrimSuffix(filepath.Base(f), ".json")] = fx.Product.Name
	}
	return out
}

func TestDraftName(t *testing.T) {
	fx := fixtureNames(t)
	for _, c := range []struct{ in, want, model string }{
		// The Part 102 fixtures, as committed.
		{fx["n348465"], "an electrical actuator", "Electrical actuator"},
		{fx["n344002"], "hermetic compressors", "Hermetic compressors"},
		{fx["n350133"], "a Walk-Behind Lawn Mower", "Walk-Behind Lawn Mower"},
		// Case and spacing of the opening do not matter.
		{"  country of origin of a heat gun ", "a heat gun", "Heat gun"},
		{"THE COUNTRY OF ORIGIN OF an air fryer", "an air fryer", "Air fryer"},
		{"The country of origin of the subject motor", "the subject motor", "The subject motor"},
		// Normal names are kept as given.
		{"Kobalt 12.5-amp heat gun, model 5208257", "Kobalt 12.5-amp heat gun, model 5208257", "Kobalt 12.5-amp heat gun, model 5208257"},
		{"The North Face jacket", "The North Face jacket", "The North Face jacket"},
		{"an air fryer", "an air fryer", "an air fryer"},
		{"Country of origin label printer", "Country of origin label printer", "Country of origin label printer"},
		{"The country of origin and eligibility of steel", "The country of origin and eligibility of steel", "The country of origin and eligibility of steel"},
		{"The country of origin of", "The country of origin of", "The country of origin of"},
		{"", "", ""},
	} {
		if got := DraftName(c.in); got != c.want {
			t.Errorf("DraftName(%q) = %q, want %q", c.in, got, c.want)
		}
		if got := modelName(c.in); got != c.model {
			t.Errorf("modelName(%q) = %q, want %q", c.in, got, c.model)
		}
	}
	for id, want := range map[string]string{"n348465": "The country of origin of an electrical actuator", "n344002": "The country of origin of hermetic compressors"} {
		if fx[id] != want {
			t.Fatalf("fixture %s name is %q; update this test", id, fx[id])
		}
	}
}

// A product named by a ruling subject is never written "the country of
// origin of the country of origin of ..." anywhere in either letter: the
// opening, the RE line, the transaction, the merchandise and the question.
func TestNoDoubledSubjectInDrafts(t *testing.T) {
	for id, name := range fixtureNames(t) {
		for _, part102 := range []bool{false, true} {
			in := Input{ProductName: name, Facts: "Final assembly in Mexico.", Requested: "Mexico", Part102: part102}
			for format, render := range bothFormats {
				got := render(in)
				l := strings.ToLower(got)
				if strings.Count(l, "country of origin of the country of origin") > 0 || strings.Contains(l, "of the country of origin and") || strings.Contains(l, "— the country of origin") || strings.Contains(l, "importation of the country of origin") || strings.Contains(l, "name and model: the country of origin") {
					t.Errorf("%s %s (part102=%v) doubles the subject:\n%s", id, format, part102, got)
				}
				if want := "country of origin of " + DraftName(name); !strings.Contains(got, want) {
					t.Errorf("%s %s missing %q", id, format, want)
				}
			}
		}
	}
	got := CFR177(Input{ProductName: "The country of origin of an electrical actuator", Facts: "f"})
	for _, w := range []string{
		"**RE: Request for a binding ruling under 19 C.F.R. Part 177 — country of origin of an electrical actuator**",
		"respectfully requests a binding ruling on the country of origin of an electrical actuator, determined under",
		"Prospective importation of an electrical actuator, produced as described",
		"- Name and model: Electrical actuator.",
		"What is the country of origin of an electrical actuator, produced as described above,",
	} {
		if !strings.Contains(got, w) {
			t.Errorf("cfr177 missing %q", w)
		}
	}
	md := Markdown(Input{ProductName: "The country of origin of an electrical actuator", Facts: "f"})
	for _, w := range []string{
		"**RE: Request for a binding country of origin ruling — an electrical actuator**",
		"requests a binding ruling on the country of origin of an electrical actuator for purposes",
		"What is the country of origin of an electrical actuator, produced as described above,",
	} {
		if !strings.Contains(md, w) {
			t.Errorf("markdown missing %q", w)
		}
	}
}
