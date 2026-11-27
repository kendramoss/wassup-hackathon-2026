package aseanfta

import (
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/atiga"
)

var asean = map[string]string{
	"BN": "Brunei", "KH": "Cambodia", "ID": "Indonesia", "LA": "Laos",
	"MY": "Malaysia", "MM": "Myanmar", "PH": "Philippines", "SG": "Singapore",
	"TH": "Thailand", "VN": "Vietnam",
}

var aseanAliases = map[string]string{
	"brunei darussalam": "BN", "lao pdr": "LA", "lao people's democratic republic": "LA",
	"viet nam": "VN", "burma": "MM",
}

func with(extra map[string]string) map[string]string {
	m := map[string]string{}
	for k, v := range asean {
		m[k] = v
	}
	for k, v := range extra {
		m[k] = v
	}
	return m
}

func withAliases(extra map[string]string) map[string]string {
	m := map[string]string{}
	for k, v := range aseanAliases {
		m[k] = v
	}
	for k, v := range extra {
		m[k] = v
	}
	return m
}

// AKFTA Rule 8: the non-qualifying operations, by key.
var akftaMinimal = map[string]string{
	"preservation":       "preserving operations to ensure that the good remains in good condition during transport and storage",
	"packaging_change":   "changes of packaging, breaking-up and assembly of packages",
	"cleaning":           "simple washing, cleaning, removal of dust, oxide, oil, paint or other coverings",
	"painting_polishing": "simple painting and polishing operations",
	"cereal_husking":     "husking, partial or total bleaching, polishing and glazing of cereals and rice",
	"sugar":              "operations to colour sugar or form sugar lumps",
	"peeling":            "simple peeling, stoning or un-shelling",
	"cutting":            "sharpening, simple grinding or simple cutting",
	"sorting":            "sifting, screening, sorting, classifying, grading, matching",
	"packaging":          "simple placing in bottles, cans, flasks, bags, cases, boxes, fixing on cards or boards and all other simple packaging operations",
	"marking":            "affixing or printing marks, labels, logos and other like distinguishing signs on products or their packaging",
	"mixing":             "simple mixing of products, whether or not of different kinds",
	"assembly":           "simple assembly of parts of articles to constitute a complete article or disassembly of products into parts",
	"testing":            "simple testing or calibrations",
	"slaughtering":       "slaughtering of animals",
}

var akftaProcesses = map[string]string{
	"cut_and_sewn":    "the good is both cut and sewn in the territory of a Party",
	"printing_dyeing": "printing or dyeing is accompanied by at least two preparatory or finishing operations",
	"milk_limit":      "the good does not contain milk (or material from milk) imported from a non-Party above the stated share by weight of the total raw milk material",
	"glazed":          "the good is glazed from goods that are not glazed",
	"mco":             "the good is a multi-component integrated circuit made from goods that are not multi-component integrated circuits",
}

var acftaProcesses = map[string]string{
	"process_rule_1": "Process Rule 1: manufacturing through fibre-making (polymerisation, polycondensation and extrusion), spinning, twisting, texturising or braiding from a blend or any of silk, wool, fine or coarse animal hair, cotton fibres, vegetable textile fibres, synthetic or artificial filaments or staple fibres",
	"process_rule_2": "Process Rule 2: manufacturing from polymer or fibres (non-woven), yarns (fabrics) or raw or unbleached fabrics (finished fabrics) through needle punching, spin bonding or chemical bonding, weaving or knitting, crocheting, wadding or tufting, or dyeing or printing and finishing, impregnation, coating, covering or lamination",
	"process_rule_3": "Process Rule 3: manufacturing through cutting and assembly of parts into a complete article (apparel and tents), incorporating embroidery, embellishment or printing (made-up articles), from raw or unbleached fabric or finished fabric",
	"process_rule":   "a Process Rule of Attachment B (the table does not say which of Process Rules 1 to 3)",
	"animals_raised": "the goods are obtained from sheep, lambs or other animals raised in either Party",
	"fats_wo":        "the goods are manufactured from fats or oil wholly obtained in either Party",
}

// AKFTAProfile reads the engine as the ASEAN-Korea Annex 3 and Appendix 2.
var AKFTAProfile = &atiga.Profile{
	Name:    "AKFTA",
	Regime:  "akfta_preferential",
	Note:    "AKFTA preferential origin from the facts given (Annex 3 and the Appendix 2 product-specific rules in HS 2022). Not marking origin, not a ruling, not a Certificate of Origin (Form AK). Direct consignment (Rule 9), the industrial-zone treatment of Rule 6, packing materials, accessories, neutral elements and identical and interchangeable materials are not checked. The rules have not been reviewed by a customs professional.",
	Parties: with(map[string]string{"KR": "South Korea"}),
	Aliases: withAliases(map[string]string{"republic of korea": "KR", "korea": "KR", "south korea": "KR", "rok": "KR"}),
	Cites: atiga.Cites{
		Origin: "Rule 2", OriginWO: "Rule 2.1(a)", WO: "Rule 3", PE: "Rule 2 and 7",
		General: "Rule 4.1", Annex: "Appendix 2", RVC: "Rule 4.2", Method: "Rule 4.2, note 3",
		Cumulation: "Rule 7", Cumulate: "Rule 7", Minimal: "Rule 8", DeMinimis: "Rule 10",
		PartyWord: "Party", NotParty: "an AKFTA Party (an ASEAN Member State or Korea)", OnlyIn: "a Party",
	},
	Processes:       akftaProcesses,
	MinimalOps:      akftaMinimal,
	PackagingKeys:   []string{"packaging", "packaging_change"},
	AllowShortCodes: true,
	DeMinimis:       atiga.DeMinimisRule{Percent: 10, Textiles: "weight"},
	General: func(string) *atiga.Rule {
		return &atiga.Rule{
			Code:        "Rule 4.1",
			Description: "Goods not listed in Appendix 2",
			Concise:     "RVC40 or CTH",
			Full:        "A regional value content of not less than 40 percent of the FOB value; or a change in tariff classification at four-digit level (a change of tariff heading) of the Harmonized System",
			Alternatives: []atiga.Alternative{
				{Name: "RVC40", Requires: []atiga.Requirement{{Kind: "rvc", Threshold: 40}}},
				{Name: "CTH", Requires: []atiga.Requirement{{Kind: "ctc", Level: "heading"}}},
			},
		}
	},
}

