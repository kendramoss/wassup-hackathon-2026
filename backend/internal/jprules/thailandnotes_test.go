package jprules

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

// Thailand scope notes ("Note:" / "Note N:" cells of the Annex 2 tables) pinned
// to the official PDF. See docs/japan/thailand-scope-notes.md.

var thailandDate = time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

type thaiNote struct {
	scope, key string
	page       int
	start      string
	end        string
	in, out    []string // codes that get / do not get the note
}

var thailandNotes = []thaiNote{
	{"chapter", "03", 4, "Note: For the purposes of subheadings 0306.16, 0306.17,", "good condition.",
		[]string{"0306.16", "0306.35", "030695"}, []string{"0306.19", "0306.99", "0301.11"}},
	{"chapter", "07", 5, "Note: For the purposes of subheadings 0710.22 and 0710.29,", "good condition.",
		[]string{"0710.22", "0710.29"}, []string{"0710.30", "0701.10"}},
	{"chapter", "16", 6, "Note 1: For the purposes of subheading 1604.14,", "preserve it in good condition.",
		[]string{"1604.14"}, []string{"1604.13", "1604.15"}},
	{"chapter", "16", 6, "Note 2: For the purposes of subheadings 1604.13, 1604.15,", "good condition.",
		[]string{"1604.13", "1604.15", "1604.20", "1605.21", "1605.29"}, []string{"1604.14", "1604.19"}},
	{"chapter", "18", 7, "Note: For the purposes of subheading 1803.20 and heading 18.05,", "good condition.",
		[]string{"1803.20", "1805.00"}, []string{"1803.10", "1806.90"}},
	{"chapter", "19", 8, "Note: For the purposes of subheading 1905.90:", "member country of the ASEAN.",
		[]string{"1905.90"}, []string{"1905.10", "1901.10"}},
	{"chapter", "20", 9, "Note: For the purposes of subheadings 2001.10 through 2005.60,", "member country of the ASEAN.",
		[]string{"2001.10", "2003.10", "2005.60", "2005.80", "2007.10", "2008.11", "2008.60", "2009.89"},
		[]string{"2005.70", "2007.91", "2008.20", "2009.11", "2009.90"}},
	{"chapter", "27", 12, "Note: For the purposes of this chapter, the term “chemical reaction” means", "crystallisation.",
		[]string{"2701.11", "2710.12"}, []string{"2801.10"}},
	{"section", "VI", 12, "Note: For the purposes of chapters 28 through 38:", "",
		[]string{"2801.10", "3801.10"}, []string{"2710.12", "3901.10"}},
	{"section", "VII", 20, "Note: For the purposes of chapters 39 and 40:", "",
		[]string{"3901.10", "4001.10"}, []string{"3801.10", "4101.20"}},
	{"section", "XI", 23, "Note: For the purposes of chapters 50 through 55 and 60, the dyeing or printing process", "",
		[]string{"5208.11", "6001.10", "5001.00"}, []string{"5601.21", "6101.10", "6301.10"}},
	{"chapter", "61", 26, "Note 1: For the purposes of determining the origin of a good of this chapter,", "set out in the rule for that good.",
		[]string{"6101.10"}, []string{"6201.11"}},
	{"chapter", "61", 27, "Note 2: For the purposes of this chapter, a non-originating material which is knitted", "good condition.",
		[]string{"6101.10"}, []string{"6201.11"}},
	{"chapter", "62", 27, "Note 1: For the purposes of determining the origin of a good of this chapter,", "set out in the rule for that good.",
		[]string{"6201.11"}, []string{"6101.10"}},
	{"chapter", "62", 27, "Note 2: For the purposes of this chapter, a non-originating material which is woven or knitted", "good condition.",
		[]string{"6201.11"}, []string{"6101.10"}},
	{"chapter", "63", 28, "Note: For the purposes of determining the origin of a good of this chapter,", "set out in the rule for that good.",
		[]string{"6301.10"}, []string{"6201.11"}},
	{"chapter", "71", 29, "Note: For the purposes of this chapter, natural or cultured pearls", "subheading 7101.21.",
		[]string{"7101.10", "7102.29"}, []string{"7201.10"}},
	{"chapter", "72", 29, "Note: For the purposes of this chapter, a good shall not be considered", "dimensions.",
		[]string{"7201.10"}, []string{"7301.10"}},
	{"chapter", "73", 30, "Note: For the purposes of this chapter, a good shall not be considered", "dimensions.",
		[]string{"7301.10"}, []string{"7201.10"}},
	{"chapter", "87", 35, "Note: For the purposes of headings 87.02 through 87.04,", "of the good.",
		[]string{"8702.10", "8703.21", "8704.10"}, []string{"8701.20", "8705.10", "8711.10"}},
}

func TestThailandScopeNotesCaptured(t *testing.T) {
	s := scheduleFrom(t, mustDefault(t), "JP-Thailand EPA", "2023-01-01")
	if len(s.Notes) != len(thailandNotes) {
		t.Fatalf("got %d notes, want %d", len(s.Notes), len(thailandNotes))
	}
	for i, want := range thailandNotes {
		n := s.Notes[i]
		if n.Scope != want.scope || n.Key != want.key || n.Page != want.page ||
			!strings.HasPrefix(n.Text, want.start) || !strings.HasSuffix(n.Text, want.end) {
			t.Errorf("note %d: %s %s p%d %q ... %q", i, n.Scope, n.Key, n.Page, n.Text[:min(60, len(n.Text))], n.Text[max(0, len(n.Text)-40):])
		}
	}
}

func TestThailandScopeNotesLookup(t *testing.T) {
	table := mustDefault(t)
	for _, want := range thailandNotes {
		has := func(hs string) bool {
			ev, err := table.Lookup("JP-Thailand EPA", hs, thailandDate)
			if err != nil {
				t.Fatalf("%s: %v", hs, err)
			}
			for _, e := range ev {
				if strings.HasPrefix(e.Text, want.start) && strings.HasSuffix(e.URL, fmt.Sprintf("#page=%d", want.page)) {
					return true
				}
			}
			return false
		}
		for _, hs := range want.in {
			if !has(hs) {
				t.Errorf("%s lacks %.40q", hs, want.start)
			}
		}
		for _, hs := range want.out {
			if has(hs) {
				t.Errorf("%s wrongly has %.40q", hs, want.start)
			}
		}
	}
}

func TestThailandScopeNotesFullText(t *testing.T) {
	s := scheduleFrom(t, mustDefault(t), "JP-Thailand EPA", "2023-01-01")
	for _, n := range s.Notes {
		switch {
		case n.Page == 8:
			if !strings.Contains(n.Text, "(a) a non-originating material produced entirely in a non-Party") ||
				!strings.Contains(n.Text, "(b) a non-originating material produced entirely") {
				t.Errorf("1905.90 note lost its list: %q", n.Text)
			}
		case n.Page == 29 && n.Key == "71":
			if !strings.HasSuffix(n.Text, "pearls of subheading 7101.21.") {
				t.Errorf("chapter 71 note: %q", n.Text)
			}
		}
		if strings.Contains(n.Text, "HS Classification") || strings.Contains(n.Text, "Product Specific Rule") {
			t.Errorf("column header leaked into note: %q", n.Text)
		}
	}
}
