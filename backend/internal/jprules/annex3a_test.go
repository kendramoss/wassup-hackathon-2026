package jprules

import (
	"crypto/sha256"
	"fmt"
	"sort"
	"strings"
	"testing"
	"time"
)

// Annex 3-A (introductory notes) of the JP-EU EPA and JP-UK CEPA, quoted with a
// page and source of their own, and the Section and Chapter note cells of
// Annex 3-B. See docs/japan/annex-3a-notes.md.

// ruleEvidence drops the Annex 3-A notes, the Section and Chapter note cells
// of Annex 3-B and the embedded General Notes (isGeneralNote), which tests of
// the rule rows do not count.
func ruleEvidence(ev []Evidence) []Evidence {
	var out []Evidence
	for _, e := range ev {
		if !strings.Contains(e.Citation, "Annex 3-A") && !isGeneralNote(e) && !strings.HasPrefix(e.Text, "Section note:") && !strings.HasPrefix(e.Text, "Chapter note:") {
			out = append(out, e)
		}
	}
	return out
}

var annexDate = time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

type annexNote struct {
	scope, key string
	page       int
	label      string // after "<agreement> Annex 3-A, "
	start, end string
	applies    [][2]string
	in, out    []string // codes that get / do not get the note
}

var anyCode = []string{"0101.21", "2710.12", "5208.11", "8517.12"}

// chapters 25-40 (Note 5 is "referred to in Sections V to VII")
var note5In, note5Out = []string{"2501.00", "2710.12", "2804.10", "3808.91", "3901.10", "4001.10"}, []string{"0101.21", "2401.10", "4101.20", "5208.11"}

var textileIn, textileOut = []string{"5001.00", "5208.11", "5601.21", "6101.20", "6301.10"}, []string{"4901.99", "6401.10", "0101.21", "8517.12"}

func sched(page int, label, start, end string) annexNote {
	return annexNote{"schedule", "", page, label, start, end, nil, anyCode, nil}
}

func five(page int, label, start, end string) annexNote {
	return annexNote{"schedule", "", page, label, start, end, [][2]string{{"25", "40"}}, note5In, note5Out}
}

func xi(page int, label, start, end string) annexNote {
	return annexNote{"section", "XI", page, label, start, end, nil, textileIn, textileOut}
}

var (
	r7_5 = [][2]string{{"5106", "5110"}, {"5204", "5207"}}
	r8_2 = [][2]string{{"61", "63"}}
)

func ranged(n annexNote, r [][2]string, in, out []string) annexNote {
	n.applies, n.in, n.out = r, in, out
	return n
}

