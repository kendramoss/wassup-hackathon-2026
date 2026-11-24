package jprules

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// The GSP rows are the Japanese text of the Appendix Table to Article 9 of the
// Ministerial Ordinance for Enforcement of the Temporary Tariff Measures Law
// (e-Gov 344M50000040039), revision in force from 2023-04-01. The expectations
// below were taken from the revision XML by an extraction separate from the one
// that built the data (see docs/japan/gsp-pilot.md).

const (
	gspAgreement = "GSP"
	gspDate      = "2026-11-23"
	gspLawURL    = "https://laws.e-gov.go.jp/law/344M50000040039/20230401_505M60000040029"
	// hs2022CodesSHA256 pins data/hs2022_codes.txt. See docs/japan/gsp-pilot.md.
	hs2022CodesSHA256 = "09b1a9a72ee8ffe52a15c173c233201d0d1db18af7f9c30684c5ac4c4379597c"
)

func gspLookup(t *testing.T, hs, date string) Result {
	t.Helper()
	res, err := mustTable(t).LookupDetailed(gspAgreement, hs, day(t, date))
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func gspSchedule(t *testing.T) schedule {
	t.Helper()
	list := mustTable(t).schedules[gspAgreement]
	if len(list) != 1 {
		t.Fatalf("%d GSP schedules, want only the current window", len(list))
	}
	return list[0]
}

func hs2022All(t *testing.T) []string {
	t.Helper()
	out := strings.Fields(string(hs2022CodesTXT))
	if len(out) != 5612 {
		t.Fatalf("%d codes", len(out))
	}
	return out
}

func TestHS2022CodeListFormat(t *testing.T) {
	if got := fmt.Sprintf("%x", sha256.Sum256(hs2022CodesTXT)); got != hs2022CodesSHA256 {
		t.Fatalf("sha256 = %s", got)
	}
	lines := strings.Split(strings.TrimSuffix(string(hs2022CodesTXT), "\n"), "\n")
	if len(lines) != 5612 {
		t.Fatalf("%d codes, want 5612", len(lines))
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
	if !strings.HasSuffix(string(hs2022CodesTXT), "\n") || strings.Contains(string(hs2022CodesTXT), "\r") {
		t.Fatal("expected LF endings and a final newline")
	}
	if slices.Contains(lines, "999999") {
		t.Fatal("pseudo-code 999999 must not be in the list")
	}
	if len(hs2022Valid().six) != 5612 {
		t.Fatalf("parsed set has %d codes", len(hs2022Valid().six))
	}
}

// Known edition differences prove the two lists are different editions: HS
// 2022 added 04.10 (edible insects) and 0709.52, and dropped 0410.00.
func TestHS2022ListIsADifferentEditionFromHS2017(t *testing.T) {
	v17, v22 := hs2017Valid(), hs2022Valid()
	for _, c := range []string{"041010", "041090", "070952"} {
		if v17.six[c] || !v22.six[c] {
			t.Errorf("%s: hs2017=%v hs2022=%v", c, v17.six[c], v22.six[c])
		}
	}
	if !v17.six["041000"] || v22.six["041000"] {
		t.Errorf("041000 should be HS 2017 only")
	}
	if !v17.six["040110"] || !v22.six["040110"] {
		t.Errorf("040110 should be in both")
	}
}

// Every HS 2022 code a reviewed correlation entry names as the later edition
// is in the list, and every parent it names is an HS 2017 code. The full check
// against the UNSD HS 2022 to HS 2017 table (all 5,612 codes) was done at
// extraction and is recorded in docs/japan/gsp-pilot.md.
func TestHS2022ListAgreesWithReviewedCorrelations(t *testing.T) {
	table := mustTable(t)
	n := 0
	for _, e := range table.corr.entries {
		if !strings.Contains(e.RequestedEdition, "2022") {
			continue
		}
		n++
		if !hs2022Valid().six[e.Code] {
			t.Errorf("%s %s is not in the HS 2022 list", e.Agreement, e.Code)
		}
		for _, p := range e.Parents {
			if !hs2017Valid().six[p] {
				t.Errorf("%s %s: parent %s is not in the HS 2017 list", e.Agreement, e.Code, p)
			}
		}
	}
	if n < 20 {
		t.Fatalf("only %d HS 2022 correlation entries seen", n)
	}
}

func TestGSPRowsMatchTheXML(t *testing.T) {
	cases := []struct {
		name, hs, ref, desc, text string
	}{
		{"chapter 2", "0201.10", "別表 row 1 (第二類)", "肉及び食用のくず肉",
			"第一類又は第二類に該当する物品以外の物品からの製造（加工を含む。以下この表において同じ。）"},
		{"chapter 4 range keeps both branches", "0405.10", "別表 row 3 (第四類)",
			"酪農品、鳥卵、天然はちみつ及び他の類に該当しない食用の動物性生産品（第〇四・一〇項に該当する物品を除く。）",
			"（１）　殻付きでない鳥卵及び卵黄（生鮮のもの及び乾燥、蒸気又は水煮による調理、成型、冷凍その他保存に適する処理をしたものに限るものとし、砂糖その他の甘味料を加えてあるかないかを問わない。）のうち\n卵黄以外のもの（乾燥したもの以外のものに限る。）：第四類に該当する物品以外の物品からの製造\n（２）　その他のもの：第四類に該当する物品以外の物品からの製造"},
		{"heading with a one-line rule", "1208.10", "別表 row 10 (一二・〇八)",
			"採油用の種又は果実の粉及びミール（マスタードの粉及びミールを除く。）", "第一二類に該当する物品以外の物品からの製造"},
		{"two items of one heading", "1212.94", "別表 row 11 (一二・一二)",
			"海草その他の藻類、ローカストビーン、てん菜及びさとうきび（生鮮のもの及び冷蔵し、冷凍し又は乾燥したものに限るものとし、粉砕してあるかないかを問わない。）並びに主として食用に供する果実の核及び仁その他の植物性生産品（チコリー（キコリウム・インテュブス変種サティヴム）の根で煎つてないものを含むものとし、他の項に該当するものを除く。）",
			"（１）　主として食用に供する果実の核及び仁その他の植物性生産品：第七類、第八類又は第一二類に該当する物品以外の物品からの製造\n（２）　こんにやく芋（アモルフォファルス）（切り、乾燥し又は粉状にしたものであるかないかを問わない。）：第一二類に該当する物品以外の物品からの製造"},
		{"chapter 18 minus 18.01, 18.02 and 18.06", "1804.00", "別表 row 31 (第一八類)",
			"ココア及びその調製品（第一八・〇一項、第一八・〇二項又は第一八・〇六項に該当する物品を除く。）", "カカオ豆からの製造"},
		{"chocolate has its own row", "1806.20", "別表 row 32 (一八・〇六)", "チョコレートその他のココアを含有する調製食料品",
			"（１）　各使用材料の重量割合のうちミルク（クリームを含む。）の重量割合が最も大きいもの\n（ｉ）　しよ糖の含有量が全重量の五〇％以上のもの：第一八・〇六項に該当する物品以外の物品からの製造（非原産品割合が四〇％以下となる製造であつて、かつ、製造に使用した砂糖及びミルク（クリームを含む。）は原産品に限る。）\n（ｉｉ）　その他のもの：第一八・〇六項に該当する物品以外の物品からの製造（非原産品割合が四〇％以下となる製造であつて、かつ、製造に使用したミルク（クリームを含む。）は原産品に限る。）\n（２）　その他のもの\n（ｉ）　しよ糖の含有量が全重量の五〇％以上のもの：第一八・〇六項に該当する物品以外の物品からの製造（非原産品割合が四〇％以下となる製造であつて、かつ、製造に使用した砂糖は原産品に限る。）\n（ｉｉ）　その他のもの：第一八・〇六項に該当する物品以外の物品からの製造（非原産品割合が四〇％以下となる製造に限る。）"},
		{"24.04 added in 2018", "2404.11", "別表 row 120 (二四・〇四)",
			"たばこ、再生たばこ、ニコチン又はたばこ代用物若しくはニコチン代用物を含有する物品（非燃焼吸引用の物品に限る。）及びニコチンを含有するその他の物品（ニコチンを人体に摂取するためのものに限る。）",
			"（１）　非燃焼吸引用の物品のうち\nたばこ又は再生たばこを含有するもの以外のもので製造たばこ代用品以外のもの：第二四・〇四項及び第三八・二四項に該当する物品以外の物品からの製造（非原産品割合が五〇％以下となる製造に限る。）\n（２）　その他のもの\n（ｉ）　経口摂取用のもの：第二一・〇六項及び第二四・〇四項に該当する物品以外の物品からの製造（非原産品割合が四〇％以下となる製造に限る。）\n（ｉｉ）　その他のもの：第二四・〇四項及び第三八・二四項に該当する物品以外の物品からの製造（非原産品割合が五〇％以下となる製造に限る。）"},
		{"chapter 38 range after the excluded headings", "3824.99", "別表 row 158 (第三八類)",
			"各種の化学工業生産品（第三八・〇一項から第三八・〇七項まで、第三八・〇九項、第三八・二一項又は第三八・二三項に該当する物品を除く。）",
			"製造しようとする物品と異なる関税定率法別表の項に属する物品からの製造（非原産品割合が五〇％以下となる製造に限る。）"},
		{"44.18 bamboo and wood joinery only in part", "4418.11", "別表 row 190 (四四・一八)",
			"木製建具及び建築用木工品（セルラーウッドパネル、組み合わせた床用パネル及びこけら板を含む。）のうち",
			"セルラーウッドパネル又はセルラーバンブーパネルを使用したもの：第四四・一八項に該当するセルラーウッドパネル及びセルラーバンブーパネル以外の物品からの製造"},
		{"46.01 plaiting materials only in part", "4601.21", "別表 row 194 (四六・〇一)",
			"さなだその他これに類する組物材料から成る物品（ストリップ状であるかないかを問わない。）並びに組物材料又はさなだその他これに類する組物材料から成る物品を平行につなぎ及び織つたものであつてシート状のもの（最終製品（敷物、壁掛等）であるかないかを問わない。）のうち",
			"さなだその他これに類する組物材料から成る物品を平行につなぎ及び織つたものであつてシート状のもの：第四六・〇一項に該当する物品（さなだその他これに類する組物材料から成る物品を除く。）以外の物品からの製造"},
		{"textile heading", "5111.11", "別表 row 204 (五一・一一)", "紡毛織物（羊毛製又は繊獣毛製のものに限る。）",
			"（１）　絹の重量が全重量の一〇％を超えるもの：化学品、第四七・〇一項から第四七・〇六項まで若しくは第五〇・〇一項に該当する物品、紡織用天然繊維（生糸を除く。）、人造繊維の短繊維又は紡織用繊維くずからの製造\n（２）　その他のもの：化学品、第四七・〇一項から第四七・〇六項までに該当する物品、紡織用天然繊維、人造繊維の短繊維又は紡織用繊維くずからの製造"},
		{"chapter 61 knitted apparel", "6110.20", "別表 row 296 (第六一類)", "衣類及び衣類附属品（メリヤス編み又はクロセ編みのものに限る。）", "紡織用繊維の織物類又は編物からの製造"},
		{"chapter 64 footwear", "6404.11", "別表 row 305 (第六四類)", "履物及びゲートルその他これに類する物品並びにこれらの部分品",
			"製造しようとする物品と異なる関税定率法別表の項（第六四・〇六項を除く。）に属する物品からの製造"},
		{"chapter 64 first code", "6401.10", "別表 row 305 (第六四類)", "履物及びゲートルその他これに類する物品並びにこれらの部分品",
			"製造しようとする物品と異なる関税定率法別表の項（第六四・〇六項を除く。）に属する物品からの製造"},
		{"chapter 90 range before 90.30", "9029.10", "別表 row 385 (第九〇類)",
			"光学機器、写真用機器、映画用機器、測定機器、検査機器、精密機器及び医療用機器並びにこれらの部分品及び附属品（第九〇・〇一項又は第九〇・三〇項に該当する物品を除く。）",
			"使用した非原産品のうち、生産された物品と異なる関税定率法別表の項に属するものの価格の生産された物品の価格のうちに占める割合が四〇％以下となり、かつ、生産された物品と同じ関税定率法別表の項に属するものの価格の生産された物品の価格のうちに占める割合が五％以下となる製造"},
		{"96.03 brushes only in part, with a sub-heading exclusion", "9603.21", "別表 row 390 (九六・〇三)",
			"ほうき、ブラシ（機械類又は車両の部分品として使用するブラシを含む。）、動力駆動式でない手動床掃除機、モップ及び羽毛ダスター、ほうき又はブラシの製造用に結束し又は房状にした物品、ペイントパッド、ペイントローラー並びにスクイージー（ローラースクイージーを除く。）のうち",
			"ほうき及びブラシ（第九六〇三・一〇号に該当するものを除く。）並びにペイントローラー、スクイージー及びモップ：第九六・〇三項に該当する物品以外の物品からの製造（非原産品割合が五〇％以下となる製造に限る。）\n動力駆動式でない手動床掃除機：第九六・〇三項に該当する動力駆動式でない手動床掃除機以外の物品からの製造"},
		{"last row", "9617.00", "別表 row 398 (九六・一七)", "魔法瓶その他の真空容器及びその部分品（ガラス製の内部容器を除く。）",
			"第七〇・二〇項に該当する物品（魔法瓶その他の真空容器用のガラス製の瓶に限る。）及び第九六・一七項に該当する物品以外の物品からの製造"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			res := gspLookup(t, c.hs, gspDate)
			if res.Reason != ReasonPSR || len(res.Evidence) == 0 {
				t.Fatalf("%s: %+v", c.hs, res)
			}
			e := res.Evidence[1] // the caveat comes first
			if want := c.desc + ": " + c.text; e.Text != want {
				t.Errorf("text = %q\nwant   %q", e.Text, want)
			}
			if !strings.HasSuffix(e.Citation, ", "+c.ref) || !strings.Contains(e.Citation, "e-Gov 344M50000040039_20230401_505M60000040029") {
				t.Errorf("citation = %q", e.Citation)
			}
			if e.URL != gspLawURL {
				t.Errorf("url = %q", e.URL)
			}
			if n := ruleRows(res, gspSchedule(t).Notes); n != 1 {
				t.Errorf("%d rule rows", n)
			}
		})
	}
}

func TestGSPChapter4RangeStopsBeforeHeading0410(t *testing.T) {
	// The row for chapter 4 excludes heading 04.10 in its own description, so
	// the stored range is 04.01 to 04.09 and heading 04.10 has no row.
	for _, hs := range []string{"0401.10", "0409.00", "0409"} {
		if res := gspLookup(t, hs, gspDate); res.Reason != ReasonPSR {
			t.Errorf("%s: %+v", hs, res)
		}
	}
	for _, hs := range []string{"0410.10", "0410.90", "0410"} {
		if res := gspLookup(t, hs, gspDate); res.Reason != ReasonNoMatch || len(res.Evidence) != 0 {
			t.Errorf("%s: %+v", hs, res)
		}
	}
	// Old HS 2017 code 0410.00 is not in HS 2022: no rule for it.
	if res := gspLookup(t, "0410.00", gspDate); res.Reason != ReasonNotInEdition {
		t.Errorf("0410.00: %+v", res)
	}
	// Heading 90.30 is excluded from the chapter 90 row, 90.31 is not.
	if res := gspLookup(t, "9030.10", gspDate); res.Reason != ReasonNoMatch {
		t.Errorf("9030.10 is excluded from the chapter 90 row: %+v", res)
	}
	if res := gspLookup(t, "9031.10", gspDate); res.Reason != ReasonPSR {
		t.Errorf("9031.10: %+v", res)
	}
}

func TestGSPDatesAndAgreementName(t *testing.T) {
	cases := []struct {
		date string
		want Reason
	}{
		{"2023-03-31", ReasonNoDatedSource},
		{"2023-04-01", ReasonPSR},
		{"2024-12-15", ReasonPSR},
		{"2030-01-01", ReasonPSR},
		{"2022-12-31", ReasonNoDatedSource},
		{"2019-07-01", ReasonNoDatedSource},
	}
	for _, c := range cases {
		res := gspLookup(t, "6404.11", c.date)
		if res.Reason != c.want {
			t.Errorf("%s: %s, want %s", c.date, res.Reason, c.want)
		}
		if c.want == ReasonNoDatedSource && len(res.Evidence) != 0 {
			t.Errorf("%s: evidence without a source", c.date)
		}
	}
	s := gspSchedule(t)
	if s.From != "2023-04-01" || s.Until != "" || s.HSEdition != 2022 {
		t.Errorf("window %q-%q edition %d", s.From, s.Until, s.HSEdition)
	}
	if !mustTable(t).HasAgreement(gspAgreement) {
		t.Error("GSP has no schedule")
	}
}

func TestGSPCitationIsHonestAboutTheEdition(t *testing.T) {
	s := gspSchedule(t)
	for _, want := range []string{
		"HS edition not stated; codes validated against HS 2022",
		"Japanese text",
		"e-Gov 344M50000040039_20230401_505M60000040029",
		"revision in force from 2023-04-01",
	} {
		if !strings.Contains(s.Citation, want) {
			t.Errorf("citation lacks %q: %s", want, s.Citation)
		}
	}
	if strings.Contains(s.Citation, "(HS 2017)") || strings.Contains(s.Citation, "(HS 2022)") {
		t.Errorf("citation claims an edition: %s", s.Citation)
	}
	if s.URL != gspLawURL || s.SHA256 != "8dfd625b110a8fd9699f6a17d583c17b0cb3a957388da398b7484392a8ddfb56" {
		t.Errorf("source %s %s", s.URL, s.SHA256)
	}
}

func TestGSPEvidenceUsesRefNotAPageFragment(t *testing.T) {
	for _, hs := range []string{"6404.11", "1806.20", "6110.20", "8541.10"} {
		res := gspLookup(t, hs, gspDate)
		if res.Reason != ReasonPSR {
			t.Fatalf("%s: %+v", hs, res)
		}
		for _, e := range res.Evidence {
			if strings.Contains(e.URL, "#") || strings.Contains(e.Citation, "PDF page") {
				t.Errorf("%s: page style citation %+v", hs, e)
			}
			if e.URL == "" {
				if !strings.HasPrefix(e.Text, caveatPrefix) || e.Citation != caveatPrefix {
					t.Errorf("%s: bad caveat %+v", hs, e)
				}
				continue
			}
			if e.URL != gspLawURL || !strings.Contains(e.Citation, ", 別表") && !strings.Contains(e.Citation, ", 第") {
				t.Errorf("%s: evidence %+v", hs, e)
			}
		}
	}
}

func TestGSPNotesFollowTheirScope(t *testing.T) {
	const (
		art8     = "令第二十六条第一項第一号（原産地の意義）に規定する財務省令で定める物品は、次に掲げる物品とする。"
		art91    = "令第二十六条第一項第二号に規定する財務省令で定める加工又は製造は、"
		art92    = "前項の規定の適用上、関税定率法別表第五十類から第六十三類までに該当する物品にあつては、"
		art93    = "第一項の規定の適用上、異なる材料から成る物品、"
		rem1     = "一　この表の次の用語については、それぞれ次に定めるところによる。"
		rem2     = "二　この表の各項において、同表の下欄において製造につき非原産品割合が一定の率以下となることが必要とされる条件"
		rem3     = "三　この表の下欄において定率基準以外の条件は、"
		rem4     = "四　この表の下欄に記載する紡織用天然繊維及び人造繊維の短繊維には、"
		rem5     = "五　関税定率法別表第六一類から第六三類までに該当する物品が原産品であるか否かを決定するに当たり、"
		caveatTx = "Unde Labs note (not treaty text)"
	)
	has := func(ev []Evidence, frag string) *Evidence {
		for i := range ev {
			if strings.Contains(ev[i].Text, frag) {
				return &ev[i]
			}
		}
		return nil
	}
	all := []string{art8, art91, art93, rem1, rem2, rem3, caveatTx}
	cases := []struct {
		hs      string
		with    []string
		without []string
	}{
		{"0201.10", all, []string{art92, rem4, rem5}},
		{"6404.11", all, []string{art92, rem4, rem5}},
		{"8541.10", all, []string{art92, rem4, rem5}},
		{"5111.11", append([]string{art92, rem4}, all...), []string{rem5}},
		{"5407.10", append([]string{art92, rem4}, all...), []string{rem5}},
		{"6110.20", append([]string{art92, rem4, rem5}, all...), nil},
		{"6203.42", append([]string{art92, rem4, rem5}, all...), nil},
		{"6307.90", append([]string{art92, rem4, rem5}, all...), nil},
		{"6001.10", append([]string{art92, rem4}, all...), []string{rem5}},
		{"6310.90", append([]string{art92, rem4, rem5}, all...), nil},
	}
	for _, c := range cases {
		t.Run(c.hs, func(t *testing.T) {
			res := gspLookup(t, c.hs, gspDate)
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
			seen := map[string]bool{}
			for _, e := range res.Evidence {
				if seen[e.Text] {
					t.Errorf("duplicate evidence %.30q", e.Text)
				}
				seen[e.Text] = true
			}
			first := res.Evidence[0]
			if !strings.HasPrefix(first.Text, caveatTx) || first.URL != "" {
				t.Errorf("first evidence = %+v", first)
			}
		})
	}
	// Notes only ever accompany a rule row.
	for _, hs := range []string{"0101.21", "0410.10", "2501.00"} {
		if res := gspLookup(t, hs, gspDate); len(res.Evidence) != 0 {
			t.Errorf("%s: %+v", hs, res)
		}
	}
	// Notes are stored once for the schedule, not once per HS section.
	s := gspSchedule(t)
	if len(s.Notes) != 10 {
		t.Errorf("%d notes", len(s.Notes))
	}
	for _, n := range s.Notes {
		if n.Scope == "chapter" {
			t.Errorf("unexpected chapter note %+v", n)
		}
	}
}

func TestGSPCaveatNamesTheOmissions(t *testing.T) {
	res := gspLookup(t, "6404.11", gspDate)
	c := res.Evidence[0].Text
	for _, want := range []string{
		"least-developed", "different or relaxed rules", "were not checked",
		"beneficiary", "Cabinet Order", "Form A", "cumulation", "direct consignment",
		"NOT checked", "not an eligibility finding", "does not name an HS edition", "HS 2022", "partial",
		"2023-04-01", "Article 9 paragraph 1", "narrowing words",
	} {
		if !strings.Contains(c, want) {
			t.Errorf("caveat lacks %q:\n%s", want, c)
		}
	}
	own, err := json.Marshal(gspSchedule(t))
	if err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"/Users/", ".claude", "controller", "dossier", "handoff", "prototype"} {
		if strings.Contains(string(own), bad) {
			t.Errorf("public data contains %q", bad)
		}
	}
}

