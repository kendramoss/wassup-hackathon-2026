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
	{12, "Article 3.7", "ARTICLE 3.7 Unit of qualification 1.", "when applying the provisions of this Chapter.", false},
	{13, "Article 3.8 paragraphs 1 to 4", "ARTICLE 3.8 Accounting segregation 1.", "generally accepted in the Party.", false},
	{14, "Article 3.8 paragraphs 5 and 6", "5. A Party may require, under conditions", "if the materials had been physically segregated.", false},
	{14, "Article 3.9", "ARTICLE 3.9 Sets A set,", "ex-works or free on board price of the set.", false},
	{16, "Article 3.12 paragraph 1(a)", "ARTICLE 3.12 Accessories, spare parts, tools and instructional or other information materials 1.", "from, the product; and", false},
	{17, "Article 3.12 paragraphs 1(b) to 4", "(b) the types, quantities and value", "with which they are delivered.", false},
	{18, "Article 3.13", "ARTICLE 3.13 Neutral elements", "reasonably be demonstrated to be a part of that production.", false},
	{19, "Articles 3.14 and 3.15", "ARTICLE 3.14 Packing materials and containers for shipment", "value requirement to the product.", false},
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
	{11, "Article 3.7", "ARTICLE 3.7 Unit of qualification 1.", "when applying the provisions of this Chapter.", false},
	{11, "Article 3.8 paragraphs 1 to 4", "ARTICLE 3.8 Accounting segregation 1.", "generally accepted in the Party.", false},
	{12, "Article 3.8 paragraphs 5 and 6", "5. A Party may require, under conditions", "if the materials had been physically segregated.", false},
	{12, "Article 3.9", "ARTICLE 3.9 Sets A set,", "ex-works or free on board price of the set.", false},
	{14, "Article 3.12", "ARTICLE 3.12 Accessories, spare parts, tools and instructional or other information materials 1.", "with which they are delivered.", false},
	{14, "Article 3.13, introduction", "ARTICLE 3.13 Neutral elements", "determine the originating status of the following elements:", false},
	{15, "Article 3.13(a) to (g), Article 3.14 and Article 3.15 paragraph 1", "(a) fuel, energy, catalysts and solvents;", "or whether the product is wholly obtained.", false},
	{16, "Article 3.15 paragraph 2", "2. If a product is subject to a value requirement set out in Annex 3-B,", "value requirement to the product.", false},
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
	// scoped: note index -> the applies list of a note scoped by its own wording
	// (CPTPP Annex 3-C), in fmt.Sprint form.
	scoped map[int]string
	// heading matches an article heading in a quoted note; headings are the
	// article numbers allowed; banned are titles of articles left out.
	heading  *regexp.Regexp
	headings []string
	banned   []string
}

var euUKHeading = regexp.MustCompile(`ARTICLE (3\.\d+)`)

// euUKBanned: the EU and UK articles left out (procedural, transport or return).
var euUKBanned = []string{"Statement on origin", "Claim for preferential tariff treatment", "Verification", "Record keeping", "Non-alteration", "Returning products"}

var euUKHeadings = []string{"3.12", "3.13", "3.14", "3.15", "3.2", "3.3", "3.4", "3.5", "3.6", "3.7", "3.8", "3.9"}

