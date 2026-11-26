package tariffshift

// GeneralRules are the general sections a rule table is applied through:
// the order of tests, the de minimis rule and the citations the answer
// gives. The tariff-shift rows are data (Table.Rules); these are the parts
// of the regulation around them that differ between rule sets (D721).
//
// Two sets exist: the U.S. marking rules of 19 CFR Part 102 (the default,
// Table.Regime empty) and Canada's marking rules for goods imported from a
// CUSMA country, SOR/94-23 (Table.Regime "ca-sor-94-23", D720-D723).
type GeneralRules struct {
	// ID names the set: "us-19cfr102" or "ca-sor-94-23".
	ID string
	// Name is the regulation, for display.
	Name string
	// Table names the table of rows in reasons ("102.20", "Schedule III").
	Table string

	// Bases cite the step that decided the answer. Shift, DeMinimis and the
	// two essential-character bases are followed by the rule's ID.
	Domestic, Shift, DeMinimis, EssentialOne, EssentialAll string
	// Short references used inside reasons.
	TestA, DeMinimisRef, EssentialRef, EssentialOneRef, EssentialPick string
	// NonQualifying is the assumption every tariff-shift answer carries: the
	// work is more than the operations that do not count. %s is the country.
	NonQualifying string
	// NotSet is the assumption of an essential-character answer.
	NotSet string

	// DeMinimisExcluded are the chapters de minimis does not apply to.
	DeMinimisExcluded []string
	// ByWeight are the chapters whose de minimis test is by weight, not
	// value (Canada: chapters 50 to 63, SOR/94-23 s. 11(1)(a)).
	ByWeight []string

	// ProductionOutside applies SOR/94-23 s. 9: an answer of Canada for a
	// good produced in another CUSMA country becomes that country.
	ProductionOutside bool
	// PreferenceOverride notes SOR/94-23 s. 8 (not decided by code: it
	// needs a certificate of origin) on answers it could change.
	PreferenceOverride bool
	// ImportedFrom is an assumption every answer carries about where the
	// goods come from ("" for none).
	ImportedFrom string
}

// US is 19 CFR Part 102 as the package applied it before rule sets existed;
// its strings are the ones every committed U.S. answer carries.
var US = GeneralRules{
	ID:                "us-19cfr102",
	Name:              "19 CFR 102",
	Table:             "102.20",
	Domestic:          "19 CFR 102.11(a)(2)",
	Shift:             "19 CFR 102.11(a)(3); ",
	DeMinimis:         "19 CFR 102.13(a); 19 CFR 102.11(a)(3); ",
	EssentialOne:      "19 CFR 102.11(b)(1); 19 CFR 102.18(b)(1)(iii); ",
	EssentialAll:      "19 CFR 102.11(b)(1); 19 CFR 102.18(b)(1); ",
	TestA:             "102.11(a)",
	DeMinimisRef:      "102.13",
	EssentialRef:      "102.18(b)(1)",
	EssentialOneRef:   "102.18(b)(1)(iii)",
	EssentialPick:     "102.11(b)",
	NonQualifying:     "the work in %s is more than the operations 102.17 says do not count (for example simple packing, dilution or disassembly)",
	NotSet:            "the good is not a set or mixture (102.11(b) does not apply to sets)",
	DeMinimisExcluded: []string{"01", "02", "03", "04", "07", "08", "11", "12", "15", "17", "20"},
}

// Canada is SOR/94-23, the Determination of Country of Origin for the
// Purpose of Marking Goods (CUSMA Countries) Regulations. Each difference
// from US is a decision in docs/DECISIONS.md (D722).
var Canada = GeneralRules{
	ID:              "ca-sor-94-23",
	Name:            "SOR/94-23",
	Table:           "Schedule III",
	Domestic:        "SOR/94-23 s. 4(1)(b)",
	Shift:           "SOR/94-23 s. 4(1)(c); ",
	DeMinimis:       "SOR/94-23 s. 11(1); SOR/94-23 s. 4(1)(c); ",
	EssentialOne:    "SOR/94-23 s. 5(1); SOR/94-23 s. 2(2)(a); ",
	EssentialAll:    "SOR/94-23 s. 5(1); SOR/94-23 s. 2(2)(a); ",
	TestA:           "s. 4",
	DeMinimisRef:    "s. 11",
	EssentialRef:    "s. 2(2)(a)",
	EssentialOneRef: "s. 2(2)(a)",
	EssentialPick:   "s. 5(1)",
	NonQualifying:   "the work in %s is more than the operations s. 13 says do not count (a change in end use, disassembly, dilution, or collecting parts under GRI 2(a))",
	NotSet:          "the good is not a set or mixture (s. 5(1) does not apply to sets)",
	// s. 11(1)(b): "other than under any of Chapters 1 to 4, 6 to 8, 11,
	// 12, 15, 17 and 20". Chapter 6 is excluded here and not in 102.13(b).
	DeMinimisExcluded:  []string{"01", "02", "03", "04", "06", "07", "08", "11", "12", "15", "17", "20"},
	ByWeight:           chapterSpan(50, 63),
	ProductionOutside:  true,
	PreferenceOverride: true,
	ImportedFrom:       "the goods are imported into Canada from the United States or Mexico (s. 3(1))",
}

func chapterSpan(lo, hi int) []string {
	var out []string
	for c := lo; c <= hi; c++ {
		out = append(out, string([]byte{byte('0' + c/10), byte('0' + c%10)}))
	}
	return out
}

// Mexico is the Acuerdo on non-preferential country-of-origin rules (DOF
// 30-08-1994), Anexo I articles 3 to 7 with its Apéndice de Reglas
// Específicas (D914). It applies to goods from any country. Its text has no
// de minimis rule, so every chapter is excluded from de minimis.
var Mexico = GeneralRules{
	ID:                "mx-acuerdo-1994",
	Name:              "Acuerdo DOF 30-08-1994, Anexo I",
	Table:             "Apéndice de Reglas Específicas",
	Domestic:          "Anexo I, art. 3 I b)",
	Shift:             "Anexo I, art. 3 I c); ",
	DeMinimis:         "",
	EssentialOne:      "Anexo I, art. 3 III; Anexo I, art. 7 II; ",
	EssentialAll:      "Anexo I, art. 3 III; Anexo I, art. 7 II; ",
	TestA:             "art. 3 I",
	DeMinimisRef:      "none",
	EssentialRef:      "art. 7 II",
	EssentialOneRef:   "art. 7 II",
	EssentialPick:     "art. 3 III",
	NonQualifying:     "the work in %s is more than the operations art. 6 says do not count (a change in end use, disassembly, simple packing, dilution, or collecting parts under GRI 2(a))",
	NotSet:            "the good is not a set or mixture (art. 3 III does not apply to sets)",
	DeMinimisExcluded: chapterSpan(1, 97),
	ImportedFrom:      "the origin is for non-preferential purposes in Mexico (quotas, trade remedies, marking); a trade-agreement preference follows that agreement's own rules",
}

// generalFor returns the rule set a table names.
func generalFor(regime string) GeneralRules {
	switch regime {
	case Canada.ID:
		return Canada
	case Mexico.ID:
		return Mexico
	}
	return US
}

// cusma are the CUSMA countries as domain.NormalizeCountry writes them.
var cusma = map[string]bool{"Canada": true, "United States": true, "Mexico": true}
