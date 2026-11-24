package jprules

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

// General Notes of the text-layer Annexes, quoted from their own PDF with the
// page of the paragraph. See docs/japan/general-notes-embedded.md. Each entry is
// a PDF-truth fixture: the start and end words of the quoted text, the PDF page,
// the citation and the applicability read from the paragraph's own wording.

type gnNote struct {
	page       int
	label      string // after "<prefix>, "
	start, end string
	applies    [][2]string
	in, out    []string // codes that get / do not get the note
}

type gnAgreement struct {
	agreement, scheduleSub, from string // scheduleSub picks the schedule by its citation
	pdf                          string
	prefix                       string
	oldNotes                     int // notes the schedule had before this change
	notes                        []gnNote
}

const customsText = "https://www.customs.go.jp/roo/english/text/"

func rng(pairs ...string) [][2]string {
	var out [][2]string
	for i := 0; i < len(pairs); i += 2 {
		out = append(out, [2]string{pairs[i], pairs[i+1]})
	}
	return out
}

var (
	gnAnyHS  = []string{"0101.21", "2803.00", "5208.11", "6101.20", "8501.10"}
	gnTextil = []string{"5208.11", "6101.20"}
)

var gnAgreements = []gnAgreement{
	{"JP-Thailand EPA", "Annex 2", "2023-01-01", customsText + "thailand2-2.pdf", "JP-Thailand EPA Annex 2 General Notes", 20, []gnNote{
		{1, "paragraphs (b) and (c)", "(b) a requirement of a change in tariff classification", "unless otherwise specified in the Harmonized System;", nil, gnAnyHS, nil},
		{1, "paragraph (d)(v)", "(v) the term “QVC 40” means", "has been performed in a Party;", nil, gnAnyHS, nil},
		{2, "paragraphs (d)(vi) to (d)(ix)", "(vi) the term “CC” denotes", "as defined in paragraph 2 of Article 28;", nil, gnAnyHS, nil},
		{2, "paragraph (f)", "(f) specific percentages referred to in Article 30,", "10 per cent in value of the good; and",
			rng("19", "24", "28", "97"), []string{"2001.10", "2803.00", "5208.11", "6101.20", "8501.10"}, []string{"0101.21", "1701.99", "2501.00"}},
		{3, "paragraph (f)(iii)", "(iii) in the case of a good provided for in chapters 50 through 63", "10 per cent by weight of the good; and",
			rng("50", "63"), gnTextil, []string{"0101.21", "2803.00", "8501.10"}},
		{3, "Notes 1 and 2 to paragraph (f)", "Note 1: The term “value of non-originating materials”", "the value provided for in paragraph 5 of that Article.", nil, gnAnyHS, nil},
		{3, "paragraph (g)", "(g) a good which is covered by Attachment A or B", "classified in subheadings 8541.10 through 8542.90.", nil, gnAnyHS, nil},
	}},
	{"JP-Indonesia EPA", "Annex 2", "2024-02-05", "https://www.customs.go.jp/roo/text/indonesia2-2.pdf", "JP-Indonesia EPA Annex 2 General Notes", 0, []gnNote{
		{1, "paragraph (b)", "(b) reference to weight in this Annex means dry weight", "unless otherwise specified in the Harmonized System;", nil, gnAnyHS, nil},
		{1, "paragraphs (d), (e)(i) and (e)(ii)", "(d) where a specific rule of origin is defined using the criterion", "(i.e. a change in heading) of the Harmonized System;", nil, gnAnyHS, nil},
		{2, "paragraphs (e)(iii) and (e)(iv)", "(iii) the term “CTSH” denotes", "has been performed in a Party;", nil, gnAnyHS, nil},
		{2, "paragraph (g)", "(g) specific percentages referred to in Article 31,", "seven percent by weight of the good;",
			rng("28", "97"), []string{"2901.10", "5208.11", "8501.10"}, []string{"0101.21"}},
		{2, "Notes 1 and 2 to paragraph (g)", "Note 1: The term “value of non-originating materials”", "the value set out in paragraph 5 of that Article.", nil, gnAnyHS, nil},
		{2, "paragraph (h)(i)", "(h) For the purposes of chapters 28 through 40", "(C) the addition or elimination of water of crystallization;",
			rng("28", "40"), []string{"2901.10", "3901.10"}, []string{"0101.21", "5208.11", "8501.10"}},
		{3, "paragraphs (h)(ii) to (h)(iv)", "(ii) the term “purification” means", "of cellular or intercellular structures;",
			rng("28", "40"), []string{"2901.10", "3901.10"}, []string{"0101.21", "5208.11", "8501.10"}},
		{3, "paragraph (i)(i)", "(i) (i) For the purposes of chapters 50 through 55 and 60", "(2) antimelt finish;",
			rng("50", "55", "60", "60"), []string{"5208.11", "6001.10"}, []string{"0101.21", "6101.10", "8501.10"}},
		{4, "paragraph (i)(i), items (3) to (25)", "(3) antimosquito finish;", "(25) milling;",
			rng("50", "55", "60", "60"), []string{"5208.11", "6001.10"}, []string{"0101.21", "6101.10", "8501.10"}},
		{5, "paragraph (i)(i), items (26) to (48)", "(26) moare finishing;", "(48) wire raising;",
			rng("50", "55", "60", "60"), []string{"5208.11", "6001.10"}, []string{"0101.21", "6101.10", "8501.10"}},
		{6, "paragraph (i)(ii)", "(ii) For the purposes of headings 50.07, 51.11 through 51.13,", "any other operation to preserve it in good condition;",
			rng("5007", "5007", "5111", "5113", "5204", "5212", "5309", "5311", "5407", "5407", "5408", "5408", "5508", "5516", "5604", "5609", "5902", "5902", "5910", "5910", "57", "58", "60", "63"),
			[]string{"5701.10", "5208.11", "6101.10", "6001.10"}, []string{"0101.21", "5001.00", "5601.21", "8501.10"}},
		{6, "paragraph (i)(iii)", "(iii) For the purposes of determining the origin of a good of chapters 61 through 63", "set out in the rule for that good;",
			rng("61", "63"), []string{"6101.10", "6201.11"}, []string{"5208.11", "6001.10", "0101.21"}},
		{6, "paragraph (j)", "(j) (i) a good which is covered by Attachment A or B", "8541.10 through 8542.90.", nil, gnAnyHS, nil},
	}},
	{"JP-Malaysia EPA", "Annex 2", "2006-07-13", customsText + "malaysia2.pdf", "JP-Malaysia EPA Annex 2 General Notes", 9, []gnNote{
		{1, "paragraph (b)", "(b) reference to weight in this Annex means dry weight", "unless otherwise specified in the Harmonized System;", nil, gnAnyHS, nil},
		{1, "paragraph (e)", "(e) Specific percentages referred to in Article 30,", "7 percent by weight of the good;",
			rng("28", "97"), []string{"2803.00", "5208.11", "8501.10"}, []string{"0101.21", "1701.99"}},
		{2, "Notes 1 and 2 to paragraph (e)", "Note 1: the term “value of non-originating materials”", "the value set out in paragraph 5 of that Article.", nil, gnAnyHS, nil},
		{2, "paragraph (f)", "(f) (i) a good which is covered by Attachment A or B", "to subheading 8541.10 through 8542.90 from any other subheading.", nil, gnAnyHS, nil},
	}},
	{"JP-Philippines EPA", "Annex 2", "2008-12-11", customsText + "philippine2.pdf", "JP-Philippines EPA Annex 2 General Notes", 8, []gnNote{
		{1, "paragraphs (b) and (c)", "(b) reference to weight in this Annex means dry weight", "does not indicate priority of application;", nil, gnAnyHS, nil},
		{1, "paragraph (f), introduction", "(f) Specific percentages referred to in Article 31,", "are as follows:",
			rng("28", "97"), []string{"2803.00", "5208.11", "8501.10"}, []string{"0101.21", "1701.99"}},
		{2, "paragraphs (f)(i) and (f)(ii)", "(i) in the case of a good provided for in chapter 28 through 49", "7 percent by weight of the good;",
			rng("28", "97"), []string{"2803.00", "5208.11", "8501.10"}, []string{"0101.21", "1701.99"}},
		{2, "Notes 1 and 2 to paragraph (f)", "Note 1: The term “value of non-originating materials”", "the value set out in paragraph 5 of that Article.", nil, gnAnyHS, nil},
		{2, "paragraph (g)", "(g) (i) a good which is covered by Attachment A or B", "to subheading 8541.10 through 8542.90 from any other subheading.", nil, gnAnyHS, nil},
	}},
	{"JP-Mexico EPA", "Annex 4", "2005-04-01", customsText + "mexico2.pdf", "JP-Mexico EPA Annex 4 General Interpretative Notes", 9, []gnNote{
		{1, "paragraph (b)", "(b) a requirement of a change in tariff classification applies only to non-originating materials;", "applies only to non-originating materials;", nil, gnAnyHS, nil},
		{1, "paragraph (c)", "(c) reference to weight in the rules for goods provided for in chapter 1 through 24", "unless otherwise specified in the Harmonized System;",
			rng("01", "24"), []string{"0101.21", "2401.10"}, []string{"2803.00", "6101.20", "8501.10"}},
		{1, "paragraph (f)", "(f) the goods classified on Chapter 61, 62 or 63 and exported from Mexico to Japan", "calculated on a F.O.B. basis:",
			rng("61", "63"), []string{"6101.20", "6201.11", "6301.10"}, []string{"0101.21", "2501.00", "8501.10"}},
		{2, "paragraphs (f)(i) to (f)(iii)", "(i) 61.01 – 61.17 A change to heading 61.01 through 61.17 from any other chapter;", "63.10 from any other chapter; and",
			rng("61", "63"), []string{"6101.20", "6201.11", "6301.10"}, []string{"0101.21", "2501.00", "8501.10"}},
		{2, "paragraph (g)", "(g) paragraph (f) above shall be implemented in accordance with", "management of such allocation.",
			rng("61", "63"), []string{"6101.20", "6201.11", "6301.10"}, []string{"0101.21", "2501.00", "8501.10"}},
	}},
	{"JP-Chile EPA", "Annex 2", "2007-09-03", customsText + "chile2.pdf", "JP-Chile EPA Annex 2 General Notes", 6, []gnNote{
		{1, "paragraphs (b) and (c)", "(b) reference to weight in this Annex means dry weight", "applies only to non-originating materials;", nil, gnAnyHS, nil},
		{1, "paragraph (d), Build-down and Build-up methods", "the term “Build-down method” means the method referred to in subparagraph 1(a) of Article 30;", "subparagraph 1(b) of Article 30;", nil, gnAnyHS, nil},
		{2, "paragraph (f)", "(f) specific percentages referred to in Article 32,", "7 percent by weight of the good.",
			rng("19", "19", "200110", "200891", "200892", "200892", "200899", "200990", "21", "21", "28", "97"),
			[]string{"1901.10", "2001.10", "2103.10", "2803.00", "5501.10"}, []string{"0101.21"}},
		{2, "Notes 1 and 2 to paragraph (f)", "Note 1: The term “value of non-originating materials”", "the value set out in paragraph 2 of that Article.", nil, gnAnyHS, nil},
	}},
	{"JP-Vietnam EPA", "Annex 2", "2009-10-01", customsText + "vietnam2.pdf", "JP-Vietnam EPA Annex 2", 0, []gnNote{
		{1, "paragraph 1", "1. For the purposes of the product specific rules set out in this Annex and the Attachment to Annex 3, the term:", "as defined in Article 25.", nil, []string{"0901.21", "2103.90", "3915.00", "8504.10"}, nil},
		{1, "paragraph 3", "3. For the purposes of subparagraph 1(b) of Article 28, the following shall apply:", "10 percent of the FOB; and",
			rng("090121", "090122", "180310", "180320", "180500", "180500", "210390", "210390"), []string{"0901.21", "0901.22", "1803.10", "1805.00", "2103.90"}, []string{"0902.10", "2103.10", "3915.00"}},
		{2, "paragraph 3(b)", "(b) in the case of a good classified under subheading 2103.90", "seven percent of the FOB.",
			rng("210390", "210390"), []string{"2103.90"}, []string{"0901.21", "1803.10", "2103.10"}},
	}},
	{"JP-Switzerland EPA", "Appendix 1", "2009-09-01", customsText + "switzerland3.pdf", "JP-Switzerland EPA Appendix 1 to Annex II Introductory Notes", 0, []gnNote{
		{1, "paragraph 1(a)", "1. For the purposes of the product specific rules set out in this Appendix:", "as defined in Article III of Annex II;", nil, []string{"0101.21", "6101.20"}, nil},
		{2, "paragraph 1(b)", "(b) In determining the origin of a product classified under chapters 61 to 63,", "whether or not they contain textiles.",
			rng("61", "63"), []string{"6101.20"}, []string{"0101.21"}},
	}},
	{"CPTPP", "Annex 3-D", "2018-12-30", customsText + "tpp2.pdf", "CPTPP Annex 3-D Section A General Interpretative Notes", 23, []gnNote{
		{2, "paragraphs 3(c) to 3(g)", "(c) the requirement of a change in tariff classification shall apply only to non-originating materials;", "within the group.", nil, []string{"0101.21", "8401.10"}, nil},
		{2, "paragraph 5", "5. For goods of chapters 84 and 87 marked with a symbol", "Parts of Vehicles) to this Annex.",
			rng("84", "84", "87", "87"), []string{"8401.10", "8707.10"}, []string{"0101.21"}},
	}},
	{"CPTPP", "Annex 4-A", "2018-12-30", customsText + "tpp5.pdf", "CPTPP Annex 4-A Section A General Interpretative Notes", 3, []gnNote{
		{2, "paragraphs 3(c) to 3(g)", "(c) the requirement of a change in tariff classification shall apply only to non-originating materials;", "rules of origin set out in this Annex.", nil, []string{"6101.20", "6201.11"}, nil},
	}},
}

