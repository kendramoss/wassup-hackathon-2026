package jprules

import (
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"
)

// JP-Australia EPA Annex 2 column 4 (alternative rules of chapters 27-40) and
// the General Notes that define the rule codes, pinned to the official PDF
// (australia2.pdf). See docs/japan/australia-column4.md.

const (
	australiaFrom  = "2015-01-15"
	australiaLabel = "; alternative (column 4): "
)

var australiaDate = time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

func australiaSchedule(t *testing.T) schedule {
	t.Helper()
	return scheduleFrom(t, mustDefault(t), "JP-Australia EPA", australiaFrom)
}

// column4Text returns the labelled column-4 part of a row text, or "".
func column4Text(text string) string {
	_, c4, ok := strings.Cut(text, australiaLabel)
	if !ok {
		return ""
	}
	return c4
}

func TestAustraliaColumn4PinnedRows(t *testing.T) {
	s := australiaSchedule(t)
	cases := []struct {
		start, end string
		page       int
		text       string
	}{
		{"2701", "2701", 21, "Rule (column 3): CTH; alternative (column 4): CR"},
		{"271012", "271012", 22, "Rule (column 3): CTH, or a change to any good of heading 27.10 from any other good of heading 27.10, provided that the good resulting from such a change is the product of a chemical reaction, atmospheric distillation or vacuum distillation; alternative (column 4): CR or D"},
		{"271099", "271099", 23, "Rule (column 3): CTH, or CTSH provided that the good resulting from such a change is the product of a chemical reaction, atmospheric distillation or vacuum distillation; alternative (column 4): CR or D"},
		{"271500", "271500", 23, "Rule (column 3): CTH except from heading 27.14 or subheading 2713.20; alternative (column 4): CR"},
		{"2801", "2801", 23, "Rule (column 3): CTSH; alternative (column 4): CR, P, SM or IS"},
		{"280700", "280700", 23, "Rule (column 3): CTH or QVC 40; alternative (column 4): CR, P, SM or IS"},
		{"2907", "2907", 30, "Rule (column 3): CTSH; alternative (column 4): CR, P, SM or IS"},
		{"3302", "3302", 40, "Rule (column 3): CTH except from subheading 2106.90 or heading 22.07, 22.08 or 33.01; alternative (column 4): CR or P"},
		{"3507", "3507", 43, "Rule (column 3): CTSH; alternative (column 4): CR, P, SM or IS"},
		{"36", "36", 43, "Rule (column 3): CTH or QVC 40; alternative (column 4): CR"},
		{"3801", "3801", 44, "Rule (column 3): CTSH; alternative (column 4): CR, P or SM"},
		{"382600", "382600", 47, "Rule (column 3): CTSH or QVC 40; alternative (column 4): CR, P or SM"},
		{"3901", "3901", 47, "Rule (column 3): CTH; alternative (column 4): CR, P or IS"},
		{"3915", "3915", 48, "Rule (column 3): CTH; alternative (column 4): CR, P, CPS or IS"},
		{"392690", "392690", 50, "Rule (column 3): CTH; alternative (column 4): CR, P, CPS or IS"},
		{"401700", "401700", 52, "Rule (column 3): CTH; alternative (column 4): CR"},
	}
	for _, c := range cases {
		got := rowsAt(s, c.start, c.end, c.page)
		if len(got) != 1 || got[0].Text != c.text {
			t.Errorf("%s-%s p%d: %+v", c.start, c.end, c.page, got)
		}
	}
	// A row without a column 4 keeps its plain text.
	if got := rowsAt(s, "01", "01", 11); len(got) != 1 || got[0].Text != "CC" {
		t.Errorf("chapter 1: %+v", got)
	}
	// Column 4 reaches the model through a lookup.
	ev, err := mustDefault(t).Lookup("JP-Australia EPA", "2710.12", australiaDate)
	if err != nil || len(ev) == 0 || column4Text(ev[0].Text) != "CR or D" || !strings.HasSuffix(ev[0].URL, "#page=22") {
		t.Fatalf("2710.12: %+v %v", ev, err)
	}
}

