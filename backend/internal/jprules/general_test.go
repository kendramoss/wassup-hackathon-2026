package jprules

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"
)

func day(t *testing.T, s string) time.Time {
	t.Helper()
	d, err := time.Parse(time.DateOnly, s)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func mustTable(t *testing.T) *Table {
	t.Helper()
	table, err := Default()
	if err != nil {
		t.Fatal(err)
	}
	return table
}

func TestLookupDetailedMatchesLegacyLookup(t *testing.T) {
	table := mustTable(t)
	agreements := []string{"AJCEP", "RCEP", "CPTPP", "JP-EU EPA", "JP-UK CEPA", "JP-Thailand EPA", "JP-Switzerland EPA", "GSP", "JP-US TA", "", "Unknown"}
	dates := []string{"2005-01-01", "2019-02-01", "2022-12-31", "2023-01-01", "2026-11-23", "2027-02-14", "2027-02-15", "2030-01-01"}
	codes := []string{"", "x", "0000.00", "0101", "6404.11", "6204.43", "52.11", "6210.50", "0406.90-000", "1517.90", "0304", "8708.94", "9999", "7701", "321590", "3215", "1704.90-010", "170490010"}
	for h := 1; h <= 9999; h++ {
		codes = append(codes, fmt.Sprintf("%04d", h))
	}
	for _, a := range agreements {
		for _, ds := range dates {
			d := day(t, ds)
			for _, c := range codes {
				old, err1 := table.Lookup(a, c, d)
				res, err2 := table.LookupDetailed(a, c, d)
				if err1 != nil || err2 != nil {
					t.Fatal(err1, err2)
				}
				switch res.Reason {
				case ReasonPSR:
					if !reflect.DeepEqual(res.Evidence, old) || len(old) == 0 {
						t.Fatalf("%s %s %s: psr evidence differs: %v vs %v", a, ds, c, res.Evidence, old)
					}
				case ReasonGeneral:
					if len(old) != 0 || len(res.Evidence) == 0 {
						t.Fatalf("%s %s %s: general must not leak into Lookup", a, ds, c)
					}
				default:
					if len(old) != 0 || res.Evidence != nil {
						t.Fatalf("%s %s %s: reason %q has evidence", a, ds, c, res.Reason)
					}
				}
			}
		}
	}
	// Zero date.
	if res, _ := table.LookupDetailed("AJCEP", "3215", time.Time{}); res.Reason != ReasonMalformed || res.Evidence != nil {
		t.Fatalf("zero date: %+v", res)
	}
}

func TestLookupDetailedReasons(t *testing.T) {
	table := mustTable(t)
	cases := []struct {
		name, agreement, hs, date string
		want                      Reason
	}{
		{"malformed syntax", "AJCEP", "not a code", "2025-01-01", ReasonMalformed},
		{"empty", "AJCEP", "", "2025-01-01", ReasonMalformed},
		{"unsupported agreement", "JP-Peru EPA", "3215", "2025-01-01", ReasonUnsupportedAgreement},
		{"GSP is supported from its window", "GSP", "3215", "2025-01-01", ReasonNoMatch},
		{"GSP before its window", "GSP", "3215", "2023-03-31", ReasonNoDatedSource},
		{"JP-US TA unlisted goods", "JP-US TA", "3215", "2025-01-01", ReasonNoMatch},
		{"unsupported agreement empty", "", "3215", "2025-01-01", ReasonUnsupportedAgreement},
		{"RCEP before start", "RCEP", "6404.11", "2022-12-31", ReasonNoDatedSource},
		{"AJCEP before window", "AJCEP", "3215", "2022-12-31", ReasonNoDatedSource},
		{"JP-Thailand before its start", "JP-Thailand EPA", "3215", "2022-12-31", ReasonNoDatedSource},
		{"RCEP miss is no_match", "RCEP", "0000.00", "2025-01-01", ReasonNoMatch},
		{"RCEP hit", "RCEP", "6404.11", "2025-01-01", ReasonPSR},
		{"too broad AJCEP heading", "AJCEP", "0901", "2025-01-01", ReasonTooBroad},
		{"too broad CPTPP heading", "CPTPP", "0304", "2025-01-01", ReasonTooBroad},
		{"withheld CPTPP row", "CPTPP", "8708.94", "2025-01-01", ReasonWithheld},
		{"withheld EU row", "JP-EU EPA", "8708.94", "2025-01-01", ReasonWithheld},
		{"AJCEP PSR", "AJCEP", "0901.11", "2025-01-01", ReasonPSR},
		{"AJCEP PSR via chapter row", "AJCEP", "0101", "2025-01-01", ReasonPSR},
		{"AJCEP textile PSR unchanged", "AJCEP", "6210.50", "2025-01-01", ReasonPSR},
		{"AJCEP unlisted 4-digit", "AJCEP", "3215", "2025-01-01", ReasonGeneral},
		{"AJCEP unlisted 6-digit", "AJCEP", "3215.90", "2025-01-01", ReasonGeneral},
		{"AJCEP unlisted 9-digit", "AJCEP", "3215.90-000", "2025-01-01", ReasonGeneral},
		{"AJCEP unlisted chapter", "AJCEP", "9018", "2025-01-01", ReasonGeneral},
		{"AJCEP chapter 77 reserved", "AJCEP", "7701", "2025-01-01", ReasonInvalidCode},
		{"AJCEP chapter 98 reserved", "AJCEP", "9801.00", "2025-01-01", ReasonInvalidCode},
		{"AJCEP chapter 99 reserved", "AJCEP", "9999", "2025-01-01", ReasonInvalidCode},
		{"AJCEP chapter 00", "AJCEP", "0000", "2025-01-01", ReasonInvalidCode},
		{"AJCEP from inclusive-1", "AJCEP", "3215", "2022-12-31", ReasonNoDatedSource},
		{"AJCEP from inclusive", "AJCEP", "3215", "2023-01-01", ReasonGeneral},
		{"AJCEP until exclusive-1", "AJCEP", "3215", "2027-02-14", ReasonGeneral},
		{"AJCEP until exclusive", "AJCEP", "3215", "2027-02-15", ReasonNoMatch},
		{"AJCEP HS2022 future PSR unchanged", "AJCEP", "0101", "2027-02-15", ReasonPSR},
		{"AJCEP HS2022 edition has no default", "AJCEP", "9999", "2027-02-15", ReasonInvalidCode},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			res, err := table.LookupDetailed(c.agreement, c.hs, day(t, c.date))
			if err != nil {
				t.Fatal(err)
			}
			if res.Reason != c.want {
				t.Fatalf("reason = %q (%d evidence), want %q", res.Reason, len(res.Evidence), c.want)
			}
			if c.want != ReasonPSR && c.want != ReasonGeneral && res.Evidence != nil {
				t.Fatalf("non-evidence reason carried evidence: %v", res.Evidence)
			}
		})
	}
}