func TestGSPRowShapeAndCounts(t *testing.T) {
	s := gspSchedule(t)
	if len(s.Rows) != 251 {
		t.Fatalf("%d rows", len(s.Rows))
	}
	two, four := 0, 0
	for _, r := range s.Rows {
		switch len(r.Start) {
		case 2:
			two++
		case 4:
			four++
		default:
			t.Errorf("row width %q", r.Start)
		}
		if !r.Default {
			t.Errorf("row %s-%s is not edition gated", r.Start, r.End)
		}
		if r.Blocked || r.Page != 1 || !strings.HasPrefix(r.Ref, "別表 row ") {
			t.Errorf("row %s-%s: %+v", r.Start, r.End, r)
		}
		if strings.TrimSpace(r.Description) == "" {
			t.Errorf("row %s-%s has no description", r.Start, r.End)
		}
		// Row ends lie in the HS 2022 list: the table names real codes.
		for _, w := range []string{r.Start, r.End} {
			if !hs2022Valid().hasPrefix(w) {
				t.Errorf("row %s-%s: %s is not an HS 2022 prefix", r.Start, r.End, w)
			}
		}
	}
	if two != 13 || four != 238 {
		t.Errorf("two-digit %d, four-digit %d", two, four)
	}
	for _, n := range s.Notes {
		if n.Page == 0 {
			if n.Ref != "" {
				t.Errorf("caveat has a ref: %+v", n)
			}
			continue
		}
		if n.Page != 1 || n.Ref == "" {
			t.Errorf("note %+v", n)
		}
	}
}