var euAnnex3A = []annexNote{
	sched(1, "Note 1 paragraph 3", "3. Reference to weight in a product specific rule of origin means the net weight,", "not including the weight of packaging."),
	sched(2, "Note 2 paragraphs 1 to 3", "Note 2 The structure of Annex 3-B 1. Notes on Sections or Chapters", "satisfies all of the requirements."),
	sched(3, "Note 2 paragraph 5", `5. For the purposes of product specific rules of origin, the following abbreviations apply:1 "CC" means`, "individually or jointly."),
	sched(4, "Note 3", "Note 3 Application of Annex 3-B 1. Paragraph 3 of Article 3.2,", "because of their inherent nature."),
	sched(5, "Note 4", "Note 4 Calculation of a maximum value of non-originating materials and of a minimum regional value content Definitions: 1.", "when the product obtained is exported; or"),
	sched(6, "Note 4", "(ii) if there is no price paid or payable or if the actual price paid does not reflect all", "when the product obtained is exported; or"),
	sched(7, "Note 4", "(ii) if there is no price paid or payable or if the actual price paid does not reflect all", "regional value content of a product, expressed as a percentage; and"),
	sched(8, "Note 4", `(f) "VNM" means the value of non-originating materials`, "MaxNOM(%) = (VNM / EXW) × 100 RVC(%) = ((FOB − VNM) / FOB) × 100"),
	five(9, "Note 5", "Note 5 Definitions of processes referred to in Sections V to VII in Annex 3-B", "different from those of the input materials;"),
	five(10, "Note 5", `(c) "chemical reaction" means a process`, "to produce light to heavy vacuum gas oils and residuum;"),
	five(11, "Note 5", `(e) "isomer separation" means`, "content of existing impurities."),
	xi(11, "Note 6", "Note 6 Definitions of terms used in Section XI of Annex 3-B", "of headings 55.01 to 55.07;"),
	xi(12, "Note 6", `(b) "natural fibres" means fibres other than`, "45 per cent of the FOB of the product."),
	xi(13, "Note 7", "Note 7 Tolerances applicable to products containing two or more basic textile materials 1.", "(k) sisal and other textile fibres of the genus Agave;"),
	xi(14, "Note 7", "(l) coconut, abaca, ramie", "(v) synthetic man-made staple fibres of poly (phenylene sulphide);"),
	xi(15, "Note 7", "(w) synthetic man-made staple fibres of poly (vinyl chloride);", "(ff) metal fibres."),
	xi(16, "Note 7", "2. Where reference to this Note is made in Annex 3-B,", "shall not exceed 10 per cent."),
	xi(17, "Note 7 paragraph 4", "4. Notwithstanding Note 7.2 (b), for products containing", "shall not exceed 10 per cent."),
	ranged(xi(17, "Note 7 paragraph 5", "5. For a product of headings 51.06 to 51.10 and headings 52.04 to 52.07,", "40 per cent of the weight of the product."), r7_5,
		[]string{"5106.10", "5107.10", "5110.00", "5204.11", "5207.10"}, []string{"5111.11", "5208.11", "5105.10", "5203.00", "6101.20"}),
	xi(17, "Note 8 paragraph 1", "Note 8 Other tolerances applicable to certain textile products 1.", "8 per cent of the EXW or FOB of the product."),
	ranged(xi(18, "Note 8 paragraph 2", "2. Non-originating materials which are not classified in Chapters 50 to 63 may be used", "slide-fasteners normally contain textiles."), r8_2,
		[]string{"6101.20", "6203.42", "6301.10"}, []string{"5208.11", "5001.00", "5601.21", "6401.10"}),
	xi(18, "Note 8 paragraph 3", "3. Where a requirement set out in Annex 3-B consists in a maximum value", "calculation of the value of the non-originating materials."),
}

