package jprules

import (
	"crypto/sha256"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// hs2017CodesSHA256 pins data/hs2017_codes.txt. See docs/japan/ajcep-general-validity.md.
const hs2017CodesSHA256 = "516da3067b86077fe0508822e70076ea7a0c6721f7bb853c6fc524030dc4eb73"

func TestHS2017CodeListFormat(t *testing.T) {
	if got := fmt.Sprintf("%x", sha256.Sum256(hs2017CodesTXT)); got != hs2017CodesSHA256 {
		t.Fatalf("sha256 = %s", got)
	}
	lines := strings.Split(strings.TrimSuffix(string(hs2017CodesTXT), "\n"), "\n")
	if len(lines) != 5387 {
		t.Fatalf("%d codes, want 5387", len(lines))
	}
	six := regexp.MustCompile(`^[0-9]{6}$`)
	for i, l := range lines {
		if !six.MatchString(l) {
			t.Fatalf("line %d %q is not six digits", i+1, l)
		}
		if i > 0 && lines[i-1] >= l {
			t.Fatalf("line %d %q not strictly after %q (must be sorted and unique)", i+1, l, lines[i-1])
		}
	}
	if !strings.HasSuffix(string(hs2017CodesTXT), "\n") || strings.Contains(string(hs2017CodesTXT), "\r") {
		t.Fatal("expected LF endings and a final newline")
	}
	if slices.Contains(lines, "999999") {
		t.Fatal("pseudo-code 999999 must not be in the list")
	}
	if len(hs2017Valid().six) != 5387 {
		t.Fatalf("parsed set has %d codes", len(hs2017Valid().six))
	}
}

// Every six-digit start and end of every schedule that cites HS 2017 is a real
// subheading, which shows the list is complete for those schedules.
func TestHS2017ListCoversEverySchedule(t *testing.T) {
	table := mustTable(t)
	v := hs2017Valid()
	checked, schedules := 0, 0
	for agreement, list := range table.schedules {
		for _, s := range list {
			if !strings.Contains(s.Citation, "(HS 2017)") {
				continue
			}
			schedules++
			for _, r := range s.Rows {
				for _, w := range []string{r.Start, r.End} {
					switch len(w) {
					case 6:
						checked++
						if !v.six[w] {
							t.Errorf("%s: %s is not in the HS 2017 list", agreement, w)
						}
					case 4, 2:
						if !v.hasPrefix(w) {
							t.Errorf("%s: prefix %s is not in the HS 2017 list", agreement, w)
						}
					}
				}
			}
		}
	}
	if schedules < 5 || checked < 500 {
		t.Fatalf("self-check saw %d schedules and %d six-digit codes", schedules, checked)
	}
}

func TestGeneralFallbackRequiresValidHS2017Code(t *testing.T) {
	table := mustTable(t)
	cases := []struct {
		name, hs, date string
		want           Reason
	}{
		{"real subheading", "3215.90", "2025-01-01", ReasonGeneral},
		{"real subheading 9 digit", "3215.90-000", "2025-01-01", ReasonGeneral},
		{"real heading", "3215", "2025-01-01", ReasonGeneral},
		{"real chapter 90 subheading", "9018.90", "2025-01-01", ReasonGeneral},
		{"absent subheading in a real heading", "3215.99", "2025-01-01", ReasonNotInEdition},
		// PSR rows (here a chapter-wide row) are matched first, even for a code absent from the list.
		{"PSR chapter row wins over validity", "0101.99", "2025-01-01", ReasonPSR},
		{"absent subheading chapter 90", "9018.99", "2025-01-01", ReasonNotInEdition},
		{"absent heading", "3299", "2025-01-01", ReasonNotInEdition},
		{"absent heading 0900", "0900", "2025-01-01", ReasonNotInEdition},
		// Order: the validity check is before excluded_general.
		{"absent chapter 50 heading is invalid not excluded", "5008", "2025-01-01", ReasonNotInEdition},
		// Reserved chapters stay invalid before and independent of the list.
		{"reserved 77", "7701", "2025-01-01", ReasonInvalidCode},
		// The list is HS 2017 only: no change outside the general window.
		{"HS 2022 window keeps no_match", "3215", "2027-02-15", ReasonNoMatch},
		{"before window", "3215", "2022-12-31", ReasonNoDatedSource},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			res, err := table.LookupDetailed("AJCEP", c.hs, day(t, c.date))
			if err != nil {
				t.Fatal(err)
			}
			if res.Reason != c.want {
				t.Fatalf("%s: reason %q, want %q", c.hs, res.Reason, c.want)
			}
			if res.Reason == ReasonNotInEdition && res.Evidence != nil {
				t.Fatal("invalid code carried evidence")
			}
		})
	}
}