func TestChapters50To63ExcludedFromGeneralDefault(t *testing.T) {
	table := mustTable(t)
	d := day(t, "2025-01-01")
	v := hs2017Valid()
	for ch := 50; ch <= 63; ch++ {
		for h := 0; h < 100; h++ {
			hs := fmt.Sprintf("%02d%02d", ch, h)
			res, err := table.LookupDetailed("AJCEP", hs, d)
			if err != nil {
				t.Fatal(err)
			}
			if res.Reason == ReasonGeneral {
				t.Fatalf("%s got general evidence", hs)
			}
			if res.Evidence != nil && res.Reason != ReasonPSR {
				t.Fatalf("%s reason %q has evidence", hs, res.Reason)
			}
			// Since the general fallback requires a real HS 2017 code, a
			// code that reaches it is excluded_general only if it exists; the
			// shipped schedule has a PSR row for every real textile heading,
			// so a code without a PSR row here must be absent from HS 2017.
			if res.Reason == ReasonInvalidCode && v.hasPrefix(hs) {
				t.Fatalf("%s is a real heading but invalid_code", hs)
			}
			if res.Reason == ReasonExcludedGeneral && !v.hasPrefix(hs) {
				t.Fatalf("%s excluded_general but absent from HS 2017", hs)
			}
		}
	}
	// The excluded_general branch itself is exercised in
	// TestTextileOrderingValidBeforeExcluded on a schedule without textile rows.
	// Chapters 49 and 64 are not excluded.
	for _, hs := range []string{"4911", "6499", "6401"} {
		res, _ := table.LookupDetailed("AJCEP", hs, d)
		if res.Reason == ReasonExcludedGeneral {
			t.Fatalf("%s wrongly excluded", hs)
		}
	}
}