var ukAnnex3A = []annexNote{
	sched(1, "Note 1 paragraph 3", "3. Reference to weight in a product specific rule of origin means the net weight,", "not including the weight of packaging."),
	sched(1, "Note 2 paragraph 1", "Note 2 The structure of Annex 3-B 1. Notes on Sections or Chapters", "for the relevant Section, Chapter, heading or subheading."),
	sched(2, "Note 2 paragraphs 2 and 3", "2. Each product specific rule of origin set out in Column 2 of Annex 3-B", "satisfies all of the requirements."),
	sched(2, "Note 2 paragraph 5", `5. For the purposes of product specific rules of origin, the following abbreviations apply:1 (a) "CC" means`, "individually or jointly."),
	sched(3, "Note 2 paragraph 5", `(b) "CTH" means production from non-originating materials`, "in subheading) of the Harmonized System."),
	sched(3, "Note 3", "Note 3 Application of Annex 3-B 1. Paragraph 3 of Article 3.2,", "because of their inherent nature."),
	sched(4, "Note 3 paragraph 4", "4. For greater certainty, in respect of materials which are wholly obtained,", "determine whether a material is wholly obtained in the European Union."),
	sched(4, "Note 4", "Note 4 Calculation of a maximum value of non-originating materials and of a minimum regional value content Definitions: 1.", "when the product obtained is exported; or"),
	sched(5, "Note 4", "(ii) if there is no price paid or payable or if the actual price paid does not reflect all", "repaid when the product obtained is exported;"),
	sched(6, "Note 4", `(d) "MaxNOM" means the maximum value`, "(b) RVC(%) = ((FOB − VNM) / FOB) × 100"),
	five(7, "Note 5", "Note 5 Definitions of processes referred to in Sections V to VII in Annex 3-B", "(iii) the addition or elimination of water of crystallisation;"),
	five(8, "Note 5", `(d) "distillation" means:`, "content of existing impurities."),
	xi(9, "Note 6", "Note 6 Definitions of terms used in Section XI of Annex 3-B", "45 per cent of the FOB of the product."),
	xi(10, "Note 7", "Note 7 Tolerances applicable to products containing two or more basic textile materials 1.", "(n) artificial man-made filaments;"),
	xi(11, "Note 7", "(o) current-conducting filaments;", "between two layers of plastic film;"),
	xi(12, "Note 7", "(dd) other products of heading 56.05;", "shall not exceed 10 per cent."),
	ranged(xi(13, "Note 7 paragraph 5", "5. For a product of headings 51.06 to 51.10 and 52.04 to 52.07,", "40 per cent of the weight of the product."), r7_5,
		[]string{"5106.10", "5107.10", "5110.00", "5204.11", "5207.10"}, []string{"5111.11", "5208.11", "5105.10", "5203.00", "6101.20"}),
	xi(13, "Note 8 paragraph 1", "Note 8 Other tolerances applicable to certain textile products 1.", "of the EXW or FOB of the product."),
	ranged(xi(13, "Note 8 paragraph 2", "2. For the purposes of determining whether a product classified under Chapters 61 to 63", "for that product."), r8_2,
		[]string{"6101.20", "6203.42", "6301.10"}, []string{"5208.11", "5001.00", "5601.21", "6401.10"}),
	xi(13, "Note 8 paragraph 3", "3. Where a requirement set out in Column 2 of Annex 3-B consists in a maximum value", "calculation of the value of the non-originating materials."),
}

// The Section and Chapter note cells of Annex 3-B itself (own PDF, own page).
type cellNote struct {
	scope, key string
	page       int
	text       string
	in, out    []string
}

const sectionNote5 = "Section note: For definitions of horizontal processing rules within this Section, see Note 5 of Annex 3-A."

var euCells = []cellNote{
	{"section", "V", 12, sectionNote5, []string{"2501.00", "2710.12"}, []string{"2804.10", "0101.21"}},
	{"section", "VI", 14, sectionNote5, []string{"2804.10", "3808.91"}, []string{"2710.12", "3901.10"}},
	{"section", "VII", 24, sectionNote5, []string{"3901.10", "4001.10"}, []string{"3808.91", "4101.20"}},
	{"section", "XI", 30, "Section note: For definitions of terms used for and tolerances applicable to certain products made of textile materials, see Notes 6, 7 and 8 of Annex 3-A.", []string{"5001.00", "6101.20"}, []string{"4901.99", "6401.10"}},
	{"chapter", "57", 38, "Chapter note: For products of this Chapter jute fabric may be used as a backing.", []string{"5701.10", "5705.00"}, []string{"5601.21", "5801.10"}},
}

var ukCells = []cellNote{
	{"section", "V", 14, sectionNote5, euCells[0].in, euCells[0].out},
	{"section", "VI", 16, sectionNote5, euCells[1].in, euCells[1].out},
	{"section", "VII", 26, sectionNote5, euCells[2].in, euCells[2].out},
	{"section", "XI", 32, euCells[3].text, euCells[3].in, euCells[3].out},
	{"chapter", "57", 40, euCells[4].text, euCells[4].in, euCells[4].out},
}

type annexAgreement struct {
	name, label, url string
	from             string
	notes            []annexNote
	cells            []cellNote
	totalNotes       int
}

var annexAgreements = []annexAgreement{
	{"JP-EU EPA", "JP-EU EPA Annex 3-A", "https://www.customs.go.jp/roo/english/text/eu2.pdf", "2019-02-01", euAnnex3A, euCells, 1 + 5 + 22},
	{"JP-UK CEPA", "JP-UK CEPA Annex 3-A", "https://www.customs.go.jp/roo/english/text/uk2.pdf", "2021-01-01", ukAnnex3A, ukCells, 5 + 20},
}

