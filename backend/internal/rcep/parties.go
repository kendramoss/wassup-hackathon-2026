package rcep

import (
	"strings"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
)

// Parties are the fifteen signatories of the RCEP Agreement, by ISO 3166
// alpha-2 code. A Party must have the Agreement in force with the importing
// country for a claim to work; that is the caller's check.
var Parties = map[string]string{
	"AU": "Australia", "BN": "Brunei", "KH": "Cambodia", "CN": "China",
	"ID": "Indonesia", "JP": "Japan", "KR": "South Korea", "LA": "Laos",
	"MY": "Malaysia", "MM": "Myanmar", "NZ": "New Zealand", "PH": "Philippines",
	"SG": "Singapore", "TH": "Thailand", "VN": "Vietnam",
}

var partyAliases = map[string]string{
	"brunei darussalam": "BN", "lao pdr": "LA", "lao people's democratic republic": "LA",
	"korea": "KR", "republic of korea": "KR", "viet nam": "VN",
}

// Party returns the ISO code of an RCEP Party from a name or code ("Viet
// Nam", "vn", "PRC"), or "" when s is not a Party.
func Party(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	if up := strings.ToUpper(s); len(up) == 2 {
		if _, ok := Parties[up]; ok {
			return up
		}
		return ""
	}
	if c, ok := partyAliases[strings.ToLower(s)]; ok {
		return c
	}
	n := domain.NormalizeCountry(s)
	for code, name := range Parties {
		if strings.EqualFold(n, name) {
			return code
		}
	}
	if c, ok := partyAliases[strings.ToLower(n)]; ok {
		return c
	}
	return ""
}
