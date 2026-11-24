package jprules

import (
	"fmt"
	"regexp"
	"strings"
	"testing"
)

// Chapter 3 (Rules of origin) articles of the JP-EU EPA and JP-UK CEPA, quoted
// with a page and source of their own. See docs/japan/chapter3-articles.md.

type artNote struct {
	page       int
	label      string // after "<agreement> Chapter 3 (Rules of origin), "
	start, end string
	wholly     bool // scoped to the Annex 3-B rows whose rule says "wholly obtained"
}

var euArticles = []artNote{
	{2, "Article 3.1(e) and (f)", `(e) "material" means any matter`, "cannot be determined;", false},
	{3, "Article 3.1(h) and (i)", `(h) "product" means any matter`, "including assembly.", false},
	{3, "Article 3.2 paragraph 1", "ARTICLE 3.2 Requirements for originating products 1. For the purpose", "all applicable requirements of Annex 3-B.", false},
	{4, "Article 3.2 paragraphs 2 to 4", "2. For the purposes of this Chapter, the territorial scope", "shall be satisfied without interruption in a Party.", false},
	{4, "Article 3.3 paragraph 1(a) to (c)", "ARTICLE 3.3 Wholly obtained products 1. For the purposes of Article 3.2,", "(c) a product obtained from a live animal raised there;", true},
	{5, "Article 3.3 paragraph 1(d) to (j)", "(d) a product obtained from a slaughtered animal", "in accordance with international law;", true},
	{6, "Article 3.3 paragraphs 1(k), 1(l) and 2", "(k) a product that is:", "(i) it is at least 50 per cent owned by one or more natural persons of a Party; or", true},
	{7, "Article 3.3 paragraph 2(c)(ii)", "(ii) it is owned by one or more juridical persons1:", "natural persons or juridical persons of a Party.", true},
	{7, "Article 3.3 footnote 1", `1 For the purposes of this Chapter, "juridical person" means`, "sole proprietorship or association.", true},
	{7, "Article 3.4 paragraph 1(a)", "ARTICLE 3.4 Insufficient working or processing 1. Notwithstanding", "remains in good condition during transport and storage;", false},
	{8, "Article 3.4 paragraph 1(b) to (k)", "(b) changes of packaging;", "matching including the making-up of sets of articles;", false},
	{9, "Article 3.4 paragraph 1(l) to (q) and footnotes", "(l) simple placing in bottles", "foul-tasting substances.", false},
	{10, "Article 3.4 paragraph 2", "2. For the purpose of paragraph 1, operations", "needed for carrying out those operations.", false},
	{10, "Article 3.5 paragraphs 1 to 4", "ARTICLE 3.5 Accumulation 1. A product that qualifies", "information as provided for in Annex 3-C.", false},
	{11, "Article 3.5 paragraph 5", "5. The information referred to in paragraph 4", "on which the information was provided.", false},
	{11, "Article 3.6 paragraph 1", "ARTICLE 3.6 Tolerances 1. If a non-originating material", "as amended on 1 January 2017.", false},
	{12, "Article 3.6 paragraph 2", "2. Paragraph 1 does not apply if the value", "specified in the requirements set out in Annex 3-B.", false},
	{12, "Article 3.6 paragraph 3", "3. Paragraph 1 does not apply to products wholly obtained", "paragraphs 1 and 2 apply.", true},
	{16, "Article 3.12 paragraph 1(a)", "ARTICLE 3.12 Accessories, spare parts, tools and instructional or other information materials 1.", "from, the product; and", false},
	{17, "Article 3.12 paragraphs 1(b) to 4", "(b) the types, quantities and value", "with which they are delivered.", false},
}