var ch3Agreements = []ch3Agreement{
	{"JP-EU EPA", "JP-EU EPA Chapter 3 (Rules of origin)", "https://www.customs.go.jp/roo/english/text/eu1.pdf", "2019-02-01", euWholly, euArticles,
		[]string{"JP-EU EPA", "Articles 3.1", "do not replace the listed rule", "do not themselves establish origin", "Article 3.2 paragraph 1(b)", "context only", "unless it says otherwise", "only partly quoted", "Note 1 paragraphs 1, 2 and 4", "Note 2 paragraph 4", "Articles 3.10 and 3.11", "Annex 3-C", "Appendix 3-B-1", "3.16 to 3.29", "NOT supplied", "not verified", "undetermined"},
		nil, euUKHeading, euUKHeadings, euUKBanned},
	{"JP-UK CEPA", "JP-UK CEPA Chapter 3 (Rules of origin)", "https://www.customs.go.jp/roo/english/text/uk1.pdf", "2021-01-01", ukWholly, ukArticles,
		[]string{"JP-UK CEPA", "Articles 3.1", "do not replace the listed rule", "do not themselves establish origin", "Article 3.2 paragraph 1(b)", "context only", "unless it says otherwise", "only partly quoted", "Note 1 paragraphs 1, 2 and 4", "Note 2 paragraph 4", "Articles 3.10 and 3.11", "Annex 3-C", "Annex 3-D", "Appendix 3-B-1", "Article 3.5 paragraphs 10 to 12", "3.16 to 3.28", "NOT supplied", "not verified", "undetermined"},
		nil, euUKHeading, euUKHeadings, euUKBanned},
	{"CPTPP", "CPTPP Chapter 3 (Rules of Origin and Origin Procedures)", "https://www.customs.go.jp/roo/english/text/tpp1.pdf", "2018-12-30", "", cptppArticles,
		[]string{"CPTPP", "Articles 3.2, 3.6, 3.7, 3.10 to 3.17 and Annex 3-C", "Annex 3-D product-specific rule", "do not replace the listed rule", "do not themselves establish origin", "3.18 to 3.32", "NOT supplied", "not verified", "undetermined"},
		map[int]string{9: cptppAnnex3C, 10: cptppAnnex3C, 11: cptppAnnex3C},
		regexp.MustCompile(`Article (3\.\d+):`),
		[]string{"3.10", "3.11", "3.12", "3.13", "3.14", "3.15", "3.16", "3.17", "3.2", "3.6", "3.7"},
		[]string{"Transit and Transhipment", "Claims for Preferential Treatment", "Verification of Origin", "Net Cost", "Regional Value Content"}},
}

// CPTPP Chapter 3 (tpp1.pdf), quoted for the Annex 3-D schedule (D916).
var cptppArticles = []artNote{
	{2, "Article 3.2(a), (b) and (c), opening", "Article 3.2: Originating Goods Except as otherwise provided", "provided the good satisfies all", false},
	{3, "Article 3.2(c), end", "applicable requirements of Annex 3-D (Product-Specific Rules of Origin), and the good satisfies", "all other applicable requirements of this Chapter.", false},
	{6, "Article 3.6", "Article 3.6: Materials Used in Production 1.", "used in the production of the non-originating material undertaken in the territory of one or more of the Parties.", false},
	{6, "Article 3.7(a) to (c)(i)", "Article 3.7: Value of Materials Used in Production", "which includes general expenses; and", false},
	{7, "Article 3.7(c)(ii)", "(ii) an amount equivalent to the profit", "self-produced material that is being valued.", false},
	{12, "Article 3.10", "Article 3.10: Accumulation 1.", "sufficient to confer originating status to the material itself.", false},
	{13, "Articles 3.11, 3.12 and 3.13 paragraph 1(a)", "Article 3.11: De Minimis 1.", "are to be disregarded; and", false},
	{14, "Article 3.13 paragraph 1(b) to Article 3.15", "(b) in determining whether a good meets a regional value content requirement,", "are disregarded in determining whether a good is originating.", false},
	{15, "Articles 3.16 and 3.17", "Article 3.16: Indirect materials", "the value of non-originating materials and the value of the good.", false},
	{30, "Annex 3-C (Exceptions to Article 3.11), paragraphs (a) to (c), opening", "ANNEX 3-C EXCEPTIONS TO ARTICLE 3.11 (De Minimis)", "or a fruit or vegetable juice of any single", false},
	{30, "Annex 3-C footnote 7", "7 For greater certainty, milk powder", "the goods listed in subparagraph (b).", false},
	{31, "Annex 3-C paragraphs (c), end, to (e)", "fruit or vegetable, fortified with minerals or vitamins", "used in the production of a good of heading 20.08.", false},
}

// The goods CPTPP Annex 3-C names: its notes apply to those rows only.
const cptppAnnex3C = "[[0401 0406] [1507 1508] [1512 1512] [1514 1514] [1901 1901] [2008 2009] [2105 2106] [2202 2202] [2309 2309]]"

func chapter3Notes(s schedule, url string) (caveats, quoted []note) {
	for _, n := range s.Notes {
		switch {
		case n.Page == 0 && strings.Contains(n.Text, "Chapter 3"):
			caveats = append(caveats, n)
		case n.SourceURL == url:
			quoted = append(quoted, n)
		}
	}
	return
}

