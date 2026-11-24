package jprules

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/jpcorpus"
)

// The Japan-US Trade Agreement rows are the Japanese text of Annex I Section C
// (customs.go.jp/roo/text/us2.pdf). The expectations below were compared with
// a second, independent text extraction of that PDF.

const (
	jpusAgreement = "JP-US TA"
	jpusDefault   = "前款１(a)から(p)までに分類される産品の各品目別原産地規則は、次の表に別段の定めがある場合を除き、ＣＣとする。"
)

func jpusLookup(t *testing.T, hs, date string) Result {
	t.Helper()
	res, err := mustTable(t).LookupDetailed(jpusAgreement, hs, day(t, date))
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func TestJPUSTableRowsMatchThePDF(t *testing.T) {
	const dairy = "第一九〇一・九〇号の酪農調製品（乳固形分の含有量が乾燥状態において全重量の十パーセントを超えるもの）"
	cases := []struct {
		name, hs string
		page     int
		text     string // exact first evidence text
	}{
		{"dairy range start page 15", "0402.10", 15, "ＣＣ（" + dairy + "からの変更を除く。）"},
		{"dairy heading 4.06 on page 16", "0406.90", 16, "ＣＣ（" + dairy + "からの変更を除く。）"},
		{"CTSH one line", "0801.32", 16, "ＣＴＳＨ"},
		{"two branches", "0904.21", 16, "ＣＣ（第〇七〇九・六〇号の材料からの変更を除く。）（とうがらしに限る。） ＣＣ（その他の産品）"},
		{"US maize proviso", "1108.12", 18, "ＣＣ。ただし、産品がアメリカ合衆国において収穫されるとうもろこしからアメリカ合衆国において生産されることを条件とする。"},
		{"heading range 15.18-15.22", "1520.00", 18, "ＣＴＨ"},
		{"value test without change of class", "2009.90", 24, "非原産材料の価額が産品の価額の五十五パーセントを超えないこと（関税分類の変更を必要としない。）。"},
		{"CTSH range 2905.43-45", "2905.44", 26, "ＣＴＳＨ"},
		{"CTSH range end", "3823.70", 27, "ＣＴＳＨ"},
		{"CTH heading 35.04", "3504.00", 27, "ＣＴＨ"},
		{"cocoa confectionery branches", "1806.90", 19, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			res := jpusLookup(t, c.hs, "2026-11-23")
			if res.Reason != ReasonPSR || len(res.Evidence) == 0 {
				t.Fatalf("%s: %+v", c.hs, res)
			}
			e := res.Evidence[0]
			if c.text != "" && e.Text != c.text {
				t.Errorf("text = %q", e.Text)
			}
			wantPage := "#page=" + itoa(c.page)
			if !strings.HasSuffix(e.URL, wantPage) || !strings.HasPrefix(e.URL, "https://www.customs.go.jp/roo/text/us2.pdf") ||
				!strings.HasSuffix(e.Citation, "PDF page "+itoa(c.page)) {
				t.Errorf("citation/url = %q %q", e.Citation, e.URL)
			}
		})
	}
}

func itoa(n int) string {
	b, _ := json.Marshal(n)
	return string(b)
}

