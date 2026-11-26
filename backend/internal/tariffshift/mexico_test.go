package tariffshift

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func mustMexico(t *testing.T) *Rules {
	t.Helper()
	r, err := MexicoRules()
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestMXRender(t *testing.T) {
	for in, want := range map[string]string{
		"Un cambio a la partida 25.01 a 25.30 de cualquier otro capítulo.":                                                                       "A change to heading 2501 through 2530 from any other chapter.",
		"Un cambio a la subpartida 8483.20 de cualquier otra subpartida, excepto de la subpartida 8482.10 a 8482.80.":                            "A change to subheading 8483.20 from any other subheading, except from subheading 8482.10 through 8482.80.",
		"Un cambio a la partida 51.06 a 51.10 de cualquier partida fuera del grupo.":                                                             "A change to heading 5106 through 5110 from any heading outside that group.",
		"Un cambio a la subpartida 3004.10 de cualquier otra subpartida, excepto de la subpartida 2941.10, 2941.20 o 3006.80.":                   "A change to subheading 3004.10 from any other subheading, except from subheading 2941.10, 2941.20 or 3006.80.",
		"Un cambio a la partida 87.01 a 87.05 de cualquier otra partida, incluyendo otra partida dentro del grupo, excepto de la partida 87.06.": "A change to heading 8701 through 8705 from any other heading, including another heading within that group, except from heading 8706.",
	} {
		got, ok := MXRender(in)
		if !ok || got != want {
			t.Errorf("MXRender(%q) = %q, %v; want %q", in, got, ok, want)
		}
	}
	// Spanish left over: never rendered, so never encoded.
	if _, ok := MXRender("Un cambio a la subpartida 3808.50 de cualquier otra subpartida, siempre que el bien sea el resultado de una reacción química."); ok {
		t.Error("a proviso in Spanish was rendered")
	}
	if got := mxColumn("2905.22-905.29"); got != "2905.22-2905.29" {
		t.Errorf("mxColumn: %q", got)
	}
}

// The committed alternatives are what the parser makes of the committed
// text, and the coverage is the one D914 states.
func TestRulesMXJSONMatchesParser(t *testing.T) {
	tab := mustMexico(t).Table()
	if tab.Version != MexicoTableVersion || tab.Source.SHA256 == "" || !strings.Contains(tab.Source.Licence, "DOF") {
		t.Fatalf("source: %+v version %q", tab.Source, tab.Version)
	}
	re, err := Reparse(tab)
	if err != nil {
		t.Fatal(err)
	}
	for i := range re.Rules {
		if !reflect.DeepEqual(re.Rules[i], tab.Rules[i]) {
			a, _ := json.Marshal(re.Rules[i])
			b, _ := json.Marshal(tab.Rules[i])
			t.Fatalf("rule %s: parser gives\n%s\ncommitted\n%s", tab.Rules[i].HTS, a, b)
		}
	}
	c := CoverageOf(tab)
	if c.Rows != 943 || c.RowsFull != 788 || c.RowsNone != 155 {
		t.Fatalf("coverage %+v; D914 states 943 rows, 788 encoded, 155 not", c)
	}
	for _, r := range tab.Rules {
		if r.TextES == "" {
			t.Fatalf("row %s has no Spanish text", r.HTS)
		}
		if r.Text == r.TextES && r.Alternatives[0].Encoded {
			t.Fatalf("row %s encoded from Spanish text", r.HTS)
		}
	}
	if tab.Chapters[0] != "25" || tab.Chapters[len(tab.Chapters)-1] != "97" {
		t.Fatalf("chapters %v", tab.Chapters)
	}
}

func TestMexicoEvaluate(t *testing.T) {
	r := mustMexico(t)
	// 8483.20 from any other subheading except 8482.10-8482.80: bearings
	// of 8482.10 do not shift; a casting of 7325 does.
	got := r.Evaluate(Input{GoodHTS: "8483.20", Country: "Vietnam", Materials: []Material{{Name: "housing", HTS: "7325.99", Origin: "China"}}})
	if got.Status != Decided || got.Origin != "Vietnam" || !strings.Contains(got.Basis, "Anexo I, art. 3 I c)") {
		t.Fatalf("shift: %+v", got)
	}
	got = r.Evaluate(Input{GoodHTS: "8483.20", Country: "Vietnam", Materials: []Material{{Name: "bearing", HTS: "8482.10", Origin: "China"}}})
	if got.Status == Decided && got.Origin == "Vietnam" {
		t.Fatalf("no shift decided for Vietnam: %+v", got)
	}
	// No de minimis in the Mexican text: a tiny non-shifting share still
	// fails the shift.
	share := 0.01
	got = r.Evaluate(Input{GoodHTS: "8483.20", Country: "Vietnam", Materials: []Material{
		{Name: "housing", HTS: "7325.99", Origin: "China", ValueShare: &share},
		{Name: "bearing", HTS: "8482.10", Origin: "China", ValueShare: &share},
	}})
	if got.Status == Decided && got.Origin == "Vietnam" {
		t.Fatalf("de minimis applied: %+v", got)
	}
	// Chapters 1 to 24 have no rows: the general test (art. 3 II) needs a
	// person.
	got = r.Evaluate(Input{GoodHTS: "2009.90", Country: "Vietnam", Materials: []Material{{Name: "oranges", HTS: "0805.10", Origin: "Brazil"}}})
	if got.Status != OutOfScope {
		t.Fatalf("chapter 20: %+v", got)
	}
}
