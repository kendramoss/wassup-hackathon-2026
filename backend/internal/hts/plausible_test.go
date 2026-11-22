package hts

import (
	"strings"
	"testing"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
)

// D471, the a12 case of the agent evaluation (D451): a CV axle (family
// vehicle_parts) got 1704 for its centre bar and 6401 for its CV boots.
// Both are flagged and kept in place; the plausible codes are not flagged.
func TestFlagImplausibleA12(t *testing.T) {
	res := PartsResult{
		Good: &GoodSuggestion{Candidates: []PartCandidate{{HTS: "8708.80.65"}}},
		Parts: []PartSuggestion{
			{Name: "CV boots", Candidates: []PartCandidate{{HTS: "6401.10.00"}, {HTS: "6401.92.90"}}},
			{Name: "centre bar", Candidates: []PartCandidate{{HTS: "1704.90.10"}, {HTS: "1704.90.35"}, {HTS: "7115.90"}}},
			{Name: "inner ball joint housing", Candidates: []PartCandidate{{HTS: "8482.99.05"}, {HTS: "8481.80.10"}}},
			{Name: "clamps", Candidates: []PartCandidate{{HTS: "7326.90.86"}}},
		},
	}
	shared := res.Parts[0].Candidates // as if from the cache
	n := FlagImplausible(&res, domain.Product{Name: "Constant Velocity (CV) Axle", Family: "vehicle_parts"})
	if n != 5 {
		t.Errorf("flagged %d, want 5 (two footwear, two sugar, one precious-metal codes)", n)
	}
	for i, want := range []bool{true, true} {
		if got := res.Parts[0].Candidates[i].Check != ""; got != want {
			t.Errorf("CV boots %d flagged %v", i, got)
		}
	}
	if c := res.Parts[1].Candidates[0]; !strings.Contains(c.Check, "Chapter 17 (sugars and sugar confectionery)") || !strings.Contains(c.Check, "check this code") || c.HTS != "1704.90.10" {
		t.Errorf("centre bar: %+v", c)
	}
	for _, p := range res.Parts[2:] {
		for _, c := range p.Candidates {
			if c.Check != "" {
				t.Errorf("%s %s flagged: %s", p.Name, c.HTS, c.Check)
			}
		}
	}
	if len(res.Parts[1].Candidates) != 3 {
		t.Error("a flagged code was removed")
	}
	if shared[0].Check != "" {
		t.Error("the cached slice was written to")
	}
}

func TestPlausibilityNote(t *testing.T) {
	for _, tc := range []struct {
		family    string
		good      int
		part, hts string
		flag      string // substring, "" for no flag
	}{
		{"vehicle_parts", 87, "seal rings", "4016.93", ""},
		{"medical_devices", 90, "arm cuff", "6307.90", ""},       // made-up textile articles are plausible
		{"medical_devices", 90, "isolation gown", "6210.10", ""}, // allowed for this family
		{"pcba_electronic_devices", 95, "buttons", "9504.50", ""},
		{"furniture", 94, "down filling", "0505.10", ""},
		{"vehicle_parts", 87, "grease", "2710.19", ""},
		{"hand_tools", 82, "blade", "6401.10", "Chapter 64 (footwear)"},
		// No family: food chapters are flagged only for a manufactured good.
		{"", 84, "seal", "1704.90", "far from the product's chapter 84"},
		{"", 22, "neutral grain alcohol", "2207", ""},
		{"", 0, "sugar", "1701.14", ""},
		// Materials.
		{"", 0, "steel balls", "3926.90", "says steel or iron, but chapter 39 is plastics"},
		{"", 0, "steel balls", "7326.90", ""},
		{"", 0, "steel balls", "8482.91", ""},
		{"", 0, "plastic casing", "9504.50", ""},
		{"", 0, "plastic-coated steel wire", "7217.20", ""},
		{"", 0, "rubber seal", "4016.93", ""},
		{"", 0, "aluminum pegs", "7326.90", "says aluminium"},
		{"", 0, "plywood substrate", "4412.31", ""},
		{"", 0, "bracket", "3926.90", ""},
		{"", 0, "x", "", ""},
		// Dev rows read by hand (D471): these were flagged by the first
		// version and are plausible codes.
		{"", 85, "chemicals (i.e. silver and aluminum pastes)", "7106.92", ""},
		{"", 85, "silver solder ribbon", "7106.92", ""},
		{"vehicle_parts", 87, "centre bar", "7115.90", "Chapter 71"},
		{"", 0, "Frame subassembly (stamped metal frame, mounting hardware, molded plastic parts)", "7326.90", ""},
		{"", 0, "Rubber-coated handle and top plate", "7326.90", ""},
		{"", 0, "glass epoxy sheets", "3921.90", ""},
		{"", 0, "glass polyester sheets", "3921.90", ""},
		{"", 0, "paper phenolic sheets", "3921.90", ""},
	} {
		got := PlausibilityNote(tc.family, tc.good, tc.part, tc.hts)
		if (tc.flag == "") != (got == "") || !strings.Contains(got, tc.flag) {
			t.Errorf("%s/%d %q %s: %q, want %q", tc.family, tc.good, tc.part, tc.hts, got, tc.flag)
		}
	}
}

func TestChapter(t *testing.T) {
	for in, want := range map[string]int{"8708.99.81": 87, "0505": 5, "1": 0, "": 0, "6401 10 00": 64} {
		if got := Chapter(in); got != want {
			t.Errorf("Chapter(%q) = %d, want %d", in, got, want)
		}
	}
}