func TestGSPEveryHS2022CodeGetsAtMostOneRuleRow(t *testing.T) {
	notes := gspSchedule(t).Notes
	psr, none := 0, 0
	for _, code := range hs2022All(t) {
		res := gspLookup(t, code, gspDate)
		switch res.Reason {
		case ReasonPSR:
			psr++
			if n := ruleRows(res, notes); n != 1 {
				t.Errorf("%s: %d rule rows", code, n)
			}
		case ReasonNoMatch:
			none++
			if len(res.Evidence) != 0 {
				t.Errorf("%s: evidence on no_match", code)
			}
		default:
			t.Errorf("%s: reason %s", code, res.Reason)
		}
	}
	if psr < 2000 || none == 0 {
		t.Fatalf("psr %d, none %d", psr, none)
	}
	// Four digit inputs also meet at most one row.
	for h := 100; h <= 9999; h++ {
		code := fmt.Sprintf("%04d", h)
		res := gspLookup(t, code, gspDate)
		switch res.Reason {
		case ReasonPSR:
			if n := ruleRows(res, notes); n != 1 {
				t.Errorf("%s: %d rule rows", code, n)
			}
		case ReasonNoMatch, ReasonNotInEdition, ReasonInvalidCode:
		default:
			t.Errorf("%s: reason %s", code, res.Reason)
		}
	}
}

