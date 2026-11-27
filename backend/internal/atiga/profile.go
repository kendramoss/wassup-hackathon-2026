package atiga

import (
	"sort"
	"strings"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
)

// Cites are the article references the engine prints in its reasons, so one
// engine can speak for each agreement's own text.
type Cites struct {
	Origin     string // origin criteria ("Art. 26")
	OriginWO   string // the wholly-obtained limb of the origin criteria
	WO         string // wholly obtained goods ("Art. 27")
	PE         string // produced only from originating materials
	General    string // the general rule for goods the table does not list
	Annex      string // the product-specific rules ("Annex 3")
	RVC        string // regional value content ("Art. 29")
	Method     string // the single-method rule
	Cumulation string // originating materials from other Parties
	Cumulate   string // the cumulation rule as cited in \"count as originating\"
	Partial    string // partial cumulation
	Minimal    string // minimal operations
	DeMinimis  string // de minimis
	PartyWord  string // "Member State" or "Party"
	NotParty   string // "an ATIGA Member State"
	OnlyIn     string // "a Member State"
}

// DeMinimis says how the agreement's de minimis rule measures a good.
type DeMinimisRule struct {
	// Percent of the FOB value (or of the weight) the materials that fail a
	// tariff-shift requirement may reach.
	Percent float64
	// Textiles: how goods of chapters 50 to 63 are measured: "" (value, like
	// every other good), "weight" (weight only) or "weight_or_value".
	Textiles string
}

// Profile is one agreement's reading of the shared engine: its parties, its
// cumulation, de minimis and minimal-operation rules and its general rule.
// ATIGA is the original; the ASEAN-Korea and ASEAN-China profiles (package
// aseanfta) reuse the same table format and checks.
type Profile struct {
	// Name is the agreement ("ATIGA", "AKFTA", "ACFTA").
	Name string
	// Regime labels every result so it cannot be read as marking origin.
	Regime string
	// Note goes with every result.
	Note string
	// Parties by ISO 3166 alpha-2 code, with the aliases accepted for names.
	Parties map[string]string
	Aliases map[string]string
	Cites   Cites
	// Processes are the conditions written in words the caller states.
	Processes map[string]string
	// MinimalOps are the operations that confer nothing.
	MinimalOps map[string]string
	// MinimalOnlyForWO: the agreement's minimal-operations rule speaks only to
	// wholly obtained goods (ACFTA Art. 7); otherwise a good whose only work is
	// minimal is not originating.
	MinimalOnlyForWO bool
	// PackagingKeys are the MinimalOps keys that mean packaging or presenting
	// goods for sale, for "except when the change is only through packaging".
	PackagingKeys []string
	// BuildDownOnly: only the indirect formula exists (ACFTA Art. 5).
	BuildDownOnly bool
	// PartialCumulation enables the ATIGA Art. 30.2 / Annex 6 rule.
	PartialCumulation bool
	// PE: a good made only from originating materials of the Parties
	// qualifies by that alone (ATIGA Art. 26 and 30.1, ACFTA Art. 2(b)).
	PE        bool
	DeMinimis DeMinimisRule
	// AllowShortCodes lets table rows be written at chapter (2 digits) or
	// heading (4 digits) level, as the AKFTA Appendix 2 does.
	AllowShortCodes bool
	// General gives the rule for a good the table does not list.
	General func(hs string) *Rule
	// FinalProcess: RVC-only rules need the last process in a Party (ACFTA).
	FinalProcess string
}

// ATIGAProfile is the ATIGA reading, the engine's default.
var ATIGAProfile = &Profile{
	Name:    "ATIGA",
	Regime:  Regime,
	Note:    Note,
	Parties: Members,
	Aliases: memberAliases,
	Cites: Cites{
		Origin: "Art. 26", OriginWO: "Art. 26(a)", WO: "Art. 27", PE: "Art. 26 and 30.1",
		General: "Art. 28.1", Annex: "Annex 3", RVC: "Art. 29", Method: "Art. 29.3",
		Cumulation: "Art. 30", Cumulate: "Art. 30.1", Partial: "Art. 30.2, Annex 6", Minimal: "Art. 31", DeMinimis: "Art. 33",
		PartyWord: "Member State", NotParty: "an ATIGA Member State", OnlyIn: "a Member State",
	},
	Processes:         Processes,
	MinimalOps:        MinimalOperations,
	PackagingKeys:     []string{"packaging"},
	PartialCumulation: true,
	PE:                true,
	DeMinimis:         DeMinimisRule{Percent: 10},
	General:           func(string) *Rule { return &GeneralRule },
}

// member returns the ISO code of a Party from a name or code, or "".
func (p *Profile) member(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	if up := strings.ToUpper(s); len(up) == 2 {
		if _, ok := p.Parties[up]; ok {
			return up
		}
		return ""
	}
	if c, ok := p.Aliases[strings.ToLower(s)]; ok {
		return c
	}
	n := domain.NormalizeCountry(s)
	for code, name := range p.Parties {
		if strings.EqualFold(n, name) {
			return code
		}
	}
	if c, ok := p.Aliases[strings.ToLower(n)]; ok {
		return c
	}
	return ""
}

// Member returns the ISO code of a Party of the profile's agreement from a
// name or code, or "".
func (p *Profile) Member(s string) string { return p.member(s) }

// ProcessKeys lists the profile's process conditions, sorted.
func (p *Profile) ProcessKeys() []string {
	var ks []string
	for k := range p.Processes {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return ks
}
