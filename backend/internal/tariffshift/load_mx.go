package tariffshift

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"sync"
)

// MexicoTableVersion is the version of rules_mx.json.
const MexicoTableVersion = "mx-acuerdo-1994-v1"

//go:embed rules_mx.json
var rulesMXJSON []byte

var (
	mxOnce  sync.Once
	mxRules *Rules
	mxErr   error
)

// MexicoRules returns Mexico's non-preferential origin table (rules_mx.json,
// Acuerdo DOF 30-08-1994, Anexo I Apéndice), applied through the Anexo I
// general rules (D914). A rules check only.
func MexicoRules() (*Rules, error) {
	mxOnce.Do(func() {
		var t Table
		if err := json.Unmarshal(rulesMXJSON, &t); err != nil {
			mxErr = fmt.Errorf("tariffshift: rules_mx.json: %w", err)
			return
		}
		if t.Regime != Mexico.ID {
			mxErr = fmt.Errorf("tariffshift: rules_mx.json regime %q, want %q", t.Regime, Mexico.ID)
			return
		}
		mxRules = New(t)
	})
	return mxRules, mxErr
}
