package textilerules

import (
	"embed"
	"encoding/json"
	"fmt"
	"sync"
)

//go:embed rules.json source/ecfr-102.21.xml source/fr-87-68356-6201-6208.txt
var files embed.FS

// SourceInfo is the provenance written into rules.json.
var SourceInfo = Source{
	ECFR: "19 CFR 102.21, eCFR point in time 18 Nov 2026 (the Part 102 file of D360), public domain; source/ecfr-102.21.xml",
	FR:   "87 FR 68356, 15 Nov 2022 (CBP Dec. 22-25), the entry for 6201-6208 that the eCFR could not incorporate (its editorial note); source/fr-87-68356-6201-6208.txt",
	Note: "The text is the regulation's; the alternatives, conditions and requirements are parsed from it by Build, and a test re-parses the committed text so rules.json cannot drift.",
}

// Source files, for the builder and the drift test.
func SourceFiles() (ecfr, fr []byte, err error) {
	if ecfr, err = files.ReadFile("source/ecfr-102.21.xml"); err != nil {
		return nil, nil, err
	}
	fr, err = files.ReadFile("source/fr-87-68356-6201-6208.txt")
	return ecfr, fr, err
}

var (
	once sync.Once
	tbl  *Rules
	terr error
)

// Default is the committed table.
func Default() (*Rules, error) {
	once.Do(func() {
		b, err := files.ReadFile("rules.json")
		if err != nil {
			terr = err
			return
		}
		var t Table
		if err := json.Unmarshal(b, &t); err != nil {
			terr = fmt.Errorf("textilerules: rules.json: %w", err)
			return
		}
		tbl = &Rules{T: t}
	})
	return tbl, terr
}

// Rules evaluates cases against a Table.
type Rules struct{ T Table }
