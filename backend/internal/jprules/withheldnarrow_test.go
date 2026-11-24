package jprules

import (
	"fmt"
	"strings"
	"testing"
)

// Withheld rows narrowed to the exact limit the official text proves. See
// docs/japan/withheld-rows-narrowing.md. Row text, codes and pages are pinned
// to the Japan Customs PDFs (eu3.pdf p55, switzerland3.pdf p10, uk3.pdf p57,
// p65, p70).

const (
	euFootnote7007 = "For the products of subheadings 7007.11 and 7007.21, see also Appendix 3-B-1."
	euRule7007     = "CTH; MaxNOM 50 % (EXW); or RVC 55 % (FOB)."
)

func lookupReason(t *testing.T, table *Table, agreement, hs, date string) Result {
	t.Helper()
	r, err := table.LookupDetailed(agreement, hs, day(t, date))
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestEU7007Narrowed(t *testing.T) {
	table := mustDefault(t)
	s := scheduleFrom(t, table, "JP-EU EPA", "2019-02-01")
	for _, c := range []struct {
		start, end string
		blocked    bool
	}{{"700711", "700711", true}, {"700721", "700721", true}, {"7007", "7009", false}} {
		rows := rowsAt(s, c.start, c.end, 55)
		if len(rows) != 1 || rows[0].Blocked != c.blocked || rows[0].Text != euRule7007 {
			t.Errorf("%s-%s: %+v", c.start, c.end, rows)
		}
	}
	for _, hs := range []string{"7007.11", "7007.21", "700711", "8407.10", "8408.20", "8701.10", "8703.21", "8707.10", "8708.94", "8708.10"} {
		if r := lookupReason(t, table, "JP-EU EPA", hs, "2025-03-04"); r.Reason != ReasonWithheld {
			t.Errorf("%s: %v", hs, r.Reason)
		}
	}
	for _, hs := range []string{"7007.19", "7007.29", "7008.00", "7009.10", "7009.92"} {
		r := lookupReason(t, table, "JP-EU EPA", hs, "2025-03-04")
		if r.Reason != ReasonPSR || len(r.Evidence) != 2 {
			t.Fatalf("%s: %v %+v", hs, r.Reason, r.Evidence)
		}
		if r.Evidence[0].Text != euRule7007 || !strings.HasSuffix(r.Evidence[0].Citation, "PDF page 55") {
			t.Errorf("%s rule: %+v", hs, r.Evidence[0])
		}
		if r.Evidence[1].Text != euFootnote7007 || !strings.HasSuffix(r.Evidence[1].Citation, "PDF page 55") ||
			!strings.HasSuffix(r.Evidence[1].URL, "eu3.pdf#page=55") {
			t.Errorf("%s note: %+v", hs, r.Evidence[1])
		}
	}
	// The note stays inside 7007-7009.
	if r := lookupReason(t, table, "JP-EU EPA", "7006.10", "2025-03-04"); r.Reason == ReasonPSR {
		for _, e := range r.Evidence {
			if strings.Contains(e.Text, "Appendix 3-B-1") {
				t.Errorf("note leaked to 7006: %+v", e)
			}
		}
	}
}

func TestSwiss3501To3505Split(t *testing.T) {
	table := mustDefault(t)
	s := scheduleFrom(t, table, "JP-Switzerland EPA", "2009-09-01")
	for _, c := range []struct{ start, end, text string }{
		{"3501", "3501", "CC"},
		{"350211", "350219", "CC except from chapter 4."},
		{"350220", "350499", "CTH"},
		{"3505", "3505", "CC except from chapter 11."},
	} {
		rows := rowsAt(s, c.start, c.end, 10)
		if len(rows) != 1 || rows[0].Blocked || rows[0].Text != c.text {
			t.Errorf("%s-%s: %+v", c.start, c.end, rows)
		}
	}
	if rows := rowsAt(s, "3501", "3505", 10); len(rows) != 0 {
		t.Errorf("merged row remains: %+v", rows)
	}
	for hs, want := range map[string]string{
		"3501.10": "CC", "3501.90": "CC", "3502.11": "CC except from chapter 4.", "3502.19": "CC except from chapter 4.",
		"3502.20": "CTH", "3502.90": "CTH", "3503.00": "CTH", "3504.00": "CTH", "3505.10": "CC except from chapter 11.",
		"3505.20": "CC except from chapter 11.",
	} {
		r := lookupReason(t, table, "JP-Switzerland EPA", hs, "2024-04-16")
		if r.Reason != ReasonPSR || len(r.Evidence) != 1 || r.Evidence[0].Text != want || !strings.HasSuffix(r.Evidence[0].Citation, "PDF page 10") {
			t.Errorf("%s: %v %+v", hs, r.Reason, r.Evidence)
		}
	}
	// Source lines say nothing about 3502.10 or 3506.
	for _, hs := range []string{"3502.10", "3506.10"} {
		if r := lookupReason(t, table, "JP-Switzerland EPA", hs, "2024-04-16"); r.Reason == ReasonWithheld || len(r.Evidence) > 0 && hs == "3502.10" {
			t.Errorf("%s: %v", hs, r.Reason)
		}
	}
}

func TestUKFootnotedRowsWithheld(t *testing.T) {
	table := mustDefault(t)
	s := scheduleFrom(t, table, "JP-UK CEPA", "2021-01-01")
	for _, c := range []struct {
		start, end string
		page       int
		text       string
		blocked    bool
	}{
		{"7007", "7009", 57, euRule7007, true},
		{"8407", "8408", 65, "MaxNOM 50 % (EXW); or RVC 55 % (FOB).", true},
		{"8701", "8701", 70, "MaxNOM 50 % (EXW); or RVC 55 % (FOB).", true},
		{"8702", "8705", 70, "MaxNOM 45 % (EXW); or RVC 60 % (FOB).", true},
		{"8706", "8706", 70, "MaxNOM 50 % (EXW); or RVC 55 % (FOB).", true},
		{"8707", "8707", 70, "MaxNOM 45 % (EXW); or RVC 60 % (FOB).", true},
		{"8708", "8708", 70, euRule7007, true},
		{"8709", "8711", 70, euRule7007, false},
	} {
		rows := rowsAt(s, c.start, c.end, c.page)
		if len(rows) != 1 || rows[0].Blocked != c.blocked || rows[0].Text != c.text {
			t.Errorf("%s-%s: %+v", c.start, c.end, rows)
		}
	}
	for _, hs := range []string{"7007.11", "7007.29", "7008.00", "7009.10", "8407.10", "8408.20", "8701.10", "8703.21", "8704.21", "8705.10", "8706.00", "8707.10", "8708.10", "8708.99"} {
		if r := lookupReason(t, table, "JP-UK CEPA", hs, "2024-01-01"); r.Reason != ReasonWithheld {
			t.Errorf("%s: %v", hs, r.Reason)
		}
	}
	for _, hs := range []string{"8709.11", "8710.00", "8711.20"} {
		r := lookupReason(t, table, "JP-UK CEPA", hs, "2024-01-01")
		if r.Reason != ReasonPSR || len(r.Evidence) != 1 || r.Evidence[0].Text != euRule7007 || !strings.HasSuffix(r.Evidence[0].Citation, "PDF page 70") {
			t.Errorf("%s: %v %+v", hs, r.Reason, r.Evidence)
		}
	}
}

// Every withheld row is deliberate: a new block needs an entry here.
func TestWithheldRowAllowlist(t *testing.T) {
	table := mustDefault(t)
	want := map[string]bool{}
	for _, k := range []string{
		"JP-EU EPA 700711-700711 55", "JP-EU EPA 700721-700721 55", "JP-EU EPA 8407-8408 63",
		"JP-EU EPA 8701-8707 68", "JP-EU EPA 8708-8708 68",
		"JP-UK CEPA 7007-7009 57", "JP-UK CEPA 8407-8408 65", "JP-UK CEPA 8701-8701 70",
		"JP-UK CEPA 8702-8705 70", "JP-UK CEPA 8706-8706 70", "JP-UK CEPA 8707-8707 70", "JP-UK CEPA 8708-8708 70",
		"CPTPP 840733-840734 96", "CPTPP 840820-840820 97", "CPTPP 870110-870130 157", "CPTPP 8702-8705 157",
		"CPTPP 8706-8706 158", "CPTPP 870810-870821 158", "CPTPP 870829-870829 158", "CPTPP 870830-870840 159",
		"CPTPP 870850-870850 159", "CPTPP 870880-870880 160", "CPTPP 870894-870894 160", "CPTPP 870895-870899 161",
		"JP-Switzerland EPA 06-0602 2", "JP-Switzerland EPA 09-090190 3", "JP-Switzerland EPA 11-110820 3",
		"JP-Switzerland EPA 1501-152200 4", "JP-Switzerland EPA 1701-170490 5", "JP-Switzerland EPA 1901-190590 6",
		"JP-Switzerland EPA 21-2106 7", "JP-Switzerland EPA 23-2309 8", "JP-Switzerland EPA 24-240130 8",
		"JP-Switzerland EPA 280461-284450 8", "JP-Switzerland EPA 3801-382460 10", "JP-Switzerland EPA 46-460219 11",
		"JP-Switzerland EPA 50-5006 11", "JP-Switzerland EPA 51-5113 12", "JP-Switzerland EPA 52-5212 12",
		"JP-Switzerland EPA 53-5311 12", "JP-Switzerland EPA 54-5408 12", "JP-Switzerland EPA 59-5911 13",
		"JP-Switzerland EPA 65-6502 15", "JP-Switzerland EPA 701810-701890 15", "JP-Switzerland EPA 721070-722699 17",
		"JP-Switzerland EPA 852351-854390 18", "JP-Switzerland EPA 91-911390 18", "JP-Switzerland EPA 9601-9605 19",
	} {
		want[k] = true
	}
	got := map[string]bool{}
	for agreement, list := range table.schedules {
		for _, s := range list {
			for _, r := range s.Rows {
				if r.Blocked {
					got[fmt.Sprintf("%s %s-%s %d", agreement, r.Start, r.End, r.Page)] = true
				}
			}
		}
	}
	for k := range got {
		if !want[k] {
			t.Errorf("unexpected withheld row %s", k)
		}
	}
	for k := range want {
		if !got[k] {
			t.Errorf("missing withheld row %s", k)
		}
	}
}

// No code of a narrowed range gets two rule rows (a rule row is evidence
// other than the Appendix footnote).
func TestNarrowedRangesNeverReturnTwoRules(t *testing.T) {
	table := mustDefault(t)
	touched := func(agreement, code string) bool {
		h := code[:4]
		switch agreement {
		case "JP-EU EPA", "JP-UK CEPA":
			return h == "7007" || h == "7008" || h == "7009" || h == "8407" || h == "8408" || code[:2] == "87" && h != "8712" && h != "8713"
		}
		return h >= "3501" && h <= "3505"
	}
	for _, agreement := range []string{"JP-EU EPA", "JP-Switzerland EPA", "JP-UK CEPA"} {
		n := 0
		for code := range hs2017Valid().six {
			if !touched(agreement, code) {
				continue
			}
			n++
			r := lookupReason(t, table, agreement, code, "2024-04-16")
			rules := 0
			for _, e := range r.Evidence {
				if !strings.Contains(e.Text, "see also Appendix 3-B-1") {
					rules++
				}
			}
			if rules > 1 {
				t.Errorf("%s %s: %d rules", agreement, code, rules)
			}
		}
		if n < 8 {
			t.Errorf("%s: only %d codes checked", agreement, n)
		}
	}
}
