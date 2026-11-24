package euorigin

import (
	"encoding/json"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/nporigin"
)

func load(t *testing.T) *nporigin.Rules {
	t.Helper()
	r, err := Rules()
	if err != nil {
		t.Fatal(err)
	}
	return r
}

// TestRulesEUJSON: the committed table re-parses to itself, so a parser
// change cannot drift from the data (the source XHTML is not committed).
func TestRulesEUJSON(t *testing.T) {
	var tab nporigin.Table
	if err := json.Unmarshal(rulesJSON, &tab); err != nil {
		t.Fatal(err)
	}
	for _, ch := range tab.Chapters {
		for _, row := range ch.Rows {
			applies, ex, split, key, err := nporigin.ParseCode(row.Code)
			if err != nil || key != row.Key || ex != row.Ex || split != row.Split || !reflect.DeepEqual(applies, row.Applies) {
				t.Errorf("%s: code re-parse %v %v %q %q %v", row.Code, applies, ex, split, key, err)
			}
			if got := nporigin.ParseRule(row.Key, row.Text); !reflect.DeepEqual(got, row.Alts) {
				t.Errorf("%s: rule re-parse differs:\n got %+v\nwant %+v", row.Code, got, row.Alts)
			}
		}
	}
}

// TestCoverageEU pins what the table encodes (D763). A change here is a
// change to the published coverage: update docs/scope-and-jurisdiction.md
// and docs/NUMBERS.md with it.
func TestCoverageEU(t *testing.T) {
	tab := load(t).Table()
	want := []string{"02", "04", "09", "11", "14", "17", "20", "22", "34", "35", "42", "49", "50", "51", "52", "53", "54", "55", "56", "57", "58", "59", "60", "61", "62", "63", "64", "69", "71", "72", "73", "82", "84", "85", "90", "91", "94"}
	if got := tab.ChapterCodes(); !slices.Equal(got, want) {
		t.Errorf("chapters %v", got)
	}
	c, un := nporigin.CoverageOf(tab)
	if c.Rows != 446 || c.RowsByCode != 182 || c.RowsWithProcess != 259 || c.RowsUnread != 5 || c.Alternatives != 542 || c.ByCode != 199 || c.Process != 338 || c.Unread != 5 {
		t.Errorf("coverage %+v", c)
	}
	var rows []string
	for _, u := range un {
		rows = append(rows, u.Row)
	}
	if !slices.Equal(rows, []string{"ex 7205 21 (a)", "ex 7205 29 (a)", "ex 7222 (c)", "ex 7226 (d)", "ex 7228 (c)"}) {
		t.Errorf("unread %v", rows)
	}
	s := tab.Source
	if s.SHA256 != "87be4f1a432bc0ef35a6c27a94d237a65fc41d036278cf43fd4889a4f79818be" || !strings.Contains(s.Version, CELEX) || s.AsOf != Consolidation {
		t.Errorf("source %+v", s)
	}
}

func f(v float64) *float64 { return &v }