var ukArticles = []artNote{
	{2, "Article 3.1(e) and (f)", `(e) "material" means any matter`, "cannot be determined;", false},
	{2, "Article 3.1(h) and (i)", `(h) "product" means any matter`, "including assembly.", false},
	{2, "Article 3.2 paragraph 1", "ARTICLE 3.2 Requirements for originating products 1. For the purpose", "all applicable requirements of Annex 3-B.", false},
	{3, "Article 3.2 paragraphs 2 to 4", "2. For the purposes of this Chapter, the territorial scope", "shall be satisfied without interruption in a Party.", false},
	{3, "Article 3.3 paragraph 1(a) to (g)", "ARTICLE 3.3 Wholly obtained products 1. For the purposes of Article 3.2,", "extracted or taken there;", true},
	{4, "Article 3.3 paragraph 1(h) to (l) and 2(a), (b)", "(h) fish, shellfish or other marine life", "(b) flies the flag of a Party; and", true},
	{5, "Article 3.3 paragraph 2(c) and footnotes", "(c) satisfies one of the following requirements:", "laws and regulations of that Member State.", true},
	{6, "Article 3.4 paragraph 1(a) to (k)", "ARTICLE 3.4 Insufficient working or processing 1. Notwithstanding", "matching including the making-up of sets of articles;", false},
	{7, "Article 3.4 paragraph 1(l) to (q) and 2", "(l) simple placing in bottles", "needed for carrying out those operations.", false},
	{7, "Article 3.4 footnotes 1 and 2", "1 For the purposes of this Article, simple mixing of products", "foul-tasting substances.", false},
	{7, "Article 3.5 paragraph 1", "ARTICLE 3.5 Accumulation 1. A product that qualifies", "used as a material in the production of another product in the other Party.", false},
	{8, "Article 3.5 paragraphs 2 to 7", "2. A product that qualifies as originating in the European Union", "as amended on 1 January 2017.", false},
	{9, "Article 3.5 paragraphs 8 and 9", "8. The information referred to in paragraph 7", "European Union on a non-originating material.", false},
	{10, "Article 3.6 paragraphs 1 and 2", "ARTICLE 3.6 Tolerances 1. If a non-originating material", "specified in the requirements set out in Annex 3-B.", false},
	{10, "Article 3.6 paragraph 3", "3. Paragraph 1 does not apply to products wholly obtained", "paragraphs 1 and 2 apply.", true},
	{14, "Article 3.12", "ARTICLE 3.12 Accessories, spare parts, tools and instructional or other information materials 1.", "with which they are delivered.", false},
}

// The 26 Annex 3-B rows of each schedule whose rule text says "wholly obtained",
// as code ranges with touching ranges joined.
const (
	euWholly = "[[0101 0106] [0201 0210] [03 03] [0401 0410] [0601 0604] [0701 0714] [0801 0814] [090210 090220] [1001 1008] [1101 1109] [1401 1404] [1507 1507] [1509 1510] [1514 1515] [1601 1605] [2002 2009] [2106 2106] [2203 2209] [2309 2309]]"
	ukWholly = "[[0101 0106] [0201 0210] [03 03] [0401 0410] [0601 0604] [0701 0714] [0801 0814] [090210 090220] [1001 1008] [1102 1103] [1105 1109] [1401 1404] [1507 1507] [1509 1510] [1514 1515] [1601 1601] [1603 1605] [2002 2009] [2106 2106] [2203 2209]]"
)

type ch3Agreement struct {
	name, label, url, from, wholly string
	notes                          []artNote
	caveatNeeds                    []string
}

var ch3Agreements = []ch3Agreement{
	{"JP-EU EPA", "JP-EU EPA Chapter 3 (Rules of origin)", "https://www.customs.go.jp/roo/english/text/eu1.pdf", "2019-02-01", euWholly, euArticles,
		[]string{"JP-EU EPA", "Articles 3.1", "do not replace the listed rule", "do not themselves establish origin", "Article 3.2 paragraph 1(b)", "context only", "unless it says otherwise", "only partly quoted", "Note 1 paragraphs 1, 2 and 4", "Note 2 paragraph 4", "Articles 3.7 to 3.11 and 3.13 to 3.15", "Annex 3-C", "Appendix 3-B-1", "3.16 to 3.29", "NOT supplied", "not verified", "undetermined"}},
	{"JP-UK CEPA", "JP-UK CEPA Chapter 3 (Rules of origin)", "https://www.customs.go.jp/roo/english/text/uk1.pdf", "2021-01-01", ukWholly, ukArticles,
		[]string{"JP-UK CEPA", "Articles 3.1", "do not replace the listed rule", "do not themselves establish origin", "Article 3.2 paragraph 1(b)", "context only", "unless it says otherwise", "only partly quoted", "Note 1 paragraphs 1, 2 and 4", "Note 2 paragraph 4", "Articles 3.7 to 3.11 and 3.13 to 3.15", "Annex 3-C", "Annex 3-D", "Appendix 3-B-1", "Article 3.5 paragraphs 10 to 12", "3.16 to 3.28", "NOT supplied", "not verified", "undetermined"}},
}

func chapter3Notes(s schedule, url string) (caveats, quoted []note) {
	for _, n := range s.Notes {
		switch {
		case n.Page == 0 && strings.Contains(n.Text, "Chapter 3 (Rules of origin)"):
			caveats = append(caveats, n)
		case n.SourceURL == url:
			quoted = append(quoted, n)
		}
	}
	return
}