func TestGSPNoCodeOutsideHS2022ReturnsARule(t *testing.T) {
	valid := map[string]bool{}
	for _, c := range hs2022All(t) {
		valid[c] = true
	}
	table := mustTable(t)
	for h := 100; h <= 999999; h++ {
		code := fmt.Sprintf("%06d", h)
		if valid[code] {
			continue
		}
		res, err := table.LookupDetailed(gspAgreement, code, day(t, gspDate))
		if err != nil {
			t.Fatal(err)
		}
		if res.Reason == ReasonPSR || len(res.Evidence) > 0 {
			t.Fatalf("%s is not an HS 2022 code but got %+v", code, res)
		}
		switch code[:2] {
		case "00", "77", "98", "99":
		default:
			if res.Reason != ReasonNotInEdition {
				t.Fatalf("%s: reason %s", code, res.Reason)
			}
		}
	}
	for _, hs := range []string{"0100.99", "1999.99", "0399.99", "6499.99", "8599.99"} {
		if res := gspLookup(t, hs, gspDate); res.Reason != ReasonNotInEdition {
			t.Errorf("%s: %+v", hs, res)
		}
	}
	// A heading with no subheading in HS 2022 is not a code either.
	if res := gspLookup(t, "1999", gspDate); res.Reason != ReasonNotInEdition {
		t.Errorf("1999: %+v", res)
	}
}

