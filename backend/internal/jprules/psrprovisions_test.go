package jprules

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

// The provisions returned after an AJCEP HS 2017 rule row, in order. Annex 2
// paragraphs 1-3 define the rule codes and cite Article 27 (RVC formula),
// Article 25 (WO) and Article 28(1)(b) (tolerance). Article 26 is left out on
// purpose: a listed rule applies in place of the general criteria of Article
// 26(1), and Annex 2 does not cite Article 26. Articles 24 (originating goods),
// 29 (accumulation) and 30 (non-qualifying operations) are returned since D916:
// they decide whether a material counts as originating and whether the work
// done counts.
var psrProvisionIDs = []string{
	"annex2-1-3a", "annex2-3b", "art24", "art25-intro", "art25-a-i", "art25-j-k",
	"art27-1-3", "art27-4-6", "art28-1a", "art28-1b-2", "art29", "art30-a-d", "art30-e-g",
}

func provisionEvidence(t *testing.T, b *generalBundle, id string) Evidence {
	t.Helper()
	all := b.evidence()
	for i, p := range b.Provisions {
		if p.ID == id {
			return all[i]
		}
	}
	t.Fatalf("no provision %s", id)
	return Evidence{}
}

func TestPSRHitReturnsAnnex2ProvisionsAfterRows(t *testing.T) {
	table := mustTable(t)
	b := table.general
	if fmt.Sprint(b.PSRProvisions) != fmt.Sprint(psrProvisionIDs) {
		t.Fatalf("psr_provisions = %v", b.PSRProvisions)
	}
	var tail []Evidence
	for _, id := range psrProvisionIDs {
		tail = append(tail, provisionEvidence(t, b, id))
	}
	if len(b.PSRNotes) != 1 {
		t.Fatalf("psr notes %d", len(b.PSRNotes))
	}
	tail = append(tail, Evidence{Text: b.PSRNotes[0].Text, Citation: b.PSRNotes[0].Citation})
	// RVC row, CTH row, CC rows with and without a para 3 tolerance, WO row, ch57.
	for _, hs := range []string{"0901.21", "1704.10", "1517", "1805.00", "1803.10", "2103.90", "2103.10", "6309.00", "5701.10"} {
		for _, ds := range []string{"2023-01-01", "2025-06-01", "2027-02-14"} {
			d := day(t, ds)
			res, err := table.LookupDetailed("AJCEP", hs, d)
			if err != nil || res.Reason != ReasonPSR {
				t.Fatalf("%s %s: %+v %v", hs, ds, res.Reason, err)
			}
			old, _ := table.Lookup("AJCEP", hs, d)
			if fmt.Sprint(old) != fmt.Sprint(res.Evidence) {
				t.Fatalf("%s: Lookup differs from LookupDetailed", hs)
			}
			n := len(res.Evidence) - len(tail)
			if n < 1 {
				t.Fatalf("%s: evidence %d", hs, len(res.Evidence))
			}
			if fmt.Sprint(res.Evidence[n:]) != fmt.Sprint(tail) {
				t.Fatalf("%s %s: tail differs", hs, ds)
			}
			if !strings.Contains(res.Evidence[0].URL, "asean2-1.pdf") || strings.Contains(res.Evidence[0].Text, "Article 28") {
				t.Fatalf("%s: first evidence is not the row: %+v", hs, res.Evidence[0])
			}
			// rows and notes come before every provision
			for _, e := range res.Evidence[:n] {
				for _, p := range b.Provisions {
					if e.Text == p.Text {
						t.Fatalf("%s: provision %s before the end", hs, p.ID)
					}
				}
			}
		}
	}
}

func TestPSRProvisionsAreScopedAndKeepSources(t *testing.T) {
	table := mustTable(t)
	res, _ := table.LookupDetailed("AJCEP", "2103.90", day(t, "2025-06-01"))
	all := ""
	for _, e := range res.Evidence {
		all += e.Text + "\n"
	}
	for _, frag := range []string{
		// Annex 2 para 3: exact code boundaries of the 10% and 7% tolerances.
		"subheadings 1803.10, 1803.20 and 1805.00 of the HS", "ten (10) per cent of the FOB",
		"subheading 2103.90 of the HS", "seven (7) per cent of the FOB",
		// Article 28: chapter boundaries and the FOB / weight bases.
		"Chapters 16, 19, 20, 22, 23, 28 through 49, and 64 through 97", "Chapters 18 and 21",
		"Chapters 50 through 63", "ten (10) per cent of the total weight",
		// Article 28 applies to a CTC rule only and the RVC rule still counts the value.
		"A good that does not satisfy the requirements of subparagraph 1(b) of Article 26 or an applicable CTC-based rule of origin set out in Annex 2",
		"shall, however, be included in the value of non-originating materials for any applicable RVC-based rule",
		// Article 27 formula and Article 25 wholly obtained.
		"RVC = ((FOB - VNM) / FOB) X 100 %", "Article 25 Goods Wholly Obtained or Produced",
	} {
		if !strings.Contains(all, frag) {
			t.Errorf("missing %q", frag)
		}
	}
	// Counterexamples: the general criteria and unrelated provisions are not returned.
	for _, frag := range []string{
		"Article 26 Goods Not Wholly Obtained",
		"No Annex 2 product specific rule row was found", "Information Technology",
	} {
		if strings.Contains(all, frag) {
			t.Errorf("unexpected %q", frag)
		}
	}
	last := res.Evidence[len(res.Evidence)-1]
	if last.URL != "" || !strings.HasPrefix(last.Text, "Unde Labs note (not treaty text): ") ||
		!strings.Contains(last.Text, "in place of the general criteria") {
		t.Fatalf("scope note: %+v", last)
	}
	for _, frag := range []string{"sets tolerance codes by reference to Article 28(1)(b)", "Under Article 26(2) a listed rule applies",
		"Article 24 (originating goods), Article 29 (accumulation) and Article 30 (non-qualifying operations) are supplied",
		"Article 31 (Direct Consignment) and Annex 4 are not supplied", "not supplied and have not been verified"} {
		if !strings.Contains(last.Text, frag) {
			t.Errorf("scope note lacks %q", frag)
		}
	}
	// The provision evidence keeps the bundle's citation, page and URL.
	for _, e := range res.Evidence {
		if strings.Contains(e.URL, "asean1.pdf") {
			if !strings.HasPrefix(e.Citation, "AJCEP Agreement, Chapter 3 (Rules of Origin), Article ") || !strings.Contains(e.Citation, ", PDF page ") {
				t.Errorf("citation %q", e.Citation)
			}
		}
	}
}