func TestChapter3NotesCaptured(t *testing.T) {
	leak := regexp.MustCompile(`(?:EU|UK)/JP/en|_{4,}|Working Group on Wine|ARTICLE 4\.1`)
	for _, a := range ch3Agreements {
		s := scheduleFrom(t, mustDefault(t), a.name, a.from)
		caveats, quoted := chapter3Notes(s, a.url)
		if len(quoted) != len(a.notes) {
			t.Fatalf("%s: %d chapter 3 notes, want %d", a.name, len(quoted), len(a.notes))
		}
		for i, want := range a.notes {
			n := quoted[i]
			wantApplies := "[]"
			if want.wholly {
				wantApplies = a.wholly
			}
			if n.Scope != "schedule" || n.Key != "" || n.Page != want.page || n.Ref != "" ||
				!strings.HasPrefix(n.Text, want.start) || !strings.HasSuffix(n.Text, want.end) ||
				n.Citation != a.label+", "+want.label || fmt.Sprint(n.Applies) != wantApplies {
				t.Errorf("%s note %d (%s): %s %q p%d %q ... %q %q %v", a.name, i, want.label, n.Scope, n.Key, n.Page, n.Text[:min(70, len(n.Text))], n.Text[max(0, len(n.Text)-50):], n.Citation, n.Applies)
			}
			if leak.MatchString(n.Text) {
				t.Errorf("%s note %d: footer, rule line or neighbouring chapter leaked: %q", a.name, i, n.Text)
			}
			if i > 0 && n.Page < quoted[i-1].Page {
				t.Errorf("%s note %d: pages out of order", a.name, i)
			}
		}
		// One Unde Labs caveat (no URL, page 0) per agreement.
		if len(caveats) != 1 {
			t.Fatalf("%s: %d caveat notes", a.name, len(caveats))
		}
		c := caveats[0]
		if c.Scope != "schedule" || len(c.Applies) != 0 || c.SourceURL != "" || c.Citation != "" || c.Ref != "" || !strings.HasPrefix(c.Text, caveatPrefix) {
			t.Errorf("%s caveat: %+v", a.name, c)
		}
		for _, frag := range a.caveatNeeds {
			if !strings.Contains(c.Text, frag) {
				t.Errorf("%s caveat lacks %q", a.name, frag)
			}
		}
	}
}

func TestChapter3WhollyObtainedRangesAreTheRowRanges(t *testing.T) {
	table := mustDefault(t)
	for _, a := range ch3Agreements {
		s := scheduleFrom(t, table, a.name, a.from)
		_, quoted := chapter3Notes(s, a.url)
		var wholly, other []row
		for _, r := range s.Rows {
			if strings.Contains(r.Text, "wholly obtained") {
				wholly = append(wholly, r)
			} else {
				other = append(other, r)
			}
		}
		if len(wholly) != 26 {
			t.Fatalf("%s: %d wholly obtained rows, want 26", a.name, len(wholly))
		}
		overlaps := func(r row) bool {
			for _, w := range wholly {
				k := min(len(r.Start), len(w.Start))
				if r.Start[:k] <= w.End[:k] && w.Start[:k] <= r.End[:k] {
					return true
				}
			}
			return false
		}
		for _, n := range quoted {
			if len(n.Applies) == 0 {
				for _, w := range wholly {
					if !n.appliesTo(w.Start) {
						t.Errorf("%s %s: schedule note must apply to %s", a.name, n.Citation, w.Start)
					}
				}
				continue
			}
			for _, w := range wholly {
				for _, c := range []string{w.Start, w.End} {
					if !n.appliesTo(c) {
						t.Errorf("%s %s: lacks wholly obtained row %s-%s", a.name, n.Citation, w.Start, w.End)
					}
				}
			}
			for _, r := range other {
				if !overlaps(r) && (n.appliesTo(r.Start) || n.appliesTo(r.End)) {
					t.Errorf("%s %s: wrongly applies to row %s-%s: %.50s", a.name, n.Citation, r.Start, r.End, r.Text)
				}
			}
		}
	}
}

