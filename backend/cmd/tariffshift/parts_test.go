package main

import (
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/hts"
)

func TestProductName(t *testing.T) {
	for in, want := range map[string]string{
		"The classification and country of origin of a wall mounted wine rack kit with vinyl skin": "wall mounted wine rack kit with vinyl skin",
		"Country of Origin Marking; proMESH":                              "proMESH",
		"The tariff classification and country of origin of solar panels": "solar panels",
		"The country of origin of a manual rack and pinion gear":          "manual rack and pinion gear",
	} {
		if got := productName(in); got != want {
			t.Errorf("productName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestGoldFor(t *testing.T) {
	st := []hts.Statement{
		{Part: "aluminum pegs", HTS: "8302.50"},
		{Part: "pegs", HTS: "8302.50.00"},
		{Part: "acrylic panel", HTS: "3920"},
		{Part: "panel", HTS: "4411"},
	}
	if g, ok, amb := goldFor("Aluminum peg", st); !ok || amb || g.HTS != "8302.50.00" {
		t.Errorf("peg = %+v %v %v (agreeing codes: the longest)", g, ok, amb)
	}
	if _, ok, amb := goldFor("acrylic panel", st); ok || !amb {
		t.Errorf("acrylic panel matched %v, ambiguous %v: 3920 and 4411 disagree", ok, amb)
	}
	if _, ok, _ := goldFor("vinyl skin", st); ok {
		t.Errorf("vinyl skin has no statement")
	}
}

func TestCodeMatch(t *testing.T) {
	for _, tt := range []struct {
		s, g string
		want bool
	}{{"8302.50.00", "8302.50", true}, {"8302.50", "8302.50.00", false}, {"8302.10", "8302", true}, {"8473.30.51", "8473.30.5100", true}, {"8473.30.11", "8473.30.5100", false}} {
		if got := codeMatch(tt.s, tt.g); got != tt.want {
			t.Errorf("codeMatch(%s, %s) = %v", tt.s, tt.g, got)
		}
	}
}

// leakLike finds what is left of an HTS number after stripCodes, written
// independently of it: a dotted code anywhere, a number next to a heading
// word or before ", HTSUS", or a number listed right after a removed code.
var leakLike = regexp.MustCompile(`(?i)\d{4}\.\d{2}|headings?\s*\d{4}|\b\d{4}\b,\s+HTSU?S?\b|\[code removed\],?\s*(?:and\s+|or\s+)?\d{4}\b`)

// No committed dev or held-out product reaches the suggester with an HTS
// number in its name or description (D381; #576).
func TestEvalProductsHideCodes(t *testing.T) {
	t.Chdir("../..")
	for _, split := range []string{"dev", "final"} {
		ps, err := evalProducts(split)
		if err != nil {
			t.Fatal(err)
		}
		usmca, fam := 0, 0
		for _, p := range ps {
			if p.Set == "usmca" {
				usmca++
			} else {
				fam++
			}
			if m := leakLike.FindString(p.Product.Description + " " + p.Product.Name); m != "" || p.Product.HTS != "" {
				t.Errorf("%s %s: a code reaches the suggester: %q", split, p.Ruling, m)
			}
			for _, c := range p.Product.Components {
				if c.HTS != "" {
					t.Errorf("%s: part code given", p.Ruling)
				}
			}
		}
		t.Logf("%s products: %d Part 102, %d family", split, usmca, fam)
		if split == "dev" && (usmca < 50 || fam != 23) || split == "final" && (usmca != 29 || fam != 0) {
			t.Errorf("%s products = %d, %d", split, usmca, fam)
		}
	}
}

// #576: codes glued to "heading"/"subheading" and every code of a list are
// removed.
func TestStripCodes(t *testing.T) {
	for in, want := range map[string]string{
		"is classifiable under subheading8419.34.0000, Harmonized": "is classifiable under [code removed], Harmonized",
		"outside of headings 8539 and 8540, Harmonized":            "outside of [code removed], Harmonized",
		"headings 3926, 4015, 4016, and 8484, HTSUS":               "[code removed], HTSUS",
		"or 4015, 4016, and 8484, HTSUS":                           "or [code removed], HTSUS",
		"classified in subheading 8501.31.4000, HTSUS":             "classified in [code removed], HTSUS",
		"under HTSUS8419.34 and":                                   "under HTSUS[code removed] and",
		"subheadings 8486.10 through 8486.40.":                     "[code removed].",
		"a 1200 watt motor, 2024 model, 3 heading angles":          "a 1200 watt motor, 2024 model, 3 heading angles",
		"not part of the 2024 HTSUS.":                              "not part of the 2024 HTSUS.",
	} {
		if got := stripCodes(in); got != want {
			t.Errorf("stripCodes(%q) = %q, want %q", in, got, want)
		}
	}
}

// The committed inputs that carried an HTS number (#576): exactly these,
// found by comparing the v1 stripper with the current one.
func TestInputCodesOfTheCommittedRuns(t *testing.T) {
	t.Chdir("../..")
	want := map[string]string{
		"dev":   "H342474:8540 N339851:4421 N343893:4016,8484 N352810:3914",
		"final": "H351116:9802.00.40,9802.00.80,5405 H355020:3914,3914 N357063:3914 N363600:8419.34.0000 N364586:3914",
	}
	for split, w := range want {
		m, err := inputCodes(split)
		if err != nil {
			t.Fatal(err)
		}
		var got []string
		for id, cs := range m {
			got = append(got, id+":"+strings.Join(cs, ","))
		}
		sort.Strings(got)
		if g := strings.Join(got, " "); g != w {
			t.Errorf("%s: %s, want %s", split, g, w)
		}
	}
}
