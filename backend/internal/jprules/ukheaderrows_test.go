package jprules

import (
	"fmt"
	"strings"
	"testing"
)

// JP-UK CEPA heading rows whose rule cell is empty in uk3.pdf. Their rules are
// the "-" lines below the heading; each line is one row keyed to the heading's
// codes with the line label as description. Texts are the PDF cells verbatim
// (page given). See docs/japan/uk-header-rows.md.

const (
	ukAgreement = "JP-UK CEPA"
	ukFrom      = "2021-01-01"
	ukRuleOther = "CTH; MaxNOM 50 % (EXW); or RVC 55 % (FOB)."
)

type ukBranch struct {
	start, end  string
	page        int
	description string
	text        string
}

var ukBranches = []ukBranch{
	{"7006", "7006", 57, "- Coated glass-plate substrates", "CTH; or Production from non-coated glass-plate substrate of heading 70.06."},
	{"7006", "7006", 57, "- Others", "CTH except from headings 70.02 to 70.05."},
	{"7010", "7010", 58, "- Glass and glassware, containers made of glass", "CTH; however, non-originating materials of heading 70.10 may be used, provided that their total value does not exceed 15 % of the EXW or the FOB of the product."},
	{"7010", "7010", 58, "- Others", ukRuleOther},
	{"1515", "1515", 5, "- Rice bran oil and its fractions", "Production in which all the materials of headings 10.06 and 15.15 used are wholly obtained."},
	{"1515", "1515", 5, "- Others", "CTH"},
	{"190110", "190110", 7, "- Containing more than 10 % by dry weight of milk solids", "CC except from headings 04.01 to 04.06."},
	{"190110", "190110", 7, "- Others", "CC"},
	{"6101", "6117", 48, "- Obtained by sewing together or otherwise assembling, two or more pieces of knitted or crocheted fabric which have been either cut to form or obtained directly to form", "Knitting or crocheting combined with making-up including cutting of fabric; or Making-up including cutting of fabric, provided that the value of non-originating materials used does not exceed 50 % of the EXW or 45 % of the FOB of the product."},
	{"6101", "6117", 48, "- Others", "Spinning of natural or man-made staple fibres combined with knitting or with crocheting; Extrusion of man-made filament yarn combined with knitting or with crocheting; Knitting and making-up in one operation; or Making-up including cutting of fabric, provided that the value of non-originating materials used does not exceed 50 % of the EXW or 45 % of the FOB of the product."},
	{"6202", "6202", 49, "- Embroidered", "Weaving combined with making-up including cutting of fabric; Production from unembroidered fabric, provided that the value of non-originating unembroidered fabric used does not exceed 50 % of the EXW or 45 % of the FOB of the product; or Making-up including cutting of fabric, provided that the value of non-originating materials used does not exceed 50 % of the EXW or 45 % of the FOB of the product."},
	{"6202", "6202", 49, "- Others", "Weaving combined with making-up including cutting of fabric; Making-up including cutting of fabric preceded by printing (as standalone operation); or Making-up including cutting of fabric, provided that the value of non-originating materials used does not exceed 50 % of the EXW or 45 % of the FOB of the product."},
	{"6301", "6304", 54, "- Of felt and of nonwovens", "Nonwoven fabric formation combined with making-up including cutting of fabric."},
	{"6301", "6304", 55, "- Others -- Embroidered", "Weaving, knitting or crocheting combined with making-up including cutting of fabric; or Production from unembroidered fabric (other than knitted or crocheted), provided that the value of non-originating unembroidered fabric used does not exceed 40 % of the EXW or 35 % of the FOB of the product."},
	{"6301", "6304", 55, "-- Others", "Weaving, knitting or crocheting combined with making-up including cutting of fabric."},
	{"6306", "6306", 55, "- Of nonwovens", "Nonwoven fabric formation combined with making-up including cutting of fabric."},
	{"6306", "6306", 55, "- Others", "Weaving combined with making-up including cutting of fabric."},
	{"7109", "7109", 60, "- Metals clad with precious metals, semi-manufactured", "Production from metals clad with precious metals, unwrought."},
	{"7109", "7109", 60, "- Others", "CTH"},
	{"7110", "7110", 60, "- Unwrought", "CTH except from headings 71.06, 71.08 and 71.10; Electrolytic, thermal or chemical separation of precious metals of headings 71.06, 71.08 and 71.10; or Fusion or alloying of precious metals of headings 71.06, 71.08 and 71.10 with each other or with base metals or purification."},
	{"7110", "7110", 61, "- Semi-manufactured or in powder form", "Production from unwrought precious metals."},
	{"9506", "9506", 73, "- Golf clubs and parts thereof", "CTH; however non-originating roughly-shaped blocks for making golf club heads may be used."},
	{"9506", "9506", 73, "- Others", ukRuleOther},
}

func ukSchedule(t *testing.T) schedule {
	t.Helper()
	return scheduleFrom(t, mustDefault(t), ukAgreement, ukFrom)
}

func TestUKHeadingBranchesPinnedToPDF(t *testing.T) {
	s := ukSchedule(t)
	for _, b := range ukBranches {
		n := 0
		for _, r := range s.Rows {
			if r.Start == b.start && r.End == b.end && r.Description == b.description {
				n++
				if r.Page != b.page || r.Text != b.text || r.Blocked {
					t.Errorf("%s-%s %q: page %d blocked %v text %q", b.start, b.end, b.description, r.Page, r.Blocked, r.Text)
				}
			}
		}
		if n != 1 {
			t.Errorf("%s-%s %q: %d rows", b.start, b.end, b.description, n)
		}
	}
}