func TestChapter3Lookup(t *testing.T) {
	table := mustDefault(t)
	inWholly := map[string][]string{
		"JP-EU EPA":  {"0101.21", "0406.90", "0301.11", "1101.00", "1601.00", "1602.10", "2204.21", "2309.90", "0902.10"},
		"JP-UK CEPA": {"0101.21", "0406.90", "0301.11", "1102.20", "1603.00", "2204.21", "0902.10"},
	}
	outWholly := map[string][]string{
		"JP-EU EPA":  {"0901.11", "1511.10", "2001.10", "2710.12", "5208.11", "6101.20", "8517.12", "2201.10"},
		"JP-UK CEPA": {"0901.11", "1101.00", "1602.10", "1511.10", "2001.10", "2710.12", "5208.11", "8517.12", "2309.90"},
	}
	for _, a := range ch3Agreements {
		has := func(hs string, n artNote) bool {
			ev, err := table.Lookup(a.name, hs, annexDate)
			if err != nil || len(ev) == 0 {
				t.Fatalf("%s %s: %v %v", a.name, hs, ev, err)
			}
			cite := fmt.Sprintf("%s, %s, PDF page %d", a.label, n.label, n.page)
			for _, e := range ev {
				if strings.HasPrefix(e.Text, n.start) && strings.HasSuffix(e.Text, n.end) && e.Citation == cite &&
					e.URL == fmt.Sprintf("%s#page=%d", a.url, n.page) {
					return true
				}
			}
			return false
		}
		for _, n := range a.notes {
			for _, hs := range append(append([]string{}, inWholly[a.name]...), outWholly[a.name]...) {
				want := !n.wholly || contains(inWholly[a.name], hs)
				if got := has(hs, n); got != want {
					t.Errorf("%s %s %s: note present %v, want %v", a.name, hs, n.label, got, want)
				}
			}
		}
		// every lookup of the schedule carries the caveat exactly once
		for _, hs := range []string{"0406.90", "8517.12"} {
			ev, _ := table.Lookup(a.name, hs, annexDate)
			k := 0
			for _, e := range ev {
				if e.Citation == caveatPrefix && strings.Contains(e.Text, "Chapter 3 (Rules of origin)") {
					k++
					if e.URL != "" {
						t.Errorf("%s: caveat has a URL", a.name)
					}
				}
			}
			if k != 1 {
				t.Errorf("%s %s: %d chapter 3 caveats", a.name, hs, k)
			}
		}
	}
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

func TestChapter3OnlyTheListedArticlesAreQuoted(t *testing.T) {
	head := regexp.MustCompile(`ARTICLE (3\.\d+)`)
	for _, a := range ch3Agreements {
		s := scheduleFrom(t, mustDefault(t), a.name, a.from)
		_, quoted := chapter3Notes(s, a.url)
		seen := map[string]bool{}
		for _, n := range quoted {
			for _, m := range head.FindAllStringSubmatch(n.Text, -1) {
				seen[m[1]] = true
			}
			// no procedural article, optional article or neighbouring chapter
			for _, bad := range []string{"Statement on origin", "Claim for preferential tariff treatment", "Verification", "Record keeping",
				"Unit of qualification", "Non-alteration", "Accounting segregation", "Returning products", "Neutral elements", "Packing materials"} {
				if strings.Contains(n.Text, bad) {
					t.Errorf("%s: %q quoted in %s", a.name, bad, n.Citation)
				}
			}
		}
		want := map[string]bool{"3.2": true, "3.3": true, "3.4": true, "3.5": true, "3.6": true, "3.12": true}
		if fmt.Sprint(seen) != fmt.Sprint(want) {
			t.Errorf("%s: article headings %v", a.name, seen)
		}
		for _, n := range s.Notes {
			if strings.Contains(n.Citation, "Article 3.1") && !strings.Contains(n.Citation, "Article 3.1(") && !strings.Contains(n.Citation, "Article 3.12") {
				t.Errorf("%s: whole Article 3.1 quoted: %s", a.name, n.Citation)
			}
		}
	}
}

func TestChapter3OtherAgreementsAndOldEvidenceUnchanged(t *testing.T) {
	// With the Chapter 3 notes left out, every agreement (EU and UK included) has
	// the evidence hashes taken on main.
	goldenHashes(t, annexGolden, func(e Evidence) bool { return isGeneralNote(e) || isChapter3Article(e) })
}

// Full EU and UK evidence including the Chapter 3 articles and caveat.
var chapter3Golden = map[string]string{"JP-EU EPA": "d17bd27b04754e12", "JP-UK CEPA": "6a572d16a3f767d4"}

func TestChapter3EvidenceHashes(t *testing.T) {
	want := map[string]string{}
	for a, h := range annexGolden {
		want[a] = h
	}
	for a, h := range generalNotesGolden {
		want[a] = h
	}
	for a, h := range chapter3Golden {
		want[a] = h
	}
	goldenHashes(t, want, func(Evidence) bool { return false })
}

// isChapter3Article reports whether evidence is one of the quoted Chapter 3 articles or
// the Unde Labs note that goes with them.
func isChapter3Article(e Evidence) bool {
	return strings.Contains(e.Citation, "Chapter 3 (Rules of origin)") ||
		e.Citation == caveatPrefix && strings.Contains(e.Text, "Chapter 3 (Rules of origin)")
}