func TestGSPGoodsNotInTheTableGetNoRule(t *testing.T) {
	for _, hs := range []string{"0101.21", "2501.00", "2710.12", "7108.12", "8471.30"} {
		res := gspLookup(t, hs, gspDate)
		if res.Reason != ReasonNoMatch || len(res.Evidence) != 0 {
			t.Errorf("%s: %+v", hs, res)
		}
	}
}

// The edition list selects which rows apply: the same default row answers an
// HS 2022 code under an HS 2022 schedule and an HS 2017 code under HS 2017.
func TestScheduleEditionSelectsTheListThatGatesDefaultRows(t *testing.T) {
	mk := func(edition int) *Table {
		s := schedule{
			Agreement: "X", Citation: "c", URL: "https://example.org/x", From: "2020-01-01", HSEdition: edition,
			Rows: []row{{Start: "04", End: "04", Page: 1, Text: "CC", Default: true}},
		}
		if err := validateSchedule(s); err != nil {
			t.Fatal(err)
		}
		return &Table{schedules: map[string][]schedule{"X": {s}}}
	}
	cases := []struct {
		edition int
		hs      string
		want    Reason
	}{
		{2022, "0410.10", ReasonPSR},          // HS 2022 only
		{2022, "0410.00", ReasonNotInEdition}, // HS 2017 only
		{2017, "0410.00", ReasonPSR},
		{2017, "0410.10", ReasonNotInEdition},
		{0, "0410.00", ReasonPSR}, // no edition means HS 2017
		{0, "0410.10", ReasonNotInEdition},
		{2022, "0401.10", ReasonPSR},
		{2017, "0401.10", ReasonPSR},
		{2022, "0499.99", ReasonNotInEdition},
	}
	for _, c := range cases {
		res, err := mk(c.edition).LookupDetailed("X", c.hs, day(t, "2026-11-23"))
		if err != nil {
			t.Fatal(err)
		}
		if res.Reason != c.want {
			t.Errorf("edition %d %s: %s, want %s", c.edition, c.hs, res.Reason, c.want)
		}
	}
}

