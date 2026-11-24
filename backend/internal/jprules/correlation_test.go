package jprules

import (
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"
)

var correlationDate = time.Date(2026, 11, 23, 0, 0, 0, 0, time.UTC)

func mustDefault(t *testing.T) *Table {
	t.Helper()
	table, err := Default()
	if err != nil {
		t.Fatal(err)
	}
	return table
}

func TestMappedCodes(t *testing.T) {
	table := mustDefault(t)
	cases := []struct {
		name, agreement, hs   string
		parents               []string
		rowText               string
		page                  int
		noteCites             []string
		requested, scheduleEd string
	}{
		{"EU 2404.11", "JP-EU EPA", "2404.11", []string{"240391", "240399"},
			"CTH; MaxNOM 35 % (EXW); or RVC 70 % (FOB).", 12,
			[]string{"correlation Table I", "page 6"}, "HS 2022", "HS 2017"},
		{"CPTPP 2202.99", "CPTPP", "2202.99", []string{"220290"},
			"A change to a single fruit or single vegetable juice of subheading 2202.90", 40,
			[]string{"correlation Table I", "page 9"}, "HS 2017 or later", "HS 2012"},
		{"CPTPP 3824.99", "CPTPP", "3824.99", []string{"382490"},
			"A change to a good of subheading 3824.10 through 3824.90 from any other subheading.", 56,
			[]string{"correlation Table I", "page 15"}, "HS 2017 or later", "HS 2012"},
		{"Malaysia 3824.99", "JP-Malaysia EPA", "382499", []string{"382420", "382471", "382479", "382490"},
			malaysia3824Rule, 28,
			[]string{"correlation Table I", "page 13", "page 20"}, "HS 2017 or later", "HS 2002"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			res, err := table.LookupDetailed(c.agreement, c.hs, correlationDate)
			if err != nil || res.Reason != ReasonMapped {
				t.Fatalf("got %+v, %v", res, err)
			}
			digits := strings.NewReplacer(".", "").Replace(c.hs)
			if res.Digits != digits {
				t.Errorf("original code lost: %q", res.Digits)
			}
			m := res.Mapped
			if m == nil || m.From != digits || !reflect.DeepEqual(m.To, c.parents) ||
				m.RequestedEdition != c.requested || m.ScheduleEdition != c.scheduleEd || !strings.HasPrefix(m.SourceURL, "https://") {
				t.Fatalf("bad mapping detail %+v", m)
			}
			if len(res.Evidence) < 2 {
				t.Fatalf("want note plus row evidence, got %+v", res.Evidence)
			}
			note := res.Evidence[0]
			if note.URL != "" {
				t.Errorf("note must carry no URL: %q", note.URL)
			}
			for _, want := range append([]string{"Unde Labs note (not treaty text)", digits[:4] + "." + digits[4:], "administrative correlation", "not a rule", "remains the declared tariff code", c.scheduleEd}, c.noteCites...) {
				if !strings.Contains(note.Text+" "+note.Citation, want) {
					t.Errorf("note lacks %q:\n%s\n%s", want, note.Text, note.Citation)
				}
			}
			for _, p := range c.parents {
				if !strings.Contains(note.Text, p[:4]+"."+p[4:]) {
					t.Errorf("note does not name parent %s: %s", p, note.Text)
				}
			}
			// the row evidence is exactly what the parent's own lookup returns
			want, err := table.Lookup(c.agreement, c.parents[0], correlationDate)
			if err != nil || !reflect.DeepEqual(res.Evidence[1:], want) {
				t.Fatalf("row evidence changed:\n%+v\n%+v", res.Evidence[1:], want)
			}
			row := res.Evidence[1]
			if !strings.Contains(row.Text, c.rowText) || !strings.HasSuffix(row.URL, "#page="+strconv.Itoa(c.page)) ||
				!strings.Contains(row.Citation, "PDF page "+strconv.Itoa(c.page)) {
				t.Errorf("wrong row: %+v", row)
			}
			// plain Lookup never returns a mapped result
			if ev, err := table.Lookup(c.agreement, c.hs, correlationDate); err != nil || len(ev) != 0 {
				t.Errorf("Lookup returned %v, %v", ev, err)
			}
		})
	}
}

const malaysia3824Rule = "A change to subheading 3809.91 through 3824.90 from any other heading; or No required change in tariff classification to subheading 3809.91 through 3824.90, provided there is a qualifying value content of not less than 40 percent."