func TestAnnex3ANotesCaptured(t *testing.T) {
	for _, a := range annexAgreements {
		s := scheduleFrom(t, mustDefault(t), a.name, a.from)
		if len(s.Notes) != a.totalNotes {
			t.Fatalf("%s: got %d notes, want %d", a.name, len(s.Notes), a.totalNotes)
		}
		var cells, annex []note
		for _, n := range s.Notes {
			if n.SourceURL == "" {
				cells = append(cells, n)
			} else {
				annex = append(annex, n)
			}
		}
		if len(annex) != len(a.notes) {
			t.Fatalf("%s: %d Annex 3-A notes, want %d", a.name, len(annex), len(a.notes))
		}
		for i, want := range a.notes {
			n := annex[i]
			if n.Scope != want.scope || n.Key != want.key || n.Page != want.page ||
				!strings.HasPrefix(n.Text, want.start) || !strings.HasSuffix(n.Text, want.end) ||
				n.SourceURL != a.url || n.Citation != a.label+", "+want.label || fmt.Sprint(n.Applies) != fmt.Sprint(want.applies) {
				t.Errorf("%s note %d: %s %q p%d %q ... %q %q %v", a.name, i, n.Scope, n.Key, n.Page, n.Text[:min(70, len(n.Text))], n.Text[max(0, len(n.Text)-50):], n.Citation, n.Applies)
			}
			if strings.Contains(n.Text, "EU/JP/Annex") || strings.Contains(n.Text, "________") {
				t.Errorf("%s note %d: footer or rule line leaked: %q", a.name, i, n.Text)
			}
		}
		// The Annex 3-B cells keep the schedule's own URL and the old page rule.
		wantCells := a.cells
		if a.name == "JP-EU EPA" {
			if len(cells) != 6 || cells[len(cells)-1].Key != "70" {
				t.Fatalf("EU cells: %+v", cells)
			}
		}
		for _, want := range wantCells {
			found := false
			for _, n := range cells {
				if n.Scope == want.scope && n.Key == want.key && n.Page == want.page && n.Text == want.text && len(n.Applies) == 0 && n.Citation == "" {
					found = true
				}
			}
			if !found {
				t.Errorf("%s lacks cell note %s %s p%d", a.name, want.scope, want.key, want.page)
			}
		}
	}
}

func TestAnnex3ANotesLookup(t *testing.T) {
	table := mustDefault(t)
	for _, a := range annexAgreements {
		check := func(hs, start, urlEnd, citation string) bool {
			ev, err := table.Lookup(a.name, hs, annexDate)
			if err != nil || len(ev) == 0 {
				t.Fatalf("%s %s: %v %v", a.name, hs, ev, err)
			}
			for _, e := range ev {
				if strings.HasPrefix(e.Text, start) && strings.HasSuffix(e.URL, urlEnd) && strings.Contains(e.Citation, citation) {
					return true
				}
			}
			return false
		}
		for _, want := range a.notes {
			page := fmt.Sprintf("#page=%d", want.page)
			cite := fmt.Sprintf("%s, %s, PDF page %d", a.label, want.label, want.page)
			for _, hs := range want.in {
				if !check(hs, want.start, a.url[strings.LastIndex(a.url, "/"):]+page, cite) {
					t.Errorf("%s %s lacks %.40q (%s)", a.name, hs, want.start, cite)
				}
			}
			for _, hs := range want.out {
				if check(hs, want.start, page, a.label) {
					t.Errorf("%s %s wrongly has %.40q", a.name, hs, want.start)
				}
			}
		}
		for _, want := range a.cells {
			for _, hs := range want.in {
				if !check(hs, want.text, fmt.Sprintf("%s#page=%d", a.annexBURL(), want.page), a.name) {
					t.Errorf("%s %s lacks cell note %.40q", a.name, hs, want.text)
				}
			}
			for _, hs := range want.out {
				if check(hs, want.text, fmt.Sprintf("#page=%d", want.page), a.name) {
					t.Errorf("%s %s wrongly has cell note %.40q", a.name, hs, want.text)
				}
			}
		}
	}
}

