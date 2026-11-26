package erpimport

import (
	"strings"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
)

// iso2 maps ISO 3166-1 alpha-2 codes, which SAP and NetSuite store for
// countries, to the names the engine reads. It covers the website's country
// table (domain.CountryTable) and the larger trading partners.
var iso2 = map[string]string{
	"AR": "Argentina", "AT": "Austria", "AU": "Australia", "BD": "Bangladesh", "BE": "Belgium", "BG": "Bulgaria",
	"BR": "Brazil", "CA": "Canada", "CH": "Switzerland", "CL": "Chile", "CN": "China", "CO": "Colombia",
	"CR": "Costa Rica", "CZ": "Czech Republic", "DE": "Germany", "DK": "Denmark", "DO": "Dominican Republic",
	"EE": "Estonia", "EG": "Egypt", "ES": "Spain", "FI": "Finland", "FR": "France", "GB": "United Kingdom",
	"GR": "Greece", "GT": "Guatemala", "HK": "Hong Kong", "HN": "Honduras", "HR": "Croatia", "HU": "Hungary",
	"ID": "Indonesia", "IE": "Ireland", "IL": "Israel", "IN": "India", "IT": "Italy", "JO": "Jordan", "JP": "Japan",
	"KH": "Cambodia", "KR": "South Korea", "LA": "Laos", "LK": "Sri Lanka", "LT": "Lithuania", "LU": "Luxembourg",
	"LV": "Latvia", "MA": "Morocco", "MM": "Myanmar", "MX": "Mexico", "MY": "Malaysia", "NI": "Nicaragua",
	"NL": "Netherlands", "NO": "Norway", "NZ": "New Zealand", "PE": "Peru", "PH": "Philippines", "PK": "Pakistan",
	"PL": "Poland", "PT": "Portugal", "RO": "Romania", "SA": "Saudi Arabia", "SE": "Sweden", "SG": "Singapore",
	"SI": "Slovenia", "SK": "Slovakia", "SV": "El Salvador", "TH": "Thailand", "TN": "Tunisia", "TR": "Turkey",
	"TW": "Taiwan", "UA": "Ukraine", "AE": "United Arab Emirates", "UK": "United Kingdom", "US": "United States",
	"VN": "Vietnam", "ZA": "South Africa",
}

// country reads an ERP country: an ISO alpha-2 code or a name. ok is false
// for a two-letter value that is not a known code (it is almost surely a code
// the table lacks, not a name).
func country(s string) (string, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", true
	}
	if len(s) == 2 {
		name, ok := iso2[strings.ToUpper(s)]
		if !ok {
			return "", false
		}
		return domain.NormalizeCountry(name), true
	}
	return domain.NormalizeCountry(s), true
}