func TestJPUSPageSpanningRowsKeepEveryBranch(t *testing.T) {
	cases := []struct {
		hs       string
		page     int // page the row starts on; it ends on the next page
		branches []string
		last     string // text from the second page
	}{
		{"1901.20", 19, []string{
			"ＣＣ（第〇四・〇一項から第〇四・〇六項までの各項の材料からの変更を除く。）（バター脂の含有量が乾燥状態において全重量の二十五パーセントを超える産品であって小売用でないものに限る。）",
			"ＣＣ。ただし、非原産材料である第一一〇二・九〇号の米粉の価額が産品の価額の三十パーセントを超えないことを条件とする。（米粉の含有量が乾燥状態において全重量の三十パーセントを超える産品に限る。）",
			"ＣＣ（その他の産品）",
			"注二以上の品目別原産地規則が適用可能である場合には、産品は、それぞれの適用可能な品目別原産地規則の要件を満たさなければならない。",
		}, "満たさなければならない。"},
		{"2005.99", 21, []string{
			"ＣＣ（第〇七・〇一項、第〇七〇九・五一号若しくは第〇七〇九・六〇号の材料又は第〇七・一〇項から第〇七・一二項までの各項のばれいしょ若しくはきのこ（はらたけ属のもの）からの変更を除く。）（野菜の調製品（二以上の野菜から得たものを除く。）に限る。）",
			"ＣＣ。ただし、第〇七・〇一項、第〇七〇九・五一号及び第〇七〇九・六〇号の非原産材料並びに非原産材料である第〇七・一〇項から第〇七・一二項までの各項のばれいしょ及びきのこ（はらたけ属のもの）の価額が産品の価額の四十パーセントを超えないことを条件とする。（その他の産品）",
		}, "（その他の産品）"},
		{"2008.19", 22, []string{
			"ＣＣ（第〇八・〇二項又は第一二・〇二項の材料からの変更を除く。）（単に煎っただけのナット又は落花生（乾いたもの又は油漬けしたものであって、塩を加えたかどうかを問わない。）に限る。）",
			"ＣＣ（第〇八・〇二項又は第一二・〇二項の材料からの変更を除く。）（単に煎っただけのナット又は落花生（乾いたもの又は油漬けしたものであって、塩を加えたかどうかを問わない。）の含有量が乾燥状態において全重量の五十パーセント以上である混合物に限る。）",
			"ＣＣ（その他の産品）",
		}, "ＣＣ（その他の産品）"},
	}
	for _, c := range cases {
		t.Run(c.hs, func(t *testing.T) {
			res := jpusLookup(t, c.hs, "2026-11-23")
			if res.Reason != ReasonPSR || len(res.Evidence) == 0 {
				t.Fatalf("%+v", res)
			}
			e := res.Evidence[0]
			if want := strings.Join(c.branches, " "); e.Text != want {
				t.Errorf("text = %q\nwant   %q", e.Text, want)
			}
			if !strings.HasSuffix(e.Text, c.last) {
				t.Errorf("last branch missing: %q", e.Text)
			}
			if !strings.HasSuffix(e.URL, "#page="+itoa(c.page)) {
				t.Errorf("row starts on page %d: %q", c.page, e.URL)
			}
			// One table row only: the second page adds no extra row.
			rows := 0
			for _, ev := range res.Evidence {
				if strings.HasPrefix(ev.Text, c.branches[0][:12]) {
					rows++
				}
			}
			if rows != 1 {
				t.Errorf("%d rows", rows)
			}
		})
	}
}

func TestJPUSDefaultRowOnlyWhereTheTextMakesCCTheDefault(t *testing.T) {
	// Goods named in Subsection 2 note 1(a)-(p) without a table row get the
	// default sentence of the head of Subsection 3, quoted exactly.
	inside := []string{
		"0101.21", "0201.10", "0407.11", "0701.10", "0801.11", "1001.99", "1507.10",
		"1601.00", "1602.10", "1603.00", "1701.12", "2202.10", "2204.10",
		"2301.10",
	}
	for _, hs := range inside {
		res := jpusLookup(t, hs, "2026-11-23")
		if res.Reason != ReasonPSR {
			t.Errorf("%s: reason %s", hs, res.Reason)
			continue
		}
		first := res.Evidence[0]
		if !strings.Contains(first.Text, jpusDefault) || !strings.HasSuffix(first.URL, "#page=15") {
			t.Errorf("%s: %+v", hs, first)
		}
	}
}