// acftaCTH lists the chapters Art. 4(1)(b) gives a change of tariff heading
// as an alternative to RVC 40, and the headings within them it takes back
// (notes 3 to 5: RVC 40 only).
var acftaCTH = func() map[string]bool {
	m := map[string]bool{}
	add := func(from, to int) {
		for c := from; c <= to; c++ {
			m[twoDigits(c)] = true
		}
	}
	for _, c := range []int{25, 26, 28, 29, 31, 39} {
		add(c, c)
	}
	add(42, 49)
	add(57, 59)
	add(61, 62)
	add(64, 64)
	add(66, 71)
	add(73, 83)
	add(86, 86)
	add(88, 88)
	add(91, 97)
	return m
}()

func twoDigits(c int) string {
	return string([]byte{byte('0' + c/10), byte('0' + c%10)})
}

var acftaRVCOnlyHeadings = map[string]bool{
	"2901": true, "2902": true, "3105": true, "3901": true, "3902": true, "3903": true, "3907": true, "3908": true,
}

// ACFTAProfile reads the engine as the ASEAN-China Annex 1 (Upgrading
// Protocol) and Attachment B.
var ACFTAProfile = &atiga.Profile{
	Name:    "ACFTA",
	Regime:  "acfta_preferential",
	Note:    "ACFTA preferential origin from the facts given (Annex 1 of the Upgrading Protocol and Attachment B, HS 2017). Not marking origin, not a ruling, not a Certificate of Origin (Form E). Attachment B is written in HS 2017: for a line renumbered since, give the HS 2017 code. Direct consignment (Art. 8), packing, accessories, neutral elements and identical and interchangeable materials are not checked. The ACFTA 3.0 Upgrade Protocol is not read. The rules have not been reviewed by a customs professional.",
	Parties: with(map[string]string{"CN": "China"}),
	Aliases: withAliases(map[string]string{"prc": "CN", "people's republic of china": "CN", "mainland china": "CN"}),
	Cites: atiga.Cites{
		Origin: "Art. 2", OriginWO: "Art. 2(a)", WO: "Art. 3", PE: "Art. 2(b)",
		General: "Art. 4(1)", Annex: "Attachment B", RVC: "Art. 5", Method: "Art. 5",
		Cumulation: "Art. 6", Cumulate: "Art. 6", Minimal: "Art. 7", DeMinimis: "Art. 9",
		PartyWord: "Party", NotParty: "an ACFTA Party (an ASEAN Member State or China)", OnlyIn: "a Party",
	},
	Processes: acftaProcesses,
	MinimalOps: map[string]string{
		"preservation": "ensuring preservation of goods in good condition for the purposes of transport or storage",
		"shipment":     "facilitating shipment or transportation",
		"packaging":    "packaging or presenting goods for sale",
	},
	MinimalOnlyForWO: true,
	PackagingKeys:    []string{"packaging"},
	BuildDownOnly:    true,
	PE:               true,
	DeMinimis:        atiga.DeMinimisRule{Percent: 10, Textiles: "weight_or_value"},
	FinalProcess:     "Art. 4(1)(a) also needs the final process of production to be performed in a Party; produced_in is taken as that Party.",
	General: func(hs string) *atiga.Rule {
		cth := len(hs) >= 2 && acftaCTH[hs[:2]] && !(len(hs) >= 4 && acftaRVCOnlyHeadings[hs[:4]])
		r := &atiga.Rule{
			Code:        "Art. 4(1)",
			Description: "Goods not listed in Attachment B",
			Alternatives: []atiga.Alternative{
				{Name: "RVC40", Requires: []atiga.Requirement{{Kind: "rvc", Threshold: 40}}},
			},
		}
		r.Concise = "RVC40"
		r.Full = "A regional value content of not less than 40 per cent of FOB, calculated with the build-down formula of Art. 5, and the final process of production performed within a Party"
		if cth {
			r.Concise = "RVC40 or CTH"
			r.Full += "; or, for goods of the chapters listed in Art. 4(1)(b), all non-originating materials have undergone a change in tariff classification at the four-digit level (a change in tariff heading)"
			r.Alternatives = append(r.Alternatives, atiga.Alternative{Name: "CTH", Requires: []atiga.Requirement{{Kind: "ctc", Level: "heading"}}})
		}
		return r
	},
}
