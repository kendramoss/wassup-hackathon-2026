package tariffshift

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"sync"
)

// CanadaTableVersion is the version of rules_ca.json. Bump it when the
// rows, the parse or the notes change.
const CanadaTableVersion = "ca-sor-94-23-v1"

//go:embed rules_ca.json
var rulesCAJSON []byte

var (
	caOnce  sync.Once
	caRules *Rules
	caErr   error
)

// CanadaRules returns Canada's committed marking-rule table (rules_ca.json,
// SOR/94-23 Schedule III), applied through the SOR/94-23 general rules
// (D720-D723). A rules check only: no Canadian rulings test it.
func CanadaRules() (*Rules, error) {
	caOnce.Do(func() {
		var t Table
		if err := json.Unmarshal(rulesCAJSON, &t); err != nil {
			caErr = fmt.Errorf("tariffshift: rules_ca.json: %w", err)
			return
		}
		if t.Regime != Canada.ID {
			caErr = fmt.Errorf("tariffshift: rules_ca.json regime %q, want %q", t.Regime, Canada.ID)
			return
		}
		caRules = New(t)
	})
	return caRules, caErr
}

// Coverage counts what a table encodes.
type Coverage struct {
	Rows         int `json:"rows"`
	RowsFull     int `json:"rows_fully_encoded"`
	RowsPartial  int `json:"rows_partly_encoded"`
	RowsNone     int `json:"rows_not_encoded"`
	Alternatives int `json:"alternatives"`
	Encoded      int `json:"encoded"`
}

// CoverageOf counts the rows and alternatives of t that the parser encoded.
func CoverageOf(t Table) Coverage {
	var c Coverage
	for _, r := range t.Rules {
		c.Rows++
		enc := 0
		for _, a := range r.Alternatives {
			c.Alternatives++
			if a.Encoded {
				enc++
			}
		}
		c.Encoded += enc
		switch {
		case enc == len(r.Alternatives) && enc > 0:
			c.RowsFull++
		case enc == 0:
			c.RowsNone++
		default:
			c.RowsPartial++
		}
	}
	return c
}