func TestJPUSNothingOutsideTheListedGoods(t *testing.T) {
	// Codes just outside each range of note 1(a)-(p), all real HS 2017
	// subheadings. The embedded rules cover only the listed goods.
	outside := []string{
		"0301.11", "0302.11", // chapter 3 sits between (a) and (b)
		"1604.11", "1605.10", // 16.04, 16.05 after (c)
		"2201.10", "2203.00", "2205.10", // around (e) and (f)
		"2207.20", "2208.20", // 22.07 other than the listed national item, 22.08
		"2401.10",            // chapter 24
		"2905.41", "2905.49", // either side of 2905.43-45
		"3302.10", "3503.00", "3506.10", // beside 33.01 and 35.01-02, 35.04-05
		"3809.91", "3824.10", "3822.00", // beside 3809.10 and 3823.11-70
		"2501.00", "6404.11", "8708.94", "9999.00",
	}
	for _, hs := range outside {
		res := jpusLookup(t, hs, "2026-11-23")
		if res.Reason == ReasonPSR || len(res.Evidence) > 0 {
			t.Errorf("%s: %+v", hs, res)
		}
		if hs != "9999.00" && res.Reason != ReasonNoMatch {
			t.Errorf("%s reason = %s", hs, res.Reason)
		}
		old, err := mustTable(t).Lookup(jpusAgreement, hs, day(t, "2026-11-23"))
		if err != nil || len(old) != 0 {
			t.Errorf("Lookup(%s) = %v", hs, old)
		}
	}
	// No ordinary default applies to anything else: every heading of 2300-9999
	// but the listed ones stays without evidence.
	table := mustTable(t)
	listed := map[string]bool{"23": true, "2905": true, "3301": true, "3501": true, "3502": true, "3504": true, "3505": true, "3809": true, "3823": true}
	for h := 2400; h <= 9999; h++ {
		code := itoa4(h)
		if listed[code] || listed[code[:2]] {
			continue
		}
		if ev, _ := table.Lookup(jpusAgreement, code, day(t, "2026-11-23")); len(ev) != 0 {
			t.Fatalf("%s returned evidence", code)
		}
	}
}

func itoa4(n int) string {
	s := itoa(n)
	return strings.Repeat("0", 4-len(s)) + s
}

func TestJPUSNationalItemsAreTooBroadNotAWholeHeadingRule(t *testing.T) {
	// 2206.00-100 and 2207.10-199 are nine digit national items. Lookup keeps
	// six digits, so the code can neither get a rule for the whole subheading
	// nor a "no rule" answer: it is too broad.
	for _, hs := range []string{"2206.00", "2206.00-100", "2206", "2207.10", "2207.10-199", "2207.10-100", "2207"} {
		res := jpusLookup(t, hs, "2026-11-23")
		if res.Reason != ReasonTooBroad || len(res.Evidence) != 0 {
			t.Errorf("%s: %+v", hs, res)
		}
	}
	// The sister subheadings of the same headings are not listed goods.
	for _, hs := range []string{"2207.20", "2205.90"} {
		if res := jpusLookup(t, hs, "2026-11-23"); res.Reason != ReasonNoMatch {
			t.Errorf("%s: %+v", hs, res)
		}
	}
	// 2209 has a table row for the whole heading; 2202 has table rows below it.
	if res := jpusLookup(t, "2209.00", "2026-11-23"); res.Reason != ReasonPSR {
		t.Errorf("2209.00: %+v", res)
	}
	if res := jpusLookup(t, "2202", "2026-11-23"); res.Reason != ReasonTooBroad {
		t.Errorf("2202: %+v", res)
	}
}

func TestJPUSDatesAndAgreementName(t *testing.T) {
	if res := jpusLookup(t, "0406.90", "2019-12-31"); res.Reason != ReasonNoDatedSource || len(res.Evidence) != 0 {
		t.Errorf("before entry into force: %+v", res)
	}
	for _, d := range []string{"2020-01-01", "2026-11-23", "2035-01-01"} {
		if res := jpusLookup(t, "0406.90", d); res.Reason != ReasonPSR {
			t.Errorf("%s: %+v", d, res)
		}
	}
	if _, ok := jpcorpus.AgreementNames[jpusAgreement]; !ok {
		t.Fatalf("%q is not a jpcorpus agreement code", jpusAgreement)
	}
	table := mustTable(t)
	if !table.HasAgreement(jpusAgreement) {
		t.Fatal("agreement not embedded")
	}
	// Every embedded agreement string is a corpus code, and the JP-US entry is
	// the only one under its name.
	for name := range table.schedules {
		if _, ok := jpcorpus.AgreementNames[name]; !ok && name != "RCEP" {
			t.Errorf("schedule agreement %q is not a corpus code", name)
		}
	}
	if n := len(table.schedules[jpusAgreement]); n != 1 {
		t.Errorf("%d schedules", n)
	}
}