func TestMalaysiaSplitRowRepaired(t *testing.T) {
	table := mustDefault(t)
	var rows []row
	for _, s := range table.schedules["JP-Malaysia EPA"] {
		for _, r := range s.Rows {
			if r.Page == 28 && r.End >= "380991" && r.Start <= "382490" && len(r.Start) == 6 {
				rows = append(rows, r)
			}
		}
	}
	if len(rows) != 1 || rows[0].Text != malaysia3824Rule || rows[0].Start != "380991" || rows[0].End != "382490" {
		t.Fatalf("PDF page 28 row must be one complete rule, got %+v", rows)
	}
	ev, err := table.Lookup("JP-Malaysia EPA", "3824.90", correlationDate)
	if err != nil || len(ev) != 1 || ev[0].Text != malaysia3824Rule {
		t.Fatalf("got %v, %v", ev, err)
	}
}

func TestEditionHint(t *testing.T) {
	table := mustDefault(t)
	res, err := table.LookupDetailed("CPTPP", "0307.43", correlationDate)
	if err != nil || res.Reason != ReasonEditionHint {
		t.Fatalf("got %+v, %v", res, err)
	}
	if res.Evidence != nil || res.Mapped != nil || res.Digits != "030743" {
		t.Errorf("hint must carry no evidence: %+v", res)
	}
	for _, want := range []string{"0307.49", "0307.99", "HS 2012", "differ"} {
		if !strings.Contains(res.Hint, want) {
			t.Errorf("hint lacks %q: %s", want, res.Hint)
		}
	}
	for _, bad := range []string{"CC from", "smoked", "regional value"} {
		if strings.Contains(res.Hint, bad) {
			t.Errorf("hint quotes rule text %q", bad)
		}
	}
}

func TestCorrelationLeavesEverythingElseUnchanged(t *testing.T) {
	table := mustDefault(t)
	d := correlationDate
	noMatch := []struct{ agreement, hs string }{
		{"JP-EU EPA", "2404.12"}, {"JP-EU EPA", "2404.19"}, {"JP-EU EPA", "2404.91"},
		{"JP-EU EPA", "2404.92"}, {"JP-EU EPA", "2404.99"},
		{"JP-UK CEPA", "2404.11"}, {"AJCEP", "2404.11"}, {"CPTPP", "2404.11"},
		{"JP-EU EPA", "2202.99"}, {"JP-Thailand EPA", "0307.43"},
		{"JP-Australia EPA", "2202.99"},
	}
	for _, c := range noMatch {
		res, err := table.LookupDetailed(c.agreement, c.hs, d)
		if err != nil || res.Mapped != nil || res.Hint != "" || res.Reason == ReasonMapped || res.Reason == ReasonEditionHint {
			t.Errorf("%+v: %+v, %v", c, res, err)
		}
	}
	// 2404.12 and the others stay plain no_match for JP-EU
	for _, hs := range []string{"2404.12", "2404.19", "2404.91", "2404.99"} {
		if res, _ := table.LookupDetailed("JP-EU EPA", hs, d); res.Reason != ReasonNoMatch {
			t.Errorf("%s: %v", hs, res.Reason)
		}
	}
	// codes that exist keep their own rule
	for _, c := range []struct{ agreement, hs string }{{"JP-EU EPA", "2403.99"}, {"JP-EU EPA", "2403.91"}, {"CPTPP", "2202.90"}, {"CPTPP", "3824.90"}, {"CPTPP", "0307.49"}} {
		res, _ := table.LookupDetailed(c.agreement, c.hs, d)
		if res.Reason != ReasonPSR {
			t.Errorf("%+v: %v", c, res.Reason)
		}
	}
	// before the schedule starts and for malformed input nothing changes
	if res, _ := table.LookupDetailed("JP-EU EPA", "2404.11", time.Date(2019, 1, 31, 0, 0, 0, 0, time.UTC)); res.Reason != ReasonNoDatedSource || res.Mapped != nil {
		t.Errorf("before schedule: %+v", res)
	}
	if res, _ := table.LookupDetailed("JP-EU EPA", "24.04.11x", d); res.Reason != ReasonMalformed {
		t.Errorf("malformed: %+v", res)
	}
	if res, _ := table.LookupDetailed("JP-EU EPA", "2404.11", time.Time{}); res.Reason != ReasonMalformed {
		t.Errorf("zero date: %+v", res)
	}
	// a four-digit or heading level request for an affected heading is not mapped
	if res, _ := table.LookupDetailed("JP-EU EPA", "2404", d); res.Reason == ReasonMapped {
		t.Errorf("heading mapped: %+v", res)
	}
}

