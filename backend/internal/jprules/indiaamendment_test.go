package jprules

import (
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"
)

const (
	indiaOldWO    = "Manufacture in which all the materials used are wholly obtained."
	indiaNewWO    = "締約国において製造され、かつ、製造に使用する全ての材料が当該締約国において完全に得られるものであること。"
	indiaNewCC    = "第０３０４．９９号の産品への他の類の材料からの変更"
	indiaNoticeEv = "https://www.customs.go.jp/roo/information/indiakaisei.html"
	indiaCitation = "JP-India CEPA Annex 2 (HS 2007)"
	indiaAmended  = indiaCitation + " as amended, effective 2022-04-04"
	indiaPDFSHA   = "4011ed6be153ec99ce9b199bb314c0f0fa69204677f7150bef649bf55a0b39e9"
)

func indiaDay(t *testing.T, s string) time.Time {
	t.Helper()
	d, err := time.Parse(time.DateOnly, s)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func TestIndiaHasTwoWindows(t *testing.T) {
	var got [][2]string
	for _, s := range mustDefault(t).schedules["JP-India CEPA"] {
		got = append(got, [2]string{s.From, s.Until})
	}
	want := [][2]string{{"2011-08-01", "2022-04-04"}, {"2022-04-04", ""}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("windows = %v, want %v", got, want)
	}
}

// until is exclusive: the old row ends 2022-04-03, the new rows start 2022-04-04.
func TestIndiaChapter3AtTheBoundary(t *testing.T) {
	table := mustDefault(t)
	type want struct {
		date, hs, text, cite string
	}
	for _, c := range []want{
		{"2022-04-03", "030110", indiaOldWO, "JP-India CEPA Annex 2 (HS 2007), PDF page 2"},
		{"2022-04-03", "030499", indiaOldWO, "JP-India CEPA Annex 2 (HS 2007), PDF page 2"},
		{"2022-04-03", "0302", indiaOldWO, "JP-India CEPA Annex 2 (HS 2007), PDF page 2"},
		{"2022-04-04", "030110", indiaNewWO, ""},
		{"2022-04-04", "030491", indiaNewWO, ""},
		{"2022-04-04", "030499", indiaNewCC, ""},
		{"2022-04-04", "030510", indiaNewWO, ""},
		{"2022-04-04", "030799", indiaNewWO, ""},
		{"2022-04-04", "0302", indiaNewWO, ""},
		{"2022-04-04", "0307", indiaNewWO, ""},
		{"2026-01-01", "030499", indiaNewCC, ""},
	} {
		ev, err := table.Lookup("JP-India CEPA", c.hs, indiaDay(t, c.date))
		if err != nil || len(ev) == 0 {
			t.Fatalf("%s %s: %v %v", c.date, c.hs, ev, err)
		}
		first := ev[0]
		if first.Text != c.text {
			t.Errorf("%s %s: text %q, want %q", c.date, c.hs, first.Text, c.text)
		}
		if c.cite != "" && first.Citation != c.cite {
			t.Errorf("%s %s: citation %q", c.date, c.hs, first.Citation)
		}
		if c.cite == "" && (first.URL != indiaNoticeEv || !strings.HasPrefix(first.Citation, indiaAmended) || !strings.Contains(first.Citation, "Japan Customs notice of 2022-03-23")) {
			t.Errorf("%s %s: evidence not from the notice: %+v", c.date, c.hs, first)
		}
	}
	// 0304 mixes wholly obtained and change-of-chapter subheadings.
	res, err := table.LookupDetailed("JP-India CEPA", "0304", indiaDay(t, "2022-04-04"))
	if err != nil || res.Reason != ReasonTooBroad {
		t.Errorf("0304 after amendment: %v %v", res.Reason, err)
	}
	res, _ = table.LookupDetailed("JP-India CEPA", "0304", indiaDay(t, "2022-04-03"))
	if res.Reason != ReasonPSR {
		t.Errorf("0304 before amendment: %v", res.Reason)
	}
	res, _ = table.LookupDetailed("JP-India CEPA", "030110", indiaDay(t, "2011-07-31"))
	if res.Reason != ReasonNoDatedSource {
		t.Errorf("before entry into force: %v", res.Reason)
	}
}

func indiaPair(t *testing.T) (old, amended schedule) {
	t.Helper()
	table := mustDefault(t)
	return scheduleFrom(t, table, "JP-India CEPA", "2011-08-01"), scheduleFrom(t, table, "JP-India CEPA", "2022-04-04")
}

func isChapter3(r row) bool { return r.Start[:2] == "03" }

// Only chapter 3 changes: every other row, and the source, are identical.
func TestIndiaAmendedEntryChangesOnlyChapter3(t *testing.T) {
	old, amended := indiaPair(t)
	// The schedule keeps the original citation; only the chapter 3 rows say "as amended".
	if amended.Citation != indiaCitation || old.Citation != indiaCitation {
		t.Errorf("citations %q %q", old.Citation, amended.Citation)
	}
	if old.URL != amended.URL || old.SHA256 != amended.SHA256 || old.SHA256 != indiaPDFSHA || old.HSEdition != amended.HSEdition {
		t.Errorf("source differs: %q %q", old.URL, amended.URL)
	}
	var a, b []row
	for _, r := range old.Rows {
		if !isChapter3(r) {
			a = append(a, r)
		}
	}
	for _, r := range amended.Rows {
		if !isChapter3(r) {
			b = append(b, r)
		}
	}
	ja, _ := json.Marshal(a)
	jb, _ := json.Marshal(b)
	if len(a) != 377 || string(ja) != string(jb) {
		t.Errorf("non-chapter-3 rows differ: %d vs %d", len(a), len(b))
	}
	if len(old.Rows)-len(a) != 1 {
		t.Errorf("old chapter 3 rows = %d", len(old.Rows)-len(a))
	}
	var ch3 []string
	for _, r := range amended.Rows {
		if isChapter3(r) {
			ch3 = append(ch3, r.Start+"-"+r.End)
			if r.SourceURL != indiaNoticeEv || !strings.HasPrefix(r.Citation, indiaAmended) || r.Page != 1 || r.Ref == "" || r.Default || r.Blocked {
				t.Errorf("chapter 3 row %+v", r)
			}
			if r.Start == "030499" && r.Text != indiaNewCC || r.Start != "030499" && r.Text != indiaNewWO {
				t.Errorf("chapter 3 row text %+v", r)
			}
		}
	}
	if want := []string{"0301-0303", "030410-030492", "030499-030499", "0305-0307"}; !reflect.DeepEqual(ch3, want) {
		t.Errorf("chapter 3 rows %v, want %v", ch3, want)
	}
	if len(old.Notes) != 0 {
		t.Errorf("the original has notes: %d", len(old.Notes))
	}
}

func TestIndiaOtherChaptersIdenticalInBothWindows(t *testing.T) {
	table := mustDefault(t)
	old, _ := indiaPair(t)
	for _, r := range old.Rows {
		if isChapter3(r) {
			continue
		}
		a, _ := table.Lookup("JP-India CEPA", r.Start, indiaDay(t, "2022-04-03"))
		b, _ := table.Lookup("JP-India CEPA", r.Start, indiaDay(t, "2022-04-04"))
		if len(a) == 0 || !reflect.DeepEqual(a, b) {
			t.Fatalf("%s differs between the windows:\n%v\n%v", r.Start, a, b)
		}
	}
}

func TestIndiaAmendedNotes(t *testing.T) {
	_, amended := indiaPair(t)
	if amended.CaveatFirst {
		t.Error("the rule stays the first evidence item")
	}
	var caveat, transition *note
	for i, n := range amended.Notes {
		switch {
		case n.Page == 0:
			caveat = &amended.Notes[i]
		case n.Page == 1:
			transition = &amended.Notes[i]
		default:
			t.Errorf("unexpected note %+v", n)
		}
	}
	if caveat == nil || transition == nil {
		t.Fatalf("notes: %+v", amended.Notes)
	}
	for _, frag := range []string{caveatPrefix, "india2.pdf", indiaPDFSHA, "2011", "2022-03-23", "no consolidated", "no English text"} {
		if !strings.Contains(caveat.Text, frag) {
			t.Errorf("caveat lacks %q: %s", frag, caveat.Text)
		}
	}
	// Both notes are scoped to chapter 3 so that every other lookup is unchanged.
	for _, n := range []*note{caveat, transition} {
		if n.Scope != "chapter" || n.Key != "03" || !reflect.DeepEqual(n.Applies, [][2]string{{"0301", "0307"}}) {
			t.Errorf("note scope %+v", n)
		}
	}
	wantTransition := "なお、本改正においては、改正前の品目別規則に基づきインドの当局が発給した原産地証明書についても、改正後の規則を満たすことが明らかなため、有効期間内（発給から１年間）は輸入通関時に有効なものと認めます。"
	if transition.Text != wantTransition || transition.SourceURL != indiaNoticeEv || transition.Page != 1 || transition.Ref == "" {
		t.Errorf("transition note %+v", transition)
	}

	table := mustDefault(t)
	ev, _ := table.Lookup("JP-India CEPA", "030499", indiaDay(t, "2023-01-01"))
	if len(ev) != 3 || ev[0].Text != indiaNewCC || ev[1].Citation != caveatPrefix || ev[2].Text != wantTransition {
		t.Fatalf("chapter 3 evidence: %+v", ev)
	}
	// Other chapters get the rule only, exactly as before the amendment.
	ev, _ = table.Lookup("JP-India CEPA", "0401", indiaDay(t, "2023-01-01"))
	if len(ev) != 1 || ev[0].Citation != indiaCitation+", PDF page 2" {
		t.Fatalf("chapter 4 evidence: %+v", ev)
	}
	// The old window has no notes.
	ev, _ = table.Lookup("JP-India CEPA", "030499", indiaDay(t, "2022-04-03"))
	if len(ev) != 1 {
		t.Fatalf("old window evidence: %+v", ev)
	}
}

func TestSourcedRowValidation(t *testing.T) {
	mk := func(f func(r *row)) schedule {
		r := row{Start: "0301", End: "0303", Page: 1, Text: "x", Ref: "r1", SourceURL: "https://x.example/n.html", Citation: "Notice"}
		f(&r)
		return schedule{Agreement: "A", Citation: "A Annex", URL: "https://x.example/a.pdf", From: "2020-01-01",
			Rows: []row{{Start: "0101", End: "0101", Page: 2, Text: "CC"}, r}}
	}
	if err := validateSchedule(mk(func(r *row) {})); err != nil {
		t.Fatalf("valid sourced row in a paged schedule rejected: %v", err)
	}
	bad := map[string]func(r *row){
		"url only":      func(r *row) { r.Citation = "" },
		"citation only": func(r *row) { r.SourceURL = "" },
		"fragment":      func(r *row) { r.SourceURL += "#page=1" },
		"http":          func(r *row) { r.SourceURL = "http://x.example/n.html" },
		"padded":        func(r *row) { r.Citation = " Notice" },
		"no ref":        func(r *row) { r.Ref = "" },
		"page 2":        func(r *row) { r.Page = 2 },
		"blank ref":     func(r *row) { r.Ref = " " },
	}
	for name, f := range bad {
		if err := validateSchedule(mk(f)); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	// A ref-only row beside page-numbered rows stays rejected.
	s := mk(func(r *row) { r.SourceURL, r.Citation = "", "" })
	if err := validateSchedule(s); err == nil {
		t.Error("unsourced ref row in a paged schedule accepted")
	}
}

func TestSourcedRowEvidence(t *testing.T) {
	s := schedule{Agreement: "A", Citation: "A Annex", URL: "https://x.example/a.pdf", From: "2020-01-01",
		Rows: []row{{Start: "0301", End: "0303", Page: 1, Text: "x", Ref: "row 1", SourceURL: "https://x.example/n.html", Citation: "Notice"}}}
	got, _ := s.match("030110")
	if len(got) != 1 {
		t.Fatal(got)
	}
	e := s.rowEvidence(got[0])
	if e.URL != "https://x.example/n.html" || e.Citation != "Notice, row 1" || e.Text != "x" {
		t.Errorf("%+v", e)
	}
}

func TestIndiaWindowsDocumented(t *testing.T) {
	raw, err := os.ReadFile(dateWindowsDoc)
	if err != nil {
		t.Fatal(err)
	}
	doc := string(raw)
	rows := 0
	for _, line := range strings.Split(doc, "\n") {
		if strings.HasPrefix(line, "| JP-India CEPA | 2011-08-01") && strings.Contains(line, "2022-04-03") ||
			strings.HasPrefix(line, "| JP-India CEPA | 2022-04-04") {
			rows++
		}
	}
	if rows != 2 {
		t.Errorf("date-windows.md has %d India window rows, want 2", rows)
	}
	for _, gone := range []string{"amended text was not found", "No amendment is added"} {
		if strings.Contains(doc, gone) {
			t.Errorf("date-windows.md still says %q", gone)
		}
	}
	amend, err := os.ReadFile("../../../docs/japan/india-chapter3-amendment.md")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{indiaPDFSHA, "598631d6e349c0c7ea8ff2b43cef2755b26f12051f44576e45bd511548c560b2", "18b31d5a980b10769aae60e257dc60d9390b2b8f7758fa72414017fb1df7c909", "2022-04-04", "2022-03-23", "not modelled"} {
		if !strings.Contains(string(amend), want) {
			t.Errorf("india-chapter3-amendment.md lacks %q", want)
		}
	}
	if strings.Contains(string(amend), "/Users/") {
		t.Error("amendment doc has a private path")
	}
}

// The notice's ranges are HS 2007 codes. HS 2017 0304.93-0304.97 do not exist in
// HS 2007 and sit between the ranges. The WCO tables (HS 2017 to 2012 page 6, HS
// 2012 to 2007 page 6) name ex 0304.99 as their HS 2007 source, so they get a
// review hint with the amended 0304.99 rule, never an applied rule.
func TestIndiaHS2017CodesBetweenTheNoticeRangesGetAHint(t *testing.T) {
	table := mustDefault(t)
	for _, hs := range []string{"030493", "030494", "030495", "030496", "030497"} {
		res, _ := table.LookupDetailed("JP-India CEPA", hs, indiaDay(t, "2022-04-03"))
		if res.Reason != ReasonPSR {
			t.Errorf("%s before: %v", hs, res.Reason)
		}
		res, _ = table.LookupDetailed("JP-India CEPA", hs, indiaDay(t, "2022-04-04"))
		if res.Reason != ReasonEditionHint || len(res.Evidence) != 0 {
			t.Fatalf("%s after: %v %v", hs, res.Reason, res.Evidence)
		}
		for _, frag := range []string{dotted(hs), "HS 2007", "ex 0304.99", indiaNewCC, "2022-03-23", "specialist"} {
			if !strings.Contains(res.Hint, frag) {
				t.Errorf("%s hint lacks %q: %s", hs, frag, res.Hint)
			}
		}
		if strings.Contains(res.Hint, "applies to") {
			t.Errorf("%s hint chooses the rule: %s", hs, res.Hint)
		}
	}
	// 0304.98 does not exist in HS 2017; 0304.99 has its own row; 0304 stays too broad.
	if res, _ := table.LookupDetailed("JP-India CEPA", "030498", indiaDay(t, "2022-04-04")); res.Reason == ReasonEditionHint {
		t.Errorf("030498 got a hint")
	}
	if res, _ := table.LookupDetailed("JP-India CEPA", "0304", indiaDay(t, "2022-04-04")); res.Reason != ReasonTooBroad {
		t.Errorf("0304: %v", res.Reason)
	}
}