func TestJPUSCitationIsHonestAboutTheEdition(t *testing.T) {
	s := mustTable(t).schedules[jpusAgreement][0]
	if strings.Contains(s.Citation, "(HS 2017)") || strings.Contains(s.Citation, "(HS 2022)") {
		t.Errorf("the agreement names no HS edition: %q", s.Citation)
	}
	for _, want := range []string{"Annex I", "Section C", "Subsection 3", "Japanese text", "edition not stated"} {
		if !strings.Contains(s.Citation, want) {
			t.Errorf("citation %q lacks %q", s.Citation, want)
		}
	}
	if s.URL != "https://www.customs.go.jp/roo/text/us2.pdf" || s.From != "2020-01-01" || s.Until != "" {
		t.Errorf("source or window: %+v", s)
	}
	var raw []map[string]any
	if err := json.Unmarshal(schedulesJSON, &raw); err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, r := range raw {
		if r["agreement"] == jpusAgreement {
			found = true
			if r["sha256"] != "af460808179a30ce688197ac064b170f64f7d55f943bc898860771eabf9962d6" {
				t.Errorf("sha256 = %v", r["sha256"])
			}
		}
	}
	if !found {
		t.Fatal("no entry")
	}
}

func TestJPUSNotesFollowTheirScope(t *testing.T) {
	const (
		gen1   = "１この節の規定の適用上、"
		gen2   = "２(a)日本国は、この節に別段の定めがある場合を除くほか、"
		gen3   = "３(a)日本国は、(c)に規定する場合を除くほか、"
		interp = "第二款品目別原産地規則の解釈のための一般的注釈"
		dflt   = "第三款品目別原産地規則前款１(a)から(p)までに分類される産品の各品目別原産地規則は、"
		plants = "４第六類から第一四類までの各類の規定の適用上、"
		cocoa  = "５第一八・〇六項の規定の適用上、"
		caveat = "Unde Labs note (not treaty text)"
	)
	has := func(ev []Evidence, frag string) *Evidence {
		for i := range ev {
			if strings.Contains(ev[i].Text, frag) {
				return &ev[i]
			}
		}
		return nil
	}
	cases := []struct {
		hs      string
		date    string
		with    []string
		without []string
	}{
		{"0406.90", "2026-11-23", []string{gen1, gen2, gen3, interp, dflt, caveat}, []string{plants, cocoa}},
		{"0709.99", "2026-11-23", []string{gen1, gen2, gen3, interp, dflt, plants, caveat}, []string{cocoa}},
		{"1507.10", "2026-11-23", []string{gen1, gen2, gen3, interp, dflt, caveat}, []string{plants, cocoa}},
		{"1806.20", "2026-11-23", []string{gen1, gen2, gen3, interp, dflt, cocoa, caveat}, []string{plants}},
		{"1704.10", "2026-11-23", []string{gen1, gen2, gen3, interp, dflt, caveat}, []string{plants, cocoa}},
		{"2905.44", "2026-11-23", []string{gen1, gen2, gen3, interp, dflt, caveat}, []string{plants, cocoa}},
		{"3823.11", "2026-11-23", []string{gen1, gen2, gen3, interp, dflt, caveat}, []string{plants, cocoa}},
	}
	for _, c := range cases {
		t.Run(c.hs, func(t *testing.T) {
			res := jpusLookup(t, c.hs, c.date)
			if res.Reason != ReasonPSR {
				t.Fatalf("%+v", res)
			}
			for _, f := range c.with {
				if has(res.Evidence, f) == nil {
					t.Errorf("missing note %q", f)
				}
			}
			for _, f := range c.without {
				if has(res.Evidence, f) != nil {
					t.Errorf("note %q applies outside its scope", f)
				}
			}
			// Each note once, row first, caveat last.
			seen := map[string]bool{}
			for _, e := range res.Evidence {
				if seen[e.Text] {
					t.Errorf("duplicate evidence %.30q", e.Text)
				}
				seen[e.Text] = true
			}
			last := res.Evidence[len(res.Evidence)-1]
			if !strings.HasPrefix(last.Text, caveat) || last.URL != "" || last.Citation != caveat {
				t.Errorf("last evidence = %+v", last)
			}
			for _, e := range res.Evidence[:len(res.Evidence)-1] {
				if e.URL == "" || !strings.Contains(e.URL, "us2.pdf#page=") {
					t.Errorf("treaty evidence without a link: %+v", e)
				}
			}
		})
	}
	// Notes only ever accompany a rule row: none for no_match, too_broad, early dates.
	for _, hs := range []string{"0301.11", "2206.00", "6404.11"} {
		if res := jpusLookup(t, hs, "2026-11-23"); len(res.Evidence) != 0 {
			t.Errorf("%s: %+v", hs, res)
		}
	}
	if res := jpusLookup(t, "0406.90", "2019-12-31"); len(res.Evidence) != 0 {
		t.Errorf("before 2020: %+v", res)
	}
}