// isGeneralNote reports whether evidence is one of the General Notes above.
func isGeneralNote(e Evidence) bool {
	for _, a := range gnAgreements {
		if strings.HasPrefix(e.Citation, a.prefix+", ") {
			return true
		}
	}
	return false
}

func gnSchedule(t *testing.T, table *Table, a gnAgreement) schedule {
	t.Helper()
	for _, s := range table.schedules[a.agreement] {
		if s.From == a.from && strings.Contains(s.Citation, a.scheduleSub) {
			return s
		}
	}
	t.Fatalf("no %s %s schedule from %s", a.agreement, a.scheduleSub, a.from)
	return schedule{}
}

func TestGeneralNotesPinnedToThePDF(t *testing.T) {
	table := mustDefault(t)
	for _, a := range gnAgreements {
		s := gnSchedule(t, table, a)
		if len(s.Notes) != a.oldNotes+len(a.notes) {
			t.Fatalf("%s %s: %d notes, want %d old + %d new", a.agreement, a.scheduleSub, len(s.Notes), a.oldNotes, len(a.notes))
		}
		for i, want := range a.notes {
			n := s.Notes[a.oldNotes+i]
			where := fmt.Sprintf("%s %s note %d (%s)", a.agreement, a.scheduleSub, i, want.label)
			if n.Scope != "schedule" || n.Key != "" || n.Page != want.page || n.Ref != "" {
				t.Errorf("%s: scope %q key %q page %d ref %q", where, n.Scope, n.Key, n.Page, n.Ref)
			}
			if !strings.HasPrefix(n.Text, want.start) || !strings.HasSuffix(n.Text, want.end) {
				t.Errorf("%s: text %q ... %q", where, n.Text[:min(70, len(n.Text))], n.Text[max(0, len(n.Text)-60):])
			}
			if n.SourceURL != a.pdf || n.Citation != a.prefix+", "+want.label {
				t.Errorf("%s: source %q citation %q", where, n.SourceURL, n.Citation)
			}
			if fmt.Sprint(n.Applies) != fmt.Sprint(want.applies) {
				t.Errorf("%s: applies %v, want %v", where, n.Applies, want.applies)
			}
			if strings.Contains(n.Text, "ANNEX 3-D –") || strings.Contains(n.Text, "ANNEX 4-A –") || strings.Contains(n.Text, "Part 2 Product Specific Rules") ||
				strings.Contains(n.Text, "  ") || n.Text != strings.TrimSpace(n.Text) {
				t.Errorf("%s: footer, table text or spacing leaked: %q", where, n.Text)
			}
		}
	}
}