// Ordering ruling: the validity check comes before excluded_general. A code
// that is absent from HS 2017 in chapters 50-63 is invalid_code. A real one
// with no PSR row stays excluded_general; the shipped AJCEP schedule has a row
// for every real textile code, so that branch is shown on a table copy whose
// textile rows are removed.
func TestTextileOrderingValidBeforeExcluded(t *testing.T) {
	base := mustTable(t)
	d := day(t, "2025-01-01")
	cp := *base
	cp.schedules = map[string][]schedule{}
	for a, list := range base.schedules {
		for _, s := range list {
			if a == "AJCEP" {
				rows := []row{}
				for _, r := range s.Rows {
					if len(r.Start) < 2 || r.Start[:2] < "50" || r.Start[:2] > "63" {
						rows = append(rows, r)
					}
				}
				s.Rows = rows
			}
			cp.schedules[a] = append(cp.schedules[a], s)
		}
	}
	v := hs2017Valid()
	for _, hs := range []string{"5001", "5208", "6307", "5201.00", "630790"} {
		if !v.hasPrefix(strings.NewReplacer(".", "").Replace(hs)) {
			t.Fatalf("%s should be real", hs)
		}
		if res, _ := cp.LookupDetailed("AJCEP", hs, d); res.Reason != ReasonExcludedGeneral {
			t.Errorf("real %s: %q, want excluded_general", hs, res.Reason)
		}
	}
	for _, hs := range []string{"5008", "5099", "5099.99", "6399", "6399.00"} {
		if v.hasPrefix(strings.NewReplacer(".", "").Replace(hs)) {
			t.Fatalf("%s should be absent", hs)
		}
		if res, _ := cp.LookupDetailed("AJCEP", hs, d); res.Reason != ReasonNotInEdition {
			t.Errorf("absent %s: %q, want not_in_schedule_edition", hs, res.Reason)
		}
	}
	// The shipped schedule: absent 5008 is invalid_code.
	if res, _ := base.LookupDetailed("AJCEP", "5008", d); res.Reason != ReasonNotInEdition {
		t.Errorf("shipped 5008: %q", res.Reason)
	}
}

// Every four-digit code with no PSR row is general or excluded_general only
// when some HS 2017 subheading starts with it; otherwise invalid_code.
func TestHeadingValidityMatchesList(t *testing.T) {
	table := mustTable(t)
	v := hs2017Valid()
	d := day(t, "2025-01-01")
	for h := 100; h <= 9699; h++ {
		hs := fmt.Sprintf("%04d", h)
		res, err := table.LookupDetailed("AJCEP", hs, d)
		if err != nil {
			t.Fatal(err)
		}
		switch res.Reason {
		case ReasonGeneral, ReasonExcludedGeneral:
			if !v.hasPrefix(hs) {
				t.Fatalf("%s: %q for a heading absent from HS 2017", hs, res.Reason)
			}
		case ReasonNotInEdition:
			if v.hasPrefix(hs) {
				t.Fatalf("%s: not_in_schedule_edition for a real heading", hs)
			}
		}
	}
}

