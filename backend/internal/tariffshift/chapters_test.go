package tariffshift

import (
	"encoding/json"
	"os"
	"reflect"
	"slices"
	"strings"
	"testing"
)

// Chapters added in D460. Each example is checked by hand against the row's
// eCFR text (as of 18 Nov 2026); the goods and codes are illustrative, not
// taken from a ruling.

func TestAddedChaptersEncodings(t *testing.T) {
	// 2106.90: a proviso on sugar content is not a code test, so the
	// sentence is not encoded (it once read "65 percent" as chapter 65).
	for _, a := range rule(t, "2106.90").Alternatives {
		if a.Encoded {
			t.Fatalf("2106.90 alternative encoded: %+v", a)
		}
	}
	// 3922-3926: the exception is for goods of heading 9619 only; headings
	// 3901 to 3914 describe their material.
	p := rule(t, "3922-3926").Alternatives[0]
	if !p.Encoded || p.From != 6 || len(p.Except) != 1 ||
		!reflect.DeepEqual(p.Except[0].From, []Range{{"9619", "9619"}}) || !reflect.DeepEqual(p.Except[0].To, []Range{{"3926", "3926"}}) || !p.Except[0].Described {
		t.Fatalf("3922-3926 %+v", p)
	}
	// The chapter 39 note sends three codes to 102.21.
	if got := rule(t, "3920.10-3921.90").Textile; !reflect.DeepEqual(got, []Range{{"39211215", "39211215"}, {"39211315", "39211315"}, {"3921902550", "3921902550"}}) {
		t.Fatalf("3920.10-3921.90 textile %v", got)
	}
	// 7107: "except from Chapter 72 through 76 or Chapter 78 through 83".
	if e := rule(t, "7107").Alternatives[0].Except; len(e) != 1 || !reflect.DeepEqual(e[0].From, []Range{{"72", "76"}, {"78", "83"}}) {
		t.Fatalf("7107 %+v", e)
	}
	// The erratum row keeps its published column and reads as 4413-4421.
	if r := rule(t, "4441-4421"); r.Applies != (Range{"4413", "4421"}) || r.ID != "19 CFR 102.20(i), 4441-4421" {
		t.Fatalf("4441-4421 %+v", r)
	}
	// Rows whose every sentence names goods in words stay undecidable.
	for _, h := range []string{"9503", "4811", "4823.90", "3920.10-3921.90", "4413-4421", "4441-4421", "4002.80-4002.99", "7116", "3901-3915"} {
		for _, a := range rule(t, h).Alternatives {
			if a.Encoded {
				t.Errorf("%s: encoded %q", h, a.Text)
			}
		}
	}
}

