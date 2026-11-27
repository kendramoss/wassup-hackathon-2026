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

// The D956 error cases: 2019-2020 New York rulings on knit-to-shape garments
// whose panels are knit in Hong Kong and linked in China. CBP holds Hong
// Kong; the legacy labeller (Hong Kong written as China) labelled them China.
func TestLabelHoldingTextileKeepsHongKong(t *testing.T) {
	for _, tt := range []struct {
		name, text, textile, legacy string
	}{
		{"hat knit in Hong Kong, linked in China (N303311)",
			"Dear Mr. Lee: The fabric that forms the hat is knit to shape in Hong Kong. The knit fabric from Hong Kong is sent to China where it is linked and looped to form the hat. The determination of the country of origin of style LU9AEHS is to be governed by 19 C.F.R. 102.21(c)(3)(i), wherein the country of origin of the good is the single country, territory, or insular possession in which the good was knit.The country of origin of the hat submitted as style LU9AEHS is Hong Kong.The applicable subheading will be 6505.00.1540.",
			"Hong Kong", "China"},
		{"two products, one holding sentence each (pullover and hat)",
			"Dear Mr. Lee: The pullover is knit to shape in Hong Kong and assembled in China. ANALYSIS: Under 19 C.F.R. 102.21(c)(3)(i) the good is the product of the place where it was knit. HOLDING: The country of origin of the women's knit to shape pullover is Hong Kong. The country of origin of the hat is Hong Kong.",
			"Hong Kong", "China"},
		{"Hong Kong and China are different holdings",
			"Dear Mr. Lee: The pullover is knit in Hong Kong. The scarf is woven and assembled in China. ANALYSIS: Under 19 C.F.R. 102.21 the pullover is of one place and the scarf of another. HOLDING: The country of origin of the pullover is Hong Kong. The country of origin of the scarf is China.",
			"", "China"},
		{"China stays China",
			"Dear Mr. Lee: The top is cut and sewn in China from fabric woven in China. ANALYSIS: Under 19 C.F.R. 102.21(c)(2) the country of origin of the top is China.",
			"China", "China"},
		{"Macau and Taiwan are their own places",
			"Dear Mr. Lee: The cardigan is knit to shape in Macau and linked in China. ANALYSIS: Under 19 C.F.R. 102.21(c)(3)(i) the country of origin of the cardigan is Macau.",
			"Macau", "Macau"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, meta := SplitFacts(tt.text)
			l := LabelHoldingTextile("N303311", "2019-04-01T00:00:00", "ny", "s", tt.text, meta)
			if l.Origin() != tt.textile {
				t.Errorf("textile label = %q, want %q (evidence %q)", l.Origin(), tt.textile, l.Evidence)
			}
			if old := LabelHolding("N303311", "2019-04-01T00:00:00", "ny", "s", tt.text, meta); old.Origin() != tt.legacy {
				t.Errorf("legacy label = %q, want %q: the legacy labeller must not change", old.Origin(), tt.legacy)
			}
		})
	}
}

// Holdings on several goods and holdings that depend on a scenario are not a
// single origin (D957). Each text is cut down from the D956 ruling named.
func TestLabelHoldingTextileMultipleHoldings(t *testing.T) {
	for _, tt := range []struct {
		name, text, want string
	}{
		{"one sentence per good, the second without a cue (N307920)",
			"Dear Ms. Lee: Towels and a pet bowl mat are made abroad. ANALYSIS: Under 19 C.F.R. 102.21 the tariff shift applies. HOLDING: The country of origin of the microfiber towel and chenille door mat is China. The country of origin of the pet bowl mat is Vietnam. In the event there is a change in the facts previously furnished, this may affect the determination of country of origin.",
			""},
		{"two goods, no good named, different places (N352382)",
			"Dear Ms. Lee: Tents and an insert are made abroad. ANALYSIS: Under 19 C.F.R. 102.21 the tariff shift applies. HOLDING: The applicable subheading for the tents will be 6306.22, HTSUS, and the country of origin is China. The applicable subheading for the mesh insert will be 6307.90.9891, HTSUS, and the country of origin is Vietnam.",
			""},
		{"one place under both scenarios is one holding (N343157)",
			"Dear Ms. Lee: A blanket is made abroad. ANALYSIS: Under 19 C.F.R. 102.21 the tariff shift applies. HOLDING: The applicable subheading for the hooded blanket will be 6307.90.9891, HTSUS, and the country of origin is China under both proposed manufacturing scenarios.",
			"China"},
		{"a place per numbered scenario (N347636)",
			"Dear Ms. Lee: A mat is made abroad. ANALYSIS: Under 19 C.F.R. 102.21 the tariff shift applies. HOLDING: The applicable subheading for the mat will be 5702.42.2080, HTSUS, and the country of origin is Vietnam under Scenario 1 and China under Scenarios 2 and 3.",
			""},
		{"a sentence per numbered scenario (N339406)",
			"Dear Ms. Lee: A bathmat is made abroad. ANALYSIS: Under 19 C.F.R. 102.21 the tariff shift applies. HOLDING: The country of origin of the bathmat under scenario 1 is Vietnam. Under scenario 2, the country of origin of the bathmat is China; as a result, Section 301 remedies apply.",
			""},
		{"a place per condition (N302194)",
			"Dear Ms. Lee: Pillowcases are made abroad. ANALYSIS: Under 19 C.F.R. 102.21 the tariff shift applies. HOLDING: The country of origin of the pillowcases is conferred in India when the fabric is woven in India and in Pakistan when the fabric is woven in Pakistan.",
			""},
		{"is conferred in, one good (N304732)",
			"Dear Ms. Lee: A mattress pad is made abroad. ANALYSIS: Under 19 C.F.R. 102.21 the tariff shift applies. Accordingly, the country of origin of the mattress pad is conferred in China.",
			"China"},
		{"the US as a holding (N361490)",
			"Dear Ms. Lee: Lures are made abroad. ANALYSIS: Under 19 C.F.R. 102.21 the tariff shift applies. Therefore, the country of origin for the Plug Lures is Canada. The powder coating does not transform the blanks, therefore the country of origin for the Spoon Lures is the US.",
			""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, meta := SplitFacts(tt.text)
			l := LabelHoldingTextile("N300000", "2019-04-01T00:00:00", "ny", "s", tt.text, meta)
			if l.Origin() != tt.want {
				t.Errorf("origin = %q, want %q (evidence %q)", l.Origin(), tt.want, l.Evidence)
			}
		})
	}
}