func TestJPUSCaveatNamesTheOmissions(t *testing.T) {
	res := jpusLookup(t, "0406.90", "2026-11-23")
	c := res.Evidence[len(res.Evidence)-1].Text
	for _, want := range []string{"partial", "Paragraphs 4 to 10", "Annex II", "Subsection 3", "names no HS edition", "not a claim"} {
		if !strings.Contains(c, want) {
			t.Errorf("caveat lacks %q:\n%s", want, c)
		}
	}
	own, err := json.Marshal(mustTable(t).schedules[jpusAgreement])
	if err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"/Users/", ".claude", "controller", "dossier", "batch", "handoff"} {
		if strings.Contains(string(own), bad) {
			t.Errorf("public data contains %q", bad)
		}
	}
}

func TestJPUSRowShapeAndCounts(t *testing.T) {
	s := mustTable(t).schedules[jpusAgreement][0]
	explicit, def := 0, 0
	for _, r := range s.Rows {
		if r.Description != "" {
			def++
		} else {
			explicit++
		}
		if r.Blocked {
			t.Errorf("row %+v withheld", r)
		}
	}
	if explicit != 92 || def != 10 || len(s.Rows) != 102 {
		t.Errorf("rows: %d explicit, %d default, %d total", explicit, def, len(s.Rows))
	}
	// The default rows are exactly the goods of note 1(a)-(p), item level
	// entries as nine digit national items.
	want := map[string]bool{
		"01-02": true, "04-05": true, "06-14": true, "15-15": true, "1601-1603": true,
		"17-21": true, "2202-2202": true, "2204-2204": true, "23-23": true,
		"220710199-220710199": true,
	}
	// Goods of note 1 whose every HS 2017 code has a table row (2209, 2905.43-45,
	// 3301, 3501-3502, 3504-3505, 3809.10, 3823.11-70) get no default row: a
	// default row must never sit beside a table row for the same code.
	got := map[string]bool{}
	for _, r := range s.Rows {
		if r.Description != "" {
			if r.Text != jpusDefault || r.Page != 15 {
				t.Errorf("default row %+v", r)
			}
			got[r.Start+"-"+r.End] = true
		}
	}
	for k := range want {
		if !got[k] {
			t.Errorf("default range %s missing", k)
		}
	}
	for k := range got {
		if !want[k] {
			t.Errorf("default range %s over-extends the note 1 list", k)
		}
	}
	if len(s.Notes) < 27 {
		t.Errorf("%d notes", len(s.Notes))
	}
}

