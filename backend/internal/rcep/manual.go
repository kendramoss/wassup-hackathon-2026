package rcep

// Hand encodings of the eight Annex 3A rows the parser leaves as "manual"
// (eval/rcep/parse_psr.py lists them). Seven split the rule by what the
// product is; the caller picks the variant. Heading 38.08 adds a condition
// in words. Load refuses to start if a row's text no longer matches, so a
// new table version cannot silently reuse a stale encoding.

type manualRule struct {
	text         string
	alternatives []Alternative
	variants     []Variant
}

var (
	altCC    = Alternative{Criterion: "CC"}
	altCTH   = Alternative{Criterion: "CTH"}
	altRVC40 = Alternative{Criterion: "RVC", Threshold: 40}
	altCR    = Alternative{Criterion: "CR"}
)

const carpList = "Carp (Catla catla, Osteochilus hasselti, Leptobarbus hoeveni, Cyprinus spp.(except Cyprinus carpio), Carassius spp.(except Carassius carassius), Labeo spp., Megalobrama spp.)"

var insectVariants = []Variant{
	{ID: "insect", Label: "of insects", Alternatives: []Alternative{altCC, altRVC40}},
	{ID: "other", Label: "any other good", Alternatives: []Alternative{altCC}},
}

var manualRules = map[string]manualRule{
	"0304.39": {
		text: "1. CTH for " + carpList + ", 2. CC for any other good",
		variants: []Variant{
			{ID: "carp", Label: "carp of the listed species", Alternatives: []Alternative{altCTH}},
			{ID: "other", Label: "any other fish", Alternatives: []Alternative{altCC}},
		},
	},
	"0304.69": {
		text: "1. CC or RVC40 for " + carpList + " , 2. CC for any other good",
		variants: []Variant{
			{ID: "carp", Label: "carp of the listed species", Alternatives: []Alternative{altCC, altRVC40}},
			{ID: "other", Label: "any other fish", Alternatives: []Alternative{altCC}},
		},
	},
	"1601.00": {text: "1. CC or RVC40 for of insect 2. CC for any other good", variants: insectVariants},
	"1602.10": {text: "1. CC or RVC40 for of insect 2. CC for any other good", variants: insectVariants},
	"1602.90": {text: "1. CC or RVC40 for of insect 2. CC for any other good", variants: insectVariants},
	"2404.19": {
		text: "1. CTH except from heading 24.03 for manufactured tobacco substitutes 2. CTH except from heading 38.24, or RVC40 for any other good",
		variants: []Variant{
			{ID: "tobacco_substitute", Label: "manufactured tobacco substitutes", Alternatives: []Alternative{
				{Criterion: "CTH", Except: []Range{{Level: "heading", From: "24.03", To: "24.03"}}},
			}},
			{ID: "other", Label: "any other good", Alternatives: []Alternative{
				{Criterion: "CTH", Except: []Range{{Level: "heading", From: "38.24", To: "38.24"}}}, altRVC40,
			}},
		},
	},
	"2909.60": {
		text: "1. CTH, or RVC40 for that of acetal and hemiacetal peroxide 2. CTH, or RVC40, or CR for others",
		variants: []Variant{
			{ID: "acetal_peroxide", Label: "acetal and hemiacetal peroxides", Alternatives: []Alternative{altCTH, altRVC40}},
			{ID: "other", Label: "other peroxides", Alternatives: []Alternative{altCTH, altRVC40, altCR}},
		},
	},
	"38.08": {
		text: "CTSH provided that at least 50 % by weight of the active ingredient or ingredients is originating, or RVC40",
		alternatives: []Alternative{
			{Criterion: "CTSH", Condition: "active_ingredient_50pct"}, altRVC40,
		},
	},
}