// Every shipped entry names a code that is absent from its schedule edition,
// so the correlation never competes with a listed rule.
func TestShippedEntriesAreAbsentFromTheirSchedule(t *testing.T) {
	table := mustDefault(t)
	if len(table.corr.entries) != 25 {
		t.Fatalf("want 25 entries, got %d", len(table.corr.entries))
	}
	for _, e := range table.corr.entries {
		base, err := table.lookup(e.Agreement, e.Code, correlationDate, true)
		if err != nil || base.Reason != ReasonNoMatch {
			t.Errorf("%s %s: base lookup is %v", e.Agreement, e.Code, base.Reason)
		}
	}
}

func synthetic(rows ...row) map[string][]schedule {
	return map[string][]schedule{"X": {{Agreement: "X", Citation: "X Annex (HS 2012)", URL: "https://example.org/x.pdf", From: "2000-01-01", Rows: rows}}}
}

const syntheticCorr = `[{"agreement":"X","code":"111199","requested_edition":"HS 2022","schedule_edition":"HS 2012","parents":["111110","111120"],"mode":"map",
"sources":[{"name":"T","url":"https://example.org/t.pdf","sha256":"` + "0000000000000000000000000000000000000000000000000000000000000000" + `","page":3,"table":"Table I"}]}]`

func syntheticTable(t *testing.T, corr string, rows ...row) *Table {
	t.Helper()
	sch := synthetic(rows...)
	c, err := parseCorrelations([]byte(corr), sch)
	if err != nil {
		t.Fatal(err)
	}
	return &Table{schedules: sch, corr: c}
}

func TestParentsWithDifferentRowsDowngradeToHint(t *testing.T) {
	same := syntheticTable(t, syntheticCorr, row{Start: "111110", End: "111120", Page: 1, Text: "CC"})
	if res, _ := same.LookupDetailed("X", "111199", correlationDate); res.Reason != ReasonMapped {
		t.Fatalf("same rows should map: %+v", res)
	}
	diff := syntheticTable(t, syntheticCorr,
		row{Start: "111110", End: "111110", Page: 1, Text: "CC"},
		row{Start: "111120", End: "111120", Page: 2, Text: "CTH"})
	res, err := diff.LookupDetailed("X", "111199", correlationDate)
	if err != nil || res.Reason != ReasonEditionHint || res.Evidence != nil || res.Mapped != nil ||
		!strings.Contains(res.Hint, "1111.10") || !strings.Contains(res.Hint, "1111.20") {
		t.Fatalf("got %+v, %v", res, err)
	}
	// a parent with no row, or a withheld parent, also downgrades
	missing := syntheticTable(t, syntheticCorr, row{Start: "111110", End: "111110", Page: 1, Text: "CC"})
	if res, _ := missing.LookupDetailed("X", "111199", correlationDate); res.Reason != ReasonEditionHint {
		t.Errorf("missing parent row: %+v", res)
	}
	blocked := syntheticTable(t, syntheticCorr, row{Start: "111110", End: "111120", Page: 1, Text: "CC", Blocked: true})
	if res, _ := blocked.LookupDetailed("X", "111199", correlationDate); res.Reason != ReasonEditionHint {
		t.Errorf("withheld parent: %+v", res)
	}
}

