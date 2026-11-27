package adcvd

import (
	"strings"
	"testing"
)

func testTable() *Table {
	return NewTable("test-v1", "2026-11-27", []Order{
		{Case: "A-570-900", Type: "AD", Country: "The People's Republic of China", Product: "Diamond Sawblades and Parts Thereof", OrderDate: "2009-11-04", Status: "active",
			HTS: []string{"8202.39.00.00", "8202.39.0040", "6804.21.00.00"}},
		{Case: "C-570-901", Type: "CVD", Country: "The People's Republic of China", Product: "Diamond Sawblades and Parts Thereof", OrderDate: "2009-11-04", Status: "active",
			HTS: []string{"8202.39.00.00"}},
		{Case: "A-580-809", Type: "AD", Country: "Republic of Korea", Product: "Circular Welded Non-Alloy Steel Pipe", Status: "active",
			HTS: []string{"7306.30"}},
		{Case: "A-552-834", Type: "AD", Country: "Socialist Republic of Vietnam", Product: "Paper File Folders", Status: "active",
			HTS: []string{"4820"}},
		{Case: "A-489-844", Type: "AD", Country: "Republic of Türkiye", Product: "Aluminum Foil", Status: "active",
			HTS: []string{"7607.11.30", "7607.11.60"}},
		{Case: "A-100-001", Type: "AD", Country: "Japan", Product: "Thing without codes", Status: "active"},
	})
}

func TestHTSMatchAtListedLevel(t *testing.T) {
	cases := []struct {
		part, listed string
		want         bool
	}{
		{"8202.39.00.00", "8202.39.00.00", true},
		{"8202390000", "8202.39.00.00", true},
		{"8202.39.00", "8202.39.00.00", true}, // part less specific than the listed line: possible
		{"8202.39", "8202.39.00.00", true},    // heading-level part: possible
		{"8202", "8202.39.00.00", true},
		{"8202.39.0070", "8202.39.0040", false}, // another statistical suffix
		{"8202.39.0040", "8202.39.0040", true},
		{"7306.30.5025", "7306.30", true}, // order listed at six digits
		{"7306.40.5025", "7306.30", false},
		{"4820.10", "4820", true},
		{"4821.10", "4820", false},
		{"8202.31", "8202.39.00.00", false},
	}
	for _, c := range cases {
		if got := htsMatch(digits(c.part), digits(c.listed)); got != c.want {
			t.Errorf("htsMatch(%q, %q) = %v, want %v", c.part, c.listed, got, c.want)
		}
	}
}

func TestDigitsRejectsBadCodes(t *testing.T) {
	for _, s := range []string{"", "82", "820", "82023", "8202.39.0", "82023900001", "abcd", "8202.39.00.000"} {
		if d := digits(s); d != "" {
			t.Errorf("digits(%q) = %q, want empty", s, d)
		}
	}
	for s, want := range map[string]string{"8202.39.00.00": "8202390000", " 8202 39 00 ": "82023900", "7306": "7306", "7306.30": "730630"} {
		if got := digits(s); got != want {
			t.Errorf("digits(%q) = %q, want %q", s, got, want)
		}
	}
}

func TestPossibleExposureNeedsHTSAndOrigin(t *testing.T) {
	tb := testTable()
	r := tb.Check(Input{Parts: []Part{
		{Name: "blade", HTS: "8202.39.00.00", Origin: "China"},
		{Name: "blade, other origin", HTS: "8202.39.00.00", Origin: "Germany"},
		{Name: "pipe", HTS: "7306.30.5025", Origin: "South Korea"},
		{Name: "folder", HTS: "4820.30", Origin: "Vietnam"},
		{Name: "foil", HTS: "7607.11.30", Origin: "Turkey"},
		{Name: "no match", HTS: "9999.00", Origin: "China"},
	}})
	want := []string{StatusPossible, StatusNone, StatusPossible, StatusPossible, StatusPossible, StatusNone}
	for i, w := range want {
		if r.Parts[i].Status != w {
			t.Errorf("part %d (%s) status = %s, want %s", i, r.Parts[i].Name, r.Parts[i].Status, w)
		}
	}
	m := r.Parts[0].Matches
	if len(m) != 1 || len(m[0].Cases) != 2 || m[0].Cases[0] != "A-570-900" || m[0].Cases[1] != "C-570-901" {
		t.Fatalf("matches for the blade = %+v, want one entry holding the AD and CVD cases", m)
	}
	if m[0].Product != "Diamond Sawblades and Parts Thereof" || m[0].OrderDate != "2009-11-04" || len(m[0].ListedHTS) != 1 || m[0].ListedHTS[0] != "8202.39.00.00" {
		t.Errorf("match = %+v", m[0])
	}
}

