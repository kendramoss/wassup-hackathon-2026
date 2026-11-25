package ukorigin

import (
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

// TestRulesUKJSON: the committed table re-parses to itself (the .docx is
// not committed).
func TestRulesUKJSON(t *testing.T) {
	tab := load(t).Table()
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

// TestCoverageUK pins what the table encodes (D763).
func TestCoverageUK(t *testing.T) {
	tab := load(t).Table()
	want := []string{"02", "04", "09", "11", "14", "17", "20", "22", "34", "35", "42", "49", "50", "51", "52", "53", "54", "55", "56", "57", "58", "59", "60", "61", "62", "63", "64", "69", "71", "72", "73", "82", "84", "85", "90", "91", "94"}
	if got := tab.ChapterCodes(); !slices.Equal(got, want) {
		t.Errorf("chapters %v", got)
	}
	c, un := nporigin.CoverageOf(tab)
	if c.Rows != 452 || c.RowsByCode != 184 || c.RowsWithProcess != 263 || c.RowsUnread != 5 || c.Alternatives != 552 || c.ByCode != 205 || c.Process != 342 || c.Unread != 5 {
		t.Errorf("coverage %+v", c)
	}
	var rows []string
	for _, u := range un {
		rows = append(rows, u.Row)
	}
	if !slices.Equal(rows, []string{"ex 7205 21 (a)", "ex 7205 29 (a)", "ex 72 22 (c)", "ex 72 26 (d)", "ex 72 28 (c)"}) {
		t.Errorf("unread %v", rows)
	}
	s := tab.Source
	if s.SHA256 != "239b6b3d103dd4d22bfc639638759bb41a6ba98233540e150c79b912d22b850d" || !strings.Contains(s.Version, "1.2") || !strings.Contains(s.Licence, "Open Government Licence v3.0") {
		t.Errorf("source %+v", s)
	}
	if !strings.Contains(s.Note, "7b806032c37b28ac2d988f69c1d53d3d300f7a846951fd756808fa5e214636da") || !strings.Contains(s.Instrument, "SI 2020/1433") {
		t.Errorf("instrument pin missing: %+v", s)
	}
}

func f(v float64) *float64 { return &v }

// Synthetic fixtures: invented goods run through the committed UK table.
// The reference document has no worked examples, and HMRC publishes no
// origin rulings.
func TestUKFixtures(t *testing.T) {
	r := load(t)
	yes := true
	for _, c := range []struct {
		name   string
		in     nporigin.Input
		status string
		origin string
		need   string
	}{
		{"synthetic: flat panel display module (8524, UK only) from Chinese parts",
			nporigin.Input{GoodHTS: "8524.11", Country: "Vietnam", Materials: []nporigin.Material{{Name: "glass", HTS: "7007.19", Origin: "China"}, {Name: "driver", HTS: "8542.31", Origin: "China"}}},
			nporigin.Decided, "Vietnam", ""},
		{"synthetic: life-jacket (6307 20) where non-originating materials are 35% (value rule read as non-originating, Part Four value terms)",
			nporigin.Input{GoodHTS: "6307.20", Country: "Turkey", Materials: []nporigin.Material{{Name: "fabric", HTS: "5903.20", Origin: "India", ValueShare: f(0.35)}, {Name: "foam", HTS: "3921.13", Origin: "Turkey", ValueShare: f(0.2)}}},
			nporigin.Decided, "Turkey", ""},
		{"synthetic: laptop (8471) is not in Part Four: substantial-transformation judgement",
			nporigin.Input{GoodHTS: "8471.30", Country: "Vietnam", Materials: []nporigin.Material{{Name: "board", HTS: "8473.30", Origin: "China"}}},
			nporigin.NeedsJudgement, "", ""},
		{"synthetic: laptop, only Part Two operations in Vietnam: the UK text gives no fallback, judgement",
			nporigin.Input{GoodHTS: "8471.30", Country: "Vietnam", MinimalOnly: &yes, Materials: []nporigin.Material{{Name: "board", HTS: "8473.30", Origin: "China", ValueShare: f(0.8)}}},
			nporigin.NeedsJudgement, "", ""},
		{"synthetic: hearing-aid part (ex 9021.10 is orthopaedic; 9021.40 has no row)",
			nporigin.Input{GoodHTS: "9021.40", Country: "Denmark", Materials: []nporigin.Material{{HTS: "8518.90", Origin: "China"}}},
			nporigin.NeedsJudgement, "", ""},
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
	res := r.Evaluate(nporigin.Input{GoodHTS: "8471.30", Country: "Vietnam", Materials: []nporigin.Material{{HTS: "8473.30", Origin: "China"}}})
	if !strings.Contains(res.Basis, "s. 17(3)") {
		t.Errorf("judgement must cite TCTA 2018 s. 17(3): %+v", res)
	}
}