// annexBURL is the Annex 3-B PDF of the schedule.
func (a annexAgreement) annexBURL() string {
	if a.name == "JP-EU EPA" {
		return "https://www.customs.go.jp/roo/english/text/eu3.pdf"
	}
	return "https://www.customs.go.jp/roo/english/text/uk3.pdf"
}

func TestAnnex3AKeyDefinitionsAreQuoted(t *testing.T) {
	for _, a := range annexAgreements {
		s := scheduleFrom(t, mustDefault(t), a.name, a.from)
		var all strings.Builder
		for _, n := range s.Notes {
			all.WriteString(n.Text + "\n")
		}
		for _, want := range []string{
			`"MaxNOM" means the maximum value of non-originating materials expressed as a percentage;`,
			`"RVC" means the minimum regional value content of a product, expressed as a percentage;`,
			`"EXW" means:`, `"FOB" means:`, `"VNM" means the value of non-originating materials`,
			`"CC" means production from non-originating materials of any Chapter`,
			`"CTH" means production from non-originating materials of any heading`,
			`"CTSH" means production from non-originating materials of any subheading`,
			"the product shall be originating if it satisfies one of the alternatives.",
			"those requirements do not apply to non-originating materials classified elsewhere in the Harmonized System.",
			"10 per cent of the",
			"of the EXW or FOB of the product.",
		} {
			if !strings.Contains(all.String(), want) {
				t.Errorf("%s lacks %q", a.name, want)
			}
		}
	}
}

func TestAnnex3AEUAndUKDifferWhereThePDFsDo(t *testing.T) {
	// UK Note 3 paragraph 4 (wholly obtained EU materials) and Note 8 paragraph 2
	// (Chapters 61 to 63: only the component that determines classification)
	// exist only in the UK Annex 3-A.
	eu := scheduleFrom(t, mustDefault(t), "JP-EU EPA", "2019-02-01")
	uk := scheduleFrom(t, mustDefault(t), "JP-UK CEPA", "2021-01-01")
	has := func(s schedule, frag string) bool {
		for _, n := range s.Notes {
			if strings.Contains(n.Text, frag) {
				return true
			}
		}
		return false
	}
	for frag, wantUK := range map[string]bool{
		"wholly obtained in the European Union":                        true,
		"shall apply only to the component that determines the tariff": true,
		"may be used without restriction in the production of textile": false,
	} {
		if has(uk, frag) != wantUK || has(eu, frag) == wantUK {
			t.Errorf("%q: uk=%v eu=%v", frag, has(uk, frag), has(eu, frag))
		}
	}
}