func TestChapter3NotesCaptured(t *testing.T) {
	leak := regexp.MustCompile(`(?:EU|UK)/JP/en|_{4,}|Working Group on Wine|ARTICLE 4\.1|3-\d \d|3- \d`)
	for _, a := range ch3Agreements {
		s := ch3Schedule(t, mustDefault(t), a)
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
			if sc, ok := a.scoped[i]; ok {
				wantApplies = sc
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
		if a.wholly == "" {
			continue
		}
		s := ch3Schedule(t, table, a)
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
	// in: codes inside the scoped notes' ranges (wholly obtained rows; CPTPP Annex 3-C goods); out: codes outside.
	inWholly := map[string][]string{
		"JP-EU EPA":  {"0101.21", "0406.90", "0301.11", "1101.00", "1601.00", "1602.10", "2204.21", "2309.90", "0902.10"},
		"JP-UK CEPA": {"0101.21", "0406.90", "0301.11", "1102.20", "1603.00", "2204.21", "0902.10"},
		"CPTPP":      {"0406.90", "1901.90", "2106.90", "2202.90", "2309.90", "2009.11", "1507.10"},
	}
	outWholly := map[string][]string{
		"JP-EU EPA":  {"0901.11", "1511.10", "2001.10", "2710.12", "5208.11", "6101.20", "8517.12", "2201.10"},
		"JP-UK CEPA": {"0901.11", "1101.00", "1602.10", "1511.10", "2001.10", "2710.12", "5208.11", "8517.12", "2309.90"},
		"CPTPP":      {"0901.11", "1511.10", "2001.10", "2710.12", "8517.12", "6403.99", "2201.10"},
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
		for i, n := range a.notes {
			_, scoped := a.scoped[i]
			for _, hs := range append(append([]string{}, inWholly[a.name]...), outWholly[a.name]...) {
				want := !(n.wholly || scoped) || contains(inWholly[a.name], hs)
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
				if e.Citation == caveatPrefix && strings.Contains(e.Text, "Chapter 3") && strings.Contains(e.Text, a.name) {
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
	for _, a := range ch3Agreements {
		head := a.heading
		s := ch3Schedule(t, mustDefault(t), a)
		_, quoted := chapter3Notes(s, a.url)
		seen := map[string]bool{}
		for _, n := range quoted {
			for _, m := range head.FindAllStringSubmatch(n.Text, -1) {
				seen[m[1]] = true
			}
			// no procedural article, optional article or neighbouring chapter
			for _, bad := range a.banned {
				if strings.Contains(n.Text, bad) {
					t.Errorf("%s: %q quoted in %s", a.name, bad, n.Citation)
				}
			}
		}
		want := map[string]bool{}
		for _, h := range a.headings {
			want[h] = true
		}
		if fmt.Sprint(seen) != fmt.Sprint(want) {
			t.Errorf("%s: article headings %v", a.name, seen)
		}
		for _, n := range s.Notes {
			if strings.Contains(n.Citation, "Article 3.1") && !strings.Contains(n.Citation, "Article 3.1(") && !regexp.MustCompile(`Articles? 3\.1\d`).MatchString(n.Citation) {
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

// Full EU, UK and CPTPP evidence including the Chapter 3 articles and caveat
// (EU and UK Articles 3.7 to 3.9 and 3.13 to 3.15 and the CPTPP Chapter 3 added in D916).
var chapter3Golden = map[string]string{"JP-EU EPA": "76ef54405c7a95c0", "JP-UK CEPA": "90c09980cdd026da", "CPTPP": "11b172523e8737c4"}

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
	return strings.Contains(e.Citation, "Chapter 3 (Rules of origin)") || strings.Contains(e.Citation, "Chapter 3 (Rules of Origin and Origin Procedures)") ||
		e.Citation == caveatPrefix && (strings.Contains(e.Text, "Chapter 3 (Rules of origin)") || strings.Contains(e.Text, "of Chapter 3 of the CPTPP"))
}

// ch3Schedule is the schedule of a that carries its Chapter 3 notes (CPTPP has
// two schedules from the same date: Annex 3-D carries them, Annex 4-A does not).
func ch3Schedule(t *testing.T, table *Table, a ch3Agreement) schedule {
	t.Helper()
	for _, s := range table.schedules[a.name] {
		if s.From != a.from {
			continue
		}
		for _, n := range s.Notes {
			if n.SourceURL == a.url {
				return s
			}
		}
	}
	t.Fatalf("no %s schedule from %s with notes from %s", a.name, a.from, a.url)
	return schedule{}
}