func TestReservedAndMalformedNeverFallThrough(t *testing.T) {
	table := mustTable(t)
	d := day(t, "2025-01-01")
	for _, hs := range []string{"0000", "00.01", "7799", "7701.10", "9800", "9899.00", "9900", "9999.99-999", "abc", "12", "123", "12345", "1234567", "321590 x"} {
		res, err := table.LookupDetailed("AJCEP", hs, d)
		if err != nil {
			t.Fatal(err)
		}
		if res.Reason == ReasonGeneral || res.Reason == ReasonPSR || res.Evidence != nil {
			t.Errorf("%q: %+v", hs, res)
		}
	}
}

func TestGeneralBundleEvidence(t *testing.T) {
	table := mustTable(t)
	required := map[string]int{ // provision id -> pdf page
		"art23-a": 1, "art23-b-f": 2, "art23-g-k": 3, "art25-intro": 3, "art25-a-i": 4, "art25-j-k": 5,
		"art24": 3, "art26-1-2": 5, "art26-3-5": 6, "art27-1-3": 6, "art27-4-6": 7,
		"art28-1a": 7, "art28-1b-2": 8, "art29": 8, "art30-a-d": 8, "art30-e-g": 9,
		"art32-33a": 9, "art33": 10, "art34-a": 10, "art34-b-35": 11,
		"annex3": 1, "annex2-1-3a": 1, "annex2-3b": 2,
		"note-scope": 0, "note-hs": 0, "note-annex3": 0, "note-direct-consignment": 0, "note-certification": 0,
	}
	b := table.general
	if b == nil {
		t.Fatal("no general bundle")
	}
	ids := map[string]generalProvision{}
	for _, p := range b.Provisions {
		ids[p.ID] = p
	}
	for id, page := range required {
		p, ok := ids[id]
		if !ok || p.Page != page {
			t.Errorf("provision %s: got %+v, want page %d", id, p, page)
		}
	}
	if len(ids) != len(required) {
		t.Errorf("provisions %d, required %d", len(ids), len(required))
	}
	for _, hs := range []string{"3215", "3215.90", "321590", "9018.90-000"} {
		res, err := table.LookupDetailed("AJCEP", hs, day(t, "2025-06-01"))
		if err != nil || res.Reason != ReasonGeneral {
			t.Fatalf("%s: %+v %v", hs, res, err)
		}
		if len(res.Evidence) != len(b.Provisions) {
			t.Fatalf("evidence %d, provisions %d", len(res.Evidence), len(b.Provisions))
		}
		for i, e := range res.Evidence {
			p := b.Provisions[i]
			if p.Kind == "note" {
				if e.Text != p.Text || e.URL != "" || e.Citation != "Unde Labs note (not treaty text)" || p.Page != 0 || p.Source != "" {
					t.Fatalf("note evidence %d = %+v for %+v", i, e, p)
				}
				continue
			}
			src := b.source(p.Source)
			if e.Text != p.Text || e.URL != fmt.Sprintf("%s#page=%d", src.URL, p.Page) ||
				e.Citation != fmt.Sprintf("%s, PDF page %d", p.Citation, p.Page) {
				t.Fatalf("evidence %d = %+v for %+v", i, e, p)
			}
			if !strings.HasPrefix(e.URL, "https://www.customs.go.jp/roo/english/text/asean") {
				t.Fatalf("url %s", e.URL)
			}
		}
	}
	// Required legal content.
	all := ""
	for _, p := range b.Provisions {
		all += p.Text + "\n"
	}
	for _, frag := range []string{
		"\"exporter\" means a natural or juridical person", "\"materials\" means any matter or substance", "\"production\" means methods of obtaining a good",
		"\"originating good\" or \"originating material\" means", "\"identical and interchangeable materials\" means",
		"Article 25 Goods Wholly Obtained or Produced", "(k) goods obtained or produced in the Party exclusively from goods referred to in paragraphs (a) through (j)",
		"not less than forty (40) per cent", "exporter of the good to decide whether to use subparagraph (a) or (b)",
		"Notwithstanding paragraph 1, a good subject to product specific rules",
		"RVC = ((FOB - VNM) / FOB) X 100 %", "Agreement on Customs Valuation", "ten (10) per cent of the total weight",
		"Originating materials of a Party used in the production of a good in another Party",
		"(g) any combination of operations referred to in subparagraphs (a) through (f)",
		"Indirect materials shall be treated as originating materials", "generally accepted accounting principles of stock control",
		"except where the former good is classified under subheadings 8541.10 through 8542.90",
		"based on the Harmonized System as amended on 1 January 2017", "subheading 2103.90",
		"not treaty text", "ITA", "Article 31", "Annex 4", "not a claim of full preferential entitlement",
		"does not validate that the tariff code is correct", "does not convert codes between Harmonized System editions",
	} {
		if !strings.Contains(all, frag) {
			t.Errorf("missing %q", frag)
		}
	}
	for _, p := range b.Provisions {
		if p.Kind == "note" && strings.Contains(p.Citation, "PDF page") {
			t.Errorf("%s: note citation names a page", p.ID)
		}
		if (p.Kind == "note") != strings.HasPrefix(p.Text, "Unde Labs note (not treaty text): ") {
			t.Errorf("%s kind/prefix mismatch", p.ID)
		}
		if p.Kind != "note" && p.Kind != "treaty" {
			t.Errorf("%s kind %q", p.ID, p.Kind)
		}
	}
}

