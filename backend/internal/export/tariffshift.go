package export

import (
	"fmt"
	"strings"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
)

// TariffShiftLine is the draft's one paragraph on the 19 CFR 102.20 check
// of the setup it asks about (D441, D500), worded for a broker: what the
// rule gives on the component codes, which parts do not make the change,
// or why it could not be applied. confirm is the placeholder that ends it.
// "" when there is no check. On a substantial-transformation case it says
// the check is for reference only.
func TariffShiftLine(rc *domain.PlanRuleCheck, confirm string) string {
	if rc == nil || rc.Status == "" {
		return ""
	}
	var b strings.Builder
	b.WriteString("Tariff-shift check (19 C.F.R. § 102.20): ")
	if rc.Applies != "part102" {
		b.WriteString("for reference only, as this request is made under substantial transformation; ")
	}
	where := ""
	if rc.Country != "" {
		where = fmt.Sprintf(", with the good finished in %s", rc.Country)
	}
	switch rc.Status {
	case "shifts", "no_shift", "other_country":
		fmt.Fprintf(&b, "applied to the component codes in the bill of materials%s, the rule gives %s", where, rc.Origin)
		if basis := cfr(rc.Basis); basis != "" {
			fmt.Fprintf(&b, " (%s)", basis)
		}
		b.WriteString(".")
		if len(rc.Failing) > 0 {
			fmt.Fprintf(&b, " Components that do not make the required tariff change: %s.", strings.Join(rc.Failing, ", "))
		}
		if rc.Review {
			b.WriteString(" This differs from the earlier ruling cited in support of the change.")
		}
	default:
		if rc.Why == "de_minimis" && len(rc.Failing) > 0 {
			fmt.Fprintf(&b, "applied to the component codes in the bill of materials%s, %s do not make the required tariff change; the result turns on their value (19 C.F.R. § 102.13, de minimis), which this draft does not state.", where, strings.Join(rc.Failing, ", "))
		} else {
			fmt.Fprintf(&b, "not determined: %s.", undeterminedWords(rc.Label))
		}
	}
	return b.String() + " " + confirm
}

// cfr writes "19 CFR 102.11(a)(3)" as the letter does, "19 C.F.R. § 102.11(a)(3)".
func cfr(s string) string {
	return strings.ReplaceAll(strings.TrimSpace(s), "19 CFR ", "19 C.F.R. § ")
}

// undeterminedWords is the reason inside an undetermined label, "102.20
// undetermined (no code for 2 parts)".
func undeterminedWords(label string) string {
	if i, j := strings.Index(label, "("), strings.LastIndex(label, ")"); i >= 0 && j > i {
		return label[i+1 : j]
	}
	return "the codes given do not decide it"
}