// Every non-empty column 4 cell of the PDF (testdata is dumped from the raw
// table cells by eval/jp/australia_column4_truth.py) is in exactly the row of
// its page, and no other row carries one.
func TestAustraliaEveryPdfColumn4IsKept(t *testing.T) {
	raw, err := os.ReadFile("testdata/australia-column4.tsv")
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	if len(lines) != 530 {
		t.Fatalf("truth has %d rows", len(lines))
	}
	s := australiaSchedule(t)
	matched := map[int]bool{}
	for _, line := range lines {
		f := strings.Split(line, "\t")
		page, _ := strconv.Atoi(f[0])
		found := 0
		for i, r := range s.Rows {
			if r.Start == f[1] && r.End == f[1] && r.Page == page && column4Text(r.Text) == f[2] {
				found++
				matched[i] = true
			}
		}
		if found != 1 {
			t.Errorf("p%s %s: column 4 %q found in %d rows", f[0], f[1], f[2], found)
		}
	}
	for i, r := range s.Rows {
		if column4Text(r.Text) != "" && !matched[i] {
			t.Errorf("row %s-%s p%d carries a column 4 the PDF does not have", r.Start, r.End, r.Page)
		}
		if !matched[i] && strings.Contains(r.Text, "column 4") {
			t.Errorf("row %s-%s p%d: stray column label %q", r.Start, r.End, r.Page, r.Text)
		}
		if matched[i] {
			if ch, _ := strconv.Atoi(r.Start[:2]); ch < 27 || ch > 40 || !strings.HasPrefix(r.Text, "Rule (column 3): ") {
				t.Errorf("row %s-%s p%d: %q", r.Start, r.End, r.Page, r.Text)
			}
		}
	}
}

func TestAustraliaColumn4RowsPerChapter(t *testing.T) {
	// Rows with a column 4 per chapter, counted from the PDF table (chapters
	// 27-39 as the audit named, plus chapter 40, which the PDF also fills).
	s := australiaSchedule(t)
	perChapter := map[string]int{}
	for _, r := range s.Rows {
		if column4Text(r.Text) != "" {
			perChapter[r.Start[:2]]++
		}
	}
	want := map[string]int{"27": 19, "28": 81, "29": 163, "30": 14, "31": 5, "32": 39, "33": 14, "34": 20, "35": 13,
		"36": 1, "37": 7, "38": 55, "39": 69, "40": 30}
	for ch, n := range want {
		if perChapter[ch] != n {
			t.Errorf("chapter %s: %d rows with column 4, want %d", ch, perChapter[ch], n)
		}
	}
	if len(perChapter) != len(want) {
		t.Errorf("unexpected chapters: %v", perChapter)
	}
}

var australiaNotes = []struct {
	label string // how the note is recognised
	page  int
	start string
	end   string
}{
	{"alt", 1, "1. The specific rule or specific set of rules that applies", "if a good satisfies one of the alternatives."},
	{"QVC 40", 2, "5. For the purposes of column 3 of the product specific rules set out in this Annex: (d) “QVC 40” means", "shall apply."},
	{"(1)", 2, "5. For the purposes of column 3 of the product specific rules set out in this Annex: (e) “(1)” denotes", "one or both Parties;"},
	{"(2)", 3, "5. For the purposes of column 3 of the product specific rules set out in this Annex: (f) “(2)” denotes", "one or both Parties;"},
	{"(3)", 3, "5. For the purposes of column 3 of the product specific rules set out in this Annex: (g) “(3)” denotes", "one or both Parties;"},
	{"(4)", 3, "5. For the purposes of column 3 of the product specific rules set out in this Annex: (h) “(4)” denotes", "one or both Parties;"},
	{"(5)", 4, "5. For the purposes of column 3 of the product specific rules set out in this Annex: (i) “(5)” denotes", "one or both Parties;"},
	{"(6)", 4, "5. For the purposes of column 3 of the product specific rules set out in this Annex: (j) “(6)” denotes", "one or both Parties;"},
	{"(7)", 4, "5. For the purposes of column 3 of the product specific rules set out in this Annex: (k) “(7)” denotes", "one or both Parties;"},
	{"(8)", 4, "5. For the purposes of column 3 of the product specific rules set out in this Annex: (l) “(8)” denotes", "one or both Parties;"},
	{"(9)", 5, "5. For the purposes of column 3 of the product specific rules set out in this Annex: (m) “(9)” denotes", "spun entirely in the Area of one or both Parties;"},
	{"(10)", 5, "5. For the purposes of column 3 of the product specific rules set out in this Annex: (n) “(10)” denotes", "spun entirely in the Area of one or both Parties;"},
	{"(11)", 5, "5. For the purposes of column 3 of the product specific rules set out in this Annex: (o) “(11)” denotes", "spun entirely in the Area of one or both Parties;"},
	{"(12)", 5, "5. For the purposes of column 3 of the product specific rules set out in this Annex: (p) “(12)” denotes", "spun entirely in the Area of one or both Parties;"},
	{"(13)", 6, "5. For the purposes of column 3 of the product specific rules set out in this Annex: (q) “(13)” denotes", "one or both Parties;"},
	{"(14)", 6, "5. For the purposes of column 3 of the product specific rules set out in this Annex: (r) “(14)” denotes", "one or both Parties;"},
	{"(15)", 6, "5. For the purposes of column 3 of the product specific rules set out in this Annex: (s) “(15)” denotes", "woven entirely in the Area of one or both Parties;"},
	{"(16)", 7, "5. For the purposes of column 3 of the product specific rules set out in this Annex: (t) “(16)” denotes", "one or both Parties; and"},
	{"(17)", 7, "5. For the purposes of column 3 of the product specific rules set out in this Annex: (u) “(17)” denotes", "assembled in the Area of one or both Parties."},
	{"CR", 7, "6. For the purposes of column 4 of the product specific rules set out in this Annex: (a) “CR” means “Chemical Reaction Origin Rule”:", "occurred in the Area of a Party;"},
	{"D", 8, "6. For the purposes of column 4 of the product specific rules set out in this Annex: (b) “D” means “Distillation Rule”:", "further processed into lubricating oils;"},
	{"P", 8, "6. For the purposes of column 4 of the product specific rules set out in this Annex: (c) “P” means “Purification Origin Rule”:", "purification process occurred in the Area of a Party;"},
	{"CPS", 9, "6. For the purposes of column 4 of the product specific rules set out in this Annex: (d) “CPS” means “Change in Particle Size Origin Rule”:", "if the process occurred in the Area of a Party;"},
	{"SM", 9, "6. For the purposes of column 4 of the product specific rules set out in this Annex: (e) “SM” means “Standards Materials Origin Rule”:", "production process occurred in the Area of a Party; and"},
	{"IS", 10, "6. For the purposes of column 4 of the product specific rules set out in this Annex: (f) “IS” means “Isomer Separation Origin Rule”:", "if the process occurred in the Area of a Party."},
	{"61-63", 10, "7. In determining the origin of a good classified under chapters 61 to 63,", "whether or not they contain textiles."},
	{"header", 11, "Note: Goods which satisfy the applicable rules set out in column 3 or column 4 below", "considered as originating goods of a Party."},
}

