package jprules

import (
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// Rows pinned to the official PDF text after the split-row repair
// (eval/jp/build_psr.py). See docs/japan/psr-split-row-repair.md.

func scheduleFrom(t *testing.T, table *Table, agreement, from string) schedule {
	t.Helper()
	for _, s := range table.schedules[agreement] {
		if s.From == from {
			return s
		}
	}
	t.Fatalf("no %s schedule from %s", agreement, from)
	return schedule{}
}

func rowsAt(s schedule, start, end string, page int) []row {
	var out []row
	for _, r := range s.Rows {
		if r.Start == start && r.End == end && r.Page == page {
			out = append(out, r)
		}
	}
	return out
}

func TestSplitRowsPinnedToPDF(t *testing.T) {
	table := mustDefault(t)
	cases := []struct {
		name, agreement, from, start, end string
		page                              int
		exact, prefix, suffix, notSuffix  string
	}{
		{name: "Malaysia 3901.10-3901.20", agreement: "JP-Malaysia EPA", from: "2006-07-13", start: "390110", end: "390120", page: 28,
			exact: "A change to subheading 3901.10 through 3901.20 from any other chapter; or No required change in tariff classification to subheading 3901.10 through 3901.20, provided there is a qualifying value content of not less than 40 percent."},
		{name: "Malaysia 3809.91-3824.90", agreement: "JP-Malaysia EPA", from: "2006-07-13", start: "380991", end: "382490", page: 28, exact: malaysia3824Rule},
		{name: "Mexico 2921.41-2921.45", agreement: "JP-Mexico EPA", from: "2005-04-01", start: "292141", end: "292145", page: 24,
			prefix: "A change to subheading 2921.41 through 2921.45 from any other heading, except from heading 29.01, 29.02, 29.04, 29.16, 29.17 or 29.26; or "},
		{name: "Philippines 2922.43-2924.24", agreement: "JP-Philippines EPA", from: "2008-12-11", start: "292243", end: "292424", page: 21,
			prefix: "A change to subheading 2922.43 through 2924.24 from any other subheading; or No required change"},
		{name: "Chile 7325.99-7326.90", agreement: "JP-Chile EPA", from: "2007-09-03", start: "732599", end: "732690", page: 55,
			exact: "A change to subheading 7325.99 through 7326.90 from any heading outside that group."},
		{name: "India 0902.30-0902.40", agreement: "JP-India CEPA", from: "2011-08-01", start: "090230", end: "090240", page: 3,
			prefix: "A change to subheading 0902.30 through 0902.40", suffix: "provided that there is a qualifying value content of not less than 50 percent."},
		{name: "Thailand 62.13-62.17", agreement: "JP-Thailand EPA", from: "2023-01-01", start: "6213", end: "6217", page: 28,
			exact: "CC, provided that non-originating materials of headings 50.07, 51.11 through 51.13, 52.08 through 52.12, 53.09 through 53.11, 54.07, 54.08, 55.12 through 55.16 or chapter 60 are used, each of the non-originating materials is woven in either Party or a non-Party which is a member country of the ASEAN."},
		{name: "Thailand 2001.10", agreement: "JP-Thailand EPA", from: "2023-01-01", start: "200110", end: "200110", page: 9,
			prefix: "CC, provided that where non-originating materials of chapter 7 are used", suffix: "a member country of the ASEAN."},
		{name: "Vietnam 5208", agreement: "JP-Vietnam EPA", from: "2009-10-01", start: "5208", end: "5208", page: 33,
			prefix: "CTH outside heading 52.08 through 52.12, provided that,", suffix: "a Member State of the ASEAN.", notSuffix: "non-"},
		{name: "AJCEP 2022 51.11", agreement: "AJCEP", from: "2027-02-15", start: "5111", end: "5111", page: 24,
			prefix: "CTH except from heading 51.11 through 51.13, provided that, where non-originating materials of heading 51.06 through 51.10 are used,"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rows := rowsAt(scheduleFrom(t, table, c.agreement, c.from), c.start, c.end, c.page)
			if len(rows) != 1 {
				t.Fatalf("want one row, got %+v", rows)
			}
			got := rows[0].Text
			if c.exact != "" && got != c.exact {
				t.Errorf("text %q", got)
			}
			if c.prefix != "" && !strings.HasPrefix(got, c.prefix) {
				t.Errorf("prefix: %q", got)
			}
			if c.notSuffix != "" && strings.HasSuffix(got, c.notSuffix) {
				t.Errorf("truncated: %q", got)
			}
			if c.suffix != "" && !strings.HasSuffix(got, c.suffix) {
				t.Errorf("suffix: %q", got)
			}
		})
	}
}

func TestSplitRowsNoPhantoms(t *testing.T) {
	table := mustDefault(t)
	malaysia := scheduleFrom(t, table, "JP-Malaysia EPA", "2006-07-13")
	for _, r := range malaysia.Rows {
		if r.Start == "1108" && r.End == "1108" || r.Start == "390120" && r.End == "390120" || r.Start == "382490" && r.End == "382490" && r.Page == 28 {
			t.Errorf("phantom Malaysia row from a wrapped line: %+v", r)
		}
		if r.Start == "380910" && r.End == "380910" && !strings.HasSuffix(r.Text, "except from heading 11.08 or 35.05.") {
			t.Errorf("Malaysia 3809.10 truncated: %q", r.Text)
		}
	}
	india := scheduleFrom(t, table, "JP-India CEPA", "2011-08-01")
	for _, r := range india.Rows {
		if r.Start == "35" && r.End == "35" && strings.HasPrefix(r.Text, "percent") {
			t.Errorf("phantom India chapter 35 row: %+v", r)
		}
		if r.Start == "7007" && r.End == "7007" && !strings.HasSuffix(r.Text, "not less than 35 percent.") {
			t.Errorf("India 7007 truncated: %q", r.Text)
		}
	}
}

var (
	danglingEnd   = regexp.MustCompile(`(?i)(?:^|[\s,])(?:through|to|from|or|and|than|of|the)$|,$`)
	fragmentStart = regexp.MustCompile(`^(?:[a-z]|percent\b|per cent\b)`)
)

// splitRowAllowlist lists rows verified in the official PDF to end or begin
// the way the structural check forbids. Key: agreement|from|start|end|page.
var splitRowAllowlist = map[string]string{
	// Not a split row: this 6-column page layout puts the product description
	// where the rule cell is expected. A separate, unrepaired extraction defect
	// (see docs/japan/psr-split-row-repair.md, remaining unknowns).
	"JP-Vietnam EPA|2009-10-01|280461|280461|18": "description read as rule",
}

func TestNoSplitRowFragments(t *testing.T) {
	table := mustDefault(t)
	for _, schedules := range table.schedules {
		for _, s := range schedules {
			for _, r := range s.Rows {
				key := strings.Join([]string{s.Agreement, s.From, r.Start, r.End, strconv.Itoa(r.Page)}, "|")
				if _, ok := splitRowAllowlist[key]; ok {
					continue
				}
				text := strings.TrimSpace(r.Text)
				if danglingEnd.MatchString(text) {
					t.Errorf("%s: row ends mid-sentence: %q", key, text)
				}
				if fragmentStart.MatchString(text) {
					t.Errorf("%s: row starts with a continuation fragment: %q", key, text)
				}
			}
		}
	}
}
