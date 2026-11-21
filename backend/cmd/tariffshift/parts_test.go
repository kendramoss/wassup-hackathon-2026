package main

import (
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

func TestEvalProductsHideCodes(t *testing.T) {
	t.Chdir("../..")
	ps, err := evalProducts("dev")
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
		if codeLike.MatchString(p.Product.Description) || p.Product.HTS != "" {
			t.Errorf("%s: a code reaches the suggester", p.Ruling)
		}
		for _, c := range p.Product.Components {
			if c.HTS != "" {
				t.Errorf("%s: part code given", p.Ruling)
			}
		}
	}
	t.Logf("dev products: %d Part 102, %d family", usmca, fam)
	if usmca < 50 || fam != 23 {
		t.Errorf("products = %d, %d", usmca, fam)
	}
}