func TestNotCheckedNamesWhatIsMissing(t *testing.T) {
	r := testTable().Check(Input{Parts: []Part{
		{Name: "a", HTS: "", Origin: "China"},
		{Name: "b", HTS: "8202.39", Origin: ""},
		{Name: "c", HTS: "82", Origin: "China"},
	}})
	for i, sub := range []string{"HTS", "origin", "HTS"} {
		p := r.Parts[i]
		if p.Status != StatusNotChecked || !strings.Contains(p.Reason, sub) {
			t.Errorf("part %d = %s %q, want not_checked naming %s", i, p.Status, p.Reason, sub)
		}
	}
}

func TestOrderWithoutListedCodesNeverMatches(t *testing.T) {
	r := testTable().Check(Input{Parts: []Part{{Name: "x", HTS: "8202.39", Origin: "Japan"}}})
	if r.Parts[0].Status != StatusNone {
		t.Fatalf("status = %s", r.Parts[0].Status)
	}
}

func TestOriginNames(t *testing.T) {
	for _, c := range []struct{ part, order string }{
		{"China", "The People's Republic of China"}, {"PRC", "People's Republic of China"}, {"Vietnam", "Socialist Republic of Vietnam"},
		{"South Korea", "Republic of Korea"}, {"Korea", "Republic of Korea"}, {"Turkey", "Republic of Türkiye"}, {"Türkiye", "Republic of Türkiye"},
		{"China/Vietnam", "Socialist Republic of Vietnam"}, {"Laos", "Lao People's Democratic Republic"}, {"Taiwan", "Taiwan"},
		{"Czech Republic", "Czech Republic"}, {"UAE", "United Arab Emirates"}, {"Brazil", "Federative Republic of Brazil"},
	} {
		if !originMatches(c.part, c.order) {
			t.Errorf("originMatches(%q, %q) = false", c.part, c.order)
		}
	}
	if originMatches("India", "The People's Republic of China") || originMatches("", "China") {
		t.Error("origin matched wrongly")
	}
}

func TestResultCarriesTheNoticeAndLimits(t *testing.T) {
	r := testTable().Check(Input{Parts: []Part{{Name: "a", HTS: "8202.39", Origin: "China"}}})
	if r.Snapshot != "2026-11-27" || r.Notice == "" || len(r.Limits) == 0 {
		t.Fatalf("result = %+v", r)
	}
	if !strings.Contains(r.Notice, "written") || !strings.Contains(r.Notice, "scope") || !strings.Contains(r.Notice, "convenience") {
		t.Errorf("notice must say the HTS numbers are for convenience and the written scope governs: %q", r.Notice)
	}
}

func TestEmbeddedTable(t *testing.T) {
	tb, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(tb.Orders()) < 500 {
		t.Fatalf("only %d orders in the embedded file", len(tb.Orders()))
	}
	if tb.Snapshot() == "" || tb.Fetched() == "" {
		t.Error("snapshot and fetch date must be recorded")
	}
	withHTS := 0
	for _, o := range tb.Orders() {
		if !caseRE.MatchString(o.Case) {
			t.Errorf("bad case number %q", o.Case)
		}
		if o.Status != "active" {
			t.Errorf("%s status %q", o.Case, o.Status)
		}
		if strings.TrimSpace(o.Country) == "" || strings.TrimSpace(o.Product) == "" {
			t.Errorf("%s missing country or product", o.Case)
		}
		if len(o.HTS) > 0 {
			withHTS++
			if o.ScopeNotice == nil || o.ScopeNotice.URL == "" || o.ScopeNotice.Date == "" {
				t.Errorf("%s names HTS numbers without their source", o.Case)
			}
		}
		for _, h := range o.HTS {
			if digits(h) == "" {
				t.Errorf("%s listed HTS %q is not a code", o.Case, h)
			}
		}
		if canon := originCanon(o.Country); canon == "" || strings.Contains(strings.ToLower(canon), "republic of") || strings.HasPrefix(canon, "The ") {
			t.Errorf("%s country %q normalises to %q", o.Case, o.Country, canon)
		}
	}
	if withHTS < len(tb.Orders())/2 {
		t.Errorf("only %d of %d orders have HTS numbers", withHTS, len(tb.Orders()))
	}
}