func TestGeneralBundleMetadata(t *testing.T) {
	var raw struct {
		Agreement string `json:"agreement"`
		Citation  string `json:"citation"`
		From      string `json:"from"`
		Until     string `json:"until"`
		HS        int    `json:"hs_edition"`
		Retrieved string `json:"retrieved"`
		Sources   []struct {
			Name     string `json:"name"`
			URL      string `json:"url"`
			SHA256   string `json:"sha256"`
			PDFPages int    `json:"pdf_pages"`
		} `json:"sources"`
	}
	if err := json.Unmarshal(generalJSON, &raw); err != nil {
		t.Fatal(err)
	}
	wantHash := map[string]string{
		"asean1.pdf":   "7f8c9e8167ff15203faf538d2959bf646018f77aa8a715b60a34fba295e350ba",
		"asean2-1.pdf": "00b7e42a64fd0fb7dad809ba3b3d487ecfd84ded5e103eea229fe973de3e127d",
		"asean3.pdf":   "6640710884d66b31d34d2eacfadc33a3bbc7930a5a81658db2edbd19d39bd92e",
	}
	if len(raw.Sources) != len(wantHash) {
		t.Fatalf("sources %d", len(raw.Sources))
	}
	for _, s := range raw.Sources {
		if wantHash[s.Name] != s.SHA256 || s.URL != "https://www.customs.go.jp/roo/english/text/"+s.Name || s.PDFPages < 1 {
			t.Errorf("source %+v", s)
		}
	}
	if raw.Agreement != "AJCEP" || raw.HS != 2017 || raw.Citation == "" || raw.Retrieved == "" {
		t.Fatalf("metadata %+v", raw)
	}
	// Window must equal the embedded AJCEP HS 2017 schedule.
	table := mustTable(t)
	var found *schedule
	for i, s := range table.schedules["AJCEP"] {
		if strings.Contains(s.Citation, "HS 2017") {
			found = &table.schedules["AJCEP"][i]
		}
	}
	if found == nil || raw.From != found.From || raw.Until != found.Until || found.Until == "" {
		t.Fatalf("bundle window %s..%s vs schedule %+v", raw.From, raw.Until, found)
	}
	if table.general.From != found.From || table.general.Until != found.Until {
		t.Fatal("parsed bundle window drifted")
	}
	// No private paths or process references in the public data.
	for _, bad := range []string{"/Users/", "/home/", ".claude", "superpowers", "dossier", "controller", "handoff", "webfetch"} {
		if strings.Contains(string(generalJSON), bad) {
			t.Errorf("public data contains %q", bad)
		}
	}
}