func TestGeneralNotesExistingNotesUnchanged(t *testing.T) {
	// The old notes stay first and gain no source of their own.
	table := mustDefault(t)
	for _, a := range gnAgreements {
		s := gnSchedule(t, table, a)
		for i, n := range s.Notes[:a.oldNotes] {
			if n.SourceURL != "" || n.Citation != "" {
				t.Errorf("%s note %d gained a source", a.agreement, i)
			}
		}
	}
}

func TestGeneralNotesLookupScopes(t *testing.T) {
	table := mustDefault(t)
	date := func(a gnAgreement) time.Time {
		d, _ := time.Parse(time.DateOnly, a.from)
		return d.AddDate(0, 0, 7)
	}
	for _, a := range gnAgreements {
		for _, want := range a.notes {
			page := fmt.Sprintf("#page=%d", want.page)
			cite := fmt.Sprintf("%s, %s, PDF page %d", a.prefix, want.label, want.page)
			has := func(hs string) bool {
				res, err := table.LookupDetailed(a.agreement, hs, date(a))
				if err != nil || res.Reason != ReasonPSR {
					t.Fatalf("%s %s: reason %q err %v", a.agreement, hs, res.Reason, err)
				}
				for _, e := range res.Evidence {
					if strings.HasPrefix(e.Text, want.start) && e.URL == a.pdf+page && e.Citation == cite {
						return true
					}
				}
				return false
			}
			for _, hs := range want.in {
				if !has(hs) {
					t.Errorf("%s %s lacks %.50q (%s)", a.agreement, hs, want.start, cite)
				}
			}
			for _, hs := range want.out {
				if has(hs) {
					t.Errorf("%s %s wrongly has %.50q", a.agreement, hs, want.start)
				}
			}
		}
	}
}