func TestFallbackUnchangedByPSRProvisions(t *testing.T) {
	table := mustTable(t)
	res, _ := table.LookupDetailed("AJCEP", "3215", day(t, "2025-06-01"))
	if res.Reason != ReasonGeneral || len(res.Evidence) != len(table.general.Provisions) {
		t.Fatalf("fallback: %v %d", res.Reason, len(res.Evidence))
	}
	for i, e := range table.general.evidence() {
		if res.Evidence[i] != e {
			t.Fatalf("fallback evidence %d changed", i)
		}
	}
	for _, e := range res.Evidence {
		if strings.Contains(e.Text, "in place of the general criteria") {
			t.Fatal("PSR scope note leaked into the fallback")
		}
	}
}

func TestNoPSRProvisionsOutsideAJCEPHS2017Window(t *testing.T) {
	table := mustTable(t)
	isBundle := func(e Evidence) bool {
		return strings.Contains(e.URL, "asean1.pdf") || strings.Contains(e.URL, "asean3.pdf") ||
			strings.Contains(e.Citation, "Annex 2 (HS 2017), paragraph") || strings.Contains(e.Text, "in place of the general criteria")
	}
	// HS 2022 window: rows only, the bundle is a documented gap.
	for _, hs := range []string{"0901.21", "1517", "2103.90", "6309.00"} {
		for _, ds := range []string{"2027-02-15", "2028-01-01"} {
			res, _ := table.LookupDetailed("AJCEP", hs, day(t, ds))
			if res.Reason != ReasonPSR {
				t.Fatalf("%s %s: %v", hs, ds, res.Reason)
			}
			for _, e := range res.Evidence {
				if isBundle(e) {
					t.Fatalf("%s %s: HS 2017 provision in the HS 2022 window: %+v", hs, ds, e)
				}
			}
		}
	}
	// Other agreements never see the bundle.
	for _, a := range []string{"CPTPP", "JP-EU EPA", "JP-UK CEPA", "JP-Thailand EPA", "JP-Indonesia EPA", "JP-Australia EPA", "RCEP", "GSP", "JP-US TA"} {
		for h := 101; h <= 9999; h += 7 {
			res, _ := table.LookupDetailed(a, fmt.Sprintf("%04d", h), day(t, "2025-06-01"))
			for _, e := range res.Evidence {
				if isBundle(e) {
					t.Fatalf("%s %04d: %+v", a, h, e)
				}
			}
		}
	}
}

func TestPSRProvisionsFieldValidation(t *testing.T) {
	table := mustTable(t)
	raw, err := json.Marshal(table.general)
	if err != nil {
		t.Fatal(err)
	}
	mutate := func(f func(m map[string]any)) error {
		var m map[string]any
		if err := json.Unmarshal(raw, &m); err != nil {
			t.Fatal(err)
		}
		f(m)
		b, _ := json.Marshal(m)
		_, err := parseGeneral(b, table.schedules)
		return err
	}
	if err := mutate(func(map[string]any) {}); err != nil {
		t.Fatalf("unmodified bundle rejected: %v", err)
	}
	note := func(id string) map[string]any {
		return map[string]any{"id": id, "citation": "Unde Labs note (not treaty text)", "page": 0, "kind": "note",
			"text": "Unde Labs note (not treaty text): x"}
	}
	bad := map[string]func(m map[string]any){
		"unknown id":      func(m map[string]any) { m["psr_provisions"] = []any{"annex2-1-3a", "nope"} },
		"duplicate id":    func(m map[string]any) { m["psr_provisions"] = []any{"annex2-1-3a", "annex2-1-3a"} },
		"empty id":        func(m map[string]any) { m["psr_provisions"] = []any{""} },
		"note in list":    func(m map[string]any) { m["psr_provisions"] = []any{"note-hs"} },
		"empty list":      func(m map[string]any) { m["psr_provisions"] = []any{} },
		"missing list":    func(m map[string]any) { delete(m, "psr_provisions") },
		"no psr note":     func(m map[string]any) { m["psr_notes"] = []any{} },
		"psr note dup id": func(m map[string]any) { m["psr_notes"] = []any{note("note-hs")} },
		"psr note treaty": func(m map[string]any) {
			n := note("x1")
			n["kind"], n["source"], n["page"] = "treaty", "asean1.pdf", 1
			m["psr_notes"] = []any{n}
		},
		"psr note prefix": func(m map[string]any) {
			n := note("x2")
			n["text"] = "plain"
			m["psr_notes"] = []any{n}
		},
		"psr note page": func(m map[string]any) {
			n := note("x3")
			n["page"] = 2
			m["psr_notes"] = []any{n}
		},
	}
	for name, f := range bad {
		if err := mutate(f); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}
