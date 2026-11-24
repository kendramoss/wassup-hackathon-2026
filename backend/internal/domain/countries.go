package domain

// countryNames is the country table the website's country picker uses
// (frontend/src/lib/countries.ts, the keys of countryCodes; a test keeps the
// two equal). Aliases are kept as written there; KnownCountry normalises.
var countryNames = []string{
	"Argentina", "Australia", "Austria", "Bangladesh", "Belgium", "Brazil", "Cambodia", "Canada", "Chile", "China", "Colombia",
	"Costa Rica", "Czech Republic", "Czechia", "Denmark", "Dominican Republic", "European Union", "Egypt", "El Salvador", "Finland",
	"France", "Germany", "Guatemala", "Honduras", "Hong Kong", "Hungary", "India", "Indonesia", "Ireland", "Israel", "Italy", "Japan",
	"Jordan", "Korea", "South Korea", "Republic of Korea", "Laos", "Malaysia", "Mexico", "Morocco", "Netherlands", "New Zealand",
	"Nicaragua", "Norway", "Pakistan", "Peru", "Philippines", "Poland", "Portugal", "Romania", "Singapore", "Slovakia", "Slovenia",
	"South Africa", "Spain", "Sri Lanka", "Sweden", "Switzerland", "Taiwan", "Thailand", "Tunisia", "Turkey", "Ukraine",
	"United Arab Emirates", "United Kingdom", "UK", "United States", "USA", "Vietnam", "Viet Nam",
}

// CountryTable is the picker's table, as written.
func CountryTable() []string { return append([]string(nil), countryNames...) }

var knownCountries = func() map[string]bool {
	m := map[string]bool{}
	for _, n := range countryNames {
		if n == "European Union" { // a customs union, not a country where a part is made
			continue
		}
		m[NormalizeCountry(n)] = true
	}
	return m
}()

// KnownCountry reports whether s names a country of the table (after
// NormalizeCountry), and returns its canonical name.
func KnownCountry(s string) (string, bool) {
	n := NormalizeCountry(s)
	return n, n != "" && knownCountries[n]
}