func TestUKHeadingLookupReturnsEveryBranch(t *testing.T) {
	table := mustDefault(t)
	for _, c := range []struct{ hs, start string }{
		{"7006.10", "7006"}, {"7010.90", "7010"}, {"1515.90", "1515"}, {"1901.10", "190110"},
		{"6116.91", "6101"}, {"6202.11", "6202"}, {"6303.12", "6301"}, {"7109.00", "7109"},
		{"7110.11", "7110"}, {"9506.31", "9506"},
	} {
		want := 0
		for _, b := range ukBranches {
			if b.start == c.start {
				want++
			}
		}
		res := lookupReason(t, table, ukAgreement, c.hs, "2025-03-04")
		res.Evidence = ruleEvidence(res.Evidence)
		if res.Reason != ReasonPSR || len(res.Evidence) != want || want < 2 {
			t.Fatalf("%s: %v, %d evidence, want %d branches", c.hs, res.Reason, len(res.Evidence), want)
		}
		for _, b := range ukBranches {
			if b.start != c.start {
				continue
			}
			found := false
			for _, e := range res.Evidence {
				if e.Text == b.description+": "+b.text && strings.HasSuffix(e.URL, fmt.Sprintf("uk3.pdf#page=%d", b.page)) {
					found = true
				}
			}
			if !found {
				t.Errorf("%s: branch %q (page %d) missing from %+v", c.hs, b.description, b.page, res.Evidence)
			}
		}
	}
}

// 7010 sat on page 57 when it was copied; the heading and its branches are on 58.
func TestUK7010IsOnPage58(t *testing.T) {
	for _, r := range ukSchedule(t).Rows {
		if r.Start == "7010" && r.Page != 58 {
			t.Errorf("7010 row on page %d", r.Page)
		}
	}
}

// ukRepeats lists the rows whose text equals the previous row's text. Each is
// a row with its own rule cell in uk3.pdf that repeats the text, except
// 8709-8711 which is the second code of the one footnoted 87.085-87.11 cell.
// (start, end, page) of the later row.
var ukRepeats = map[string]bool{
	"151610-151710-5": true, "1518-1522-5": true, "1801-1805-6": true, "1902-1902-8": true,
	"230910-230910-13": true, "3105-3105-20": true, "3701-3707-24": true, "3825-3825-25": true,
	"4001-4011-27": true, "4501-4504-30": true, "460121-460122-30": true, "4701-4707-31": true,
	"4801-4823-31": true, "4901-4911-31": true, "5006-5006-32": true, "6203-6203-49": true,
	"6205-6205-50": true, "6207-6208-50": true, "6215-6215-53": true, "7011-7011-58": true,
	"7112-7112-61": true, "7201-7206-61": true, "7401-7402-63": true, "8001-8007-64": true,
	"8301-8311-65": true, "8401-8406-65": true, "8709-8711-70": true, "8801-8805-70": true,
	"9301-9307-72": true, "9503-9505-73": true, "9507-9508-74": true,
}

func TestUKNoRowCopiesThePreviousRow(t *testing.T) {
	s := ukSchedule(t)
	seen := map[string]bool{}
	for i, r := range s.Rows {
		if strings.TrimSpace(r.Text) == "" {
			t.Errorf("row %d %s-%s p%d has no text", i, r.Start, r.End, r.Page)
		}
		if i == 0 || r.Text != s.Rows[i-1].Text {
			continue
		}
		key := fmt.Sprintf("%s-%s-%d", r.Start, r.End, r.Page)
		seen[key] = true
		if !ukRepeats[key] {
			t.Errorf("row %s repeats the previous row's text %q", key, r.Text)
		}
	}
	for key := range ukRepeats {
		if !seen[key] {
			t.Errorf("allowlisted repeat %s no longer exists", key)
		}
	}
}

// Every "-" line row belongs to its heading's codes. Chapter 3 is the only
// place where the lines sit directly under a chapter title (the tuna rows).
func TestUKBranchRowsAreKeyedToTheirHeading(t *testing.T) {
	chapterKeyed := 0
	for _, r := range ukSchedule(t).Rows {
		if r.Description == "" {
			continue
		}
		if !strings.HasPrefix(r.Description, "-") {
			t.Errorf("%s-%s: description %q", r.Start, r.End, r.Description)
		}
		if len(r.Start) == 2 {
			chapterKeyed++
			if r.Start != "03" {
				t.Errorf("chapter-keyed branch row %s %q", r.Start, r.Description)
			}
		}
	}
	if chapterKeyed != 2 {
		t.Errorf("%d chapter-keyed branch rows, want the 2 tuna rows", chapterKeyed)
	}
}

// Rows that are one cell in the PDF for several codes still carry the same text
// and the withheld footnote rows are untouched.
func TestUKFootnoteRowsStayWithheld(t *testing.T) {
	table := mustDefault(t)
	for _, hs := range []string{"7007.11", "7008.00", "8407.10", "8701.10", "8708.94"} {
		if r := lookupReason(t, table, ukAgreement, hs, "2025-03-04"); r.Reason != ReasonWithheld {
			t.Errorf("%s: %v", hs, r.Reason)
		}
	}
	s := ukSchedule(t)
	for _, r := range s.Rows {
		if r.Start == "8709" && (r.Blocked || r.Text != "CTH; MaxNOM 50 % (EXW); or RVC 55 % (FOB).") {
			t.Errorf("8709 row %+v", r)
		}
	}
}