func TestAustraliaGeneralNotesCapturedVerbatim(t *testing.T) {
	s := australiaSchedule(t)
	if len(s.Notes) != len(australiaNotes) {
		t.Fatalf("got %d notes, want %d", len(s.Notes), len(australiaNotes))
	}
	for i, want := range australiaNotes {
		n := s.Notes[i]
		if n.Scope != "schedule" || n.Key != "" || n.Page != want.page ||
			!strings.HasPrefix(n.Text, want.start) || !strings.HasSuffix(n.Text, want.end) {
			t.Errorf("note %d (%s): %s p%d %q ... %q", i, want.label, n.Scope, n.Page, n.Text[:min(70, len(n.Text))], n.Text[max(0, len(n.Text)-40):])
		}
		if strings.ContainsAny(n.Text, "\n\t") || strings.Contains(n.Text, "  ") || regexp.MustCompile(` \d{3}$`).MatchString(n.Text) {
			t.Errorf("note %d: layout leaked: %q", i, n.Text[max(0, len(n.Text)-30):])
		}
	}
	// Whole texts of the two notes that carry a Note: and exception lists.
	qvc := s.Notes[1].Text
	if !strings.Contains(qvc, "of not less than 40 per cent, and the last process of production of the good has been performed in the exporting Party; Note: For the purpose of this Annex, paragraph 2 of Article 3.5 (Rules of Origin - Calculation of Qualifying Value Content) shall apply.") {
		t.Errorf("QVC 40 note: %q", qvc)
	}
	if !strings.Contains(s.Notes[21].Text, "3809.10, 38.23, 3824.60, and 38.25), “purification” means") {
		t.Errorf("P exceptions lost: %q", s.Notes[22].Text)
	}
	if !strings.Contains(s.Notes[0].Text, "set out in column 3 or 4 of the product specific rules") {
		t.Errorf("paragraph 1: %q", s.Notes[0].Text)
	}
	if !strings.Contains(s.Notes[26].Text, "column 3 or column 4") {
		t.Errorf("header: %q", s.Notes[26].Text)
	}
}

func australiaNoteLabels(ev []Evidence) map[string]bool {
	got := map[string]bool{}
	for _, e := range ev {
		for _, want := range australiaNotes {
			if strings.HasPrefix(e.Text, want.start) {
				got[want.label] = true
			}
		}
	}
	return got
}

