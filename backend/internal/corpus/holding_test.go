package corpus

import (
	"reflect"
	"testing"
)

// Expected origins and evidence were produced by extract()+choose() in
// hackathon-research/experiments/origin-proof/scripts/label.py on the same
// texts, so the Go port labels as eval/data/labels.jsonl was labelled. (On
// the 2,696 regex rows of labels.jsonl the port gives the same primary origin
// for every ruling; checked 17 Oct 2026 with the raw corpus.)
func TestLabelHoldingMatchesResearchLabeller(t *testing.T) {
	for _, tt := range []struct {
		name, text, origin string
		evidence           []string
		multi              bool
	}{
		{"holding wins", "Dear Ms. Lee: The bolt seals are made in Mexico from steel wire of China (heading 7217). LAW AND ANALYSIS: Section 102.11(a)(3) applies. The wire undergoes the tariff shift. Accordingly, the country of origin of the high security bolt seal for marking purposes will be Mexico. HOLDING: The country of origin of the bolt seals is Mexico.",
			"Mexico", []string{"HOLDING: The country of origin of the bolt seals is Mexico."}, true},
		{"hypotheticals and contentions skipped", "Dear Sir: Parts from Japan are assembled in Canada. ANALYSIS: You contend that the origin is Canada. If the parts were made in Canada, the origin would be Canada. Therefore, the country of origin of the camera is Japan.",
			"Japan", []string{"Therefore, the country of origin of the camera is Japan."}, true},
		{"alternatives chained", "Dear Sir: Kits are packed in Mexico from parts of Taiwan and Vietnam. ANALYSIS: Section 102.11(c) applies. Therefore, the country of origin of the kit is Taiwan and Vietnam.",
			"Taiwan | Vietnam", []string{"Therefore, the country of origin of the kit is Taiwan and Vietnam."}, true},
		{"two products, two origins: no primary", "Dear Sir: Product A is from Canada; product B from China. ANALYSIS: Thus, the country of origin of product A is Canada. Thus, the country of origin of product B is China.",
			"", []string{"ANALYSIS: Thus, the country of origin of product A is Canada.", "Thus, the country of origin of product B is China."}, true},
		{"negation skipped", "Dear Sir: The drill is made in China and packed in Mexico. ANALYSIS: Packing is not a tariff shift. The country of origin of the drill is not Mexico. Accordingly, China is the country of origin of the drill.",
			"China", []string{"Accordingly, China is the country of origin of the drill."}, true},
		{"abbreviations do not end a sentence", "Dear Sir: Goods of the U.S. are processed in Canada. ANALYSIS: See Corp. v. United States. Based on the facts, the country of origin of the resin is the U.S. and no marking is required.",
			"United States", []string{"Based on the facts, the country of origin of the resin is the U.S. and no marking is required."}, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, meta := SplitFacts(tt.text)
			l := LabelHolding("N123456", "2025-03-01T00:00:00", "ny", "s", tt.text, meta)
			if l.Origin() != tt.origin || !reflect.DeepEqual(l.Evidence, tt.evidence) {
				t.Errorf("origin %q evidence %q, want %q %q", l.Origin(), l.Evidence, tt.origin, tt.evidence)
			}
			if l.Date != "2025-03-01" || l.Collection != "NY" || l.Method != "regex" || l.Regime != "part102_tariff_shift" || l.MultiCountry != tt.multi {
				t.Errorf("label fields = %+v", l)
			}
		})
	}
}

func TestUSMCARegime(t *testing.T) {
	for _, tt := range []struct {
		name string
		text string
		want bool
	}{
		{"tariff shift applied", "Under 19 CFR 102.11(a)(3), each foreign material undergoes the change in tariff classification of 102.20.", true},
		{"essential character", "The rules in 102.1 through 102.18 and 102.20 apply. Under 102.11(b) the motor imparts the essential character.", true},
		{"textile rule applied", "Section 102.11 does not apply to textiles. Under 102.21(c)(2) the fabric was formed in China; tariff shift.", false},
		{"cited, not applied", "See 19 CFR 102.20 generally. The good is substantially transformed in Mexico.", false},
		{"no Part 102", "The good is substantially transformed in Mexico.", false},
	} {
		if got := USMCARegime(tt.text); got != tt.want {
			t.Errorf("%s: USMCARegime = %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestTextileTariffs(t *testing.T) {
	for in, want := range map[string]bool{
		"6302.21.9020":               true,
		"5208.11, 6307.90.9889":      true,
		"6307.90.9889, 8309.90.0080": false,
		"8309.90.0080":               false,
		"":                           false,
	} {
		if got := TextileTariffs(in); got != want {
			t.Errorf("TextileTariffs(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestUSMCALabelsShipWithFacts(t *testing.T) {
	labels, err := LoadLabels("../../data/labels_usmca.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	s, err := Load(labels, "../../data/facts")
	if err != nil {
		t.Fatal(err)
	}
	withOrigin := 0
	for _, l := range labels {
		if l.Origin() != "" {
			withOrigin++
			if _, ok := s.Get(l.Ruling); !ok {
				t.Errorf("%s: no facts in the image's data", l.Ruling)
			}
		}
	}
	if withOrigin < 150 || s.Len() != withOrigin {
		t.Errorf("%d USMCA rulings with an origin, %d in the store", withOrigin, s.Len())
	}
}
