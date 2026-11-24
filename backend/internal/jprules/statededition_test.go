package jprules

import (
	"encoding/json"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"
)

func ip(n int) *int { return &n }

// The declared edition is a typed field. The only schedules without one are
// the Japan-US Agreement and GSP, and the field is present in the data.
func TestStatedEditionIsTypedAndNullOnlyForJPUSAndGSP(t *testing.T) {
	var raw []map[string]json.RawMessage
	if err := json.Unmarshal(schedulesJSON, &raw); err != nil {
		t.Fatal(err)
	}
	var parsed []schedule
	if err := json.Unmarshal(schedulesJSON, &parsed); err != nil {
		t.Fatal(err)
	}
	var nulls []string
	for i, s := range parsed {
		if _, ok := raw[i]["stated_edition"]; !ok {
			t.Errorf("%s: stated_edition key missing", s.Agreement)
		}
		if s.StatedEdition == nil {
			nulls = append(nulls, s.Agreement)
			if !strings.Contains(s.Citation, "not stated") {
				t.Errorf("%s: null edition without 'not stated'", s.Agreement)
			}
			continue
		}
		if !strings.Contains(s.Citation, "(HS "+strconv.Itoa(*s.StatedEdition)+")") {
			t.Errorf("%s: citation %q disagrees with %d", s.Agreement, s.Citation, *s.StatedEdition)
		}
	}
	sort.Strings(nulls)
	if !reflect.DeepEqual(nulls, []string{"GSP", "JP-US TA"}) {
		t.Fatalf("null set %v", nulls)
	}
	// GSP: stated null while the validation list is 2022; both coexist.
	for _, s := range parsed {
		if s.Agreement == "GSP" && (s.StatedEdition != nil || s.HSEdition != 2022) {
			t.Errorf("GSP stated %v validation %d", s.StatedEdition, s.HSEdition)
		}
	}
}

func TestStatedEditionLoaderValidation(t *testing.T) {
	base := func(cit string, st *int) schedule {
		return schedule{Agreement: "A", Citation: cit, URL: "https://x.example/a.pdf", From: "2020-01-01", StatedEdition: st}
	}
	ok := []schedule{
		base("A Annex (HS 2017)", ip(2017)),
		base("A Annex (HS 2002)", ip(2002)),
		base("A Annex (HS 2007)", ip(2007)),
		base("A Annex (HS 2012)", ip(2012)),
		base("A Annex (HS 2022)", ip(2022)),
		base("A Annex, edition not stated; validated against HS 2022", nil),
		// Metamorphic: surrounding wording does not matter.
		base("validated against HS 2022; A Annex (HS 2017)", ip(2017)),
		base("(HS 2017) A Annex, HS 2012 in prose", ip(2017)),
	}
	for _, s := range ok {
		if err := validateStatedEdition(s); err != nil {
			t.Errorf("%q: %v", s.Citation, err)
		}
	}
	bad := map[string]schedule{
		"mismatch":           base("A Annex (HS 2012)", ip(2017)),
		"prose only":         base("A Annex, validated against HS 2022", ip(2022)),
		"unknown edition":    base("A Annex (HS 2023)", ip(2023)),
		"zero":               base("A Annex (HS 0)", ip(0)),
		"null with edition":  base("A Annex (HS 2017)", nil),
		"null without words": base("A Annex", nil),
	}
	for name, s := range bad {
		if err := validateStatedEdition(s); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
}

// Wording around the edition never changes the derived edition.
func TestStatedEditionIgnoresCitationProse(t *testing.T) {
	table := mustTable(t)
	for _, s := range table.schedules["GSP"] {
		if s.StatedEdition != nil {
			t.Fatal("GSP must not read 'validated against HS 2022' as stated")
		}
		s.Citation = "GSP (HS 2017), validated against HS 2022"
		if s.StatedEdition != nil {
			t.Fatal("citation edit changed the typed edition")
		}
		if validateStatedEdition(s) == nil {
			t.Fatal("null with an HS edition in the citation accepted")
		}
	}
}

// parseGeneral matches the AJCEP window on the typed edition, not the prose.
func TestParseGeneralUsesTypedStatedEdition(t *testing.T) {
	table := mustTable(t)
	raw, err := json.Marshal(table.general)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := parseGeneral(raw, table.schedules); err != nil {
		t.Fatal(err)
	}
	mod := map[string][]schedule{}
	for k, v := range table.schedules {
		mod[k] = append([]schedule(nil), v...)
	}
	for i, s := range mod["AJCEP"] {
		// Prose says HS 2017 but the typed edition says 2022: no match for 2017.
		if s.StatedEdition != nil && *s.StatedEdition == 2017 {
			mod["AJCEP"][i].StatedEdition = ip(2022)
		}
	}
	if _, err := parseGeneral(raw, mod); err == nil {
		t.Fatal("window matched on prose instead of the typed edition")
	}
	// Reworded citation with the typed edition intact still matches.
	mod2 := map[string][]schedule{}
	for k, v := range table.schedules {
		mod2[k] = append([]schedule(nil), v...)
	}
	for i := range mod2["AJCEP"] {
		mod2["AJCEP"][i].Citation = "reworded"
	}
	if _, err := parseGeneral(raw, mod2); err != nil {
		t.Fatalf("typed edition not used: %v", err)
	}
}