// Synthetic fixtures: invented goods run through the committed EU table.
// Neither Annex 22-01 nor the DA has worked examples to use instead.
func TestEUFixtures(t *testing.T) {
	r := load(t)
	yes := true
	for _, c := range []struct {
		name   string
		in     nporigin.Input
		status string
		origin string
		need   string
	}{
		{"synthetic: radio assembled in Malaysia from Chinese boards and speakers (8527, CTH except 8529)",
			nporigin.Input{GoodHTS: "8527.13", Country: "Malaysia", Materials: []nporigin.Material{{Name: "board", HTS: "8534.00", Origin: "China"}, {Name: "speaker", HTS: "8518.29", Origin: "China"}}},
			nporigin.Decided, "Malaysia", ""},
		{"synthetic: radio from a Chinese tuner of 8529 worth 70%: residual rule by value",
			nporigin.Input{GoodHTS: "8527.13", Country: "Malaysia", Answers: map[string]bool{"ch85.r": false}, Materials: []nporigin.Material{{Name: "tuner", HTS: "8529.90", Origin: "China", ValueShare: f(0.7)}, {Name: "case", HTS: "3926.90", Origin: "Malaysia", ValueShare: f(0.2)}}},
			nporigin.Decided, "China", ""},
		{"synthetic: laptop (8471) is not in Annex 22-01: substantial-transformation judgement",
			nporigin.Input{GoodHTS: "8471.30", Country: "Vietnam", Materials: []nporigin.Material{{Name: "board", HTS: "8473.30", Origin: "China"}}},
			nporigin.NeedsJudgement, "", ""},
		{"synthetic: laptop, only minimal operations in Vietnam: Art. 34 DA by value",
			nporigin.Input{GoodHTS: "8471.30", Country: "Vietnam", MinimalOnly: &yes, Materials: []nporigin.Material{{Name: "board", HTS: "8473.30", Origin: "China", ValueShare: f(0.8)}, {Name: "box", HTS: "4819.10", Origin: "Vietnam", ValueShare: f(0.05)}}},
			nporigin.Decided, "China", ""},
		{"synthetic: 8473 part: is it a memory module (ex 8473)?",
			nporigin.Input{GoodHTS: "8473.30", Country: "Taiwan", Materials: []nporigin.Material{{Name: "chips", HTS: "8542.32", Origin: "South Korea"}}},
			nporigin.Undetermined, "", "row"},
		{"synthetic: memory module from Korean chips (CTH)",
			nporigin.Input{GoodHTS: "8473.30", Country: "Taiwan", Row: "8473x", Materials: []nporigin.Material{{Name: "chips", HTS: "8542.32", Origin: "South Korea"}}},
			nporigin.Decided, "Taiwan", ""},
		{"synthetic: 8473 part that is not a memory module",
			nporigin.Input{GoodHTS: "8473.30", Country: "Taiwan", Row: "none", Materials: []nporigin.Material{{Name: "chips", HTS: "8542.32", Origin: "South Korea"}}},
			nporigin.NeedsJudgement, "", ""},
		{"synthetic: trainers (6404) from Vietnamese uppers of 6406: is it only assembly of uppers?",
			nporigin.Input{GoodHTS: "6404.11", Country: "Cambodia", Materials: []nporigin.Material{{Name: "uppers", HTS: "6406.10", Origin: "Vietnam"}}},
			nporigin.Undetermined, "", "6404.1.c"},
		{"synthetic: wire (7217) cold-drawn in Poland from Ukrainian rod",
			nporigin.Input{GoodHTS: "7217.10", Country: "Poland", Answers: map[string]bool{"7217.2.c": true}, Materials: []nporigin.Material{{Name: "rod", HTS: "7213.91", Origin: "Ukraine", ValueShare: f(0.8)}}},
			nporigin.Decided, "Poland", ""},
		{"synthetic: bovine meat (0201) from French cattle, not fattened three months in Germany: chapter note, review",
			nporigin.Input{GoodHTS: "0201.10", Country: "Germany", Answers: map[string]bool{"0201.1": false}, Materials: []nporigin.Material{{Name: "cattle", HTS: "0102.29", Origin: "France", WeightShare: f(1)}}},
			nporigin.Undetermined, "", "review"},
		{"synthetic: wheat flour (1101) milled in Italy from Canadian wheat (CC)",
			nporigin.Input{GoodHTS: "1101.00", Country: "Italy", Materials: []nporigin.Material{{Name: "wheat", HTS: "1001.99", Origin: "Canada"}}},
			nporigin.Decided, "Italy", ""},
		{"synthetic: shirt (6205) sewn in Bangladesh from Chinese fabric: split heading",
			nporigin.Input{GoodHTS: "6205.20", Country: "Bangladesh", Materials: []nporigin.Material{{Name: "fabric", HTS: "5208.21", Origin: "China"}}},
			nporigin.Undetermined, "", "row"},
	} {
		res := r.Evaluate(c.in)
		if res.Status != c.status || res.Origin != c.origin {
			t.Errorf("%s: got %s %q (%s)", c.name, res.Status, res.Origin, res.Reason)
			continue
		}
		if c.need != "" && !slices.ContainsFunc(res.Needs, func(n nporigin.Need) bool { return n.Key == c.need }) {
			t.Errorf("%s: want need %s, got %+v", c.name, c.need, res.Needs)
		}
	}
}

func TestEUJudgementCitesArt60(t *testing.T) {
	res := load(t).Evaluate(nporigin.Input{GoodHTS: "8471.30", Country: "Vietnam", Materials: []nporigin.Material{{HTS: "8473.30", Origin: "China"}}})
	if !strings.Contains(res.Basis, "Art. 60(2) UCC") || !strings.Contains(res.Reason, "judgement") {
		t.Errorf("got %+v", res)
	}
}

// The two HS 2017 rows are read in HS 2022 (D767): a semiconductor imaging
// component of 8525 89 and a smartconnect module of 8548 00 reach them.
func TestHS2017RowsReadInHS2022(t *testing.T) {
	r := load(t)
	for _, c := range []struct{ good, row string }{{"8525.89", "852580x"}, {"8525.81", "852580x"}, {"8548.00", "854890x"}} {
		res := r.Evaluate(nporigin.Input{GoodHTS: c.good, Country: "Malaysia", Row: c.row, Materials: []nporigin.Material{{HTS: "8542.31", Origin: "Taiwan"}}})
		if res.Status != nporigin.Decided || res.Rule == nil || res.Rule.Key != c.row {
			t.Errorf("%s: %+v", c.good, res)
		}
	}
	if res := r.Evaluate(nporigin.Input{GoodHTS: "8525.89", Country: "Malaysia", Materials: []nporigin.Material{{HTS: "8542.31", Origin: "Taiwan"}}}); res.Needs[0].Kind != nporigin.NeedRow {
		t.Errorf("an ex row must be picked: %+v", res)
	}
}