func TestExistingSchedulesKeepTheHS2017List(t *testing.T) {
	for name, list := range mustTable(t).schedules {
		if name == gspAgreement {
			continue
		}
		for _, s := range list {
			if s.HSEdition != 0 {
				t.Errorf("%s %s sets an edition", name, s.From)
			}
			for _, r := range s.Rows {
				// Only the India chapter 3 rows, quoted from the official notice
				// (docs/japan/india-chapter3-amendment.md), carry their own source.
				if r.SourceURL != "" && !(name == "JP-India CEPA" && s.From == "2022-04-04" && strings.HasPrefix(r.Start, "03")) {
					t.Errorf("%s %s: row %s has its own source", name, s.From, r.Start)
				}
				if r.Ref != "" && r.SourceURL == "" {
					t.Errorf("%s: row %s has a ref", name, r.Start)
				}
			}
			for _, n := range s.Notes {
				// A schedule-scope note of a PDF-paged schedule needs its own source
				// (source_url, EU/UK Annex 3-A) or, for the Australia General Notes
				// (australiacolumn4_test.go), an applies list, or be the one Unde Labs
				// caveat (page 0) of the EU and UK Chapter 3 articles (chapter3_test.go).
				sourced := n.SourceURL != "" || name == "JP-Australia EPA" && len(n.Applies) > 0 ||
					n.Page == 0 && (name == "JP-EU EPA" || name == "JP-UK CEPA") && strings.Contains(n.Text, "Chapter 3 (Rules of origin)")
				if n.Ref != "" && !(n.SourceURL != "" && name == "JP-India CEPA" && s.From == "2022-04-04" && n.Key == "03") || n.Scope == "schedule" && !sourced {
					t.Errorf("%s: note %q has a ref or schedule scope", name, n.Key)
				}
			}
		}
	}
}