func TestPSRKeepsLegacyEvidenceOverDefault(t *testing.T) {
	table := mustTable(t)
	for _, hs := range []string{"0101", "0901.11", "6210.50", "52.11", "2103.90", "8703"} {
		d := day(t, "2025-01-01")
		old, _ := table.Lookup("AJCEP", hs, d)
		res, _ := table.LookupDetailed("AJCEP", hs, d)
		if len(old) == 0 {
			continue
		}
		if res.Reason != ReasonPSR || !reflect.DeepEqual(res.Evidence, old) {
			t.Errorf("%s: %+v vs %+v", hs, res, old)
		}
		// Only the listed Annex 2 provisions follow the rows; nothing else
		// of the general fallback (Article 26 and others, Annex 3, notes).
		allowed := map[string]bool{}
		for _, e := range table.general.psrEvidence() {
			allowed[e.Text] = true
		}
		for _, e := range res.Evidence {
			if (strings.Contains(e.URL, "asean1.pdf") || strings.Contains(e.URL, "asean3.pdf")) && !allowed[e.Text] {
				t.Errorf("general text appended to PSR for %s: %.60s", hs, e.Text)
			}
		}
	}
}

func TestNoGeneralDefaultOutsideAJCEPHS2017(t *testing.T) {
	table := mustTable(t)
	for _, a := range []string{"CPTPP", "JP-EU EPA", "JP-UK CEPA", "JP-Thailand EPA", "JP-Indonesia EPA", "JP-India CEPA", "JP-Australia EPA", "RCEP", "GSP", "JP-US TA", "Non-preferential", "ajcep", "AJCEP "} {
		for _, ds := range []string{"2025-01-01", "2027-02-15"} {
			for h := 1; h <= 9999; h++ {
				res, err := table.LookupDetailed(a, fmt.Sprintf("%04d", h), day(t, ds))
				if err != nil || res.Reason == ReasonGeneral || res.Reason == ReasonExcludedGeneral {
					t.Fatalf("%s %s %04d: %+v %v", a, ds, h, res.Reason, err)
				}
			}
		}
	}
	// AJCEP at or after the HS 2022 schedule's start never gets the default.
	for _, ds := range []string{"2027-02-15", "2028-01-01"} {
		for h := 1; h <= 9999; h++ {
			res, _ := table.LookupDetailed("AJCEP", fmt.Sprintf("%04d", h), day(t, ds))
			if res.Reason == ReasonGeneral || res.Reason == ReasonExcludedGeneral {
				t.Fatalf("AJCEP %s %04d: %+v", ds, h, res.Reason)
			}
		}
	}
}