func TestCorrelationNeverOverridesAnExistingOutcome(t *testing.T) {
	// the code exists in the schedule: PSR wins
	psr := syntheticTable(t, syntheticCorr, row{Start: "111199", End: "111199", Page: 1, Text: "OWN"}, row{Start: "1111", End: "1111", Page: 2, Text: "CC"})
	res, _ := psr.LookupDetailed("X", "111199", correlationDate)
	if res.Reason != ReasonPSR || res.Evidence[0].Text != "OWN" {
		t.Errorf("psr overridden: %+v", res)
	}
	// withheld row for the code itself
	wh := syntheticTable(t, syntheticCorr, row{Start: "111199", End: "111199", Page: 1, Text: "W", Blocked: true}, row{Start: "1111", End: "1111", Page: 2, Text: "CC"})
	if res, _ := wh.LookupDetailed("X", "111199", correlationDate); res.Reason != ReasonWithheld {
		t.Errorf("withheld overridden: %+v", res)
	}
	// too broad: a more specific row overlaps a 4-digit request
	tb := syntheticTable(t, syntheticCorr, row{Start: "111110", End: "111110", Page: 1, Text: "CC"})
	if res, _ := tb.LookupDetailed("X", "1111", correlationDate); res.Reason != ReasonTooBroad {
		t.Errorf("too broad overridden: %+v", res)
	}
	// a date outside the schedule is not dated source: no mapping
	if res, _ := psr.LookupDetailed("X", "111199", time.Date(1999, 1, 1, 0, 0, 0, 0, time.UTC)); res.Reason != ReasonNoDatedSource {
		t.Errorf("before schedule: %+v", res)
	}
	// a different agreement with the same code is untouched
	other := syntheticTable(t, syntheticCorr, row{Start: "111110", End: "111120", Page: 1, Text: "CC"})
	other.schedules["Y"] = []schedule{{Agreement: "Y", Citation: "Y", URL: "https://example.org/y.pdf", From: "2000-01-01", Rows: []row{{Start: "1111", End: "1111", Page: 1, Text: "CC"}}}}
	if res, _ := other.LookupDetailed("Y", "111199", correlationDate); res.Reason != ReasonPSR {
		t.Errorf("other agreement: %+v", res)
	}
}

func TestCorrelationLoadValidation(t *testing.T) {
	sch := synthetic(row{Start: "1111", End: "1111", Page: 1, Text: "CC"})
	good := syntheticCorr
	if _, err := parseCorrelations([]byte(good), sch); err != nil {
		t.Fatal(err)
	}
	bad := map[string]string{
		"not json":        `{`,
		"unknown field":   strings.Replace(good, `"mode":"map"`, `"mode":"map","extra":1`, 1),
		"trailing data":   good + `[]`,
		"short code":      strings.Replace(good, `"111199"`, `"11119"`, 1),
		"dotted code":     strings.Replace(good, `"111199"`, `"1111.99"`, 1),
		"letters code":    strings.Replace(good, `"111199"`, `"11119a"`, 1),
		"bad mode":        strings.Replace(good, `"mode":"map"`, `"mode":"auto"`, 1),
		"no parents":      strings.Replace(good, `"parents":["111110","111120"]`, `"parents":[]`, 1),
		"bad parent":      strings.Replace(good, `"111120"`, `"1111"`, 1),
		"dup parent":      strings.Replace(good, `"111120"`, `"111110"`, 1),
		"self parent":     strings.Replace(good, `"111120"`, `"111199"`, 1),
		"unknown agr":     strings.Replace(good, `"agreement":"X"`, `"agreement":"Nope"`, 1),
		"no sources":      strings.Replace(good, `"sources":[`, `"sources":[],"unused":[`, 1),
		"no edition":      strings.Replace(good, `"requested_edition":"HS 2022"`, `"requested_edition":""`, 1),
		"no sched ed":     strings.Replace(good, `"schedule_edition":"HS 2012"`, `"schedule_edition":""`, 1),
		"http url":        strings.Replace(good, `https://example.org/t.pdf`, `http://example.org/t.pdf`, 1),
		"short sha":       strings.Replace(good, strings.Repeat("0", 64), "abc", 1),
		"upper sha":       strings.Replace(good, strings.Repeat("0", 64), strings.Repeat("A", 64), 1),
		"zero page":       strings.Replace(good, `"page":3`, `"page":0`, 1),
		"no table":        strings.Replace(good, `"table":"Table I"`, `"table":""`, 1),
		"no source name":  strings.Replace(good, `"name":"T"`, `"name":""`, 1),
		"hint on map":     strings.Replace(good, `"mode":"map"`, `"mode":"map","hint":"x"`, 1),
		"hint missing":    strings.Replace(good, `"mode":"map"`, `"mode":"hint"`, 1),
		"duplicate entry": "[" + strings.Trim(good, "[]") + "," + strings.Trim(good, "[]") + "]",
	}
	for name, data := range bad {
		if _, err := parseCorrelations([]byte(data), sch); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	// a hint entry with text is valid
	hint := strings.Replace(good, `"mode":"map"`, `"mode":"hint","hint":"Rules differ."`, 1)
	if _, err := parseCorrelations([]byte(hint), sch); err != nil {
		t.Errorf("hint entry: %v", err)
	}
}