func TestLoaderRejectsMalformedRefSchedules(t *testing.T) {
	good := schedule{
		Agreement: "X", Citation: "c", URL: "https://laws.example.org/law/1", From: "2020-01-01", HSEdition: 2022,
		Rows: []row{{Start: "0401", End: "0404", Page: 1, Ref: "別表 row 3", Text: "CC", Default: true}},
		Notes: []note{
			{Scope: "schedule", Text: "t", Page: 1, Ref: "第八条"},
			{Scope: "section", Key: "I", Text: "t2", Page: 1, Ref: "備考一", Applies: [][2]string{{"01", "02"}}},
		},
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
		"edition 2012":               mut(func(s *schedule) { s.HSEdition = 2012 }),
		"edition negative":           mut(func(s *schedule) { s.HSEdition = -1 }),
		"row ref blank":              mut(func(s *schedule) { s.Rows[0].Ref = "  " }),
		"row ref padded":             mut(func(s *schedule) { s.Rows[0].Ref = " 別表 row 3" }),
		"row ref but page 2":         mut(func(s *schedule) { s.Rows[0].Page = 2 }),
		"url with fragment":          mut(func(s *schedule) { s.URL = "https://laws.example.org/law/1#page=1" }),
		"row missing the ref":        mut(func(s *schedule) { s.Rows[0].Ref = "" }),
		"note missing the ref":       mut(func(s *schedule) { s.Notes[1].Ref = "" }),
		"note ref on page 2":         mut(func(s *schedule) { s.Notes[1].Page = 2 }),
		"schedule scope with a key":  mut(func(s *schedule) { s.Notes[0].Key = "I" }),
		"schedule scope bad applies": mut(func(s *schedule) { s.Notes[0].Applies = [][2]string{{"9", "9"}} }),
		"caveat with a ref":          mut(func(s *schedule) { s.Notes[0] = note{Scope: "schedule", Text: caveatPrefix + ": partial.", Ref: "x"} }),
		"row without ref beside refs": mut(func(s *schedule) {
			s.Rows = append(s.Rows, row{Start: "05", End: "05", Page: 3, Text: "CC", Default: true})
		}),
	}
	for name, s := range bad {
		if err := validateSchedule(s); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	// A page-numbered schedule rejects a stray ref instead of silently dropping it.
	paged := schedule{
		Agreement: "X", Citation: "c", URL: "https://example.org/x.pdf", From: "2020-01-01",
		Rows: []row{{Start: "0401", End: "0404", Page: 3, Text: "CC"}, {Start: "0405", End: "0406", Page: 4, Text: "CC", Ref: "row 1"}},
	}
	if err := validateSchedule(paged); err == nil {
		t.Error("mixed ref and page numbering accepted")
	}
	// A schedule scope note is the one scope that needs no key and may carry applicability.
	ok := mut(func(s *schedule) { s.Notes[0].Applies = [][2]string{{"61", "63"}} })
	if err := validateSchedule(ok); err != nil {
		t.Errorf("schedule scope with applicability rejected: %v", err)
	}
}

func TestEvidenceCitationWithRefHasNoPageFragment(t *testing.T) {
	s := schedule{
		Agreement: "X", Citation: "Law Z", URL: "https://laws.example.org/law/1", From: "2020-01-01", HSEdition: 2022,
		Rows:  []row{{Start: "0401", End: "0404", Page: 1, Ref: "別表 row 3", Text: "CC", Default: true}},
		Notes: []note{{Scope: "schedule", Text: "N", Page: 1, Ref: "第八条"}},
	}
	if err := validateSchedule(s); err != nil {
		t.Fatal(err)
	}
	tbl := &Table{schedules: map[string][]schedule{"X": {s}}}
	res, err := tbl.LookupDetailed("X", "0402.10", day(t, "2026-11-23"))
	if err != nil || res.Reason != ReasonPSR || len(res.Evidence) != 2 {
		t.Fatalf("%+v %v", res, err)
	}
	want := []Evidence{
		{Text: "CC", URL: "https://laws.example.org/law/1", Citation: "Law Z, 別表 row 3"},
		{Text: "N", URL: "https://laws.example.org/law/1", Citation: "Law Z, 第八条"},
	}
	if !slices.Equal(res.Evidence, want) {
		t.Errorf("evidence = %+v", res.Evidence)
	}
}

func TestCaveatFirstPutsTheCaveatBeforeTheRows(t *testing.T) {
	s := schedule{
		Agreement: "X", Citation: "Law Z", URL: "https://laws.example.org/law/1", From: "2020-01-01", CaveatFirst: true,
		Rows:  []row{{Start: "0401", End: "0404", Page: 1, Ref: "r", Text: "CC"}},
		Notes: []note{{Scope: "schedule", Text: "N", Page: 1, Ref: "n"}, {Scope: "schedule", Text: caveatPrefix + ": partial."}},
	}
	if err := validateSchedule(s); err != nil {
		t.Fatal(err)
	}
	res, err := (&Table{schedules: map[string][]schedule{"X": {s}}}).LookupDetailed("X", "0402", day(t, "2026-11-23"))
	if err != nil || len(res.Evidence) != 3 || !strings.HasPrefix(res.Evidence[0].Text, caveatPrefix) || res.Evidence[1].Text != "CC" || res.Evidence[2].Text != "N" {
		t.Fatalf("%+v %v", res, err)
	}
	s.Notes = s.Notes[:1]
	if err := validateSchedule(s); err == nil {
		t.Error("caveat_first without a caveat accepted")
	}
	for name, list := range mustTable(t).schedules {
		for _, e := range list {
			if e.CaveatFirst != (name == gspAgreement) {
				t.Errorf("%s caveat_first = %v", name, e.CaveatFirst)
			}
		}
	}
}