func TestAustraliaNotesReachOnlyTheRowsThatUseThem(t *testing.T) {
	table := mustDefault(t)
	cases := []struct {
		hs   string
		want []string
	}{
		{"0101", nil},
		{"2701.11", []string{"alt", "CR", "header"}},
		{"2710.12", []string{"alt", "CR", "D", "header"}},
		{"2801.10", []string{"alt", "CR", "P", "SM", "IS", "header"}},
		{"2807.00", []string{"alt", "QVC 40", "CR", "P", "SM", "IS", "header"}},
		{"3302.10", []string{"alt", "CR", "P", "header"}},
		{"3801.10", []string{"alt", "CR", "P", "SM", "header"}},
		{"3901.10", []string{"alt", "CR", "P", "IS", "header"}},
		{"3915.10", []string{"alt", "CR", "P", "CPS", "IS", "header"}},
		{"4017.00", []string{"alt", "CR", "header"}},
		{"6101.20", []string{"(14)", "61-63"}},
		{"6203.43", []string{"(15)", "61-63"}},
		{"6203.42", []string{"61-63"}},
		{"5208.11", []string{"(4)"}},
	}
	for _, c := range cases {
		ev, err := table.Lookup("JP-Australia EPA", c.hs, australiaDate)
		if err != nil || len(ev) == 0 {
			t.Fatalf("%s: %v %v", c.hs, ev, err)
		}
		got := australiaNoteLabels(ev)
		for _, w := range c.want {
			if !got[w] {
				t.Errorf("%s lacks note %s", c.hs, w)
			}
			delete(got, w)
		}
		for extra := range got {
			// QVC 40 and textile codes depend on the row text, not the chapter.
			if extra == "QVC 40" && strings.Contains(ev[0].Text, "QVC 40") {
				continue
			}
			t.Errorf("%s wrongly has note %s", c.hs, extra)
		}
	}
	// A product outside chapters 27-40 never gets the column-4 definitions.
	ev, _ := table.Lookup("JP-Australia EPA", "8501.10", australiaDate)
	for label := range australiaNoteLabels(ev) {
		if label != "QVC 40" || !strings.Contains(ev[0].Text, "QVC 40") {
			t.Errorf("8501.10 has note %s", label)
		}
	}
}

// Structural: for every row, each definition note is attached exactly when the
// row's own text uses the code.
func TestAustraliaNoteApplicabilityMatchesRowUsage(t *testing.T) {
	s := australiaSchedule(t)
	use := func(label string, text string) bool {
		c3, rest, _ := strings.Cut(strings.TrimPrefix(text, "Rule (column 3): "), australiaLabel)
		if !strings.HasPrefix(text, "Rule (column 3): ") {
			c3, rest = text, ""
		}
		switch label {
		case "alt", "header":
			return false // chapter-scoped, checked below
		case "61-63":
			return false
		case "QVC 40":
			return strings.Contains(c3, "QVC 40")
		}
		if strings.HasPrefix(label, "(") {
			return strings.Contains(c3, label)
		}
		for _, w := range regexp.MustCompile(`[A-Z]+`).FindAllString(rest, -1) {
			if w == label {
				return true
			}
		}
		return false
	}
	for i, want := range australiaNotes {
		n := s.Notes[i]
		for _, r := range s.Rows {
			pad := r.Start + strings.Repeat("0", 6-len(r.Start))
			padEnd := r.End + strings.Repeat("9", 6-len(r.End))
			in := n.appliesTo(pad) && n.appliesTo(padEnd)
			var expect bool
			switch want.label {
			case "alt", "header":
				ch, _ := strconv.Atoi(r.Start[:2])
				expect = ch >= 27 && ch <= 40
			case "61-63":
				expect = r.Start[:2] >= "61" && r.Start[:2] <= "63"
			default:
				expect = use(want.label, r.Text)
			}
			if in != expect {
				t.Errorf("note %s row %s-%s: applies=%v, row uses=%v (%q)", want.label, r.Start, r.End, in, expect, r.Text)
			}
		}
	}
}

// Every textile code and column-4 code a row uses has a defining note (and the
// rows use no code the PDF's General Notes do not define).
func TestAustraliaEveryCodeInRowsIsDefined(t *testing.T) {
	s := australiaSchedule(t)
	defined := map[string]bool{}
	for _, n := range australiaNotes {
		defined[n.label] = true
	}
	textile := regexp.MustCompile(`\((\d+)\)`)
	for _, r := range s.Rows {
		c3, c4, _ := strings.Cut(strings.TrimPrefix(r.Text, "Rule (column 3): "), australiaLabel)
		for _, m := range textile.FindAllString(c3, -1) {
			if !defined[m] {
				t.Errorf("row %s-%s uses undefined %s", r.Start, r.End, m)
			}
		}
		for _, w := range regexp.MustCompile(`[A-Z]+`).FindAllString(c4, -1) {
			if !defined[w] {
				t.Errorf("row %s-%s uses undefined code %s", r.Start, r.End, w)
			}
		}
	}
}
