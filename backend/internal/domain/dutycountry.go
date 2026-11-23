package domain

import "strings"

// dutyAliases maps the lower-cased NormalizeCountry output of a spelling to the
// canonical name duty pricing uses. NormalizeCountry only title-cases a name its
// alias table does not know, so these spellings reach it as "Dprk", "Uae",
// "Korea South" and so on. Only spellings verified for #718 are listed; this is
// not a world country dictionary.
var dutyAliases = map[string]string{
	"dprk":                                  "North Korea",
	"democratic people's republic of korea": "North Korea",
	"korea north":                           "North Korea",
	"republic of cuba":                      "Cuba",
	"byelorussia":                           "Belarus",
	"uae":                                   "United Arab Emirates",
	"chinese taipei":                        "Taiwan",
	"taiwan china":                          "Taiwan",
	"korea south":                           "South Korea",
}

// DutyCountry is the identity of a country for duty pricing: the Column 2
// guard, the country-wide additional duties, the missing-rate check and the
// rate lookup. It is NormalizeCountry plus the verified spellings above, mapped
// to the canonical name. NormalizeCountry itself, and the origin facts and
// legal checks built on it, are unchanged. An "A/B" origin is mapped per part.
func DutyCountry(s string) string {
	n := NormalizeCountry(s)
	if n == "" {
		return ""
	}
	parts := strings.Split(n, "/")
	for i, p := range parts {
		if c, ok := dutyAliases[strings.ToLower(p)]; ok {
			parts[i] = c
		}
	}
	return strings.Join(parts, "/")
}
