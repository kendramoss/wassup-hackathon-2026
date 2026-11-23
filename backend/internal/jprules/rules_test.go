package jprules

import (
	"strings"
	"testing"
	"time"
)

func TestOfficialSchedules(t *testing.T) {
	table, err := Default()
	if err != nil {
		t.Fatal(err)
	}
	date := time.Date(2026, 11, 23, 0, 0, 0, 0, time.UTC)
	cases := []struct {
		name, agreement, hs, citation, fragment string
	}{
		{"wholly obtained materials", "JP-EU EPA", "0406.90-000", "Annex 3-B (HS 2017)", "materials of Chapter 4 used are wholly obtained"},
		{"product description branches", "JP-EU EPA", "1517.90", "Annex 3-B (HS 2017)", "Mixed vegetable oils not further processed"},
		{"original heading range", "CPTPP", "6404.11", "Annex 3-D (HS 2012)", "heading 64.04 from any other chapter"},
		{"textile annex overrides general", "CPTPP", "6204.43", "Annex 4-A (HS 2012)", "cut or knit to shape"},
		{"Asia treaty merged cell", "AJCEP", "6210.50", "Annex 2 (HS 2017)", "each of the non-originating materials is woven"},
		{"Asia treaty cross-page rule", "AJCEP", "52.11", "Annex 2 (HS 2017)", "is woven entirely in one or more of the Parties"},
		{"RCEP annex", "RCEP", "6404.11", "Annex 3A (HS 2022)", "RVC40"},
		{"India CEPA", "JP-India CEPA", "0101", "Annex 2 (HS 2007)", "wholly obtained"},
		{"Thailand EPA", "JP-Thailand EPA", "0101", "Annex 2 (HS 2017)", "CC"},
		{"Indonesia EPA", "JP-Indonesia EPA", "0101", "Annex 2 (HS 2017)", "CC"},
		{"Australia EPA", "JP-Australia EPA", "0101", "Annex 2 (HS 2012)", "CC"},
		{"UK CEPA", "JP-UK CEPA", "0101", "Annex 3-B (HS 2017)", "wholly obtained"},
		{"Vietnam EPA", "JP-Vietnam EPA", "0101", "Annex 2 (HS 2007)", "CC"},
		{"Malaysia EPA", "JP-Malaysia EPA", "0101", "Annex 2 (HS 2002)", "from any other chapter"},
		{"Mexico EPA", "JP-Mexico EPA", "0101", "Annex 4 (HS 2002)", "from any other chapter"},
		{"Philippines EPA", "JP-Philippines EPA", "0101", "Annex 2 (HS 2002)", "from any other chapter"},
		{"Switzerland EPA", "JP-Switzerland EPA", "0101", "Appendix 1 to Annex II (HS 2007)", "CC"},
		{"Chile EPA", "JP-Chile EPA", "0101", "Annex 2 (HS 2002)", "from any other chapter"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			evidence, err := table.Lookup(c.agreement, c.hs, date)
			if err != nil || len(evidence) == 0 {
				t.Fatalf("Lookup(%q, %q): %v, %v", c.agreement, c.hs, evidence, err)
			}
			got := evidence[0]
			if !strings.Contains(got.Citation, c.citation) ||
				!strings.Contains(got.Text, c.fragment) ||
				!strings.HasPrefix(got.URL, "https://") || !strings.Contains(got.URL, "#page=") {
				t.Fatalf("wrong rule: %+v", got)
			}
		})
	}
	t.Run("future AJCEP schedule activates on its listed date", func(t *testing.T) {
		date := time.Date(2027, 2, 15, 0, 0, 0, 0, time.UTC)
		evidence, err := table.Lookup("AJCEP", "0101", date)
		if err != nil || len(evidence) == 0 || !strings.Contains(evidence[0].Citation, "HS 2022") {
			t.Fatalf("future AJCEP: %v, %v", evidence, err)
		}
	})
	t.Run("product branches stay together", func(t *testing.T) {
		rules, err := table.Lookup("JP-EU EPA", "1517.90", date)
		if err != nil || len(rules) != 2 || !strings.Contains(rules[1].Text, "Others:") {
			t.Fatalf("got %v, %v", rules, err)
		}
	})
	t.Run("textile notes stay with the rule", func(t *testing.T) {
		rules, err := table.Lookup("AJCEP", "6210.50", date)
		if err != nil || len(rules) != 2 || !strings.Contains(rules[1].Text, "component that determines the tariff classification") {
			t.Fatalf("got %v, %v", rules, err)
		}
	})
}

func TestLookupWithoutApplicableRule(t *testing.T) {
	table, err := Default()
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct{ agreement, hs, date string }{
		{"GSP", "6404.11", "2026-11-23"},
		{"JP-US TA", "6404.11", "2026-11-23"},
		{"JP-EU EPA", "not an HS code 0406.90", "2026-11-23"},
		{"AJCEP", "9999", "2027-02-15"},
		{"JP-Switzerland EPA", "06.02", "2026-11-23"}, // multi-row cell with no mapped rule
		{"RCEP", "6404.11", "2022-12-31"},
		{"CPTPP", "0304", "2026-11-23"},    // per-subheading rules
		{"CPTPP", "8708.94", "2026-11-23"}, // needs separate vehicle appendix
		{"JP-EU EPA", "8708.94", "2026-11-23"},
	}
	for _, c := range cases {
		date, err := time.Parse(time.DateOnly, c.date)
		if err != nil {
			t.Fatal(err)
		}
		got, err := table.Lookup(c.agreement, c.hs, date)
		if err != nil || len(got) > 0 {
			t.Errorf("%+v: got %v, %v", c, got, err)
		}
	}
}