func TestSourcedNoteValidation(t *testing.T) {
	base := func(n note) schedule {
		return schedule{Agreement: "A", Citation: "A Annex", URL: "https://x.example/a.pdf", From: "2020-01-01",
			Rows:  []row{{Start: "0101", End: "0101", Page: 1, Text: "CC"}},
			Notes: []note{n}}
	}
	ok := note{Scope: "schedule", Text: "t", Page: 2, SourceURL: "https://x.example/b.pdf", Citation: "A Annex 3-A, Note 1"}
	if err := validateSchedule(base(ok)); err != nil {
		t.Fatalf("valid sourced note rejected: %v", err)
	}
	if err := validateSchedule(base(note{Scope: "schedule", Text: "t", Page: 2})); err != nil {
		t.Fatalf("plain note rejected: %v", err)
	}
	// A source without PDF pages (a web notice) is located by ref on page 1.
	if err := validateSchedule(base(note{Scope: "schedule", Text: "t", Page: 1, Ref: "r", SourceURL: ok.SourceURL, Citation: ok.Citation})); err != nil {
		t.Fatalf("sourced note with a ref rejected: %v", err)
	}
	bad := map[string]note{
		"url without citation": {Scope: "schedule", Text: "t", Page: 2, SourceURL: ok.SourceURL},
		"citation without url": {Scope: "schedule", Text: "t", Page: 2, Citation: ok.Citation},
		"url with fragment":    {Scope: "schedule", Text: "t", Page: 2, SourceURL: "https://x.example/b.pdf#page=2", Citation: ok.Citation},
		"url not https":        {Scope: "schedule", Text: "t", Page: 2, SourceURL: "http://x.example/b.pdf", Citation: ok.Citation},
		"padded citation":      {Scope: "schedule", Text: "t", Page: 2, SourceURL: ok.SourceURL, Citation: " A "},
		"padded url":           {Scope: "schedule", Text: "t", Page: 2, SourceURL: " https://x.example/b.pdf", Citation: ok.Citation},
		"no page":              {Scope: "schedule", Text: caveatPrefix + " t", Page: 0, SourceURL: ok.SourceURL, Citation: ok.Citation},
		"with ref on page 2":   {Scope: "schedule", Text: "t", Page: 2, Ref: "r", SourceURL: ok.SourceURL, Citation: ok.Citation},
	}
	for name, n := range bad {
		if err := validateSchedule(base(n)); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestSourcedNoteEvidenceUsesItsOwnSource(t *testing.T) {
	s := schedule{Agreement: "A", Citation: "A Annex 3-B", URL: "https://x.example/a.pdf", From: "2020-01-01",
		Rows: []row{{Start: "0101", End: "0101", Page: 1, Text: "CC"}},
		Notes: []note{
			{Scope: "schedule", Text: "own", Page: 3},
			{Scope: "schedule", Text: "other", Page: 7, SourceURL: "https://x.example/b.pdf", Citation: "A Annex 3-A, Note 4"},
		}}
	got := s.notesFor("010101")
	want := []Evidence{
		{Text: "own", URL: "https://x.example/a.pdf#page=3", Citation: "A Annex 3-B, PDF page 3"},
		{Text: "other", URL: "https://x.example/b.pdf#page=7", Citation: "A Annex 3-A, Note 4, PDF page 7"},
	}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("got %+v", got)
	}
}

// Golden hash of every other schedule, taken on main before this change: the
// rows and notes returned for each listed code at the schedule's first day. For
// EU and UK the Annex 3-A notes and the Section and Chapter note cells are left
// out, so only those two evidence lists may change.
// JP-India CEPA changed deliberately (a second window from 2022-04-04, docs/japan/india-chapter3-amendment.md).
// JP-Australia EPA changed deliberately (column 4 and General Notes, docs/japan/australia-column4.md).
var annexGolden = map[string]string{
	"AJCEP": "8bf3fd2c4050a40b", "CPTPP": "bf1e2b327d3058aa", "GSP": "f92eb48efa4bda93",
	"JP-Australia EPA": "c1b8267d06781365", "JP-Chile EPA": "d2a9e16a64373c8f", "JP-EU EPA": "611312c2392bec7c",
	"JP-India CEPA": "2d3d95dbbc3ac286", "JP-Indonesia EPA": "9dbd1aab47f662ca", "JP-Malaysia EPA": "51fd6558fa3d530a",
	"JP-Mexico EPA": "fb63a1bc3cc2def6", "JP-Philippines EPA": "183174e8a417fa6e", "JP-Switzerland EPA": "cf787dcabdd33d67",
	"JP-Thailand EPA": "b4fdad59224353b0", "JP-UK CEPA": "7ce305399eff57ef", "JP-US TA": "ecb919bc1c7ecf01",
	"JP-Vietnam EPA": "eb7adeac63c68982",
}

// ajcepProvisionsGolden pins the AJCEP evidence including the Annex 2
// provisions that now follow a matched rule (psr_provisions, psr_notes). The
// AJCEP entry in annexGolden still pins the rows and notes alone: the
// appended provisions are cut from that hash, so it is unchanged from main.
const ajcepProvisionsGolden = "02c4f1c66b22b72f"

func TestOtherEvidenceByteIdentical(t *testing.T) {
	// The General Notes embedded for nine agreements are left out here: with
	// them out, every agreement's rows and old notes are byte-identical to main
	// (the hashes above). TestGeneralNotesEvidenceHashes pins the nine with them in.
	goldenHashes(t, annexGolden, func(e Evidence) bool { return isGeneralNote(e) })
}

// General Notes of Thailand, Indonesia, Malaysia, the Philippines, Chile, Vietnam,
// Switzerland, Mexico and CPTPP (docs/japan/general-notes-embedded.md): the same
// hash with their evidence kept. Only these nine agreements differ from the hashes
// above, because their lookups gained the quoted notes; every other agreement is
// unchanged and is checked by the test above.
var generalNotesGolden = map[string]string{
	"CPTPP": "65694caafec49018", "JP-Chile EPA": "3180e63f2aa6768d", "JP-Indonesia EPA": "3570ca8a59f0d32e",
	"JP-Malaysia EPA": "25f293b0d9d1c90e", "JP-Mexico EPA": "87e99c0b8806dda9", "JP-Philippines EPA": "799ea99e2b4ced59",
	"JP-Switzerland EPA": "6f5d5c2e5751639b", "JP-Thailand EPA": "9fb9eb191f08ea22", "JP-Vietnam EPA": "cef54b35473849f8",
}

func TestGeneralNotesEvidenceHashes(t *testing.T) {
	want := map[string]string{}
	for a, h := range annexGolden {
		want[a] = h
	}
	for a, h := range generalNotesGolden {
		want[a] = h
	}
	goldenHashes(t, want, func(Evidence) bool { return false })
}

func goldenHashes(t *testing.T, golden map[string]string, skip func(Evidence) bool) {
	table := mustDefault(t)
	var names []string
	for a := range table.schedules {
		names = append(names, a)
	}
	sort.Strings(names)
	if len(names) != len(golden) {
		t.Fatalf("agreements %v", names)
	}
	for _, a := range names {
		h, full := sha256.New(), sha256.New()
		for _, s := range table.schedules[a] {
			d, _ := time.Parse(time.DateOnly, s.From)
			seen := map[string]bool{}
			for _, r := range s.Rows {
				for _, c := range []string{r.Start, r.End} {
					if seen[c] {
						continue
					}
					seen[c] = true
					res, _ := table.LookupDetailed(a, c, d)
					fmt.Fprintf(h, "%s|%s|%s|%s\n", a, s.From, c, res.Reason)
					fmt.Fprintf(full, "%s|%s|%s|%s\n", a, s.From, c, res.Reason)
					for _, e := range res.Evidence {
						fmt.Fprintf(full, "%q|%s|%s\n", e.Text, e.URL, e.Citation)
					}
					if a == "AJCEP" && res.Reason == ReasonPSR && table.general.applies(a, s.From) {
						res.Evidence = res.Evidence[:len(res.Evidence)-len(table.general.psrEvidence())]
					}
					for _, e := range res.Evidence {
						if strings.Contains(e.Citation, "Annex 3-A") || strings.HasPrefix(e.Text, "Section note:") || strings.HasPrefix(e.Text, "Chapter note:") || skip(e) {
							continue
						}
						fmt.Fprintf(h, "%q|%s|%s\n", e.Text, e.URL, e.Citation)
					}
				}
			}
		}
		if got := fmt.Sprintf("%x", h.Sum(nil))[:16]; got != golden[a] {
			t.Errorf("%s: evidence changed (%s, was %s)", a, got, golden[a])
		}
		if a == "AJCEP" {
			if got := fmt.Sprintf("%x", full.Sum(nil))[:16]; got != ajcepProvisionsGolden {
				t.Errorf("AJCEP: evidence with Annex 2 provisions changed (%s, was %s)", got, ajcepProvisionsGolden)
			}
		}
	}
}