func TestGeneralNotesAreQuotesNotParaphrases(t *testing.T) {
	table := mustDefault(t)
	byName := map[string]string{}
	for _, a := range gnAgreements {
		var b strings.Builder
		for _, n := range gnSchedule(t, table, a).Notes {
			if n.SourceURL != "" {
				b.WriteString(n.Text + "\n")
			}
		}
		byName[a.agreement+" "+a.scheduleSub] = b.String()
	}
	must := map[string][]string{
		"JP-Thailand EPA Annex 2": {
			"the term “QVC 40” means that the good has a qualifying value content, calculated using the formula set out in paragraph 4 of Article 28, of not less than 40 per cent, and the final process of production has been performed in a Party;",
			"(ix) the term “WO” means that the good is wholly obtained or produced entirely in a Party as defined in paragraph 2 of Article 28;",
			"(i) in the case of a good provided for in chapters 19 through 24 of the Harmonized System, seven per cent in value of the good; (ii) in the case of a good provided for in chapters 28 through 49 and 64 through 97 of the Harmonized System, 10 per cent in value of the good; and",
			"Note 2: The term “value of the good” means the free-on-board value of the good referred to in subparagraph 4(b) of Article 28",
		},
		"JP-Indonesia EPA Annex 2": {
			"(iv) the term “QVC 40” or “QVC 50” respectively means",
			"(AA) pharmaceutical, medical, cosmetic, veterinary or food grade substances;",
			"(1) antibacterial finish; (2) antimelt finish;", "(25) milling;", "(26) moare finishing;", "(47) windbreak finish; or (48) wire raising;",
			"(B) through one or more non-Parties for the purpose of transit or temporary storage in warehouses in such non-Parties",
			"shall only apply to the component that determines the tariff classification of the good",
		},
		"JP-Vietnam EPA Annex 2": {
			"“LVC 40%” means that the good has a local value content, calculated using the formula set out in Article 27, of not less than 40 percent, and the final process of production has been performed in a Party; Note: For the purposes of this Annex, subparagraph 2(a) of Article 27 shall apply.",
			"0901.21, 0901.22, 1803.10, 1803.20, and 1805.00 of the HS",
		},
		"JP-Chile EPA Annex 2": {
			"the term “Build-down method” means the method referred to in subparagraph 1(a) of Article 30; and the term “Build-up method” means the method referred to in subparagraph 1(b) of Article 30;",
			"(i) in the case of a good provided for in chapter 19, subheading 2001.10 through 2008.91, 2008.99 through 2009.90 and chapter 21 of the Harmonized System, 7 percent in value of the good;",
		},
		"JP-Mexico EPA Annex 4": {
			"for an annual aggregate value of such goods in total imported to Japan of 200,000,000 United States dollars calculated on a F.O.B. basis:",
			"Mexico shall implement and manage the allocation of the aggregate value",
		},
		"JP-Switzerland EPA Appendix 1": {
			"“VNM” denotes the maximum value, in percentage of the ex-works price of the product as indicated by the figure immediately after it, of non-originating materials used in the production of the product.",
			"materials used in the production of the product which are not classified under chapters 50 to 63 shall be disregarded",
		},
		"CPTPP Annex 3-D": {
			"it shall be construed to mean that the product-specific rule of origin requires that the excluded materials be originating for the good to be originating;",
			"if a good is subject to alternative product-specific rules of origin, the good shall be originating if it satisfies one of the alternatives;",
			"an optional methodology for satisfying the regional value content requirement",
		},
		"CPTPP Annex 4-A": {
			"the Short Supply List of Products, set out as Appendix 1 to this Annex, is read in conjunction with the product-specific rules of origin set out in this Annex.",
		},
		"JP-Malaysia EPA Annex 2": {
			"where the good covered by Attachment A or B of the Declaration referred to in the said subparagraph is classified in subheading 8541.10 through 8542.90",
		},
		"JP-Philippines EPA Annex 2": {
			"the order of the description of the rules does not indicate priority of application;",
		},
	}
	for name, frags := range must {
		text, ok := byName[name]
		if !ok {
			t.Fatalf("no %s", name)
		}
		for _, f := range frags {
			if !strings.Contains(text, f) {
				t.Errorf("%s lacks %q", name, f)
			}
		}
	}
}

func TestGeneralNotesLeaveOutWhatDoesNotChangeQualification(t *testing.T) {
	// Cuts documented in docs/japan/general-notes-embedded.md: the HS edition
	// statements, the digit-prefix definitions of section, chapter, heading and
	// subheading, the "adjacent rule" paragraph and CPTPP paragraphs 1, 2, 3(a), 3(b), 4.
	table := mustDefault(t)
	for _, a := range gnAgreements {
		for _, n := range gnSchedule(t, table, a).Notes[a.oldNotes:] {
			for _, cut := range []string{
				"is based on the Harmonized System", "means the first four digits", "means the first six digits",
				"means a section of the Harmonized System", "is set out immediately adjacent",
				"are contained in Annex 4-A", "Under this Annex, a good is an originating good", "are found at the beginning of each",
			} {
				if strings.Contains(n.Text, cut) {
					t.Errorf("%s: cut text %q is embedded in %.60q", a.agreement, cut, n.Text)
				}
			}
		}
	}
}