func TestLoaderRejectsMalformedSchedules(t *testing.T) {
	good := schedule{
		Agreement: "X", Citation: "c", URL: "https://example.org/x.pdf", From: "2020-01-01",
		Rows:  []row{{Start: "0401", End: "0404", Page: 1, Text: "CC"}},
		Notes: []note{{Scope: "section", Key: "I", Text: "t", Page: 1, Applies: [][2]string{{"01", "02"}}}},
	}
	if err := validateSchedule(good); err != nil {
		t.Fatalf("good schedule rejected: %v", err)
	}
	mut := func(f func(s *schedule)) schedule {
		s := good
		s.Rows = append([]row(nil), good.Rows...)
		s.Notes = append([]note(nil), good.Notes...)
		f(&s)
		return s
	}
	bad := map[string]schedule{
		"no agreement":       mut(func(s *schedule) { s.Agreement = "" }),
		"bad from":           mut(func(s *schedule) { s.From = "2020-1-1" }),
		"until before from":  mut(func(s *schedule) { s.Until = "2019-01-01" }),
		"row start letters":  mut(func(s *schedule) { s.Rows[0].Start = "04AB" }),
		"row unequal widths": mut(func(s *schedule) { s.Rows[0].End = "040405" }),
		"row reversed":       mut(func(s *schedule) { s.Rows[0].Start, s.Rows[0].End = "0404", "0401" }),
		"row odd width":      mut(func(s *schedule) { s.Rows[0].Start, s.Rows[0].End = "040", "040" }),
		"row too wide":       mut(func(s *schedule) { s.Rows[0].Start, s.Rows[0].End = "0401000000", "0401000000" }),
		"row no page":        mut(func(s *schedule) { s.Rows[0].Page = 0 }),
		"row no text":        mut(func(s *schedule) { s.Rows[0].Text = " " }),
		"note bad scope":     mut(func(s *schedule) { s.Notes[0].Scope = "heading" }),
		"note unknown key":   mut(func(s *schedule) { s.Notes[0].Key = "ZZ" }),
		"note bad applies":   mut(func(s *schedule) { s.Notes[0].Applies = [][2]string{{"02", "01"}} }),
		"note no text":       mut(func(s *schedule) { s.Notes[0].Text = "" }),
		"note page 0 treaty": mut(func(s *schedule) { s.Notes[0].Page = 0 }),
		"note negative page": mut(func(s *schedule) { s.Notes[0].Page = -1 }),
	}
	for name, s := range bad {
		if err := validateSchedule(s); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	// A withheld row may span unequal widths; a listed row may not.
	blocked := mut(func(s *schedule) { s.Rows[0].Start, s.Rows[0].End, s.Rows[0].Blocked = "06", "0602", true })
	if err := validateSchedule(blocked); err != nil {
		t.Errorf("withheld mixed-width row rejected: %v", err)
	}
	// A page-less note is allowed only as an explicit Unde Labs caveat.
	caveat := mut(func(s *schedule) {
		s.Notes[0].Page = 0
		s.Notes[0].Text = "Unde Labs note (not treaty text): partial."
	})
	if err := validateSchedule(caveat); err != nil {
		t.Errorf("caveat rejected: %v", err)
	}
	// Every embedded schedule passes, so the strict check protects the data.
	for name, list := range mustTable(t).schedules {
		for _, s := range list {
			if err := validateSchedule(s); err != nil {
				t.Errorf("%s: %v", name, err)
			}
		}
	}
}

func hs2017All(t *testing.T) []string {
	t.Helper()
	out := strings.Fields(string(hs2017CodesTXT))
	if len(out) != 5387 {
		t.Fatalf("%d codes", len(out))
	}
	return out
}

// ruleRows counts the table or default rows in a result, notes excluded: they
// are the evidence whose text is not a note of the agreement.
func ruleRows(res Result, notes []note) int {
	n := 0
	for _, e := range res.Evidence {
		isNote := false
		for _, nt := range notes {
			if e.Text == nt.Text {
				isNote = true
				break
			}
		}
		if !isNote {
			n++
		}
	}
	return n
}

func TestJPUSEveryHS2017CodeGetsExactlyOneRuleRow(t *testing.T) {
	table := mustTable(t)
	notes := table.schedules[jpusAgreement][0].Notes
	psr, none := 0, 0
	for _, code := range hs2017All(t) {
		res := jpusLookup(t, code, "2026-11-23")
		switch res.Reason {
		case ReasonPSR:
			psr++
			if n := ruleRows(res, notes); n != 1 {
				t.Errorf("%s: %d rule rows: %+v", code, n, res.Evidence)
			}
		case ReasonNoMatch, ReasonTooBroad:
			none++
		default:
			t.Errorf("%s: reason %s", code, res.Reason)
		}
	}
	if psr == 0 || none == 0 {
		t.Fatalf("psr %d, none %d", psr, none)
	}
}

func TestJPUSNoCodeOutsideHS2017ReturnsARule(t *testing.T) {
	table := mustTable(t)
	valid := map[string]bool{}
	for _, c := range hs2017All(t) {
		valid[c] = true
	}
	mapped := map[string]bool{}
	for _, e := range table.corr.entries {
		if e.Agreement == jpusAgreement {
			mapped[e.Code] = true
		}
	}
	for h := 100; h <= 999999; h++ {
		code := itoa(h)
		code = strings.Repeat("0", 6-len(code)) + code
		if valid[code] {
			continue
		}
		res, err := table.LookupDetailed(jpusAgreement, code, day(t, "2026-11-23"))
		if err != nil {
			t.Fatal(err)
		}
		switch {
		case mapped[code]: // a reviewed correlation, tested separately
		case res.Reason == ReasonPSR || len(res.Evidence) > 0:
			t.Fatalf("%s is not an HS 2017 code but got %+v", code, res)
		case code[:2] != "00" && code[:2] != "77" && code[:2] != "98" && code[:2] != "99" && res.Reason != ReasonNotInEdition && res.Reason != ReasonTooBroad:
			t.Fatalf("%s: reason %s", code, res.Reason)
		}
	}
	for _, hs := range []string{"0100.99", "1999.99", "0399.99"} {
		if res := jpusLookup(t, hs, "2026-11-23"); res.Reason != ReasonNotInEdition {
			t.Errorf("%s: %+v", hs, res)
		}
	}
}

func TestJPUSHS2022OnlyCodesAreMappedToTheirParents(t *testing.T) {
	// WCO HS 2022 to HS 2017 correlation Table I, pages 2 and 3.
	mapped := map[string]string{
		"0410.10": "041000", "0410.90": "041000",
		"0709.52": "070959", "0709.53": "070959", "0709.54": "070959", "0709.55": "070959", "0709.56": "070959",
		"0712.34": "071239", "0802.91": "080290", "0802.92": "080290", "0802.99": "080290",
		"1211.60": "121190", "1509.20": "150910", "1509.30": "150910", "1509.40": "150910",
		"1510.10": "151000", "1510.90": "151000", "1515.60": "151590", "1516.30": "151620",
	}
	for hs, parent := range mapped {
		res := jpusLookup(t, hs, "2026-11-23")
		want := jpusLookup(t, parent, "2026-11-23")
		if res.Reason != ReasonMapped || res.Mapped == nil || !slices.Contains(res.Mapped.To, parent) || len(res.Evidence) == 0 || want.Reason != ReasonPSR {
			t.Errorf("%s: %+v", hs, res)
			continue
		}
		// the evidence is the parent's rule behind one mapping note, requested code kept
		if res.Digits != strings.ReplaceAll(hs, ".", "") || len(res.Evidence) != len(want.Evidence)+1 {
			t.Errorf("%s: digits %s, %d evidence vs %d", hs, res.Digits, len(res.Evidence), len(want.Evidence))
		}
	}
	// 0403.20 descends from 0403.10 (table row for 04.01-04.04) and part of
	// 1901.90 (a different row): no single rule can be shown.
	res := jpusLookup(t, "0403.20", "2026-11-23")
	if res.Reason != ReasonEditionHint || len(res.Evidence) != 0 || !strings.Contains(res.Hint, "0403.10") || !strings.Contains(res.Hint, "1901.90") {
		t.Errorf("0403.20: %+v", res)
	}
}