func TestAddedChaptersExamples(t *testing.T) {
	r := mustDefault(t)
	mat := func(name, hts, origin string) Material { return Material{Name: name, HTS: hts, Origin: origin} }
	for _, c := range []struct {
		name    string
		in      Input
		status  string
		origin  string
		matWant []string
		reason  string
	}{
		// 3922-3926: from any other subheading. Resin of 3902.10 shifts.
		{"39 moulded article from resin", Input{GoodHTS: "3926.90.99", Country: "Mexico", Materials: []Material{mat("polypropylene resin", "3902.10", "China")}},
			Decided, "Mexico", []string{MatShift}, ""},
		// 3919: outside that group. Film of 3920.62 shifts; a jumbo roll of 3919.90 does not.
		{"39 tape from film", Input{GoodHTS: "3919.10", Country: "Canada", Materials: []Material{mat("PET film", "3920.62", "Korea"), mat("adhesive", "3506.91", "Canada")}},
			Decided, "Canada", []string{MatShift, MatShift}, ""},
		{"39 tape from a jumbo roll", Input{GoodHTS: "3919.10", Country: "Canada", Materials: []Material{{Name: "jumbo roll", HTS: "3919.90", Origin: "Korea", ValueShare: f(0.9)}}},
			Decided, "South Korea", []string{MatNoShift}, ""},
		// Chapter 39 note: 3921.12.15 is a 102.21 good.
		{"39 textile-note code", Input{GoodHTS: "3921.12.15", Country: "Mexico", Materials: []Material{mat("fabric", "5407.61", "China")}},
			OutOfScope, "", nil, "102.21"},
		{"39 code too short for the note", Input{GoodHTS: "3921.12", Country: "Mexico", Materials: []Material{mat("fabric", "5407.61", "China")}},
			Undetermined, "", nil, "give the full code"},
		// 4016.10-4016.99: from any other subheading. 4005 from 4001 or 4002 is barred.
		{"40 gasket from synthetic rubber", Input{GoodHTS: "4016.93", Country: "Canada", Materials: []Material{mat("synthetic rubber", "4002.19", "Korea")}},
			Decided, "Canada", []string{MatShift}, ""},
		{"40 compounded rubber from 4002", Input{GoodHTS: "4005.10", Country: "Canada", Materials: []Material{{Name: "synthetic rubber", HTS: "4002.19", Origin: "Korea", ValueShare: f(0.8)}}},
			Decided, "South Korea", []string{MatNoShift}, ""},
		{"40 proviso on rubber content", Input{GoodHTS: "4002.99", Country: "Canada", Materials: []Material{mat("latex", "4001.10", "Malaysia")}},
			Undetermined, "", []string{MatNeedsFact}, ""},
		// 4401-4411: from any other heading; 4413-4421 has two rows.
		{"44 mouldings from logs", Input{GoodHTS: "4409.10", Country: "Canada", Materials: []Material{mat("logs", "4403.21", "United States")}},
			Decided, "Canada", []string{MatShift}, ""},
		{"44 two rows", Input{GoodHTS: "4421.99", Country: "Canada", Materials: []Material{mat("lumber", "4407.11", "United States")}},
			Undetermined, "", nil, "more than one rule covers"},
		// 4817-4822: from any other heading.
		{"48 cartons from kraft paper", Input{GoodHTS: "4819.10", Country: "Mexico", Materials: []Material{mat("kraft paper", "4804.11", "United States")}},
			Decided, "Mexico", []string{MatShift}, ""},
		// 7113.11-7115.90 from any other subheading; 7107 not from chapters 72-76.
		{"71 gold ring from gold", Input{GoodHTS: "7113.19", Country: "Mexico", Materials: []Material{mat("gold", "7108.13", "Switzerland")}},
			Decided, "Mexico", []string{MatShift}, ""},
		{"71 clad base metal from copper", Input{GoodHTS: "7107.00", Country: "Mexico", Materials: []Material{{Name: "copper strip", HTS: "7409.11", Origin: "Chile", ValueShare: f(0.7)}}},
			Decided, "Chile", []string{MatNoShift}, ""},
		// 7616: from any other subheading; 7605 not from 7604.
		{"76 bracket from sheet", Input{GoodHTS: "7616.99", Country: "Mexico", Materials: []Material{mat("aluminium sheet", "7606.12", "China")}},
			Decided, "Mexico", []string{MatShift}, ""},
		{"76 wire from bar", Input{GoodHTS: "7605.11", Country: "Canada", Materials: []Material{{Name: "aluminium bar", HTS: "7604.10", Origin: "China", ValueShare: f(0.9)}}},
			Decided, "China", []string{MatNoShift}, ""},
		// 8302: from any other subheading. 8301.10-8301.50 not from 8301.60 under GRI 2(a).
		{"83 hinge from steel", Input{GoodHTS: "8302.10", Country: "Mexico", Materials: []Material{mat("steel strip", "7211.23", "China")}},
			Decided, "Mexico", []string{MatShift}, ""},
		{"83 padlock from lock parts, GRI 2(a) unknown", Input{GoodHTS: "8301.10", Country: "Mexico", Materials: []Material{mat("lock parts", "8301.60", "China")}},
			Undetermined, "", []string{MatNeedsFact}, ""},
		{"83 padlock from lock parts, not GRI 2(a)", Input{GoodHTS: "8301.10", Country: "Mexico", GRI2a: b(false), Materials: []Material{mat("lock parts", "8301.60", "China")}},
			Decided, "Mexico", []string{MatShift}, ""},
		// 9506.31 not from 9506.39; 9503 is all by description.
		{"95 golf clubs from club parts", Input{GoodHTS: "9506.31", Country: "Mexico", Materials: []Material{{Name: "heads and shafts", HTS: "9506.39", Origin: "China", ValueShare: f(0.7)}}},
			Decided, "China", []string{MatNoShift}, ""},
		{"95 exercise bench from tubes", Input{GoodHTS: "9506.91", Country: "Mexico", Materials: []Material{mat("steel tubes", "7306.30", "China")}},
			Decided, "Mexico", []string{MatShift}, ""},
		{"95 toy", Input{GoodHTS: "9503.00", Country: "Mexico", Materials: []Material{mat("resin", "3903.20", "China")}},
			Undetermined, "", []string{MatNeedsFact}, ""},
		// 1501-1516 from any other chapter; de minimis does not apply in chapter 15.
		{"15 oil from seeds", Input{GoodHTS: "1512.11", Country: "Canada", Materials: []Material{mat("sunflower seeds", "1206.00", "Ukraine")}},
			Decided, "Canada", []string{MatShift}, ""},
		{"15 blend from palm oil", Input{GoodHTS: "1516.20", Country: "Canada", Materials: []Material{{Name: "palm oil", HTS: "1511.10", Origin: "Malaysia", ValueShare: f(0.05)}}},
			Decided, "Malaysia", []string{MatNoShift}, ""},
		// 2103.90 from any other subheading; 2106.90 is not decided by code.
		{"21 sauce from pepper", Input{GoodHTS: "2103.90", Country: "Mexico", Materials: []Material{mat("chili peppers", "0904.22", "India")}},
			Decided, "Mexico", []string{MatShift}, ""},
		{"21 food preparation", Input{GoodHTS: "2106.90", Country: "Mexico", Materials: []Material{mat("sugar", "1701.99", "Brazil")}},
			Undetermined, "", []string{MatNeedsFact}, ""},
	} {
		res := r.Evaluate(c.in)
		if res.Status != c.status || res.Origin != c.origin || (c.reason != "" && !strings.Contains(res.Reason, c.reason)) {
			t.Errorf("%s: %s %q %q", c.name, res.Status, res.Origin, res.Reason)
			continue
		}
		var got []string
		for _, m := range res.Materials {
			got = append(got, m.Status)
		}
		if c.matWant != nil && !slices.Equal(got, c.matWant) {
			t.Errorf("%s: materials %v, want %v", c.name, got, c.matWant)
		}
	}
}

// The encoded chapters are D360's seven plus those chapter_counts.py picks
// from development and earlier data (D460).
func TestScopeMatchesChapterCounts(t *testing.T) {
	b, err := os.ReadFile("../../../eval/tariffshift/chapter-counts.json")
	if err != nil {
		t.Fatal(err)
	}
	var c struct {
		Before []string `json:"chapters_before"`
		Added  []string `json:"chapters_added"`
	}
	if err := json.Unmarshal(b, &c); err != nil {
		t.Fatal(err)
	}
	want := append(c.Before, c.Added...)
	slices.Sort(want)
	if !slices.Equal(want, ScopeChapters) {
		t.Fatalf("ScopeChapters %v, chapter-counts.json gives %v", ScopeChapters, want)
	}
}