// The correlation step runs on no_match, so a code that has a reviewed
// correlation must reach it before the general validity check turns it into
// invalid_code. No AJCEP correlation ships today, so the test injects one.
func TestNotInEditionIsDistinctFromReserved(t *testing.T) {
	table := mustTable(t)
	d := day(t, "2025-01-01")
	if ReasonNotInEdition != "not_in_schedule_edition" {
		t.Fatalf("reason value %q", ReasonNotInEdition)
	}
	// 3907.29 exists only in a later HS edition than the schedule's.
	for hs, want := range map[string]Reason{"3907.29": ReasonNotInEdition, "390729": ReasonNotInEdition, "7701": ReasonInvalidCode, "9999": ReasonInvalidCode, "0000": ReasonInvalidCode} {
		if res, _ := table.LookupDetailed("AJCEP", hs, d); res.Reason != want {
			t.Errorf("%s: %q, want %q", hs, res.Reason, want)
		}
	}
}

func TestCorrelationBeforeGeneralValidity(t *testing.T) {
	base := mustTable(t)
	d := day(t, "2025-01-01")
	const code = "321599" // not an HS 2017 subheading, no PSR row
	if hs2017Valid().six[code] {
		t.Fatal("test code must be absent from the list")
	}
	src := []correlationSource{{Name: "t", URL: "https://example.org/t.pdf", SHA256: strings.Repeat("a", 64), Page: 1, Table: "t"}}
	with := func(mode, hint string, parents ...string) *Table {
		tt := *base
		tt.corr = &correlationSet{entries: []correlation{{Agreement: "AJCEP", Code: code,
			RequestedEdition: "HS 2022", ScheduleEdition: "HS 2017", Parents: parents, Mode: mode, Sources: src, Hint: hint}}}
		return &tt
	}
	// No correlation: invalid_code.
	if res, _ := base.LookupDetailed("AJCEP", code, d); res.Reason != ReasonNotInEdition {
		t.Fatalf("no correlation: %q", res.Reason)
	}
	// Mapped correlation to parents sharing a PSR row: mapped, not invalid_code.
	res, err := with(modeMap, "", "010121", "010129").LookupDetailed("AJCEP", code, d)
	if err != nil || res.Reason != ReasonMapped || res.Mapped == nil || res.Mapped.From != code || res.Digits != code || len(res.Evidence) < 2 {
		t.Fatalf("mapped: %+v %v", res, err)
	}
	// A hint correlation yields the hint.
	res, _ = with(modeHint, "check 0101.21", "010121").LookupDetailed("AJCEP", code, d)
	if res.Reason != ReasonEditionHint || res.Hint == "" {
		t.Fatalf("hint: %+v", res)
	}
	// A correlation never rescues a code in a reserved chapter.
	tt := *base
	tt.corr = &correlationSet{entries: []correlation{{Agreement: "AJCEP", Code: "770100",
		RequestedEdition: "HS 2022", ScheduleEdition: "HS 2017", Parents: []string{"010121"}, Mode: modeHint, Sources: src, Hint: "x"}}}
	if r, _ := tt.LookupDetailed("AJCEP", "770100", d); r.Reason != ReasonInvalidCode {
		t.Fatalf("reserved with correlation: %q", r.Reason)
	}
	// A correlation for another agreement does not apply.
	tt2 := *base
	tt2.corr = &correlationSet{entries: []correlation{{Agreement: "CPTPP", Code: code,
		RequestedEdition: "HS 2022", ScheduleEdition: "HS 2017", Parents: []string{"010121"}, Mode: modeHint, Sources: src, Hint: "x"}}}
	if r, _ := tt2.LookupDetailed("AJCEP", code, d); r.Reason != ReasonNotInEdition {
		t.Fatalf("other agreement: %q", r.Reason)
	}
	// Plain Lookup never returns it.
	if ev, _ := with(modeMap, "", "010121", "010129").Lookup("AJCEP", code, d); ev != nil {
		t.Fatal("Lookup leaked")
	}
}
