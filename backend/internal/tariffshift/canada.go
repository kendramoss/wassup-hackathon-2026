package tariffshift

import (
	"fmt"
	"strings"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
)

// canadaAfter applies the SOR/94-23 steps that come after the shared ones
// (D722): s. 9 (an answer of Canada for a good produced in another CUSMA
// country), and the assumptions a decided answer carries under s. 3(1),
// s. 4(1)(d) and s. 8.
func (r *Rules) canadaAfter(in Input, res Result) Result {
	g0 := r.g
	country := domain.NormalizeCountry(in.Country)
	if g0.ProductionOutside && country != "" && country != "Canada" && cusma[country] {
		if res.Status == Decided && res.Origin == "Canada" {
			res.Origin = country
			res.Basis += "; SOR/94-23 s. 9"
			res.Reason += fmt.Sprintf("; the good was then produced in %s, so s. 9 makes %s the origin", country, country)
			res.Assumes = append(res.Assumes, fmt.Sprintf("the work in %s is more than minor processing (s. 9)", country))
		}
		if c := res.IfNotDeMinimis; c != nil && c.Origin == "Canada" {
			cc := *c
			cc.Origin, cc.Basis = country, cc.Basis+"; SOR/94-23 s. 9"
			res.IfNotDeMinimis = &cc
		}
	}
	if res.Status != Decided {
		return res
	}
	if r.overridden(in) {
		res.Assumes = append(res.Assumes, "the note on this rule that sets origin by a process (for example a chemical reaction or where a plant was grown) does not apply")
	}
	if g0.PreferenceOverride && (strings.Contains(res.Origin, "/") || !cusma[res.Origin]) {
		res.Assumes = append(res.Assumes, "the good is not a CUSMA-originating good with a certificate of origin (s. 8 would then name the last CUSMA country of production)")
	}
	if g0.ImportedFrom != "" {
		res.Assumes = append(res.Assumes, g0.ImportedFrom)
	}
	return res
}

// overridden says whether the good's rule carries a note that can set the
// origin by process.
func (r *Rules) overridden(in Input) bool {
	rule, _, ok := r.Lookup(in.GoodHTS)
	return ok && rule.NoteOverrides
}
