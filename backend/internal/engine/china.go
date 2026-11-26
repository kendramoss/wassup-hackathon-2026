package engine

import (
	"regexp"
	"sort"
	"strings"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
)

// ChinaVersion names ChinaRules; a prediction for a case ChinaCase names,
// made with them, records PromptVersion+"+"+ChinaVersion (D860, #831).
const ChinaVersion = "china-v1"

// ChinaRules is CBP practice on goods made from Chinese parts or materials
// and worked on in another country, added to the system prompt only for a
// case ChinaCase names. It was written from development rulings in both
// directions (D860), so a development result is not independent.
const ChinaRules = `CBP practice when parts or materials come from China and work is done in another country (or the reverse):
1. Origin is the last country whose processing gives the good its name, character and use. It is not where most parts, the costliest part or the raw material was made, and not the country a trade measure is aimed at.
2. Work that makes the working article moves origin there: building the mechanism, motor or circuit board (SMT assembly) and integrating it, forming and machining the main body from raw stock, sintering or chemically making a new material, or assembling many parts in a complex, skilled process. Air purifiers, showerheads, gear-motor devices, filters and tiaras assembled this way in Vietnam, Thailand or Cambodia from Chinese parts took the assembly country.
3. Simple work does not: putting finished items into a kit or retail set, joining a few finished parts with screws or clips, adding a cord, housing or bracket, cutting, coating, testing, labelling and packing. Then origin stays where the component that does the article's main job was made complete, often China. Filter media pleated, a sensing element wired, or a lock assembled abroad around a complete Chinese core kept China.
4. Weigh each country's work as the facts describe it. When precedents for the same kind of good point both ways, follow the one whose work in each country is closest to these facts.`

// chinaRe and otherCountryRe find China and any other country of the table
// (the United States, the usual importing country, does not count).
var (
	chinaRe        = regexp.MustCompile(`\b(?:China|Chinese)\b`)
	otherCountryRe = func() *regexp.Regexp {
		var names []string
		for _, n := range domain.CountryTable() {
			if n != "China" && n != "United States" {
				names = append(names, regexp.QuoteMeta(n))
			}
		}
		sort.Slice(names, func(i, j int) bool { return len(names[i]) > len(names[j]) })
		return regexp.MustCompile(`\b(?:` + strings.Join(names, "|") + `)\b`)
	}()
)

// ChinaCase reports whether the facts name China and another country.
// Deterministic and facts only.
func ChinaCase(c domain.Case) bool {
	return chinaRe.MatchString(c.Facts) && otherCountryRe.MatchString(c.Facts)
}

// ChinaCaseRules is Options.CaseRules for china-v1: ChinaRules for a case
// ChinaCase names, "" for any other.
func ChinaCaseRules(c domain.Case) string {
	if ChinaCase(c) {
		return ChinaRules
	}
	return ""
}
